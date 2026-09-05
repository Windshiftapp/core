package services

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"windshift/internal/database"
	"windshift/internal/emailutil"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/smtp"
)

// EmailReplyService sends threaded SMTP replies to portal customers
// when internal users add comments to email-originated items.
type EmailReplyService struct {
	db         database.Database
	smtpSender ThreadedEmailSender
	idResolver *IDResolverService
	outboxMu   sync.Mutex
	canonical  bool
}

// NewEmailReplyService creates a new EmailReplyService.
func NewEmailReplyService(db database.Database, smtpSender ThreadedEmailSender) *EmailReplyService {
	return &EmailReplyService{
		db:         db,
		smtpSender: smtpSender,
		idResolver: NewIDResolverService(db),
	}
}

// HandleCommentCreated checks if an outbound email should be sent for a new comment.
// It sends a threaded email to the portal customer if:
// - The comment is not private
// - The comment is from an internal user (not from a portal customer)
// - The item was created via an email channel by a portal customer
func (s *EmailReplyService) HandleCommentCreated(params HandleCommentParams) error {
	if s.canonical {
		return nil
	}
	return s.handleCommentCreated(params, true)
}

func (s *EmailReplyService) handleCommentCreated(params HandleCommentParams, deliverImmediately bool) error {
	// Guard: skip private comments
	if params.IsPrivate {
		return nil
	}

	// Guard: skip if comment is FROM a portal customer (don't echo back)
	if params.PortalCustomerID != nil {
		return nil
	}

	// Guard: skip if no identified author
	if params.AuthorID == 0 {
		return nil
	}

	// Query item: channel, portal customer creator, workspace key, item number, title
	item, err := repository.NewItemRepository(s.db).FindByIDWithDetails(params.ItemID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("failed to query item for email reply: %w", err)
	}

	// Skip if item has no channel or no portal customer creator
	if item.ChannelID == nil || item.CreatorPortalCustomerID == nil {
		return nil
	}

	// Verify channel is email type
	var channelType string
	err = s.db.QueryRow("SELECT type FROM channels WHERE id = ? AND type = 'email'", *item.ChannelID).Scan(&channelType)
	if err != nil {
		// Not an email channel or doesn't exist — skip
		return nil
	}

	// Look up portal customer email
	var customerEmail, customerName string
	err = s.db.QueryRow("SELECT email, name FROM portal_customers WHERE id = ?", *item.CreatorPortalCustomerID).Scan(&customerEmail, &customerName)
	if err != nil || customerEmail == "" {
		slog.Debug("no email for portal customer, skipping reply",
			slog.String("component", "email_reply_service"),
			slog.Int("customer_id", *item.CreatorPortalCustomerID),
		)
		return nil
	}

	// Build threading headers from email_message_tracking
	type trackingRecord struct {
		MessageID string
		Subject   sql.NullString
	}
	rows, err := s.db.Query(`
		SELECT message_id, subject FROM email_message_tracking
		WHERE item_id = ? AND channel_id = ?
		ORDER BY processed_at ASC
	`, params.ItemID, *item.ChannelID)
	if err != nil {
		return fmt.Errorf("failed to query email tracking: %w", err)
	}
	defer rows.Close()

	var records []trackingRecord
	for rows.Next() {
		var rec trackingRecord
		if err = rows.Scan(&rec.MessageID, &rec.Subject); err != nil {
			continue
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to iterate email tracking: %w", err)
	}

	if len(records) == 0 {
		// No email tracking records — can't thread, skip
		slog.Debug("no email tracking records for item, skipping reply",
			slog.String("component", "email_reply_service"),
			slog.Int("item_id", params.ItemID),
		)
		return nil
	}

	// References: all Message-IDs chronologically
	var references []string
	for _, rec := range records {
		if rec.MessageID != "" {
			references = append(references, rec.MessageID)
		}
	}

	// In-Reply-To: most recent Message-ID
	inReplyTo := records[len(records)-1].MessageID

	// Subject: Re: {original subject} from first tracking record
	originalSubject := item.Title
	if records[0].Subject.Valid && records[0].Subject.String != "" {
		originalSubject = records[0].Subject.String
	}
	subject := originalSubject
	if !strings.HasPrefix(strings.ToLower(subject), "re:") {
		subject = "Re: " + subject
	}

	// Get SMTP domain for Message-ID generation
	smtpDomain := s.getSMTPDomain()

	// Generate Message-ID for this outbound email
	messageID := fmt.Sprintf("<ws-comment-%d@%s>", params.CommentID, smtpDomain)

	// Get author name for email template
	authorName := s.idResolver.ResolveUserName(params.AuthorID)
	if authorName == "" {
		authorName = "Team member"
	}

	// Build email body via the shared template pipeline. We pre-compute the
	// threaded subject above (Re: …) and pass it through as OriginalSubject;
	// the rendered subject from RenderEmail is discarded so the threading
	// stays correct.
	itemKey := fmt.Sprintf("%s-%d", item.WorkspaceKey, item.WorkspaceItemNumber)
	_, htmlBody, textBody, err := s.smtpSender.RenderEmail(emailutil.TemplatePortalReply, struct {
		AuthorName      string
		ItemKey         string
		ItemTitle       string
		Content         string
		OriginalSubject string
	}{
		AuthorName:      authorName,
		ItemKey:         itemKey,
		ItemTitle:       item.Title,
		Content:         params.Content,
		OriginalSubject: subject,
	})
	if err != nil {
		return fmt.Errorf("failed to render portal reply email: %w", err)
	}

	referencesJSON, err := json.Marshal(references)
	if err != nil {
		return fmt.Errorf("encode email references: %w", err)
	}
	_, err = s.db.ExecWrite(`
		INSERT INTO email_reply_outbox (
			comment_id, channel_id, item_id, to_email, to_name, subject,
			html_body, text_body, message_id, in_reply_to, references_json,
			from_email, from_name, next_attempt_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(comment_id) DO NOTHING
	`, params.CommentID, *item.ChannelID, params.ItemID, customerEmail, customerName,
		subject, htmlBody, textBody, messageID, inReplyTo, string(referencesJSON),
		s.getSMTPFromEmail(), authorName)
	if err != nil {
		return fmt.Errorf("enqueue threaded email reply: %w", err)
	}

	// Sending immediately keeps the current low-latency behavior. The durable
	// row remains pending on failure and is retried by NotificationScheduler.
	if !deliverImmediately || !s.smtpSender.IsSMTPConfigured() {
		return nil
	}
	s.outboxMu.Lock()
	defer s.outboxMu.Unlock()
	if _, err := s.deliverPendingReply(params.CommentID); err != nil {
		return fmt.Errorf("threaded email queued for retry: %w", err)
	}
	return nil
}

type emailReplyOutboxRow struct {
	CommentID      int
	ChannelID      int
	ItemID         int
	ToEmail        string
	ToName         string
	Subject        string
	HTMLBody       string
	TextBody       string
	MessageID      string
	InReplyTo      string
	ReferencesJSON string
	FromEmail      string
	FromName       string
	AttemptCount   int
}

// ProcessPendingReplies retries a bounded batch from the durable reply outbox.
// It is called by NotificationScheduler on the existing SMTP cadence.
func (s *EmailReplyService) ProcessPendingReplies(limit int) (int, error) {
	if limit <= 0 {
		limit = 50
	}
	if !s.smtpSender.IsSMTPConfigured() {
		return 0, nil
	}

	s.outboxMu.Lock()
	defer s.outboxMu.Unlock()

	rows, err := s.db.Query(`
		SELECT comment_id
		FROM email_reply_outbox
		WHERE delivered_at IS NULL AND next_attempt_at <= CURRENT_TIMESTAMP
		ORDER BY created_at ASC
		LIMIT ?
	`, limit)
	if err != nil {
		return 0, fmt.Errorf("query email reply outbox: %w", err)
	}
	var commentIDs []int
	for rows.Next() {
		var commentID int
		if err := rows.Scan(&commentID); err != nil {
			_ = rows.Close()
			return 0, fmt.Errorf("scan email reply outbox: %w", err)
		}
		commentIDs = append(commentIDs, commentID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, fmt.Errorf("iterate email reply outbox: %w", err)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("close email reply outbox rows: %w", err)
	}

	delivered := 0
	var lastErr error
	for _, commentID := range commentIDs {
		sent, err := s.deliverPendingReply(commentID)
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
	if _, err := s.db.ExecWrite(`DELETE FROM email_reply_outbox WHERE delivered_at IS NOT NULL AND delivered_at < ?`, cutoff); err != nil {
		slog.Warn("failed to prune delivered email reply outbox rows", "error", err)
	}
	return delivered, lastErr
}

func (s *EmailReplyService) deliverPendingReply(commentID int) (bool, error) {
	var row emailReplyOutboxRow
	// Atomically lease the row before crossing the SMTP boundary. The process
	// mutex prevents duplicates within one server; this conditional UPDATE also
	// prevents two application instances from selecting and sending the same
	// pending reply. A crashed worker releases itself when the lease expires.
	leaseUntil := time.Now().Add(5 * time.Minute)
	err := s.db.QueryRow(`
		UPDATE email_reply_outbox
		SET next_attempt_at = ?, updated_at = CURRENT_TIMESTAMP
		WHERE comment_id = ? AND delivered_at IS NULL
		  AND next_attempt_at <= CURRENT_TIMESTAMP
		RETURNING comment_id, channel_id, item_id, to_email, to_name, subject,
		       html_body, text_body, message_id, in_reply_to, references_json,
		       from_email, from_name, attempt_count
	`, leaseUntil, commentID).Scan(
		&row.CommentID, &row.ChannelID, &row.ItemID, &row.ToEmail, &row.ToName,
		&row.Subject, &row.HTMLBody, &row.TextBody, &row.MessageID, &row.InReplyTo,
		&row.ReferencesJSON, &row.FromEmail, &row.FromName, &row.AttemptCount,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim pending email reply: %w", err)
	}

	var references []string
	if err := json.Unmarshal([]byte(row.ReferencesJSON), &references); err != nil {
		s.recordReplyFailure(row.CommentID, row.AttemptCount, err)
		return false, fmt.Errorf("decode pending email references: %w", err)
	}
	err = s.smtpSender.SendThreadedEmail(smtp.ThreadedEmailParams{
		ToEmail:    row.ToEmail,
		ToName:     row.ToName,
		Subject:    row.Subject,
		HTMLBody:   row.HTMLBody,
		TextBody:   row.TextBody,
		MessageID:  row.MessageID,
		InReplyTo:  row.InReplyTo,
		References: references,
	})
	if err != nil {
		s.recordReplyFailure(row.CommentID, row.AttemptCount, err)
		return false, fmt.Errorf("send threaded email: %w", err)
	}

	if _, err := s.db.ExecWrite(`
		UPDATE email_reply_outbox
		SET delivered_at = CURRENT_TIMESTAMP, last_error = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE comment_id = ? AND delivered_at IS NULL
	`, row.CommentID); err != nil {
		return false, fmt.Errorf("mark threaded email delivered: %w", err)
	}

	// Tracking is best-effort after delivery. Do not retry SMTP merely because
	// this audit/threading insert failed; that would duplicate customer mail.
	if _, err := s.db.ExecWrite(`
		INSERT INTO email_message_tracking (
			channel_id, message_id, dedup_key, in_reply_to, from_email, from_name, subject,
			item_id, comment_id, direction, processed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'outbound', CURRENT_TIMESTAMP)
		ON CONFLICT(channel_id, dedup_key) DO NOTHING
	`, row.ChannelID, row.MessageID, row.MessageID, row.InReplyTo, row.FromEmail,
		row.FromName, row.Subject, row.ItemID, row.CommentID); err != nil {
		slog.Warn("failed to record delivered outbound email in tracking",
			"comment_id", row.CommentID, "error", err)
	}

	slog.Info("sent threaded email reply to customer",
		"component", "email_reply_service",
		"comment_id", row.CommentID,
		"item_id", row.ItemID,
		"to", row.ToEmail,
	)
	return true, nil
}

func (s *EmailReplyService) recordReplyFailure(commentID, previousAttempts int, sendErr error) {
	shift := min(previousAttempts, 6)
	nextAttempt := time.Now().Add(time.Minute * time.Duration(1<<shift))
	if _, err := s.db.ExecWrite(`
		UPDATE email_reply_outbox
		SET attempt_count = attempt_count + 1, next_attempt_at = ?,
		    last_error = ?, updated_at = CURRENT_TIMESTAMP
		WHERE comment_id = ? AND delivered_at IS NULL
	`, nextAttempt, sendErr.Error(), commentID); err != nil {
		slog.Error("failed to record email reply delivery failure", "comment_id", commentID, "error", err)
	}
}

// fallbackSMTPFromEmail is used when no default outbound SMTP channel is
// configured or its config can't be read.
const fallbackSMTPFromEmail = "noreply@windshift.local"

// getSMTPDomain extracts the domain from the SMTP from email.
func (s *EmailReplyService) getSMTPDomain() string {
	fromEmail := s.getSMTPFromEmail()
	if idx := strings.LastIndex(fromEmail, "@"); idx >= 0 {
		return fromEmail[idx+1:]
	}
	return "windshift.local"
}

// getSMTPFromEmail gets the configured SMTP from email address.
func (s *EmailReplyService) getSMTPFromEmail() string {
	var configJSON string
	err := s.db.QueryRow(`
		SELECT COALESCE(config, '{}') FROM channels
		WHERE type = 'smtp' AND direction = 'outbound'
		  AND status = 'enabled' AND is_default = true
		ORDER BY updated_at DESC
		LIMIT 1
	`).Scan(&configJSON)
	if err != nil {
		return fallbackSMTPFromEmail
	}

	var cfg models.ChannelConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil || cfg.SMTPFromEmail == "" {
		return fallbackSMTPFromEmail
	}
	return cfg.SMTPFromEmail
}
