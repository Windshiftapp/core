package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// ZammadOAuthToken is private persistence state. It deliberately contains
// ciphertext only and is never returned by handlers.
type ZammadOAuthToken struct {
	ProviderID              string
	OAuthGeneration         int64
	ExpiresAt               time.Time
	ReauthorizationRequired bool
}

type ZammadOAuthState struct {
	ProviderID      string
	InitiatedBy     int
	OAuthGeneration int64
}

func (r *ZammadRepository) CreateOAuthState(state, providerID string, initiatedBy int, generation int64, expiresAt time.Time) error {
	// Expired-state cleanup is deliberately outside the per-connection
	// transaction. PostgreSQL must never hold an OAuth-state lock while waiting
	// for the connection row, since reset paths lock in the opposite order.
	if _, err := r.db.ExecWrite("DELETE FROM zammad_oauth_state WHERE expires_at <= CURRENT_TIMESTAMP"); err != nil {
		return err
	}
	return database.WithTx(r.db, func(tx database.Tx) error {
		claim, err := tx.Exec(`UPDATE zammad_connections SET oauth_attempt_id = ?, updated_at = CURRENT_TIMESTAMP
			WHERE provider_id = ? AND oauth_generation = ?`, state, providerID, generation)
		if err != nil {
			return err
		}
		rows, err := claim.RowsAffected()
		if err != nil {
			return err
		}
		if rows != 1 {
			return ErrNotFound
		}
		result, err := tx.Exec(`INSERT INTO zammad_oauth_state(state, provider_id, initiated_by, oauth_generation, expires_at)
			SELECT ?, provider_id, ?, oauth_generation, ? FROM zammad_connections
			WHERE provider_id = ? AND oauth_generation = ?
			ON CONFLICT(provider_id) DO UPDATE SET
			state = excluded.state, initiated_by = excluded.initiated_by,
			oauth_generation = excluded.oauth_generation, expires_at = excluded.expires_at,
			created_at = CURRENT_TIMESTAMP`, state, initiatedBy, expiresAt, providerID, generation)
		if err != nil {
			return err
		}
		rows, err = result.RowsAffected()
		if err != nil {
			return err
		}
		if rows != 1 {
			return ErrNotFound
		}
		return nil
	})
}

// ConsumeOAuthState atomically deletes a usable state. A second callback, an
// expired state, or a state for another connection cannot observe it.
func (r *ZammadRepository) ConsumeOAuthState(state string) (*ZammadOAuthState, error) {
	consumed := &ZammadOAuthState{}
	err := r.db.QueryRow(`DELETE FROM zammad_oauth_state
		WHERE state = ? AND expires_at > CURRENT_TIMESTAMP
		RETURNING provider_id, initiated_by, oauth_generation`, state).Scan(&consumed.ProviderID, &consumed.InitiatedBy, &consumed.OAuthGeneration)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("consume Zammad OAuth state: %w", err)
	}
	return consumed, nil
}

// InvalidateOAuthState discards an abandoned provider callback without
// returning any state metadata. It is intentionally idempotent.
func (r *ZammadRepository) InvalidateOAuthState(state string) error {
	var providerID string
	var generation int64
	err := r.db.QueryRow(`SELECT provider_id, oauth_generation
		FROM zammad_oauth_state WHERE state = ?`, state).Scan(&providerID, &generation)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return database.WithTx(r.db, func(tx database.Tx) error {
		// Keep the same connection -> state lock order as StartOAuth and reset.
		if _, err := tx.Exec(`UPDATE zammad_connections SET oauth_attempt_id = NULL
			WHERE provider_id = ? AND oauth_generation = ? AND oauth_attempt_id = ?`, providerID, generation, state); err != nil {
			return err
		}
		_, err := tx.Exec("DELETE FROM zammad_oauth_state WHERE state = ?", state)
		return err
	})
}

