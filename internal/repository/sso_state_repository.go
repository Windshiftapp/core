package repository

import (
	"database/sql"
	"time"

	"windshift/internal/database"
)

// SSOStateRepository manages short-lived SSO state tokens.
type SSOStateRepository struct {
	db database.Database
}

func NewSSOStateRepository(db database.Database) *SSOStateRepository {
	return &SSOStateRepository{db: db}
}

// SSOStateToken is stored application metadata for an SSO state value.
type SSOStateToken struct {
	ID          int
	RedirectURI string
	RememberMe  bool
	// RequestID is the SP-issued SAML AuthnRequest ID, used to bind the
	// returned assertion via InResponseTo. Empty for OIDC flows.
	RequestID string
	// Nonce is the OIDC nonce sent on the authorization request. Empty for SAML.
	Nonce string
}

// Store persists an SSO state token. requestID is the SAML SP-issued
// AuthnRequest ID (pass "" for OIDC, where it is stored as NULL).
func (r *SSOStateRepository) Store(providerID int, state, requestID, redirectURI string, rememberMe bool, expiresAt time.Time) error {
	var reqID any
	if requestID != "" {
		reqID = requestID
	}
	_, err := r.db.ExecWrite(`
		INSERT INTO sso_state_tokens (provider_id, state, request_id, redirect_uri, remember_me, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, providerID, state, reqID, redirectURI, rememberMe, expiresAt)
	return err
}

// StoreOIDC persists an OIDC state token together with the nonce sent on the
// authorization request, so the callback can require it in the ID token.
func (r *SSOStateRepository) StoreOIDC(providerID int, state, nonce, redirectURI string, rememberMe bool, expiresAt time.Time) error {
	_, err := r.db.ExecWrite(`
		INSERT INTO sso_state_tokens (provider_id, state, nonce, redirect_uri, remember_me, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, providerID, state, nonce, redirectURI, rememberMe, expiresAt)
	return err
}

func (r *SSOStateRepository) GetValid(state string, providerID int, now time.Time) (*SSOStateToken, error) {
	var token SSOStateToken
	var requestID, nonce sql.NullString
	err := r.db.QueryRow(`
		SELECT id, request_id, nonce, redirect_uri, remember_me FROM sso_state_tokens
		WHERE state = ? AND provider_id = ? AND expires_at > ?
	`, state, providerID, now).Scan(&token.ID, &requestID, &nonce, &token.RedirectURI, &token.RememberMe)
	if err != nil {
		return nil, err
	}
	token.RequestID = requestID.String
	token.Nonce = nonce.String
	return &token, nil
}

func (r *SSOStateRepository) Delete(id int) error {
	_, err := r.db.ExecWrite("DELETE FROM sso_state_tokens WHERE id = ?", id)
	return err
}
