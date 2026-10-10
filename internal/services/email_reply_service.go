package services

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
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
	// leaseOwner identifies this process's delivery claims so a manual retry
	// can tell a live claim from retry backoff (WI-1572).
	leaseOwner string
}

// NewEmailReplyService creates a new EmailReplyService.
func NewEmailReplyService(db database.Database, smtpSender ThreadedEmailSender) *EmailReplyService {
	hostname, _ := os.Hostname()
	return &EmailReplyService{
		db:         db,
		smtpSender: smtpSender,
		idResolver: NewIDResolverService(db),
		leaseOwner: fmt.Sprintf("%s-%d", hostname, os.Getpid()),
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

	// Any existing channel works as long as the conversation is threadable:
	// email-originated tickets thread from the customer's real messages,
	// portal/form tickets thread from their synthetic anchor.
	var channelType string
	err = s.db.QueryRow("SELECT type FROM channels WHERE id = ?", *item.ChannelID).Scan(&channelType)
	if err != nil {
		// Channel gone — nothing to thread from, skip.
		return nil
	}

	// Portal/form-originated tickets thread from the anchor minted at
	// submission; the ensure covers pre-upgrade tickets (created before the
	// anchor existed) and repairs a failed best-effort mint. It is a no-op for
	// email channels and for items that already have their anchor.
	maybeRecordPortalThreadAnchor(s.db, int64(params.ItemID), item.ChannelID, item.CreatorPortalCustomerID, item.Title)

	// Recipients: the creator plus every external request participant
	// (WI-1136). Each gets its own outbox row so replies attribute per
	// recipient, and the intake participant guard accepts them as repliers.
	var customerEmail, customerName string
	err = s.db.QueryRow("SELECT email, name FROM portal_customers WHERE id = ?", *item.CreatorPortalCustomerID).Scan(&customerEmail, &customerName)
	if err != nil || customerEmail == "" {
		slog.Debug("no email for portal customer, skipping reply",
			slog.String("component", "email_reply_service"),
			slog.Int("customer_id", *item.CreatorPortalCustomerID),
		)
		return nil
	}
	recipients := []emailRecipient{{Email: customerEmail, Name: customerName}}
	participants, err := repository.NewItemParticipantRepository(s.db).ListByItem(params.ItemID)
	if err != nil {
		return fmt.Errorf("load item participants for email reply: %w", err)
	}
	for _, participant := range participants {
		if participant.CustomerEmail == "" || strings.EqualFold(participant.CustomerEmail, customerEmail) {
			continue
		}
		recipients = append(recipients, emailRecipient{Email: participant.CustomerEmail, Name: participant.CustomerName})
	}

	// Build threading headers from email_message_tracking
	type trackingRecord struct {
		MessageID string
		Subject   sql.NullString
	}
	rows, err := s.db.Query(`
		SELECT message_id, subject FROM email_message_tracking
		WHERE item_id = ?
		ORDER BY processed_at ASC
	`, params.ItemID)
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

	// References: all Message-IDs chronologically (item-scoped — the thread
	// follows the conversation across channels).
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
	// Route replies back to the monitored intake mailbox, not the SMTP sender.
	// Persisted per row so a retry is stable if the channel config changes.
	replyToEmail, replyToName := s.resolveReplyToAddress(params.ItemID, *item.ChannelID)
	outboxIDs := make([]int, 0, len(recipients))
	for _, recipient := range recipients {
		messageID := recipientMessageID(params.CommentID, recipient.Email, smtpDomain)
		var outboxID int
		err = s.db.QueryRow(`
			INSERT INTO email_reply_outbox (
				comment_id, channel_id, item_id, to_email, to_name, subject,
				html_body, text_body, message_id, in_reply_to, references_json,
				from_email, from_name, reply_to_email, reply_to_name,
				next_attempt_at, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			ON CONFLICT(comment_id, to_email) DO NOTHING
			RETURNING id
		`, params.CommentID, *item.ChannelID, params.ItemID, recipient.Email, recipient.Name,
			subject, htmlBody, textBody, messageID, inReplyTo, string(referencesJSON),
			s.getSMTPFromEmail(), authorName, replyToEmail, replyToName).Scan(&outboxID)
		if errors.Is(err, sql.ErrNoRows) {
			continue // already enqueued for this recipient
		}
		if err != nil {
			return fmt.Errorf("enqueue threaded email reply: %w", err)
		}
		outboxIDs = append(outboxIDs, outboxID)
	}

	// Sending immediately keeps the current low-latency behavior. The durable
	// rows remain pending on failure and are retried by NotificationScheduler.
	if !deliverImmediately || !s.smtpSender.IsSMTPConfigured() {
		return nil
	}
	s.outboxMu.Lock()
	defer s.outboxMu.Unlock()
	for _, outboxID := range outboxIDs {
		if _, err := s.deliverPendingReply(context.Background(), outboxID); err != nil {
			return fmt.Errorf("threaded email queued for retry: %w", err)
		}
	}
	return nil
}

// resolveReplyToAddress returns the monitored inbound address that customer
// replies should route back to for an item on the given channel. Empty when no
// monitored mailbox applies; the transport then omits Reply-To and falls back
// to From.
//
// The item's own intake is authoritative: the mailbox that fed it owns the
// reply address, so a portal with several intakes never answers from an
// unrelated one. A portal web submission has no feeding intake, so it falls
// back to the oldest enabled intake exposing requests to that portal.
func (s *EmailReplyService) resolveReplyToAddress(itemID, channelID int) (address, name string) {
	var intakeMailboxID sql.NullInt64
	if err := s.db.QueryRow(`
		SELECT i.mailbox_id
		FROM items it
		JOIN intakes i ON i.id = it.intake_id
		WHERE it.id = ?
	`, itemID).Scan(&intakeMailboxID); err == nil && intakeMailboxID.Valid {
		if address, name = s.mailboxAddress(int(intakeMailboxID.Int64)); address != "" {
			return address, name
		}
	}
	// Email-originated items also carry the mailbox as their own channel.
	if address, name = s.mailboxAddress(channelID); address != "" {
		return address, name
	}
	// Portal-originated items carry a portal channel. Route replies back to the
	// mailbox whose enabled intake feeds that portal, preferring the oldest one
	// so the choice is stable across sends. Disabled mailboxes are skipped, and
	// a mailbox with no readable address falls through to the next candidate.
	rows, err := s.db.Query(`
		SELECT i.mailbox_id
		FROM intakes i
		JOIN channels c ON c.id = i.mailbox_id
		WHERE i.portal_channel_id = ?
		  AND i.status = ? AND c.status = 'enabled'
		ORDER BY i.id
	`, channelID, models.IntakeStatusEnabled)
	if err != nil {
		return "", ""
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var mailboxID int
		if err := rows.Scan(&mailboxID); err != nil {
			return "", ""
		}
		if address, name = s.mailboxAddress(mailboxID); address != "" {
			return address, name
		}
	}
	if err := rows.Err(); err != nil {
		return "", ""
	}
	return "", ""
}

// mailboxAddress resolves the monitored address of an email/imap channel.
func (s *EmailReplyService) mailboxAddress(channelID int) (address, name string) {
	var configJSON string
	if err := s.db.QueryRow(
		`SELECT COALESCE(config, '{}') FROM channels WHERE id = ? AND type IN ('email', 'imap')`,
		channelID,
	).Scan(&configJSON); err != nil {
		return "", ""
	}
	var cfg models.ChannelConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return "", ""
	}
	address = strings.TrimSpace(cfg.EmailOAuthEmail)
	if address == "" {
		address = strings.TrimSpace(cfg.IMAPUsername)
	}
	if address == "" {
		return "", ""
	}
	_ = s.db.QueryRow(`SELECT name FROM channels WHERE id = ?`, channelID).Scan(&name)
	return address, name
}

