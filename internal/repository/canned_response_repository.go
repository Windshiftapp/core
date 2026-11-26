package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// CannedResponseRepository persists workspace canned responses (WI-1138).
type CannedResponseRepository struct {
	db database.Database
}

// NewCannedResponseRepository creates a CannedResponseRepository.
func NewCannedResponseRepository(db database.Database) *CannedResponseRepository {
	return &CannedResponseRepository{db: db}
}

const cannedResponseColumns = `id, workspace_id, name, body, is_private, is_active,
	created_by, updated_by, used_count, last_used_at, created_at, updated_at`

// ListByWorkspace returns a workspace's canned responses ordered by name.
// includeArchived controls whether is_active = false rows are returned.
func (r *CannedResponseRepository) ListByWorkspace(workspaceID int, includeArchived bool) ([]models.CannedResponse, error) {
	query := "SELECT " + cannedResponseColumns + " FROM canned_responses WHERE workspace_id = ?"
	if !includeArchived {
		query += " AND is_active = true"
	}
	query += " ORDER BY name"
	rows, err := r.db.Query(query, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list canned responses for workspace %d: %w", workspaceID, err)
	}
	defer func() { _ = rows.Close() }()
	return scanCannedResponses(rows)
}

// GetByID loads one canned response by primary key. Returns ErrNotFound when
// missing.
func (r *CannedResponseRepository) GetByID(id int) (*models.CannedResponse, error) {
	var cr models.CannedResponse
	err := r.db.QueryRow(
		"SELECT "+cannedResponseColumns+" FROM canned_responses WHERE id = ?", id,
	).Scan(&cr.ID, &cr.WorkspaceID, &cr.Name, &cr.Body, &cr.IsPrivate, &cr.IsActive,
		&cr.CreatedBy, &cr.UpdatedBy, &cr.UsedCount, &cr.LastUsedAt, &cr.CreatedAt, &cr.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get canned response %d: %w", id, err)
	}
	return &cr, nil
}

// NameExistsInWorkspace reports whether a response with the given name
// already exists in the workspace, case-insensitively (matching the
// LOWER(name) unique index). excludeID > 0 excludes that row so an update
// does not collide with itself.
func (r *CannedResponseRepository) NameExistsInWorkspace(workspaceID int, name string, excludeID int) (bool, error) {
	query := "SELECT COUNT(*) FROM canned_responses WHERE workspace_id = ? AND LOWER(name) = LOWER(?)"
	args := []any{workspaceID, name}
	if excludeID > 0 {
		query += " AND id != ?"
		args = append(args, excludeID)
	}
	var count int
	if err := r.db.QueryRow(query, args...).Scan(&count); err != nil {
		return false, fmt.Errorf("check canned response name %q in workspace %d: %w", name, workspaceID, err)
	}
	return count > 0, nil
}

// Create inserts a canned response and returns the new row.
func (r *CannedResponseRepository) Create(cr *models.CannedResponse) (*models.CannedResponse, error) {
	now := time.Now()
	cr.CreatedAt, cr.UpdatedAt = now, now
	var id int
	err := r.db.QueryRowWrite(`
		INSERT INTO canned_responses
			(workspace_id, name, body, is_private, is_active, created_by, updated_by, used_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?, ?) RETURNING id
	`, cr.WorkspaceID, cr.Name, cr.Body, cr.IsPrivate, cr.IsActive, cr.CreatedBy, cr.UpdatedBy, now, now).Scan(&id)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return nil, ErrDuplicateEntry
		}
		return nil, fmt.Errorf("create canned response: %w", err)
	}
	cr.ID = id
	return cr, nil
}

// Update overwrites the mutable fields of a canned response.
// Returns ErrDuplicateEntry when the new name collides within the workspace.
func (r *CannedResponseRepository) Update(cr *models.CannedResponse) error {
	cr.UpdatedAt = time.Now()
	_, err := r.db.ExecWrite(`
		UPDATE canned_responses
		SET name = ?, body = ?, is_private = ?, is_active = ?, updated_by = ?, updated_at = ?
		WHERE id = ?
	`, cr.Name, cr.Body, cr.IsPrivate, cr.IsActive, cr.UpdatedBy, cr.UpdatedAt, cr.ID)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return ErrDuplicateEntry
		}
		return fmt.Errorf("update canned response %d: %w", cr.ID, err)
	}
	return nil
}

// Delete removes a canned response row.
func (r *CannedResponseRepository) Delete(id int) error {
	if _, err := r.db.ExecWrite("DELETE FROM canned_responses WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete canned response %d: %w", id, err)
	}
	return nil
}

// TouchUsage bumps the usage counter and last-used timestamp after an agent
// or automation inserts the response into a comment.
func (r *CannedResponseRepository) TouchUsage(id int) error {
	_, err := r.db.ExecWrite(
		"UPDATE canned_responses SET used_count = used_count + 1, last_used_at = ? WHERE id = ?",
		time.Now(), id,
	)
	if err != nil {
		return fmt.Errorf("touch canned response usage %d: %w", id, err)
	}
	return nil
}

// GetPortalCustomerName returns the display name of a portal customer, or
// ErrNotFound when the customer is missing.
func (r *CannedResponseRepository) GetPortalCustomerName(id int) (string, error) {
	var name string
	err := r.db.QueryRow("SELECT name FROM portal_customers WHERE id = ?", id).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get portal customer %d name: %w", id, err)
	}
	return name, nil
}

func scanCannedResponses(rows *sql.Rows) ([]models.CannedResponse, error) {
	result := []models.CannedResponse{}
	for rows.Next() {
		var cr models.CannedResponse
		if err := rows.Scan(&cr.ID, &cr.WorkspaceID, &cr.Name, &cr.Body, &cr.IsPrivate, &cr.IsActive,
			&cr.CreatedBy, &cr.UpdatedBy, &cr.UsedCount, &cr.LastUsedAt, &cr.CreatedAt, &cr.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan canned response: %w", err)
		}
		result = append(result, cr)
	}
	return result, rows.Err()
}
