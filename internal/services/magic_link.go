package services

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"time"

	"windshift/internal/database"
	"windshift/internal/smtp"
)

var (
	ErrMagicLinkExpired           = errors.New("magic link has expired")
	ErrMagicLinkInvalid           = errors.New("magic link is invalid")
	ErrMagicLinkAlreadyUsed       = errors.New("magic link has already been used")
	ErrPortalCustomerNotFound     = errors.New("portal customer not found")
	ErrMagicLinkSMTPNotConfigured = errors.New("SMTP is not configured")
	ErrMagicLinkGenerationFailed  = errors.New("failed to generate magic link token")
)

const (
	// MagicLinkExpiry is the duration for which a magic link token is valid
	MagicLinkExpiry = 15 * time.Minute
	// MagicLinkTokenLength is the length of the random bytes for the token
	MagicLinkTokenLength = 32
)

// MagicLinkService handles magic link authentication for portal customers
type MagicLinkService struct {
	db         database.Database
	smtpSender *smtp.NotificationSMTPSender
	baseURL    string
}

// MagicLinkResult contains the result of validating a magic link
type MagicLinkResult struct {
	PortalCustomerID int
	ChannelID        *int
	CustomerEmail    string
	CustomerName     string
}

// NewMagicLinkService creates a new magic link service
func NewMagicLinkService(db database.Database, smtpSender *smtp.NotificationSMTPSender, baseURL string) *MagicLinkService {
	return &MagicLinkService{
		db:         db,
		smtpSender: smtpSender,
		baseURL:    baseURL,
	}
}

// GenerateMagicLink creates a new magic link token for a portal customer
func (s *MagicLinkService) GenerateMagicLink(portalCustomerID int, channelID *int) (string, error) {
	// Generate a cryptographically secure random token
	tokenBytes := make([]byte, MagicLinkTokenLength)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("%w: %v", ErrMagicLinkGenerationFailed, err)
	}
	token := base64.URLEncoding.EncodeToString(tokenBytes)

	// Set expiry time
	expiresAt := time.Now().Add(MagicLinkExpiry)

	// Store token in database
	query := `
		INSERT INTO portal_customer_magic_links (portal_customer_id, token, channel_id, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?)
	`
	_, err := s.db.ExecWrite(query, portalCustomerID, token, channelID, expiresAt, time.Now())
	if err != nil {
		return "", fmt.Errorf("failed to store magic link token: %w", err)
	}

	slog.Debug("magic link generated", slog.String("component", "magic_link"), slog.Int("portal_customer_id", portalCustomerID))
	return token, nil
}

// SendMagicLinkEmail sends the magic link email to the portal customer
func (s *MagicLinkService) SendMagicLinkEmail(email, name, token, portalSlug string) error {
	if s.smtpSender == nil || !s.smtpSender.IsSMTPConfigured() {
		return ErrMagicLinkSMTPNotConfigured
	}

	// Generate magic link URL
	magicLinkURL := fmt.Sprintf("%s/portal/%s/verify?token=%s", s.baseURL, portalSlug, token)

	// Generate email content
	subject := "Sign in to your portal"
	htmlBody, textBody, err := s.generateEmailBody(name, magicLinkURL)
	if err != nil {
		return fmt.Errorf("failed to generate email body: %w", err)
	}

	// Send email using the SMTP sender
	return s.smtpSender.SendCustomEmail(email, subject, htmlBody, textBody)
}