// emailRecipient is one outbound reply target.
type emailRecipient struct {
	Email string
	Name  string
}

// recipientMessageID derives a deterministic, per-recipient Message-ID so each
// outbox row and tracking entry is distinct and idempotent across retries.
func recipientMessageID(commentID int, email, domain string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return fmt.Sprintf("<ws-comment-%d-%s@%s>", commentID, hex.EncodeToString(sum[:6]), domain)
}

type emailReplyOutboxRow struct {
	ID             int
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
	ReplyToEmail   string
	ReplyToName    string
	AttemptCount   int
}

// SendAutomationNotice implements CustomerNotifier: it emails the portal
// customer who created the item through the same threaded transport as reply
// notifications. Automation notices send directly rather than through the
// comment-bound reply outbox; action run history is the durable record.
// Skips (no customer, no thread, SMTP unset) are reported, not errors.
func (s *EmailReplyService) SendAutomationNotice(itemID, actorID int, subject, message string) (delivered bool, skipReason string, err error) {
	item, skipReason, err := s.loadNoticeItem(itemID)
	if err != nil || item == nil {
		return false, skipReason, err
	}
	if item.CreatorPortalCustomerID == nil {
		return false, "no_customer", nil
	}
	customerEmail, customerName, ok := s.noticeCustomer(*item.CreatorPortalCustomerID)
	if !ok {
		return false, "no_customer", nil
	}
	return s.sendItemNotice(item, customerEmail, customerName, subject, message, actorID)
}

