package scheduler

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// defaultBatchInterval is the production cadence: every 5 minutes the
// scheduler scans for unread notifications and emails one batch per user.
// Override via WINDSHIFT_NOTIFICATION_BATCH_INTERVAL (e.g. "5s") for tests
// or for deployments that want a different cadence.
const defaultBatchInterval = 5 * time.Minute

// resolveBatchInterval reads the env override and falls back to the default
// when it's missing, malformed, or non-positive.
func resolveBatchInterval() time.Duration {
	raw := strings.TrimSpace(os.Getenv("WINDSHIFT_NOTIFICATION_BATCH_INTERVAL"))
	if raw == "" {
		return defaultBatchInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		slog.Warn("invalid WINDSHIFT_NOTIFICATION_BATCH_INTERVAL, using default",
			slog.String("component", "scheduler"),
			slog.String("raw", raw),
			slog.Any("error", err))
		return defaultBatchInterval
	}
	slog.Info("notification batch interval overridden via env",
		slog.String("component", "scheduler"),
		slog.String("interval", d.String()))
	return d
}

// maxBatchSize caps how many notifications one email can carry. Without this,
// a user with a thousand unread notifications gets a single oversize email on
// every 5-minute tick (and many SMTP servers will reject it).
const maxBatchSize = 50

// maxConsecutiveFailures is the per-user circuit breaker. After K failed
// sends in a row, we stop trying and let the user's notifications accumulate
// until the cooldown expires, instead of re-flooding the SMTP server with
// a growing batch every tick.
const maxConsecutiveFailures = 5

// failureCooldown is how long to skip a user after hitting the failure cap.
const failureCooldown = 30 * time.Minute

// NotificationScheduler handles batching and sending of notifications every 5 minutes
type NotificationScheduler struct {
	db         database.Database
	ticker     *time.Ticker
	stopChan   chan struct{}
	mu         sync.RWMutex
	running    bool
	smtpSender SMTPSender

	// Per-user failure tracking for the circuit breaker. Keyed by user email
	// (matching how we fan-out batches today). Resets on restart — the goal
	// is to prevent hot-loop flooding within a process, not to persist state.
	failMu     sync.Mutex
	failCounts map[string]int
	skipUntil  map[string]time.Time
}

// SMTPSender interface for sending emails
type SMTPSender interface {
	SendBatchedNotifications(userEmail, userName string, notifications []models.Notification) error
	IsSMTPConfigured() bool
}

// NewNotificationScheduler creates a new notification scheduler
func NewNotificationScheduler(db database.Database, smtpSender SMTPSender) *NotificationScheduler {
	return &NotificationScheduler{
		db:         db,
		ticker:     time.NewTicker(resolveBatchInterval()),
		stopChan:   make(chan struct{}),
		running:    false,
		smtpSender: smtpSender,
		failCounts: make(map[string]int),
		skipUntil:  make(map[string]time.Time),
	}
}

// Start begins the notification batching scheduler
func (ns *NotificationScheduler) Start() {
	ns.mu.Lock()
	defer ns.mu.Unlock()

	if ns.running {
		return
	}

	ns.running = true
	slog.Debug("Starting notification scheduler", slog.String("component", "scheduler"), slog.String("interval", "5m"))

	go ns.schedulerLoop()
}

// Stop stops the notification scheduler
func (ns *NotificationScheduler) Stop() {
	ns.mu.Lock()
	defer ns.mu.Unlock()

	if !ns.running {
		return
	}

	ns.running = false
	ns.ticker.Stop()
	close(ns.stopChan)
	slog.Debug("Notification scheduler stopped", slog.String("component", "scheduler"))
}

// schedulerLoop runs the main scheduler loop
func (ns *NotificationScheduler) schedulerLoop() {
	for {
		select {
		case <-ns.ticker.C:
			ns.processPendingNotifications()
		case <-ns.stopChan:
			return
		}
	}
}

