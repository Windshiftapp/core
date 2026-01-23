package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/restapi"
	"windshift/internal/restapi/v1/dto"
	"windshift/internal/restapi/v1/middleware"
	"windshift/internal/restapi/v1/shared"
	"windshift/internal/services"
)

// WorkspaceHandler handles public API requests for workspaces
type WorkspaceHandler struct {
	db    database.Database
	perms *shared.PermissionHelper
}

// NewWorkspaceHandler creates a new workspace handler
func NewWorkspaceHandler(db database.Database, permissionService *services.PermissionService) *WorkspaceHandler {
	return &WorkspaceHandler{
		db:    db,
		perms: shared.NewPermissionHelper(db, permissionService),
	}
}

// WorkspaceResponse is the public API representation of a Workspace
type WorkspaceResponse struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Key         string `json:"key"`
	Description string `json:"description"`
	Active      bool   `json:"active"`
	IsPersonal  bool   `json:"is_personal"`
	Icon        string `json:"icon,omitempty"`
	Color       string `json:"color,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// WorkspaceCreateRequest is the request body for creating a workspace
type WorkspaceCreateRequest struct {
	Name        string `json:"name" validate:"required,max=100"`
	Key         string `json:"key" validate:"required,min=2,max=10,alphanum"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Color       string `json:"color,omitempty"`
}

// WorkspaceUpdateRequest is the request body for updating a workspace
type WorkspaceUpdateRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Active      *bool   `json:"active,omitempty"`
	Icon        *string `json:"icon,omitempty"`
	Color       *string `json:"color,omitempty"`
}

// List handles GET /rest/api/v1/workspaces
func (h *WorkspaceHandler) List(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	pagination := restapi.ParsePaginationParams(r)

	rows, err := h.db.Query(`
		SELECT DISTINCT w.id, w.name, w.key, w.description, w.active, w.is_personal,
		       w.icon, w.color, w.created_at, w.updated_at
		FROM workspaces w
		LEFT JOIN user_workspace_roles uwr ON w.id = uwr.workspace_id AND uwr.user_id = ?
		LEFT JOIN (
			SELECT DISTINCT gwr.workspace_id
			FROM group_workspace_roles gwr
			JOIN group_members gm ON gwr.group_id = gm.group_id
			WHERE gm.user_id = ?
		) grp ON w.id = grp.workspace_id
		WHERE w.active = 1
		   OR (w.active = 0 AND uwr.role_id IS NOT NULL)
		   OR (w.active = 0 AND grp.workspace_id IS NOT NULL)
		   OR (w.is_personal = 1 AND w.owner_id = ?)
		ORDER BY w.name
		LIMIT ? OFFSET ?
	`, user.ID, user.ID, user.ID, pagination.Limit, pagination.Offset)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}
	defer rows.Close()

	var workspaces []WorkspaceResponse
	for rows.Next() {
		var ws WorkspaceResponse
		var icon, color sql.NullString
		err := rows.Scan(&ws.ID, &ws.Name, &ws.Key, &ws.Description, &ws.Active, &ws.IsPersonal,
			&icon, &color, &ws.CreatedAt, &ws.UpdatedAt)
		if err != nil {
			continue
		}
		ws.Icon = nullStringValue(icon)
		ws.Color = nullStringValue(color)
		workspaces = append(workspaces, ws)
	}

	// Get total count
	var total int
	h.db.QueryRow(`
		SELECT COUNT(DISTINCT w.id)
		FROM workspaces w
		LEFT JOIN user_workspace_roles uwr ON w.id = uwr.workspace_id AND uwr.user_id = ?
		LEFT JOIN (
			SELECT DISTINCT gwr.workspace_id
			FROM group_workspace_roles gwr
			JOIN group_members gm ON gwr.group_id = gm.group_id
			WHERE gm.user_id = ?
		) grp ON w.id = grp.workspace_id
		WHERE w.active = 1
		   OR (w.active = 0 AND uwr.role_id IS NOT NULL)
		   OR (w.active = 0 AND grp.workspace_id IS NOT NULL)
		   OR (w.is_personal = 1 AND w.owner_id = ?)
	`, user.ID, user.ID, user.ID).Scan(&total)

	restapi.RespondPaginated(w, workspaces, restapi.NewPaginationMeta(pagination, total))
}

