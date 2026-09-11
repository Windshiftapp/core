// Package auth provides authentication and session management functionality.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

const (
	SessionCookieName       = "windshift_session"
	SessionTokenLength      = 32 // 256-bit session tokens
	DefaultSessionDuration  = 24 * time.Hour
	ExtendedSessionDuration = 30 * 24 * time.Hour // 30 days for "remember me"
	// DefaultSessionValidationCacheTTL bounds how long another instance's
	// session or user-state mutation can remain invisible to this process.
	DefaultSessionValidationCacheTTL = 5 * time.Second
)

var (
	ErrSessionNotFound = errors.New("session not found")
	ErrSessionExpired  = errors.New("session expired")
	ErrInvalidSession  = errors.New("invalid session")
)

const sessionTokenHashPrefix = "sha256:"

const (
	AuthPendingEnrollment = "passkey_enrollment"
	// #nosec G101 -- authentication workflow state, not a credential.
	AuthPendingPasskeyVerification = "passkey_verification"

	pendingEnrollmentDuration   = 30 * time.Minute
	pendingVerificationDuration = 5 * time.Minute
)

func hashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return sessionTokenHashPrefix + hex.EncodeToString(sum[:])
}

// SessionManager handles secure session management
type SessionManager struct {
	cookieManager
	db                database.Database
	opaqueKey         []byte
	sessionValidation *sessionValidator
	closeOnce         sync.Once
	closeErr          error
	// ipBinding is the resolved SESSION_IP_BINDING mode (config.SessionIPBinding*)
	// that session validation applies to a client-IP change. An unknown or
	// zero value is treated as strict so managers built without config.Load
	// fail closed.
	ipBinding string
}

// Close stops the session validation cache's background worker without closing
// the shared database. It is safe with caching disabled and on repeated calls.
func (sm *SessionManager) Close() error {
	sm.closeOnce.Do(func() {
		if sm.sessionValidation != nil && sm.sessionValidation.cache != nil {
			sm.closeErr = sm.sessionValidation.cache.Close()
		}
	})
	return sm.closeErr
}

// Session represents an active user session
type Session struct {
	ID                 int          `json:"id"`
	UserID             int          `json:"user_id"`
	Token              string       `json:"-"`
	ExpiresAt          time.Time    `json:"expires_at"`
	IPAddress          string       `json:"ip_address"`
	UserAgent          string       `json:"user_agent"`
	IsActive           bool         `json:"is_active"`
	EnrollmentRequired bool         `json:"enrollment_required"`
	AuthPendingType    string       `json:"-"`
	CreatedAt          time.Time    `json:"created_at"`
	User               *models.User `json:"user,omitempty"`
}

// NewSessionManager creates a new session manager with secure cookie handling.
// If cookieSecret is non-empty, deterministic cookie keys are derived from it
// so that sessions survive process restarts with the same secret.
// ipBinding is the resolved SESSION_IP_BINDING mode.
// last review: ser, 210426
func NewSessionManager(db database.Database, useSecureCookies, useProxy bool, additionalProxies []string, cookieSecret, ipBinding string) *SessionManager {
	return NewSessionManagerWithValidationCacheTTL(
		db,
		useSecureCookies,
		useProxy,
		additionalProxies,
		cookieSecret,
		ipBinding,
		DefaultSessionValidationCacheTTL,
	)
}

// NewSessionManagerWithValidationCacheTTL creates a session manager with a
// bounded local validation cache. A non-positive TTL disables retained cache
// entries while preserving in-flight request coalescing.
func NewSessionManagerWithValidationCacheTTL(db database.Database, useSecureCookies, useProxy bool, additionalProxies []string, cookieSecret, ipBinding string, validationCacheTTL time.Duration, cacheSizeMB ...int) *SessionManager {
	return newSessionManagerWithValidationCache(
		db,
		useSecureCookies,
		useProxy,
		additionalProxies,
		cookieSecret,
		ipBinding,
		validationCacheTTL,
		"session_validation",
		cacheSizeMB...,
	)
}

// NewSessionManagerWithNamedValidationCacheTTL creates a session manager whose
// validation cache has an explicit diagnostics name. The SSH server uses it so
// the HTTP and SSH allocations remain independently visible.
func NewSessionManagerWithNamedValidationCacheTTL(db database.Database, useSecureCookies, useProxy bool, additionalProxies []string, cookieSecret, ipBinding string, validationCacheTTL time.Duration, cacheName string, cacheSizeMB int) *SessionManager {
	return newSessionManagerWithValidationCache(
		db,
		useSecureCookies,
		useProxy,
		additionalProxies,
		cookieSecret,
		ipBinding,
		validationCacheTTL,
		cacheName,
		cacheSizeMB,
	)
}

