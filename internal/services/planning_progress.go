package services

import (
	"fmt"
	"strings"

	"windshift/internal/models"
	"windshift/internal/repository"
)

// last review: ser, 080626

// StatusBreakdown represents item counts per status category in a progress report.
// Used by both milestones and iterations.
type StatusBreakdown struct {
	CategoryName       string `json:"category_name"`
	CategoryBuiltinKey string `json:"category_builtin_key,omitempty"`
	CategoryColor      string `json:"category_color,omitempty"`
	ItemCount          int    `json:"item_count"`
	IsCompleted        bool   `json:"is_completed"`
}

// ProgressItem represents a work item in a progress report.
// Used by both milestones and iterations.
type ProgressItem struct {
	ID                 int    `json:"id"`
	Title              string `json:"title"`
	WorkspaceID        int    `json:"workspace_id"`
	WorkspaceKey       string `json:"workspace_key"`
	ItemNumber         int    `json:"item_number"`
	StatusName         string `json:"status_name,omitempty"`
	StatusBuiltinKey   string `json:"status_builtin_key,omitempty"`
	StatusColor        string `json:"status_color,omitempty"`
	PriorityName       string `json:"priority_name,omitempty"`
	PriorityBuiltinKey string `json:"priority_builtin_key,omitempty"`
	PriorityColor      string `json:"priority_color,omitempty"`
	AssigneeName       string `json:"assignee_name,omitempty"`
	AssigneeAvatar     string `json:"assignee_avatar,omitempty"`
}

// progressAccumulator collects items and computes progress stats.
type progressAccumulator struct {
	TotalItems      int
	CompletedItems  int
	PercentComplete float64
	StatusBreakdown []StatusBreakdown
	ItemsByCategory map[string][]ProgressItem
}

// progressPageSize matches the item repository's pagination cap.
const progressPageSize = 1000

// planningWorkspaceFilter returns a fail-closed SQL suffix for a trusted
// workspace column. Planning reports are often attached to global objects, but
// their items and test sets remain workspace-scoped; every report query must
// therefore carry the caller's visible workspace IDs.
func planningWorkspaceFilter(column string, workspaceIDs []int) (whereClause string, queryArgs []any) {
	if len(workspaceIDs) == 0 {
		return " AND 1=0", nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(workspaceIDs)), ",")
	args := make([]any, len(workspaceIDs))
	for i, id := range workspaceIDs {
		args[i] = id
	}
	return " AND " + column + " IN (" + placeholders + ")", args
}

// buildProgressReport lists all items matching the given filters (e.g.
// MilestoneID or IterationID) through the item repository and computes
// progress stats, grouping items by status category.
func (s *PlanningService) buildProgressReport(filters repository.ItemFilters, workspaceIDs []int) (*progressAccumulator, error) {
	acc := &progressAccumulator{
		StatusBreakdown: []StatusBreakdown{},
		ItemsByCategory: make(map[string][]ProgressItem),
	}
	if len(workspaceIDs) == 0 {
		return acc, nil
	}

	statuses, err := s.statuses.List()
	if err != nil {
		return nil, fmt.Errorf("failed to list statuses: %w", err)
	}
	statusByID := make(map[int]models.Status, len(statuses))
	for _, st := range statuses {
		statusByID[st.ID] = st
	}

	breakdownMap := make(map[string]*StatusBreakdown)

	for offset := 0; ; offset += progressPageSize {
		items, total, err := s.items.FindAllWithDetails(repository.ItemListParams{
			WorkspaceIDs: workspaceIDs,
			Filters:      filters,
			Pagination:   repository.PaginationParams{Limit: progressPageSize, Offset: offset},
			SortBy:       "key",
			SortAsc:      true,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to query progress items: %w", err)
		}

		for _, item := range items {
			categoryName := "No Status"
			categoryBuiltinKey := ""
			categoryColor := "#9ca3af"
			statusColor := ""
			isCompleted := false
			if item.StatusID != nil {
				if st, ok := statusByID[*item.StatusID]; ok {
					categoryName = st.CategoryName
					categoryBuiltinKey = st.CategoryBuiltinKey
					categoryColor = st.CategoryColor
					statusColor = st.CategoryColor
					isCompleted = st.IsCompleted
				}
			}

			if _, exists := breakdownMap[categoryName]; !exists {
				breakdownMap[categoryName] = &StatusBreakdown{
					CategoryName:       categoryName,
					CategoryBuiltinKey: categoryBuiltinKey,
					CategoryColor:      categoryColor,
					IsCompleted:        isCompleted,
				}
			}
			breakdownMap[categoryName].ItemCount++

			acc.ItemsByCategory[categoryName] = append(acc.ItemsByCategory[categoryName], ProgressItem{
				ID:                 item.ID,
				Title:              item.Title,
				WorkspaceID:        item.WorkspaceID,
				WorkspaceKey:       item.WorkspaceKey,
				ItemNumber:         item.WorkspaceItemNumber,
				StatusName:         item.StatusName,
				StatusBuiltinKey:   item.StatusBuiltinKey,
				StatusColor:        statusColor,
				PriorityName:       item.PriorityName,
				PriorityBuiltinKey: item.PriorityBuiltinKey,
				PriorityColor:      item.PriorityColor,
				AssigneeName:       item.AssigneeName,
				AssigneeAvatar:     item.AssigneeAvatar,
			})
			acc.TotalItems++
			if isCompleted {
				acc.CompletedItems++
			}
		}

		if len(items) == 0 || offset+len(items) >= total {
			break
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
