package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"windshift/internal/database"
	"windshift/internal/repository"
)

const (
	PortalSessionCookieName  = "windshift_portal_session"
	PortalSessionTokenLength = 32                 // 256-bit session tokens
	PortalSessionDuration    = 7 * 24 * time.Hour // 7 days
)

var (
	ErrPortalSessionNotFound = errors.New("portal session not found")
	ErrPortalSessionExpired  = errors.New("portal session expired")
	ErrPortalSessionInvalid  = errors.New("invalid portal session")
	// ErrPortalCustomerDeactivated guards session creation: a deactivated
	// customer must never mint a session on any auth path (WI-1554).
	ErrPortalCustomerDeactivated = errors.New("portal customer deactivated")
)

// PortalCustomer represents a portal customer from the database
type PortalCustomer struct {
	ID                     int        `json:"id"`
	Name                   string     `json:"name"`
	Email                  string     `json:"email"`
	Phone                  string     `json:"phone,omitempty"`
	CustomerOrganisationID *int       `json:"customer_organisation_id,omitempty"` //nolint:misspell // database column name
	DeactivatedAt          *time.Time `json:"deactivated_at,omitempty"`           // Security deactivation; passkey login refuses deactivated customers (WI-1554)
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

// PortalSession represents an active portal customer session.
// ChannelID is the portal channel the session was authenticated through; it
// is non-nil for sessions minted after the channel-binding migration. Legacy
// rows have ChannelID == nil and are rejected by the binding check on next
// request, forcing a re-auth.
type PortalSession struct {
	ID               int             `json:"id"`
	PortalCustomerID int             `json:"portal_customer_id"`
	Token            string          `json:"-"`
	ChannelID        *int            `json:"channel_id,omitempty"`
	ExpiresAt        time.Time       `json:"expires_at"`
	IPAddress        string          `json:"ip_address"`
	UserAgent        string          `json:"user_agent"`
	IsActive         bool            `json:"is_active"`
	CreatedAt        time.Time       `json:"created_at"`
	Customer         *PortalCustomer `json:"customer,omitempty"`
}

// PortalSessionManager handles secure session management for portal customers
type PortalSessionManager struct {
	cookieManager
	db database.Database
	// ipBinding is the resolved SESSION_IP_BINDING mode (config.SessionIPBinding*)
	// that portal session validation applies to a client-IP change. An unknown
	// or zero value is treated as strict so managers built without config.Load
	// fail closed.
	ipBinding string
}

// NewPortalSessionManager creates a new portal session manager with secure cookie handling.
// If cookieSecret is set, deterministic cookie keys are derived from it
// so that sessions survive process restarts with the same secret.
// ipBinding is the resolved SESSION_IP_BINDING mode; portal sessions share the
// setting with internal user sessions because they enforce the same binding.
// last review: ser, 210426, NOTE: Found hardcoded env var in caller
func NewPortalSessionManager(db database.Database, useSecureCookies, useProxy bool, additionalProxies []string, cookieSecret, ipBinding string) *PortalSessionManager {
	return &PortalSessionManager{
		cookieManager: newCookieManager(useSecureCookies, useProxy, additionalProxies, cookieSecret,
			"windshift-portal-cookie-hash", "windshift-portal-cookie-block"),
		db:        db,
		ipBinding: ipBinding,
	}
}

// CreatePortalSession creates a new session for a portal customer bound to a
// specific portal channel. channelID is required so the session can be
// rejected if presented on a different portal slug.
// last review: ser, 210426, TODO: Remove inline sql
func (sm *PortalSessionManager) CreatePortalSession(portalCustomerID, channelID int, ipAddress, userAgent string) (*PortalSession, error) {
	// Normalise to host-only so validation can compare consistently even if a
	// caller accidentally passes host:port.
	if host, _, err := net.SplitHostPort(ipAddress); err == nil {
		ipAddress = host
	}

	// Deactivated customers must never mint a session, regardless of which
	// auth path resolved them (magic-link redemption, passkey login, ...).
	var deactivatedAt sql.NullTime
	if err := sm.db.QueryRow(`SELECT deactivated_at FROM portal_customers WHERE id = ?`, portalCustomerID).Scan(&deactivatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPortalSessionInvalid
		}
		slog.Error("portal session customer state check failed", slog.String("component", "portal_auth"), slog.Any("error", err))
		return nil, fmt.Errorf("failed to create portal session: %w", err)
	}
	if deactivatedAt.Valid {
		return nil, ErrPortalCustomerDeactivated
	}
	if err := repository.NewCustomerAccessRepository(sm.db).PortalLoginAllowed(context.Background(), portalCustomerID, channelID); err != nil {
		return nil, err
	}

	slog.Debug("creating portal session", slog.String("component", "portal_auth"), slog.Int("portal_customer_id", portalCustomerID), slog.Int("channel_id", channelID), slog.String("ip_address", ipAddress))

	token, err := generateSessionToken()
	if err != nil {
		return nil, err
	}

	expiresAt := time.Now().Add(PortalSessionDuration)

	// Insert session into database using RETURNING clause
	query := `
		INSERT INTO portal_customer_sessions (portal_customer_id, session_token, channel_id, expires_at, ip_address, user_agent, is_active, created_at)
		VALUES (?, ?, ?, ?, ?, ?, true, ?)
		RETURNING id
	`
	var sessionID int64
	err = sm.db.QueryRow(query, portalCustomerID, hashSessionToken(token), channelID, expiresAt, ipAddress, userAgent, time.Now()).Scan(&sessionID)
	if err != nil {
		slog.Error("portal session db insert failed", slog.String("component", "portal_auth"), slog.Any("error", err))
		return nil, fmt.Errorf("failed to create portal session: %w", err)
	}

	slog.Debug("portal session inserted", slog.String("component", "portal_auth"), slog.Int64("session_id", sessionID))

	return &PortalSession{
		ID:               int(sessionID),
		PortalCustomerID: portalCustomerID,
		Token:            token,
		ChannelID:        &channelID,
		ExpiresAt:        expiresAt,
		IPAddress:        ipAddress,
		UserAgent:        userAgent,
		IsActive:         true,
		CreatedAt:        time.Now(),
	}, nil
}