// generateEmailBody generates the HTML and text email body for the magic link
func (s *MagicLinkService) generateEmailBody(firstName, magicLinkURL string) (htmlBody, textBody string, err error) {
	if firstName == "" {
		firstName = "there"
	}

	// HTML template
	htmlTemplate := `
<!DOCTYPE html>
<html>
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>Sign in to your portal</title>
	<style>
		body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; margin: 0; padding: 0; background-color: #f5f5f5; }
		.container { max-width: 600px; margin: 0 auto; background-color: white; }
		.header { background-color: #2563eb; color: white; padding: 24px; text-align: center; }
		.header h1 { margin: 0; font-size: 24px; font-weight: 600; }
		.content { padding: 32px 24px; }
		.greeting { font-size: 16px; color: #374151; margin-bottom: 24px; }
		.message { font-size: 14px; color: #4b5563; line-height: 1.6; margin-bottom: 24px; }
		.button-container { text-align: center; margin: 32px 0; }
		.button { display: inline-block; background-color: #2563eb; color: white; text-decoration: none; padding: 14px 32px; border-radius: 6px; font-weight: 600; font-size: 14px; }
		.button:hover { background-color: #1d4ed8; }
		.link-fallback { font-size: 12px; color: #9ca3af; margin-top: 24px; word-break: break-all; }
		.expiry-notice { font-size: 14px; color: #6b7280; margin-top: 24px; padding: 16px; background-color: #f9fafb; border-radius: 6px; }
		.security-notice { font-size: 12px; color: #9ca3af; margin-top: 24px; padding: 16px; background-color: #fef3c7; border-radius: 6px; }
		.footer { background-color: #f9fafb; padding: 24px; text-align: center; font-size: 14px; color: #6b7280; border-top: 1px solid #e5e7eb; }
	</style>
</head>
<body>
	<div class="container">
		<div class="header">
			<h1>Sign In</h1>
		</div>
		<div class="content">
			<div class="greeting">
				Hi {{.FirstName}},
			</div>
			<div class="message">
				Click the button below to sign in to your portal. This link will expire in 15 minutes.
			</div>
			<div class="button-container">
				<a href="{{.MagicLinkURL}}" class="button">Sign In to Portal</a>
			</div>
			<div class="expiry-notice">
				This link expires in 15 minutes. If you didn't request this link, you can safely ignore this email.
			</div>
			<div class="security-notice">
				<strong>Security tip:</strong> Never share this link with anyone. We will never ask you to share this link.
			</div>
			<div class="link-fallback">
				If the button doesn't work, copy and paste this link into your browser:<br>
				{{.MagicLinkURL}}
			</div>
		</div>
		<div class="footer">
			This is an automated email. Please do not reply.
		</div>
	</div>
</body>
</html>`

	// Text template
	textTemplate := `Hi {{.FirstName}},

Click the link below to sign in to your portal:
{{.MagicLinkURL}}

This link expires in 15 minutes.

If you didn't request this link, you can ignore this email.

Security tip: Never share this link with anyone.

---
This is an automated email. Please do not reply.`

	// Prepare template data
	templateData := struct {
		FirstName    string
		MagicLinkURL string
	}{
		FirstName:    firstName,
		MagicLinkURL: magicLinkURL,
	}

	// Parse and execute HTML template
	htmlTmpl, err := template.New("html").Parse(htmlTemplate)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse HTML template: %w", err)
	}

	var htmlBuffer bytes.Buffer
	if err = htmlTmpl.Execute(&htmlBuffer, templateData); err != nil {
		return "", "", fmt.Errorf("failed to execute HTML template: %w", err)
	}

	// Parse and execute text template
	textTmpl, err := template.New("text").Parse(textTemplate)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse text template: %w", err)
	}

	var textBuffer bytes.Buffer
	if err := textTmpl.Execute(&textBuffer, templateData); err != nil {
		return "", "", fmt.Errorf("failed to execute text template: %w", err)
	}

	return htmlBuffer.String(), textBuffer.String(), nil
}

