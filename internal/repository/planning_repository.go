package repository

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"windshift/internal/database"
)

// PlanningRepository owns milestone and iteration persistence that does not
// belong to an item, workspace, or test repository.
type PlanningRepository struct {
	db database.Database
}

func NewPlanningRepository(db database.Database) *PlanningRepository {
	return &PlanningRepository{db: db}
}

// ListMilestoneNamesForItem returns milestone names attached to an item.
func (r *PlanningRepository) ListMilestoneNamesForItem(itemID int) ([]string, error) {
	rows, err := r.db.Query(`
		SELECT m.name
		FROM item_milestones im
		JOIN milestones m ON m.id = im.milestone_id
		WHERE im.item_id = ?
		ORDER BY m.name
	`, itemID)
	if err != nil {
		return nil, fmt.Errorf("list item milestone names: %w", err)
	}
	defer func() { _ = rows.Close() }()
	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan item milestone name: %w", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate item milestone names: %w", err)
	}
	return names, nil
}

type WorkspaceMilestoneStatusBreakdown struct {
	CategoryName       string
	CategoryBuiltinKey string
	CategoryColor      string
	ItemCount          int
	IsCompleted        bool
}

type WorkspaceMilestoneProgress struct {
	MilestoneID     int
	MilestoneName   string
	TargetDate      *string
	Status          string
	CategoryColor   string
	TotalItems      int
	CompletedItems  int
	PercentComplete float64
	StatusBreakdown []WorkspaceMilestoneStatusBreakdown
}

// GetWorkspaceMilestoneProgress returns active milestone summaries for a
// workspace, optionally constrained by a caller-validated item filter.
func (r *PlanningRepository) GetWorkspaceMilestoneProgress(workspaceID int, filterSQL string, filterArgs []any) ([]WorkspaceMilestoneProgress, error) {
	query := `
		SELECT m.id, m.name, m.target_date, m.status, mc.color,
		       sc.name, COALESCE(sc.builtin_key, ''), sc.color, sc.is_completed, COUNT(i.id)
		` + ItemListFilterFromClause() + `
		JOIN item_milestones im ON im.item_id = i.id
		JOIN milestones m ON m.id = im.milestone_id
		LEFT JOIN milestone_categories mc ON m.category_id = mc.id
		WHERE i.workspace_id = ?
		  AND (m.status IS NULL OR LOWER(m.status) <> 'completed')`
	args := []any{workspaceID}
	if filterSQL != "" {
		query += " AND (" + filterSQL + ")"
		args = append(args, filterArgs...)
	}
	query += `
		GROUP BY m.id, m.name, m.target_date, m.status, mc.color,
		         sc.name, sc.builtin_key, sc.color, sc.is_completed
		ORDER BY m.target_date IS NULL, m.target_date, m.name`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("load workspace milestone progress: %w", err)
	}
	defer func() { _ = rows.Close() }()

	progressByID := make(map[int]*WorkspaceMilestoneProgress)
	for rows.Next() {
		var milestoneID, itemCount int
		var milestoneName string
		var targetDate, milestoneStatus, milestoneColor sql.NullString
		var categoryName, categoryBuiltinKey, categoryColor sql.NullString
		var categoryCompleted sql.NullBool
		if err := rows.Scan(&milestoneID, &milestoneName, &targetDate, &milestoneStatus,
			&milestoneColor, &categoryName, &categoryBuiltinKey, &categoryColor, &categoryCompleted, &itemCount); err != nil {
			return nil, fmt.Errorf("scan workspace milestone progress: %w", err)
		}
		if itemCount == 0 {
			continue
		}
		progress := progressByID[milestoneID]
		if progress == nil {
			progress = &WorkspaceMilestoneProgress{
				MilestoneID:     milestoneID,
				MilestoneName:   milestoneName,
				Status:          milestoneStatus.String,
				CategoryColor:   milestoneColor.String,
				StatusBreakdown: []WorkspaceMilestoneStatusBreakdown{},
			}
			if targetDate.Valid {
				progress.TargetDate = &targetDate.String
			}
			progressByID[milestoneID] = progress
		}
		label := strings.TrimSpace(categoryName.String)
		if label == "" {
			label = "No Status"
		}
		breakdown := WorkspaceMilestoneStatusBreakdown{
			CategoryName:       label,
			CategoryBuiltinKey: categoryBuiltinKey.String,
			CategoryColor:      categoryColor.String,
			ItemCount:          itemCount,
			IsCompleted:        categoryCompleted.Valid && categoryCompleted.Bool,
		}
		progress.StatusBreakdown = append(progress.StatusBreakdown, breakdown)
		progress.TotalItems += itemCount
		if breakdown.IsCompleted {
			progress.CompletedItems += itemCount
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace milestone progress: %w", err)
	}

	ids := make([]int, 0, len(progressByID))
	for id := range progressByID {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		left, right := progressByID[ids[i]], progressByID[ids[j]]
		if left.TargetDate == nil && right.TargetDate != nil {
			return false
		}
		if left.TargetDate != nil && right.TargetDate == nil {
			return true
		}
		if left.TargetDate != nil && right.TargetDate != nil && *left.TargetDate != *right.TargetDate {
			return *left.TargetDate < *right.TargetDate
		}
		return strings.ToLower(left.MilestoneName) < strings.ToLower(right.MilestoneName)
	})

	out := make([]WorkspaceMilestoneProgress, 0, len(ids))
	for _, id := range ids {
		entry := progressByID[id]
		if entry.TotalItems > 0 {
			entry.PercentComplete = float64(entry.CompletedItems) / float64(entry.TotalItems) * 100
		}
		sort.SliceStable(entry.StatusBreakdown, func(i, j int) bool {
			if entry.StatusBreakdown[i].ItemCount == entry.StatusBreakdown[j].ItemCount {
				return strings.ToLower(entry.StatusBreakdown[i].CategoryName) < strings.ToLower(entry.StatusBreakdown[j].CategoryName)
			}
			return entry.StatusBreakdown[i].ItemCount > entry.StatusBreakdown[j].ItemCount
		})
		out = append(out, *entry)
	}
	return out, nil
}

// MilestoneUsableInWorkspace reports whether a milestone is global or belongs
// to workspaceID.
func (r *PlanningRepository) MilestoneUsableInWorkspace(milestoneID, workspaceID int) (bool, error) {
	var count int
	err := r.db.QueryRow(`
		SELECT COUNT(*)
		FROM milestones
		WHERE id = ? AND (COALESCE(is_global, FALSE) = TRUE OR workspace_id = ?)
	`, milestoneID, workspaceID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("validate milestone workspace: %w", err)
	}
	return count > 0, nil
}
