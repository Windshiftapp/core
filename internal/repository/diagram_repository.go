package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// DiagramRepository persists item diagrams (the per-item embedded
// excalidraw / mermaid stored as opaque diagram_data) and the small
// item_history rows the diagram handler emits when one is created,
// updated, or deleted.
type DiagramRepository struct {
	db database.Database
}

// NewDiagramRepository creates a DiagramRepository.
func NewDiagramRepository(db database.Database) *DiagramRepository {
	return &DiagramRepository{db: db}
}

const diagramSelectWithUsers = `
	SELECT
		d.id, d.item_id, d.name, d.diagram_data, d.created_at, d.updated_at, d.created_by, d.updated_by,
		u1.first_name || ' ' || u1.last_name as creator_name, u1.email as creator_email,
		u2.first_name || ' ' || u2.last_name as updated_by_name, u2.email as updated_by_email
	FROM item_diagrams d
	LEFT JOIN users u1 ON d.created_by = u1.id
	LEFT JOIN users u2 ON d.updated_by = u2.id`

func scanDiagramWithUsers(scanner interface{ Scan(dest ...any) error }) (models.ItemDiagram, error) {
	var d models.ItemDiagram
	var creatorName, creatorEmail, updatedByName, updatedByEmail sql.NullString

	if err := scanner.Scan(
		&d.ID, &d.ItemID, &d.Name, &d.DiagramData, &d.CreatedAt, &d.UpdatedAt, &d.CreatedBy, &d.UpdatedBy,
		&creatorName, &creatorEmail,
		&updatedByName, &updatedByEmail,
	); err != nil {
		return d, err
	}

	if creatorName.Valid {
		d.CreatorName = creatorName.String
	}
	if creatorEmail.Valid {
		d.CreatorEmail = creatorEmail.String
	}
	if updatedByName.Valid {
		d.UpdatedByName = updatedByName.String
	}
	if updatedByEmail.Valid {
		d.UpdatedByEmail = updatedByEmail.String
	}
	return d, nil
}

// ListByItem returns all diagrams for the given item, joined with
// creator/updater user names.
func (r *DiagramRepository) ListByItem(itemID int) ([]models.ItemDiagram, error) {
	rows, err := r.db.Query(diagramSelectWithUsers+` WHERE d.item_id = ? ORDER BY d.created_at DESC`, itemID)
	if err != nil {
		return nil, fmt.Errorf("list diagrams for item %d: %w", itemID, err)
	}
	defer func() { _ = rows.Close() }()

	diagrams := []models.ItemDiagram{}
	for rows.Next() {
		d, scanErr := scanDiagramWithUsers(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan diagram: %w", scanErr)
		}
		diagrams = append(diagrams, d)
	}
	return diagrams, nil
}

// GetByID returns a single diagram with joined user names. Returns
// ErrNotFound when no row matches.
func (r *DiagramRepository) GetByID(id int) (*models.ItemDiagram, error) {
	row := r.db.QueryRow(diagramSelectWithUsers+` WHERE d.id = ?`, id)
	d, err := scanDiagramWithUsers(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get diagram %d: %w", id, err)
	}
	return &d, nil
}

// Create inserts a new diagram. createdBy may be nil for system-created rows.
// Returns the new id and the timestamp it stamped on created_at/updated_at.
func (r *DiagramRepository) Create(itemID int, name, diagramData string, createdBy *int) (int64, time.Time, error) {
	now := time.Now()
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO item_diagrams (item_id, name, diagram_data, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING id
	`, itemID, name, diagramData, createdBy, now, now).Scan(&id)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("create diagram for item %d: %w", itemID, err)
	}
	return id, now, nil
}

// Update overwrites a diagram's editable fields. Returns ErrNotFound when no
// row was updated (diagram missing).
func (r *DiagramRepository) Update(id int, name, diagramData string, updatedBy *int) error {
	now := time.Now()
	result, err := r.db.ExecWrite(`
		UPDATE item_diagrams
		SET name = ?, diagram_data = ?, updated_at = ?, updated_by = ?
		WHERE id = ?
	`, name, diagramData, now, updatedBy, id)
	if err != nil {
		return fmt.Errorf("update diagram %d: %w", id, err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes a diagram row. Returns ErrNotFound when no row matches.
func (r *DiagramRepository) Delete(id int) error {
	result, err := r.db.ExecWrite(`DELETE FROM item_diagrams WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete diagram %d: %w", id, err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// GetNameAndItemID fetches just the name and item_id for a diagram. Used by
// the handler to log history and authorize edits without a full SELECT.
// Returns ErrNotFound when missing.
func (r *DiagramRepository) GetNameAndItemID(id int) (name string, itemID int, err error) {
	err = r.db.QueryRow("SELECT name, item_id FROM item_diagrams WHERE id = ?", id).Scan(&name, &itemID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, ErrNotFound
	}
	if err != nil {
		return "", 0, fmt.Errorf("get diagram %d name/item: %w", id, err)
	}
	return name, itemID, nil
}

// RecordHistory appends an item_history row for a diagram-related action.
// userID is required (the handler skips the call entirely when nil).
func (r *DiagramRepository) RecordHistory(itemID, userID int, action string, oldValue *string, newValue string) error {
	if _, err := r.db.ExecWrite(
		`INSERT INTO item_history (item_id, user_id, field_name, old_value, new_value, changed_at)
		 VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		itemID, userID, action, oldValue, newValue,
	); err != nil {
		return fmt.Errorf("record diagram history for item %d: %w", itemID, err)
	}
	return nil
}
