package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"

	"windshift/internal/database"
)

// IntegrationTokenRepository persists per-user tokens for integration
// providers (Notion, Todoist, Docmost). It exists so the Docmost refresh path
// and the knowledge source can share one implementation instead of raw SQL.
type IntegrationTokenRepository struct {
	db database.Database
}

// NewIntegrationTokenRepository creates the repository.
func NewIntegrationTokenRepository(db database.Database) *IntegrationTokenRepository {
	return &IntegrationTokenRepository{db: db}
}

// UserIntegrationTokenRow is one stored per-user token.
type UserIntegrationTokenRow struct {
	ID                    string
	UserID                string
	ProviderID            string
	EncryptedAccessToken  string
	EncryptedRefreshToken string
	ExpiresAt             *time.Time
	Scopes                string
	AuthMethod            string
	InvalidatedAt         *time.Time
	ProviderMetadata      string
	ConnectedAt           time.Time
}

// GetUserToken returns the user's token for a provider, or ErrNotFound.
func (r *IntegrationTokenRepository) GetUserToken(providerID string, userID int) (*UserIntegrationTokenRow, error) {
	var row UserIntegrationTokenRow
	var refresh, scopes, authMethod, metadata sql.NullString
	var expiresAt, invalidatedAt sql.NullTime
	err := r.db.QueryRow(`
		SELECT id, user_id, integration_provider_id, oauth_access_token_encrypted,
		       encrypted_refresh_token, expires_at, scopes, auth_method, invalidated_at, provider_metadata, connected_at
		FROM user_integration_tokens
		WHERE integration_provider_id = ? AND user_id = ?
	`, providerID, strconv.Itoa(userID)).Scan(
		&row.ID, &row.UserID, &row.ProviderID, &row.EncryptedAccessToken,
		&refresh, &expiresAt, &scopes, &authMethod, &invalidatedAt, &metadata, &row.ConnectedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get integration token: %w", err)
	}
	row.EncryptedRefreshToken = refresh.String
	row.Scopes = scopes.String
	row.AuthMethod = authMethod.String
	row.ProviderMetadata = metadata.String
	if expiresAt.Valid {
		row.ExpiresAt = &expiresAt.Time
	}
	if invalidatedAt.Valid {
		row.InvalidatedAt = &invalidatedAt.Time
	}
	return &row, nil
}

// MarkInvalid flags the token as rejected by the provider so the UI prompts a
// reconnect.
func (r *IntegrationTokenRepository) MarkInvalid(providerID string, userID int) error {
	_, err := r.db.ExecWrite(`
		UPDATE user_integration_tokens
		SET invalidated_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE integration_provider_id = ? AND user_id = ?
	`, providerID, strconv.Itoa(userID))
	if err != nil {
		return fmt.Errorf("mark integration token invalid: %w", err)
	}
	return nil
}

// MarkVerified clears a prior invalidated_at after a successful call.
func (r *IntegrationTokenRepository) MarkVerified(providerID string, userID int) error {
	_, err := r.db.ExecWrite(`
		UPDATE user_integration_tokens
		SET invalidated_at = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE integration_provider_id = ? AND user_id = ?
	`, providerID, strconv.Itoa(userID))
	if err != nil {
		return fmt.Errorf("mark integration token verified: %w", err)
	}
	return nil
}

// UpsertOAuthToken stores a token pair and clears any refresh claim and
// invalidated state.
func (r *IntegrationTokenRepository) UpsertOAuthToken(providerID string, userID int, encryptedAccess, encryptedRefresh, scopes string, expiresAt *time.Time) error {
	var refresh, scopeArg any
	if encryptedRefresh != "" {
		refresh = encryptedRefresh
	}
	if scopes != "" {
		scopeArg = scopes
	}
	var expiry any
	if expiresAt != nil {
		expiry = *expiresAt
	}
	_, err := r.db.ExecWrite(`
		INSERT INTO user_integration_tokens
			(id, user_id, integration_provider_id, oauth_access_token_encrypted, encrypted_refresh_token, expires_at, scopes, auth_method, provider_metadata, connected_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'oauth', '{}', CURRENT_TIMESTAMP)
		ON CONFLICT(user_id, integration_provider_id) DO UPDATE SET
			oauth_access_token_encrypted = excluded.oauth_access_token_encrypted,
			encrypted_refresh_token = excluded.encrypted_refresh_token,
			expires_at = excluded.expires_at,
			scopes = excluded.scopes,
			auth_method = 'oauth',
			refresh_lock_until = NULL,
			refresh_claim_owner = NULL,
			invalidated_at = NULL,
			connected_at = CURRENT_TIMESTAMP,
			updated_at = CURRENT_TIMESTAMP
	`, uuid.New().String(), strconv.Itoa(userID), providerID, encryptedAccess, refresh, expiry, scopeArg)
	if err != nil {
		return fmt.Errorf("upsert integration token: %w", err)
	}
	return nil
}

// DeleteUserToken removes the user's token for a provider.
func (r *IntegrationTokenRepository) DeleteUserToken(providerID string, userID int) (bool, error) {
	result, err := r.db.ExecWrite(`
		DELETE FROM user_integration_tokens
		WHERE integration_provider_id = ? AND user_id = ?
	`, providerID, strconv.Itoa(userID))
	if err != nil {
		return false, fmt.Errorf("delete integration token: %w", err)
	}
	rows, _ := result.RowsAffected()
	return rows > 0, nil
}

// ClaimRefresh atomically claims the right to refresh this token.
func (r *IntegrationTokenRepository) ClaimRefresh(providerID string, userID int, owner string, until time.Time) (bool, error) {
	result, err := r.db.ExecWrite(`
		UPDATE user_integration_tokens
		SET refresh_lock_until = ?, refresh_claim_owner = ?, updated_at = CURRENT_TIMESTAMP
		WHERE integration_provider_id = ? AND user_id = ?
		  AND (refresh_lock_until IS NULL OR refresh_lock_until < CURRENT_TIMESTAMP)
	`, until, owner, providerID, strconv.Itoa(userID))
	if err != nil {
		return false, fmt.Errorf("claim integration refresh: %w", err)
	}
	rows, _ := result.RowsAffected()
	return rows == 1, nil
}

// ReleaseRefreshClaim frees a claim held by owner.
func (r *IntegrationTokenRepository) ReleaseRefreshClaim(providerID string, userID int, owner string) error {
	_, err := r.db.ExecWrite(`
		UPDATE user_integration_tokens
		SET refresh_lock_until = NULL, refresh_claim_owner = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE integration_provider_id = ? AND user_id = ? AND refresh_claim_owner = ?
	`, providerID, strconv.Itoa(userID), owner)
	if err != nil {
		return fmt.Errorf("release integration refresh claim: %w", err)
	}
	return nil
}

// UpdateRefreshed stores a rotated token pair and clears the refresh claim.
func (r *IntegrationTokenRepository) UpdateRefreshed(providerID string, userID int, encryptedAccess, encryptedRefresh string, expiresAt time.Time) error {
	_, err := r.db.ExecWrite(`
		UPDATE user_integration_tokens
		SET oauth_access_token_encrypted = ?, encrypted_refresh_token = ?, expires_at = ?,
		    refresh_lock_until = NULL, refresh_claim_owner = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE integration_provider_id = ? AND user_id = ?
	`, encryptedAccess, encryptedRefresh, expiresAt, providerID, strconv.Itoa(userID))
	if err != nil {
		return fmt.Errorf("update refreshed integration token: %w", err)
	}
	return nil
}
