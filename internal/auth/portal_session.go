package auth

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/securecookie"
	"windshift/internal/database"
)

const (
	PortalSessionCookieName    = "windshift_portal_session"
	PortalSessionTokenLength   = 32 // 256-bit session tokens
	PortalSessionDuration      = 7 * 24 * time.Hour // 7 days
)

var (
	ErrPortalSessionNotFound = errors.New("portal session not found")
	ErrPortalSessionExpired  = errors.New("portal session expired")
	ErrPortalSessionInvalid  = errors.New("invalid portal session")
)

// PortalCustomer represents a portal customer from the database
type PortalCustomer struct {
	ID                     int       `json:"id"`
	Name                   string    `json:"name"`
	Email                  string    `json:"email"`
	Phone                  string    `json:"phone,omitempty"`
	CustomerOrganisationID *int      `json:"customer_organisation_id,omitempty"`
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
	db                database.Database
	secureCookie      *securecookie.SecureCookie
	useSecure         bool     // Whether to set Secure flag on cookies (true for HTTPS, false for HTTP)
	useProxy          bool     // Whether proxy mode is enabled
	additionalProxies []net.IP // Additional proxy IPs beyond private ranges
}

// NewPortalSessionManager creates a new portal session manager with secure cookie handling
func NewPortalSessionManager(db database.Database, useSecureCookies bool, useProxy bool, additionalProxies []string) *PortalSessionManager {
	// Generate secure cookie keys (in production, these should be from config/env)
	hashKey := generateSecureKey(64)  // 512-bit key for HMAC
	blockKey := generateSecureKey(32) // 256-bit key for encryption

	// Parse additional proxy IPs (beyond auto-trusted private ranges)
	var additionalIPs []net.IP
	for _, proxyStr := range additionalProxies {
		if ip := net.ParseIP(strings.TrimSpace(proxyStr)); ip != nil {
			additionalIPs = append(additionalIPs, ip)
		}
	}

	return &PortalSessionManager{
		db:                db,
		secureCookie:      securecookie.New(hashKey, blockKey),
		useSecure:         useSecureCookies,
		useProxy:          useProxy,
		additionalProxies: additionalIPs,
	}
}