// noticeCustomer resolves a portal customer's email and name for a notice.
// ok is false when the row is missing; that is an expected skip, not an error.
func (s *EmailReplyService) noticeCustomer(customerID int) (email, name string, ok bool) {
	if err := s.db.QueryRow("SELECT email, name FROM portal_customers WHERE id = ?", customerID).Scan(&email, &name); err != nil {
		return "", "", false
	}
	return email, name, true
}

// SendParticipantAddedNotice emails a newly added external request participant
// through the item's threaded transport (WI-1136). Participants receive
// customer-facing content only. Expected skips (no SMTP, no channel, no
// thread, no email) are reported, not errors.
func (s *EmailReplyService) SendParticipantAddedNotice(itemID, customerID, actorID int) (delivered bool, skipReason string, err error) {
	item, skipReason, err := s.loadNoticeItem(itemID)
	if err != nil || item == nil {
		return false, skipReason, err
	}
	customerEmail, customerName, ok := s.noticeCustomer(customerID)
	if !ok {
		return false, "no_customer", nil
	}
	message := fmt.Sprintf("You have been added as a participant to %s.", item.Title)
	return s.sendItemNotice(item, customerEmail, customerName, "", message, actorID)
}

// loadNoticeItem loads an item for a customer-facing notice and ensures its
// portal thread anchor exists. Returns (nil, reason, nil) for expected skips.
func (s *EmailReplyService) loadNoticeItem(itemID int) (*models.Item, string, error) {
	if !s.smtpSender.IsSMTPConfigured() {
		return nil, "smtp_not_configured", nil
	}
	item, err := repository.NewItemRepository(s.db).FindByIDWithDetails(itemID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, "item_missing", nil
		}
		return nil, "", fmt.Errorf("load item for customer notice: %w", err)
	}
	if item.ChannelID == nil {
		return nil, "no_channel", nil
	}
	// Same anchor ensure as the reply path: notices must thread for
	// portal-originated tickets (and survive pre-upgrade tickets) too.
	maybeRecordPortalThreadAnchor(s.db, int64(itemID), item.ChannelID, item.CreatorPortalCustomerID, item.Title)
	return item, "", nil
}

