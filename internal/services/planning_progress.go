package services

import (
	"database/sql"
	"fmt"

	"windshift/internal/database"
)

// StatusBreakdown represents item counts per status category in a progress report.
// Used by both milestones and iterations.
type StatusBreakdown struct {
	CategoryName  string `json:"category_name"`
	CategoryColor string `json:"category_color,omitempty"`
	ItemCount     int    `json:"item_count"`
	IsCompleted   bool   `json:"is_completed"`
}

// ProgressItem represents a work item in a progress report.
// Used by both milestones and iterations.
type ProgressItem struct {
	ID             int    `json:"id"`
	Title          string `json:"title"`
	WorkspaceID    int    `json:"workspace_id"`
	WorkspaceKey   string `json:"workspace_key"`
	ItemNumber     int    `json:"item_number"`
	StatusName     string `json:"status_name,omitempty"`
	StatusColor    string `json:"status_color,omitempty"`
	PriorityName   string `json:"priority_name,omitempty"`
	PriorityColor  string `json:"priority_color,omitempty"`
	AssigneeName   string `json:"assignee_name,omitempty"`
	AssigneeAvatar string `json:"assignee_avatar,omitempty"`
}

// progressAccumulator collects items and computes progress stats from query rows.
type progressAccumulator struct {
	TotalItems      int
	CompletedItems  int
	PercentComplete float64
	StatusBreakdown []StatusBreakdown
	ItemsByCategory map[string][]ProgressItem
}

// buildProgressReport scans rows of progress items and returns computed stats.
// Each row must supply the 14-column shape: id, title, workspace_id, workspace_key,
// item_number, category_name, category_color, is_completed, status_name, status_color,
// priority_name, priority_color, assignee_name, assignee_avatar.
func buildProgressReport(rows *sql.Rows) (*progressAccumulator, error) { //nolint:unparam // error is always nil but kept for consistency with scan pattern
	acc := &progressAccumulator{
		ItemsByCategory: make(map[string][]ProgressItem),
	}

	breakdownMap := make(map[string]*StatusBreakdown)

	for rows.Next() {
		var item ProgressItem
		var categoryName, categoryColor string
		var isCompleted bool
		var statusColor, priorityColor sql.NullString

		err := rows.Scan(
			&item.ID, &item.Title, &item.WorkspaceID, &item.WorkspaceKey, &item.ItemNumber,
			&categoryName, &categoryColor, &isCompleted,
			&item.StatusName, &statusColor,
			&item.PriorityName, &priorityColor,
			&item.AssigneeName, &item.AssigneeAvatar,
		)
		if err != nil {
			continue
		}

		item.StatusColor = statusColor.String
		item.PriorityColor = priorityColor.String

		if _, exists := breakdownMap[categoryName]; !exists {
			breakdownMap[categoryName] = &StatusBreakdown{
				CategoryName:  categoryName,
				CategoryColor: categoryColor,
				IsCompleted:   isCompleted,
			}
		}
		breakdownMap[categoryName].ItemCount++

		acc.ItemsByCategory[categoryName] = append(acc.ItemsByCategory[categoryName], item)
		acc.TotalItems++
		if isCompleted {
			acc.CompletedItems++
		}
	}

	acc.StatusBreakdown = make([]StatusBreakdown, 0, len(breakdownMap))
	for _, b := range breakdownMap {
		acc.StatusBreakdown = append(acc.StatusBreakdown, *b)
	}

	if acc.TotalItems > 0 {
		acc.PercentComplete = float64(acc.CompletedItems) / float64(acc.TotalItems) * 100.0
	}

	return acc, nil
}

// queryProgressItems runs the standard progress-items query, filtering by the
// given WHERE clause (e.g. "i.iteration_id = ?" or an EXISTS subquery against
// item_milestones), and returns the computed progress accumulator.
func queryProgressItems(db database.Database, whereClause string, arg int) (*progressAccumulator, error) {
	rows, err := db.Query(`
		SELECT
			i.id, i.title, i.workspace_id, w.key as workspace_key, i.workspace_item_number,
			COALESCE(sc.name, 'No Status') as category_name,
			COALESCE(sc.color, '#9ca3af') as category_color,
			COALESCE(sc.is_completed, false) as is_completed,
			COALESCE(st.name, '') as status_name,
			COALESCE(sc.color, '') as status_color,
			COALESCE(p.name, '') as priority_name,
			COALESCE(p.color, '') as priority_color,
			COALESCE(u.first_name || ' ' || u.last_name, '') as assignee_name,
			COALESCE(u.avatar_url, '') as assignee_avatar
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN statuses st ON i.status_id = st.id
		LEFT JOIN status_categories sc ON st.category_id = sc.id
		LEFT JOIN priorities p ON i.priority_id = p.id
		LEFT JOIN users u ON i.assignee_id = u.id
		WHERE `+whereClause+`
		ORDER BY sc.name, i.workspace_item_number
	`, arg)
	if err != nil {
		return nil, fmt.Errorf("failed to query progress items: %w", err)
	}
	defer rows.Close()

	return buildProgressReport(rows)
}
