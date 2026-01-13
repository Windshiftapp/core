package webauthn

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
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

// SaveRegistrationSession stores registration session data
func (s *SessionStore) SaveRegistrationSession(userID int, sessionData *webauthn.SessionData) (string, error) {
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
		VALUES (?, ?, ?, ?, 'registration', ?, ?)
	`, sessionID, userID, sessionData.Challenge, string(sessionJSON), expiresAt, time.Now())

	if err != nil {
		return "", fmt.Errorf("failed to save registration session: %w", err)
	}

	// Clean up expired sessions occasionally
	s.cleanupExpiredSessions()

	return sessionID, nil
}

// SaveAuthenticationSession stores authentication session data
func (s *SessionStore) SaveAuthenticationSession(userID *int, sessionData *webauthn.SessionData) (string, error) {
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
		VALUES (?, ?, ?, ?, 'authentication', ?, ?)
	`, sessionID, userID, sessionData.Challenge, string(sessionJSON), expiresAt, time.Now())

	if err != nil {
		return "", fmt.Errorf("failed to save authentication session: %w", err)
	}

	// Clean up expired sessions occasionally
	s.cleanupExpiredSessions()

	return sessionID, nil
}

// GetSession retrieves and deletes a session by ID (one-time use)
func (s *SessionStore) GetSession(sessionID string) (*webauthn.SessionData, error) {
	var sessionJSON string
	var expiresAt time.Time

	// Retrieve session
	err := s.db.QueryRow(`
		SELECT session_data, expires_at
		FROM webauthn_sessions
		WHERE id = ?
	`, sessionID).Scan(&sessionJSON, &expiresAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	// Check if expired
	if time.Now().After(expiresAt) {
		// Delete expired session
		s.db.Exec("DELETE FROM webauthn_sessions WHERE id = ?", sessionID)
		return nil, fmt.Errorf("session expired")
	}

	// Delete session after retrieval (one-time use)
	_, err = s.db.Exec("DELETE FROM webauthn_sessions WHERE id = ?", sessionID)
	if err != nil {
		// Log but don't fail - session was retrieved successfully
		fmt.Printf("Warning: failed to delete session after retrieval: %v\n", err)
	}

	// Deserialize session data
	var sessionData webauthn.SessionData
	if err := json.Unmarshal([]byte(sessionJSON), &sessionData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session data: %w", err)
	}

	return &sessionData, nil
}

// GetRegistrationSession retrieves a registration session
func (s *SessionStore) GetRegistrationSession(sessionID string) (*webauthn.SessionData, error) {
	return s.GetSession(sessionID)
}

// GetAuthenticationSession retrieves an authentication session
func (s *SessionStore) GetAuthenticationSession(sessionID string) (*webauthn.SessionData, error) {
	return s.GetSession(sessionID)
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
			fmt.Printf("Warning: failed to cleanup expired sessions: %v\n", err)
		}
	}()
}

// GetUserSessions retrieves all active sessions for a user (for debugging/admin)
func (s *SessionStore) GetUserSessions(userID int) ([]SessionData, error) {
	rows, err := s.db.Query(`
		SELECT id, user_id, challenge, session_data, session_type, expires_at, created_at
		FROM webauthn_sessions
		WHERE user_id = ? AND expires_at > ?
		ORDER BY created_at DESC
	`, userID, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to get user sessions: %w", err)
	}
	defer rows.Close()

	var sessions []SessionData
	for rows.Next() {
		var session SessionData
		var userIDPtr *int
		err := rows.Scan(&session.ID, &userIDPtr, &session.Challenge,
			&session.SessionData, &session.SessionType, &session.ExpiresAt, &session.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}
		if userIDPtr != nil {
			session.UserID = userIDPtr
		}
		sessions = append(sessions, session)
	}

	return sessions, nil
}

// DeleteUserSessions removes all sessions for a user
func (s *SessionStore) DeleteUserSessions(userID int) error {
	_, err := s.db.Exec(`
		DELETE FROM webauthn_sessions
		WHERE user_id = ?
	`, userID)
	if err != nil {
		return fmt.Errorf("failed to delete user sessions: %w", err)
	}
	return nil
}