// processPendingNotifications finds unread notifications and sends them in batches
func (ns *NotificationScheduler) processPendingNotifications() {
	// Check if SMTP is configured first
	if !ns.smtpSender.IsSMTPConfigured() {
		slog.Debug("SMTP not configured, skipping notification batch processing", slog.String("component", "scheduler"))
		return
	}

	slog.Debug("Processing notification batches", slog.String("component", "scheduler"))

	// Get all users with unread notifications
	userBatches, err := ns.getUnreadNotificationsByUser()
	if err != nil {
		slog.Error("Failed to get unread notifications", slog.String("component", "scheduler"), slog.Any("error", err))
		return
	}

	if len(userBatches) == 0 {
		slog.Debug("No unread notifications to process", slog.String("component", "scheduler"))
		return
	}

	// Send batches for each user
	for userEmail, batch := range userBatches {
		if ns.inCooldown(userEmail) {
			slog.Debug("skipping user in failure cooldown",
				slog.String("component", "scheduler"),
				slog.String("user_email", userEmail),
			)
			continue
		}
		// Mark sent_at BEFORE the SMTP call. If the send succeeds, we're
		// done. If it fails, we roll back sent_at for the batch. Without
		// this pre-mark, a crash or mark-as-sent DB failure between send
		// and mark would re-send the same batch on the next tick.
		if err := ns.markNotificationsSent(batch.NotificationIDs); err != nil {
			slog.Error("failed to pre-mark notifications sent; skipping batch",
				slog.String("component", "scheduler"),
				slog.String("user_email", userEmail),
				slog.Any("error", err),
			)
			continue
		}
		if err := ns.sendNotificationBatch(batch); err != nil {
			slog.Error("Failed to send notification batch", slog.String("component", "scheduler"), slog.String("user_email", userEmail), slog.Any("error", err))
			if rollbackErr := ns.rollbackSent(batch.NotificationIDs); rollbackErr != nil {
				slog.Error("failed to roll back sent_at after SMTP failure",
					slog.String("component", "scheduler"),
					slog.String("user_email", userEmail),
					slog.Any("error", rollbackErr),
				)
			}
			ns.recordFailure(userEmail)
			continue
		}
		ns.recordSuccess(userEmail)
	}

	slog.Debug("Processed notification batches", slog.String("component", "scheduler"), slog.Int("batch_count", len(userBatches)))
}

// UserNotificationBatch represents a batch of notifications for a user
type UserNotificationBatch struct {
	UserID          int
	UserEmail       string
	UserName        string
	Notifications   []models.Notification
	NotificationIDs []int
}

// getUnreadNotificationsByUser gets notifications that still need emailing,
// grouped by user. Filters out in-app-read notifications so a user who reads
// their tray within the batch window doesn't get a redundant email. Caps each
// user's batch at maxBatchSize to keep single emails under SMTP-reasonable
// sizes even for users with a huge backlog.
func (ns *NotificationScheduler) getUnreadNotificationsByUser() (map[string]*UserNotificationBatch, error) {
	query := `
		SELECT n.id, n.user_id, n.title, n.message, n.type, n.timestamp, n.read,
		       n.sent_at, n.avatar, n.action_url, n.metadata, n.created_at, n.updated_at,
		       u.email, u.first_name, u.last_name
		FROM notifications n
		JOIN users u ON n.user_id = u.id
		WHERE n.sent_at IS NULL AND n.read = false AND u.email != ''
		ORDER BY u.email, n.timestamp DESC
	`

	rows, err := ns.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query unread notifications: %w", err)
	}
	defer rows.Close()

	userBatches := make(map[string]*UserNotificationBatch)

	for rows.Next() {
		var n models.Notification
		var avatar, actionURL, metadata *string
		var email, firstName, lastName string

		err := rows.Scan(
			&n.ID, &n.UserID, &n.Title, &n.Message, &n.Type,
			&n.Timestamp, &n.Read, &n.SentAt, &avatar, &actionURL, &metadata,
			&n.CreatedAt, &n.UpdatedAt, &email, &firstName, &lastName,
		)
		if err != nil {
			return nil, err
		}

		// Set optional fields
		if avatar != nil {
			n.Avatar = *avatar
		}
		if actionURL != nil {
			n.ActionURL = *actionURL
		}
		if metadata != nil {
			n.Metadata = *metadata
		}

		// Get or create user batch
		batch, exists := userBatches[email]
		if !exists {
			userName := fmt.Sprintf("%s %s", firstName, lastName)
			batch = &UserNotificationBatch{
				UserID:          n.UserID,
				UserEmail:       email,
				UserName:        userName,
				Notifications:   []models.Notification{},
				NotificationIDs: []int{},
			}
			userBatches[email] = batch
		}

		// Cap per-user batch size — extras roll to the next tick.
		if len(batch.Notifications) >= maxBatchSize {
			continue
		}
		batch.Notifications = append(batch.Notifications, n)
		batch.NotificationIDs = append(batch.NotificationIDs, n.ID)
	}

	return userBatches, rows.Err()
}

