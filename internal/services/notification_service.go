package services

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// NotificationManager interface for adding notifications. AddNotification
// returns the stored notification with its DB-assigned id populated; service
// callers that only need delivery can discard the returned value.
type NotificationManager interface {
	AddNotification(notification models.Notification) (models.Notification, error)
	AddNotifications(notifications []models.Notification) ([]models.Notification, error)
	AddNotificationsContext(ctx context.Context, notifications []models.Notification) ([]models.Notification, error)
	DeleteUserNotifications(userID int) error
	MarkNotificationsSent(notificationIDs []int) error
	MarkNotificationsSendFailed(notificationIDs []int) error
	RollbackNotificationsSent(notificationIDs []int) error
}

// NotificationEvent represents an event that should trigger notifications
type NotificationEvent struct {
	EventType                     string         // e.g., "item.created", "comment.added"
	WorkspaceID                   int            // Workspace where event occurred
	ActorUserID                   int            // User who triggered the event
	ItemID                        int            // Work item ID (for action URL)
	AssigneeID                    *int           // Current assignee (if applicable)
	CreatorID                     *int           // Item creator (if applicable)
	Title                         string         // Event title
	TemplateData                  map[string]any // Data for template substitution
	ReferencedWorkspaceID         int            // Optional secondary workspace whose content appears in TemplateData
	ReferencedWorkspacePermission string         // Permission required to receive secondary-entity data
	ReferencedEntityType          string         // Entity whose rendered data requires independent authorization
	ReferencedEntityID            int            // Referenced entity ID
}

// RuleCache stores cached notification rules for fast lookup
type RuleCache struct {
	WorkspaceConfigSets map[int]int                            // workspace_id -> config_set_id
	EventRules          map[int][]models.NotificationEventRule // config_set_id -> rules
	Templates           map[string]string                      // template_name -> content
	DefaultConfigSetID  int                                    // configuration_sets.is_default — fallback scheme when a workspace has none
	PersonalWorkspaces  map[int]bool                           // workspace_id -> true for is_personal workspaces (never notified)
	LastRefreshed       time.Time
}

// NotificationServiceConfig represents configuration for the notification service
type NotificationServiceConfig struct {
	RefreshInterval     time.Duration // How often to refresh rules cache
	EventBufferSize     int           // Hard bound for queued events
	EventWorkers        int           // Bounded concurrent event processors
	EventProcessTimeout time.Duration // Deadline for one event
	ShutdownTimeout     time.Duration // Deadline used by Close
}

// DefaultNotificationServiceConfig returns default configuration
func DefaultNotificationServiceConfig() NotificationServiceConfig {
	return NotificationServiceConfig{
		RefreshInterval:     5 * time.Minute,
		EventBufferSize:     1000,
		EventWorkers:        4,
		EventProcessTimeout: 15 * time.Second,
		ShutdownTimeout:     20 * time.Second,
	}
}

type queuedNotificationEvent struct {
	event      *NotificationEvent
	enqueuedAt time.Time
}

// UserNotificationBatch is a set of unread notifications destined for one
// user's batched email, along with the contact fields needed to address it.
type UserNotificationBatch struct {
	UserID          int
	UserEmail       string
	UserName        string
	Notifications   []models.Notification
	NotificationIDs []int
}

const (
	notificationEmailPending = "pending"
	notificationEmailClaimed = "claimed"
	notificationEmailSent    = "sent"
	notificationEmailFailed  = "failed"
)

var ErrNotificationEmailClaimLost = errors.New("notification email claim lost")

// NotificationEmailClaim fences one recipient batch to a scheduler instance.
type NotificationEmailClaim struct {
	*UserNotificationBatch
	Owner     string
	Token     string
	ExpiresAt time.Time
}

// NotificationService handles asynchronous notification creation
type NotificationService struct {
	db                  database.Database
	notificationManager NotificationManager
	permService         *PermissionService
	notificationAuth    *NotificationAuthorizer
	config              NotificationServiceConfig

	// Rule cache
	ruleCache *RuleCache
	cacheMu   sync.RWMutex

	// Event processing
	eventChan      chan queuedNotificationEvent
	cacheStop      chan struct{}
	emitMu         sync.RWMutex
	closeOnce      sync.Once
	closed         bool
	workerCtx      context.Context
	workerCancel   context.CancelFunc
	processEventFn func(context.Context, *NotificationEvent) error
	durableIngress *DurableNotificationIngress
	wg             sync.WaitGroup

	// Statistics
	eventsProcessed  int64
	eventsDropped    int64
	eventsQueued     int64
	activeWorkers    int64
	maxActiveWorkers int64
	lastQueueWaitNS  int64
	maxQueueWaitNS   int64
	cacheHits        int64
	cacheMisses      int64
	errors           int64
}

// UnreadEmailBatches resolves one current workspace scope per recipient, then
// bulk-filters and caps that recipient's pending rows. Legacy scopes fail closed.
func (ns *NotificationService) UnreadEmailBatches(maxBatchSize int) (map[string]*UserNotificationBatch, error) {
	return ns.unreadEmailBatches(maxBatchSize, time.Now())
}

func (ns *NotificationService) unreadEmailBatches(maxBatchSize int, now time.Time) (map[string]*UserNotificationBatch, error) {
	rows, err := ns.db.Query(`
		SELECT DISTINCT u.id, u.email, u.first_name, u.last_name
		FROM notifications n
		JOIN users u ON n.user_id = u.id
		WHERE n.sent_at IS NULL AND n.read = false AND u.email != '' AND u.is_active = true
		  AND (n.email_delivery_state IN (?, ?) OR
		       (n.email_delivery_state = ? AND (n.email_claim_expires_at IS NULL OR n.email_claim_expires_at <= ?)))
		  AND n.authorization_scope IN (?, ?, ?)
	`, notificationEmailPending, notificationEmailFailed, notificationEmailClaimed, now,
		models.NotificationScopeSystem, models.NotificationScopeWorkspace, models.NotificationScopeAsset)
	if err != nil {
		return nil, fmt.Errorf("query notification email recipients: %w", err)
	}
	type recipient struct {
		id                 int
		email, first, last string
	}
	recipients := make([]recipient, 0)
	for rows.Next() {
		var candidate recipient
		if err := rows.Scan(&candidate.id, &candidate.email, &candidate.first, &candidate.last); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan notification email recipient: %w", err)
		}
		recipients = append(recipients, candidate)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate notification email recipients: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close notification email recipients: %w", err)
	}

	batches := make(map[string]*UserNotificationBatch, len(recipients))
	for _, candidate := range recipients {
		snapshot, err := ns.notificationAuth.Snapshot(candidate.id)
		if err != nil {
			return nil, fmt.Errorf("resolve notification scope for user %d: %w", candidate.id, err)
		}
		notifications, err := ns.unreadEmailNotificationsForUser(candidate.id, snapshot, maxBatchSize, now)
		if err != nil {
			return nil, err
		}
		if len(notifications) == 0 {
			continue
		}
		batch := &UserNotificationBatch{
			UserID:          candidate.id,
			UserEmail:       candidate.email,
			UserName:        strings.TrimSpace(candidate.first + " " + candidate.last),
			Notifications:   notifications,
			NotificationIDs: make([]int, len(notifications)),
		}
		for i, notification := range notifications {
			batch.NotificationIDs[i] = notification.ID
		}
		batches[candidate.email] = batch
	}
	return batches, nil
}

