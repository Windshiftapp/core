package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"windshift/internal/database"
)

// MaxOAuthClientsForDynamicRegistration bounds anonymous registration growth.
// All clients count, including disabled and administrator-managed clients.
const MaxOAuthClientsForDynamicRegistration = 1000

var ErrOAuthClientRegistrationCapacity = errors.New("OAuth client registration capacity reached")

// OAuthClientRepository persists OAuth client registrations.
type OAuthClientRepository struct {
	db database.Database
}

// NewOAuthClientRepository constructs an OAuth client repository.
func NewOAuthClientRepository(db database.Database) *OAuthClientRepository {
	return &OAuthClientRepository{db: db}
}

// EnabledByID returns ErrNotFound when the client does not exist.
func (r *OAuthClientRepository) EnabledByID(id int) (bool, error) {
	var enabled bool
	err := r.db.QueryRow("SELECT enabled FROM oauth_clients WHERE id = ?", id).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("get OAuth client %d: %w", id, err)
	}
	return enabled, nil
}

// CreateDynamicPublicClient inserts a public client only while capacity remains.
// A generated client-ID collision is returned as ErrDuplicateEntry.
func (r *OAuthClientRepository) CreateDynamicPublicClient(
	ctx context.Context,
	slug, displayName, clientID, redirectsJSON, scopesJSON, resourceURI string,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin dynamic OAuth registration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// SQLite serializes writers. PostgreSQL needs an explicit lock so concurrent
	// registrations cannot both observe the last available slot.
	if database.IsPostgresDriver(r.db.GetDriverName()) {
		if _, err := tx.ExecContext(ctx, "LOCK TABLE oauth_clients IN SHARE ROW EXCLUSIVE MODE"); err != nil {
			return fmt.Errorf("lock OAuth registration capacity: %w", err)
		}
	}
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM oauth_clients").Scan(&count); err != nil {
		return fmt.Errorf("count OAuth registration capacity: %w", err)
	}
	if count >= MaxOAuthClientsForDynamicRegistration {
		return ErrOAuthClientRegistrationCapacity
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO oauth_clients (
			slug, display_name, client_id, client_secret_hash, client_type,
			redirect_uris, allowed_scopes, resource_uri, enabled, created_by
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, slug, displayName, clientID, sql.NullString{}, "public", redirectsJSON,
		scopesJSON, resourceURI, true, nil)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return ErrDuplicateEntry
		}
		return fmt.Errorf("insert dynamic OAuth client: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit dynamic OAuth registration: %w", err)
	}
	return nil
}
