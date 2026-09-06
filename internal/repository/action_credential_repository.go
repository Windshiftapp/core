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
	var createdBy sql.NullInt64
	var prefix, metadata sql.NullString
	if err := scanner.Scan(
		&c.ID, &c.Name, &c.CredentialType, &c.AppliesToAllWorkspaces, &createdBy,
		&c.EncryptedSecret, &prefix, &metadata, &c.IsEnabled,
		&c.CreatedAt, &c.UpdatedAt,
	); err != nil {
		return nil, err
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
const actionCredentialColumns = `id, name, credential_type, applies_to_all_workspaces, created_by, ` + //nolint:gosec // G101: SQL column list, not a credential
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
	if !c.AppliesToAllWorkspaces {
		ids, err := r.GetCredentialWorkspaceIDs(c.ID)
		if err != nil {
			return nil, err
		}
		c.WorkspaceIDs = ids
	}
	return c, nil
}

// ListActionCredentialsGlobal returns credentials that apply to all workspaces.
func (r *ActionCredentialRepository) ListActionCredentialsGlobal() ([]*models.ActionCredential, error) {
	return r.queryActionCredentials(
		"failed to list global action credentials",
		`SELECT `+actionCredentialColumns+` FROM action_credentials WHERE applies_to_all_workspaces = true ORDER BY name`,
	)
}

// ListActionCredentialsForWorkspace returns credentials usable in this
// workspace: rows that apply to all workspaces, plus rows scoped to it via the
// join table. The execution layer uses this same view to validate that a
// credential reference is in-scope for a capability.
func (r *ActionCredentialRepository) ListActionCredentialsForWorkspace(workspaceID int) ([]*models.ActionCredential, error) {
	return r.queryActionCredentials(
		"failed to list action credentials for workspace",
		`SELECT `+actionCredentialColumns+`
		 FROM action_credentials
		 WHERE applies_to_all_workspaces = true
		    OR id IN (SELECT credential_id FROM action_credential_workspaces WHERE workspace_id = ?)
		 ORDER BY applies_to_all_workspaces DESC, name`,
		workspaceID,
	)
}

// ListAllActionCredentials returns every credential (admin-only view).
func (r *ActionCredentialRepository) ListAllActionCredentials() ([]*models.ActionCredential, error) {
	return r.queryActionCredentials(
		"failed to list action credentials",
		`SELECT `+actionCredentialColumns+` FROM action_credentials ORDER BY applies_to_all_workspaces DESC, name`,
	)
}

func (r *ActionCredentialRepository) queryActionCredentials(errLabel, query string, args ...any) ([]*models.ActionCredential, error) {
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
	if err := r.populateCredentialWorkspaceIDs(out); err != nil {
		return nil, err
	}
	return out, nil
}

