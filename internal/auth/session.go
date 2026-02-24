// Package auth provides authentication and session management functionality.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/securecookie"
	"golang.org/x/crypto/hkdf"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/utils"
)

const (
	SessionCookieName       = "windshift_session"
	SessionTokenLength      = 32 // 256-bit session tokens
	DefaultSessionDuration  = 24 * time.Hour
	ExtendedSessionDuration = 30 * 24 * time.Hour // 30 days for "remember me"
)

var (
	ErrSessionNotFound = errors.New("session not found")
	ErrSessionExpired  = errors.New("session expired")
	ErrInvalidSession  = errors.New("invalid session")
)

// SessionManager handles secure session management
type SessionManager struct {
	db                database.Database
	secureCookie      *securecookie.SecureCookie
	useSecure         bool     // Whether to set Secure flag on cookies (true for HTTPS, false for HTTP)
	useProxy          bool     // Whether proxy mode is enabled
	additionalProxies []net.IP // Additional proxy IPs beyond private ranges
}

// Session represents an active user session
type Session struct {
	ID        int          `json:"id"`
	UserID    int          `json:"user_id"`
	Token     string       `json:"token"`
	ExpiresAt time.Time    `json:"expires_at"`
	IPAddress string       `json:"ip_address"`
	UserAgent string       `json:"user_agent"`
	IsActive  bool         `json:"is_active"`
	CreatedAt time.Time    `json:"created_at"`
	User      *models.User `json:"user,omitempty"`
}

// NewSessionManager creates a new session manager with secure cookie handling.
// If cookieSecret is non-empty, deterministic cookie keys are derived from it
// so that sessions survive process restarts with the same secret.
func NewSessionManager(db database.Database, useSecureCookies, useProxy bool, additionalProxies []string, cookieSecret string) *SessionManager {
	var hashKey, blockKey []byte
	if cookieSecret != "" {
		hashKey = deriveKey(cookieSecret, "windshift-cookie-hash", 64)
		blockKey = deriveKey(cookieSecret, "windshift-cookie-block", 32)
	} else {
		hashKey = generateSecureKey(64)  // 512-bit key for HMAC
		blockKey = generateSecureKey(32) // 256-bit key for encryption
	}

	// Parse additional proxy IPs (beyond auto-trusted private ranges)
	var additionalIPs []net.IP
	for _, proxyStr := range additionalProxies {
		if ip := net.ParseIP(strings.TrimSpace(proxyStr)); ip != nil {
			additionalIPs = append(additionalIPs, ip)
		}
	}

	return &SessionManager{
		db:                db,
		secureCookie:      securecookie.New(hashKey, blockKey),
		useSecure:         useSecureCookies,
		useProxy:          useProxy,
		additionalProxies: additionalIPs,
	}
}

// generateSecureKey creates a cryptographically secure random key
func generateSecureKey(length int) []byte {
	key := make([]byte, length)
	if _, err := rand.Read(key); err != nil {
		panic(fmt.Sprintf("Failed to generate secure key: %v", err))
	}
	return key
}

// deriveKey deterministically derives a key of the given length from a secret
// using HKDF-SHA256. This allows cookie encryption keys to be stable across
// process restarts when the same secret is provided.
func deriveKey(secret, info string, length int) []byte {
	r := hkdf.New(sha256.New, []byte(secret), nil, []byte(info))
	key := make([]byte, length)
	if _, err := io.ReadFull(r, key); err != nil {
		panic(fmt.Sprintf("failed to derive key: %v", err))
	}
	return key
}

