package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/utils"
)

type MilestoneHandler struct {
	db database.Database
}

func NewMilestoneHandler(db database.Database) *MilestoneHandler {
	return &MilestoneHandler{db: db}
}

func (h *MilestoneHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	categoryID := r.URL.Query().Get("category_id")
	status := r.URL.Query().Get("status")
	workspaceID := r.URL.Query().Get("workspace_id")
	includeGlobal := r.URL.Query().Get("include_global") != "false" // Default to true

	query := `
		SELECT m.id, m.name, m.description, m.target_date, m.status, m.category_id,
		       m.is_global, m.workspace_id, m.created_at, m.updated_at,
		       mc.name as category_name, mc.color as category_color, w.name as workspace_name
		FROM milestones m
		LEFT JOIN milestone_categories mc ON m.category_id = mc.id
		LEFT JOIN workspaces w ON m.workspace_id = w.id
		WHERE 1=1`

	var args []interface{}

	// Filter by workspace - show local milestones for this workspace + global milestones
	if workspaceID != "" {
		if id, err := strconv.Atoi(workspaceID); err == nil {
			if includeGlobal {
				query += " AND (m.workspace_id = ? OR m.is_global = ?)"
				args = append(args, id, true)
			} else {
				query += " AND m.workspace_id = ?"
				args = append(args, id)
			}
		}
	} else if includeGlobal {
		// If no workspace specified, only show global milestones
		query += " AND m.is_global = ?"
		args = append(args, true)
	}

	if categoryID != "" {
		if categoryID == "null" || categoryID == "0" {
			query += " AND m.category_id IS NULL"
		} else {
			query += " AND m.category_id = ?"
			if id, err := strconv.Atoi(categoryID); err == nil {
				args = append(args, id)
			}
		}
	}

	if status != "" {
		query += " AND m.status = ?"
		args = append(args, status)
	}

	query += " ORDER BY m.target_date, m.name"

	rows, err := h.db.Query(query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var milestones []models.Milestone
	for rows.Next() {
		var milestone models.Milestone
		var description sql.NullString
		var categoryID sql.NullInt64
		var categoryName sql.NullString
		var categoryColor sql.NullString
		var targetDate sql.NullString
		var workspaceID sql.NullInt64
		var workspaceName sql.NullString

		err := rows.Scan(&milestone.ID, &milestone.Name, &description, &targetDate,
			&milestone.Status, &categoryID, &milestone.IsGlobal, &workspaceID,
			&milestone.CreatedAt, &milestone.UpdatedAt,
			&categoryName, &categoryColor, &workspaceName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		milestone.Description = description.String
		milestone.TargetDate = utils.NullStringToPtr(targetDate)
		milestone.CategoryID = utils.NullInt64ToPtr(categoryID)
		milestone.CategoryName = categoryName.String
		milestone.CategoryColor = categoryColor.String
		milestone.WorkspaceID = utils.NullInt64ToPtr(workspaceID)
		milestone.WorkspaceName = workspaceName.String

		milestones = append(milestones, milestone)
	}

	// Always return an array, even if empty
	if milestones == nil {
		milestones = []models.Milestone{}
	}

	respondJSONOK(w, milestones)
}

func (h *MilestoneHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var milestone models.Milestone
	var description sql.NullString
	var categoryID sql.NullInt64
	var categoryName sql.NullString
	var categoryColor sql.NullString
	var targetDate sql.NullString
	var workspaceID sql.NullInt64
	var workspaceName sql.NullString

	err := h.db.QueryRow(`
		SELECT m.id, m.name, m.description, m.target_date, m.status, m.category_id,
		       m.is_global, m.workspace_id, m.created_at, m.updated_at,
		       mc.name as category_name, mc.color as category_color, w.name as workspace_name
		FROM milestones m
		LEFT JOIN milestone_categories mc ON m.category_id = mc.id
		LEFT JOIN workspaces w ON m.workspace_id = w.id
		WHERE m.id = ?
	`, id).Scan(&milestone.ID, &milestone.Name, &description, &targetDate,
		&milestone.Status, &categoryID, &milestone.IsGlobal, &workspaceID,
		&milestone.CreatedAt, &milestone.UpdatedAt,
		&categoryName, &categoryColor, &workspaceName)

	if err == sql.ErrNoRows {
		http.Error(w, "Milestone not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	milestone.Description = description.String
	milestone.TargetDate = utils.NullStringToPtr(targetDate)
	milestone.CategoryID = utils.NullInt64ToPtr(categoryID)
	milestone.CategoryName = categoryName.String
	milestone.CategoryColor = categoryColor.String
	milestone.WorkspaceID = utils.NullInt64ToPtr(workspaceID)
	milestone.WorkspaceName = workspaceName.String

	respondJSONOK(w, milestone)
}

func (h *MilestoneHandler) Create(w http.ResponseWriter, r *http.Request) {
	var milestone models.Milestone
	if err := json.NewDecoder(r.Body).Decode(&milestone); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if strings.TrimSpace(milestone.Name) == "" {
		http.Error(w, "Milestone name is required", http.StatusBadRequest)
		return
	}

	// Handle empty target_date (set to nil)
	if milestone.TargetDate != nil && strings.TrimSpace(*milestone.TargetDate) == "" {
		milestone.TargetDate = nil
	}

	// Validate status
	validStatuses := []string{"planning", "in-progress", "completed", "cancelled"}
	statusValid := false
	for _, validStatus := range validStatuses {
		if milestone.Status == validStatus {
			statusValid = true
			break
		}
	}
	if !statusValid {
		milestone.Status = "planning" // Default status
	}

	// Validate global vs workspace constraints
	if milestone.IsGlobal && milestone.WorkspaceID != nil {
		http.Error(w, "Global milestones cannot have a workspace_id", http.StatusBadRequest)
		return
	}
	if !milestone.IsGlobal && milestone.WorkspaceID == nil {
		http.Error(w, "Local milestones must have a workspace_id", http.StatusBadRequest)
		return
	}

	// Validate category_id if provided
	if milestone.CategoryID != nil {
		var categoryExists int
		err := h.db.QueryRow("SELECT COUNT(*) FROM milestone_categories WHERE id = ?", *milestone.CategoryID).Scan(&categoryExists)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if categoryExists == 0 {
			http.Error(w, "Invalid category ID", http.StatusBadRequest)
			return
		}
	}

	// Validate workspace_id if provided
	if milestone.WorkspaceID != nil {
		var workspaceExists int
		err := h.db.QueryRow("SELECT COUNT(*) FROM workspaces WHERE id = ?", *milestone.WorkspaceID).Scan(&workspaceExists)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if workspaceExists == 0 {
			http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
			return
		}
	}

	// Sanitize user input to prevent XSS
	milestone.Name = utils.StripHTMLTags(milestone.Name)
	milestone.Description = utils.StripHTMLTags(milestone.Description)

	now := time.Now()
	var id int64
	err := h.db.QueryRow(`
		INSERT INTO milestones (name, description, target_date, status, category_id, is_global, workspace_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, milestone.Name, milestone.Description, milestone.TargetDate, milestone.Status, milestone.CategoryID,
		milestone.IsGlobal, milestone.WorkspaceID, now, now).Scan(&id)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the created milestone with category info
	var createdMilestone models.Milestone
	var description sql.NullString
	var categoryID sql.NullInt64
	var categoryName sql.NullString
	var categoryColor sql.NullString
	var createdTargetDate sql.NullString
	var workspaceID sql.NullInt64
	var workspaceName sql.NullString

	err = h.db.QueryRow(`
		SELECT m.id, m.name, m.description, m.target_date, m.status, m.category_id,
		       m.is_global, m.workspace_id, m.created_at, m.updated_at,
		       mc.name as category_name, mc.color as category_color, w.name as workspace_name
		FROM milestones m
		LEFT JOIN milestone_categories mc ON m.category_id = mc.id
		LEFT JOIN workspaces w ON m.workspace_id = w.id
		WHERE m.id = ?
	`, id).Scan(&createdMilestone.ID, &createdMilestone.Name, &description, &createdTargetDate,
		&createdMilestone.Status, &categoryID, &createdMilestone.IsGlobal, &workspaceID,
		&createdMilestone.CreatedAt, &createdMilestone.UpdatedAt,
		&categoryName, &categoryColor, &workspaceName)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	createdMilestone.Description = description.String
	createdMilestone.TargetDate = utils.NullStringToPtr(createdTargetDate)
	createdMilestone.CategoryID = utils.NullInt64ToPtr(categoryID)
	createdMilestone.CategoryName = categoryName.String
	createdMilestone.CategoryColor = categoryColor.String
	createdMilestone.WorkspaceID = utils.NullInt64ToPtr(workspaceID)
	createdMilestone.WorkspaceName = workspaceName.String

	respondJSONCreated(w, createdMilestone)
}

func (h *MilestoneHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var milestone models.Milestone
	if err := json.NewDecoder(r.Body).Decode(&milestone); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if strings.TrimSpace(milestone.Name) == "" {
		http.Error(w, "Milestone name is required", http.StatusBadRequest)
		return
	}

	// Handle empty target_date (set to nil)
	if milestone.TargetDate != nil && strings.TrimSpace(*milestone.TargetDate) == "" {
		milestone.TargetDate = nil
	}

	// Validate status
	validStatuses := []string{"planning", "in-progress", "completed", "cancelled"}
	statusValid := false
	for _, validStatus := range validStatuses {
		if milestone.Status == validStatus {
			statusValid = true
			break
		}
	}
	if !statusValid {
		http.Error(w, "Invalid status", http.StatusBadRequest)
		return
	}

	// Validate global vs workspace constraints
	if milestone.IsGlobal && milestone.WorkspaceID != nil {
		http.Error(w, "Global milestones cannot have a workspace_id", http.StatusBadRequest)
		return
	}
	if !milestone.IsGlobal && milestone.WorkspaceID == nil {
		http.Error(w, "Local milestones must have a workspace_id", http.StatusBadRequest)
		return
	}

	// Validate category_id if provided
	if milestone.CategoryID != nil {
		var categoryExists int
		err := h.db.QueryRow("SELECT COUNT(*) FROM milestone_categories WHERE id = ?", *milestone.CategoryID).Scan(&categoryExists)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if categoryExists == 0 {
			http.Error(w, "Invalid category ID", http.StatusBadRequest)
			return
		}
	}

	// Validate workspace_id if provided
	if milestone.WorkspaceID != nil {
		var workspaceExists int
		err := h.db.QueryRow("SELECT COUNT(*) FROM workspaces WHERE id = ?", *milestone.WorkspaceID).Scan(&workspaceExists)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if workspaceExists == 0 {
			http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
			return
		}
	}

	// Sanitize user input to prevent XSS
	milestone.Name = utils.StripHTMLTags(milestone.Name)
	milestone.Description = utils.StripHTMLTags(milestone.Description)

	now := time.Now()
	_, err := h.db.ExecWrite(`
		UPDATE milestones
		SET name = ?, description = ?, target_date = ?, status = ?, category_id = ?, is_global = ?, workspace_id = ?, updated_at = ?
		WHERE id = ?
	`, milestone.Name, milestone.Description, milestone.TargetDate, milestone.Status, milestone.CategoryID,
		milestone.IsGlobal, milestone.WorkspaceID, now, id)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the updated milestone with category info
	var updatedMilestone models.Milestone
	var description sql.NullString
	var categoryID sql.NullInt64
	var categoryName sql.NullString
	var categoryColor sql.NullString
	var updatedTargetDate sql.NullString
	var workspaceID sql.NullInt64
	var workspaceName sql.NullString

	err = h.db.QueryRow(`
		SELECT m.id, m.name, m.description, m.target_date, m.status, m.category_id,
		       m.is_global, m.workspace_id, m.created_at, m.updated_at,
		       mc.name as category_name, mc.color as category_color, w.name as workspace_name
		FROM milestones m
		LEFT JOIN milestone_categories mc ON m.category_id = mc.id
		LEFT JOIN workspaces w ON m.workspace_id = w.id
		WHERE m.id = ?
	`, id).Scan(&updatedMilestone.ID, &updatedMilestone.Name, &description, &updatedTargetDate,
		&updatedMilestone.Status, &categoryID, &updatedMilestone.IsGlobal, &workspaceID,
		&updatedMilestone.CreatedAt, &updatedMilestone.UpdatedAt,
		&categoryName, &categoryColor, &workspaceName)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	updatedMilestone.Description = description.String
	updatedMilestone.TargetDate = utils.NullStringToPtr(updatedTargetDate)
	updatedMilestone.CategoryID = utils.NullInt64ToPtr(categoryID)
	updatedMilestone.CategoryName = categoryName.String
	updatedMilestone.CategoryColor = categoryColor.String
	updatedMilestone.WorkspaceID = utils.NullInt64ToPtr(workspaceID)
	updatedMilestone.WorkspaceName = workspaceName.String

	respondJSONOK(w, updatedMilestone)
}

func (h *MilestoneHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	_, err := h.db.ExecWrite("DELETE FROM milestones WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *MilestoneHandler) GetTestStatistics(w http.ResponseWriter, r *http.Request) {
	milestoneID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Query test plan statistics for this milestone
	testPlanStats := struct {
		TotalTestPlans     int `json:"total_test_plans"`
		TotalTestRuns      int `json:"total_test_runs"`
		SuccessfulTestRuns int `json:"successful_test_runs"`
		FailedTestRuns     int `json:"failed_test_runs"`
		InProgressTestRuns int `json:"in_progress_test_runs"`
		TotalTestCases     int `json:"total_test_cases"`
	}{}

	// Get test plan counts for this milestone
	err := h.db.QueryRow(`
		SELECT 
			COUNT(DISTINCT ts.id) as total_test_plans,
			COALESCE(SUM(run_stats.total_runs), 0) as total_test_runs,
			COALESCE(SUM(run_stats.successful_runs), 0) as successful_test_runs,
			COALESCE(SUM(run_stats.failed_runs), 0) as failed_test_runs,
			COALESCE(SUM(run_stats.in_progress_runs), 0) as in_progress_test_runs,
			COALESCE(SUM(tc_counts.test_case_count), 0) as total_test_cases
		FROM test_sets ts
		LEFT JOIN (
			SELECT 
				set_id,
				COUNT(*) as total_runs,
				SUM(CASE WHEN ended_at IS NOT NULL THEN 1 ELSE 0 END) as successful_runs,
				SUM(CASE WHEN ended_at IS NULL THEN 1 ELSE 0 END) as failed_runs,
				0 as in_progress_runs
			FROM test_runs
			GROUP BY set_id
		) run_stats ON ts.id = run_stats.set_id
		LEFT JOIN (
			SELECT 
				stc.set_id,
				COUNT(stc.test_case_id) as test_case_count
			FROM set_test_cases stc
			GROUP BY stc.set_id
		) tc_counts ON ts.id = tc_counts.set_id
		WHERE ts.milestone_id = ?
	`, milestoneID).Scan(
		&testPlanStats.TotalTestPlans,
		&testPlanStats.TotalTestRuns,
		&testPlanStats.SuccessfulTestRuns,
		&testPlanStats.FailedTestRuns,
		&testPlanStats.InProgressTestRuns,
		&testPlanStats.TotalTestCases,
	)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSONOK(w, testPlanStats)
}

// MilestoneProgressItem represents a work item in the milestone progress report
type MilestoneProgressItem struct {
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

// MilestoneProgressReport represents the full milestone progress data
type MilestoneProgressReport struct {
	MilestoneID     int                                `json:"milestone_id"`
	MilestoneName   string                             `json:"milestone_name"`
	Description     string                             `json:"description,omitempty"`
	TargetDate      *string                            `json:"target_date,omitempty"`
	Status          string                             `json:"status"`
	CategoryColor   string                             `json:"category_color,omitempty"`
	TotalItems      int                                `json:"total_items"`
	CompletedItems  int                                `json:"completed_items"`
	PercentComplete float64                            `json:"percent_complete"`
	StatusBreakdown []MilestoneStatusBreakdown         `json:"status_breakdown"`
	ItemsByCategory map[string][]MilestoneProgressItem `json:"items_by_category"`
}

// GetProgress handles GET /api/milestones/{id}/progress - returns milestone progress report
func (h *MilestoneHandler) GetProgress(w http.ResponseWriter, r *http.Request) {
	milestoneID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get milestone details
	var report MilestoneProgressReport
	report.MilestoneID = milestoneID
	report.ItemsByCategory = make(map[string][]MilestoneProgressItem)

	var description sql.NullString
	var targetDate sql.NullString
	var categoryColor sql.NullString

	err := h.db.QueryRow(`
		SELECT m.name, m.description, m.target_date, m.status, mc.color
		FROM milestones m
		LEFT JOIN milestone_categories mc ON m.category_id = mc.id
		WHERE m.id = ?
	`, milestoneID).Scan(&report.MilestoneName, &description, &targetDate, &report.Status, &categoryColor)

	if err == sql.ErrNoRows {
		http.Error(w, "Milestone not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	report.Description = description.String
	report.TargetDate = utils.NullStringToPtr(targetDate)
	report.CategoryColor = categoryColor.String

	// Get status breakdown and items grouped by status category
	rows, err := h.db.Query(`
		SELECT
			i.id, i.title, i.workspace_id, w.key as workspace_key, i.workspace_item_number,
			COALESCE(sc.name, 'No Status') as category_name,
			COALESCE(sc.color, '#9ca3af') as category_color,
			COALESCE(sc.is_completed, false) as is_completed,
			COALESCE(s.name, '') as status_name,
			COALESCE(sc.color, '') as status_color,
			COALESCE(p.name, '') as priority_name,
			COALESCE(p.color, '') as priority_color,
			COALESCE(u.first_name || ' ' || u.last_name, '') as assignee_name,
			COALESCE(u.avatar_url, '') as assignee_avatar
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		LEFT JOIN priorities p ON i.priority_id = p.id
		LEFT JOIN users u ON i.assignee_id = u.id
		WHERE i.milestone_id = ?
		ORDER BY sc.name, i.workspace_item_number
	`, milestoneID)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	// Track status breakdown counts
	breakdownMap := make(map[string]*MilestoneStatusBreakdown)

	for rows.Next() {
		var item MilestoneProgressItem
		var categoryName string
		var categoryColorVal string
		var isCompleted bool
		var statusColor sql.NullString
		var priorityColor sql.NullString

		err := rows.Scan(
			&item.ID, &item.Title, &item.WorkspaceID, &item.WorkspaceKey, &item.ItemNumber,
			&categoryName, &categoryColorVal, &isCompleted,
			&item.StatusName, &statusColor,
			&item.PriorityName, &priorityColor,
			&item.AssigneeName, &item.AssigneeAvatar,
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		item.StatusColor = statusColor.String
		item.PriorityColor = priorityColor.String

		// Update breakdown counts
		if _, exists := breakdownMap[categoryName]; !exists {
			breakdownMap[categoryName] = &MilestoneStatusBreakdown{
				CategoryName:  categoryName,
				CategoryColor: categoryColorVal,
				IsCompleted:   isCompleted,
				ItemCount:     0,
			}
		}
		breakdownMap[categoryName].ItemCount++

		// Add item to category group
		report.ItemsByCategory[categoryName] = append(report.ItemsByCategory[categoryName], item)

		// Update totals
		report.TotalItems++
		if isCompleted {
			report.CompletedItems++
		}
	}

	if err := rows.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Convert breakdown map to slice
	report.StatusBreakdown = make([]MilestoneStatusBreakdown, 0, len(breakdownMap))
	for _, breakdown := range breakdownMap {
		report.StatusBreakdown = append(report.StatusBreakdown, *breakdown)
	}

	// Calculate percentage
	if report.TotalItems > 0 {
		report.PercentComplete = float64(report.CompletedItems) / float64(report.TotalItems) * 100.0
	}

	respondJSONOK(w, report)
}