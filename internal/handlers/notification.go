package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"windshift/internal/cacheutil"
	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/sanitize"
	"windshift/internal/services"
	"windshift/internal/utils"

	"github.com/allegro/bigcache/v3"
)

// NotificationManagerConfig holds tuning parameters for the notification manager.
type NotificationManagerConfig struct {
	FlushInterval time.Duration // Deprecated: read-state writes are synchronous
	MaxBatchSize  int           // Maximum notifications per multi-row INSERT chunk
	SyncInterval  time.Duration // Deprecated: cache entries are immutable DB views
	MaxCacheSize  int           // Hard cache limit in MiB
}

// DefaultNotificationManagerConfig returns a config with sensible defaults.
func DefaultNotificationManagerConfig() NotificationManagerConfig {
	return NotificationManagerConfig{
		FlushInterval: 30 * time.Second,
		MaxBatchSize:  50,
		SyncInterval:  2 * time.Minute,
		MaxCacheSize:  123,
	}
}

// NotificationManager handles notification caching and persistence
type NotificationManager struct {
	cache           *bigcache.BigCache
	db              database.Database
	stopOnce        sync.Once
	insertChunkSize int

	// Cache and state mutations are serialized per exact user. Slow work for
	// one user never takes a cross-user global lock.
	userLocks      sync.Map // map[int]*sync.Mutex
	beforeUserLock func(int)

	// pushDispatcher, when set, receives every newly-created notification for
	// fan-out to Web Push. It owns a bounded queue and worker pool.
	dispatcherMu   sync.RWMutex
	pushDispatcher PushDispatcher

	insertBatches       int64
	inserted            int64
	insertErrors        int64
	insertDurationNS    int64
	lastInsertDuration  int64
	maxInsertDuration   int64
	cacheUpdates        int64
	cacheErrors         int64
	cacheBytesWritten   int64
	maxCacheEntryBytes  int64
	cacheUpdateDuration int64
	lastCacheDuration   int64
	maxCacheDuration    int64
	pushRejected        int64
}

const (
	notificationCachePageSize = 100
	notificationInsertTimeout = 15 * time.Second
)

// SetPushDispatcher wires the Web Push dispatcher. Safe to call once at startup
// before the manager handles traffic; nil leaves push disabled.
func (nm *NotificationManager) SetPushDispatcher(d PushDispatcher) {
	nm.dispatcherMu.Lock()
	defer nm.dispatcherMu.Unlock()
	nm.pushDispatcher = d
}

// NotificationService interface for cache management
type NotificationService interface {
	ForceRefreshCache() error
}

// PushDispatcher delivers a freshly-created notification to the user's
// registered Web Push subscriptions. Implemented by services.PushService.
// Kept as an interface here so the notification manager stays decoupled from
// the push transport (and so push is a no-op when none is wired).
type PushDispatcher interface {
	Enqueue(notification models.Notification) bool
	Close(context.Context) error
}

// NotificationHandler handles HTTP requests for notifications
type NotificationHandler struct {
	manager          *NotificationManager
	service          NotificationService
	permService      *services.PermissionService
	notificationAuth *services.NotificationAuthorizer
}