func (r *ZammadRepository) UpsertOAuthToken(token ZammadOAuthToken) error {
	if token.OAuthGeneration == 0 {
		if err := r.db.QueryRow("SELECT oauth_generation FROM zammad_connections WHERE provider_id = ?", token.ProviderID).Scan(&token.OAuthGeneration); err != nil {
			return err
		}
	}
	_, err := r.db.ExecWrite(`INSERT INTO zammad_oauth_tokens
		(provider_id, oauth_generation, expires_at, reauthorization_required, refresh_lock_until, refresh_claim_owner, updated_at)
		VALUES (?, ?, ?, false, NULL, NULL, CURRENT_TIMESTAMP)
		ON CONFLICT(provider_id) DO UPDATE SET
		oauth_generation = excluded.oauth_generation,
		expires_at = excluded.expires_at,
		reauthorization_required = false,
		refresh_lock_until = NULL,
		refresh_claim_owner = NULL,
		updated_at = CURRENT_TIMESTAMP`, token.ProviderID, token.OAuthGeneration, token.ExpiresAt)
	return err
}

func (r *ZammadRepository) UpsertOAuthTokenTx(tx database.Tx, token ZammadOAuthToken) error {
	_, err := tx.Exec(`INSERT INTO zammad_oauth_tokens
		(provider_id, oauth_generation, expires_at, reauthorization_required, refresh_lock_until, refresh_claim_owner, updated_at)
		VALUES (?, ?, ?, false, NULL, NULL, CURRENT_TIMESTAMP)
		ON CONFLICT(provider_id) DO UPDATE SET expires_at = excluded.expires_at,
		oauth_generation = excluded.oauth_generation, reauthorization_required = false,
		refresh_lock_until = NULL, refresh_claim_owner = NULL, updated_at = CURRENT_TIMESTAMP`, token.ProviderID, token.OAuthGeneration, token.ExpiresAt)
	return err
}

func (r *ZammadRepository) GetOAuthToken(providerID string) (*ZammadOAuthToken, error) {
	t := &ZammadOAuthToken{ProviderID: providerID}
	err := r.db.QueryRow(`SELECT oauth_generation, expires_at, reauthorization_required
		FROM zammad_oauth_tokens WHERE provider_id = ?`, providerID).Scan(&t.OAuthGeneration, &t.ExpiresAt, &t.ReauthorizationRequired)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get Zammad OAuth token: %w", err)
	}
	return t, nil
}

func (r *ZammadRepository) GetOAuthTokenForRefreshClaim(providerID string, generation int64, owner string) (*ZammadOAuthToken, error) {
	t := &ZammadOAuthToken{ProviderID: providerID}
	err := r.db.QueryRow(`SELECT oauth_generation, expires_at, reauthorization_required
		FROM zammad_oauth_tokens WHERE provider_id = ? AND oauth_generation = ?
		AND refresh_claim_owner = ? AND refresh_lock_until > CURRENT_TIMESTAMP`, providerID, generation, owner).
		Scan(&t.OAuthGeneration, &t.ExpiresAt, &t.ReauthorizationRequired)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get claimed Zammad OAuth token: %w", err)
	}
	return t, nil
}

