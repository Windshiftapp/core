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

// LabelRepository persists workspace labels and the item↔label join
// (item_labels). Both the per-label CRUD endpoints and the item-list
// endpoints' bulk-load helper route through here.
type LabelRepository struct {
	db database.Database
}

// NewLabelRepository creates a LabelRepository.
func NewLabelRepository(db database.Database) *LabelRepository {
	return &LabelRepository{db: db}
}

const labelColumns = "id, name, color, workspace_id, created_at, updated_at"

// ListByWorkspace returns all labels in the given workspace, ordered by name.
func (r *LabelRepository) ListByWorkspace(workspaceID int) ([]models.Label, error) {
	rows, err := r.db.Query(
		"SELECT "+labelColumns+" FROM labels WHERE workspace_id = ? ORDER BY name",
		workspaceID,
	)
	if err != nil {
		return nil, fmt.Errorf("list labels for workspace %d: %w", workspaceID, err)
	}
	defer func() { _ = rows.Close() }()

	return scanLabels(rows)
}

// GetByID loads a single label by its primary key. Returns ErrNotFound when missing.
func (r *LabelRepository) GetByID(id int) (*models.Label, error) {
	var label models.Label
	err := r.db.QueryRow(
		"SELECT "+labelColumns+" FROM labels WHERE id = ?",
		id,
	).Scan(&label.ID, &label.Name, &label.Color, &label.WorkspaceID, &label.CreatedAt, &label.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get label %d: %w", id, err)
	}
	return &label, nil
}

// GetWorkspaceID returns the workspace_id for a label or ErrNotFound when missing.
func (r *LabelRepository) GetWorkspaceID(id int) (int, error) {
	var workspaceID int
	err := r.db.QueryRow("SELECT workspace_id FROM labels WHERE id = ?", id).Scan(&workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("get label %d workspace: %w", id, err)
	}
	return workspaceID, nil
}

// NameExistsInWorkspace reports whether a label with the given name already
// exists in the workspace. excludeID > 0 excludes that row from the check (so
// an Update doesn't collide with itself).
func (r *LabelRepository) NameExistsInWorkspace(workspaceID int, name string, excludeID int) (bool, error) {
	var count int
	var err error
	if excludeID > 0 {
		err = r.db.QueryRow(
			"SELECT COUNT(*) FROM labels WHERE name = ? AND workspace_id = ? AND id != ?",
			name, workspaceID, excludeID,
		).Scan(&count)
	} else {
		err = r.db.QueryRow(
			"SELECT COUNT(*) FROM labels WHERE name = ? AND workspace_id = ?",
			name, workspaceID,
		).Scan(&count)
	}
	if err != nil {
		return false, fmt.Errorf("check label name %q in workspace %d: %w", name, workspaceID, err)
	}
	return count > 0, nil
}