// generateSessionToken creates a cryptographically secure session token
func generateSessionToken() (string, error) {
	bytes := make([]byte, SessionTokenLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate session token: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// CreateSession creates a new session for a user
func (sm *SessionManager) CreateSession(userID int, ipAddress, userAgent string, rememberMe bool) (*Session, error) {
	slog.Debug("creating session", slog.String("component", "sso"), slog.Int("user_id", userID), slog.String("ip_address", ipAddress))

	token, err := generateSessionToken()
	if err != nil {
		return nil, err
	}

	duration := DefaultSessionDuration
	if rememberMe {
		duration = ExtendedSessionDuration
	}
	expiresAt := time.Now().Add(duration)

	// Insert session into database using RETURNING clause (supported by both SQLite 3.35+ and PostgreSQL)
	query := `
		INSERT INTO user_sessions (user_id, session_token, expires_at, ip_address, user_agent, is_active, created_at)
		VALUES (?, ?, ?, ?, ?, true, ?)
		RETURNING id
	`
	var sessionID int64
	err = sm.db.QueryRow(query, userID, token, expiresAt, ipAddress, userAgent, time.Now()).Scan(&sessionID)
	if err != nil {
		slog.Error("session db insert failed", slog.String("component", "sso"), slog.Any("error", err))
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	slog.Debug("session inserted", slog.String("component", "sso"), slog.Int64("session_id", sessionID))

	return &Session{
		ID:        int(sessionID),
		UserID:    userID,
		Token:     token,
		ExpiresAt: expiresAt,
		IPAddress: ipAddress,
		UserAgent: userAgent,
		IsActive:  true,
		CreatedAt: time.Now(),
	}, nil
}

// ValidateSession validates a session token and returns the session with user info
func (sm *SessionManager) ValidateSession(token, ipAddress string) (*Session, error) {
	if token == "" {
		return nil, ErrInvalidSession
	}

	query := `
		SELECT
			s.id, s.user_id, s.session_token, s.expires_at, s.ip_address, s.user_agent, s.is_active, s.created_at,
			u.email, u.username, u.first_name, u.last_name, u.is_active, u.avatar_url, u.requires_password_reset, u.timezone, u.language, u.email_verified, u.created_at, u.updated_at
		FROM user_sessions s
		JOIN users u ON s.user_id = u.id
		WHERE s.session_token = ? AND s.is_active = true
	`

	row := sm.db.QueryRow(query, token)

	session := &Session{User: &models.User{}}
	var avatarURL, timezone, language sql.NullString

	err := row.Scan(
		&session.ID, &session.UserID, &session.Token, &session.ExpiresAt, &session.IPAddress, &session.UserAgent, &session.IsActive, &session.CreatedAt,
		&session.User.Email, &session.User.Username, &session.User.FirstName, &session.User.LastName, &session.User.IsActive, &avatarURL, &session.User.RequiresPasswordReset, &timezone, &language, &session.User.EmailVerified, &session.User.CreatedAt, &session.User.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("failed to validate session: %w", err)
	}

	// Check if session is expired
	if time.Now().After(session.ExpiresAt) {
		// Clean up expired session
		_ = sm.DeleteSession(token)
		return nil, ErrSessionExpired
	}

	// Validate IP address for security (optional - can be configured)
	if ipAddress != "" && session.IPAddress != ipAddress {
		return nil, ErrInvalidSession
	}

	// Set user fields
	session.User.ID = session.UserID
	if avatarURL.Valid {
		session.User.AvatarURL = avatarURL.String
	}
	if timezone.Valid {
		session.User.Timezone = timezone.String
	}
	if language.Valid {
		session.User.Language = language.String
	} else {
		session.User.Language = "en" // default
	}
	session.User.FullName = fmt.Sprintf("%s %s", session.User.FirstName, session.User.LastName)

	return session, nil
}

// DeleteSession invalidates a session
func (sm *SessionManager) DeleteSession(token string) error {
	query := `UPDATE user_sessions SET is_active = false WHERE session_token = ?`
	_, err := sm.db.ExecWrite(query, token)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}
	return nil
}

// DeleteAllUserSessions invalidates all sessions for a user
func (sm *SessionManager) DeleteAllUserSessions(userID int) error {
	query := `UPDATE user_sessions SET is_active = false WHERE user_id = ?`
	_, err := sm.db.ExecWrite(query, userID)
	if err != nil {
		return fmt.Errorf("failed to delete user sessions: %w", err)
	}
	return nil
}

// CleanupExpiredSessions removes expired sessions from the database
func (sm *SessionManager) CleanupExpiredSessions() error {
	query := `UPDATE user_sessions SET is_active = false WHERE expires_at < ? AND is_active = true`
	_, err := sm.db.ExecWrite(query, time.Now())
	if err != nil {
		return fmt.Errorf("failed to cleanup expired sessions: %w", err)
	}
	return nil
}

// RefreshSession extends the expiration time of a session
func (sm *SessionManager) RefreshSession(token string, rememberMe bool) error {
	duration := DefaultSessionDuration
	if rememberMe {
		duration = ExtendedSessionDuration
	}

	newExpiresAt := time.Now().Add(duration)
	query := `UPDATE user_sessions SET expires_at = ? WHERE session_token = ? AND is_active = true`
	_, err := sm.db.ExecWrite(query, newExpiresAt, token)
	if err != nil {
		return fmt.Errorf("failed to refresh session: %w", err)
	}
	return nil
}

// isSecureRequest checks if the request is over HTTPS (either direct or via trusted proxy)
func (sm *SessionManager) isSecureRequest(r *http.Request) bool {
	// Check if request came via HTTPS directly
	if r.TLS != nil {
		slog.Debug("secure request check: TLS present", slog.String("component", "sso"), slog.Bool("result", true))
		return true
	}

	// Check if local HTTPS is enabled
	if sm.useSecure {
		slog.Debug("secure request check: useSecure enabled", slog.String("component", "sso"), slog.Bool("result", true))
		return true
	}

	// Extract direct client IP (not from headers)
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}

	clientIP := net.ParseIP(host)
	if clientIP == nil {
		slog.Debug("secure request check: failed to parse IP", slog.String("component", "sso"), slog.String("remote_addr", host), slog.Bool("result", false))
		return false
	}

	// Only trust X-Forwarded-Proto if request comes from a trusted proxy
	isTrusted := utils.IsTrustedProxy(clientIP, sm.useProxy, sm.additionalProxies)
	proto := r.Header.Get("X-Forwarded-Proto")
	if isTrusted {
		result := proto == "https"
		slog.Debug("secure request check: trusted proxy",
			slog.String("component", "sso"),
			slog.String("ip", clientIP.String()),
			slog.String("x_forwarded_proto", proto),
			slog.Bool("result", result))
		return result
	}

	slog.Debug("secure request check: untrusted proxy",
		slog.String("component", "sso"),
		slog.String("ip", clientIP.String()),
		slog.String("x_forwarded_proto", proto),
		slog.Bool("result", false))
	return false
}

// SetSessionCookie sets a secure session cookie
func (sm *SessionManager) SetSessionCookie(w http.ResponseWriter, r *http.Request, token string, rememberMe bool) error {
	maxAge := int(DefaultSessionDuration.Seconds())
	if rememberMe {
		maxAge = int(ExtendedSessionDuration.Seconds())
	}

	// Create secure cookie value
	encoded, err := sm.secureCookie.Encode(SessionCookieName, token)
	if err != nil {
		return fmt.Errorf("failed to encode session cookie: %w", err)
	}

	// Dynamically determine if cookie should be secure based on request
	useSecure := sm.isSecureRequest(r)

	slog.Debug("setting session cookie",
		slog.String("component", "sso"),
		slog.String("remote_addr", r.RemoteAddr),
		slog.Bool("tls", r.TLS != nil),
		slog.String("x_forwarded_proto", r.Header.Get("X-Forwarded-Proto")),
		slog.Bool("use_secure", useSecure))

	cookie := &http.Cookie{
		Name:     SessionCookieName,
		Value:    encoded,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   useSecure, // Dynamic: true for HTTPS (local or via proxy), false for HTTP
		SameSite: http.SameSiteLaxMode,
	}

	http.SetCookie(w, cookie)
	slog.Debug("session cookie set", slog.String("component", "sso"), slog.String("name", SessionCookieName), slog.Bool("secure", useSecure))
	return nil
}

// GetSessionFromCookie extracts session token from cookie
func (sm *SessionManager) GetSessionFromCookie(r *http.Request) (string, error) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return "", err
	}

	var token string
	err = sm.secureCookie.Decode(SessionCookieName, cookie.Value, &token)
	if err != nil {
		return "", fmt.Errorf("failed to decode session cookie: %w", err)
	}

	return token, nil
}

