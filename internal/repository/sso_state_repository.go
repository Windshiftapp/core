package repository

import (
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
}

func (r *SSOStateRepository) Store(providerID int, state, redirectURI string, rememberMe bool, expiresAt time.Time) error {
	_, err := r.db.Exec(`
		INSERT INTO sso_state_tokens (provider_id, state, redirect_uri, remember_me, expires_at)
		VALUES (?, ?, ?, ?, ?)
	`, providerID, state, redirectURI, rememberMe, expiresAt)
	return err
}

func (r *SSOStateRepository) GetValid(state string, providerID int, now time.Time) (*SSOStateToken, error) {
	var token SSOStateToken
	err := r.db.QueryRow(`
		SELECT id, redirect_uri, remember_me FROM sso_state_tokens
		WHERE state = ? AND provider_id = ? AND expires_at > ?
	`, state, providerID, now).Scan(&token.ID, &token.RedirectURI, &token.RememberMe)
	if err != nil {
		return nil, err
	}
	return &token, nil
}

func (r *SSOStateRepository) Delete(id int) error {
	_, err := r.db.Exec("DELETE FROM sso_state_tokens WHERE id = ?", id)
	return err
}
