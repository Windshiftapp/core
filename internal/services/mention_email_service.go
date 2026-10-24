package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"windshift/internal/database"
	"windshift/internal/emailutil"
	"windshift/internal/models"
	"windshift/internal/smtp"
)

// MentionEmailData is the template payload for the dedicated mention email.
// The template renders Content as Markdown and links ActionURL.
type MentionEmailData struct {
	RecipientName string
	ActorName     string
	ItemKey       string
	ItemTitle     string
	SourceLabel   string
	Content       string
	ActionURL     string
}

// MentionEmailParams describes one mention to queue for email. The caller
// (MentionService) has already resolved the item and actor so the enqueue path
// stays a couple of inserts.
type MentionEmailParams struct {
	MentionID      int
	NotificationID int
	UserID         int
	WorkspaceID    int
	ItemID         int
	SourceType     string
	Content        string
	ActorName      string
	ItemKey        string
	ItemTitle      string
	ActionURL      string
}

// MentionNotificationMarker suppresses the in-app notification from the
// generic digest once the dedicated mention email has been delivered, so a
// mention is never emailed twice.
type MentionNotificationMarker interface {
	MarkNotificationsSent(notificationIDs []int) error
}

// MentionEmailService sends @mention recipients a dedicated email carrying the
// message body. It persists each send in mention_email_outbox so a crash or
// SMTP failure retries, and it re-checks workspace view permission immediately
// before every SMTP delivery so a revoked recipient is never notified.
type MentionEmailService struct {
	db         database.Database
	sender     MentionEmailSender
	marker     MentionNotificationMarker
	authorizer *NotificationAuthorizer
	baseURL    string
	leaseOwner string
	outboxMu   sync.Mutex
}

// NewMentionEmailService creates a new MentionEmailService.
func NewMentionEmailService(db database.Database, sender MentionEmailSender, marker MentionNotificationMarker, authorizer *NotificationAuthorizer) *MentionEmailService {
	hostname, _ := os.Hostname()
	return &MentionEmailService{
		db:         db,
		sender:     sender,
		marker:     marker,
		authorizer: authorizer,
		leaseOwner: fmt.Sprintf("%s-%d", hostname, os.Getpid()),
	}
}

// SetBaseURL sets the browser-visible origin used to turn the stored relative
// action path into a clickable link. Called from server startup.
func (s *MentionEmailService) SetBaseURL(baseURL string) {
	s.baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
}

// EnqueueMentionEmail persists a durable mention email for one recipient.
// Unknown, inactive, or agent users are skipped. The email is delivered by
// ProcessPendingMentions on the scheduler cadence, before the digest claims
// unread notifications, so the dedicated email wins the race for the same
// mention.
func (s *MentionEmailService) EnqueueMentionEmail(params MentionEmailParams) error {
	if params.MentionID <= 0 || params.UserID <= 0 || params.ItemID <= 0 {
		return nil
	}
	var toEmail, firstName, lastName string
	var isAgent bool
	err := s.db.QueryRow(`
		SELECT email, first_name, last_name, is_agent
		FROM users
		WHERE id = ? AND is_active = true
	`, params.UserID).Scan(&toEmail, &firstName, &lastName, &isAgent)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("resolve mention email recipient %d: %w", params.UserID, err)
	}
	if isAgent || strings.TrimSpace(toEmail) == "" {
		return nil
	}
	recipientName := strings.TrimSpace(firstName + " " + lastName)
	var notificationID *int
	if params.NotificationID > 0 {
		notificationID = &params.NotificationID
	}

	_, err = s.db.ExecWrite(`
		INSERT INTO mention_email_outbox (
			mention_id, notification_id, user_id, workspace_id, item_id,
			to_email, to_name, actor_name, item_key, item_title, source_type,
			content, action_url, next_attempt_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(mention_id) DO NOTHING
	`, params.MentionID, nullableInt(notificationID), params.UserID, params.WorkspaceID, params.ItemID,
		toEmail, recipientName, params.ActorName, params.ItemKey, params.ItemTitle, params.SourceType,
		params.Content, params.ActionURL)
	if err != nil {
		return fmt.Errorf("enqueue mention email: %w", err)
	}
	return nil
}

type mentionEmailOutboxRow struct {
	ID             int
	MentionID      int
	NotificationID sql.NullInt64
	UserID         int
	WorkspaceID    int
	ItemID         int
	ToEmail        string
	ToName         string
	ActorName      string
	ItemKey        string
	ItemTitle      string
	SourceType     string
	Content        string
	ActionURL      string
	AttemptCount   int
}

