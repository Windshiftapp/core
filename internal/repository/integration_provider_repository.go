package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/integrations"
	"windshift/internal/models"
)

// IntegrationProviderRepository handles persistence for integration_providers
// (the admin-managed records that back third-party connections like Notion).
type IntegrationProviderRepository struct {
	db database.Database
}

// NewIntegrationProviderRepository creates an IntegrationProviderRepository.
func NewIntegrationProviderRepository(db database.Database) *IntegrationProviderRepository {
	return &IntegrationProviderRepository{db: db}
}

// IntegrationProvider is the row shape returned by reads. The encrypted
// secret column is collapsed into HasOAuthClientSecret so callers don't
// accidentally surface ciphertext in API responses; the secret-bearing
// row is never read by the handler that uses this repo.
type IntegrationProvider struct {
	ID                   string
	Slug                 string
	Name                 string
	ProviderType         models.IntegrationProviderType
	Enabled              bool
	OAuthClientID        string
	HasOAuthClientSecret bool
	ProviderConfig       string
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// IntegrationProviderInsert carries the fields needed to insert a new row.
// OAuthClientSecretEncrypted must be pre-encrypted by the caller; the repo
// performs no crypto.
type IntegrationProviderInsert struct {
	ID                         string
	Slug                       string
	Name                       string
	ProviderType               string
	Enabled                    bool
	OAuthClientID              string
	OAuthClientSecretEncrypted string
	ProviderConfig             string
}

// IntegrationProviderUpdate carries an optional set of fields to update.
// Nil-valued pointers are skipped. OAuthClientSecretEncrypted is pre-encrypted.
type IntegrationProviderUpdate struct {
	Slug                       *string
	Name                       *string
	Enabled                    *bool
	OAuthClientID              *string
	OAuthClientSecretEncrypted *string
	ProviderConfig             *string
}

const integrationProviderColumns = "id, slug, name, provider_type, enabled, oauth_client_id, oauth_client_secret_encrypted, provider_config, created_at, updated_at"

// List returns all integration_providers ordered by name.
func (r *IntegrationProviderRepository) List() ([]IntegrationProvider, error) {
	rows, err := r.db.Query("SELECT " + integrationProviderColumns + " FROM integration_providers ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("list integration_providers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var providers []IntegrationProvider
	for rows.Next() {
		p, scanErr := scanIntegrationProvider(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan integration_provider: %w", scanErr)
		}
		if integrations.SupportsAdminProviderCRUD(p.ProviderType) {
			providers = append(providers, p)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate integration_providers: %w", err)
	}
	return providers, nil
}

// GetByID returns a single integration_provider or ErrNotFound.
func (r *IntegrationProviderRepository) GetByID(id string) (*IntegrationProvider, error) {
	row := r.db.QueryRow("SELECT "+integrationProviderColumns+" FROM integration_providers WHERE id = ?", id)
	p, err := scanIntegrationProvider(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get integration_provider %s: %w", id, err)
	}
	if !integrations.SupportsAdminProviderCRUD(p.ProviderType) {
		return nil, ErrNotFound
	}
	return &p, nil
}

// Create inserts a new integration_provider. Returns ErrDuplicateEntry when
// the slug collides with an existing row.
func (r *IntegrationProviderRepository) Create(req IntegrationProviderInsert) error {
	if !integrations.SupportsAdminProviderCRUD(models.IntegrationProviderType(req.ProviderType)) {
		return ErrInvalidInput
	}
	_, err := r.db.ExecWrite(`
		INSERT INTO integration_providers (
			id, slug, name, provider_type, enabled,
			oauth_client_id, oauth_client_secret_encrypted, provider_config
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, req.ID, req.Slug, req.Name, req.ProviderType, req.Enabled,
		nullStringFromString(req.OAuthClientID),
		nullStringFromString(req.OAuthClientSecretEncrypted),
		nullStringFromString(req.ProviderConfig),
	)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return ErrDuplicateEntry
		}
		return fmt.Errorf("create integration_provider: %w", err)
	}
	return nil
}

// Update applies a partial update. Only non-nil fields in req are written.
// updated_at is always bumped when at least one field is set. Returns
// ErrNotFound when no row matches and ErrDuplicateEntry on slug collision.
func (r *IntegrationProviderRepository) Update(id string, req IntegrationProviderUpdate) error {
	if _, err := r.GetByID(id); err != nil {
		return err
	}

	var sets []string
	var args []any

	if req.Slug != nil {
		sets = append(sets, "slug = ?")
		args = append(args, *req.Slug)
	}
	if req.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *req.Name)
	}
	if req.Enabled != nil {
		sets = append(sets, "enabled = ?")
		args = append(args, *req.Enabled)
	}
	if req.OAuthClientID != nil {
		sets = append(sets, "oauth_client_id = ?")
		args = append(args, *req.OAuthClientID)
	}
	if req.OAuthClientSecretEncrypted != nil {
		sets = append(sets, "oauth_client_secret_encrypted = ?")
		args = append(args, *req.OAuthClientSecretEncrypted)
	}
	if req.ProviderConfig != nil {
		sets = append(sets, "provider_config = ?")
		args = append(args, *req.ProviderConfig)
	}

	if len(sets) == 0 {
		return nil
	}

	sets = append(sets, "updated_at = CURRENT_TIMESTAMP")
	args = append(args, id)

	query := "UPDATE integration_providers SET " + strings.Join(sets, ", ") + " WHERE id = ?"
	result, err := r.db.ExecWrite(query, args...)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return ErrDuplicateEntry
		}
		return fmt.Errorf("update integration_provider %s: %w", id, err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes a row. Returns ErrNotFound when no row matches.
func (r *IntegrationProviderRepository) Delete(id string) error {
	if _, err := r.GetByID(id); err != nil {
		return err
	}
	result, err := r.db.ExecWrite("DELETE FROM integration_providers WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete integration_provider %s: %w", id, err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func scanIntegrationProvider(scanner interface {
	Scan(dest ...any) error
}) (IntegrationProvider, error) {
	var p IntegrationProvider
	var clientID, secretEnc, config sql.NullString
	if err := scanner.Scan(
		&p.ID, &p.Slug, &p.Name, &p.ProviderType, &p.Enabled,
		&clientID, &secretEnc, &config,
		&p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		return p, err
	}
	if clientID.Valid {
		p.OAuthClientID = clientID.String
	}
	p.HasOAuthClientSecret = secretEnc.Valid && secretEnc.String != ""
	if config.Valid {
		p.ProviderConfig = config.String
	}
	return p, nil
}

// nullStringFromString returns a *string suitable for SQL NULL when s is empty.
// Local helper because handlers/helpers.go's nullString isn't reachable here.
func nullStringFromString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
