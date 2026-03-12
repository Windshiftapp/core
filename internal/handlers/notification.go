package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"

	"github.com/allegro/bigcache/v3"
)

// notificationWrite represents a notification queued for database persistence
type notificationWrite struct {
	Notification models.Notification
	IsNew        bool // true = INSERT, false = UPDATE (e.g. read status change)
}

// NotificationManagerConfig holds tuning parameters for the notification manager.
type NotificationManagerConfig struct {
	FlushInterval time.Duration // WriteBatcher flush interval (default: 30s)
	MaxBatchSize  int           // WriteBatcher max batch size (default: 50)
	SyncInterval  time.Duration // Periodic consistency check interval (default: 2min)
}

// DefaultNotificationManagerConfig returns a config with sensible defaults.
func DefaultNotificationManagerConfig() NotificationManagerConfig {
	return NotificationManagerConfig{
		FlushInterval: 30 * time.Second,
		MaxBatchSize:  50,
		SyncInterval:  2 * time.Minute,
	}
}

// NotificationManager handles notification caching and persistence
type NotificationManager struct {
	cache      *bigcache.BigCache
	db         database.Database
	batcher    *services.WriteBatcher[notificationWrite]
	syncTicker *time.Ticker
	stopChan   chan struct{}
	mu         sync.RWMutex
}

// NotificationService interface for cache management
type NotificationService interface {
	ForceRefreshCache() error
}

// NotificationHandler handles HTTP requests for notifications
type NotificationHandler struct {
	manager *NotificationManager
	service NotificationService
}

// NewNotificationManager creates a new notification manager with BigCache
func NewNotificationManager(db database.Database, nmCfg NotificationManagerConfig) (*NotificationManager, error) {
	cacheConfig := services.NewBigCacheConfig(services.BigCacheOptions{
		TTL:          24 * time.Hour,
		MaxCacheMB:   512,
		MaxEntrySize: 1024, // 1KB per entry
	})

	cache, err := bigcache.New(context.Background(), cacheConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create BigCache: %w", err)
	}

	manager := &NotificationManager{
		cache:      cache,
		db:         db,
		syncTicker: time.NewTicker(nmCfg.SyncInterval),
		stopChan:   make(chan struct{}),
	}

	batcherConfig := services.WriteBatcherConfig{
		FlushInterval: nmCfg.FlushInterval,
		MaxBatchSize:  nmCfg.MaxBatchSize,
		Name:          "notifications",
	}
	manager.batcher = services.NewWriteBatcher(batcherConfig, manager.flushNotificationBatch)
	manager.batcher.Start()

	// Start periodic sync goroutine (consistency check, reconciles temp IDs)
	go manager.periodicSync()

	return manager, nil
}

// NewNotificationHandler creates a new notification handler
func NewNotificationHandler(manager *NotificationManager, service NotificationService) *NotificationHandler {
	return &NotificationHandler{
		manager: manager,
		service: service,
	}
}

// getCacheKey generates a cache key for a user's notifications
func (nm *NotificationManager) getCacheKey(userID int) string {
	return fmt.Sprintf("user:%d:notifications", userID)
}

// GetUserNotifications retrieves notifications for a user (cache-first)
func (nm *NotificationManager) GetUserNotifications(userID, limit, offset int) ([]models.Notification, error) {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	cacheKey := nm.getCacheKey(userID)

	// Try cache first
	if entry, err := nm.cache.Get(cacheKey); err == nil {
		var cache models.NotificationCache
		if err := json.Unmarshal(entry, &cache); err == nil {
			// Apply pagination
			start := offset
			end := offset + limit
			if start > len(cache.Notifications) {
				return []models.Notification{}, nil
			}
			if end > len(cache.Notifications) {
				end = len(cache.Notifications)
			}
			return cache.Notifications[start:end], nil
		}
	}

	// Cache miss, load from database
	return nm.loadNotificationsFromDB(userID, limit, offset)
}

