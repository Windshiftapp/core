package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// LabelRepository persists the global label catalog and item assignments.
type LabelRepository struct {
	db database.Database
}

// NewLabelRepository creates a LabelRepository.
func NewLabelRepository(db database.Database) *LabelRepository {
	return &LabelRepository{db: db}
}

const labelColumns = "id, name, color, created_at, updated_at"

// ListAll returns the global label catalog ordered by name.
func (r *LabelRepository) ListAll() ([]models.Label, error) {
	rows, err := r.db.Query("SELECT " + labelColumns + " FROM labels ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("list labels: %w", err)
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
	).Scan(&label.ID, &label.Name, &label.Color, &label.CreatedAt, &label.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get label %d: %w", id, err)
	}
	return &label, nil
}

// FindIDByName returns a case-insensitive global label match.
func (r *LabelRepository) FindIDByName(name string) (int, error) {
	var id int
	err := r.db.QueryRow("SELECT id FROM labels WHERE LOWER(name) = LOWER(?)", name).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("find label %q: %w", name, err)
	}
	return id, nil
}

// Create inserts a label and returns the id + the stamped timestamp.
func (r *LabelRepository) Create(name, color string) (int64, time.Time, error) {
	now := time.Now()
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO labels (name, color, created_at, updated_at)
		VALUES (?, ?, ?, ?) RETURNING id
	`, name, color, now, now).Scan(&id)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return 0, time.Time{}, ErrDuplicateEntry
		}
		return 0, time.Time{}, fmt.Errorf("create label: %w", err)
	}
	return id, now, nil
}

// Update overwrites a label's name and color.
func (r *LabelRepository) Update(id int, name, color string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin update label %d: %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now()
	_, err = tx.Exec(
		"UPDATE labels SET name = ?, color = ?, updated_at = ? WHERE id = ?",
		name, color, now, id,
	)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return ErrDuplicateEntry
		}
		return fmt.Errorf("update label %d: %w", id, err)
	}
	if err := touchItemsAssignedLabel(tx, id, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit update label %d: %w", id, err)
	}
	return nil
}

// Delete removes a label row (cascading item_labels via FK).
func (r *LabelRepository) Delete(id int) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin delete label %d: %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := touchItemsAssignedLabel(tx, id, time.Now()); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM labels WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete label %d: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete label %d: %w", id, err)
	}
	return nil
}

// touchItemsAssignedLabel invalidates item payloads without treating a global
// catalog edit as activity on every assigned card.
func touchItemsAssignedLabel(tx database.Tx, labelID int, now time.Time) error {
	if _, err := tx.Exec(`
		UPDATE items SET updated_at = ?
		WHERE id IN (SELECT item_id FROM item_labels WHERE label_id = ?)
	`, now, labelID); err != nil {
		return fmt.Errorf("touch items assigned label %d: %w", labelID, err)
	}
	return nil
}

// ListForItem returns the labels currently attached to an item, ordered by name.
func (r *LabelRepository) ListForItem(itemID int) ([]models.Label, error) {
	rows, err := r.db.Query(`
		SELECT l.id, l.name, l.color, l.created_at, l.updated_at
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

// ListForItemTx is ListForItem inside the caller's transaction.
func (r *LabelRepository) ListForItemTx(tx database.Tx, itemID int) ([]models.Label, error) {
	rows, err := tx.Query(`
		SELECT l.id, l.name, l.color, l.created_at, l.updated_at
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

// ReplaceItemLabels atomically changes an item's labels, retaining unchanged associations.
func (r *LabelRepository) ReplaceItemLabels(itemID int, labelIDs []int) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin replace item labels: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := r.ReplaceItemLabelsTx(context.Background(), tx, itemID, labelIDs); err != nil {
		return err
	}
	if err := NewItemRepository(r.db).TouchChanged(tx, itemID, time.Now()); err != nil {
		return fmt.Errorf("touch item %d after replacing labels: %w", itemID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit replace item labels: %w", err)
	}
	return nil
}

// ReplaceItemLabelsTx swaps an item's labels inside the caller's transaction.
func (r *LabelRepository) ReplaceItemLabelsTx(ctx context.Context, tx database.Tx, itemID int, labelIDs []int) error {
	before, err := r.ListForItemTx(tx, itemID)
	if err != nil {
		return err
	}
	existing := make(map[int]bool, len(before))
	for _, label := range before {
		existing[label.ID] = true
	}
	target := make(map[int]bool, len(labelIDs))
	var added, removed []int
	for _, labelID := range labelIDs {
		if !existing[labelID] && !target[labelID] {
			added = append(added, labelID)
		}
		target[labelID] = true
	}
	for _, label := range before {
		if !target[label.ID] {
			removed = append(removed, label.ID)
		}
	}
	return r.ApplyItemLabelDeltaTx(ctx, tx, itemID, added, removed)
}

// ApplyItemLabelDeltaTx writes only changed associations in bounded batches.
func (r *LabelRepository) ApplyItemLabelDeltaTx(ctx context.Context, tx database.Tx, itemID int, added, removed []int) error {
	const batchSize = 400 // At most 800 parameters, including paired inserts.
	for start := 0; start < len(removed); start += batchSize {
		batch := removed[start:min(start+batchSize, len(removed))]
		args := make([]any, 1, len(batch)+1)
		args[0] = itemID
		for _, id := range batch {
			args = append(args, id)
		}
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(batch)), ",")
		if _, err := tx.ExecWriteContext(ctx, "DELETE FROM item_labels WHERE item_id = ? AND label_id IN ("+placeholders+")", args...); err != nil {
			return fmt.Errorf("remove labels for item %d: %w", itemID, err)
		}
	}
	for start := 0; start < len(added); start += batchSize {
		batch := added[start:min(start+batchSize, len(added))]
		args := make([]any, 0, 2*len(batch))
		for _, id := range batch {
			args = append(args, itemID, id)
		}
		values := strings.TrimSuffix(strings.Repeat("(?, ?),", len(batch)), ",")
		if _, err := tx.ExecWriteContext(ctx, "INSERT INTO item_labels (item_id, label_id) VALUES "+values+" ON CONFLICT DO NOTHING", args...); err != nil {
			return fmt.Errorf("attach labels to item %d: %w", itemID, err)
		}
	}
	return nil
}

// EnsureByNameTx returns an existing case-insensitive global label or creates it.
func (r *LabelRepository) EnsureByNameTx(ctx context.Context, tx database.Tx, name, color string) (int, error) {
	var id int
	err := tx.QueryRowContext(ctx,
		"SELECT id FROM labels WHERE LOWER(name) = LOWER(?)", name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("find label %q: %w", name, err)
	}
	err = tx.QueryRowContext(ctx, `
		INSERT INTO labels (name, color, created_at, updated_at)
		VALUES (?, ?, ?, ?) RETURNING id
	`, name, color, time.Now(), time.Now()).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create label %q: %w", name, err)
	}
	return id, nil
}

// AddItemLabel attaches a label to an item. Returns ErrDuplicateEntry when
// the pair already exists (the table has a unique constraint).
func (r *LabelRepository) AddItemLabel(itemID, labelID int) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin add item label: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	changed, err := r.AddItemLabelTx(tx, itemID, labelID)
	if err != nil {
		return err
	}
	if changed {
		if err := NewItemRepository(r.db).TouchChanged(tx, itemID, time.Now()); err != nil {
			return fmt.Errorf("touch item %d after adding label %d: %w", itemID, labelID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit add label %d to item %d: %w", labelID, itemID, err)
	}
	return nil
}

// AddItemLabelTx attaches one label inside the caller's transaction. It
// reports whether a row was inserted.
func (r *LabelRepository) AddItemLabelTx(tx database.Tx, itemID, labelID int) (bool, error) {
	result, err := tx.Exec(
		"INSERT INTO item_labels (item_id, label_id, created_at) VALUES (?, ?, ?)",
		itemID, labelID, time.Now(),
	)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return false, ErrDuplicateEntry
		}
		return false, fmt.Errorf("add label %d to item %d: %w", labelID, itemID, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read attached label count for item %d: %w", itemID, err)
	}
	return rows > 0, nil
}

// RemoveItemLabel detaches a label from an item. No-ops silently when the
// pair isn't there.
func (r *LabelRepository) RemoveItemLabel(itemID, labelID int) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin remove item label: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	changed, err := r.RemoveItemLabelTx(tx, itemID, labelID)
	if err != nil {
		return err
	}
	if changed {
		if err := NewItemRepository(r.db).TouchChanged(tx, itemID, time.Now()); err != nil {
			return fmt.Errorf("touch item %d after removing label %d: %w", itemID, labelID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit remove label %d from item %d: %w", labelID, itemID, err)
	}
	return nil
}

// RemoveItemLabelTx detaches one label inside the caller's transaction. It
// reports whether a row was removed.
func (r *LabelRepository) RemoveItemLabelTx(tx database.Tx, itemID, labelID int) (bool, error) {
	result, err := tx.Exec(
		"DELETE FROM item_labels WHERE item_id = ? AND label_id = ?",
		itemID, labelID,
	)
	if err != nil {
		return false, fmt.Errorf("remove label %d from item %d: %w", labelID, itemID, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read removed label count for item %d: %w", itemID, err)
	}
	return rows > 0, nil
}

// LoadForItems bulk-loads label rows for a slice of items and attaches them
// to each item's Labels field. Used by the item-list endpoints to avoid an
// N+1 lookup.
func (r *LabelRepository) LoadForItems(items []models.Item) error {
	return r.LoadForItemsContext(context.Background(), items)
}

// LoadForItemsContext is the request-aware form of LoadForItems.
func (r *LabelRepository) LoadForItemsContext(ctx context.Context, items []models.Item) error {
	if len(items) == 0 {
		return nil
	}

	itemIDs := make([]any, len(items))
	placeholders := make([]string, len(items))
	for i, item := range items {
		itemIDs[i] = item.ID
		placeholders[i] = "?"
	}

	query := fmt.Sprintf(`
		SELECT il.item_id, l.id, l.name, l.color, l.created_at, l.updated_at
		FROM item_labels il
		JOIN labels l ON il.label_id = l.id
		WHERE il.item_id IN (%s)
		ORDER BY l.name
	`, strings.Join(placeholders, ","))

	rows, err := r.db.QueryContext(ctx, query, itemIDs...)
	if err != nil {
		return fmt.Errorf("load labels for items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	labelMap := make(map[int][]models.Label)
	for rows.Next() {
		var itemID int
		var label models.Label
		if err := rows.Scan(&itemID, &label.ID, &label.Name, &label.Color,
			&label.CreatedAt, &label.UpdatedAt); err != nil {
			return fmt.Errorf("scan label: %w", err)
		}
		labelMap[itemID] = append(labelMap[itemID], label)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate labels: %w", err)
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
		if err := rows.Scan(&label.ID, &label.Name, &label.Color,
			&label.CreatedAt, &label.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan label: %w", err)
		}
		labels = append(labels, label)
	}
	return labels, nil
}
