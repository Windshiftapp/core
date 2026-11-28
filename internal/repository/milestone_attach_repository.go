package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// MilestoneAttachRepository manages the item↔milestone join (item_milestones).
// Modeled on LabelRepository's item_labels helpers.
type MilestoneAttachRepository struct {
	db database.Database
}

// NewMilestoneAttachRepository creates a MilestoneAttachRepository.
func NewMilestoneAttachRepository(db database.Database) *MilestoneAttachRepository {
	return &MilestoneAttachRepository{db: db}
}

// ListForItem returns the milestones currently attached to an item, ordered by name.
func (r *MilestoneAttachRepository) ListForItem(itemID int) ([]models.Milestone, error) {
	rows, err := r.db.Query(`
		SELECT m.id, m.name, COALESCE(m.description, ''), m.target_date, m.status,
		       m.category_id, m.is_global, m.workspace_id, m.created_at, m.updated_at
		FROM item_milestones im
		JOIN milestones m ON im.milestone_id = m.id
		WHERE im.item_id = ?
		ORDER BY m.name
	`, itemID)
	if err != nil {
		return nil, fmt.Errorf("list milestones for item %d: %w", itemID, err)
	}
	defer func() { _ = rows.Close() }()

	return scanMilestoneAttachRows(rows)
}

// MilestoneIDsForItem returns the IDs of the milestones attached to an item,
// in the same name order as ListForItem. Used by action conditions, which
// compare attachment as a CSV of ids.
func (r *MilestoneAttachRepository) MilestoneIDsForItem(itemID int) ([]int, error) {
	milestones, err := r.ListForItem(itemID)
	if err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(milestones))
	for _, milestone := range milestones {
		ids = append(ids, milestone.ID)
	}
	return ids, nil
}

// ReplaceItemMilestones swaps the milestone set for an item atomically: deletes
// all existing rows and inserts the new set inside a single transaction.
func (r *MilestoneAttachRepository) ReplaceItemMilestones(itemID int, milestoneIDs []int) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin replace item milestones: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := r.ReplaceItemMilestonesTx(context.Background(), tx, itemID, milestoneIDs); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit replace item milestones: %w", err)
	}
	return nil
}

// ReplaceItemMilestonesTx swaps an item's milestones inside the caller's transaction.
func (r *MilestoneAttachRepository) ReplaceItemMilestonesTx(ctx context.Context, tx database.Tx, itemID int, milestoneIDs []int) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM item_milestones WHERE item_id = ?", itemID); err != nil {
		return fmt.Errorf("delete milestones for item %d: %w", itemID, err)
	}
	now := time.Now()
	for _, milestoneID := range milestoneIDs {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO item_milestones (item_id, milestone_id, created_at) VALUES (?, ?, ?)",
			itemID, milestoneID, now); err != nil {
			return fmt.Errorf("attach milestone %d to item %d: %w", milestoneID, itemID, err)
		}
	}
	return nil
}

// AddItemMilestone attaches a milestone to an item. Returns ErrDuplicateEntry
// when the pair already exists (the table has a unique constraint).
func (r *MilestoneAttachRepository) AddItemMilestone(itemID, milestoneID int) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin add item milestone: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	inserted, err := r.AddItemMilestoneTx(tx, itemID, milestoneID)
	if err != nil {
		return err
	}
	if !inserted {
		return ErrDuplicateEntry
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit add milestone %d to item %d: %w", milestoneID, itemID, err)
	}
	return nil
}

// AddItemMilestoneTx attaches one milestone inside the caller's transaction and
// reports whether it inserted a row. An existing pair is a no-op instead of an
// error so the caller's transaction stays usable on PostgreSQL.
func (r *MilestoneAttachRepository) AddItemMilestoneTx(tx database.Tx, itemID, milestoneID int) (bool, error) {
	result, err := tx.Exec(
		"INSERT INTO item_milestones (item_id, milestone_id, created_at) VALUES (?, ?, ?) ON CONFLICT(item_id, milestone_id) DO NOTHING",
		itemID, milestoneID, time.Now(),
	)
	if err != nil {
		return false, fmt.Errorf("add milestone %d to item %d: %w", milestoneID, itemID, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read attached milestone count for item %d: %w", itemID, err)
	}
	return rows > 0, nil
}

// RemoveItemMilestone detaches a milestone from an item. No-ops silently when
// the pair isn't there.
func (r *MilestoneAttachRepository) RemoveItemMilestone(itemID, milestoneID int) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin remove item milestone: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := r.RemoveItemMilestoneTx(tx, itemID, milestoneID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit remove milestone %d from item %d: %w", milestoneID, itemID, err)
	}
	return nil
}