// sendItemNotice renders and sends one threaded customer-facing notice to a
// specific recipient on the item's conversation.
func (s *EmailReplyService) sendItemNotice(item *models.Item, toEmail, toName, subject, message string, actorID int) (delivered bool, skipReason string, err error) {
	if toEmail == "" {
		return false, "no_email", nil
	}

	// Thread against the item's recorded Message-IDs (item-scoped); without
	// a thread there is nothing to reply into, so the notice would start an
	// orphaned thread.
	var inReplyTo string
	var references []string
	var originalSubject sql.NullString
	rows, err := s.db.Query(`
		SELECT message_id, subject FROM email_message_tracking
		WHERE item_id = ?
		ORDER BY processed_at ASC
	`, item.ID)
	if err != nil {
		return false, "", fmt.Errorf("query email tracking for customer notice: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var messageID string
		if scanErr := rows.Scan(&messageID, &originalSubject); scanErr != nil {
			continue
		}
		references = append(references, messageID)
		inReplyTo = messageID
	}
	if err := rows.Err(); err != nil {
		return false, "", fmt.Errorf("iterate email tracking for customer notice: %w", err)
	}
	if len(references) == 0 {
		return false, "no_thread", nil
	}

	emailSubject := item.Title
	if originalSubject.Valid && originalSubject.String != "" {
		emailSubject = originalSubject.String
	}
	if subject != "" {
		emailSubject = subject
	}
	if !strings.HasPrefix(strings.ToLower(emailSubject), "re:") {
		emailSubject = "Re: " + emailSubject
	}

	authorName := s.idResolver.ResolveUserName(actorID)
	if authorName == "" {
		authorName = "Support Team"
	}
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
		Content:         message,
		OriginalSubject: emailSubject,
	})
	if err != nil {
		return false, "", fmt.Errorf("render customer notice email: %w", err)
	}

	smtpDomain := s.getSMTPDomain()
	messageID := fmt.Sprintf("<ws-notice-%d-%d@%s>", item.ID, time.Now().UnixNano(), smtpDomain)
	replyToEmail, replyToName := s.resolveReplyToAddress(item.ID, *item.ChannelID)
	if err := s.smtpSender.SendThreadedEmail(smtp.ThreadedEmailParams{
		ToEmail:      toEmail,
		ToName:       toName,
		ReplyToEmail: replyToEmail,
		ReplyToName:  replyToName,
		Subject:      emailSubject,
		HTMLBody:     htmlBody,
		TextBody:     textBody,
		MessageID:    messageID,
		InReplyTo:    inReplyTo,
		References:   references,
	}); err != nil {
		return false, "", fmt.Errorf("send customer notice: %w", err)
	}

	// Record the notice's own Message-ID so a reply that carries only
	// In-Reply-To (no References) still resolves to this item instead of
	// starting a new ticket. Best-effort: the mail is already sent.
	if _, err := s.db.ExecWrite(`
		INSERT INTO email_message_tracking (
			channel_id, message_id, dedup_key, in_reply_to, from_email, from_name, subject,
			item_id, comment_id, direction, processed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, 'outbound', CURRENT_TIMESTAMP)
		ON CONFLICT(channel_id, dedup_key) DO NOTHING
	`, *item.ChannelID, messageID, messageID, inReplyTo, s.getSMTPFromEmail(), authorName, emailSubject, item.ID); err != nil {
		slog.Warn("failed to record customer notice in email tracking; replies may not thread",
			slog.String("component", "email_reply_service"),
			slog.Int("item_id", item.ID),
			slog.String("message_id", messageID),
			slog.Any("error", err),
		)
	}
	return true, "", nil
}