// Create inserts a label and returns the id + the stamped timestamp.
func (r *LabelRepository) Create(name, color string, workspaceID int) (int64, time.Time, error) {
	now := time.Now()
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO labels (name, color, workspace_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?) RETURNING id
	`, name, color, workspaceID, now, now).Scan(&id)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("create label: %w", err)
	}
	return id, now, nil
}

// Update overwrites a label's name and color.
func (r *LabelRepository) Update(id int, name, color string) error {
	_, err := r.db.ExecWrite(
		"UPDATE labels SET name = ?, color = ?, updated_at = ? WHERE id = ?",
		name, color, time.Now(), id,
	)
	if err != nil {
		return fmt.Errorf("update label %d: %w", id, err)
	}
	return nil
}

// Delete removes a label row (cascading item_labels via FK).
func (r *LabelRepository) Delete(id int) error {
	if _, err := r.db.ExecWrite("DELETE FROM labels WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete label %d: %w", id, err)
	}
	return nil
}

// ListForItem returns the labels currently attached to an item, ordered by name.
func (r *LabelRepository) ListForItem(itemID int) ([]models.Label, error) {
	rows, err := r.db.Query(`
		SELECT l.id, l.name, l.color, l.workspace_id, l.created_at, l.updated_at
		FROM item_labels il
		JOIN labels l ON il.label_id = l.id
		WHERE il.item_id = ?
		ORDER BY l.name
	`, itemID)
	if err != nil {
		return nil, fmt.Errorf("list labels for item %d: %w", itemID, err)
	}
	defer func() { _ = rows.Close() }()

	return scanLabels(rows)
}

// ReplaceItemLabels swaps the label set for an item atomically: deletes all
// existing rows and inserts the new set inside a single transaction.
func (r *LabelRepository) ReplaceItemLabels(itemID int, labelIDs []int) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin replace item labels: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec("DELETE FROM item_labels WHERE item_id = ?", itemID); err != nil {
		return fmt.Errorf("delete existing item_labels for item %d: %w", itemID, err)
	}

	now := time.Now()
	for _, labelID := range labelIDs {
		if _, err := tx.Exec(
			"INSERT INTO item_labels (item_id, label_id, created_at) VALUES (?, ?, ?)",
			itemID, labelID, now,
		); err != nil {
			return fmt.Errorf("add label %d to item %d: %w", labelID, itemID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit replace item labels: %w", err)
	}
	return nil
}

// AddItemLabel attaches a label to an item. Returns ErrDuplicateEntry when
// the pair already exists (the table has a unique constraint).
func (r *LabelRepository) AddItemLabel(itemID, labelID int) error {
	_, err := r.db.ExecWrite(
		"INSERT INTO item_labels (item_id, label_id, created_at) VALUES (?, ?, ?)",
		itemID, labelID, time.Now(),
	)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return ErrDuplicateEntry
		}
		return fmt.Errorf("add label %d to item %d: %w", labelID, itemID, err)
	}
	return nil
}

// RemoveItemLabel detaches a label from an item. No-ops silently when the
// pair isn't there.
func (r *LabelRepository) RemoveItemLabel(itemID, labelID int) error {
	if _, err := r.db.ExecWrite(
		"DELETE FROM item_labels WHERE item_id = ? AND label_id = ?",
		itemID, labelID,
	); err != nil {
		return fmt.Errorf("remove label %d from item %d: %w", labelID, itemID, err)
	}
	return nil
}

// LoadForItems bulk-loads label rows for a slice of items and attaches them
// to each item's Labels field. Used by the item-list endpoints to avoid an
// N+1 lookup.
func (r *LabelRepository) LoadForItems(items []models.Item) error {
	if len(items) == 0 {
		return nil
	}

	itemIDs := make([]interface{}, len(items))
	placeholders := make([]string, len(items))
	for i, item := range items {
		itemIDs[i] = item.ID
		placeholders[i] = "?"
	}

	query := fmt.Sprintf(`
		SELECT il.item_id, l.id, l.name, l.color, l.workspace_id, l.created_at, l.updated_at
		FROM item_labels il
		JOIN labels l ON il.label_id = l.id
		WHERE il.item_id IN (%s)
		ORDER BY l.name
	`, strings.Join(placeholders, ","))

	rows, err := r.db.Query(query, itemIDs...)
	if err != nil {
		return fmt.Errorf("load labels for items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	labelMap := make(map[int][]models.Label)
	for rows.Next() {
		var itemID int
		var label models.Label
		if err := rows.Scan(&itemID, &label.ID, &label.Name, &label.Color, &label.WorkspaceID,
			&label.CreatedAt, &label.UpdatedAt); err != nil {
			return fmt.Errorf("scan label: %w", err)
		}
		labelMap[itemID] = append(labelMap[itemID], label)
	}

	for i := range items {
		if labels, ok := labelMap[items[i].ID]; ok {
			items[i].Labels = labels
		}
	}
	return nil
}

func scanLabels(rows *sql.Rows) ([]models.Label, error) {
	labels := []models.Label{}
	for rows.Next() {
		var label models.Label
		if err := rows.Scan(&label.ID, &label.Name, &label.Color, &label.WorkspaceID,
			&label.CreatedAt, &label.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan label: %w", err)
		}
		labels = append(labels, label)
	}
	return labels, nil
}