// AddNotification adds a new notification (writes to cache immediately)
func (nm *NotificationManager) AddNotification(notification models.Notification) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	slog.Debug("adding notification", slog.String("component", "notifications"), slog.Int("user_id", notification.UserID), slog.String("type", notification.Type), slog.String("title", notification.Title))

	cacheKey := nm.getCacheKey(notification.UserID)

	// Get existing cache or create new
	var cache models.NotificationCache
	if entry, err := nm.cache.Get(cacheKey); err == nil {
		_ = json.Unmarshal(entry, &cache)
	} else {
		cache = models.NotificationCache{
			Notifications: []models.Notification{},
			LastSynced:    time.Now().Add(-time.Hour), // Force sync on first write
			IsDirty:       true,
		}
	}

	// Add notification to beginning of slice (newest first)
	notification.ID = int(time.Now().UnixNano()) // Temporary ID for cache
	notification.CreatedAt = time.Now()
	notification.UpdatedAt = time.Now()

	cache.Notifications = append([]models.Notification{notification}, cache.Notifications...)
	cache.IsDirty = true

	// Keep only last 1000 notifications in cache
	if len(cache.Notifications) > 1000 {
		cache.Notifications = cache.Notifications[:1000]
	}

	// Update cache
	cacheData, _ := json.Marshal(cache)
	err := nm.cache.Set(cacheKey, cacheData)
	if err != nil {
		slog.Error("failed to set cache", slog.String("component", "notifications"), slog.Int("user_id", notification.UserID), slog.Any("error", err))
		return err
	}

	// Queue for durable DB persistence via WriteBatcher
	nm.batcher.Add(notificationWrite{
		Notification: notification,
		IsNew:        true,
	})

	slog.Debug("successfully added notification", slog.String("component", "notifications"), slog.Int("user_id", notification.UserID), slog.Int("cache_size", len(cache.Notifications)))
	return nil
}

// MarkAsRead marks a notification as read
func (nm *NotificationManager) MarkAsRead(userID, notificationID int) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	cacheKey := nm.getCacheKey(userID)

	// Get existing cache
	var cache models.NotificationCache
	if entry, err := nm.cache.Get(cacheKey); err == nil {
		if err := json.Unmarshal(entry, &cache); err != nil {
			return err
		}
	} else {
		// Load from database if not in cache
		notifications, err := nm.loadNotificationsFromDB(userID, 1000, 0)
		if err != nil {
			return err
		}
		cache = models.NotificationCache{
			Notifications: notifications,
			LastSynced:    time.Now(),
			IsDirty:       false,
		}
	}

	// Find and update notification
	for i := range cache.Notifications {
		if cache.Notifications[i].ID != notificationID {
			continue
		}

		cache.Notifications[i].Read = true
		cache.Notifications[i].UpdatedAt = time.Now()
		cache.IsDirty = true

		// Queue read-status update for DB persistence (only for real DB IDs)
		if notificationID <= int(time.Now().Unix()) {
			nm.batcher.Add(notificationWrite{
				Notification: cache.Notifications[i],
				IsNew:        false,
			})
		}
		break
	}

	// Update cache
	cacheData, _ := json.Marshal(cache)
	return nm.cache.Set(cacheKey, cacheData)
}