// ClearSessionCookie removes the session cookie
func (sm *SessionManager) ClearSessionCookie(w http.ResponseWriter, r *http.Request) {
	// Dynamically determine if cookie should be secure based on request
	useSecure := sm.isSecureRequest(r)

	cookie := &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   useSecure, // Dynamic: true for HTTPS (local or via proxy), false for HTTP
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, cookie)
}

// GetSessionFromRequest extracts session token from cookie or Authorization header
func (sm *SessionManager) GetSessionFromRequest(r *http.Request) (string, error) {
	// Try cookie first
	token, err := sm.GetSessionFromCookie(r)
	if err == nil {
		return token, nil
	}

	// Try Authorization header as fallback
	auth := r.Header.Get("Authorization")
	if auth != "" && len(auth) > 7 && auth[:7] == "Bearer " {
		return auth[7:], nil
	}

	return "", errors.New("no session token found")
}

// SetEnrollmentRequired marks a session as requiring passkey enrollment
func (sm *SessionManager) SetEnrollmentRequired(sessionID int, required bool) error {
	query := `UPDATE user_sessions SET enrollment_required = ? WHERE id = ?`
	_, err := sm.db.ExecWrite(query, required, sessionID)
	if err != nil {
		return fmt.Errorf("failed to set enrollment required: %w", err)
	}
	return nil
}

// ClearEnrollmentRequired clears the enrollment required flag for a session
func (sm *SessionManager) ClearEnrollmentRequired(sessionID int) error {
	return sm.SetEnrollmentRequired(sessionID, false)
}

// IsEnrollmentRequired checks if a session requires passkey enrollment
func (sm *SessionManager) IsEnrollmentRequired(sessionID int) (bool, error) {
	var required bool
	query := `SELECT COALESCE(enrollment_required, 0) FROM user_sessions WHERE id = ?`
	err := sm.db.QueryRow(query, sessionID).Scan(&required)
	if err != nil {
		return false, fmt.Errorf("failed to check enrollment required: %w", err)
	}
	return required, nil
}

// ClearEnrollmentRequiredByUserID clears enrollment required for all sessions of a user
// Called after successful passkey enrollment
func (sm *SessionManager) ClearEnrollmentRequiredByUserID(userID int) error {
	query := `UPDATE user_sessions SET enrollment_required = 0 WHERE user_id = ? AND is_active = 1`
	_, err := sm.db.ExecWrite(query, userID)
	if err != nil {
		return fmt.Errorf("failed to clear enrollment required: %w", err)
	}
	return nil
}
