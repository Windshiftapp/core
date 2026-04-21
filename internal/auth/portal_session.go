package auth

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"windshift/internal/database"
)

const (
	PortalSessionCookieName  = "windshift_portal_session"
	PortalSessionTokenLength = 32                 // 256-bit session tokens
	PortalSessionDuration    = 7 * 24 * time.Hour // 7 days
)

var (
	ErrPortalSessionNotFound = fmt.Errorf("portal session not found")
	ErrPortalSessionExpired  = fmt.Errorf("portal session expired")
	ErrPortalSessionInvalid  = fmt.Errorf("invalid portal session")
)

// PortalCustomer represents a portal customer from the database
type PortalCustomer struct {
	ID                     int       `json:"id"`
	Name                   string    `json:"name"`
	Email                  string    `json:"email"`
	Phone                  string    `json:"phone,omitempty"`
	CustomerOrganisationID *int      `json:"customer_organisation_id,omitempty"` //nolint:misspell // database column name
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

// PortalSession represents an active portal customer session
type PortalSession struct {
	ID               int             `json:"id"`
	PortalCustomerID int             `json:"portal_customer_id"`
	Token            string          `json:"token"`
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
}

// NewPortalSessionManager creates a new portal session manager with secure cookie handling.
// If cookieSecret is set, deterministic cookie keys are derived from it
// so that sessions survive process restarts with the same secret.
// last review: ser, 210426, NOTE: Found hardcoded env var in caller
func NewPortalSessionManager(db database.Database, useSecureCookies, useProxy bool, additionalProxies []string, cookieSecret string) *PortalSessionManager {
	return &PortalSessionManager{
		cookieManager: newCookieManager(useSecureCookies, useProxy, additionalProxies, cookieSecret,
			"windshift-portal-cookie-hash", "windshift-portal-cookie-block"),
		db: db,
	}
}

// CreatePortalSession creates a new session for a portal customer
// last review: ser, 210426, TODO: Remove inline sql
func (sm *PortalSessionManager) CreatePortalSession(portalCustomerID int, ipAddress, userAgent string) (*PortalSession, error) {
	slog.Debug("creating portal session", slog.String("component", "portal_auth"), slog.Int("portal_customer_id", portalCustomerID), slog.String("ip_address", ipAddress))

	token, err := generateSessionToken()
	if err != nil {
		return nil, err
	}

	expiresAt := time.Now().Add(PortalSessionDuration)

	// Insert session into database using RETURNING clause
	query := `
		INSERT INTO portal_customer_sessions (portal_customer_id, session_token, expires_at, ip_address, user_agent, is_active, created_at)
		VALUES (?, ?, ?, ?, ?, true, ?)
		RETURNING id
	`
	var sessionID int64
	err = sm.db.QueryRow(query, portalCustomerID, token, expiresAt, ipAddress, userAgent, time.Now()).Scan(&sessionID)
	if err != nil {
		slog.Error("portal session db insert failed", slog.String("component", "portal_auth"), slog.Any("error", err))
		return nil, fmt.Errorf("failed to create portal session: %w", err)
	}

	slog.Debug("portal session inserted", slog.String("component", "portal_auth"), slog.Int64("session_id", sessionID))

	return &PortalSession{
		ID:               int(sessionID),
		PortalCustomerID: portalCustomerID,
		Token:            token,
		ExpiresAt:        expiresAt,
		IPAddress:        ipAddress,
		UserAgent:        userAgent,
		IsActive:         true,
		CreatedAt:        time.Now(),
	}, nil
}

// ValidatePortalSession validates a session token and returns the session with customer info
// last review: ser, 210426, TODO: Remove inline sql
func (sm *PortalSessionManager) ValidatePortalSession(token string) (*PortalSession, error) {
	if token == "" {
		return nil, ErrPortalSessionInvalid
	}

	//nolint:misspell // database column name uses British spelling
	query := `
		SELECT
			s.id, s.portal_customer_id, s.session_token, s.expires_at, s.ip_address, s.user_agent, s.is_active, s.created_at,
			pc.name, pc.email, pc.phone, pc.customer_organisation_id, pc.created_at, pc.updated_at
		FROM portal_customer_sessions s
		JOIN portal_customers pc ON s.portal_customer_id = pc.id
		WHERE s.session_token = ? AND s.is_active = true
	`

	row := sm.db.QueryRow(query, token)

	session := &PortalSession{Customer: &PortalCustomer{}}
	var phone sql.NullString
	var orgID sql.NullInt64

	err := row.Scan(
		&session.ID, &session.PortalCustomerID, &session.Token, &session.ExpiresAt, &session.IPAddress, &session.UserAgent, &session.IsActive, &session.CreatedAt,
		&session.Customer.Name, &session.Customer.Email, &phone, &orgID, &session.Customer.CreatedAt, &session.Customer.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
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

// DeletePortalSession invalidates a session
// last review: ser, 210426, TODO: Remove inline sql
func (sm *PortalSessionManager) DeletePortalSession(token string) error {
	query := `UPDATE portal_customer_sessions SET is_active = false WHERE session_token = ?`
	_, err := sm.db.ExecWrite(query, token)
	if err != nil {
		return fmt.Errorf("failed to delete portal session: %w", err)
	}
	return nil
}

// CleanupExpiredSessions removes expired sessions from the database
// last review: ser, 210426, FIXME: unused
func (sm *PortalSessionManager) CleanupExpiredSessions() error {
	query := `UPDATE portal_customer_sessions SET is_active = false WHERE expires_at < ? AND is_active = true`
	_, err := sm.db.ExecWrite(query, time.Now())
	if err != nil {
		return fmt.Errorf("failed to cleanup expired portal sessions: %w", err)
	}
	return nil
}

// SetPortalSessionCookie sets a secure session cookie
func (sm *PortalSessionManager) SetPortalSessionCookie(w http.ResponseWriter, r *http.Request, token string) error {
	return sm.setSessionCookie(w, r, PortalSessionCookieName, token, int(PortalSessionDuration.Seconds()))
}

// GetPortalSessionFromCookie extracts session token from cookie
func (sm *PortalSessionManager) GetPortalSessionFromCookie(r *http.Request) (string, error) {
	return sm.getSessionFromCookie(r, PortalSessionCookieName)
}

// ClearPortalSessionCookie removes the session cookie
func (sm *PortalSessionManager) ClearPortalSessionCookie(w http.ResponseWriter, r *http.Request) {
	sm.clearSessionCookie(w, r, PortalSessionCookieName)
}

// GetPortalSessionFromRequest extracts session token from cookie or Authorization header
func (sm *PortalSessionManager) GetPortalSessionFromRequest(r *http.Request) (string, error) {
	return sm.getSessionFromRequest(r, PortalSessionCookieName)
}
