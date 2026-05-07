// Package auth provides authentication and session management functionality.
package auth

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
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
	cookieManager
	db database.Database
}

// Session represents an active user session
type Session struct {
	ID        int          `json:"id"`
	UserID    int          `json:"user_id"`
	Token     string       `json:"-"`
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
// last review: ser, 210426
func NewSessionManager(db database.Database, useSecureCookies, useProxy bool, additionalProxies []string, cookieSecret string) *SessionManager {
	return &SessionManager{
		cookieManager: newCookieManager(useSecureCookies, useProxy, additionalProxies, cookieSecret,
			"windshift-cookie-hash", "windshift-cookie-block"),
		db: db,
	}
}

// CreateSession creates a new session for a user
// last review: ser, 210426, NOTE: inline sql again
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

	// Validate IP address for security. Sessions store the client IP at
	// creation; subsequent validations must match.
	//
	// Failure modes:
	//   - session.IPAddress empty: legacy session created before IP was
	//     recorded, or a code path that didn't populate it. We can't bind
	//     retroactively — log loudly and accept so existing logged-in users
	//     don't get kicked out, but this is a signal the operator should
	//     investigate.
	//   - request ipAddress empty: proxy misconfig (X-Forwarded-For
	//     stripping, untrusted proxy, missing RemoteAddr). Previously this
	//     skipped the check; that turned a broken proxy into a stealth
	//     downgrade of every session-bound user. Fail closed.
	//   - mismatch: existing behavior — reject.
	switch {
	case session.IPAddress == "":
		slog.Warn("session has no recorded IP, skipping bind check",
			slog.Int("user_id", session.UserID),
			slog.Int("session_id", session.ID))
	case ipAddress == "":
		slog.Warn("request has no client IP, rejecting IP-bound session",
			slog.Int("user_id", session.UserID),
			slog.String("session_ip", session.IPAddress))
		return nil, ErrInvalidSession
	case session.IPAddress != ipAddress:
		slog.Warn("session IP mismatch",
			slog.Int("user_id", session.UserID),
			slog.String("session_ip", session.IPAddress),
			slog.String("request_ip", ipAddress))
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
// last review: ser, 210426, NOTE: all the following still in use
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

// SetSessionCookie sets a secure session cookie
func (sm *SessionManager) SetSessionCookie(w http.ResponseWriter, r *http.Request, token string, rememberMe bool) error {
	maxAge := int(DefaultSessionDuration.Seconds())
	if rememberMe {
		maxAge = int(ExtendedSessionDuration.Seconds())
	}
	return sm.setSessionCookie(w, r, SessionCookieName, token, maxAge)
}

// GetSessionFromCookie extracts session token from cookie
func (sm *SessionManager) GetSessionFromCookie(r *http.Request) (string, error) {
	return sm.getSessionFromCookie(r, SessionCookieName)
}

// ClearSessionCookie removes the session cookie
func (sm *SessionManager) ClearSessionCookie(w http.ResponseWriter, r *http.Request) {
	sm.clearSessionCookie(w, r, SessionCookieName)
}

// GetSessionFromRequest extracts session token from cookie or Authorization header
func (sm *SessionManager) GetSessionFromRequest(r *http.Request) (string, error) {
	return sm.getSessionFromRequest(r, SessionCookieName)
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
	query := `SELECT COALESCE(enrollment_required, false) FROM user_sessions WHERE id = ?`
	err := sm.db.QueryRow(query, sessionID).Scan(&required)
	if err != nil {
		return false, fmt.Errorf("failed to check enrollment required: %w", err)
	}
	return required, nil
}

// ClearEnrollmentRequiredByUserID clears enrollment required for all sessions of a user
// Called after successful passkey enrollment
func (sm *SessionManager) ClearEnrollmentRequiredByUserID(userID int) error {
	query := `UPDATE user_sessions SET enrollment_required = false WHERE user_id = ? AND is_active = true`
	_, err := sm.db.ExecWrite(query, userID)
	if err != nil {
		return fmt.Errorf("failed to clear enrollment required: %w", err)
	}
	return nil
}
