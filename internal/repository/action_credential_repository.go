package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// ActionCredentialRepository persists encrypted credentials referenced by
// action HTTP capabilities. The repository deals only with ciphertext — it
// never sees plaintext, and never returns ciphertext to clients (handlers are
// responsible for projecting to ActionCredentialSanitized).
type ActionCredentialRepository struct {
	db database.Database
}

// NewActionCredentialRepository creates a new ActionCredentialRepository.
func NewActionCredentialRepository(db database.Database) *ActionCredentialRepository {
	return &ActionCredentialRepository{db: db}
}

func scanActionCredential(scanner interface{ Scan(dest ...any) error }) (*models.ActionCredential, error) {
	var c models.ActionCredential
	var workspaceID, createdBy sql.NullInt64
	var prefix, metadata sql.NullString
	if err := scanner.Scan(
		&c.ID, &c.Name, &c.CredentialType, &workspaceID, &createdBy,
		&c.EncryptedSecret, &prefix, &metadata, &c.IsEnabled,
		&c.CreatedAt, &c.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if workspaceID.Valid {
		v := int(workspaceID.Int64)
		c.WorkspaceID = &v
	}
	if createdBy.Valid {
		v := int(createdBy.Int64)
		c.CreatedBy = &v
	}
	if prefix.Valid {
		c.SecretPrefix = prefix.String
	}
	if metadata.Valid {
		c.SecretMetadata = metadata.String
	}
	return &c, nil
}

// actionCredentialColumns is the SELECT column list, not credential material.
const actionCredentialColumns = `id, name, credential_type, workspace_id, created_by, ` + //nolint:gosec // G101: SQL column list, not a credential
	`encrypted_secret, secret_prefix, secret_metadata, is_enabled, created_at, updated_at`

// GetActionCredentialByID loads a credential by ID. Returns ErrNotFound when no row matches.
func (r *ActionCredentialRepository) GetActionCredentialByID(id int) (*models.ActionCredential, error) {
	row := r.db.QueryRow(`SELECT `+actionCredentialColumns+` FROM action_credentials WHERE id = ?`, id)
	c, err := scanActionCredential(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get action credential: %w", err)
	}
	return c, nil
}

// ListActionCredentialsGlobal returns all credentials with workspace_id IS NULL.
func (r *ActionCredentialRepository) ListActionCredentialsGlobal() ([]*models.ActionCredential, error) {
	return r.queryActionCredentials(
		"failed to list global action credentials",
		`SELECT `+actionCredentialColumns+` FROM action_credentials WHERE workspace_id IS NULL ORDER BY name`,
	)
}

// ListActionCredentialsForWorkspace returns credentials available to the given
// workspace: rows whose workspace_id matches, plus (when includeGlobals is true)
// every global credential. The execution layer uses this same view to validate
// that a credential reference is in-scope for a capability.
func (r *ActionCredentialRepository) ListActionCredentialsForWorkspace(workspaceID int, includeGlobals bool) ([]*models.ActionCredential, error) {
	if includeGlobals {
		return r.queryActionCredentials(
			"failed to list action credentials for workspace",
			`SELECT `+actionCredentialColumns+`
			 FROM action_credentials
			 WHERE workspace_id = ? OR workspace_id IS NULL
			 ORDER BY workspace_id IS NULL, name`,
			workspaceID,
		)
	}
	return r.queryActionCredentials(
		"failed to list workspace action credentials",
		`SELECT `+actionCredentialColumns+` FROM action_credentials WHERE workspace_id = ? ORDER BY name`,
		workspaceID,
	)
}

// ListAllActionCredentials returns every credential (admin-only view).
func (r *ActionCredentialRepository) ListAllActionCredentials() ([]*models.ActionCredential, error) {
	return r.queryActionCredentials(
		"failed to list action credentials",
		`SELECT `+actionCredentialColumns+` FROM action_credentials ORDER BY workspace_id IS NULL, workspace_id, name`,
	)
}

func (r *ActionCredentialRepository) queryActionCredentials(errLabel, query string, args ...interface{}) ([]*models.ActionCredential, error) {
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errLabel, err)
	}
	defer func() { _ = rows.Close() }()
	var out []*models.ActionCredential
	for rows.Next() {
		c, err := scanActionCredential(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan action credential: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate action credentials: %w", err)
	}
	return out, nil
}

// CreateActionCredential inserts a new credential and returns its ID. The
// caller is responsible for encrypting the secret before calling.
func (r *ActionCredentialRepository) CreateActionCredential(c *models.ActionCredential) (int, error) {
	if strings.TrimSpace(c.EncryptedSecret) == "" {
		return 0, errors.New("action credential: encrypted_secret is required")
	}
	now := time.Now()
	c.CreatedAt = now
	c.UpdatedAt = now
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO action_credentials
			(name, credential_type, workspace_id, created_by, encrypted_secret,
			 secret_prefix, secret_metadata, is_enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id
	`,
		c.Name, c.CredentialType, c.WorkspaceID, c.CreatedBy, c.EncryptedSecret,
		nullableString(c.SecretPrefix), nullableString(c.SecretMetadata), c.IsEnabled,
		c.CreatedAt, c.UpdatedAt,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create action credential: %w", err)
	}
	c.ID = int(id)
	return c.ID, nil
}

// UpdateActionCredentialMetadata updates non-secret fields. Use RotateActionCredential
// to replace the encrypted secret material.
func (r *ActionCredentialRepository) UpdateActionCredentialMetadata(c *models.ActionCredential) error {
	c.UpdatedAt = time.Now()
	_, err := r.db.Exec(`
		UPDATE action_credentials
		SET name = ?, secret_metadata = ?, is_enabled = ?, updated_at = ?
		WHERE id = ?
	`, c.Name, nullableString(c.SecretMetadata), c.IsEnabled, c.UpdatedAt, c.ID)
	if err != nil {
		return fmt.Errorf("failed to update action credential: %w", err)
	}
	return nil
}

// RotateActionCredential replaces the encrypted secret and prefix on an
// existing credential row.
func (r *ActionCredentialRepository) RotateActionCredential(id int, encryptedSecret, prefix string) error {
	if strings.TrimSpace(encryptedSecret) == "" {
		return errors.New("action credential rotate: encrypted_secret is required")
	}
	_, err := r.db.Exec(`
		UPDATE action_credentials
		SET encrypted_secret = ?, secret_prefix = ?, updated_at = ?
		WHERE id = ?
	`, encryptedSecret, nullableString(prefix), time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to rotate action credential: %w", err)
	}
	return nil
}

// DeleteActionCredential removes a credential by ID.
func (r *ActionCredentialRepository) DeleteActionCredential(id int) error {
	_, err := r.db.Exec(`DELETE FROM action_credentials WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete action credential: %w", err)
	}
	return nil
}

func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