func (ns *NotificationService) unreadEmailNotificationsForUser(userID int, snapshot *NotificationAuthorizationSnapshot, limit int, now time.Time) ([]models.Notification, error) {
	rows, err := ns.db.Query(`
		SELECT n.id, n.user_id, n.title, n.message, n.type, n.timestamp, n.read,
		       n.sent_at, n.avatar, n.action_url, n.metadata, n.authorization_scope,
		       n.workspace_id, n.item_id, n.source_type, n.source_id,
		       n.referenced_entity_type, n.referenced_entity_id, n.referenced_workspace_id, n.referenced_workspace_permission,
		       n.created_at, n.updated_at
		FROM notifications n
		WHERE n.user_id = ? AND n.sent_at IS NULL AND n.read = false
		  AND (n.email_delivery_state IN (?, ?) OR
		       (n.email_delivery_state = ? AND (n.email_claim_expires_at IS NULL OR n.email_claim_expires_at <= ?)))
		  AND n.authorization_scope IN (?, ?, ?)
		ORDER BY n.timestamp ASC, n.id ASC
	`, userID, notificationEmailPending, notificationEmailFailed, notificationEmailClaimed, now,
		models.NotificationScopeSystem, models.NotificationScopeWorkspace, models.NotificationScopeAsset)
	if err != nil {
		return nil, fmt.Errorf("query unread notifications for user %d: %w", userID, err)
	}
	defer rows.Close()

	notifications := make([]models.Notification, 0, limit)
	for rows.Next() {
		var notification models.Notification
		var avatar, actionURL, metadata, sourceType, referencedEntityType, referencedPermission *string
		if err := rows.Scan(
			&notification.ID, &notification.UserID, &notification.Title, &notification.Message, &notification.Type,
			&notification.Timestamp, &notification.Read, &notification.SentAt, &avatar, &actionURL, &metadata,
			&notification.AuthorizationScope, &notification.WorkspaceID, &notification.ItemID, &sourceType, &notification.SourceID,
			&referencedEntityType, &notification.ReferencedEntityID, &notification.ReferencedWorkspaceID, &referencedPermission,
			&notification.CreatedAt, &notification.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan unread notification for user %d: %w", userID, err)
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
		notifications = append(notifications, notification)
		if len(notifications) >= limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate unread notifications for user %d: %w", userID, err)
	}
	return notifications, nil
}

// ClaimUnreadEmailBatches atomically fences each selected recipient batch.
// Live claims are excluded; expired claims and failed attempts are recoverable.
func (ns *NotificationService) ClaimUnreadEmailBatches(owner string, now time.Time, lease time.Duration, maxBatchSize int) (map[string]*NotificationEmailClaim, error) {
	if strings.TrimSpace(owner) == "" {
		return nil, fmt.Errorf("claim notification email batches: owner is required")
	}
	if lease <= 0 {
		return nil, fmt.Errorf("claim notification email batches: lease must be positive")
	}
	batches, err := ns.unreadEmailBatches(maxBatchSize, now)
	if err != nil {
		return nil, err
	}
	claims := make(map[string]*NotificationEmailClaim, len(batches))
	for email, batch := range batches {
		token, err := newNotificationClaimToken()
		if err != nil {
			return nil, err
		}
		expiresAt := now.Add(lease)
		claimed, err := ns.claimNotificationIDs(batch.NotificationIDs, owner, token, now, expiresAt)
		if err != nil {
			return nil, err
		}
		if !claimed {
			continue
		}
		claims[email] = &NotificationEmailClaim{
			UserNotificationBatch: batch,
			Owner:                 owner,
			Token:                 token,
			ExpiresAt:             expiresAt,
		}
	}
	return claims, nil
}

func newNotificationClaimToken() (string, error) {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", fmt.Errorf("generate notification email claim token: %w", err)
	}
	return hex.EncodeToString(token[:]), nil
}