// populateCredentialWorkspaceIDs fills the WorkspaceIDs slice on each credential
// whose AppliesToAllWorkspaces is false. One IN-query rather than per-row
// lookups so list endpoints stay O(1) DB calls. Mirrors
// ActionRepository.populateWorkspaceIDs.
func (r *ActionCredentialRepository) populateCredentialWorkspaceIDs(creds []*models.ActionCredential) error {
	scopedByID := map[int]*models.ActionCredential{}
	ids := []any{}
	for _, c := range creds {
		if !c.AppliesToAllWorkspaces {
			scopedByID[c.ID] = c
			ids = append(ids, c.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}

	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	query := "SELECT credential_id, workspace_id FROM action_credential_workspaces WHERE credential_id IN (" + placeholders + ") ORDER BY workspace_id"
	rows, err := r.db.Query(query, ids...)
	if err != nil {
		return fmt.Errorf("failed to load credential workspace scope: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var credID, wsID int
		if err := rows.Scan(&credID, &wsID); err != nil {
			return fmt.Errorf("failed to scan credential workspace scope row: %w", err)
		}
		if c, ok := scopedByID[credID]; ok {
			c.WorkspaceIDs = append(c.WorkspaceIDs, wsID)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to iterate credential workspace scope rows: %w", err)
	}
	return nil
}

// CreateActionCredential inserts a new credential and returns its ID. The
// caller is responsible for encrypting the secret before calling, and for
// calling SetCredentialWorkspaces afterward when AppliesToAllWorkspaces=false.
func (r *ActionCredentialRepository) CreateActionCredential(c *models.ActionCredential) (int, error) {
	return createActionCredential(r.db, c)
}

func createActionCredential(writer capabilityWriter, c *models.ActionCredential) (int, error) {
	if strings.TrimSpace(c.EncryptedSecret) == "" {
		return 0, errors.New("action credential: encrypted_secret is required")
	}
	now := time.Now()
	c.CreatedAt = now
	c.UpdatedAt = now
	var id int64
	err := writer.QueryRow(`
		INSERT INTO action_credentials
			(name, credential_type, applies_to_all_workspaces, created_by, encrypted_secret,
			 secret_prefix, secret_metadata, is_enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id
	`,
		c.Name, c.CredentialType, c.AppliesToAllWorkspaces, c.CreatedBy, c.EncryptedSecret,
		nullableString(c.SecretPrefix), nullableString(c.SecretMetadata), c.IsEnabled,
		c.CreatedAt, c.UpdatedAt,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create action credential: %w", err)
	}
	c.ID = int(id)
	return c.ID, nil
}

// CreateActionCredentialWithWorkspaces inserts the encrypted credential and
// its scope rows atomically. If any workspace reference is stale, no orphaned
// credential row is committed.
func (r *ActionCredentialRepository) CreateActionCredentialWithWorkspaces(c *models.ActionCredential, workspaceIDs []int) (int, error) {
	var id int
	err := database.WithTx(r.db, func(tx database.Tx) error {
		createdID, err := createActionCredential(tx, c)
		if err != nil {
			return err
		}
		id = createdID
		return setCredentialWorkspaces(tx, id, workspaceIDs)
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}

// UpdateActionCredentialMetadata updates non-secret fields including the
// workspace-scope flag. Use RotateActionCredential to replace secret material,
// and SetCredentialWorkspaces to change the workspace allowlist.
func (r *ActionCredentialRepository) UpdateActionCredentialMetadata(c *models.ActionCredential) error {
	return updateActionCredentialMetadata(r.db, c)
}

func updateActionCredentialMetadata(writer capabilityWriter, c *models.ActionCredential) error {
	c.UpdatedAt = time.Now()
	_, err := writer.ExecWrite(`
		UPDATE action_credentials
		SET name = ?, secret_metadata = ?, is_enabled = ?, applies_to_all_workspaces = ?, updated_at = ?
		WHERE id = ?
	`, c.Name, nullableString(c.SecretMetadata), c.IsEnabled, c.AppliesToAllWorkspaces, c.UpdatedAt, c.ID)
	if err != nil {
		return fmt.Errorf("failed to update action credential: %w", err)
	}
	return nil
}

// UpdateActionCredentialMetadataWithWorkspaces updates metadata and scope in
// one transaction so a failed allowlist replacement cannot widen, narrow, or
// strand the credential independently of the returned error.
func (r *ActionCredentialRepository) UpdateActionCredentialMetadataWithWorkspaces(c *models.ActionCredential, workspaceIDs []int) error {
	return database.WithTx(r.db, func(tx database.Tx) error {
		if err := updateActionCredentialMetadata(tx, c); err != nil {
			return err
		}
		return setCredentialWorkspaces(tx, c.ID, workspaceIDs)
	})
}

// UpdateManagedCredentialTx updates a provider-owned credential inside the
// caller's transaction so provider policy and credential scope cannot diverge.
func (r *ActionCredentialRepository) UpdateManagedCredentialTx(tx database.Tx, c *models.ActionCredential, workspaceIDs []int, encryptedSecret, prefix *string) error {
	if err := updateActionCredentialMetadata(tx, c); err != nil {
		return err
	}
	if err := setCredentialWorkspaces(tx, c.ID, workspaceIDs); err != nil {
		return err
	}
	if encryptedSecret != nil && prefix != nil {
		_, err := tx.ExecWrite(`UPDATE action_credentials
			SET encrypted_secret = ?, secret_prefix = ?, updated_at = ? WHERE id = ?`,
			*encryptedSecret, nullableString(*prefix), time.Now(), c.ID)
		return err
	}
	return nil
}

// UpdateManagedCredentialSecretTx rotates only secret material. The metadata
// predicate keeps the provider purpose/owner check valid at the write point,
// while deliberately leaving name, enabled state, and workspace scope alone.
func (r *ActionCredentialRepository) UpdateManagedCredentialSecretTx(tx database.Tx, id int, expectedMetadata, encryptedSecret, prefix string) (bool, error) {
	if strings.TrimSpace(encryptedSecret) == "" {
		return false, errors.New("managed action credential: encrypted_secret is required")
	}
	result, err := tx.ExecWrite(`UPDATE action_credentials
		SET encrypted_secret = ?, secret_prefix = ?, updated_at = ?
		WHERE id = ? AND secret_metadata = ?`, encryptedSecret, nullableString(prefix), time.Now(), id, expectedMetadata)
	if err != nil {
		return false, fmt.Errorf("failed to rotate managed action credential: %w", err)
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

// RotateActionCredential replaces the encrypted secret and prefix on an
// existing credential row.
func (r *ActionCredentialRepository) RotateActionCredential(id int, encryptedSecret, prefix string) error {
	if strings.TrimSpace(encryptedSecret) == "" {
		return errors.New("action credential rotate: encrypted_secret is required")
	}
	_, err := r.db.ExecWrite(`
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
	_, err := r.db.ExecWrite(`DELETE FROM action_credentials WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete action credential: %w", err)
	}
	return nil
}

// IsCredentialScopedToWorkspace returns true if the credential either applies
// to all workspaces or is explicitly scoped to the given workspace. Returns
// ErrNotFound when the credential ID doesn't exist. Mirrors
// ActionRepository.IsCapabilityScopedToWorkspace.
func (r *ActionCredentialRepository) IsCredentialScopedToWorkspace(credentialID, workspaceID int) (bool, error) {
	var appliesAll bool
	err := r.db.QueryRow(`SELECT applies_to_all_workspaces FROM action_credentials WHERE id = ?`, credentialID).Scan(&appliesAll)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("failed to load credential scope: %w", err)
	}
	if appliesAll {
		return true, nil
	}
	var n int
	err = r.db.QueryRow(`SELECT COUNT(*) FROM action_credential_workspaces WHERE credential_id = ? AND workspace_id = ?`, credentialID, workspaceID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("failed to check credential workspace scope: %w", err)
	}
	return n > 0, nil
}

// GetCredentialWorkspaceIDs returns the workspace IDs scoped to a credential.
// Empty when the credential applies to all workspaces.
func (r *ActionCredentialRepository) GetCredentialWorkspaceIDs(credentialID int) ([]int, error) {
	rows, err := r.db.Query(`SELECT workspace_id FROM action_credential_workspaces WHERE credential_id = ? ORDER BY workspace_id`, credentialID)
	if err != nil {
		return nil, fmt.Errorf("failed to load credential workspace ids: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan credential workspace id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate credential workspace ids: %w", err)
	}
	return ids, nil
}

// SetCredentialWorkspaces replaces the workspace allowlist for a credential.
// Pass an empty slice to clear (only meaningful when AppliesToAllWorkspaces is
// false; the caller is responsible for that invariant).
func (r *ActionCredentialRepository) SetCredentialWorkspaces(credentialID int, workspaceIDs []int) error {
	return setCredentialWorkspaces(r.db, credentialID, workspaceIDs)
}

func setCredentialWorkspaces(writer capabilityWriter, credentialID int, workspaceIDs []int) error {
	if _, err := writer.ExecWrite(`DELETE FROM action_credential_workspaces WHERE credential_id = ?`, credentialID); err != nil {
		return fmt.Errorf("failed to clear credential workspace scope: %w", err)
	}
	for _, wsID := range workspaceIDs {
		if _, err := writer.ExecWrite(`INSERT INTO action_credential_workspaces (credential_id, workspace_id) VALUES (?, ?)`, credentialID, wsID); err != nil {
			return fmt.Errorf("failed to add credential workspace scope (ws %d): %w", wsID, err)
		}
	}
	return nil
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
