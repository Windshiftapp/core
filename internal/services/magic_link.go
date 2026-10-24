package services

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/emailutil"
	"windshift/internal/models"
	"windshift/internal/repository"
)

var (
	ErrMagicLinkExpired         = errors.New("magic link has expired")
	ErrMagicLinkInvalid         = errors.New("magic link is invalid")
	ErrMagicLinkAlreadyUsed     = errors.New("magic link has already been used")
	ErrMagicLinkChannelMismatch = errors.New("magic link issued for a different portal")
	// ErrPortalCustomerDeactivated refuses link issuance for a deactivated
	// customer; the session layer refuses them anyway (WI-1554).
	ErrPortalCustomerDeactivated = errors.New("portal customer deactivated")
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
	// Deactivated customers get no link — the auth paths all refuse them
	// (WI-1554), so issuing a token would only invite a confusing dead end.
	// Callers treat this like any other failure (generic success for sign-in
	// requests, so enumeration is not aided).
	var deactivatedAt sql.NullTime
	if err := s.db.QueryRow(`SELECT deactivated_at FROM portal_customers WHERE id = ?`, portalCustomerID).Scan(&deactivatedAt); err != nil {
		return "", fmt.Errorf("failed to load portal customer state: %w", err)
	}
	if deactivatedAt.Valid {
		return "", ErrPortalCustomerDeactivated
	}

	tokenBytes := make([]byte, MagicLinkTokenLength)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("%w: %w", ErrMagicLinkGenerationFailed, err)
	}
	token := base64.URLEncoding.EncodeToString(tokenBytes)

	expiresAt := time.Now().Add(expiry)

	// Store only the hash; the plaintext token leaves this process solely in
	// the emailed verify URL.
	query := `
		INSERT INTO portal_customer_magic_links (portal_customer_id, token, channel_id, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?)
	`
	_, err := s.db.ExecWrite(query, portalCustomerID, hashTokenAtRest(token), channelID, expiresAt, time.Now())
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
// (base64.URLEncoding), so no further escaping is needed. An optional `next`
// destination (a same-portal path already validated by the caller) rides
// alongside the token so the customer resumes the page that sent them to
// sign in instead of landing on the portal home.
func (s *MagicLinkService) SendMagicLinkEmail(email, name, token, portalSlug, next string) error {
	if name == "" {
		name = "there"
	}
	magicURL := fmt.Sprintf("%s/portal/%s/verify#token=%s", s.baseURL, portalSlug, token)
	if next != "" {
		magicURL += "&next=" + escapeFragmentValue(next)
	}
	return s.smtpSender.SendTransactional(email, emailutil.TemplateMagicLink, struct {
		FirstName    string
		MagicLinkURL string
		ExpiresIn    string
	}{name, magicURL, emailutil.HumanizeDuration(MagicLinkExpiry)})
}

// PortalNextPath validates an optional post-sign-in destination supplied with
// a magic-link request. Only paths inside the requesting portal are accepted,
// so a crafted request cannot turn the emailed link into an open redirect.
// Invalid or over-long values are dropped rather than rejected, keeping the
// sign-in request response uniform.
func PortalNextPath(raw, portalSlug string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 2048 {
		return ""
	}
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return ""
	}
	prefix := "/portal/" + portalSlug
	if raw != prefix && !strings.HasPrefix(raw, prefix+"/") && !strings.HasPrefix(raw, prefix+"?") {
		return ""
	}
	return raw
}

// escapeFragmentValue percent-encodes a value embedded in the verify URL
// fragment. QueryEscape handles the `&` and `?` separators; its `+` encoding
// for spaces is rewritten to %20 because the frontend decodes the fragment
// with decodeURIComponent, which does not treat `+` as a space.
func escapeFragmentValue(value string) string {
	return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
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
		ExpiresIn   string
	}{name, itemKey, itemTitle, approvalURL, emailutil.HumanizeDuration(ApprovalMagicLinkExpiry)})
}

// ValidateMagicLink validates a magic link token presented at a specific
// portal channel. On success, the row is marked used and the populated
// MagicLinkResult is returned with a nil error.
//
// On ErrMagicLinkExpired, ErrMagicLinkAlreadyUsed, and
// ErrMagicLinkChannelMismatch the result is also populated (so callers can
// drive a recovery UX that prefills the customer's email) but no session is
// minted. ErrMagicLinkInvalid returns nil.
//
// expectedChannelID is the channel the request is being made against. A
// token issued for channel A presented at the verify endpoint of channel B
// is rejected as a channel mismatch *without* consuming the token, so the
// customer can still redeem the link at the correct portal. Legacy tokens
// minted without a channel_id (channel_id IS NULL) are accepted everywhere.
func (s *MagicLinkService) ValidateMagicLink(token string, expectedChannelID int) (*MagicLinkResult, error) {
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

	err := s.db.QueryRow(query, hashTokenAtRest(token)).Scan(
		&linkID, &portalCustomerID, &channelID, &expiresAt, &usedAt,
		&email, &name,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
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

	// Channel binding: a token minted for channel A cannot be redeemed via
	// portal B's verify endpoint. The check happens before the atomic
	// mark-as-used UPDATE so a misdirected token is not burned by the wrong
	// portal — the customer can still complete sign-in at the right portal.
	if hint.ChannelID != nil && *hint.ChannelID != expectedChannelID {
		return hint, ErrMagicLinkChannelMismatch
	}

	// Atomic mark-as-used: the `used_at IS NULL` guard turns concurrent
	// validations of the same token into a single winner. The loser sees
	// RowsAffected == 0 and is rejected as already-used.
	res, err := s.db.ExecWrite(
		`UPDATE portal_customer_magic_links SET used_at = ? WHERE id = ? AND used_at IS NULL`,
		time.Now(), linkID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to mark magic link as used: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to inspect magic link update: %w", err)
	}
	if affected == 0 {
		return hint, ErrMagicLinkAlreadyUsed
	}

	slog.Info("magic link validated", slog.String("component", "magic_link"), slog.Int("portal_customer_id", portalCustomerID), slog.String("email", email))

	return hint, nil
}

// FindOrCreatePortalCustomer finds a portal customer by email or creates one
// if it doesn't exist, then grants access to the channel.
func (s *MagicLinkService) FindOrCreatePortalCustomer(email, name string, channelID int) (int, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return 0, fmt.Errorf("email is required")
	}

	repo := repository.NewPortalCustomerRepository(s.db)
	customerID, created, err := repo.FindOrCreateByEmail(context.Background(), name, email, models.CustomerCreatedViaMagicLink)
	if err != nil {
		return 0, err
	}
	if _, err := repo.EnsureChannelAccess(customerID, channelID); err != nil {
		return 0, err
	}

	if created {
		slog.Info("portal customer created", slog.String("component", "magic_link"), slog.Int("portal_customer_id", customerID), slog.String("email", email))
	}
	return customerID, nil
}

// GetPortalCustomerByEmail finds a portal customer by email
func (s *MagicLinkService) GetPortalCustomerByEmail(email string) (customerID int, firstName string, err error) {
	query := `SELECT id, name FROM portal_customers WHERE LOWER(email) = LOWER(?)`
	err = s.db.QueryRow(query, email).Scan(&customerID, &firstName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
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