func (ns *NotificationService) claimNotificationIDs(ids []int, owner, token string, now, expiresAt time.Time) (bool, error) {
	if len(ids) == 0 {
		return false, nil
	}
	placeholders := make([]string, len(ids))
	args := []any{notificationEmailClaimed, owner, token, expiresAt, now}
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	args = append(args, notificationEmailPending, notificationEmailFailed, notificationEmailClaimed, now)
	result, err := ns.db.ExecWrite(`
		UPDATE notifications
		SET email_delivery_state = ?, email_claim_owner = ?, email_claim_token = ?,
		    email_claim_expires_at = ?, email_delivery_attempts = email_delivery_attempts + 1,
		    email_last_error = NULL, updated_at = ?
		WHERE id IN (`+strings.Join(placeholders, ",")+`)
		  AND sent_at IS NULL AND read = false
		  AND (email_delivery_state IN (?, ?) OR
		       (email_delivery_state = ? AND (email_claim_expires_at IS NULL OR email_claim_expires_at <= ?)))
	`, args...)
	if err != nil {
		return false, fmt.Errorf("claim notification email batch: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("count claimed notification rows: %w", err)
	}
	if affected == int64(len(ids)) {
		return true, nil
	}
	if _, releaseErr := ns.db.ExecWrite(`
		UPDATE notifications
		SET email_delivery_state = ?, email_claim_owner = NULL, email_claim_token = NULL,
		    email_claim_expires_at = NULL, updated_at = ?
		WHERE email_delivery_state = ? AND email_claim_owner = ? AND email_claim_token = ?
	`, notificationEmailPending, now, notificationEmailClaimed, owner, token); releaseErr != nil {
		return false, fmt.Errorf("release partial notification email claim: %w", releaseErr)
	}
	return false, nil
}

// CompleteNotificationEmailClaim records a successful SMTP call. Only the
// exact live owner/token pair may complete the rows it claimed.
func (ns *NotificationService) CompleteNotificationEmailClaim(claim *NotificationEmailClaim, now time.Time) error {
	return ns.transitionNotificationEmailClaim(claim, `
		email_delivery_state = ?, sent_at = ?, email_claim_owner = NULL,
		email_claim_token = NULL, email_claim_expires_at = NULL,
		email_last_error = NULL, last_send_failed = false, updated_at = ?
	`, notificationEmailSent, now, now)
}

// FailNotificationEmailClaim records a definite sender error and makes the
// rows eligible for a later fenced retry.
func (ns *NotificationService) FailNotificationEmailClaim(claim *NotificationEmailClaim, sendErr error, now time.Time) error {
	message := "notification email send failed"
	if sendErr != nil {
		message = sendErr.Error()
	}
	if len(message) > 2000 {
		message = message[:2000]
	}
	return ns.transitionNotificationEmailClaim(claim, `
		email_delivery_state = ?, email_claim_owner = NULL, email_claim_token = NULL,
		email_claim_expires_at = NULL, email_last_error = ?, last_send_failed = true,
		updated_at = ?
	`, notificationEmailFailed, message, now)
}

// ReleaseNotificationEmailClaim returns an unsent claim to pending without
// letting a stale owner alter a newer claim.
func (ns *NotificationService) ReleaseNotificationEmailClaim(claim *NotificationEmailClaim, now time.Time) error {
	return ns.transitionNotificationEmailClaim(claim, `
		email_delivery_state = ?, email_claim_owner = NULL, email_claim_token = NULL,
		email_claim_expires_at = NULL, updated_at = ?
	`, notificationEmailPending, now)
}

// NotificationEmailClaimDeliverable rechecks recipient and content access
// immediately before SMTP starts. Once SMTP is in progress, deactivation
// cannot recall a message the remote server may already have accepted.
func (ns *NotificationService) NotificationEmailClaimDeliverable(claim *NotificationEmailClaim) (bool, error) {
	if claim == nil || claim.UserNotificationBatch == nil {
		return false, fmt.Errorf("%w: invalid claim", ErrNotificationEmailClaimLost)
	}
	snapshot, err := ns.notificationAuth.Snapshot(claim.UserID)
	if err != nil {
		return false, err
	}
	for _, notification := range claim.Notifications {
		visible, err := snapshot.Visible(notification)
		if err != nil {
			return false, err
		}
		if !visible {
			return false, nil
		}
	}
	return true, nil
}

func (ns *NotificationService) transitionNotificationEmailClaim(claim *NotificationEmailClaim, setClause string, setArgs ...any) error {
	if claim == nil || claim.UserNotificationBatch == nil || len(claim.NotificationIDs) == 0 || claim.Owner == "" || claim.Token == "" {
		return fmt.Errorf("%w: invalid claim", ErrNotificationEmailClaimLost)
	}
	placeholders := make([]string, len(claim.NotificationIDs))
	args := append([]any(nil), setArgs...)
	args = append(args, notificationEmailClaimed, claim.Owner, claim.Token)
	for i, id := range claim.NotificationIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	result, err := ns.db.ExecWrite(`UPDATE notifications SET `+setClause+`
		WHERE email_delivery_state = ? AND email_claim_owner = ? AND email_claim_token = ?
		  AND id IN (`+strings.Join(placeholders, ",")+`)`, args...)
	if err != nil {
		return fmt.Errorf("transition notification email claim: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count transitioned notification email rows: %w", err)
	}
	if affected != int64(len(claim.NotificationIDs)) {
		return fmt.Errorf("%w: transitioned %d of %d rows", ErrNotificationEmailClaimLost, affected, len(claim.NotificationIDs))
	}
	return nil
}

// NewNotificationService creates a new notification service. The
// permService argument is used to re-authorize recipients at delivery time
// so a user who has lost workspace access does not receive notifications
// for items in that workspace via a stale watch or admin assignment.
func NewNotificationService(db database.Database, notificationManager NotificationManager, permService *PermissionService, config NotificationServiceConfig, assetPermissions ...AssetSetPermissionChecker) *NotificationService {
	defaults := DefaultNotificationServiceConfig()
	if config.RefreshInterval <= 0 {
		config.RefreshInterval = defaults.RefreshInterval
	}
	if config.EventBufferSize <= 0 {
		config.EventBufferSize = defaults.EventBufferSize
	}
	if config.EventWorkers <= 0 {
		config.EventWorkers = defaults.EventWorkers
	}
	if config.EventProcessTimeout <= 0 {
		config.EventProcessTimeout = defaults.EventProcessTimeout
	}
	if config.ShutdownTimeout <= 0 {
		config.ShutdownTimeout = defaults.ShutdownTimeout
	}
	workerCtx, workerCancel := context.WithCancel(context.Background()) //nolint:gosec // retained and called by CloseContext
	var assetPermissionChecker AssetSetPermissionChecker
	if len(assetPermissions) > 0 {
		assetPermissionChecker = assetPermissions[0]
	}
	service := &NotificationService{
		db:                  db,
		notificationManager: notificationManager,
		permService:         permService,
		notificationAuth:    NewNotificationAuthorizer(db, permService, assetPermissionChecker),
		config:              config,
		ruleCache: &RuleCache{
			WorkspaceConfigSets: make(map[int]int),
			EventRules:          make(map[int][]models.NotificationEventRule),
			Templates:           make(map[string]string),
			PersonalWorkspaces:  make(map[int]bool),
			LastRefreshed:       time.Time{},
		},
		eventChan:    make(chan queuedNotificationEvent, config.EventBufferSize),
		cacheStop:    make(chan struct{}),
		workerCtx:    workerCtx,
		workerCancel: workerCancel,
	}

	// Load initial cache
	if err := service.refreshRuleCache(); err != nil {
		slog.Warn("failed to load initial notification rule cache", slog.String("component", "notifications"), slog.Any("error", err))
	}

	// Start a fixed event worker pool plus the independent cache refresher.
	service.wg.Add(config.EventWorkers + 1)
	for workerID := 0; workerID < config.EventWorkers; workerID++ {
		go service.eventProcessor(workerID)
	}
	go service.cacheRefresher()

	slog.Debug("notification service initialized", slog.String("component", "notifications"), slog.Duration("refresh_interval", config.RefreshInterval), slog.Int("event_workers", config.EventWorkers), slog.Int("event_buffer_size", config.EventBufferSize))

	return service
}

// itemActionURL is the in-app deep link for an item, shared by every
// notification this service stores.
func itemActionURL(workspaceID, itemID int) string {
	return fmt.Sprintf("/workspaces/%d/items/%d", workspaceID, itemID)
}

// NotifyUsers creates a notification directly for each user in userIDs,
// bypassing rule-based recipient determination. Callers (e.g., action
// notify_user nodes) that have already resolved recipients from their own
// config use this to avoid the event→rule→recipient pipeline which can't
// express "notify exactly these users".
func (ns *NotificationService) NotifyUsers(userIDs []int, workspaceID, itemID, actorUserID int, notifType, title, message string) error {
	var authorize func(int) (bool, error)
	if ns.permService != nil {
		authorize = func(userID int) (bool, error) {
			return ns.permService.HasWorkspacePermission(userID, workspaceID, models.PermissionItemView)
		}
	}
	_, err := ns.notifyUsersAtURL(userIDs, actorUserID, notifType, title, message, itemActionURL(workspaceID, itemID),
		models.NotificationScopeWorkspace, &workspaceID, &itemID, "item", &itemID, authorize)
	return err
}

// NotifyUsersForAsset creates direct notifications for users who currently
// retain view access to the asset's set. Asset actions cannot use the
// workspace notification-rule pipeline, and silently skipping this check
// would disclose asset-derived titles/messages to arbitrary configured IDs.
func (ns *NotificationService) NotifyUsersForAsset(userIDs []int, setID, assetID, actorUserID int, notifType, title, message string, includeLink bool, checker AssetSetPermissionChecker) ([]int, error) {
	if checker == nil {
		return nil, fmt.Errorf("asset notification blocked: asset permission checker not configured")
	}
	actionURL := ""
	if includeLink {
		actionURL = fmt.Sprintf("/assets/%d", assetID)
	}
	authorize := func(userID int) (bool, error) {
		return checker.HasAssetSetPermission(userID, setID, AssetPermissionKeyView)
	}
	return ns.notifyUsersAtURL(userIDs, actorUserID, notifType, title, message, actionURL,
		models.NotificationScopeAsset, nil, nil, "asset", &assetID, authorize)
}

func (ns *NotificationService) notifyUsersAtURL(
	userIDs []int,
	actorUserID int,
	notifType, title, message, actionURL, authorizationScope string,
	workspaceID, itemID *int,
	sourceType string,
	sourceID *int,
	authorize func(int) (bool, error),
) ([]int, error) {
	if ns.notificationManager == nil {
		return nil, fmt.Errorf("notification manager not configured")
	}
	skipAsAgent := map[int]bool{}
	if ns.db != nil {
		skipAsAgent = ns.agentOrUnknownUsers(userIDs)
	}
	seen := make(map[int]bool, len(userIDs))
	notifications := make([]models.Notification, 0, len(userIDs))
	deliveredUserIDs := make([]int, 0, len(userIDs))
	now := time.Now()
	for _, uid := range userIDs {
		if uid <= 0 || uid == actorUserID || seen[uid] || skipAsAgent[uid] {
			continue
		}
		seen[uid] = true
		if authorize != nil {
			allowed, err := authorize(uid)
			if err != nil {
				return nil, fmt.Errorf("authorize notification recipient %d: %w", uid, err)
			}
			if !allowed {
				continue
			}
		}
		notifications = append(notifications, models.Notification{
			UserID:             uid,
			Title:              title,
			Message:            message,
			Type:               notifType,
			Timestamp:          now,
			Read:               false,
			ActionURL:          actionURL,
			AuthorizationScope: authorizationScope,
			WorkspaceID:        workspaceID,
			ItemID:             itemID,
			SourceType:         sourceType,
			SourceID:           sourceID,
		})
		deliveredUserIDs = append(deliveredUserIDs, uid)
	}
	if len(notifications) == 0 {
		return []int{}, nil
	}
	if _, err := ns.notificationManager.AddNotifications(notifications); err != nil {
		return nil, fmt.Errorf("add notifications for %d users: %w", len(notifications), err)
	}
	return deliveredUserIDs, nil
}

// CreateNotification stores a single notification through the notification
// manager so cache, persistence, and push fan-out stay consistent.
func (ns *NotificationService) CreateNotification(notification models.Notification) (models.Notification, error) {
	if ns.notificationManager == nil {
		return notification, fmt.Errorf("notification manager not configured")
	}
	return ns.notificationManager.AddNotification(notification)
}

// DeleteUserNotifications removes all notifications for a user through the
// manager so any per-user notification cache is invalidated with the DB rows.
func (ns *NotificationService) DeleteUserNotifications(userID int) error {
	if ns.notificationManager == nil {
		return fmt.Errorf("notification manager not configured")
	}
	return ns.notificationManager.DeleteUserNotifications(userID)
}

// MarkNotificationsSent stamps sent_at for email-batched notifications through
// the manager rather than letting the scheduler mutate notification rows itself.
func (ns *NotificationService) MarkNotificationsSent(notificationIDs []int) error {
	if ns.notificationManager == nil {
		return fmt.Errorf("notification manager not configured")
	}
	return ns.notificationManager.MarkNotificationsSent(notificationIDs)
}

// MarkNotificationsSendFailed flags notification rows whose email-send rollback
// failed, keeping scheduler DB mutations behind the notification service layer.
func (ns *NotificationService) MarkNotificationsSendFailed(notificationIDs []int) error {
	if ns.notificationManager == nil {
		return fmt.Errorf("notification manager not configured")
	}
	return ns.notificationManager.MarkNotificationsSendFailed(notificationIDs)
}

// RollbackNotificationsSent clears sent_at after a failed SMTP send.
func (ns *NotificationService) RollbackNotificationsSent(notificationIDs []int) error {
	if ns.notificationManager == nil {
		return fmt.Errorf("notification manager not configured")
	}
	return ns.notificationManager.RollbackNotificationsSent(notificationIDs)
}

// EmitEvent durably admits an event when the domain event engine is installed.
// The bounded in-memory path remains available to standalone embeddings.
func (ns *NotificationService) EmitEvent(event *NotificationEvent) {
	ns.TryEmitEvent(event)
}

// TryEmitEvent reports whether the event was durably admitted or queued.
func (ns *NotificationService) TryEmitEvent(event *NotificationEvent) bool {
	if event == nil {
		atomic.AddInt64(&ns.eventsDropped, 1)
		atomic.AddInt64(&ns.errors, 1)
		return false
	}
	slog.Debug("queuing event", slog.String("component", "notifications"), slog.String("event_type", event.EventType), slog.Int("workspace_id", event.WorkspaceID), slog.Int("actor_user_id", event.ActorUserID), slog.Int("item_id", event.ItemID))
	if ns.durableIngress != nil {
		if err := ns.durableIngress.Emit(context.Background(), event); err != nil {
			slog.Error("failed to admit durable notification event", "event_type", event.EventType, "error", err)
			atomic.AddInt64(&ns.eventsDropped, 1)
			atomic.AddInt64(&ns.errors, 1)
			return false
		}
		atomic.AddInt64(&ns.eventsQueued, 1)
		return true
	}

	ns.emitMu.RLock()
	defer ns.emitMu.RUnlock()
	if ns.closed {
		atomic.AddInt64(&ns.eventsDropped, 1)
		return false
	}
	select {
	case ns.eventChan <- queuedNotificationEvent{event: event, enqueuedAt: time.Now()}:
		atomic.AddInt64(&ns.eventsQueued, 1)
		slog.Debug("event queued successfully", slog.String("component", "notifications"), slog.String("event_type", event.EventType), slog.Int("item_id", event.ItemID))
		return true
	default:
		slog.Warn("event channel full, dropping event", slog.String("component", "notifications"), slog.String("event_type", event.EventType), slog.Int("workspace_id", event.WorkspaceID))
		atomic.AddInt64(&ns.eventsDropped, 1)
		atomic.AddInt64(&ns.errors, 1)
		return false
	}
}

// eventProcessor runs as one member of the fixed worker pool.
func (ns *NotificationService) eventProcessor(workerID int) {
	defer ns.wg.Done()
	for queued := range ns.eventChan {
		wait := time.Since(queued.enqueuedAt)
		atomic.StoreInt64(&ns.lastQueueWaitNS, int64(wait))
		updateNotificationServiceMaximum(&ns.maxQueueWaitNS, int64(wait))
		active := atomic.AddInt64(&ns.activeWorkers, 1)
		updateNotificationServiceMaximum(&ns.maxActiveWorkers, active)
		ctx, cancel := context.WithTimeout(ns.workerCtx, ns.config.EventProcessTimeout)
		processEvent := ns.processEvent
		if ns.processEventFn != nil {
			processEvent = ns.processEventFn
		}
		err := processEvent(ctx, queued.event)
		cancel()
		atomic.AddInt64(&ns.activeWorkers, -1)
		if err != nil {
			slog.Error("failed to process notification event", slog.String("component", "notifications"), slog.Int("worker_id", workerID), slog.String("event_type", queued.event.EventType), slog.Any("error", err))
			atomic.AddInt64(&ns.errors, 1)
		} else {
			atomic.AddInt64(&ns.eventsProcessed, 1)
		}
	}
}

// cacheRefresher runs in background and periodically refreshes the rule cache
func (ns *NotificationService) cacheRefresher() {
	defer ns.wg.Done()

	ticker := time.NewTicker(ns.config.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := ns.refreshRuleCache(); err != nil {
				slog.Error("failed to refresh notification rule cache", slog.String("component", "notifications"), slog.Any("error", err))
			}
		case <-ns.cacheStop:
			slog.Debug("stopping notification cache refresher", slog.String("component", "notifications"))
			return
		}
	}
}

