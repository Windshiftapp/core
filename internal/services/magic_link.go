package services

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"windshift/internal/database"
	"windshift/internal/emailutil"
)

var (
	ErrMagicLinkExpired          = errors.New("magic link has expired")
	ErrMagicLinkInvalid          = errors.New("magic link is invalid")
	ErrMagicLinkAlreadyUsed      = errors.New("magic link has already been used")
	ErrPortalCustomerNotFound    = errors.New("portal customer not found")
	ErrMagicLinkGenerationFailed = errors.New("failed to generate magic link token")
)

const (
	// MagicLinkExpiry is how long a portal-initiated sign-in link is valid.
	// Bumped from 15 min so customers have a comfortable window to fish the
	// email out of clutter without a re-request.
	MagicLinkExpiry = 30 * time.Minute
	// ApprovalMagicLinkExpiry is how long an "approval requested" email's
	// embedded magic link is valid. Approvals are pushed to the customer
	// and routinely sit in inboxes for hours; 24h matches realistic cadence.
	// The token still grants a full portal session, so the expired-link
	// path falls back to a fresh sign-in (handled by the frontend) rather
	// than extending this further.
	ApprovalMagicLinkExpiry = 24 * time.Hour
	// MagicLinkTokenLength is the length of the random bytes for the token
	MagicLinkTokenLength = 32
)

// MagicLinkService handles magic link authentication for portal customers
type MagicLinkService struct {
	db         database.Database
	smtpSender TransactionalEmailSender
	baseURL    string
}

// MagicLinkResult contains the result of validating a magic link
type MagicLinkResult struct {
	PortalCustomerID int
	ChannelID        *int
	CustomerEmail    string
	CustomerName     string
}

// NewMagicLinkService creates a new magic link service.
func NewMagicLinkService(db database.Database, smtpSender TransactionalEmailSender, baseURL string) *MagicLinkService {
	return &MagicLinkService{
		db:         db,
		smtpSender: smtpSender,
		baseURL:    baseURL,
	}
}

// GenerateMagicLink creates a sign-in magic link token for a portal customer.
// Uses MagicLinkExpiry; for approval-requested emails use GenerateApprovalMagicLink.
func (s *MagicLinkService) GenerateMagicLink(portalCustomerID int, channelID *int) (string, error) {
	return s.generateMagicLink(portalCustomerID, channelID, MagicLinkExpiry)
}

// GenerateApprovalMagicLink creates a magic link token destined for an
// "approval requested" email. Same shape and security model as the sign-in
// link (single-use, full portal session on consume); only the TTL differs.
func (s *MagicLinkService) GenerateApprovalMagicLink(portalCustomerID int, channelID *int) (string, error) {
	return s.generateMagicLink(portalCustomerID, channelID, ApprovalMagicLinkExpiry)
}

func (s *MagicLinkService) generateMagicLink(portalCustomerID int, channelID *int, expiry time.Duration) (string, error) {
	tokenBytes := make([]byte, MagicLinkTokenLength)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("%w: %w", ErrMagicLinkGenerationFailed, err)
	}
	token := base64.URLEncoding.EncodeToString(tokenBytes)

	expiresAt := time.Now().Add(expiry)

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

// SendMagicLinkEmail sends the magic link email to the portal customer.
// The token is placed in the URL fragment (#) so it is not transmitted in
// HTTP Referer headers, query-string logs, or any third-party request
// initiated by the verify page. The token is already URL-safe
// (base64.URLEncoding), so no further escaping is needed.
func (s *MagicLinkService) SendMagicLinkEmail(email, name, token, portalSlug string) error {
	if name == "" {
		name = "there"
	}
	url := fmt.Sprintf("%s/portal/%s/verify#token=%s", s.baseURL, portalSlug, token)
	return s.smtpSender.SendTransactional(email, emailutil.TemplateMagicLink, struct {
		FirstName    string
		MagicLinkURL string
	}{name, url})
}

// SendApprovalRequestEmail sends an "approval requested" email to a portal
// customer. The token is placed in the URL fragment (#) and a `next` parameter
// points the verify page at the specific approval after sign-in. The token
// uses ApprovalMagicLinkExpiry (24h); if the customer takes longer to act,
// the verify page detects the expired token, stashes the intended `next`
// target, and bounces them through a fresh sign-in that lands on the same
// approval — see PortalVerifyLink.svelte.
func (s *MagicLinkService) SendApprovalRequestEmail(email, name, token, portalSlug string, requestID int, itemKey, itemTitle string) error {
	if name == "" {
		name = "there"
	}
	approvalURL := fmt.Sprintf("%s/portal/%s/verify#token=%s&next=/portal/%s/approvals/%d", s.baseURL, portalSlug, token, portalSlug, requestID)
	return s.smtpSender.SendTransactional(email, emailutil.TemplateApprovalRequested, struct {
		FirstName   string
		ItemKey     string
		ItemTitle   string
		ApprovalURL string
	}{name, itemKey, itemTitle, approvalURL})
}

// ValidateMagicLink validates a magic link token. On success, the row is
// marked used and the populated MagicLinkResult is returned with a nil error.
// On ErrMagicLinkExpired and ErrMagicLinkAlreadyUsed, the result is also
// populated (so callers can drive a recovery UX that prefills the customer's
// email) but no session is minted. ErrMagicLinkInvalid returns nil.
func (s *MagicLinkService) ValidateMagicLink(token string) (*MagicLinkResult, error) {
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

	hint := &MagicLinkResult{
		PortalCustomerID: portalCustomerID,
		CustomerEmail:    email,
		CustomerName:     name,
	}
	if channelID.Valid {
		id := int(channelID.Int64)
		hint.ChannelID = &id
	}

	if usedAt.Valid {
		return hint, ErrMagicLinkAlreadyUsed
	}

	if time.Now().After(expiresAt) {
		return hint, ErrMagicLinkExpired
	}

	// Mark token as used
	updateQuery := `UPDATE portal_customer_magic_links SET used_at = ? WHERE id = ?`
	_, err = s.db.ExecWrite(updateQuery, time.Now(), linkID)
	if err != nil {
		return nil, fmt.Errorf("failed to mark magic link as used: %w", err)
	}

	slog.Info("magic link validated", slog.String("component", "magic_link"), slog.Int("portal_customer_id", portalCustomerID), slog.String("email", email))

	return hint, nil
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