// generatePortalSessionToken creates a cryptographically secure session token
func generatePortalSessionToken() (string, error) {
	bytes := make([]byte, PortalSessionTokenLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate portal session token: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// CreatePortalSession creates a new session for a portal customer
func (sm *PortalSessionManager) CreatePortalSession(portalCustomerID int, ipAddress, userAgent string) (*PortalSession, error) {
	slog.Debug("creating portal session", slog.String("component", "portal_auth"), slog.Int("portal_customer_id", portalCustomerID), slog.String("ip_address", ipAddress))

	token, err := generatePortalSessionToken()
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
func (sm *PortalSessionManager) ValidatePortalSession(token string) (*PortalSession, error) {
	if token == "" {
		return nil, ErrPortalSessionInvalid
	}

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
func (sm *PortalSessionManager) DeletePortalSession(token string) error {
	query := `UPDATE portal_customer_sessions SET is_active = false WHERE session_token = ?`
	_, err := sm.db.ExecWrite(query, token)
	if err != nil {
		return fmt.Errorf("failed to delete portal session: %w", err)
	}
	return nil
}

// DeleteAllCustomerSessions invalidates all sessions for a portal customer
func (sm *PortalSessionManager) DeleteAllCustomerSessions(portalCustomerID int) error {
	query := `UPDATE portal_customer_sessions SET is_active = false WHERE portal_customer_id = ?`
	_, err := sm.db.ExecWrite(query, portalCustomerID)
	if err != nil {
		return fmt.Errorf("failed to delete customer sessions: %w", err)
	}
	return nil
}

// CleanupExpiredSessions removes expired sessions from the database
func (sm *PortalSessionManager) CleanupExpiredSessions() error {
	query := `UPDATE portal_customer_sessions SET is_active = false WHERE expires_at < ? AND is_active = true`
	_, err := sm.db.ExecWrite(query, time.Now())
	if err != nil {
		return fmt.Errorf("failed to cleanup expired portal sessions: %w", err)
	}
	return nil
}

// isSecureRequest checks if the request is over HTTPS (either direct or via trusted proxy)
func (sm *PortalSessionManager) isSecureRequest(r *http.Request) bool {
	// Check if request came via HTTPS directly
	if r.TLS != nil {
		return true
	}

	// Check if local HTTPS is enabled
	if sm.useSecure {
		return true
	}

	// Extract direct client IP (not from headers)
	remoteAddr := r.RemoteAddr
	if colonIndex := strings.LastIndex(remoteAddr, ":"); colonIndex != -1 {
		remoteAddr = remoteAddr[:colonIndex]
	}

	clientIP := net.ParseIP(remoteAddr)
	if clientIP == nil {
		return false
	}

	// Only trust X-Forwarded-Proto if request comes from a trusted proxy
	isTrusted := sm.isTrustedProxy(clientIP)
	proto := r.Header.Get("X-Forwarded-Proto")
	if isTrusted {
		return proto == "https"
	}

	return false
}

// isTrustedProxy checks if an IP is a trusted proxy (private IP or in additional list)
func (sm *PortalSessionManager) isTrustedProxy(ip net.IP) bool {
	if !sm.useProxy {
		return false // Proxy mode disabled - trust nothing
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return true
	}
	for _, trustedIP := range sm.additionalProxies {
		if ip.Equal(trustedIP) {
			return true
		}
	}
	return false
}

// SetPortalSessionCookie sets a secure session cookie
func (sm *PortalSessionManager) SetPortalSessionCookie(w http.ResponseWriter, r *http.Request, token string) error {
	maxAge := int(PortalSessionDuration.Seconds())

	// Create secure cookie value
	encoded, err := sm.secureCookie.Encode(PortalSessionCookieName, token)
	if err != nil {
		return fmt.Errorf("failed to encode portal session cookie: %w", err)
	}

	// Dynamically determine if cookie should be secure based on request
	useSecure := sm.isSecureRequest(r)

	slog.Debug("setting portal session cookie",
		slog.String("component", "portal_auth"),
		slog.String("remote_addr", r.RemoteAddr),
		slog.Bool("tls", r.TLS != nil),
		slog.String("x_forwarded_proto", r.Header.Get("X-Forwarded-Proto")),
		slog.Bool("use_secure", useSecure))

	cookie := &http.Cookie{
		Name:     PortalSessionCookieName,
		Value:    encoded,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   useSecure, // Dynamic: true for HTTPS (local or via proxy), false for HTTP
		SameSite: http.SameSiteLaxMode,
	}

	http.SetCookie(w, cookie)
	slog.Debug("portal session cookie set", slog.String("component", "portal_auth"), slog.String("name", PortalSessionCookieName), slog.Bool("secure", useSecure))
	return nil
}

// GetPortalSessionFromCookie extracts session token from cookie
func (sm *PortalSessionManager) GetPortalSessionFromCookie(r *http.Request) (string, error) {
	cookie, err := r.Cookie(PortalSessionCookieName)
	if err != nil {
		return "", err
	}

	var token string
	err = sm.secureCookie.Decode(PortalSessionCookieName, cookie.Value, &token)
	if err != nil {
		return "", fmt.Errorf("failed to decode portal session cookie: %w", err)
	}

	return token, nil
}

// ClearPortalSessionCookie removes the session cookie
func (sm *PortalSessionManager) ClearPortalSessionCookie(w http.ResponseWriter, r *http.Request) {
	// Dynamically determine if cookie should be secure based on request
	useSecure := sm.isSecureRequest(r)

	cookie := &http.Cookie{
		Name:     PortalSessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   useSecure,
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, cookie)
}

// GetPortalSessionFromRequest extracts session token from cookie or Authorization header
func (sm *PortalSessionManager) GetPortalSessionFromRequest(r *http.Request) (string, error) {
	// Try cookie first
	token, err := sm.GetPortalSessionFromCookie(r)
	if err == nil {
		return token, nil
	}

	// Try Authorization header as fallback
	auth := r.Header.Get("Authorization")
	if auth != "" && len(auth) > 7 && auth[:7] == "Bearer " {
		return auth[7:], nil
	}

	return "", errors.New("no portal session token found")
}