// processEvent processes a single notification event
func (ns *NotificationService) processEvent(ctx context.Context, event *NotificationEvent) error {
	return ns.processEventWithKey(ctx, event, "")
}

func (ns *NotificationService) processEventWithKey(ctx context.Context, event *NotificationEvent, eventKey string) error {
	slog.Debug("processing event", slog.String("component", "notifications"), slog.String("event_type", event.EventType), slog.Int("workspace_id", event.WorkspaceID), slog.Int("item_id", event.ItemID))

	// Get configuration set for workspace
	configSetID, err := ns.getConfigSetForWorkspace(event.WorkspaceID)
	if err != nil || configSetID == 0 {
		// No configuration set, skip notifications
		slog.Debug("no config set for workspace, skipping notifications", slog.String("component", "notifications"), slog.Int("workspace_id", event.WorkspaceID))
		return err
	}

	slog.Debug("found config set for workspace", slog.String("component", "notifications"), slog.Int("config_set_id", configSetID), slog.Int("workspace_id", event.WorkspaceID))

	// Get event rules for this config set
	rules := ns.getEventRules(configSetID, event.EventType)
	if len(rules) == 0 {
		// No rules for this event type
		slog.Debug("no rules for event type in config set", slog.String("component", "notifications"), slog.String("event_type", event.EventType), slog.Int("config_set_id", configSetID))
		return nil
	}

	slog.Debug("found rules for event type", slog.String("component", "notifications"), slog.Int("rule_count", len(rules)), slog.String("event_type", event.EventType))

	// Resolve every rule into one atomic recipient batch for this event.
	notifications := make([]models.Notification, 0)
	now := time.Now()
	for _, rule := range rules {
		if !rule.IsEnabled {
			slog.Debug("rule is disabled, skipping", slog.String("component", "notifications"), slog.Int("rule_id", rule.ID))
			continue
		}

		slog.Debug("processing rule for event", slog.String("component", "notifications"), slog.Int("rule_id", rule.ID), slog.String("event_type", event.EventType))

		// Determine recipients
		recipients := ns.determineRecipients(event, &rule)
		if len(recipients) == 0 {
			slog.Debug("no recipients for rule", slog.String("component", "notifications"), slog.Int("rule_id", rule.ID))
			continue
		}

		slog.Debug("determined recipients for rule", slog.String("component", "notifications"), slog.Int("recipient_count", len(recipients)), slog.Int("rule_id", rule.ID), slog.Any("recipients", recipients))

		// Generate notification message
		title, message := ns.generateNotificationMessage(event, &rule)

		// Build one notification for each recipient.
		for _, userID := range recipients {
			// Skip if recipient is the actor (don't notify yourself)
			if userID == event.ActorUserID {
				slog.Debug("skipping notification for actor", slog.String("component", "notifications"), slog.Int("user_id", userID))
				continue
			}

			deliveryKey := ""
			if eventKey != "" {
				deliveryKey = fmt.Sprintf("%s:%d:%d", eventKey, rule.ID, userID)
			}
			notifications = append(notifications, models.Notification{
				UserID:                        userID,
				Title:                         title,
				Message:                       message,
				Type:                          ns.getNotificationType(event.EventType),
				Timestamp:                     now,
				Read:                          false,
				ActionURL:                     itemActionURL(event.WorkspaceID, event.ItemID),
				AuthorizationScope:            models.NotificationScopeWorkspace,
				WorkspaceID:                   &event.WorkspaceID,
				ItemID:                        &event.ItemID,
				SourceType:                    event.EventType,
				SourceID:                      &event.ItemID,
				ReferencedEntityType:          event.ReferencedEntityType,
				ReferencedEntityID:            positiveIntPointer(event.ReferencedEntityID),
				ReferencedWorkspaceID:         positiveIntPointer(event.ReferencedWorkspaceID),
				ReferencedWorkspacePermission: event.ReferencedWorkspacePermission,
				DeliveryKey:                   deliveryKey,
			})
		}
	}
	if len(notifications) > 0 {
		if _, err := ns.notificationManager.AddNotificationsContext(ctx, notifications); err != nil {
			return fmt.Errorf("persist %d event notifications: %w", len(notifications), err)
		}
	}

	slog.Debug("completed processing event", slog.String("component", "notifications"), slog.String("event_type", event.EventType), slog.Int("item_id", event.ItemID), slog.Int("notification_count", len(notifications)))
	return nil
}

