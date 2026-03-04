package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"strings"

	"windshift/internal/database"
	"windshift/internal/smtp"
)

// EmailReplyService sends threaded SMTP replies to portal customers
// when internal users add comments to email-originated items.
type EmailReplyService struct {
	db         database.Database
	smtpSender ThreadedEmailSender
}

// NewEmailReplyService creates a new EmailReplyService.
func NewEmailReplyService(db database.Database, smtpSender ThreadedEmailSender) *EmailReplyService {
	return &EmailReplyService{
		db:         db,
		smtpSender: smtpSender,
	}
}

// HandleCommentCreated checks if an outbound email should be sent for a new comment.
// It sends a threaded email to the portal customer if:
// - The comment is not private
// - The comment is from an internal user (not from a portal customer)
// - The item was created via an email channel by a portal customer
func (s *EmailReplyService) HandleCommentCreated(params HandleCommentParams) error {
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

	// Check SMTP is configured
	if !s.smtpSender.IsSMTPConfigured() {
		return nil
	}

	// Query item: channel_id, creator_portal_customer_id, workspace key, item number, title
	var channelID sql.NullInt64
	var creatorPortalCustomerID sql.NullInt64
	var workspaceKey string
	var itemNumber int
	var itemTitle string

	err := s.db.QueryRow(`
		SELECT i.channel_id, i.creator_portal_customer_id, w.key, i.workspace_item_number, i.title
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		WHERE i.id = ?
	`, params.ItemID).Scan(&channelID, &creatorPortalCustomerID, &workspaceKey, &itemNumber, &itemTitle)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return fmt.Errorf("failed to query item for email reply: %w", err)
	}

	// Skip if item has no channel or no portal customer creator
	if !channelID.Valid || !creatorPortalCustomerID.Valid {
		return nil
	}

	// Verify channel is email type
	var channelType string
	err = s.db.QueryRow("SELECT type FROM channels WHERE id = ? AND type = 'email'", channelID.Int64).Scan(&channelType)
	if err != nil {
		// Not an email channel or doesn't exist — skip
		return nil
	}

	// Look up portal customer email
	var customerEmail, customerName string
	err = s.db.QueryRow("SELECT email, name FROM portal_customers WHERE id = ?", creatorPortalCustomerID.Int64).Scan(&customerEmail, &customerName)
	if err != nil || customerEmail == "" {
		slog.Debug("no email for portal customer, skipping reply",
			slog.String("component", "email_reply_service"),
			slog.Int64("customer_id", creatorPortalCustomerID.Int64),
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
	originalSubject := itemTitle
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
	var authorName string
	if err := s.db.QueryRow("SELECT first_name || ' ' || last_name FROM users WHERE id = ?", params.AuthorID).Scan(&authorName); err != nil {
		slog.Warn("failed to look up author full name", slog.Any("error", err), slog.Int("author_id", params.AuthorID))
	}
	if authorName == "" {
		if err := s.db.QueryRow("SELECT username FROM users WHERE id = ?", params.AuthorID).Scan(&authorName); err != nil {
			slog.Warn("failed to look up author username", slog.Any("error", err), slog.Int("author_id", params.AuthorID))
		}
	}
	if authorName == "" {
		authorName = "Team member"
	}

	// Build email body
	itemKey := fmt.Sprintf("%s-%d", workspaceKey, itemNumber)
	htmlBody := s.buildHTMLBody(authorName, itemKey, itemTitle, params.Content)
	textBody := s.buildTextBody(authorName, itemKey, itemTitle, params.Content)

	// Send via SMTP
	err = s.smtpSender.SendThreadedEmail(smtp.ThreadedEmailParams{
		ToEmail:    customerEmail,
		ToName:     customerName,
		Subject:    subject,
		HTMLBody:   htmlBody,
		TextBody:   textBody,
		MessageID:  messageID,
		InReplyTo:  inReplyTo,
		References: references,
	})
	if err != nil {
		return fmt.Errorf("failed to send threaded email: %w", err)
	}

	slog.Info("sent threaded email reply to customer",
		slog.String("component", "email_reply_service"),
		slog.Int("comment_id", params.CommentID),
		slog.Int("item_id", params.ItemID),
		slog.String("to", customerEmail),
	)

	// Record outbound message in tracking
	_, err = s.db.Exec(`
		INSERT INTO email_message_tracking (
			channel_id, message_id, in_reply_to, from_email, from_name, subject,
			item_id, comment_id, direction, processed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'outbound', CURRENT_TIMESTAMP)
	`,
		channelID.Int64,
		messageID,
		inReplyTo,
		s.getSMTPFromEmail(),
		authorName,
		subject,
		params.ItemID,
		params.CommentID,
	)
	if err != nil {
		slog.Warn("failed to record outbound email in tracking",
			slog.String("component", "email_reply_service"),
			slog.Int("comment_id", params.CommentID),
			slog.Any("error", err),
		)
	}

	return nil
}

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
		SELECT config FROM channels
		WHERE type = 'smtp' AND direction = 'outbound' AND status = 'enabled'
		ORDER BY updated_at DESC, is_default DESC
		LIMIT 1
	`).Scan(&configJSON)
	if err != nil {
		return "noreply@windshift.local"
	}

	// Simple extraction — look for smtp_from_email in JSON
	// We avoid importing models here to keep it simple
	type smtpConfig struct {
		SMTPFromEmail string `json:"smtp_from_email"`
	}
	var cfg smtpConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil || cfg.SMTPFromEmail == "" {
		return "noreply@windshift.local"
	}
	return cfg.SMTPFromEmail
}

// buildHTMLBody builds the HTML email body for a comment reply.
func (s *EmailReplyService) buildHTMLBody(authorName, itemKey, itemTitle, content string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; color: #333; max-width: 600px;">
<p><strong>%s</strong> replied on %s: %s</p>
<hr style="border: none; border-top: 1px solid #e5e7eb; margin: 16px 0;">
<div style="white-space: pre-wrap;">%s</div>
<hr style="border: none; border-top: 1px solid #e5e7eb; margin: 16px 0;">
<p style="color: #6b7280; font-size: 13px;">To reply, respond directly to this email.</p>
</body>
</html>`,
		html.EscapeString(authorName),
		html.EscapeString(itemKey),
		html.EscapeString(itemTitle),
		html.EscapeString(content),
	)
}

// buildTextBody builds the plain text email body for a comment reply.
func (s *EmailReplyService) buildTextBody(authorName, itemKey, itemTitle, content string) string {
	return fmt.Sprintf(`%s replied on %s: %s
─────────────────────────────
%s
─────────────────────────────
To reply, respond directly to this email.`,
		authorName, itemKey, itemTitle, content,
	)
}