// RefreshPendingMentionContent rewrites the body of still-pending mention
// emails for a source after its content was edited. Delivered and discarded
// rows are left alone, so a recipient never receives a stale pre-edit body.
func (s *MentionEmailService) RefreshPendingMentionContent(sourceType string, sourceID int, content string) error {
	_, err := s.db.ExecWrite(`
		UPDATE mention_email_outbox
		SET content = ?, updated_at = CURRENT_TIMESTAMP
		WHERE delivered_at IS NULL AND discarded_at IS NULL
		  AND mention_id IN (SELECT id FROM mentions WHERE source_type = ? AND source_id = ?)
	`, content, sourceType, sourceID)
	if err != nil {
		return fmt.Errorf("refresh pending mention email content: %w", err)
	}
	return nil
}

// ProcessPendingMentions retries a bounded batch from the durable mention
// outbox. It is called by NotificationScheduler before it claims the generic
// digest batches, so a successfully delivered mention email suppresses the
// content-less digest entry.
func (s *MentionEmailService) ProcessPendingMentions(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 50
	}
	if !s.sender.IsSMTPConfigured() {
		return 0, nil
	}

	s.outboxMu.Lock()
	defer s.outboxMu.Unlock()

	rows, err := s.db.Query(`
		SELECT id
		FROM mention_email_outbox
		WHERE delivered_at IS NULL AND discarded_at IS NULL AND next_attempt_at <= CURRENT_TIMESTAMP
		ORDER BY created_at ASC
		LIMIT ?
	`, limit)
	if err != nil {
		return 0, fmt.Errorf("query mention email outbox: %w", err)
	}
	var outboxIDs []int
	for rows.Next() {
		var outboxID int
		if err := rows.Scan(&outboxID); err != nil {
			_ = rows.Close()
			return 0, fmt.Errorf("scan mention email outbox: %w", err)
		}
		outboxIDs = append(outboxIDs, outboxID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, fmt.Errorf("iterate mention email outbox: %w", err)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("close mention email outbox rows: %w", err)
	}

	delivered := 0
	var lastErr error
	for _, outboxID := range outboxIDs {
		if err := ctx.Err(); err != nil {
			lastErr = err
			break
		}
		sent, err := s.deliverPendingMention(ctx, outboxID)
		if err != nil {
			lastErr = err
			continue
		}
		if sent {
			delivered++
		}
	}
	// Delivered rows are retained briefly for idempotence/audit, then pruned.
	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	if _, err := s.db.ExecWrite(`DELETE FROM mention_email_outbox WHERE delivered_at IS NOT NULL AND delivered_at < ?`, cutoff); err != nil {
		slog.Warn("failed to prune delivered mention email outbox rows", "error", err)
	}
	return delivered, lastErr
}

func (s *MentionEmailService) deliverPendingMention(ctx context.Context, outboxID int) (bool, error) {
	var row mentionEmailOutboxRow
	// Atomically lease the row before crossing the SMTP boundary so two
	// scheduler instances cannot both send the same mention.
	leaseUntil := time.Now().Add(5 * time.Minute)
	err := s.db.QueryRow(`
		UPDATE mention_email_outbox
		SET next_attempt_at = ?, lease_owner = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND delivered_at IS NULL AND discarded_at IS NULL
		  AND next_attempt_at <= CURRENT_TIMESTAMP
		RETURNING id, mention_id, notification_id, user_id, workspace_id, item_id,
		       to_email, to_name, actor_name, item_key, item_title, source_type,
		       content, action_url, attempt_count
	`, leaseUntil, s.leaseOwner, outboxID).Scan(
		&row.ID, &row.MentionID, &row.NotificationID, &row.UserID, &row.WorkspaceID, &row.ItemID,
		&row.ToEmail, &row.ToName, &row.ActorName, &row.ItemKey, &row.ItemTitle, &row.SourceType,
		&row.Content, &row.ActionURL, &row.AttemptCount,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim pending mention email: %w", err)
	}

	// Re-authorize immediately before SMTP. The recipient must still be an
	// active user with item.view on the workspace; a definitive denial is
	// permanent, so the row is discarded rather than retried.
	visible, err := s.recipientStillPermitted(row.UserID, row.WorkspaceID, row.ItemID)
	if err != nil {
		s.recordMentionFailure(row.ID, row.AttemptCount, err)
		return false, fmt.Errorf("reauthorize mention email: %w", err)
	}
	if !visible {
		if err := s.discardMention(row.ID, "recipient no longer permitted"); err != nil {
			return false, err
		}
		return false, nil
	}

	// If the recipient already read the in-app notification, or the generic
	// digest already emailed it, the dedicated email would be a duplicate.
	if row.NotificationID.Valid {
		handled, err := s.mentionNotificationHandled(row.NotificationID.Int64)
		if err != nil {
			s.recordMentionFailure(row.ID, row.AttemptCount, err)
			return false, fmt.Errorf("check mention notification state: %w", err)
		}
		if handled {
			if err := s.discardMention(row.ID, "notification already read or emailed"); err != nil {
				return false, err
			}
			return false, nil
		}
	}

	data := MentionEmailData{
		RecipientName: row.ToName,
		ActorName:     row.ActorName,
		ItemKey:       row.ItemKey,
		ItemTitle:     row.ItemTitle,
		SourceLabel:   mentionSourceLabel(row.SourceType),
		Content:       row.Content,
		ActionURL:     smtp.AbsoluteActionURL(s.baseURL, row.ActionURL),
	}
	if err := s.sender.SendTransactionalContext(ctx, row.ToEmail, emailutil.TemplateMention, data); err != nil {
		s.recordMentionFailure(row.ID, row.AttemptCount, err)
		return false, fmt.Errorf("send mention email: %w", err)
	}

	if _, err := s.db.ExecWrite(`
		UPDATE mention_email_outbox
		SET delivered_at = CURRENT_TIMESTAMP, last_error = NULL, lease_owner = NULL,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND delivered_at IS NULL
	`, row.ID); err != nil {
		return false, fmt.Errorf("mark mention email delivered: %w", err)
	}

	// Suppress the content-less digest copy now that the real message is out.
	// Best-effort: the dedicated email is already delivered, and a failure here
	// only risks a duplicate digest entry, not lost mail.
	if row.NotificationID.Valid && s.marker != nil {
		if err := s.marker.MarkNotificationsSent([]int{int(row.NotificationID.Int64)}); err != nil {
			slog.Warn("failed to suppress digest copy after mention email delivery",
				slog.String("component", "mention_email_service"),
				slog.Int("outbox_id", row.ID),
				slog.Int("notification_id", int(row.NotificationID.Int64)),
				slog.Any("error", err))
		}
	}

	slog.Info("sent mention email",
		"component", "mention_email_service",
		"outbox_id", row.ID,
		"user_id", row.UserID,
		"item_id", row.ItemID,
	)
	return true, nil
}

