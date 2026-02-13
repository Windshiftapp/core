package handlers

import (
	"database/sql"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"windshift/internal/cql"
	"windshift/internal/middleware"
	"windshift/internal/utils"
)

// WorkspaceStats represents comprehensive statistics for a workspace
type WorkspaceStats struct {
	TotalCollections       int                       `json:"total_collections"`
	ItemsByStatusCategory  map[string]int            `json:"items_by_status_category"`
	TotalItems             int                       `json:"total_items"`
	AssignmentDistribution []AssignmentStats         `json:"assignment_distribution"`
	ProjectStatistics      []ProjectStats            `json:"project_statistics"`
	PriorityBreakdown      map[string]int            `json:"priority_breakdown"`
	MilestoneProgress      []MilestoneStatusProgress `json:"milestone_progress"`
}

// AssignmentStats represents the distribution of items per assignee
type AssignmentStats struct {
	UserID       *int   `json:"user_id"`
	UserName     string `json:"user_name"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	ItemCount    int    `json:"item_count"`
	IsUnassigned bool   `json:"is_unassigned"`
}

// ProjectStats represents statistics for a specific project
type ProjectStats struct {
	ProjectID         *int    `json:"project_id"`
	ProjectName       string  `json:"project_name"`
	ProjectColor      string  `json:"project_color,omitempty"`
	ItemCount         int     `json:"item_count"`
	CompletedCount    int     `json:"completed_count"`
	CompletionPercent float64 `json:"completion_percent"`
}

// MilestoneStatusBreakdown represents the distribution of items per status category within a milestone
type MilestoneStatusBreakdown struct {
	CategoryName  string `json:"category_name"`
	CategoryColor string `json:"category_color,omitempty"`
	ItemCount     int    `json:"item_count"`
	IsCompleted   bool   `json:"is_completed"`
}

// MilestoneStatusProgress aggregates milestone progress for a workspace
type MilestoneStatusProgress struct {
	MilestoneID     int                        `json:"milestone_id"`
	MilestoneName   string                     `json:"milestone_name"`
	TargetDate      *string                    `json:"target_date,omitempty"`
	Status          string                     `json:"status,omitempty"`
	CategoryColor   string                     `json:"category_color,omitempty"`
	TotalItems      int                        `json:"total_items"`
	CompletedItems  int                        `json:"completed_items"`
	PercentComplete float64                    `json:"percent_complete"`
	StatusBreakdown []MilestoneStatusBreakdown `json:"status_breakdown"`
}

// GetStats handles GET /api/workspaces/{id}/stats - returns comprehensive workspace statistics
func (h *WorkspaceHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	// Get workspace ID from URL
	workspaceID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get user from context for permission check
	user := r.Context().Value(middleware.ContextKeyUser)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	queryParams := r.URL.Query()
	vqlQuery := queryParams.Get("ql")
	if vqlQuery == "" {
		vqlQuery = queryParams.Get("vql")
	}
	if vqlQuery == "" {
		vqlQuery = queryParams.Get("cql")
	}

	// Support filtering via collection_id by reusing its CQL query
	if vqlQuery == "" {
		if collectionParam := queryParams.Get("collection_id"); collectionParam != "" {
			collectionID, err := strconv.Atoi(collectionParam)
			if err != nil {
				respondInvalidID(w, r, "collection_id")
				return
			}

			var collectionWorkspaceID sql.NullInt64
			var collectionQuery sql.NullString
			err = h.db.QueryRow(`SELECT workspace_id, ql_query FROM collections WHERE id = ?`, collectionID).
				Scan(&collectionWorkspaceID, &collectionQuery)
			if err == sql.ErrNoRows {
				respondNotFound(w, r, "collection")
				return
			}
			if err != nil {
				respondInternalError(w, r, err)
				return
			}

			if collectionWorkspaceID.Valid && collectionWorkspaceID.Int64 != int64(workspaceID) {
				respondBadRequest(w, r, "Collection does not belong to this workspace")
				return
			}

			if collectionQuery.Valid && strings.TrimSpace(collectionQuery.String) != "" {
				vqlQuery = collectionQuery.String
			}
		}
	}

	var filterSQL string
	var filterArgs []interface{}
	if strings.TrimSpace(vqlQuery) != "" {
		workspaceMap, err := h.buildWorkspaceMap()
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		evaluator := cql.NewEvaluator(workspaceMap)
		filterSQL, filterArgs, err = evaluator.EvaluateToSQL(vqlQuery)
		if err != nil {
			respondBadRequest(w, r, "VQL query error: "+err.Error())
			return
		}
	}

	// Calculate time window (last 30 days)
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30).Format("2006-01-02 15:04:05")

	stats := WorkspaceStats{
		ItemsByStatusCategory:  make(map[string]int),
		AssignmentDistribution: []AssignmentStats{},
		ProjectStatistics:      []ProjectStats{},
		PriorityBreakdown:      make(map[string]int),
		MilestoneProgress:      []MilestoneStatusProgress{},
	}

	// 1. Get total collections count
	var collectionCount int
	var err error
	err = h.db.QueryRow(`
		SELECT COUNT(*)
		FROM collections
		WHERE workspace_id = ?
	`, workspaceID).Scan(&collectionCount)
	if err != nil {
		slog.Error("failed to count collections", slog.String("component", "workspaces"), slog.Int("workspace_id", workspaceID), slog.Any("error", err))
	}
	stats.TotalCollections = collectionCount + 1 // +1 for default collection

	// 2. Get total items count
	totalItemsQuery := `
		SELECT COUNT(*)
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		WHERE i.workspace_id = ?`
	totalItemsArgs := []interface{}{workspaceID}
	if filterSQL != "" {
		totalItemsQuery += " AND (" + filterSQL + ")"
		totalItemsArgs = append(totalItemsArgs, filterArgs...)
	}

	err = h.db.QueryRow(totalItemsQuery, totalItemsArgs...).Scan(&stats.TotalItems)
	if err != nil {
		slog.Error("failed to count items", slog.String("component", "workspaces"), slog.Int("workspace_id", workspaceID), slog.Any("error", err))
	}

	// 3. Get items by status category
	statusQuery := `
		SELECT sc.name, COUNT(i.id) as item_count
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		WHERE i.workspace_id = ?`
	if filterSQL != "" {
		statusQuery += " AND (" + filterSQL + ")"
	}
	statusQuery += `
		GROUP BY sc.name`
	statusArgs := []interface{}{workspaceID}
	if filterSQL != "" {
		statusArgs = append(statusArgs, filterArgs...)
	}

	rows, err := h.db.Query(statusQuery, statusArgs...)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var categoryName sql.NullString
			var count int
			if err := rows.Scan(&categoryName, &count); err == nil {
				if categoryName.Valid {
					stats.ItemsByStatusCategory[categoryName.String] = count
				}
			}
		}
	}

	// 4. Get assignment distribution (last 30 days)
	assignmentQuery := `
		SELECT
			i.assignee_id,
			COALESCE(u.username, 'Unassigned') as user_name,
			COALESCE(u.first_name, '') as first_name,
			COALESCE(u.last_name, '') as last_name,
			COUNT(i.id) as item_count
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN users u ON i.assignee_id = u.id
		WHERE i.workspace_id = ?
		  AND i.created_at >= ?`
	if filterSQL != "" {
		assignmentQuery += " AND (" + filterSQL + ")"
	}
	assignmentQuery += `
		GROUP BY i.assignee_id, u.username, u.first_name, u.last_name
		ORDER BY item_count DESC
		LIMIT 10`
	assignmentArgs := []interface{}{workspaceID, thirtyDaysAgo}
	if filterSQL != "" {
		assignmentArgs = append(assignmentArgs, filterArgs...)
	}

	assignmentRows, err := h.db.Query(assignmentQuery, assignmentArgs...)
	if err == nil {
		defer assignmentRows.Close()
		for assignmentRows.Next() {
			var assignment AssignmentStats
			var assigneeID sql.NullInt64
			if err := assignmentRows.Scan(&assigneeID, &assignment.UserName, &assignment.FirstName, &assignment.LastName, &assignment.ItemCount); err == nil {
				if assigneeID.Valid {
					id := int(assigneeID.Int64)
					assignment.UserID = &id
					assignment.IsUnassigned = false
				} else {
					assignment.IsUnassigned = true
				}
				stats.AssignmentDistribution = append(stats.AssignmentDistribution, assignment)
			}
		}
	}

	// 5. Get project statistics (last 30 days)
	projectQuery := `
		SELECT
			tp.id,
			tp.name,
			tp.color,
			COUNT(i.id) as item_count,
			SUM(CASE WHEN LOWER(sc.name) = 'done' THEN 1 ELSE 0 END) as completed_count
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN time_projects tp ON i.time_project_id = tp.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		WHERE i.workspace_id = ?
		  AND i.created_at >= ?
		  AND i.time_project_id IS NOT NULL`
	if filterSQL != "" {
		projectQuery += " AND (" + filterSQL + ")"
	}
	projectQuery += `
		GROUP BY tp.id, tp.name, tp.color
		ORDER BY item_count DESC
		LIMIT 10`
	projectArgs := []interface{}{workspaceID, thirtyDaysAgo}
	if filterSQL != "" {
		projectArgs = append(projectArgs, filterArgs...)
	}

	projectRows, err := h.db.Query(projectQuery, projectArgs...)
	if err == nil {
		defer projectRows.Close()
		for projectRows.Next() {
			var project ProjectStats
			var projectID sql.NullInt64
			var projectColor sql.NullString
			if err := projectRows.Scan(&projectID, &project.ProjectName, &projectColor, &project.ItemCount, &project.CompletedCount); err == nil {
				project.ProjectID = utils.NullInt64ToPtr(projectID)
				project.ProjectColor = projectColor.String
				if project.ItemCount > 0 {
					project.CompletionPercent = float64(project.CompletedCount) / float64(project.ItemCount) * 100
				}
				stats.ProjectStatistics = append(stats.ProjectStatistics, project)
			}
		}
	}

	// 6. Get priority breakdown (last 30 days)
	priorityQuery := `
		SELECT
			COALESCE(pri.name, 'None') as priority,
			COUNT(i.id) as item_count
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN priorities pri ON i.priority_id = pri.id
		WHERE i.workspace_id = ?
		  AND i.created_at >= ?`
	if filterSQL != "" {
		priorityQuery += " AND (" + filterSQL + ")"
	}
	priorityQuery += `
		GROUP BY pri.name`
	priorityArgs := []interface{}{workspaceID, thirtyDaysAgo}
	if filterSQL != "" {
		priorityArgs = append(priorityArgs, filterArgs...)
	}

	priorityRows, err := h.db.Query(priorityQuery, priorityArgs...)
	if err == nil {
		defer priorityRows.Close()
		for priorityRows.Next() {
			var priority string
			var count int
			if err := priorityRows.Scan(&priority, &count); err == nil {
				stats.PriorityBreakdown[priority] = count
			}
		}
	}

	// 7. Load milestone progress for active milestones referenced in this workspace
	if milestoneProgress, mpErr := h.loadWorkspaceMilestoneProgress(workspaceID, filterSQL, filterArgs); mpErr == nil {
		stats.MilestoneProgress = milestoneProgress
	} else {
		slog.Error("failed to load milestone progress", slog.String("component", "workspaces"), slog.Int("workspace_id", workspaceID), slog.Any("error", mpErr))
	}

	respondJSONOK(w, stats)
}

// loadWorkspaceMilestoneProgress aggregates milestone status counts for a workspace
func (h *WorkspaceHandler) loadWorkspaceMilestoneProgress(workspaceID int, filterSQL string, filterArgs []interface{}) ([]MilestoneStatusProgress, error) {
	query := `
		SELECT
			m.id,
			m.name,
			m.target_date,
			m.status,
			mc.color,
			sc.name,
			sc.color,
			sc.is_completed,
			COUNT(i.id) as item_count
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		JOIN milestones m ON i.milestone_id = m.id
		LEFT JOIN milestone_categories mc ON m.category_id = mc.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		WHERE i.workspace_id = ?
		  AND i.milestone_id IS NOT NULL
		  AND (m.status IS NULL OR LOWER(m.status) <> 'completed')`

	args := []interface{}{workspaceID}
	if filterSQL != "" {
		query += " AND (" + filterSQL + ")"
		args = append(args, filterArgs...)
	}

	query += `
		GROUP BY m.id, m.name, m.target_date, m.status, mc.color, sc.name, sc.color, sc.is_completed`

	rows, err := h.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	progressMap := make(map[int]*MilestoneStatusProgress)

	for rows.Next() {
		var milestoneID int
		var milestoneName string
		var targetDate sql.NullString
		var milestoneStatus sql.NullString
		var milestoneColor sql.NullString
		var categoryName sql.NullString
		var categoryColor sql.NullString
		var categoryCompleted sql.NullBool
		var itemCount int

		if err := rows.Scan(
			&milestoneID,
			&milestoneName,
			&targetDate,
			&milestoneStatus,
			&milestoneColor,
			&categoryName,
			&categoryColor,
			&categoryCompleted,
			&itemCount,
		); err != nil {
			return nil, err
		}

		if itemCount == 0 {
			continue
		}

		progress, exists := progressMap[milestoneID]
		if !exists {
			progress = &MilestoneStatusProgress{
				MilestoneID:     milestoneID,
				MilestoneName:   milestoneName,
				StatusBreakdown: []MilestoneStatusBreakdown{},
			}
			progress.TargetDate = utils.NullStringToPtr(targetDate)
			progress.Status = milestoneStatus.String
			progress.CategoryColor = milestoneColor.String
			progressMap[milestoneID] = progress
		}

		label := strings.TrimSpace(categoryName.String)
		if label == "" {
			label = "No Status"
		}

		breakdown := MilestoneStatusBreakdown{
			CategoryName:  label,
			ItemCount:     itemCount,
			IsCompleted:   categoryCompleted.Valid && categoryCompleted.Bool,
			CategoryColor: categoryColor.String,
		}

		progress.StatusBreakdown = append(progress.StatusBreakdown, breakdown)
		progress.TotalItems += itemCount
		if breakdown.IsCompleted {
			progress.CompletedItems += itemCount
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(progressMap) == 0 {
		return []MilestoneStatusProgress{}, nil
	}

	// Build a deterministic order: upcoming target date first, then name
	keys := make([]int, 0, len(progressMap))
	for id := range progressMap {
		keys = append(keys, id)
	}
	sort.Slice(keys, func(i, j int) bool {
		left := progressMap[keys[i]]
		right := progressMap[keys[j]]

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

	results := make([]MilestoneStatusProgress, 0, len(progressMap))
	for _, id := range keys {
		entry := progressMap[id]
		if entry.TotalItems > 0 {
			entry.PercentComplete = float64(entry.CompletedItems) / float64(entry.TotalItems) * 100.0
		}

		// Order breakdown by count desc to highlight most significant segments first
		sort.SliceStable(entry.StatusBreakdown, func(i, j int) bool {
			if entry.StatusBreakdown[i].ItemCount == entry.StatusBreakdown[j].ItemCount {
				return strings.ToLower(entry.StatusBreakdown[i].CategoryName) < strings.ToLower(entry.StatusBreakdown[j].CategoryName)
			}
			return entry.StatusBreakdown[i].ItemCount > entry.StatusBreakdown[j].ItemCount
		})

		results = append(results, *entry)
	}

	return results, nil
}