// ProcessPendingReplies retries a bounded batch from the durable reply outbox.
// It is called by NotificationScheduler on the existing SMTP cadence.
func (s *EmailReplyService) ProcessPendingReplies(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 50
	}
	if !s.smtpSender.IsSMTPConfigured() {
		return 0, nil
	}

	s.outboxMu.Lock()
	defer s.outboxMu.Unlock()

	rows, err := s.db.Query(`
		SELECT id
		FROM email_reply_outbox
		WHERE delivered_at IS NULL AND discarded_at IS NULL AND next_attempt_at <= CURRENT_TIMESTAMP
		ORDER BY created_at ASC
		LIMIT ?
	`, limit)
	if err != nil {
		return 0, fmt.Errorf("query email reply outbox: %w", err)
	}
	var outboxIDs []int
	for rows.Next() {
		var outboxID int
		if err := rows.Scan(&outboxID); err != nil {
			_ = rows.Close()
			return 0, fmt.Errorf("scan email reply outbox: %w", err)
		}
		outboxIDs = append(outboxIDs, outboxID)
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
	for _, outboxID := range outboxIDs {
		if err := ctx.Err(); err != nil {
			lastErr = err
			break
		}
		sent, err := s.deliverPendingReply(ctx, outboxID)
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

func (s *EmailReplyService) deliverPendingReply(ctx context.Context, outboxID int) (bool, error) {
	var row emailReplyOutboxRow
	// Atomically lease the row before crossing the SMTP boundary. The process
	// mutex prevents duplicates within one server; this conditional UPDATE also
	// prevents two application instances from selecting and sending the same
	// pending reply. A crashed worker releases itself when the lease expires.
	leaseUntil := time.Now().Add(5 * time.Minute)
	err := s.db.QueryRow(`
		UPDATE email_reply_outbox
		SET next_attempt_at = ?, lease_owner = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND delivered_at IS NULL AND discarded_at IS NULL
		  AND next_attempt_at <= CURRENT_TIMESTAMP
		RETURNING id, comment_id, channel_id, item_id, to_email, to_name, subject,
		       html_body, text_body, message_id, in_reply_to, references_json,
		       from_email, from_name, reply_to_email, reply_to_name, attempt_count
	`, leaseUntil, s.leaseOwner, outboxID).Scan(
		&row.ID, &row.CommentID, &row.ChannelID, &row.ItemID, &row.ToEmail, &row.ToName,
		&row.Subject, &row.HTMLBody, &row.TextBody, &row.MessageID, &row.InReplyTo,
		&row.ReferencesJSON, &row.FromEmail, &row.FromName, &row.ReplyToEmail, &row.ReplyToName,
		&row.AttemptCount,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim pending email reply: %w", err)
	}

	var references []string
	if err := json.Unmarshal([]byte(row.ReferencesJSON), &references); err != nil {
		s.recordReplyFailure(row.ID, row.AttemptCount, err)
		return false, fmt.Errorf("decode pending email references: %w", err)
	}
	err = s.smtpSender.SendThreadedEmailContext(ctx, smtp.ThreadedEmailParams{
		ToEmail:      row.ToEmail,
		ToName:       row.ToName,
		ReplyToEmail: row.ReplyToEmail,
		ReplyToName:  row.ReplyToName,
		Subject:      row.Subject,
		HTMLBody:     row.HTMLBody,
		TextBody:     row.TextBody,
		MessageID:    row.MessageID,
		InReplyTo:    row.InReplyTo,
		References:   references,
	})
	if err != nil {
		s.recordReplyFailure(row.ID, row.AttemptCount, err)
		return false, fmt.Errorf("send threaded email: %w", err)
	}

	if _, err := s.db.ExecWrite(`
		UPDATE email_reply_outbox
		SET delivered_at = CURRENT_TIMESTAMP, last_error = NULL, lease_owner = NULL,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND delivered_at IS NULL
	`, row.ID); err != nil {
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

func (s *EmailReplyService) recordReplyFailure(outboxID, previousAttempts int, sendErr error) {
	shift := min(previousAttempts, 6)
	nextAttempt := time.Now().Add(time.Minute * time.Duration(1<<shift))
	if _, err := s.db.ExecWrite(`
		UPDATE email_reply_outbox
		SET attempt_count = attempt_count + 1, next_attempt_at = ?,
		    last_error = ?, lease_owner = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND delivered_at IS NULL
	`, nextAttempt, sendErr.Error(), outboxID); err != nil {
		slog.Error("failed to record email reply delivery failure", "outbox_id", outboxID, "error", err)
	}
}

// getSMTPDomain extracts the domain from the SMTP from email.
func (s *EmailReplyService) getSMTPDomain() string {
	return smtpDomain(s.db)
}

// getSMTPFromEmail gets the configured SMTP from email address.
func (s *EmailReplyService) getSMTPFromEmail() string {
	return smtpFromEmail(s.db)
}

// fallbackSMTPFromEmail is used when no default outbound SMTP channel is
// configured or its config can't be read.
const fallbackSMTPFromEmail = "noreply@windshift.local"

// smtpDomain extracts the domain from the default outbound SMTP from email.
// Shared by the reply service and the portal thread-anchor minter.
func smtpDomain(db database.Database) string {
	fromEmail := smtpFromEmail(db)
	if idx := strings.LastIndex(fromEmail, "@"); idx >= 0 {
		return fromEmail[idx+1:]
	}
	return "windshift.local"
}

// smtpFromEmail reads the configured from email of the default outbound SMTP
// channel, falling back when none is configured or its config can't be read.
func smtpFromEmail(db database.Database) string {
	var configJSON string
	err := db.QueryRow(`
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

// ErrEmailReplyNotRetryable marks an outbox row an operator cannot re-send:
// it is already delivered, explicitly discarded, or belongs to another
// channel. Handlers surface it as a conflict; repository.ErrNotFound stays
// the missing-row signal.
var ErrEmailReplyNotRetryable = errors.New("email reply is not retryable")

// ErrEmailReplyInFlight marks an outbox row whose delivery lease another
// worker still holds; the operator retry must not steal it (WI-1572).
var ErrEmailReplyInFlight = errors.New("email reply is being delivered by another worker")

// RetryPendingReply attempts immediate delivery of one pending outbound
// reply on behalf of an operator. The row's backoff lease is cleared first so
// a stuck schedule (or an earlier failure's next_attempt_at) cannot block the
// explicit retry. A failed send is recorded like any scheduler attempt:
// attempt_count increments, last_error is stored, and the row returns to the
// backoff schedule.
func (s *EmailReplyService) RetryPendingReply(channelID, commentID int) (delivered bool, err error) {
	s.outboxMu.Lock()
	defer s.outboxMu.Unlock()

	// One comment can have one outbox row per recipient (creator +
	// participants, WI-1136). Retry every retryable row and never steal a
	// live delivery lease.
	type outboxState struct {
		id        int
		delivered sql.NullTime
		discarded sql.NullTime
		lease     sql.NullString
		next      time.Time
	}
	rows, err := s.db.Query(`
		SELECT id, delivered_at, discarded_at, lease_owner, next_attempt_at
		FROM email_reply_outbox
		WHERE channel_id = ? AND comment_id = ?
	`, channelID, commentID)
	if err != nil {
		return false, fmt.Errorf("load email reply for retry: %w", err)
	}
	var states []outboxState
	for rows.Next() {
		var state outboxState
		if scanErr := rows.Scan(&state.id, &state.delivered, &state.discarded, &state.lease, &state.next); scanErr != nil {
			_ = rows.Close()
			return false, fmt.Errorf("scan email reply for retry: %w", scanErr)
		}
		states = append(states, state)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return false, fmt.Errorf("iterate email replies for retry: %w", err)
	}
	_ = rows.Close()

	if len(states) == 0 {
		return false, repository.ErrNotFound
	}

	now := time.Now()
	inFlight := false
	retryable := make([]int, 0, len(states))
	for _, state := range states {
		if state.delivered.Valid || state.discarded.Valid {
			continue
		}
		if state.lease.Valid && state.next.After(now) {
			inFlight = true
			continue
		}
		retryable = append(retryable, state.id)
	}
	if len(retryable) == 0 {
		if inFlight {
			return false, ErrEmailReplyInFlight
		}
		return false, ErrEmailReplyNotRetryable
	}

	if !s.smtpSender.IsSMTPConfigured() {
		return false, ErrSMTPNotConfigured
	}

	// Retry scheduling never steals a live claim: the reset only applies
	// while no worker holds the delivery lease. A future next_attempt_at
	// with a NULL owner is retry backoff and retries right now; a future
	// next_attempt_at with an owner is another instance's in-flight send.
	for _, id := range retryable {
		if _, err := s.db.ExecWrite(`
			UPDATE email_reply_outbox
			SET next_attempt_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
			WHERE id = ? AND (lease_owner IS NULL OR next_attempt_at <= CURRENT_TIMESTAMP)
		`, id); err != nil {
			return false, fmt.Errorf("reset email reply backoff for retry: %w", err)
		}
	}

	for _, id := range retryable {
		sent, err := s.deliverPendingReply(context.Background(), id)
		if err != nil {
			return delivered, err
		}
		if sent {
			delivered = true
		}
	}
	return delivered, nil
}