// Get handles GET /rest/api/v1/workspaces/{id}
func (h *WorkspaceHandler) Get(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	wsID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid workspace ID"))
		return
	}

	var ws WorkspaceResponse
	var icon, color sql.NullString
	err = h.db.QueryRow(`
		SELECT id, name, key, description, active, is_personal, icon, color, created_at, updated_at
		FROM workspaces WHERE id = ?
	`, wsID).Scan(&ws.ID, &ws.Name, &ws.Key, &ws.Description, &ws.Active, &ws.IsPersonal,
		&icon, &color, &ws.CreatedAt, &ws.UpdatedAt)
	if err == sql.ErrNoRows {
		restapi.RespondError(w, r, restapi.ErrWorkspaceNotFound)
		return
	}
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	// Check permission
	canView, _ := h.perms.CanViewWorkspace(user.ID, wsID)
	if !canView {
		restapi.RespondError(w, r, restapi.ErrInsufficientPermission)
		return
	}

	ws.Icon = nullStringValue(icon)
	ws.Color = nullStringValue(color)

	restapi.RespondOK(w, ws)
}

// Create handles POST /rest/api/v1/workspaces
func (h *WorkspaceHandler) Create(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	// Check workspace.create permission
	hasPermission, _ := h.perms.HasGlobalPermission(user.ID, models.PermissionWorkspaceCreate)
	if !hasPermission {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusForbidden, "FORBIDDEN", "workspace.create permission required"))
		return
	}

	var req WorkspaceCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid JSON body"))
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeMissingField, "name is required"))
		return
	}
	if strings.TrimSpace(req.Key) == "" {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeMissingField, "key is required"))
		return
	}

	// Check for duplicate key
	var exists int
	h.db.QueryRow("SELECT 1 FROM workspaces WHERE key = ?", strings.ToUpper(req.Key)).Scan(&exists)
	if exists == 1 {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusConflict, restapi.ErrCodeAlreadyExists, "Workspace key already exists"))
		return
	}

	result, err := h.db.ExecWrite(`
		INSERT INTO workspaces (name, key, description, icon, color, active)
		VALUES (?, ?, ?, ?, ?, 1)
	`, req.Name, strings.ToUpper(req.Key), req.Description, req.Icon, req.Color)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	id, _ := result.LastInsertId()

	// Grant admin permission to creator
	h.db.ExecWrite(`
		INSERT INTO user_workspace_roles (workspace_id, user_id, role_id, granted_by, granted_at)
		SELECT ?, ?, id, ?, CURRENT_TIMESTAMP FROM workspace_roles WHERE name = 'Administrator'
	`, id, user.ID, user.ID)

	// Return created workspace
	var ws WorkspaceResponse
	h.db.QueryRow(`
		SELECT id, name, key, description, active, is_personal, icon, color, created_at, updated_at
		FROM workspaces WHERE id = ?
	`, id).Scan(&ws.ID, &ws.Name, &ws.Key, &ws.Description, &ws.Active, &ws.IsPersonal,
		&ws.Icon, &ws.Color, &ws.CreatedAt, &ws.UpdatedAt)

	restapi.RespondCreated(w, ws)
}