// ValidateMagicLink validates a magic link token and returns the portal customer info
func (s *MagicLinkService) ValidateMagicLink(token string) (*MagicLinkResult, error) {
	// Find magic link by token
	query := `
		SELECT ml.id, ml.portal_customer_id, ml.channel_id, ml.expires_at, ml.used_at,
		       pc.email, pc.name
		FROM portal_customer_magic_links ml
		JOIN portal_customers pc ON ml.portal_customer_id = pc.id
		WHERE ml.token = ?
	`

	var linkID int
	var portalCustomerID int
	var channelID sql.NullInt64
	var expiresAt time.Time
	var usedAt sql.NullTime
	var email, name string

	err := s.db.QueryRow(query, token).Scan(
		&linkID, &portalCustomerID, &channelID, &expiresAt, &usedAt,
		&email, &name,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrMagicLinkInvalid
		}
		return nil, fmt.Errorf("failed to validate magic link: %w", err)
	}

	// Check if already used
	if usedAt.Valid {
		return nil, ErrMagicLinkAlreadyUsed
	}

	// Check if expired
	if time.Now().After(expiresAt) {
		return nil, ErrMagicLinkExpired
	}

	// Mark token as used
	updateQuery := `UPDATE portal_customer_magic_links SET used_at = ? WHERE id = ?`
	_, err = s.db.ExecWrite(updateQuery, time.Now(), linkID)
	if err != nil {
		return nil, fmt.Errorf("failed to mark magic link as used: %w", err)
	}

	slog.Info("magic link validated", slog.String("component", "magic_link"), slog.Int("portal_customer_id", portalCustomerID), slog.String("email", email))

	result := &MagicLinkResult{
		PortalCustomerID: portalCustomerID,
		CustomerEmail:    email,
		CustomerName:     name,
	}

	if channelID.Valid {
		id := int(channelID.Int64)
		result.ChannelID = &id
	}

	return result, nil
}

// FindOrCreatePortalCustomer finds a portal customer by email or creates one if it doesn't exist
func (s *MagicLinkService) FindOrCreatePortalCustomer(email, name string, channelID int) (int, error) {
	// First try to find existing customer
	var customerID int
	findQuery := `SELECT id FROM portal_customers WHERE email = ?`
	err := s.db.QueryRow(findQuery, email).Scan(&customerID)

	if err == nil {
		// Customer exists, grant channel access if not already granted
		s.grantChannelAccess(customerID, channelID)
		return customerID, nil
	}

	if err != sql.ErrNoRows {
		return 0, fmt.Errorf("failed to find portal customer: %w", err)
	}

	// Customer doesn't exist, create new one
	now := time.Now()
	insertQuery := `
		INSERT INTO portal_customers (name, email, created_at, updated_at)
		VALUES (?, ?, ?, ?)
		RETURNING id
	`
	err = s.db.QueryRow(insertQuery, name, email, now, now).Scan(&customerID)
	if err != nil {
		return 0, fmt.Errorf("failed to create portal customer: %w", err)
	}

	// Grant channel access
	s.grantChannelAccess(customerID, channelID)

	slog.Info("portal customer created", slog.String("component", "magic_link"), slog.Int("portal_customer_id", customerID), slog.String("email", email))
	return customerID, nil
}

// grantChannelAccess grants a portal customer access to a channel if not already granted
func (s *MagicLinkService) grantChannelAccess(portalCustomerID, channelID int) {
	// Check if already has access
	var accessID int
	checkQuery := `SELECT id FROM portal_customer_channels WHERE portal_customer_id = ? AND channel_id = ?`
	err := s.db.QueryRow(checkQuery, portalCustomerID, channelID).Scan(&accessID)
	if err == nil {
		// Already has access
		return
	}

	// Grant access
	insertQuery := `
		INSERT INTO portal_customer_channels (portal_customer_id, channel_id, created_at)
		VALUES (?, ?, ?)
	`
	_, _ = s.db.ExecWrite(insertQuery, portalCustomerID, channelID, time.Now())
}

// GetPortalCustomerByEmail finds a portal customer by email
func (s *MagicLinkService) GetPortalCustomerByEmail(email string) (customerID int, firstName string, err error) {
	query := `SELECT id, name FROM portal_customers WHERE email = ?`
	err = s.db.QueryRow(query, email).Scan(&customerID, &firstName)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, "", ErrPortalCustomerNotFound
		}
		return 0, "", fmt.Errorf("failed to find portal customer: %w", err)
	}
	return customerID, firstName, nil
}

// CleanupExpiredMagicLinks removes expired magic link tokens
func (s *MagicLinkService) CleanupExpiredMagicLinks() error {
	query := `DELETE FROM portal_customer_magic_links WHERE expires_at < ? OR used_at IS NOT NULL`
	_, err := s.db.ExecWrite(query, time.Now().Add(-24*time.Hour)) // Keep used/expired links for 24 hours for auditing
	if err != nil {
		return fmt.Errorf("failed to cleanup expired magic links: %w", err)
	}
	return nil
}