// NewNotificationManager creates a new notification manager with BigCache
func NewNotificationManager(db database.Database, nmCfg NotificationManagerConfig) (*NotificationManager, error) {
	if nmCfg.MaxCacheSize <= 0 {
		nmCfg.MaxCacheSize = DefaultNotificationManagerConfig().MaxCacheSize
	}
	cache, err := cacheutil.New("notifications", cacheutil.BigCacheOptions{
		TTL:               24 * time.Hour,
		MaxCacheMB:        nmCfg.MaxCacheSize,
		MaxEntrySize:      64 * 1024,
		Shards:            16,
		InitialCapacityMB: 4,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create BigCache: %w", err)
	}

	insertChunkSize := nmCfg.MaxBatchSize
	if insertChunkSize <= 0 {
		insertChunkSize = DefaultNotificationManagerConfig().MaxBatchSize
	}
	manager := &NotificationManager{
		cache:           cache,
		db:              db,
		insertChunkSize: insertChunkSize,
	}

	return manager, nil
}

// NewNotificationHandler creates a new notification handler
func NewNotificationHandler(manager *NotificationManager, service NotificationService, permissionServices ...*services.PermissionService) *NotificationHandler {
	handler := &NotificationHandler{manager: manager, service: service}
	if len(permissionServices) > 0 {
		handler.permService = permissionServices[0]
	}
	handler.notificationAuth = services.NewNotificationAuthorizer(manager.db, handler.permService, nil)
	return handler
}

// SetNotificationAuthorizer wires current workspace and asset authorization
// checks into notification tray reads.
func (nh *NotificationHandler) SetNotificationAuthorizer(authorizer *services.NotificationAuthorizer) {
	if authorizer != nil {
		nh.notificationAuth = authorizer
	}
}

// getCacheKey generates a cache key for a user's notifications
func (nm *NotificationManager) getCacheKey(userID int) string {
	return fmt.Sprintf("user:%d:notifications", userID)
}

func (nm *NotificationManager) userLock(userID int) *sync.Mutex {
	lock, _ := nm.userLocks.LoadOrStore(userID, &sync.Mutex{})
	userLock, ok := lock.(*sync.Mutex)
	if !ok {
		panic("notification user lock has unexpected type")
	}
	return userLock
}

func (nm *NotificationManager) lockUsers(userIDs []int) func() {
	users := make(map[int]struct{}, len(userIDs))
	for _, userID := range userIDs {
		users[userID] = struct{}{}
	}
	ordered := make([]int, 0, len(users))
	for userID := range users {
		ordered = append(ordered, userID)
	}
	sort.Ints(ordered)
	for _, userID := range ordered {
		if nm.beforeUserLock != nil {
			nm.beforeUserLock(userID)
		}
		nm.userLock(userID).Lock()
	}
	return func() {
		for i := len(ordered) - 1; i >= 0; i-- {
			nm.userLock(ordered[i]).Unlock()
		}
	}
}

func (nm *NotificationManager) cacheSnapshot(userID int) (models.NotificationCache, bool) {
	entry, err := nm.cache.Get(nm.getCacheKey(userID))
	if err != nil {
		return models.NotificationCache{}, false
	}
	var cache models.NotificationCache
	if err := json.Unmarshal(entry, &cache); err != nil {
		_ = nm.cache.Delete(nm.getCacheKey(userID))
		atomic.AddInt64(&nm.cacheErrors, 1)
		return models.NotificationCache{}, false
	}
	return cache, true
}

func (nm *NotificationManager) setCacheSnapshot(userID int, cache models.NotificationCache) error {
	startedAt := time.Now()
	cache.LastSynced = startedAt
	cache.IsDirty = false
	data, err := json.Marshal(cache)
	if err != nil {
		atomic.AddInt64(&nm.cacheErrors, 1)
		return err
	}
	if err := nm.cache.Set(nm.getCacheKey(userID), data); err != nil {
		atomic.AddInt64(&nm.cacheErrors, 1)
		return err
	}
	atomic.AddInt64(&nm.cacheUpdates, 1)
	atomic.AddInt64(&nm.cacheBytesWritten, int64(len(data)))
	duration := time.Since(startedAt)
	atomic.AddInt64(&nm.cacheUpdateDuration, int64(duration))
	atomic.StoreInt64(&nm.lastCacheDuration, int64(duration))
	updateNotificationMaximum(&nm.maxCacheDuration, int64(duration))
	updateNotificationMaximum(&nm.maxCacheEntryBytes, int64(len(data)))
	return nil
}

func sliceNotificationPage(notifications []models.Notification, limit, offset int) []models.Notification {
	if offset >= len(notifications) {
		return []models.Notification{}
	}
	end := min(offset+limit, len(notifications))
	return append([]models.Notification(nil), notifications[offset:end]...)
}

// GetUserNotifications retrieves notifications from a compact first-page cache.
// Requests beyond that fixed page go directly to SQL rather than treating a
// partial cache entry as the entire inbox.
func (nm *NotificationManager) GetUserNotifications(userID, limit, offset int) ([]models.Notification, error) {
	lock := nm.userLock(userID)
	lock.Lock()
	defer lock.Unlock()

	if cache, ok := nm.cacheSnapshot(userID); ok {
		if offset+limit <= len(cache.Notifications) || cache.Complete {
			return sliceNotificationPage(cache.Notifications, limit, offset), nil
		}
	}

	if offset+limit <= notificationCachePageSize {
		notifications, err := nm.queryNotifications(userID, notificationCachePageSize, 0)
		if err != nil {
			return nil, err
		}
		cache := models.NotificationCache{
			Notifications: notifications,
			Complete:      len(notifications) < notificationCachePageSize,
		}
		if err := nm.setCacheSnapshot(userID, cache); err != nil {
			slog.Warn("failed to cache notification page", "component", "notifications", "user_id", userID, "error", err)
		}
		return sliceNotificationPage(notifications, limit, offset), nil
	}

	return nm.queryNotifications(userID, limit, offset)
}

// AddNotification is the single-recipient wrapper around the same durable bulk
// insertion path used by event fan-out.
func (nm *NotificationManager) AddNotification(notification models.Notification) (models.Notification, error) {
	stored, err := nm.AddNotifications([]models.Notification{notification})
	if err != nil {
		return notification, err
	}
	if len(stored) == 0 {
		return notification, nil
	}
	return stored[0], nil
}

// AddNotifications atomically inserts a recipient batch with bounded multi-row
// statements, then refreshes only already-warm compact cache pages. Push work is
// enqueued after commit and never creates a goroutine per notification.
func (nm *NotificationManager) AddNotifications(notifications []models.Notification) ([]models.Notification, error) {
	ctx, cancel := context.WithTimeout(context.Background(), notificationInsertTimeout)
	defer cancel()
	return nm.AddNotificationsContext(ctx, notifications)
}

// AddNotificationsContext is the cancellable bulk insertion surface used by
// bounded background workers during shutdown.
func (nm *NotificationManager) AddNotificationsContext(ctx context.Context, notifications []models.Notification) ([]models.Notification, error) {
	if len(notifications) == 0 {
		return []models.Notification{}, nil
	}
	candidates := append([]models.Notification(nil), notifications...)
	userIDs := make([]int, len(candidates))
	now := time.Now()
	for i := range candidates {
		if err := validateNotificationAuthorizationScope(candidates[i]); err != nil {
			return nil, err
		}
		candidates[i].CreatedAt = now
		candidates[i].UpdatedAt = now
		if candidates[i].Timestamp.IsZero() {
			candidates[i].Timestamp = now
		}
		userIDs[i] = candidates[i].UserID
	}

	unlockUsers := nm.lockUsers(userIDs)
	startedAt := time.Now()
	ctx, cancel := context.WithTimeout(ctx, notificationInsertTimeout)
	defer cancel()
	tx, err := nm.db.BeginTx(ctx, nil)
	if err != nil {
		unlockUsers()
		atomic.AddInt64(&nm.insertErrors, 1)
		return nil, fmt.Errorf("begin notification insert: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stored := make([]models.Notification, 0, len(candidates))
	for start := 0; start < len(candidates); start += nm.insertChunkSize {
		end := min(start+nm.insertChunkSize, len(candidates))
		inserted, err := insertNotificationChunk(ctx, tx, candidates[start:end])
		if err != nil {
			unlockUsers()
			atomic.AddInt64(&nm.insertErrors, 1)
			return nil, err
		}
		stored = append(stored, inserted...)
	}
	if err := tx.Commit(); err != nil {
		unlockUsers()
		atomic.AddInt64(&nm.insertErrors, 1)
		return nil, fmt.Errorf("commit notification insert: %w", err)
	}
	atomic.AddInt64(&nm.insertBatches, 1)
	atomic.AddInt64(&nm.inserted, int64(len(stored)))
	duration := time.Since(startedAt)
	atomic.AddInt64(&nm.insertDurationNS, int64(duration))
	atomic.StoreInt64(&nm.lastInsertDuration, int64(duration))
	updateNotificationMaximum(&nm.maxInsertDuration, int64(duration))

	byUser := make(map[int][]models.Notification)
	for _, notification := range stored {
		byUser[notification.UserID] = append(byUser[notification.UserID], notification)
	}
	for userID, additions := range byUser {
		cache, ok := nm.cacheSnapshot(userID)
		if !ok {
			continue
		}
		sort.SliceStable(additions, func(i, j int) bool {
			if additions[i].Timestamp.Equal(additions[j].Timestamp) {
				return additions[i].ID > additions[j].ID
			}
			return additions[i].Timestamp.After(additions[j].Timestamp)
		})
		cache.Notifications = append(additions, cache.Notifications...)
		if len(cache.Notifications) > notificationCachePageSize {
			cache.Notifications = cache.Notifications[:notificationCachePageSize]
			cache.Complete = false
		}
		if err := nm.setCacheSnapshot(userID, cache); err != nil {
			slog.Warn("failed to update notification cache after insert", "component", "notifications", "user_id", userID, "error", err)
			_ = nm.cache.Delete(nm.getCacheKey(userID))
		}
	}
	unlockUsers()

	nm.dispatcherMu.RLock()
	dispatcher := nm.pushDispatcher
	nm.dispatcherMu.RUnlock()
	if dispatcher != nil {
		for _, notification := range stored {
			if !dispatcher.Enqueue(notification) {
				atomic.AddInt64(&nm.pushRejected, 1)
			}
		}
	}
	return stored, nil
}

func validateNotificationAuthorizationScope(notification models.Notification) error {
	switch notification.AuthorizationScope {
	case models.NotificationScopeSystem:
		if notification.WorkspaceID != nil {
			return fmt.Errorf("system notification cannot carry workspace authorization scope")
		}
	case models.NotificationScopeWorkspace:
		if notification.WorkspaceID == nil || *notification.WorkspaceID <= 0 {
			return fmt.Errorf("workspace notification requires workspace provenance")
		}
	case models.NotificationScopeAsset:
		if notification.SourceID == nil || *notification.SourceID <= 0 {
			return fmt.Errorf("asset notification requires source provenance")
		}
	default:
		return fmt.Errorf("notification requires trusted authorization scope")
	}
	hasReference := notification.ReferencedEntityType != "" || notification.ReferencedEntityID != nil ||
		notification.ReferencedWorkspaceID != nil || notification.ReferencedWorkspacePermission != ""
	requiresReference := notification.SourceType == models.EventItemLinked || notification.SourceType == models.EventItemUnlinked
	if hasReference || requiresReference {
		if notification.ReferencedEntityType == "" || notification.ReferencedEntityID == nil || *notification.ReferencedEntityID <= 0 || notification.ReferencedWorkspacePermission == "" {
			return fmt.Errorf("linked notification requires referenced authorization provenance")
		}
		if notification.ReferencedEntityType != "asset" && (notification.ReferencedWorkspaceID == nil || *notification.ReferencedWorkspaceID <= 0) {
			return fmt.Errorf("linked workspace notification requires referenced workspace provenance")
		}
	}
	return nil
}

func insertNotificationChunk(ctx context.Context, tx database.Tx, notifications []models.Notification) ([]models.Notification, error) {
	values := make([]string, len(notifications))
	args := make([]any, 0, len(notifications)*21)
	for i, notification := range notifications {
		values[i] = "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"
		args = append(args,
			notification.UserID, notification.Title, notification.Message, notification.Type,
			notification.Timestamp, notification.Read, nullableString(notification.Avatar),
			nullableString(notification.ActionURL), nullableString(notification.Metadata),
			notification.AuthorizationScope, notification.WorkspaceID, notification.ItemID,
			nullableString(notification.SourceType), notification.SourceID,
			nullableString(notification.ReferencedEntityType), notification.ReferencedEntityID,
			notification.ReferencedWorkspaceID, nullableString(notification.ReferencedWorkspacePermission),
			nullableString(notification.DeliveryKey),
			notification.CreatedAt, notification.UpdatedAt,
		)
	}
	query := `
		INSERT INTO notifications (user_id, title, message, type, timestamp, read, avatar, action_url, metadata,
			authorization_scope, workspace_id, item_id, source_type, source_id,
			referenced_entity_type, referenced_entity_id, referenced_workspace_id, referenced_workspace_permission,
			delivery_key, created_at, updated_at)
		VALUES ` + strings.Join(values, ",") + `
		ON CONFLICT DO NOTHING
		RETURNING id, delivery_key`
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("insert notification batch: %w", err)
	}
	defer rows.Close()
	byDeliveryKey := make(map[string]int, len(notifications))
	for i := range notifications {
		if notifications[i].DeliveryKey != "" {
			byDeliveryKey[notifications[i].DeliveryKey] = i
		}
	}
	stored := make([]models.Notification, 0, len(notifications))
	directIndex := 0
	for rows.Next() {
		var id int
		var deliveryKey sql.NullString
		if err := rows.Scan(&id, &deliveryKey); err != nil {
			return nil, fmt.Errorf("scan notification id: %w", err)
		}
		var index int
		if deliveryKey.Valid {
			var ok bool
			index, ok = byDeliveryKey[deliveryKey.String]
			if !ok {
				return nil, fmt.Errorf("notification insert returned an unknown delivery key")
			}
		} else {
			for directIndex < len(notifications) && notifications[directIndex].DeliveryKey != "" {
				directIndex++
			}
			index = directIndex
			directIndex++
		}
		if index >= len(notifications) {
			return nil, fmt.Errorf("notification insert returned an unknown delivery key")
		}
		notification := notifications[index]
		notification.ID = id
		stored = append(stored, notification)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate notification ids: %w", err)
	}
	return stored, nil
}

// MarkAsRead marks a notification as read
func (nm *NotificationManager) MarkAsRead(userID, notificationID int) error {
	lock := nm.userLock(userID)
	lock.Lock()
	defer lock.Unlock()

	now := time.Now()
	if _, err := nm.db.ExecWrite(`
		UPDATE notifications SET read = true, updated_at = ?
		WHERE id = ? AND user_id = ?
	`, now, notificationID, userID); err != nil {
		return fmt.Errorf("mark notification read: %w", err)
	}
	cache, ok := nm.cacheSnapshot(userID)
	if !ok {
		return nil
	}
	for i := range cache.Notifications {
		if cache.Notifications[i].ID != notificationID {
			continue
		}
		cache.Notifications[i].Read = true
		cache.Notifications[i].UpdatedAt = now
		break
	}
	return nm.setCacheSnapshot(userID, cache)
}

// MarkAllAsRead marks every unread notification for a user in one durable
// update, then mirrors that state into the compact tray cache when it is warm.
func (nm *NotificationManager) MarkAllAsRead(userID int) error {
	lock := nm.userLock(userID)
	lock.Lock()
	defer lock.Unlock()

	now := time.Now()
	if _, err := nm.db.ExecWrite(`
		UPDATE notifications SET read = true, updated_at = ?
		WHERE user_id = ? AND read = false
	`, now, userID); err != nil {
		return fmt.Errorf("mark all notifications read: %w", err)
	}

	cache, ok := nm.cacheSnapshot(userID)
	if !ok {
		return nil
	}
	changed := false
	for i := range cache.Notifications {
		if cache.Notifications[i].Read {
			continue
		}
		cache.Notifications[i].Read = true
		cache.Notifications[i].UpdatedAt = now
		changed = true
	}
	if !changed {
		return nil
	}
	return nm.setCacheSnapshot(userID, cache)
}

// MarkItemNotificationsAsRead marks cached unread notifications pointing at the
// given item as read. Notifications carry their item deep link in action_url
// (e.g. "/workspaces/<ws>/items/<itemID>"). The database update is durable
// before the compact cached page is refreshed.
func (nm *NotificationManager) MarkItemNotificationsAsRead(userID, itemID int) error {
	if itemID <= 0 {
		return nil
	}

	lock := nm.userLock(userID)
	lock.Lock()
	defer lock.Unlock()

	now := time.Now()
	itemPath := fmt.Sprintf("%%/items/%d", itemID)
	if _, err := nm.db.ExecWrite(`
		UPDATE notifications SET read = true, updated_at = ?
		WHERE user_id = ? AND read = false AND (
			action_url LIKE ? OR action_url LIKE ? OR action_url LIKE ? OR action_url LIKE ?
		)
	`, now, userID, itemPath, itemPath+"/%", itemPath+"?%", itemPath+"#%"); err != nil {
		return fmt.Errorf("mark item notifications read: %w", err)
	}
	cache, ok := nm.cacheSnapshot(userID)
	if !ok {
		return nil
	}
	changed := false
	for i := range cache.Notifications {
		if cache.Notifications[i].Read || !actionURLPointsToItem(cache.Notifications[i].ActionURL, itemID) {
			continue
		}
		cache.Notifications[i].Read = true
		cache.Notifications[i].UpdatedAt = now
		changed = true
	}
	if !changed {
		return nil
	}

	return nm.setCacheSnapshot(userID, cache)
}

// actionURLPointsToItem reports whether an action_url references the given
// item id via the "/items/<id>" route segment. Kept in lockstep with the
// actionUrl.js client-side matcher and the SQL LIKE above.
func actionURLPointsToItem(actionURL string, itemID int) bool {
	if actionURL == "" {
		return false
	}
	// "/items/<id>" followed by end-of-string, a separator, or a fragment.
	m := strings.Index(actionURL, "/items/")
	if m < 0 {
		return false
	}
	rest := actionURL[m+len("/items/"):]
	// Read leading digits; the segment must be exactly the numeric item id.
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	if end == 0 {
		return false
	}
	if end < len(rest) {
		switch rest[end] {
		case '/', '?', '#':
			// valid route boundary
		default:
			return false
		}
	}
	id, err := strconv.Atoi(rest[:end])
	if err != nil {
		return false
	}
	return id == itemID
}

// MarkAllAsSeen stamps seen_at on every unseen notification for the user.
// Distinct from MarkAllAsRead: "seen" reflects passive tray viewing and is
// safe to fire on an auto-timer, because the email batch scheduler keys off
// `read = false` and not seen_at. The DB write is one synchronous UPDATE.
func (nm *NotificationManager) MarkAllAsSeen(userID int) error {
	lock := nm.userLock(userID)
	lock.Lock()
	defer lock.Unlock()

	now := time.Now()
	if _, err := nm.db.ExecWrite(`
		UPDATE notifications
		SET seen_at = ?, updated_at = ?
		WHERE user_id = ? AND seen_at IS NULL
	`, now, now, userID); err != nil {
		return fmt.Errorf("mark notifications seen: %w", err)
	}

	if cache, ok := nm.cacheSnapshot(userID); ok {
		seenAt := now
		for i := range cache.Notifications {
			if cache.Notifications[i].SeenAt == nil {
				cache.Notifications[i].SeenAt = &seenAt
				cache.Notifications[i].UpdatedAt = now
			}
		}
		return nm.setCacheSnapshot(userID, cache)
	}
	return nil
}

// DeleteUserNotifications removes all notification rows for a user and drops
// the user's tray cache so cached entries cannot survive the database delete.
func (nm *NotificationManager) DeleteUserNotifications(userID int) error {
	lock := nm.userLock(userID)
	lock.Lock()
	defer lock.Unlock()

	if _, err := nm.db.ExecWrite(`DELETE FROM notifications WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("delete user notifications: %w", err)
	}
	_ = nm.cache.Delete(nm.getCacheKey(userID))
	return nil
}

// MarkNotificationsSent stamps sent_at so the email scheduler will not re-batch
// these rows after a successful or in-flight SMTP send.
func (nm *NotificationManager) MarkNotificationsSent(notificationIDs []int) error {
	if len(notificationIDs) == 0 {
		return nil
	}
	now := time.Now()
	return nm.updateNotificationsByID(notificationIDs, `sent_at = ?, updated_at = ?`, now, now)
}

// MarkNotificationsSendFailed flags rows whose SMTP-send rollback failed so an
// operator can find and repair the wedged notifications.
func (nm *NotificationManager) MarkNotificationsSendFailed(notificationIDs []int) error {
	if len(notificationIDs) == 0 {
		return nil
	}
	return nm.updateNotificationsByID(notificationIDs, `last_send_failed = true, updated_at = ?`, time.Now())
}

// RollbackNotificationsSent clears sent_at after an SMTP send failure so the
// rows become eligible for retry on a future scheduler tick.
func (nm *NotificationManager) RollbackNotificationsSent(notificationIDs []int) error {
	if len(notificationIDs) == 0 {
		return nil
	}
	return nm.updateNotificationsByID(notificationIDs, `sent_at = NULL, updated_at = ?`, time.Now())
}

func (nm *NotificationManager) updateNotificationsByID(notificationIDs []int, setClause string, args ...any) error {
	placeholders := make([]string, len(notificationIDs))
	for i, id := range notificationIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	query := fmt.Sprintf(`UPDATE notifications SET %s WHERE id IN (%s)`, setClause, strings.Join(placeholders, ","))
	if _, err := nm.db.ExecWrite(query, args...); err != nil {
		return fmt.Errorf("update notifications: %w", err)
	}
	return nil
}

// notificationTrayRetention bounds how far back the notification tray scrolls.
// Older notifications stay in the DB (audit) but are hidden from the list view.
const notificationTrayRetention = 10 * 24 * time.Hour

// queryNotifications loads one retained notification page from the database.
func (nm *NotificationManager) queryNotifications(userID, limit, offset int) ([]models.Notification, error) {
	query := `
		SELECT id, user_id, title, message, type, timestamp, read, seen_at, avatar, action_url, metadata,
		       authorization_scope, workspace_id, item_id, source_type, source_id,
		       referenced_entity_type, referenced_entity_id, referenced_workspace_id, referenced_workspace_permission,
		       created_at, updated_at
		FROM notifications
		WHERE user_id = ? AND timestamp >= ?
		ORDER BY timestamp DESC, id DESC
		LIMIT ? OFFSET ?
	`

	cutoff := time.Now().Add(-notificationTrayRetention)
	rows, err := nm.db.Query(query, userID, cutoff, limit, offset)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var notifications []models.Notification
	for rows.Next() {
		var n models.Notification
		var avatar, actionURL, metadata, sourceType, referencedEntityType, referencedPermission *string
		var workspaceID, itemID, sourceID *int

		err := rows.Scan(
			&n.ID, &n.UserID, &n.Title, &n.Message, &n.Type,
			&n.Timestamp, &n.Read, &n.SeenAt, &avatar, &actionURL, &metadata,
			&n.AuthorizationScope, &workspaceID, &itemID, &sourceType, &sourceID,
			&referencedEntityType, &n.ReferencedEntityID, &n.ReferencedWorkspaceID, &referencedPermission,
			&n.CreatedAt, &n.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		if avatar != nil {
			n.Avatar = *avatar
		}
		if actionURL != nil {
			n.ActionURL = *actionURL
		}
		if metadata != nil {
			n.Metadata = *metadata
		}
		n.WorkspaceID = workspaceID
		n.ItemID = itemID
		n.SourceID = sourceID
		if sourceType != nil {
			n.SourceType = *sourceType
		}
		if referencedEntityType != nil {
			n.ReferencedEntityType = *referencedEntityType
		}
		if referencedPermission != nil {
			n.ReferencedWorkspacePermission = *referencedPermission
		}

		notifications = append(notifications, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return notifications, nil
}

// GetVisibleUserNotifications applies current authorization before paginating
// tray results. Legacy, unknown, and incomplete scopes fail closed.
func (nm *NotificationManager) GetVisibleUserNotifications(userID int, snapshot *services.NotificationAuthorizationSnapshot, limit, offset int) ([]models.Notification, error) {
	query := `
		SELECT id, user_id, title, message, type, timestamp, read, seen_at, avatar, action_url, metadata,
		       authorization_scope, workspace_id, item_id, source_type, source_id,
		       referenced_entity_type, referenced_entity_id, referenced_workspace_id, referenced_workspace_permission,
		       created_at, updated_at
		FROM notifications
		WHERE user_id = ? AND timestamp >= ?
		ORDER BY timestamp DESC, id DESC
	`
	rows, err := nm.db.Query(query, userID, time.Now().Add(-notificationTrayRetention))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	notifications := make([]models.Notification, 0)
	for rows.Next() {
		var notification models.Notification
		var avatar, actionURL, metadata, sourceType, referencedEntityType, referencedPermission *string
		if err := rows.Scan(
			&notification.ID, &notification.UserID, &notification.Title, &notification.Message, &notification.Type,
			&notification.Timestamp, &notification.Read, &notification.SeenAt, &avatar, &actionURL, &metadata,
			&notification.AuthorizationScope, &notification.WorkspaceID, &notification.ItemID, &sourceType, &notification.SourceID,
			&referencedEntityType, &notification.ReferencedEntityID, &notification.ReferencedWorkspaceID, &referencedPermission,
			&notification.CreatedAt, &notification.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if avatar != nil {
			notification.Avatar = *avatar
		}
		if actionURL != nil {
			notification.ActionURL = *actionURL
		}
		if metadata != nil {
			notification.Metadata = *metadata
		}
		if sourceType != nil {
			notification.SourceType = *sourceType
		}
		if referencedEntityType != nil {
			notification.ReferencedEntityType = *referencedEntityType
		}
		if referencedPermission != nil {
			notification.ReferencedWorkspacePermission = *referencedPermission
		}
		visible, err := snapshot.Visible(notification)
		if err != nil {
			return nil, err
		}
		if !visible {
			continue
		}
		if offset > 0 {
			offset--
			continue
		}
		notifications = append(notifications, notification)
		if len(notifications) >= limit {
			break
		}
	}
	return notifications, rows.Err()
}

// Stop stops the notification manager
func (nm *NotificationManager) Stop() {
	nm.stopOnce.Do(func() {
		nm.dispatcherMu.RLock()
		dispatcher := nm.pushDispatcher
		nm.dispatcherMu.RUnlock()
		if dispatcher != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := dispatcher.Close(ctx); err != nil {
				slog.Warn("push dispatcher shutdown incomplete", "component", "notifications", "error", err)
			}
			cancel()
		}
		_ = nm.cache.Close()
	})
}

// GetStats returns persistence, compact-cache, and push-enqueue statistics.
func (nm *NotificationManager) GetStats() map[string]int64 {
	return map[string]int64{
		"insert_batches":           atomic.LoadInt64(&nm.insertBatches),
		"inserted":                 atomic.LoadInt64(&nm.inserted),
		"insert_errors":            atomic.LoadInt64(&nm.insertErrors),
		"insert_duration_ms":       time.Duration(atomic.LoadInt64(&nm.insertDurationNS)).Milliseconds(),
		"last_insert_duration_ms":  time.Duration(atomic.LoadInt64(&nm.lastInsertDuration)).Milliseconds(),
		"max_insert_duration_ms":   time.Duration(atomic.LoadInt64(&nm.maxInsertDuration)).Milliseconds(),
		"cache_updates":            atomic.LoadInt64(&nm.cacheUpdates),
		"cache_errors":             atomic.LoadInt64(&nm.cacheErrors),
		"cache_bytes_written":      atomic.LoadInt64(&nm.cacheBytesWritten),
		"max_cache_entry_bytes":    atomic.LoadInt64(&nm.maxCacheEntryBytes),
		"cache_update_duration_ms": time.Duration(atomic.LoadInt64(&nm.cacheUpdateDuration)).Milliseconds(),
		"last_cache_duration_ms":   time.Duration(atomic.LoadInt64(&nm.lastCacheDuration)).Milliseconds(),
		"max_cache_duration_ms":    time.Duration(atomic.LoadInt64(&nm.maxCacheDuration)).Milliseconds(),
		"push_rejected":            atomic.LoadInt64(&nm.pushRejected),
	}
}

func updateNotificationMaximum(target *int64, value int64) {
	for {
		previous := atomic.LoadInt64(target)
		if value <= previous || atomic.CompareAndSwapInt64(target, previous, value) {
			return
		}
	}
}

// HTTP Handlers

// GetNotifications handles GET /api/notifications
func (nh *NotificationHandler) GetNotifications(w http.ResponseWriter, r *http.Request) {
	// Get authenticated user from context
	user := utils.GetCurrentUser(r)
	if user == nil {
		slog.Debug("no authenticated user in context", slog.String("component", "notifications"))
		respondUnauthorized(w, r)
		return
	}
	userID := user.ID

	// Parse pagination parameters
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit := 50 // Default limit
	offset := 0 // Default offset

	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	if offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	snapshot, err := nh.notificationAuth.Snapshot(userID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	notifications, err := nh.manager.GetVisibleUserNotifications(userID, snapshot, limit, offset)
	if err != nil {
		slog.Error("failed to get notifications", slog.String("component", "notifications"), slog.Int("user_id", userID), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, notifications)
}

// ClearNotifications handles DELETE /api/notifications.
func (nh *NotificationHandler) ClearNotifications(w http.ResponseWriter, r *http.Request) {
	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	if err := nh.manager.DeleteUserNotifications(user.ID); err != nil {
		slog.Error("failed to clear notifications", slog.String("component", "notifications"), slog.Int("user_id", user.ID), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// CreateNotification handles POST /api/notifications.
// The endpoint can only mint a notification for the authenticated user — the
// request's user_id is ignored. Server-side notifications go through
// NotificationService.EmitEvent; this handler exists only so a user can push
// their own ad-hoc reminders into the tray.
func (nh *NotificationHandler) CreateNotification(w http.ResponseWriter, r *http.Request) {
	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	notification, ok := decodeJSON[models.Notification](w, r)
	if !ok {
		return
	}
	sanitize.ApplyAll(
		sanitize.Pair{Target: &notification.Title, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &notification.Message, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &notification.Type, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &notification.Avatar, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &notification.ActionURL, Policy: sanitize.PlainTextField},
	)
	// Metadata is a JSON blob that is never decoded server-side, so this
	// handler is the only bounding point — HTML stripping would corrupt
	// valid payloads, so it is size-capped and required to be well-formed
	// JSON instead, with invalid payloads rejected.
	if err := sanitize.ValidateJSONPayload("metadata", notification.Metadata); err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	// Force caller identity — never trust the request body's user_id.
	notification.UserID = user.ID
	notification.AuthorizationScope = models.NotificationScopeSystem
	notification.WorkspaceID = nil
	notification.ItemID = nil
	notification.SourceType = "user.reminder"
	notification.SourceID = nil

	if notification.Timestamp.IsZero() {
		notification.Timestamp = time.Now()
	}

	stored, err := nh.manager.AddNotification(notification)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONCreated(w, stored)
}

// MarkNotificationAsRead handles PATCH /api/notifications/{id}/read
func (nh *NotificationHandler) MarkNotificationAsRead(w http.ResponseWriter, r *http.Request) {
	// Get authenticated user from context
	user := utils.GetCurrentUser(r)
	if user == nil {
		slog.Debug("no authenticated user in context", slog.String("component", "notifications"))
		respondUnauthorized(w, r)
		return
	}
	userID := user.ID

	idStr := r.PathValue("id")

	id, err := strconv.Atoi(idStr)
	if err != nil {
		respondInvalidID(w, r, "notification ID")
		return
	}

	slog.Debug("marking notification as read", slog.String("component", "notifications"), slog.Int("user_id", userID), slog.String("username", user.Username), slog.Int("notification_id", id))

	if err := nh.manager.MarkAsRead(userID, id); err != nil {
		slog.Error("failed to mark notification as read", slog.String("component", "notifications"), slog.Int("notification_id", id), slog.Int("user_id", userID), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	slog.Debug("successfully marked notification as read", slog.String("component", "notifications"), slog.Int("notification_id", id), slog.Int("user_id", userID))
	w.WriteHeader(http.StatusOK)
}

// MarkAllNotificationsAsRead handles PATCH /api/notifications/read-all.
func (nh *NotificationHandler) MarkAllNotificationsAsRead(w http.ResponseWriter, r *http.Request) {
	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	if err := nh.manager.MarkAllAsRead(user.ID); err != nil {
		slog.Error("failed to mark all notifications as read", slog.String("component", "notifications"), slog.Int("user_id", user.ID), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// MarkAllNotificationsAsSeen handles PATCH /api/notifications/seen-all.
// Bughunt #11: separate the tray's "I looked at it" signal from "I
// acknowledge this", so an auto-timer firing 5 s after the tray opens no
// longer suppresses email batches (the scheduler keys off read = false).
func (nh *NotificationHandler) MarkAllNotificationsAsSeen(w http.ResponseWriter, r *http.Request) {
	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	if err := nh.manager.MarkAllAsSeen(user.ID); err != nil {
		slog.Error("failed to mark all notifications as seen", slog.String("component", "notifications"), slog.Int("user_id", user.ID), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// MarkItemNotificationsAsRead handles POST /api/notifications/mark-item-read.
// Marks every unread notification pointing at the given item as read. Viewing
// an item should clear its notifications regardless of entry point (deep link,
// PWA, navigation) — not only when opened from the notification tray.
func (nh *NotificationHandler) MarkItemNotificationsAsRead(w http.ResponseWriter, r *http.Request) {
	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	payload, ok := decodeJSON[struct {
		ItemID int `json:"item_id"`
	}](w, r)
	if !ok {
		return
	}
	if payload.ItemID <= 0 {
		respondValidationError(w, r, "item_id must be a positive integer")
		return
	}

	if err := nh.manager.MarkItemNotificationsAsRead(user.ID, payload.ItemID); err != nil {
		slog.Error("failed to mark item notifications as read", slog.String("component", "notifications"), slog.Int("user_id", user.ID), slog.Int("item_id", payload.ItemID), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// RefreshCache handles POST /api/notifications/refresh-cache (admin only)
func (nh *NotificationHandler) RefreshCache(w http.ResponseWriter, r *http.Request) {
	slog.Debug("admin requested manual cache refresh", slog.String("component", "notifications"))

	if nh.service == nil {
		slog.Warn("notification service not available", slog.String("component", "notifications"))
		respondInternalError(w, r, fmt.Errorf("notification service not available"))
		return
	}

	if err := nh.service.ForceRefreshCache(); err != nil {
		slog.Error("failed to refresh cache", slog.String("component", "notifications"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	slog.Debug("cache refreshed successfully", slog.String("component", "notifications"))
	respondJSONOK(w, map[string]string{
		"message": "Notification cache refreshed successfully",
	})
}