// Update handles PUT /rest/api/v1/workspaces/{id}
func (h *WorkspaceHandler) Update(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	wsID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid workspace ID"))
		return
	}

	// Check permission
	canEdit, _ := h.perms.CanEditWorkspace(user.ID, wsID)
	if !canEdit {
		restapi.RespondError(w, r, restapi.ErrInsufficientPermission)
		return
	}

	var req WorkspaceUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid JSON body"))
		return
	}

	// Load existing
	var ws models.Workspace
	err = h.db.QueryRow("SELECT id, name, description, active, icon, color FROM workspaces WHERE id = ?", wsID).
		Scan(&ws.ID, &ws.Name, &ws.Description, &ws.Active, &ws.Icon, &ws.Color)
	if err == sql.ErrNoRows {
		restapi.RespondError(w, r, restapi.ErrWorkspaceNotFound)
		return
	}

	// Apply updates
	if req.Name != nil {
		ws.Name = *req.Name
	}
	if req.Description != nil {
		ws.Description = *req.Description
	}
	if req.Active != nil {
		ws.Active = *req.Active
	}
	if req.Icon != nil {
		ws.Icon = *req.Icon
	}
	if req.Color != nil {
		ws.Color = *req.Color
	}

	_, err = h.db.ExecWrite(`
		UPDATE workspaces SET name = ?, description = ?, active = ?, icon = ?, color = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, ws.Name, ws.Description, ws.Active, ws.Icon, ws.Color, wsID)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	// Return updated workspace
	var resp WorkspaceResponse
	h.db.QueryRow(`
		SELECT id, name, key, description, active, is_personal, icon, color, created_at, updated_at
		FROM workspaces WHERE id = ?
	`, wsID).Scan(&resp.ID, &resp.Name, &resp.Key, &resp.Description, &resp.Active, &resp.IsPersonal,
		&resp.Icon, &resp.Color, &resp.CreatedAt, &resp.UpdatedAt)

	restapi.RespondOK(w, resp)
}

// Delete handles DELETE /rest/api/v1/workspaces/{id}
func (h *WorkspaceHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	wsID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid workspace ID"))
		return
	}

	// Check permission (must be admin)
	canEdit, _ := h.perms.CanEditWorkspace(user.ID, wsID)
	if !canEdit {
		restapi.RespondError(w, r, restapi.ErrInsufficientPermission)
		return
	}

	// Check workspace exists
	var exists int
	err = h.db.QueryRow("SELECT 1 FROM workspaces WHERE id = ?", wsID).Scan(&exists)
	if err == sql.ErrNoRows {
		restapi.RespondError(w, r, restapi.ErrWorkspaceNotFound)
		return
	}

	// Delete workspace (cascade will handle related records)
	_, err = h.db.ExecWrite("DELETE FROM workspaces WHERE id = ?", wsID)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	restapi.RespondNoContent(w)
}

// GetItems handles GET /rest/api/v1/workspaces/{id}/items
func (h *WorkspaceHandler) GetItems(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	wsID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid workspace ID"))
		return
	}

	canView, _ := h.perms.CanViewWorkspace(user.ID, wsID)
	if !canView {
		restapi.RespondError(w, r, restapi.ErrInsufficientPermission)
		return
	}

	pagination := restapi.ParsePaginationParams(r)
	baseURL := getBaseURL(r)

	rows, err := h.db.Query(`
		SELECT i.id, i.workspace_id, i.workspace_item_number, i.item_type_id, i.title, i.description,
		       i.status_id, i.priority_id, i.due_date, i.is_task, i.milestone_id, i.iteration_id,
		       i.project_id, i.assignee_id, i.creator_id, i.custom_field_values, i.parent_id,
		       i.created_at, i.updated_at,
		       w.name as workspace_name, w.key as workspace_key,
		       it.name as item_type_name,
		       st.name as status_name,
		       pri.name as priority_name, pri.icon as priority_icon, pri.color as priority_color,
		       COALESCE(assignee.first_name || ' ' || assignee.last_name, '') as assignee_name,
		       COALESCE(assignee.email, '') as assignee_email,
		       COALESCE(creator.first_name || ' ' || creator.last_name, '') as creator_name,
		       COALESCE(creator.email, '') as creator_email,
		       m.name as milestone_name, iter.name as iteration_name, proj.name as project_name
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN item_types it ON i.item_type_id = it.id
		LEFT JOIN statuses st ON i.status_id = st.id
		LEFT JOIN priorities pri ON i.priority_id = pri.id
		LEFT JOIN users assignee ON i.assignee_id = assignee.id
		LEFT JOIN users creator ON i.creator_id = creator.id
		LEFT JOIN milestones m ON i.milestone_id = m.id
		LEFT JOIN iterations iter ON i.iteration_id = iter.id
		LEFT JOIN time_projects proj ON i.project_id = proj.id
		WHERE i.workspace_id = ?
		ORDER BY i.created_at DESC
		LIMIT ? OFFSET ?
	`, wsID, pagination.Limit, pagination.Offset)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}
	defer rows.Close()

	items, _, _ := scanItems(rows)

	var total int
	h.db.QueryRow("SELECT COUNT(*) FROM items WHERE workspace_id = ?", wsID).Scan(&total)

	response := dto.MapItemsToResponse(items, baseURL)
	restapi.RespondPaginated(w, response, restapi.NewPaginationMeta(pagination, total))
}

// GetStatuses handles GET /rest/api/v1/workspaces/{id}/statuses
func (h *WorkspaceHandler) GetStatuses(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	wsID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid workspace ID"))
		return
	}

	canView, _ := h.perms.CanViewWorkspace(user.ID, wsID)
	if !canView {
		restapi.RespondError(w, r, restapi.ErrInsufficientPermission)
		return
	}

	// Get statuses for this workspace via configuration set
	rows, err := h.db.Query(`
		SELECT DISTINCT s.id, s.name, s.description, s.category_id, s.is_default,
		       sc.name as category_name, sc.color as category_color, sc.is_completed
		FROM statuses s
		JOIN status_categories sc ON s.category_id = sc.id
		LEFT JOIN workflow_transitions wt ON s.id = wt.from_status_id OR s.id = wt.to_status_id
		LEFT JOIN workflows wf ON wt.workflow_id = wf.id
		LEFT JOIN configuration_sets cs ON wf.id = cs.workflow_id
		LEFT JOIN workspace_configuration_sets wcs ON cs.id = wcs.configuration_set_id
		WHERE wcs.workspace_id = ? OR wcs.workspace_id IS NULL
		ORDER BY sc.id, s.name
	`, wsID)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}
	defer rows.Close()

	var statuses []dto.StatusSummary
	for rows.Next() {
		var s dto.StatusSummary
		var description sql.NullString
		var isDefault bool
		rows.Scan(&s.ID, &s.Name, &description, &s.CategoryID, &isDefault,
			&s.CategoryName, &s.CategoryColor, &s.IsCompleted)
		statuses = append(statuses, s)
	}

	restapi.RespondOK(w, statuses)
}