// inCooldown reports whether the user is currently being skipped because of
// prior consecutive SMTP failures.
func (ns *NotificationScheduler) inCooldown(userEmail string) bool {
	ns.failMu.Lock()
	defer ns.failMu.Unlock()
	until, ok := ns.skipUntil[userEmail]
	if !ok {
		return false
	}
	if time.Now().Before(until) {
		return true
	}
	// Cooldown expired — clear state so the user gets another shot.
	delete(ns.skipUntil, userEmail)
	delete(ns.failCounts, userEmail)
	return false
}

func (ns *NotificationScheduler) recordFailure(userEmail string) {
	ns.failMu.Lock()
	defer ns.failMu.Unlock()
	ns.failCounts[userEmail]++
	if ns.failCounts[userEmail] >= maxConsecutiveFailures {
		ns.skipUntil[userEmail] = time.Now().Add(failureCooldown)
		slog.Warn("tripping notification cooldown for user after repeated SMTP failures",
			slog.String("component", "scheduler"),
			slog.String("user_email", userEmail),
			slog.Duration("cooldown", failureCooldown),
		)
	}
}

func (ns *NotificationScheduler) recordSuccess(userEmail string) {
	ns.failMu.Lock()
	defer ns.failMu.Unlock()
	delete(ns.failCounts, userEmail)
	delete(ns.skipUntil, userEmail)
}

// sendNotificationBatch sends a batch of notifications to a user
func (ns *NotificationScheduler) sendNotificationBatch(batch *UserNotificationBatch) error {
	if len(batch.Notifications) == 0 {
		return nil
	}

	return ns.smtpSender.SendBatchedNotifications(batch.UserEmail, batch.UserName, batch.Notifications)
}

// markNotificationsSent stamps sent_at so the scheduler will not re-batch
// them. The function used to be named markNotificationsAsRead but it never
// touched the `read` flag — only sent_at. Callers: pre-send optimistic mark
// (with rollback on SMTP failure).
func (ns *NotificationScheduler) markNotificationsSent(notificationIDs []int) error {
	if len(notificationIDs) == 0 {
		return nil
	}

	placeholders := make([]string, len(notificationIDs))
	args := make([]interface{}, len(notificationIDs)+2)
	now := time.Now()
	args[0] = now // sent_at
	args[1] = now // updated_at

	for i, id := range notificationIDs {
		placeholders[i] = "?"
		args[i+2] = id
	}

	query := fmt.Sprintf(`
		UPDATE notifications
		SET sent_at = ?, updated_at = ?
		WHERE id IN (%s)
	`, strings.Join(placeholders, ","))

	_, err := ns.db.Exec(query, args...)
	return err
}

// rollbackSent clears sent_at for a batch whose SMTP send ultimately failed,
// so the batch is eligible to retry on a future tick.
func (ns *NotificationScheduler) rollbackSent(notificationIDs []int) error {
	if len(notificationIDs) == 0 {
		return nil
	}
	placeholders := make([]string, len(notificationIDs))
	args := make([]interface{}, len(notificationIDs)+1)
	args[0] = time.Now()
	for i, id := range notificationIDs {
		placeholders[i] = "?"
		args[i+1] = id
	}
	query := fmt.Sprintf(`
		UPDATE notifications
		SET sent_at = NULL, updated_at = ?
		WHERE id IN (%s)
	`, strings.Join(placeholders, ","))
	_, err := ns.db.Exec(query, args...)
	return err
}