func newSessionManagerWithValidationCache(db database.Database, useSecureCookies, useProxy bool, additionalProxies []string, cookieSecret, ipBinding string, validationCacheTTL time.Duration, cacheName string, cacheSizeMB ...int) *SessionManager {
	var opaqueKey []byte
	if cookieSecret != "" {
		opaqueKey = deriveKey(cookieSecret, "windshift-auth-opaque-values", 32)
	} else {
		opaqueKey = generateSecureKey(32)
	}
	return &SessionManager{
		cookieManager: newCookieManager(useSecureCookies, useProxy, additionalProxies, cookieSecret,
			"windshift-cookie-hash", "windshift-cookie-block"),
		db:                db,
		opaqueKey:         opaqueKey,
		sessionValidation: newSessionValidator(validationCacheTTL, cacheName, cacheSizeMB...),
		ipBinding:         ipBinding,
	}
}

// DeriveOpaqueValue returns a stable, non-reversible 256-bit value scoped to a
// purpose. It is used for public auth-flow decoys that must remain consistent
// across requests without exposing whether they correspond to stored data.
func (sm *SessionManager) DeriveOpaqueValue(purpose, value string) []byte {
	mac := hmac.New(sha256.New, sm.opaqueKey)
	_, _ = mac.Write([]byte(purpose))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}

// CreateSession creates a new session for a user
// last review: ser, 210426, NOTE: inline sql again
func (sm *SessionManager) CreateSession(userID int, ipAddress, userAgent string, rememberMe bool) (*Session, error) {
	// Normalise to host-only so ValidateSession (which compares against
	// getClientIP's port-stripped result) can match. Callers sometimes pass
	// host:port — notably the SSH TUI handler, which uses
	// ssh.Session.RemoteAddr().String() — and the validator would otherwise
	// reject every subsequent request from a working session.
	if host, _, err := net.SplitHostPort(ipAddress); err == nil {
		ipAddress = host
	}

	token, err := generateSessionToken()
	if err != nil {
		return nil, err
	}

	duration := DefaultSessionDuration
	if rememberMe {
		duration = ExtendedSessionDuration
	}
	expiresAt := time.Now().Add(duration)
	slog.Debug("creating session",
		slog.String("component", "auth"),
		slog.Int("user_id", userID),
		slog.String("ip_address", ipAddress),
		slog.Bool("remember_me", rememberMe),
		slog.Duration("duration", duration),
	)

	// Insert session into database using RETURNING clause (supported by both SQLite 3.35+ and PostgreSQL).
	// Store only a hash of the bearer token; the plaintext token is returned to
	// the caller once and lives in the secure cookie. Validation keeps a legacy
	// plaintext fallback so existing sessions survive the upgrade.
	query := `
		INSERT INTO user_sessions (user_id, session_token, expires_at, ip_address, user_agent, is_active, created_at)
		VALUES (?, ?, ?, ?, ?, true, ?)
		RETURNING id
	`
	var sessionID int64
	err = sm.db.QueryRow(query, userID, hashSessionToken(token), expiresAt, ipAddress, userAgent, time.Now()).Scan(&sessionID)
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

// DeleteSession invalidates a session
// last review: ser, 210426, NOTE: all the following still in use
func (sm *SessionManager) DeleteSession(token string) error {
	query := `UPDATE user_sessions SET is_active = false WHERE session_token IN (?, ?)`
	_, err := sm.db.ExecWrite(query, hashSessionToken(token), token)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}
	sm.invalidateSessionValidationToken(token)
	return nil
}

// DeleteAllUserSessions invalidates all sessions for a user
func (sm *SessionManager) DeleteAllUserSessions(userID int) error {
	query := `UPDATE user_sessions SET is_active = false WHERE user_id = ?`
	_, err := sm.db.ExecWrite(query, userID)
	if err != nil {
		return fmt.Errorf("failed to delete user sessions: %w", err)
	}
	sm.InvalidateUserSessionValidation(userID)
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

	now := time.Now()
	newExpiresAt := now.Add(duration)
	query := `UPDATE user_sessions SET expires_at = ? WHERE session_token IN (?, ?) AND is_active = true AND expires_at > ?`
	result, err := sm.db.ExecWrite(query, newExpiresAt, hashSessionToken(token), token, now)
	if err != nil {
		return fmt.Errorf("failed to refresh session: %w", err)
	}
	sm.invalidateSessionValidationToken(token)
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to inspect refreshed session: %w", err)
	}
	if rowsAffected != 1 {
		return ErrInvalidSession
	}
	return nil
}

// UpdateSessionIP rebinds a session to a new client IP. It is used by the log
// SESSION_IP_BINDING mode, where a session that moves between networks is
// followed rather than rejected. Sessions are stored as token digests with a
// legacy plaintext fallback, so the predicate matches both forms — a
// plaintext-only predicate would match zero rows for every current session.
func (sm *SessionManager) UpdateSessionIP(token, ipAddress string) error {
	query := `UPDATE user_sessions SET ip_address = ? WHERE session_token IN (?, ?) AND is_active = true`
	_, err := sm.db.ExecWrite(query, ipAddress, hashSessionToken(token), token)
	if err != nil {
		return fmt.Errorf("failed to update session IP: %w", err)
	}
	// The cached snapshot still carries the previous IP; drop it so the next
	// request revalidates against the rebound row instead of rebinding again.
	sm.invalidateSessionValidationToken(token)
	return nil
}

