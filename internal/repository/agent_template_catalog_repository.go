package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"windshift/internal/database"
	"windshift/internal/models"
)

// AgentTemplateCatalogRepository persists system-admin overrides for the
// Agent Studio creation catalog (WI-922). The rows are global (not
// workspace-scoped); the embedded defaults remain the fallback seed, so a
// fresh install with no rows still exposes the full catalog.
type AgentTemplateCatalogRepository struct {
	db database.Database
}

// NewAgentTemplateCatalogRepository constructs the repository.
func NewAgentTemplateCatalogRepository(db database.Database) *AgentTemplateCatalogRepository {
	return &AgentTemplateCatalogRepository{db: db}
}

const agentTemplateCatalogColumns = `
	id, template_key, name, default_type, instructions, enabled, created_by_user_id, created_at, updated_at`

func scanAgentTemplateCatalogEntry(row interface{ Scan(...any) error }) (*models.AgentTemplateCatalogEntry, error) {
	var entry models.AgentTemplateCatalogEntry
	var createdBy sql.NullInt64
	if err := row.Scan(
		&entry.ID,
		&entry.TemplateKey,
		&entry.Name,
		&entry.DefaultType,
		&entry.Instructions,
		&entry.Enabled,
		&createdBy,
		&entry.CreatedAt,
		&entry.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if createdBy.Valid {
		v := int(createdBy.Int64)
		entry.CreatedByUserID = &v
	}
	return &entry, nil
}

// List returns every override row, ordered by template key. Used by the
// system-admin management screen.
func (r *AgentTemplateCatalogRepository) List(ctx context.Context) ([]*models.AgentTemplateCatalogEntry, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT "+agentTemplateCatalogColumns+" FROM agent_template_catalog ORDER BY template_key")
	if err != nil {
		return nil, fmt.Errorf("list agent template catalog: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*models.AgentTemplateCatalogEntry
	for rows.Next() {
		entry, err := scanAgentTemplateCatalogEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("scan agent template catalog row: %w", err)
		}
		out = append(out, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate agent template catalog: %w", err)
	}
	return out, nil
}

// ListEnabled returns the enabled override rows used to build the merged
// creation catalog consumed by the Agent Studio templates endpoint and Draft
// creation.
func (r *AgentTemplateCatalogRepository) ListEnabled(ctx context.Context) ([]*models.AgentTemplateCatalogEntry, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT "+agentTemplateCatalogColumns+" FROM agent_template_catalog WHERE enabled = ? ORDER BY template_key", true)
	if err != nil {
		return nil, fmt.Errorf("list enabled agent template catalog: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*models.AgentTemplateCatalogEntry
	for rows.Next() {
		entry, err := scanAgentTemplateCatalogEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("scan enabled agent template catalog row: %w", err)
		}
		out = append(out, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate enabled agent template catalog: %w", err)
	}
	return out, nil
}

// Get returns a single override row by id.
func (r *AgentTemplateCatalogRepository) Get(ctx context.Context, id int64) (*models.AgentTemplateCatalogEntry, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT "+agentTemplateCatalogColumns+" FROM agent_template_catalog WHERE id = ?", id)
	entry, err := scanAgentTemplateCatalogEntry(row)
	if err != nil {
		return nil, notFoundOrWrap(err, "get agent template catalog entry")
	}
	return entry, nil
}

// GetByKey returns a single override row by template key.
func (r *AgentTemplateCatalogRepository) GetByKey(ctx context.Context, key string) (*models.AgentTemplateCatalogEntry, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT "+agentTemplateCatalogColumns+" FROM agent_template_catalog WHERE template_key = ?", key)
	entry, err := scanAgentTemplateCatalogEntry(row)
	if err != nil {
		return nil, notFoundOrWrap(err, "get agent template catalog entry by key")
	}
	return entry, nil
}

// Create inserts a new override row and returns it with its assigned id.
func (r *AgentTemplateCatalogRepository) Create(ctx context.Context, entry *models.AgentTemplateCatalogEntry) (*models.AgentTemplateCatalogEntry, error) {
	if entry.DefaultType == "" {
		entry.DefaultType = models.AgentProfileStandard
	}
	err := r.db.QueryRowWriteContext(ctx, `
		INSERT INTO agent_template_catalog (template_key, name, default_type, instructions, enabled, created_by_user_id)
		VALUES (?, ?, ?, ?, ?, ?)
		RETURNING id, created_at, updated_at
	`, entry.TemplateKey, entry.Name, string(entry.DefaultType), entry.Instructions, entry.Enabled, entry.CreatedByUserID).Scan(
		&entry.ID,
		&entry.CreatedAt,
		&entry.UpdatedAt,
	)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return nil, ErrDuplicateEntry
		}
		return nil, fmt.Errorf("insert agent template catalog entry: %w", err)
	}
	return entry, nil
}

// Update applies a partial update to an existing override row by id.
func (r *AgentTemplateCatalogRepository) Update(ctx context.Context, entry *models.AgentTemplateCatalogEntry) error {
	err := r.db.QueryRowWriteContext(ctx, `
		UPDATE agent_template_catalog
		SET name = ?, default_type = ?, instructions = ?, enabled = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
		RETURNING updated_at
	`, entry.Name, string(entry.DefaultType), entry.Instructions, entry.Enabled, entry.ID).Scan(&entry.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("update agent template catalog entry: %w", err)
	}
	return nil
}

// Delete removes an override row by id. Removing a row restores the embedded
// default for that key; it never deletes a template, only its override.
func (r *AgentTemplateCatalogRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecWriteContext(ctx, "DELETE FROM agent_template_catalog WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete agent template catalog entry: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("agent template catalog entry delete rows affected: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}
