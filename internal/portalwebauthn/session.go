package portalwebauthn

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

// SessionStore persists challenge sessions for portal WebAuthn flows.
type SessionStore struct {
	db Database
}

// NewSessionStore creates a new portal session store.
func NewSessionStore(db Database) *SessionStore { return &SessionStore{db: db} }

func generateSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate session ID: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (s *SessionStore) saveSession(portalCustomerID any, sessionData *webauthn.SessionData, sessionType string) (string, error) {
	id, err := generateSessionID()
	if err != nil {
		return "", err
	}
	sessionJSON, err := json.Marshal(sessionData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal session data: %w", err)
	}
	expiresAt := time.Now().Add(5 * time.Minute)
	_, err = s.db.Exec(`
		INSERT INTO portal_webauthn_sessions (id, portal_customer_id, challenge, session_data, session_type, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, id, portalCustomerID, sessionData.Challenge, string(sessionJSON), sessionType, expiresAt, time.Now())
	if err != nil {
		return "", fmt.Errorf("failed to save %s session: %w", sessionType, err)
	}
	s.cleanupExpiredSessions()
	return id, nil
}

// SaveRegistrationSession stores registration challenge data bound to a
// specific portal customer.
func (s *SessionStore) SaveRegistrationSession(portalCustomerID int, sessionData *webauthn.SessionData) (string, error) {
	return s.saveSession(portalCustomerID, sessionData, "registration")
}

// SaveAuthenticationSession stores an authentication challenge. portalCustomerID
// is nil for discoverable (passwordless) login; the subject is resolved from
// the userHandle returned by the authenticator at finish time.
func (s *SessionStore) SaveAuthenticationSession(portalCustomerID *int, sessionData *webauthn.SessionData) (string, error) {
	return s.saveSession(portalCustomerID, sessionData, "authentication")
}

// One-time-use retrieval. portalCustomerID filter is applied only when non-nil.
func (s *SessionStore) getSession(sessionID, sessionType string, portalCustomerID *int) (*webauthn.SessionData, error) {
	var sessionJSON string
	var expiresAt time.Time

	query := `SELECT session_data, expires_at FROM portal_webauthn_sessions WHERE id = ? AND session_type = ?`
	args := []any{sessionID, sessionType}
	if portalCustomerID != nil {
		query += ` AND portal_customer_id = ?`
		args = append(args, *portalCustomerID)
	}

	err := s.db.QueryRow(query, args...).Scan(&sessionJSON, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("session not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	deleteQuery := `DELETE FROM portal_webauthn_sessions WHERE id = ? AND session_type = ?`
	deleteArgs := []any{sessionID, sessionType}
	if portalCustomerID != nil {
		deleteQuery += ` AND portal_customer_id = ?`
		deleteArgs = append(deleteArgs, *portalCustomerID)
	}

	if time.Now().After(expiresAt) {
		_, _ = s.db.Exec(deleteQuery, deleteArgs...)
		return nil, fmt.Errorf("session expired")
	}
	if _, err := s.db.Exec(deleteQuery, deleteArgs...); err != nil {
		slog.Warn("failed to delete portal webauthn session after retrieval",
			slog.String("component", "portal_webauthn"),
			slog.Any("error", err),
			slog.String("session_id", sessionID))
	}

	var sessionData webauthn.SessionData
	if err := json.Unmarshal([]byte(sessionJSON), &sessionData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session data: %w", err)
	}
	return &sessionData, nil
}

// GetRegistrationSession retrieves a registration session bound to the customer.
func (s *SessionStore) GetRegistrationSession(sessionID string, portalCustomerID int) (*webauthn.SessionData, error) {
	return s.getSession(sessionID, "registration", &portalCustomerID)
}

// GetAuthenticationSession retrieves an authentication session. Discoverable
// sessions have a NULL portal_customer_id, so we filter on session_type only.
func (s *SessionStore) GetAuthenticationSession(sessionID string) (*webauthn.SessionData, error) {
	return s.getSession(sessionID, "authentication", nil)
}

func (s *SessionStore) cleanupExpiredSessions() {
	if time.Now().Unix()%100 != 0 {
		return
	}
	go func() {
		_, err := s.db.Exec(`DELETE FROM portal_webauthn_sessions WHERE expires_at < ?`, time.Now())
		if err != nil {
			slog.Warn("failed to cleanup expired portal webauthn sessions", slog.Any("error", err))
		}
	}()
}
