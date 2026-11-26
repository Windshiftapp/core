package persistence

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

type sessionSchema struct {
	table       string
	ownerColumn string
}

var (
	internalSessionSchema = sessionSchema{
		table:       "webauthn_sessions",
		ownerColumn: "user_id",
	}
	portalSessionSchema = sessionSchema{
		table:       "portal_webauthn_sessions",
		ownerColumn: "portal_customer_id",
	}
)

// SessionData describes the persisted WebAuthn challenge row. It remains
// exported for compatibility with the package-specific stores.
type SessionData struct {
	ID          string    `json:"id"`
	UserID      *int      `json:"user_id,omitempty"`
	Challenge   string    `json:"challenge"`
	SessionData string    `json:"session_data"`
	SessionType string    `json:"session_type"`
	ExpiresAt   time.Time `json:"expires_at"`
	CreatedAt   time.Time `json:"created_at"`
}

// SessionStore contains shared challenge serialization, expiry, atomic
// one-time consumption, and cleanup mechanics.
type SessionStore struct {
	db     Database
	schema sessionSchema
}

// NewInternalSessionStore creates a store for internal-user challenges.
func NewInternalSessionStore(db Database) *SessionStore {
	return &SessionStore{db: db, schema: internalSessionSchema}
}

// NewPortalSessionStore creates a store for portal-customer challenges.
func NewPortalSessionStore(db Database) *SessionStore {
	return &SessionStore{db: db, schema: portalSessionSchema}
}

func generateSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate session ID: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (s *SessionStore) saveSession(ownerID any, sessionData *webauthn.SessionData, sessionType string) (string, error) {
	sessionID, err := generateSessionID()
	if err != nil {
		return "", err
	}
	sessionJSON, err := json.Marshal(sessionData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal session data: %w", err)
	}
	expiresAt := time.Now().Add(5 * time.Minute)
	query := fmt.Sprintf(`
		INSERT INTO %s (id, %s, challenge, session_data, session_type, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, s.schema.table, s.schema.ownerColumn)
	if _, err := s.db.ExecWrite(query, sessionID, ownerID, sessionData.Challenge, string(sessionJSON), sessionType, expiresAt, time.Now()); err != nil {
		return "", fmt.Errorf("failed to save %s session: %w", sessionType, err)
	}
	s.cleanupExpiredSessions()
	return sessionID, nil
}

// SaveRegistrationSession stores a registration challenge bound to an owner.
func (s *SessionStore) SaveRegistrationSession(ownerID int, sessionData *webauthn.SessionData) (string, error) {
	return s.saveSession(ownerID, sessionData, "registration")
}

// SaveAuthenticationSession stores an authentication challenge. A nil owner
// is used by discoverable/passwordless login.
func (s *SessionStore) SaveAuthenticationSession(ownerID *int, sessionData *webauthn.SessionData) (string, error) {
	return s.saveSession(ownerID, sessionData, "authentication")
}

// SaveAuthenticationSessionBound binds a challenge to a pending browser
// session. The encoded session ID remains in session_type for schema
// compatibility and is matched exactly during consumption.
func (s *SessionStore) SaveAuthenticationSessionBound(ownerID, authSessionID int, sessionData *webauthn.SessionData) (string, error) {
	return s.saveSession(ownerID, sessionData, authenticationSessionType(authSessionID))
}

func authenticationSessionType(authSessionID int) string {
	return "authentication:" + strconv.Itoa(authSessionID)
}

func (s *SessionStore) getSession(sessionID, sessionType string, ownerID *int) (*webauthn.SessionData, error) {
	var sessionJSON string
	var expiresAt time.Time
	query := fmt.Sprintf(`DELETE FROM %s WHERE id = ? AND session_type = ?`, s.schema.table)
	args := []any{sessionID, sessionType}
	if ownerID != nil {
		query += fmt.Sprintf(` AND %s = ?`, s.schema.ownerColumn)
		args = append(args, *ownerID)
	}
	query += ` RETURNING session_data, expires_at`

	err := s.db.QueryRowWrite(query, args...).Scan(&sessionJSON, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("session not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}
	if time.Now().After(expiresAt) {
		return nil, fmt.Errorf("session expired")
	}

	var sessionData webauthn.SessionData
	if err := json.Unmarshal([]byte(sessionJSON), &sessionData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session data: %w", err)
	}
	return &sessionData, nil
}

// GetRegistrationSession consumes a registration challenge bound to ownerID.
func (s *SessionStore) GetRegistrationSession(sessionID string, ownerID int) (*webauthn.SessionData, error) {
	return s.getSession(sessionID, "registration", &ownerID)
}

// GetAuthenticationSession consumes an authentication challenge without an
// owner filter, as required by discoverable login.
func (s *SessionStore) GetAuthenticationSession(sessionID string) (*webauthn.SessionData, error) {
	return s.getSession(sessionID, "authentication", nil)
}

// GetAuthenticationSessionBound consumes only a challenge tied to the given
// pending browser session.
func (s *SessionStore) GetAuthenticationSessionBound(sessionID string, authSessionID int) (*webauthn.SessionData, error) {
	return s.getSession(sessionID, authenticationSessionType(authSessionID), nil)
}

func (s *SessionStore) cleanupExpiredSessions() {
	if time.Now().Unix()%100 != 0 {
		return
	}
	go func() {
		query := fmt.Sprintf(`DELETE FROM %s WHERE expires_at < ?`, s.schema.table)
		if _, err := s.db.ExecWrite(query, time.Now()); err != nil {
			slog.Warn("failed to cleanup expired webauthn sessions", slog.Any("error", err))
		}
	}()
}