// refreshRuleCache reloads notification rules from database
func (ns *NotificationService) refreshRuleCache() error {
	ns.cacheMu.Lock()
	defer ns.cacheMu.Unlock()

	newCache := &RuleCache{
		WorkspaceConfigSets: make(map[int]int),
		EventRules:          make(map[int][]models.NotificationEventRule),
		Templates:           make(map[string]string),
		PersonalWorkspaces:  make(map[int]bool),
		LastRefreshed:       time.Now(),
	}

	// Default configuration set — the fallback notification scheme for any
	// workspace with no config set assigned, mirroring how an unassigned
	// workflow falls back to workflows.is_default (workflow_service.go).
	// A missing default row leaves this 0, which resolves to "no rules" (skip).
	if err := ns.db.QueryRow(`SELECT id FROM configuration_sets WHERE is_default = true LIMIT 1`).Scan(&newCache.DefaultConfigSetID); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			slog.Warn("failed to load default configuration set for notifications", slog.String("component", "notifications"), slog.Any("error", err))
		}
	}

	// Personal workspaces never receive rule-based notifications. Cache the
	// set so the async resolver can skip them without a per-event query.
	personalRows, err := ns.db.Query(`SELECT id FROM workspaces WHERE is_personal = true`)
	if err != nil {
		slog.Warn("failed to load personal workspaces for notifications", slog.String("component", "notifications"), slog.Any("error", err))
	} else {
		for personalRows.Next() {
			var wsID int
			if err := personalRows.Scan(&wsID); err != nil {
				slog.Error("failed to scan personal workspace", slog.String("component", "notifications"), slog.Any("error", err))
				continue
			}
			newCache.PersonalWorkspaces[wsID] = true
		}
		if err := personalRows.Err(); err != nil {
			slog.Warn("failed to iterate personal workspaces", slog.String("component", "notifications"), slog.Any("error", err))
		}
		_ = personalRows.Close()
	}

	// Load workspace -> config_set mappings
	rows, err := ns.db.Query(`
		SELECT workspace_id, configuration_set_id
		FROM workspace_configuration_sets
	`)
	if err != nil {
		return fmt.Errorf("failed to load workspace configuration sets: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var workspaceID, configSetID int
		if err = rows.Scan(&workspaceID, &configSetID); err != nil {
			slog.Error("failed to scan workspace config set", slog.String("component", "notifications"), slog.Any("error", err))
			continue
		}
		newCache.WorkspaceConfigSets[workspaceID] = configSetID
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to iterate workspace configuration sets: %w", err)
	}

	// Load event rules for each config set with notification settings
	ruleRows, err := ns.db.Query(`
		SELECT
			ner.id, ner.notification_setting_id, ner.event_type, ner.is_enabled,
			ner.notify_assignee, ner.notify_creator, ner.notify_watchers, ner.notify_workspace_admins,
			ner.custom_recipients, ner.message_template,
			csns.configuration_set_id
		FROM notification_event_rules ner
		JOIN notification_settings ns ON ns.id = ner.notification_setting_id
		JOIN configuration_set_notification_settings csns ON csns.notification_setting_id = ns.id
		WHERE ns.is_active = true AND ner.is_enabled = true
	`)
	if err != nil {
		return fmt.Errorf("failed to load notification event rules: %w", err)
	}
	defer func() { _ = ruleRows.Close() }()

	for ruleRows.Next() {
		var rule models.NotificationEventRule
		var configSetID int
		var customRecipients, messageTemplate *string

		if err = ruleRows.Scan(
			&rule.ID, &rule.NotificationSettingID, &rule.EventType, &rule.IsEnabled,
			&rule.NotifyAssignee, &rule.NotifyCreator, &rule.NotifyWatchers, &rule.NotifyWorkspaceAdmins,
			&customRecipients, &messageTemplate,
			&configSetID,
		); err != nil {
			slog.Error("failed to scan notification event rule", slog.String("component", "notifications"), slog.Any("error", err))
			continue
		}

		if customRecipients != nil {
			rule.CustomRecipients = *customRecipients
		}
		if messageTemplate != nil {
			rule.MessageTemplate = *messageTemplate
		}

		newCache.EventRules[configSetID] = append(newCache.EventRules[configSetID], rule)
	}
	if err := ruleRows.Err(); err != nil {
		return fmt.Errorf("failed to iterate notification event rules: %w", err)
	}

	// Load notification templates
	templateRows, err := ns.db.Query(`
		SELECT name, content
		FROM notification_templates
		WHERE is_active = true AND template_type = 'notification_type'
	`)
	if err != nil {
		slog.Warn("failed to load notification templates", slog.String("component", "notifications"), slog.Any("error", err))
	} else {
		defer func() { _ = templateRows.Close() }()
		for templateRows.Next() {
			var name, content string
			if err := templateRows.Scan(&name, &content); err != nil {
				slog.Error("failed to scan template", slog.String("component", "notifications"), slog.Any("error", err))
				continue
			}
			newCache.Templates[name] = content
		}
		if err := templateRows.Err(); err != nil {
			slog.Warn("failed to iterate notification templates", slog.String("component", "notifications"), slog.Any("error", err))
		}
	}

	// Swap cache
	ns.ruleCache = newCache

	slog.Debug("notification rule cache refreshed", slog.String("component", "notifications"), slog.Int("workspace_count", len(newCache.WorkspaceConfigSets)), slog.Int("config_set_count", len(newCache.EventRules)), slog.Int("template_count", len(newCache.Templates)))

	return nil
}

