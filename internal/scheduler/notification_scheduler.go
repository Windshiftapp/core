package scheduler

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// NotificationScheduler handles batching and sending of notifications every 5 minutes
type NotificationScheduler struct {
	db         database.Database
	ticker     *time.Ticker
	stopChan   chan struct{}
	mu         sync.RWMutex
	running    bool
	smtpSender SMTPSender
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
		ticker:     time.NewTicker(5 * time.Minute),
		stopChan:   make(chan struct{}),
		running:    false,
		smtpSender: smtpSender,
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
		if err := ns.sendNotificationBatch(batch); err != nil {
			slog.Error("Failed to send notification batch", slog.String("component", "scheduler"), slog.String("user_email", userEmail), slog.Any("error", err))
			continue
		}

		// Mark all notifications in this batch as read
		if err := ns.markNotificationsAsRead(batch.NotificationIDs); err != nil {
			slog.Error("Failed to mark notifications as read", slog.String("component", "scheduler"), slog.String("user_email", userEmail), slog.Any("error", err))
		}
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

// getUnreadNotificationsByUser gets all unread notifications that haven't been sent yet, grouped by user
func (ns *NotificationScheduler) getUnreadNotificationsByUser() (map[string]*UserNotificationBatch, error) {
	query := `
		SELECT n.id, n.user_id, n.title, n.message, n.type, n.timestamp, n.read,
		       n.sent_at, n.avatar, n.action_url, n.metadata, n.created_at, n.updated_at,
		       u.email, u.first_name, u.last_name
		FROM notifications n
		JOIN users u ON n.user_id = u.id
		WHERE n.sent_at IS NULL
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

		// Add notification to batch
		batch.Notifications = append(batch.Notifications, n)
		batch.NotificationIDs = append(batch.NotificationIDs, n.ID)
	}

	return userBatches, rows.Err()
}

// sendNotificationBatch sends a batch of notifications to a user
func (ns *NotificationScheduler) sendNotificationBatch(batch *UserNotificationBatch) error {
	if len(batch.Notifications) == 0 {
		return nil
	}

	return ns.smtpSender.SendBatchedNotifications(batch.UserEmail, batch.UserName, batch.Notifications)
}

// markNotificationsAsRead marks notifications as read and sent by their IDs
func (ns *NotificationScheduler) markNotificationsAsRead(notificationIDs []int) error {
	if len(notificationIDs) == 0 {
		return nil
	}

	// Build placeholders for the IN clause
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