func (r *ZammadRepository) ClaimOAuthRefresh(providerID string, generation int64, owner string, until time.Time) (bool, error) {
	result, err := r.db.ExecWrite(`UPDATE zammad_oauth_tokens
		SET refresh_lock_until = ?, refresh_claim_owner = ?, updated_at = CURRENT_TIMESTAMP
		WHERE provider_id = ? AND oauth_generation = ? AND reauthorization_required = false
		AND (refresh_lock_until IS NULL OR refresh_lock_until < CURRENT_TIMESTAMP)
		AND EXISTS (SELECT 1 FROM zammad_connections zc WHERE zc.provider_id = zammad_oauth_tokens.provider_id AND zc.oauth_generation = ?)`, until, owner, providerID, generation, generation)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

func (r *ZammadRepository) ReleaseOAuthRefreshClaim(providerID string, generation int64, owner string) error {
	_, err := r.db.ExecWrite(`UPDATE zammad_oauth_tokens
		SET refresh_lock_until = NULL, refresh_claim_owner = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE provider_id = ? AND oauth_generation = ? AND refresh_claim_owner = ?`, providerID, generation, owner)
	return err
}

func (r *ZammadRepository) GuardOAuthGenerationTx(tx database.Tx, providerID string, generation int64) (bool, error) {
	result, err := tx.Exec(`UPDATE zammad_connections SET oauth_generation = oauth_generation
		WHERE provider_id = ? AND oauth_generation = ?`, providerID, generation)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

func (r *ZammadRepository) GuardOAuthCallbackTx(tx database.Tx, providerID string, generation int64, attemptID string) (bool, error) {
	result, err := tx.Exec(`UPDATE zammad_connections SET oauth_attempt_id = NULL
		WHERE provider_id = ? AND oauth_generation = ? AND oauth_attempt_id = ?
		AND EXISTS (
			SELECT 1 FROM integration_providers ip
			WHERE ip.id = zammad_connections.provider_id AND ip.enabled = true
		)`, providerID, generation, attemptID)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

func (r *ZammadRepository) ClearOAuthAttempt(providerID string, generation int64, attemptID string) error {
	_, err := r.db.ExecWrite(`UPDATE zammad_connections SET oauth_attempt_id = NULL
		WHERE provider_id = ? AND oauth_generation = ? AND oauth_attempt_id = ?`, providerID, generation, attemptID)
	return err
}

func (r *ZammadRepository) GuardOAuthRefreshClaimTx(tx database.Tx, providerID string, generation int64, owner string) (bool, error) {
	result, err := tx.Exec(`UPDATE zammad_oauth_tokens SET updated_at = updated_at
		WHERE provider_id = ? AND oauth_generation = ? AND refresh_claim_owner = ?
		AND refresh_lock_until > CURRENT_TIMESTAMP AND reauthorization_required = false`, providerID, generation, owner)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

func (r *ZammadRepository) MarkOAuthReauthorizationRequiredTx(tx database.Tx, providerID string, generation int64, owner string) (bool, error) {
	result, err := tx.Exec(`UPDATE zammad_oauth_tokens
		SET reauthorization_required = true, refresh_lock_until = NULL, refresh_claim_owner = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE provider_id = ? AND oauth_generation = ? AND refresh_claim_owner = ?`, providerID, generation, owner)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

// ResetOAuthAuthorizationTx removes all token metadata and in-flight states
// when OAuth client credentials change. The matching credential payload is
// reset by the service in the same transaction.
func (r *ZammadRepository) ResetOAuthAuthorizationTx(tx database.Tx, providerID string) error {
	result, err := tx.Exec(`UPDATE zammad_connections
		SET oauth_generation = oauth_generation + 1, oauth_attempt_id = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE provider_id = ?`, providerID)
	if err != nil {
		return err
	}
	if rows, err := result.RowsAffected(); err != nil || rows != 1 {
		if err != nil {
			return err
		}
		return ErrNotFound
	}
	if _, err := tx.Exec("DELETE FROM zammad_oauth_tokens WHERE provider_id = ?", providerID); err != nil {
		return err
	}
	_, err = tx.Exec("DELETE FROM zammad_oauth_state WHERE provider_id = ?", providerID)
	return err
}

func (r *ZammadRepository) SetOAuthAuthMethod(connection *models.ZammadConnection) error {
	_, err := r.db.ExecWrite(`UPDATE zammad_connections SET credential_id = ?, auth_method = ?, updated_at = CURRENT_TIMESTAMP WHERE provider_id = ?`, nullablePositiveInt(connection.CredentialID), connection.AuthMethod, connection.ProviderID)
	return err
}