// getConfigSetForWorkspace skips personal workspaces, then uses the assigned
// set or default. An assigned set with no rules intentionally suppresses
// notifications and must not fall back.
func (ns *NotificationService) getConfigSetForWorkspace(workspaceID int) (int, error) {
	ns.cacheMu.RLock()
	defer ns.cacheMu.RUnlock()

	if ns.ruleCache.PersonalWorkspaces[workspaceID] {
		return 0, nil
	}

	if configSetID, exists := ns.ruleCache.WorkspaceConfigSets[workspaceID]; exists && configSetID != 0 {
		atomic.AddInt64(&ns.cacheHits, 1)
		return configSetID, nil
	}

	atomic.AddInt64(&ns.cacheMisses, 1)
	return ns.ruleCache.DefaultConfigSetID, nil
}

// getEventRules retrieves event rules for a config set and event type (cached)
func (ns *NotificationService) getEventRules(configSetID int, eventType string) []models.NotificationEventRule {
	ns.cacheMu.RLock()
	defer ns.cacheMu.RUnlock()

	allRules, exists := ns.ruleCache.EventRules[configSetID]
	if !exists {
		return nil
	}

	// Filter rules for this specific event type
	var matchingRules []models.NotificationEventRule
	for _, rule := range allRules {
		if rule.EventType == eventType {
			matchingRules = append(matchingRules, rule)
		}
	}

	return matchingRules
}