// ValidatePortalSession validates a session token and returns the session with customer info
// last review: ser, 210426, TODO: Remove inline sql
func (sm *PortalSessionManager) ValidatePortalSession(token, ipAddress string) (*PortalSession, error) {
	if token == "" {
		return nil, ErrPortalSessionInvalid
	}
	if host, _, err := net.SplitHostPort(ipAddress); err == nil {
		ipAddress = host
	}

	//nolint:misspell // database column name uses British spelling
	query := `
		SELECT
			s.id, s.portal_customer_id, s.session_token, s.channel_id, s.expires_at, s.ip_address, s.user_agent, s.is_active, s.created_at,
			pc.name, pc.email, pc.phone, pc.customer_organisation_id, pc.created_at, pc.updated_at
		FROM portal_customer_sessions s
		JOIN portal_customers pc ON s.portal_customer_id = pc.id
		WHERE s.session_token IN (?, ?) AND s.is_active = true AND pc.deactivated_at IS NULL
	`

	// New sessions are stored as SHA-256 digests. Keep a bounded plaintext
	// fallback so sessions issued before this upgrade remain valid until their
	// normal seven-day expiry.
	row := sm.db.QueryRow(query, hashSessionToken(token), token)

	session := &PortalSession{Customer: &PortalCustomer{}}
	var phone sql.NullString
	var orgID sql.NullInt64
	var channelID sql.NullInt64

	err := row.Scan(
		&session.ID, &session.PortalCustomerID, &session.Token, &channelID, &session.ExpiresAt, &session.IPAddress, &session.UserAgent, &session.IsActive, &session.CreatedAt,
		&session.Customer.Name, &session.Customer.Email, &phone, &orgID, &session.Customer.CreatedAt, &session.Customer.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPortalSessionNotFound
		}
		return nil, fmt.Errorf("failed to validate portal session: %w", err)
	}

	// Check if session is expired
	if time.Now().After(session.ExpiresAt) {
		// Clean up expired session
		_ = sm.DeletePortalSession(token)
		return nil, ErrPortalSessionExpired
	}
	if channelID.Valid {
		if err := repository.NewCustomerAccessRepository(sm.db).PortalLoginAllowed(context.Background(), session.PortalCustomerID, int(channelID.Int64)); err != nil {
			return nil, err
		}
	}

	// Validate IP address for security. Portal sessions store the client IP at
	// creation; subsequent validations apply the same SESSION_IP_BINDING mode
	// as internal user sessions. Legacy rows with no recorded IP are accepted
	// but logged so operators can investigate. Under strict a missing request
	// IP or a mismatch fails closed; under log the session is followed to its
	// new IP; under off no comparison happens. No mode deactivates the session.
	switch decideIPBinding(sm.ipBinding, session.IPAddress, ipAddress) {
	case ipBindingLegacyUnbound:
		slog.Warn("portal session has no recorded IP, skipping bind check",
			slog.Int("portal_customer_id", session.PortalCustomerID),
			slog.Int("session_id", session.ID))
	case ipBindingRejectNoRequestIP:
		slog.Warn("request has no client IP, rejecting IP-bound portal session",
			slog.Int("portal_customer_id", session.PortalCustomerID),
			slog.String("session_ip", session.IPAddress))
		return nil, ErrPortalSessionInvalid
	case ipBindingAcceptNoRequestIP:
		slog.Warn("request has no client IP, accepting IP-bound portal session",
			slog.Int("portal_customer_id", session.PortalCustomerID),
			slog.String("session_ip", session.IPAddress),
			slog.String("session_ip_binding", sm.ipBinding))
	case ipBindingAcceptUnparsedIP:
		slog.Warn("request client IP is not a valid address, accepting IP-bound portal session without rebinding",
			slog.Int("portal_customer_id", session.PortalCustomerID),
			slog.String("session_ip", session.IPAddress),
			slog.String("request_ip", ipAddress),
			slog.String("session_ip_binding", sm.ipBinding))
	case ipBindingRejectMismatch:
		slog.Warn("portal session IP mismatch",
			slog.Int("portal_customer_id", session.PortalCustomerID),
			slog.String("session_ip", session.IPAddress),
			slog.String("request_ip", ipAddress))
		return nil, ErrPortalSessionInvalid
	case ipBindingRebindMismatch:
		slog.Warn("portal session IP mismatch",
			slog.Int("portal_customer_id", session.PortalCustomerID),
			slog.String("session_ip", session.IPAddress),
			slog.String("request_ip", ipAddress))
		sm.rebindPortalSessionIP(token, session, ipAddress)
	case ipBindingMatch, ipBindingSkip:
	}

	if channelID.Valid {
		id := int(channelID.Int64)
		session.ChannelID = &id
	}

	// Set customer fields
	session.Customer.ID = session.PortalCustomerID
	if phone.Valid {
		session.Customer.Phone = phone.String
	}
	if orgID.Valid {
		id := int(orgID.Int64)
		session.Customer.CustomerOrganisationID = &id
	}

	return session, nil
}