// loadNotificationsFromDB loads notifications from database and updates cache
func (nm *NotificationManager) loadNotificationsFromDB(userID, limit, offset int) ([]models.Notification, error) {
	query := `
		SELECT id, user_id, title, message, type, timestamp, read, avatar, action_url, metadata, created_at, updated_at
		FROM notifications 
		WHERE user_id = ? 
		ORDER BY timestamp DESC 
		LIMIT ? OFFSET ?
	`

	rows, err := nm.db.Query(query, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var notifications []models.Notification
	for rows.Next() {
		var n models.Notification
		var avatar, actionURL, metadata *string

		err := rows.Scan(
			&n.ID, &n.UserID, &n.Title, &n.Message, &n.Type,
			&n.Timestamp, &n.Read, &avatar, &actionURL, &metadata,
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

		notifications = append(notifications, n)
	}

	// Update cache with loaded data (only if we got the full first page)
	if offset == 0 && len(notifications) > 0 {
		cache := models.NotificationCache{
			Notifications: notifications,
			LastSynced:    time.Now(),
			IsDirty:       false,
		}
		cacheData, _ := json.Marshal(cache)
		cacheKey := nm.getCacheKey(userID)
		_ = nm.cache.Set(cacheKey, cacheData)
	}

	return notifications, nil
}

// periodicSync runs periodically to sync any remaining dirty cache entries to database (consistency check)
func (nm *NotificationManager) periodicSync() {
	for {
		select {
		case <-nm.syncTicker.C:
			nm.syncCacheToDatabase()
		case <-nm.stopChan:
			return
		}
	}
}

// syncCacheToDatabase syncs all dirty cache entries to the database
func (nm *NotificationManager) syncCacheToDatabase() {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	slog.Debug("starting periodic notification sync to database", slog.String("component", "notifications"))

	// Get all cache keys (this is a simplified approach - in production you'd want to track dirty keys)
	iterator := nm.cache.Iterator()
	for iterator.SetNext() {
		info, err := iterator.Value()
		if err != nil {
			continue
		}

		var cache models.NotificationCache
		if err = json.Unmarshal(info.Value(), &cache); err != nil {
			continue
		}

		if !cache.IsDirty {
			continue
		}

		// Extract user ID from key
		key := info.Key()
		parts := strings.Split(key, ":")
		if len(parts) < 2 {
			continue
		}
		userID, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}

		// Sync notifications to database
		if err := nm.syncUserNotifications(userID, cache.Notifications); err != nil {
			slog.Error("failed to sync notifications", slog.String("component", "notifications"), slog.Int("user_id", userID), slog.Any("error", err))
			continue
		}

		// Mark cache as clean
		cache.IsDirty = false
		cache.LastSynced = time.Now()
		cacheData, _ := json.Marshal(cache)
		_ = nm.cache.Set(key, cacheData)
	}

	slog.Debug("completed periodic notification sync to database", slog.String("component", "notifications"))
}

// syncUserNotifications syncs a user's notifications to the database
func (nm *NotificationManager) syncUserNotifications(_ int, notifications []models.Notification) error {
	tx, err := nm.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Prepare statements
	insertStmt, err := tx.Prepare(`
		INSERT INTO notifications (user_id, title, message, type, timestamp, read, avatar, action_url, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		read = excluded.read,
		updated_at = excluded.updated_at
	`)
	if err != nil {
		return err
	}
	defer func() { _ = insertStmt.Close() }()

	for _, notification := range notifications {
		// Skip temporary IDs (created in cache)
		if notification.ID > int(time.Now().Unix()) {
			// This is a temporary ID, do an insert
			_, err := insertStmt.Exec(
				notification.UserID, notification.Title, notification.Message,
				notification.Type, notification.Timestamp, notification.Read,
				nullableString(notification.Avatar), nullableString(notification.ActionURL),
				nullableString(notification.Metadata), notification.CreatedAt, notification.UpdatedAt,
			)
			if err != nil {
				return err
			}
		} else {
			// This is a real ID, do an update
			_, err := tx.Exec(`
				UPDATE notifications 
				SET read = ?, updated_at = ?
				WHERE id = ? AND user_id = ?
			`, notification.Read, notification.UpdatedAt, notification.ID, notification.UserID)
			if err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

// flushNotificationBatch persists a batch of notifications to the database.
// Called by WriteBatcher every 30s or when 50 items are queued.
func (nm *NotificationManager) flushNotificationBatch(items []notificationWrite) error {
	tx, err := nm.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, item := range items {
		n := item.Notification
		if item.IsNew {
			// INSERT new notification (omit ID to let DB auto-assign)
			_, err := tx.Exec(`
				INSERT INTO notifications (user_id, title, message, type, timestamp, read, avatar, action_url, metadata, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, n.UserID, n.Title, n.Message, n.Type, n.Timestamp, n.Read,
				nullableString(n.Avatar), nullableString(n.ActionURL),
				nullableString(n.Metadata), n.CreatedAt, n.UpdatedAt)
			if err != nil {
				return fmt.Errorf("insert notification: %w", err)
			}
		} else {
			// UPDATE existing notification (read status change)
			_, err := tx.Exec(`
				UPDATE notifications SET read = ?, updated_at = ? WHERE id = ? AND user_id = ?
			`, n.Read, n.UpdatedAt, n.ID, n.UserID)
			if err != nil {
				return fmt.Errorf("update notification: %w", err)
			}
		}
	}

	return tx.Commit()
}

// Stop stops the notification manager
func (nm *NotificationManager) Stop() {
	nm.syncTicker.Stop()
	close(nm.stopChan)
	nm.batcher.Stop()        // Flush remaining batched writes
	nm.syncCacheToDatabase() // Final consistency sync
	_ = nm.cache.Close()
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
	slog.Debug("loading notifications", slog.String("component", "notifications"), slog.Int("user_id", userID), slog.String("username", user.Username))

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

	notifications, err := nh.manager.GetUserNotifications(userID, limit, offset)
	if err != nil {
		slog.Error("failed to get notifications", slog.String("component", "notifications"), slog.Int("user_id", userID), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	slog.Debug("successfully retrieved notifications", slog.String("component", "notifications"), slog.Int("user_id", userID), slog.Int("count", len(notifications)), slog.Int("limit", limit), slog.Int("offset", offset))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(notifications)
}

// CreateNotification handles POST /api/notifications
func (nh *NotificationHandler) CreateNotification(w http.ResponseWriter, r *http.Request) {
	var notification models.Notification
	if err := json.NewDecoder(r.Body).Decode(&notification); err != nil {
		respondBadRequest(w, r, "Invalid JSON")
		return
	}

	// Set timestamp if not provided
	if notification.Timestamp.IsZero() {
		notification.Timestamp = time.Now()
	}

	if err := nh.manager.AddNotification(notification); err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(notification)
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
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"message": "Notification cache refreshed successfully",
	})
}