// determineRecipients determines who should receive notifications based on rule configuration
func (ns *NotificationService) determineRecipients(event *NotificationEvent, rule *models.NotificationEventRule) []int {
	recipientSet := make(map[int]bool)

	// Add assignee
	if rule.NotifyAssignee && event.AssigneeID != nil && *event.AssigneeID > 0 {
		recipientSet[*event.AssigneeID] = true
	}

	// Add creator
	if rule.NotifyCreator && event.CreatorID != nil && *event.CreatorID > 0 {
		recipientSet[*event.CreatorID] = true
	}

	// Add workspace admins
	if rule.NotifyWorkspaceAdmins {
		adminIDs := ns.getWorkspaceAdmins(event.WorkspaceID)
		for _, adminID := range adminIDs {
			recipientSet[adminID] = true
		}
	}

	// Add custom recipients. Schema + validator constrain this to a JSON
	// array of integer user IDs; anything else (e.g. legacy email strings
	// from older rows) is logged and dropped — emails are not supported.
	if rule.CustomRecipients != "" && rule.CustomRecipients != "[]" {
		var customIDs []int
		if err := json.Unmarshal([]byte(rule.CustomRecipients), &customIDs); err != nil {
			slog.Warn("custom_recipients is not a []int; dropping",
				slog.String("component", "notifications"),
				slog.Int("rule_id", rule.ID),
				slog.String("event_type", rule.EventType),
				slog.Any("error", err))
		} else {
			for _, userID := range customIDs {
				recipientSet[userID] = true
			}
		}
	}

	// Add watchers
	if rule.NotifyWatchers {
		watcherIDs := ns.getItemWatchers(event.ItemID)
		for _, watcherID := range watcherIDs {
			recipientSet[watcherID] = true
		}
	}

	// Re-authorize every recipient against current workspace view permission.
	// Watches, custom-recipient lists and admin roles all outlive permission
	// changes; without this check a revoked user keeps receiving titles and
	// action URLs for items they can no longer see. Agent / service-user
	// rows (is_agent = true) are filtered out too — they're non-human API
	// principals and don't consume notifications even when a rule routes
	// to them as assignee, watcher, or admin.
	candidates := make([]int, 0, len(recipientSet))
	for userID := range recipientSet {
		candidates = append(candidates, userID)
	}
	skipAsAgent := ns.agentOrUnknownUsers(candidates)

	recipients := make([]int, 0, len(candidates))
	for _, userID := range candidates {
		if skipAsAgent[userID] {
			continue
		}
		if !ns.canViewWorkspace(userID, event.WorkspaceID) {
			continue
		}
		if event.ReferencedEntityType != "" || event.ReferencedWorkspacePermission != "" {
			snapshot, err := ns.notificationAuth.Snapshot(userID)
			if err != nil {
				continue
			}
			visible, err := snapshot.Visible(models.Notification{
				UserID:                        userID,
				AuthorizationScope:            models.NotificationScopeSystem,
				SourceType:                    event.EventType,
				ReferencedEntityType:          event.ReferencedEntityType,
				ReferencedEntityID:            positiveIntPointer(event.ReferencedEntityID),
				ReferencedWorkspaceID:         positiveIntPointer(event.ReferencedWorkspaceID),
				ReferencedWorkspacePermission: event.ReferencedWorkspacePermission,
			})
			if err != nil || !visible {
				continue
			}
		}
		recipients = append(recipients, userID)
	}

	return recipients
}

func positiveIntPointer(value int) *int {
	if value <= 0 {
		return nil
	}
	return &value
}

// agentOrUnknownUsers batch-loads notification exclusions and fails closed for non-human or unreadable users.
func (ns *NotificationService) agentOrUnknownUsers(userIDs []int) map[int]bool {
	skip := make(map[int]bool, len(userIDs))
	if len(userIDs) == 0 {
		return skip
	}

	skipAll := func() map[int]bool {
		for _, id := range userIDs {
			skip[id] = true
		}
		return skip
	}

	placeholders := make([]string, len(userIDs))
	args := make([]any, len(userIDs))
	for i, id := range userIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	rows, err := ns.db.Query(
		fmt.Sprintf(`SELECT id, is_agent, is_active FROM users WHERE id IN (%s)`, strings.Join(placeholders, ",")),
		args...)
	if err != nil {
		slog.Warn("is_agent batch check failed during recipient filtering; skipping all",
			slog.String("component", "notifications"), slog.Any("error", err))
		return skipAll()
	}
	defer rows.Close()

	seen := make(map[int]bool, len(userIDs))
	for rows.Next() {
		var id int
		var isAgent, isActive bool
		if err := rows.Scan(&id, &isAgent, &isActive); err != nil {
			continue
		}
		seen[id] = true
		if isAgent || !isActive {
			skip[id] = true
		}
	}
	if err := rows.Err(); err != nil {
		slog.Warn("is_agent batch iteration failed during recipient filtering; skipping all",
			slog.String("component", "notifications"), slog.Any("error", err))
		return skipAll()
	}

	// Any candidate without a row is unknown → fail closed (skip).
	for _, id := range userIDs {
		if !seen[id] {
			skip[id] = true
		}
	}
	return skip
}

// canViewWorkspace returns true when the user currently has item-view
// permission on the workspace. A nil permService (test wiring) means we
// fall back to "allow" so legacy paths keep working.
func (ns *NotificationService) canViewWorkspace(userID, workspaceID int) bool {
	return ns.canUseWorkspacePermission(userID, workspaceID, models.PermissionItemView)
}

func (ns *NotificationService) canUseWorkspacePermission(userID, workspaceID int, permission string) bool {
	if ns.permService == nil {
		return true
	}
	ok, err := ns.permService.HasWorkspacePermission(userID, workspaceID, permission)
	if err != nil {
		slog.Warn("permission check failed during recipient filtering; denying",
			slog.String("component", "notifications"),
			slog.Int("user_id", userID),
			slog.Int("workspace_id", workspaceID),
			slog.Any("error", err))
		return false
	}
	return ok
}

// queryUserIDs runs a query that selects a single user-id column and returns
// the collected ids. Query / iteration failures are logged under errLabel and
// yield whatever was read so far — recipient lookups degrade to "fewer
// recipients", never a hard failure.
func (ns *NotificationService) queryUserIDs(errLabel, query string, args ...any) []int {
	rows, err := ns.db.Query(query, args...)
	if err != nil {
		slog.Error("failed to "+errLabel, slog.String("component", "notifications"), slog.Any("error", err))
		return nil
	}
	defer rows.Close()

	var ids []int
	for rows.Next() {
		var userID int
		if err := rows.Scan(&userID); err == nil {
			ids = append(ids, userID)
		}
	}
	if err := rows.Err(); err != nil {
		slog.Error("failed to iterate "+errLabel, slog.String("component", "notifications"), slog.Any("error", err))
	}
	return ids
}

// getWorkspaceAdmins retrieves admin user IDs for a workspace
func (ns *NotificationService) getWorkspaceAdmins(workspaceID int) []int {
	return ns.queryUserIDs("fetch workspace admins", `
		SELECT DISTINCT uwr.user_id
		FROM user_workspace_roles uwr
		JOIN workspace_roles wr ON uwr.role_id = wr.id
		WHERE uwr.workspace_id = ? AND wr.builtin_key = ?
	`, workspaceID, models.RoleBuiltinAdministrator)
}

// getItemWatchers retrieves active watcher user IDs for an item
func (ns *NotificationService) getItemWatchers(itemID int) []int {
	watchers, err := repository.NewItemRepository(ns.db).GetWatchers(itemID)
	if err != nil {
		slog.Error("failed to fetch item watchers", slog.String("component", "notifications"), slog.Any("error", err))
		return nil
	}
	return watchers
}

// generateNotificationMessage generates title and message for a notification
func (ns *NotificationService) generateNotificationMessage(event *NotificationEvent, rule *models.NotificationEventRule) (subject, body string) {
	// Use custom template if provided
	if rule.MessageTemplate != "" {
		return ns.applyTemplate(event, rule.MessageTemplate)
	}

	// Check for DB template
	ns.cacheMu.RLock()
	template, hasTemplate := ns.ruleCache.Templates[event.EventType]
	ns.cacheMu.RUnlock()

	if hasTemplate {
		return ns.applyTemplate(event, template)
	}

	// Fall back to default templates
	return ns.getDefaultMessage(event)
}