// discardMention marks an outbox row permanently skipped so it is not retried.
func (s *MentionEmailService) discardMention(outboxID int, reason string) error {
	if _, err := s.db.ExecWrite(`
		UPDATE mention_email_outbox
		SET discarded_at = CURRENT_TIMESTAMP, lease_owner = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND delivered_at IS NULL
	`, outboxID); err != nil {
		return fmt.Errorf("discard mention email: %w", err)
	}
	slog.Debug("discarded mention email",
		slog.String("component", "mention_email_service"),
		slog.Int("outbox_id", outboxID),
		slog.String("reason", reason),
	)
	return nil
}

// mentionNotificationHandled reports whether the linked in-app notification is
// already read or already emailed. A missing notification is not handled: the
// mention email is the durable record and still sends.
func (s *MentionEmailService) mentionNotificationHandled(notificationID int64) (bool, error) {
	var read bool
	var sentAt sql.NullTime
	err := s.db.QueryRow(`SELECT read, sent_at FROM notifications WHERE id = ?`, notificationID).Scan(&read, &sentAt)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return read || sentAt.Valid, nil
}

// recipientStillPermitted reports whether the recipient may currently see the
// item: an active user with item.view on the workspace. A definitive denial
// returns (false, nil); an infrastructure error returns a non-nil error so the
// delivery is retried rather than permanently discarded.
func (s *MentionEmailService) recipientStillPermitted(userID, workspaceID, itemID int) (bool, error) {
	if s.authorizer == nil {
		return false, errors.New("mention email authorizer not configured")
	}
	snapshot, err := s.authorizer.Snapshot(userID)
	if err != nil {
		return false, err
	}
	return snapshot.Visible(models.Notification{
		UserID:             userID,
		AuthorizationScope: models.NotificationScopeWorkspace,
		WorkspaceID:        &workspaceID,
		ItemID:             &itemID,
	})
}

func (s *MentionEmailService) recordMentionFailure(outboxID, previousAttempts int, sendErr error) {
	shift := min(previousAttempts, 6)
	nextAttempt := time.Now().Add(time.Minute * time.Duration(1<<shift))
	if _, err := s.db.ExecWrite(`
		UPDATE mention_email_outbox
		SET attempt_count = attempt_count + 1, next_attempt_at = ?,
		    last_error = ?, lease_owner = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND delivered_at IS NULL
	`, nextAttempt, sendErr.Error(), outboxID); err != nil {
		slog.Error("failed to record mention email delivery failure", "outbox_id", outboxID, "error", err)
	}
}

func mentionSourceLabel(sourceType string) string {
	switch sourceType {
	case "comment":
		return "a comment"
	case "item_description":
		return "the description"
	default:
		return "an item"
	}
}