// rebindPortalSessionIP moves a portal session to the client IP it is now
// presented from (log mode only). Like the user-session rebind, a failed write
// costs bookkeeping rather than availability: the request is still served and
// the next one retries. Portal sessions have no local validation cache, so
// nothing needs invalidating; the in-memory session is advanced on success
// because it is returned to the caller.
func (sm *PortalSessionManager) rebindPortalSessionIP(token string, session *PortalSession, ipAddress string) {
	// Portal tokens are stored as digests with the same legacy plaintext
	// fallback as user sessions, so the predicate must match both forms.
	query := `UPDATE portal_customer_sessions SET ip_address = ? WHERE session_token IN (?, ?) AND is_active = true`
	if _, err := sm.db.ExecWrite(query, ipAddress, hashSessionToken(token), token); err != nil {
		slog.Error("failed to rebind portal session to the new client IP",
			slog.Int("portal_customer_id", session.PortalCustomerID),
			slog.Int("session_id", session.ID),
			slog.String("request_ip", ipAddress),
			slog.Any("error", err))
		return
	}
	session.IPAddress = ipAddress
}

// DeletePortalSession invalidates a session
// last review: ser, 210426, TODO: Remove inline sql
func (sm *PortalSessionManager) DeletePortalSession(token string) error {
	query := `UPDATE portal_customer_sessions SET is_active = false WHERE session_token IN (?, ?)`
	_, err := sm.db.ExecWrite(query, hashSessionToken(token), token)
	if err != nil {
		return fmt.Errorf("failed to delete portal session: %w", err)
	}
	return nil
}

// SetPortalSessionCookie sets a secure session cookie
func (sm *PortalSessionManager) SetPortalSessionCookie(w http.ResponseWriter, r *http.Request, token string) error {
	return sm.setSessionCookie(w, r, PortalSessionCookieName, token, int(PortalSessionDuration.Seconds()))
}

// ClearPortalSessionCookie removes the session cookie
func (sm *PortalSessionManager) ClearPortalSessionCookie(w http.ResponseWriter, r *http.Request) {
	sm.clearSessionCookie(w, r, PortalSessionCookieName)
}

// GetPortalSessionFromRequest extracts session token from cookie or Authorization header
func (sm *PortalSessionManager) GetPortalSessionFromRequest(r *http.Request) (string, error) {
	return sm.getSessionFromRequest(r, PortalSessionCookieName)
}