// applyTemplate applies template variables to a template string.
// When the rendered template contains a newline, the first line is treated as
// the subject and the remainder as the body; a single-line template falls
// back to event.Title so templates don't have to repeat a title they already
// know. Previously the first line was silently discarded and event.Title was
// always used, which made multi-line templates look broken in mailboxes.
func (ns *NotificationService) applyTemplate(event *NotificationEvent, template string) (subject, body string) {
	message := template
	for key, value := range event.TemplateData {
		placeholder := fmt.Sprintf("{%s}", key)
		message = strings.ReplaceAll(message, placeholder, fmt.Sprintf("%v", value))
	}

	lines := strings.SplitN(message, "\n", 2)
	if len(lines) == 2 {
		return strings.TrimSpace(lines[0]), lines[1]
	}
	return event.Title, message
}

// getDefaultMessage generates default notification message based on event type
func (ns *NotificationService) getDefaultMessage(event *NotificationEvent) (subject, body string) {
	title := event.Title

	var message string
	data := event.TemplateData

	// Helper function to get item identifier - prefer key over title
	getItemIdentifier := func() string {
		if itemKey, ok := data["item.key"]; ok && itemKey != nil && itemKey != "" {
			return fmt.Sprintf("%v", itemKey)
		}
		if itemTitle, ok := data["item.title"]; ok && itemTitle != nil {
			return fmt.Sprintf("%v", itemTitle)
		}
		return "Unknown Item"
	}

	switch event.EventType {
	case models.EventItemCreated:
		message = fmt.Sprintf("New work item created: %s", getItemIdentifier())
	case models.EventItemUpdated:
		message = fmt.Sprintf("Work item updated: %s", getItemIdentifier())
	case models.EventItemDeleted:
		message = fmt.Sprintf("Work item deleted: %s", getItemIdentifier())
	case models.EventItemAssigned:
		message = fmt.Sprintf("You have been assigned to: %s", getItemIdentifier())
	case models.EventStatusChanged:
		message = fmt.Sprintf("Status changed to %s for: %s", data["status.name"], getItemIdentifier())
	case models.EventCommentCreated:
		message = fmt.Sprintf("New comment added by %s on: %s", data["user.name"], getItemIdentifier())
	case models.EventCommentUpdated:
		message = fmt.Sprintf("Comment updated by %s on: %s", data["user.name"], getItemIdentifier())
	case models.EventCommentDeleted:
		message = fmt.Sprintf("Comment deleted by %s on: %s", data["user.name"], getItemIdentifier())
	case models.EventItemLinked:
		message = fmt.Sprintf("Work items linked: %s", getItemIdentifier())
	case models.EventItemUnlinked:
		message = fmt.Sprintf("Work item link removed: %s", getItemIdentifier())
	case models.EventMention:
		actorName := "Someone"
		if name, ok := data["actor.name"]; ok && name != nil && name != "" {
			actorName = fmt.Sprintf("%v", name)
		}
		sourceType := "content"
		if st, ok := data["source.type"]; ok && st != nil {
			sourceType = fmt.Sprintf("%v", st)
		}
		message = fmt.Sprintf("%s mentioned you in %s on %s", actorName, sourceType, getItemIdentifier())
	default:
		message = fmt.Sprintf("Event: %s", event.EventType)
	}

	return title, message
}

// getNotificationType maps event types to notification types for UI display
func (ns *NotificationService) getNotificationType(eventType string) string {
	switch eventType {
	case models.EventItemAssigned:
		return "assignment"
	case models.EventCommentCreated, models.EventCommentUpdated, models.EventCommentDeleted:
		return "comment"
	case models.EventStatusChanged:
		return "status_change"
	case models.EventMention:
		return "mention"
	case models.EventItemDeleted:
		return "warning"
	default:
		return "info"
	}
}

// Close gracefully shuts down the notification service
func (ns *NotificationService) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), ns.config.ShutdownTimeout)
	defer cancel()
	return ns.CloseContext(ctx)
}

// CloseContext stops accepting events, drains the bounded queue, and reports
// any work that remains when the caller deadline expires.
func (ns *NotificationService) CloseContext(ctx context.Context) error {
	slog.Debug("closing notification service", slog.String("component", "notifications"))
	ns.closeOnce.Do(func() {
		ns.emitMu.Lock()
		ns.closed = true
		close(ns.eventChan)
		close(ns.cacheStop)
		ns.emitMu.Unlock()
	})
	done := make(chan struct{})
	go func() {
		ns.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		ns.workerCancel()
		slog.Debug("notification service closed successfully", slog.String("component", "notifications"))
		return nil
	case <-ctx.Done():
		ns.workerCancel()
		return fmt.Errorf("notification service shutdown with %d queued and %d active: %w", len(ns.eventChan), atomic.LoadInt64(&ns.activeWorkers), ctx.Err())
	}
}

// GetStats returns service statistics
func (ns *NotificationService) GetStats() map[string]int64 {
	ns.cacheMu.RLock()
	lastRefreshed := ns.ruleCache.LastRefreshed
	ns.cacheMu.RUnlock()

	return map[string]int64{
		"events_queued":      atomic.LoadInt64(&ns.eventsQueued),
		"events_processed":   atomic.LoadInt64(&ns.eventsProcessed),
		"events_dropped":     atomic.LoadInt64(&ns.eventsDropped),
		"pending_events":     int64(len(ns.eventChan)),
		"active_workers":     atomic.LoadInt64(&ns.activeWorkers),
		"max_active_workers": atomic.LoadInt64(&ns.maxActiveWorkers),
		"last_queue_wait_ms": time.Duration(atomic.LoadInt64(&ns.lastQueueWaitNS)).Milliseconds(),
		"max_queue_wait_ms":  time.Duration(atomic.LoadInt64(&ns.maxQueueWaitNS)).Milliseconds(),
		"cache_hits":         atomic.LoadInt64(&ns.cacheHits),
		"cache_misses":       atomic.LoadInt64(&ns.cacheMisses),
		"errors":             atomic.LoadInt64(&ns.errors),
		"cache_age_seconds":  int64(time.Since(lastRefreshed).Seconds()),
	}
}

func updateNotificationServiceMaximum(target *int64, value int64) {
	for {
		previous := atomic.LoadInt64(target)
		if value <= previous || atomic.CompareAndSwapInt64(target, previous, value) {
			return
		}
	}
}

// ForceRefreshCache manually refreshes the notification rule cache
// This is useful for admins to force cache refresh after configuration changes
func (ns *NotificationService) ForceRefreshCache() error {
	slog.Debug("force refreshing notification rule cache", slog.String("component", "notifications"))
	return ns.refreshRuleCache()
}
