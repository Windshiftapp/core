package webauthn

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

// SessionStore handles storage of WebAuthn session data
type SessionStore struct {
	db Database
}

// Database interface for session storage
type Database interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	QueryRow(query string, args ...interface{}) *sql.Row
	Query(query string, args ...interface{}) (*sql.Rows, error)
}

// NewSessionStore creates a new session store
func NewSessionStore(db Database) *SessionStore {
	return &SessionStore{db: db}
}

// SessionData represents a stored WebAuthn session
type SessionData struct {
	ID          string    `json:"id"`
	UserID      *int      `json:"user_id,omitempty"` // Nullable for passwordless
	Challenge   string    `json:"challenge"`
	SessionData string    `json:"session_data"` // JSON serialized webauthn.SessionData
	SessionType string    `json:"session_type"` // 'registration' or 'authentication'
	ExpiresAt   time.Time `json:"expires_at"`
	CreatedAt   time.Time `json:"created_at"`
}

// generateSessionID creates a secure random session ID
func generateSessionID() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate session ID: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

// saveSession stores session data with the given type and user ID.
func (s *SessionStore) saveSession(userID any, sessionData *webauthn.SessionData, sessionType string) (string, error) {
	sessionID, err := generateSessionID()
	if err != nil {
		return "", err
	}

	// Serialize session data to JSON
	sessionJSON, err := json.Marshal(sessionData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal session data: %w", err)
	}

	// Store in database with 5-minute expiration
	expiresAt := time.Now().Add(5 * time.Minute)
	_, err = s.db.Exec(`
		INSERT INTO webauthn_sessions (id, user_id, challenge, session_data, session_type, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, sessionID, userID, sessionData.Challenge, string(sessionJSON), sessionType, expiresAt, time.Now())

	if err != nil {
		return "", fmt.Errorf("failed to save %s session: %w", sessionType, err)
	}

	// Clean up expired sessions occasionally
	s.cleanupExpiredSessions()

	return sessionID, nil
}

// SaveRegistrationSession stores registration session data
func (s *SessionStore) SaveRegistrationSession(userID int, sessionData *webauthn.SessionData) (string, error) {
	return s.saveSession(userID, sessionData, "registration")
}

// SaveAuthenticationSession stores authentication session data
func (s *SessionStore) SaveAuthenticationSession(userID *int, sessionData *webauthn.SessionData) (string, error) {
	return s.saveSession(userID, sessionData, "authentication")
}

// getSession retrieves and deletes (one-time use) a session that matches the
// given id, session_type, and — when userID is non-nil — user_id. Filtering on
// all three columns prevents cross-type and cross-user reuse of a session id.
func (s *SessionStore) getSession(sessionID, sessionType string, userID *int) (*webauthn.SessionData, error) {
	var sessionJSON string
	var expiresAt time.Time

	query := `
		SELECT session_data, expires_at
		FROM webauthn_sessions
		WHERE id = ? AND session_type = ?`
	args := []any{sessionID, sessionType}
	if userID != nil {
		query += ` AND user_id = ?`
		args = append(args, *userID)
	}

	err := s.db.QueryRow(query, args...).Scan(&sessionJSON, &expiresAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("session not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	deleteQuery := `DELETE FROM webauthn_sessions WHERE id = ? AND session_type = ?`
	deleteArgs := []any{sessionID, sessionType}
	if userID != nil {
		deleteQuery += ` AND user_id = ?`
		deleteArgs = append(deleteArgs, *userID)
	}

	if time.Now().After(expiresAt) {
		_, _ = s.db.Exec(deleteQuery, deleteArgs...)
		return nil, fmt.Errorf("session expired")
	}

	if _, err := s.db.Exec(deleteQuery, deleteArgs...); err != nil {
		// Session was retrieved successfully; cleanup failure is non-fatal.
		slog.Warn("failed to delete webauthn session after retrieval", slog.Any("error", err), slog.String("session_id", sessionID))
	}

	var sessionData webauthn.SessionData
	if err := json.Unmarshal([]byte(sessionJSON), &sessionData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session data: %w", err)
	}

	return &sessionData, nil
}

// GetRegistrationSession retrieves a registration session bound to the given user.
func (s *SessionStore) GetRegistrationSession(sessionID string, userID int) (*webauthn.SessionData, error) {
	return s.getSession(sessionID, "registration", &userID)
}

// GetAuthenticationSession retrieves an authentication session. Authentication
// sessions may be created without a user id (passwordless flows), so we filter
// on session_type only.
func (s *SessionStore) GetAuthenticationSession(sessionID string) (*webauthn.SessionData, error) {
	return s.getSession(sessionID, "authentication", nil)
}

// cleanupExpiredSessions removes expired sessions from the database
// This is called occasionally to prevent buildup of old sessions
func (s *SessionStore) cleanupExpiredSessions() {
	// Only run cleanup 1% of the time to avoid performance impact
	if time.Now().Unix()%100 != 0 {
		return
	}

	// Run cleanup in background
	go func() {
		_, err := s.db.Exec(`
			DELETE FROM webauthn_sessions
			WHERE expires_at < ?
		`, time.Now())
		if err != nil {
			slog.Warn("failed to cleanup expired webauthn sessions", slog.Any("error", err))
		}
	}()
}
