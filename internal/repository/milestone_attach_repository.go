package repository

import (
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
		SELECT m.id, m.name, m.description, m.target_date, m.status,
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

// ReplaceItemMilestones swaps the milestone set for an item atomically: deletes
// all existing rows and inserts the new set inside a single transaction.
func (r *MilestoneAttachRepository) ReplaceItemMilestones(itemID int, milestoneIDs []int) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin replace item milestones: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec("DELETE FROM item_milestones WHERE item_id = ?", itemID); err != nil {
		return fmt.Errorf("delete existing item_milestones for item %d: %w", itemID, err)
	}

	now := time.Now()
	for _, milestoneID := range milestoneIDs {
		if _, err := tx.Exec(
			"INSERT INTO item_milestones (item_id, milestone_id, created_at) VALUES (?, ?, ?)",
			itemID, milestoneID, now,
		); err != nil {
			return fmt.Errorf("add milestone %d to item %d: %w", milestoneID, itemID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit replace item milestones: %w", err)
	}
	return nil
}

// AddItemMilestone attaches a milestone to an item. Returns ErrDuplicateEntry
// when the pair already exists (the table has a unique constraint).
func (r *MilestoneAttachRepository) AddItemMilestone(itemID, milestoneID int) error {
	_, err := r.db.ExecWrite(
		"INSERT INTO item_milestones (item_id, milestone_id, created_at) VALUES (?, ?, ?)",
		itemID, milestoneID, time.Now(),
	)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return ErrDuplicateEntry
		}
		return fmt.Errorf("add milestone %d to item %d: %w", milestoneID, itemID, err)
	}
	return nil
}

// RemoveItemMilestone detaches a milestone from an item. No-ops silently when
// the pair isn't there.
func (r *MilestoneAttachRepository) RemoveItemMilestone(itemID, milestoneID int) error {
	if _, err := r.db.ExecWrite(
		"DELETE FROM item_milestones WHERE item_id = ? AND milestone_id = ?",
		itemID, milestoneID,
	); err != nil {
		return fmt.Errorf("remove milestone %d from item %d: %w", milestoneID, itemID, err)
	}
	return nil
}

// LoadForItems bulk-loads milestone rows for a slice of items and attaches
// them to each item's Milestones field. Used by the item-list endpoints to
// avoid an N+1 lookup.
func (r *MilestoneAttachRepository) LoadForItems(items []models.Item) error {
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
		SELECT im.item_id, m.id, m.name, m.description, m.target_date, m.status,
		       m.category_id, m.is_global, m.workspace_id, m.created_at, m.updated_at
		FROM item_milestones im
		JOIN milestones m ON im.milestone_id = m.id
		WHERE im.item_id IN (%s)
		ORDER BY m.name
	`, strings.Join(placeholders, ","))

	rows, err := r.db.Query(query, itemIDs...)
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
