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
	"windshift/internal/services"
	"windshift/internal/utils"
)

type IterationHandler struct {
	db                database.Database
	permissionService *services.PermissionService
}

func NewIterationHandler(db database.Database, permissionService *services.PermissionService) *IterationHandler {
	return &IterationHandler{
		db:                db,
		permissionService: permissionService,
	}
}

func (h *IterationHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Parse query parameters
	workspaceID := r.URL.Query().Get("workspace_id")
	typeID := r.URL.Query().Get("type_id")
	status := r.URL.Query().Get("status")
	includeGlobal := r.URL.Query().Get("include_global") != "false" // Default to true

	// Check workspace permission if workspace_id is specified
	if workspaceID != "" {
		if wsID, err := strconv.Atoi(workspaceID); err == nil {
			if !RequireWorkspacePermission(w, user.ID, wsID, models.PermissionItemView, h.permissionService) {
				return
			}
		}
	} else {
		// For global-only iterations, check global iteration permission
		hasGlobalPerm, err := h.permissionService.HasGlobalPermission(user.ID, models.PermissionIterationManage)
		if err != nil || !hasGlobalPerm {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	}

	query := `
		SELECT i.id, i.name, i.description, i.start_date, i.end_date, i.status,
		       i.type_id, i.is_global, i.workspace_id, i.created_at, i.updated_at,
		       it.name as type_name, it.color as type_color, w.name as workspace_name
		FROM iterations i
		LEFT JOIN iteration_types it ON i.type_id = it.id
		LEFT JOIN workspaces w ON i.workspace_id = w.id
		WHERE 1=1`

	var args []interface{}

	// Filter by workspace - show local iterations for this workspace + global iterations
	if workspaceID != "" {
		if id, err := strconv.Atoi(workspaceID); err == nil {
			if includeGlobal {
				query += " AND (i.workspace_id = ? OR i.is_global = ?)"
				args = append(args, id, true)
			} else {
				query += " AND i.workspace_id = ?"
				args = append(args, id)
			}
		}
	} else if includeGlobal {
		// If no workspace specified, only show global iterations
		query += " AND i.is_global = ?"
		args = append(args, true)
	}

	if typeID != "" {
		if typeID == "null" || typeID == "0" {
			query += " AND i.type_id IS NULL"
		} else {
			query += " AND i.type_id = ?"
			if id, err := strconv.Atoi(typeID); err == nil {
				args = append(args, id)
			}
		}
	}

	if status != "" {
		query += " AND i.status = ?"
		args = append(args, status)
	}

	query += " ORDER BY i.start_date DESC, i.name"

	rows, err := h.db.Query(query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var iterations []models.Iteration
	for rows.Next() {
		var iteration models.Iteration
		var description sql.NullString
		var typeID sql.NullInt64
		var typeName sql.NullString
		var typeColor sql.NullString
		var workspaceID sql.NullInt64
		var workspaceName sql.NullString

		err := rows.Scan(&iteration.ID, &iteration.Name, &description, &iteration.StartDate,
			&iteration.EndDate, &iteration.Status, &typeID, &iteration.IsGlobal,
			&workspaceID, &iteration.CreatedAt, &iteration.UpdatedAt,
			&typeName, &typeColor, &workspaceName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		iteration.Description = description.String
		iteration.TypeID = utils.NullInt64ToPtr(typeID)
		iteration.TypeName = typeName.String
		iteration.TypeColor = typeColor.String
		iteration.WorkspaceID = utils.NullInt64ToPtr(workspaceID)
		iteration.WorkspaceName = workspaceName.String

		iterations = append(iterations, iteration)
	}

	// Always return an array, even if empty
	if iterations == nil {
		iterations = []models.Iteration{}
	}

	respondJSONOK(w, iterations)
}

func (h *IterationHandler) Get(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var iteration models.Iteration
	var description sql.NullString
	var typeID sql.NullInt64
	var typeName sql.NullString
	var typeColor sql.NullString
	var workspaceID sql.NullInt64
	var workspaceName sql.NullString

	err := h.db.QueryRow(`
		SELECT i.id, i.name, i.description, i.start_date, i.end_date, i.status,
		       i.type_id, i.is_global, i.workspace_id, i.created_at, i.updated_at,
		       it.name as type_name, it.color as type_color, w.name as workspace_name
		FROM iterations i
		LEFT JOIN iteration_types it ON i.type_id = it.id
		LEFT JOIN workspaces w ON i.workspace_id = w.id
		WHERE i.id = ?
	`, id).Scan(&iteration.ID, &iteration.Name, &description, &iteration.StartDate,
		&iteration.EndDate, &iteration.Status, &typeID, &iteration.IsGlobal,
		&workspaceID, &iteration.CreatedAt, &iteration.UpdatedAt,
		&typeName, &typeColor, &workspaceName)

	if err == sql.ErrNoRows {
		http.Error(w, "Iteration not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check permission based on whether iteration is global or workspace-scoped
	if iteration.IsGlobal {
		hasGlobalPerm, err := h.permissionService.HasGlobalPermission(user.ID, models.PermissionIterationManage)
		if err != nil || !hasGlobalPerm {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	} else if workspaceID.Valid {
		if !RequireWorkspacePermission(w, user.ID, int(workspaceID.Int64), models.PermissionItemView, h.permissionService) {
			return
		}
	}

	iteration.Description = description.String
	iteration.TypeID = utils.NullInt64ToPtr(typeID)
	iteration.TypeName = typeName.String
	iteration.TypeColor = typeColor.String
	iteration.WorkspaceID = utils.NullInt64ToPtr(workspaceID)
	iteration.WorkspaceName = workspaceName.String

	respondJSONOK(w, iteration)
}

func (h *IterationHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	var iteration models.Iteration
	if err := json.NewDecoder(r.Body).Decode(&iteration); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if strings.TrimSpace(iteration.Name) == "" {
		http.Error(w, "Iteration name is required", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(iteration.StartDate) == "" {
		http.Error(w, "Start date is required", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(iteration.EndDate) == "" {
		http.Error(w, "End date is required", http.StatusBadRequest)
		return
	}

	// Validate status
	validStatuses := []string{"planned", "active", "completed", "cancelled"}
	statusValid := false
	for _, validStatus := range validStatuses {
		if iteration.Status == validStatus {
			statusValid = true
			break
		}
	}
	if !statusValid {
		iteration.Status = "planned" // Default status
	}

	// Validate global vs workspace constraints
	if iteration.IsGlobal && iteration.WorkspaceID != nil {
		http.Error(w, "Global iterations cannot have a workspace_id", http.StatusBadRequest)
		return
	}
	if !iteration.IsGlobal && iteration.WorkspaceID == nil {
		http.Error(w, "Local iterations must have a workspace_id", http.StatusBadRequest)
		return
	}

	// Check permission based on whether iteration is global or workspace-scoped
	if iteration.IsGlobal {
		hasGlobalPerm, err := h.permissionService.HasGlobalPermission(user.ID, models.PermissionIterationManage)
		if err != nil || !hasGlobalPerm {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	} else {
		if !RequireWorkspacePermission(w, user.ID, *iteration.WorkspaceID, models.PermissionItemEdit, h.permissionService) {
			return
		}
	}

	// Validate type_id if provided
	if iteration.TypeID != nil {
		var typeExists int
		err := h.db.QueryRow("SELECT COUNT(*) FROM iteration_types WHERE id = ?", *iteration.TypeID).Scan(&typeExists)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if typeExists == 0 {
			http.Error(w, "Invalid iteration type ID", http.StatusBadRequest)
			return
		}
	}

	// Validate workspace_id if provided
	if iteration.WorkspaceID != nil {
		var workspaceExists int
		err := h.db.QueryRow("SELECT COUNT(*) FROM workspaces WHERE id = ?", *iteration.WorkspaceID).Scan(&workspaceExists)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if workspaceExists == 0 {
			http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
			return
		}
	}

	now := time.Now()
	var id int64
	err := h.db.QueryRow(`
		INSERT INTO iterations (name, description, start_date, end_date, status, type_id, is_global, workspace_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, iteration.Name, iteration.Description, iteration.StartDate, iteration.EndDate, iteration.Status,
		iteration.TypeID, iteration.IsGlobal, iteration.WorkspaceID, now, now).Scan(&id)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the created iteration with joined data
	var createdIteration models.Iteration
	var description sql.NullString
	var typeID sql.NullInt64
	var typeName sql.NullString
	var typeColor sql.NullString
	var workspaceID sql.NullInt64
	var workspaceName sql.NullString

	err = h.db.QueryRow(`
		SELECT i.id, i.name, i.description, i.start_date, i.end_date, i.status,
		       i.type_id, i.is_global, i.workspace_id, i.created_at, i.updated_at,
		       it.name as type_name, it.color as type_color, w.name as workspace_name
		FROM iterations i
		LEFT JOIN iteration_types it ON i.type_id = it.id
		LEFT JOIN workspaces w ON i.workspace_id = w.id
		WHERE i.id = ?
	`, id).Scan(&createdIteration.ID, &createdIteration.Name, &description, &createdIteration.StartDate,
		&createdIteration.EndDate, &createdIteration.Status, &typeID, &createdIteration.IsGlobal,
		&workspaceID, &createdIteration.CreatedAt, &createdIteration.UpdatedAt,
		&typeName, &typeColor, &workspaceName)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	createdIteration.Description = description.String
	createdIteration.TypeID = utils.NullInt64ToPtr(typeID)
	createdIteration.TypeName = typeName.String
	createdIteration.TypeColor = typeColor.String
	createdIteration.WorkspaceID = utils.NullInt64ToPtr(workspaceID)
	createdIteration.WorkspaceName = workspaceName.String

	respondJSONCreated(w, createdIteration)
}

func (h *IterationHandler) Update(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var iteration models.Iteration
	if err := json.NewDecoder(r.Body).Decode(&iteration); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if strings.TrimSpace(iteration.Name) == "" {
		http.Error(w, "Iteration name is required", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(iteration.StartDate) == "" {
		http.Error(w, "Start date is required", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(iteration.EndDate) == "" {
		http.Error(w, "End date is required", http.StatusBadRequest)
		return
	}

	// Validate status
	validStatuses := []string{"planned", "active", "completed", "cancelled"}
	statusValid := false
	for _, validStatus := range validStatuses {
		if iteration.Status == validStatus {
			statusValid = true
			break
		}
	}
	if !statusValid {
		http.Error(w, "Invalid status", http.StatusBadRequest)
		return
	}

	// Validate global vs workspace constraints
	if iteration.IsGlobal && iteration.WorkspaceID != nil {
		http.Error(w, "Global iterations cannot have a workspace_id", http.StatusBadRequest)
		return
	}
	if !iteration.IsGlobal && iteration.WorkspaceID == nil {
		http.Error(w, "Local iterations must have a workspace_id", http.StatusBadRequest)
		return
	}

	// Check permission based on whether iteration is global or workspace-scoped
	if iteration.IsGlobal {
		hasGlobalPerm, err := h.permissionService.HasGlobalPermission(user.ID, models.PermissionIterationManage)
		if err != nil || !hasGlobalPerm {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	} else {
		if !RequireWorkspacePermission(w, user.ID, *iteration.WorkspaceID, models.PermissionItemEdit, h.permissionService) {
			return
		}
	}

	// Validate type_id if provided
	if iteration.TypeID != nil {
		var typeExists int
		err := h.db.QueryRow("SELECT COUNT(*) FROM iteration_types WHERE id = ?", *iteration.TypeID).Scan(&typeExists)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if typeExists == 0 {
			http.Error(w, "Invalid iteration type ID", http.StatusBadRequest)
			return
		}
	}

	// Validate workspace_id if provided
	if iteration.WorkspaceID != nil {
		var workspaceExists int
		err := h.db.QueryRow("SELECT COUNT(*) FROM workspaces WHERE id = ?", *iteration.WorkspaceID).Scan(&workspaceExists)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if workspaceExists == 0 {
			http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
			return
		}
	}

	now := time.Now()
	_, err := h.db.ExecWrite(`
		UPDATE iterations
		SET name = ?, description = ?, start_date = ?, end_date = ?, status = ?, type_id = ?, is_global = ?, workspace_id = ?, updated_at = ?
		WHERE id = ?
	`, iteration.Name, iteration.Description, iteration.StartDate, iteration.EndDate, iteration.Status,
		iteration.TypeID, iteration.IsGlobal, iteration.WorkspaceID, now, id)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the updated iteration with joined data
	var updatedIteration models.Iteration
	var description sql.NullString
	var typeID sql.NullInt64
	var typeName sql.NullString
	var typeColor sql.NullString
	var workspaceID sql.NullInt64
	var workspaceName sql.NullString

	err = h.db.QueryRow(`
		SELECT i.id, i.name, i.description, i.start_date, i.end_date, i.status,
		       i.type_id, i.is_global, i.workspace_id, i.created_at, i.updated_at,
		       it.name as type_name, it.color as type_color, w.name as workspace_name
		FROM iterations i
		LEFT JOIN iteration_types it ON i.type_id = it.id
		LEFT JOIN workspaces w ON i.workspace_id = w.id
		WHERE i.id = ?
	`, id).Scan(&updatedIteration.ID, &updatedIteration.Name, &description, &updatedIteration.StartDate,
		&updatedIteration.EndDate, &updatedIteration.Status, &typeID, &updatedIteration.IsGlobal,
		&workspaceID, &updatedIteration.CreatedAt, &updatedIteration.UpdatedAt,
		&typeName, &typeColor, &workspaceName)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	updatedIteration.Description = description.String
	updatedIteration.TypeID = utils.NullInt64ToPtr(typeID)
	updatedIteration.TypeName = typeName.String
	updatedIteration.TypeColor = typeColor.String
	updatedIteration.WorkspaceID = utils.NullInt64ToPtr(workspaceID)
	updatedIteration.WorkspaceName = workspaceName.String

	respondJSONOK(w, updatedIteration)
}

func (h *IterationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// First, fetch the iteration to check its properties for permission validation
	var isGlobal bool
	var workspaceID sql.NullInt64
	err := h.db.QueryRow("SELECT is_global, workspace_id FROM iterations WHERE id = ?", id).Scan(&isGlobal, &workspaceID)
	if err == sql.ErrNoRows {
		http.Error(w, "Iteration not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check permission based on whether iteration is global or workspace-scoped
	if isGlobal {
		hasGlobalPerm, err := h.permissionService.HasGlobalPermission(user.ID, models.PermissionIterationManage)
		if err != nil || !hasGlobalPerm {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	} else if workspaceID.Valid {
		if !RequireWorkspacePermission(w, user.ID, int(workspaceID.Int64), models.PermissionItemEdit, h.permissionService) {
			return
		}
	}

	_, err = h.db.ExecWrite("DELETE FROM iterations WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// IterationProgressItem represents a work item in the iteration progress report
type IterationProgressItem struct {
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

// IterationStatusBreakdown represents item counts by status category
type IterationStatusBreakdown struct {
	CategoryName  string `json:"category_name"`
	CategoryColor string `json:"category_color,omitempty"`
	ItemCount     int    `json:"item_count"`
	IsCompleted   bool   `json:"is_completed"`
}

// IterationProgressReport represents the full iteration progress data
type IterationProgressReport struct {
	IterationID     int                                 `json:"iteration_id"`
	IterationName   string                              `json:"iteration_name"`
	Description     string                              `json:"description,omitempty"`
	StartDate       string                              `json:"start_date"`
	EndDate         string                              `json:"end_date"`
	Status          string                              `json:"status"`
	TypeColor       string                              `json:"type_color,omitempty"`
	TotalItems      int                                 `json:"total_items"`
	CompletedItems  int                                 `json:"completed_items"`
	PercentComplete float64                             `json:"percent_complete"`
	StatusBreakdown []IterationStatusBreakdown          `json:"status_breakdown"`
	ItemsByCategory map[string][]IterationProgressItem  `json:"items_by_category"`
}

// GetProgress handles GET /api/iterations/{id}/progress - returns iteration progress report
func (h *IterationHandler) GetProgress(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	iterationID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get iteration details including is_global and workspace_id for permission check
	var report IterationProgressReport
	report.IterationID = iterationID
	report.ItemsByCategory = make(map[string][]IterationProgressItem)

	var description sql.NullString
	var typeColor sql.NullString
	var isGlobal bool
	var workspaceID sql.NullInt64

	err := h.db.QueryRow(`
		SELECT i.name, i.description, i.start_date, i.end_date, i.status, it.color, i.is_global, i.workspace_id
		FROM iterations i
		LEFT JOIN iteration_types it ON i.type_id = it.id
		WHERE i.id = ?
	`, iterationID).Scan(&report.IterationName, &description, &report.StartDate, &report.EndDate, &report.Status, &typeColor, &isGlobal, &workspaceID)

	if err == sql.ErrNoRows {
		http.Error(w, "Iteration not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check permission based on whether iteration is global or workspace-scoped
	if isGlobal {
		hasGlobalPerm, err := h.permissionService.HasGlobalPermission(user.ID, models.PermissionIterationManage)
		if err != nil || !hasGlobalPerm {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	} else if workspaceID.Valid {
		if !RequireWorkspacePermission(w, user.ID, int(workspaceID.Int64), models.PermissionItemView, h.permissionService) {
			return
		}
	}

	report.Description = description.String
	report.TypeColor = typeColor.String

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
		WHERE i.iteration_id = ?
		ORDER BY sc.name, i.workspace_item_number
	`, iterationID)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	// Track status breakdown counts
	breakdownMap := make(map[string]*IterationStatusBreakdown)

	for rows.Next() {
		var item IterationProgressItem
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
			breakdownMap[categoryName] = &IterationStatusBreakdown{
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
	report.StatusBreakdown = make([]IterationStatusBreakdown, 0, len(breakdownMap))
	for _, breakdown := range breakdownMap {
		report.StatusBreakdown = append(report.StatusBreakdown, *breakdown)
	}

	// Calculate percentage
	if report.TotalItems > 0 {
		report.PercentComplete = float64(report.CompletedItems) / float64(report.TotalItems) * 100.0
	}

	respondJSONOK(w, report)
}