// RemoveItemMilestoneTx detaches one milestone inside the caller's transaction
// and reports whether it removed a row.
func (r *MilestoneAttachRepository) RemoveItemMilestoneTx(tx database.Tx, itemID, milestoneID int) (bool, error) {
	result, err := tx.Exec(
		"DELETE FROM item_milestones WHERE item_id = ? AND milestone_id = ?",
		itemID, milestoneID,
	)
	if err != nil {
		return false, fmt.Errorf("remove milestone %d from item %d: %w", milestoneID, itemID, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read removed milestone count for item %d: %w", itemID, err)
	}
	return rows > 0, nil
}

// RemoveWorkspaceMilestonesTx detaches the workspace-owned milestones from an
// item inside the caller's transaction. Global milestone memberships stay: a
// workspace move only leaves the milestones of the workspace it migrates from.
func (r *MilestoneAttachRepository) RemoveWorkspaceMilestonesTx(ctx context.Context, tx database.Tx, itemID int) error {
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM item_milestones
		WHERE item_id = ? AND milestone_id IN (SELECT id FROM milestones WHERE is_global = false)
	`, itemID); err != nil {
		return fmt.Errorf("remove workspace milestones from item %d: %w", itemID, err)
	}
	return nil
}

// LoadForItems bulk-loads milestone rows for a slice of items and attaches
// them to each item's Milestones field. Used by the item-list endpoints to
// avoid an N+1 lookup.
func (r *MilestoneAttachRepository) LoadForItems(items []models.Item) error {
	return r.LoadForItemsContext(context.Background(), items)
}

// LoadForItemsContext is the request-aware form of LoadForItems.
func (r *MilestoneAttachRepository) LoadForItemsContext(ctx context.Context, items []models.Item) error {
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
		SELECT im.item_id, m.id, m.name, COALESCE(m.description, ''), m.target_date, m.status,
		       m.category_id, m.is_global, m.workspace_id, m.created_at, m.updated_at
		FROM item_milestones im
		JOIN milestones m ON im.milestone_id = m.id
		WHERE im.item_id IN (%s)
		ORDER BY m.name
	`, strings.Join(placeholders, ","))

	rows, err := r.db.QueryContext(ctx, query, itemIDs...)
	if err != nil {
		return fmt.Errorf("load milestones for items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	milestoneMap := make(map[int][]models.Milestone)
	for rows.Next() {
		var itemID int
		var m models.Milestone
		var targetDate sql.NullString
		var categoryID sql.NullInt64
		var workspaceID sql.NullInt64
		if err := rows.Scan(&itemID, &m.ID, &m.Name, &m.Description, &targetDate, &m.Status,
			&categoryID, &m.IsGlobal, &workspaceID, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return fmt.Errorf("scan milestone: %w", err)
		}
		if targetDate.Valid {
			m.TargetDate = &targetDate.String
		}
		if categoryID.Valid {
			v := int(categoryID.Int64)
			m.CategoryID = &v
		}
		if workspaceID.Valid {
			v := int(workspaceID.Int64)
			m.WorkspaceID = &v
		}
		milestoneMap[itemID] = append(milestoneMap[itemID], m)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate milestones for items: %w", err)
	}

	for i := range items {
		if ms, ok := milestoneMap[items[i].ID]; ok {
			items[i].Milestones = ms
		}
	}
	return nil
}

func scanMilestoneAttachRows(rows *sql.Rows) ([]models.Milestone, error) {
	var milestones []models.Milestone
	for rows.Next() {
		var m models.Milestone
		var targetDate sql.NullString
		var categoryID sql.NullInt64
		var workspaceID sql.NullInt64
		if err := rows.Scan(&m.ID, &m.Name, &m.Description, &targetDate, &m.Status,
			&categoryID, &m.IsGlobal, &workspaceID, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan milestone: %w", err)
		}
		if targetDate.Valid {
			m.TargetDate = &targetDate.String
		}
		if categoryID.Valid {
			v := int(categoryID.Int64)
			m.CategoryID = &v
		}
		if workspaceID.Valid {
			v := int(workspaceID.Int64)
			m.WorkspaceID = &v
		}
		milestones = append(milestones, m)
	}
	return milestones, nil
}