// SetSessionCookie sets a secure session cookie
func (sm *SessionManager) SetSessionCookie(w http.ResponseWriter, r *http.Request, token string, rememberMe bool) error {
	maxAge := int(DefaultSessionDuration.Seconds())
	if rememberMe {
		maxAge = int(ExtendedSessionDuration.Seconds())
	}
	return sm.setSessionCookie(w, r, SessionCookieName, token, maxAge)
}

// EncodeSessionCookieValue returns the securecookie-encoded value for a session
// token under the canonical session cookie name. The desktop/native SSO flow
// (WI-446) hands this back to the app, which writes it verbatim into its
// webview's cookie store — the app can't encode it itself, having no server
// secret. The value is bound to SessionCookieName, so it must be written under
// exactly that cookie name to decode on subsequent requests.
func (sm *SessionManager) EncodeSessionCookieValue(token string) (string, error) {
	return sm.secureCookie.Encode(SessionCookieName, token)
}

// GetSessionFromCookie extracts session token from cookie
func (sm *SessionManager) GetSessionFromCookie(r *http.Request) (string, error) {
	return sm.getSessionFromCookie(r, SessionCookieName)
}

// ClearSessionCookie removes the session cookie
func (sm *SessionManager) ClearSessionCookie(w http.ResponseWriter, r *http.Request) {
	sm.clearSessionCookie(w, r, SessionCookieName)
}

// GetSessionFromRequest extracts a session token from the session cookie.
func (sm *SessionManager) GetSessionFromRequest(r *http.Request) (string, error) {
	return sm.getSessionFromRequest(r, SessionCookieName)
}

// SetAuthPending marks a password-verified session as narrowly scoped pending
// authentication. Enrollment sessions may register a first passkey; verification
// sessions may only complete an assertion with an already enrolled passkey.
func (sm *SessionManager) SetAuthPending(sessionID int, pendingType string) error {
	if pendingType != AuthPendingEnrollment && pendingType != AuthPendingPasskeyVerification {
		return fmt.Errorf("invalid auth pending type %q", pendingType)
	}
	duration := pendingEnrollmentDuration
	if pendingType == AuthPendingPasskeyVerification {
		duration = pendingVerificationDuration
	}
	query := `UPDATE user_sessions SET enrollment_required = true, auth_pending_type = ?, expires_at = ? WHERE id = ?`
	_, err := sm.db.ExecWrite(query, pendingType, time.Now().Add(duration), sessionID)
	if err != nil {
		return fmt.Errorf("failed to set auth pending state: %w", err)
	}
	sm.invalidateSessionValidationID(sessionID)
	return nil
}

// SetEnrollmentRequired is retained for callers creating enrollment-only sessions.
func (sm *SessionManager) SetEnrollmentRequired(sessionID int, required bool) error {
	if required {
		return sm.SetAuthPending(sessionID, AuthPendingEnrollment)
	}
	return sm.ClearEnrollmentRequired(sessionID)
}

// ClearEnrollmentRequired elevates a pending session after its required WebAuthn ceremony.
func (sm *SessionManager) ClearEnrollmentRequired(sessionID int) error {
	query := `UPDATE user_sessions SET enrollment_required = false, auth_pending_type = NULL, expires_at = ? WHERE id = ?`
	_, err := sm.db.ExecWrite(query, time.Now().Add(DefaultSessionDuration), sessionID)
	if err != nil {
		return fmt.Errorf("failed to clear auth pending state: %w", err)
	}
	sm.invalidateSessionValidationID(sessionID)
	return nil
}

// IsEnrollmentRequired checks if a session requires passkey enrollment
func (sm *SessionManager) IsEnrollmentRequired(sessionID int) (bool, error) {
	var required bool
	query := `SELECT COALESCE(enrollment_required, false) FROM user_sessions WHERE id = ?`
	err := sm.db.QueryRow(query, sessionID).Scan(&required)
	if err != nil {
		return false, fmt.Errorf("failed to check enrollment required: %w", err)
	}
	return required, nil
}

// ClearEnrollmentRequiredByUserID elevates enrollment-only sessions after a
// successful first-passkey registration. It deliberately does not elevate
// password+passkey verification sessions, which still require an assertion.
func (sm *SessionManager) ClearEnrollmentRequiredByUserID(userID int) error {
	query := `
		UPDATE user_sessions SET enrollment_required = false, auth_pending_type = NULL, expires_at = ?
		WHERE user_id = ? AND is_active = true
		AND (auth_pending_type = ? OR (auth_pending_type IS NULL AND enrollment_required = true))
	`
	_, err := sm.db.ExecWrite(query, time.Now().Add(DefaultSessionDuration), userID, AuthPendingEnrollment)
	if err != nil {
		return fmt.Errorf("failed to clear enrollment required: %w", err)
	}
	sm.InvalidateUserSessionValidation(userID)
	return nil
}
