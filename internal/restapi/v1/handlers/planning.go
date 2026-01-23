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
	"windshift/internal/services"
)

// ========================================
// Milestones Handler
// ========================================

type MilestoneHandler struct {
	db                database.Database
	permissionService *services.PermissionService
}

func NewMilestoneHandler(db database.Database, permissionService *services.PermissionService) *MilestoneHandler {
	return &MilestoneHandler{
		db:                db,
		permissionService: permissionService,
	}
}

type MilestoneResponse struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	TargetDate    string `json:"target_date,omitempty"`
	Status        string `json:"status"`
	CategoryID    *int   `json:"category_id,omitempty"`
	CategoryName  string `json:"category_name,omitempty"`
	CategoryColor string `json:"category_color,omitempty"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type MilestoneCreateRequest struct {
	Name        string `json:"name" validate:"required"`
	Description string `json:"description,omitempty"`
	TargetDate  string `json:"target_date,omitempty"`
	Status      string `json:"status,omitempty"`
	CategoryID  *int   `json:"category_id,omitempty"`
}

func (h *MilestoneHandler) List(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	pagination := restapi.ParsePaginationParams(r)

	rows, err := h.db.Query(`
		SELECT m.id, m.name, m.description, m.target_date, m.status, m.category_id,
		       mc.name as category_name, mc.color as category_color,
		       m.created_at, m.updated_at
		FROM milestones m
		LEFT JOIN milestone_categories mc ON m.category_id = mc.id
		ORDER BY m.target_date, m.name
		LIMIT ? OFFSET ?
	`, pagination.Limit, pagination.Offset)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}
	defer rows.Close()

	var milestones []MilestoneResponse
	for rows.Next() {
		var m MilestoneResponse
		var description, targetDate, categoryName, categoryColor sql.NullString
		var categoryID sql.NullInt64
		rows.Scan(&m.ID, &m.Name, &description, &targetDate, &m.Status, &categoryID,
			&categoryName, &categoryColor, &m.CreatedAt, &m.UpdatedAt)
		m.Description = nullStringValue(description)
		m.TargetDate = nullStringValue(targetDate)
		m.CategoryName = nullStringValue(categoryName)
		m.CategoryColor = nullStringValue(categoryColor)
		if categoryID.Valid {
			id := int(categoryID.Int64)
			m.CategoryID = &id
		}
		milestones = append(milestones, m)
	}

	var total int
	h.db.QueryRow("SELECT COUNT(*) FROM milestones").Scan(&total)

	restapi.RespondPaginated(w, milestones, restapi.NewPaginationMeta(pagination, total))
}

func (h *MilestoneHandler) Get(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid milestone ID"))
		return
	}

	var m MilestoneResponse
	var description, targetDate, categoryName, categoryColor sql.NullString
	var categoryID sql.NullInt64
	err = h.db.QueryRow(`
		SELECT m.id, m.name, m.description, m.target_date, m.status, m.category_id,
		       mc.name as category_name, mc.color as category_color,
		       m.created_at, m.updated_at
		FROM milestones m
		LEFT JOIN milestone_categories mc ON m.category_id = mc.id
		WHERE m.id = ?
	`, id).Scan(&m.ID, &m.Name, &description, &targetDate, &m.Status, &categoryID,
		&categoryName, &categoryColor, &m.CreatedAt, &m.UpdatedAt)
	if err == sql.ErrNoRows {
		restapi.RespondError(w, r, restapi.ErrNotFound)
		return
	}
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	m.Description = nullStringValue(description)
	m.TargetDate = nullStringValue(targetDate)
	m.CategoryName = nullStringValue(categoryName)
	m.CategoryColor = nullStringValue(categoryColor)
	if categoryID.Valid {
		cid := int(categoryID.Int64)
		m.CategoryID = &cid
	}

	restapi.RespondOK(w, m)
}

func (h *MilestoneHandler) Create(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	// Check milestone.create permission (REST API v1 milestones are global)
	hasPermission, _ := h.permissionService.HasGlobalPermission(user.ID, models.PermissionMilestoneCreate)
	if !hasPermission {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusForbidden, "FORBIDDEN", "milestone.create permission required"))
		return
	}

	var req MilestoneCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid JSON body"))
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeMissingField, "name is required"))
		return
	}

	status := req.Status
	if status == "" {
		status = "planning"
	}

	result, err := h.db.ExecWrite(`
		INSERT INTO milestones (name, description, target_date, status, category_id)
		VALUES (?, ?, ?, ?, ?)
	`, req.Name, req.Description, req.TargetDate, status, req.CategoryID)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	id, _ := result.LastInsertId()

	var m MilestoneResponse
	h.db.QueryRow(`
		SELECT m.id, m.name, m.description, m.target_date, m.status, m.category_id,
		       mc.name as category_name, mc.color as category_color,
		       m.created_at, m.updated_at
		FROM milestones m
		LEFT JOIN milestone_categories mc ON m.category_id = mc.id
		WHERE m.id = ?
	`, id).Scan(&m.ID, &m.Name, &m.Description, &m.TargetDate, &m.Status, &m.CategoryID,
		&m.CategoryName, &m.CategoryColor, &m.CreatedAt, &m.UpdatedAt)

	restapi.RespondCreated(w, m)
}

func (h *MilestoneHandler) Update(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid milestone ID"))
		return
	}

	// Check milestone.create permission (REST API v1 milestones are global)
	hasPermission, _ := h.permissionService.HasGlobalPermission(user.ID, models.PermissionMilestoneCreate)
	if !hasPermission {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusForbidden, "FORBIDDEN", "milestone.create permission required"))
		return
	}

	var req MilestoneCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid JSON body"))
		return
	}

	_, err = h.db.ExecWrite(`
		UPDATE milestones SET name = ?, description = ?, target_date = ?, status = ?, category_id = ?,
		       updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, req.Name, req.Description, req.TargetDate, req.Status, req.CategoryID, id)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	var m MilestoneResponse
	h.db.QueryRow(`
		SELECT m.id, m.name, m.description, m.target_date, m.status, m.category_id,
		       mc.name as category_name, mc.color as category_color,
		       m.created_at, m.updated_at
		FROM milestones m
		LEFT JOIN milestone_categories mc ON m.category_id = mc.id
		WHERE m.id = ?
	`, id).Scan(&m.ID, &m.Name, &m.Description, &m.TargetDate, &m.Status, &m.CategoryID,
		&m.CategoryName, &m.CategoryColor, &m.CreatedAt, &m.UpdatedAt)

	restapi.RespondOK(w, m)
}

func (h *MilestoneHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid milestone ID"))
		return
	}

	// Check milestone.create permission (REST API v1 milestones are global)
	hasPermission, _ := h.permissionService.HasGlobalPermission(user.ID, models.PermissionMilestoneCreate)
	if !hasPermission {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusForbidden, "FORBIDDEN", "milestone.create permission required"))
		return
	}

	_, err = h.db.ExecWrite("DELETE FROM milestones WHERE id = ?", id)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	restapi.RespondNoContent(w)
}

func (h *MilestoneHandler) GetItems(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	milestoneID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid milestone ID"))
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
		WHERE i.milestone_id = ?
		ORDER BY i.created_at DESC
		LIMIT ? OFFSET ?
	`, milestoneID, pagination.Limit, pagination.Offset)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}
	defer rows.Close()

	items, _, _ := scanItems(rows)

	var total int
	h.db.QueryRow("SELECT COUNT(*) FROM items WHERE milestone_id = ?", milestoneID).Scan(&total)

	response := dto.MapItemsToResponse(items, baseURL)
	restapi.RespondPaginated(w, response, restapi.NewPaginationMeta(pagination, total))
}

// ========================================
// Iterations Handler
// ========================================

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

type IterationResponse struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	StartDate   string `json:"start_date"`
	EndDate     string `json:"end_date"`
	Status      string `json:"status"`
	TypeID      *int   `json:"type_id,omitempty"`
	TypeName    string `json:"type_name,omitempty"`
	TypeColor   string `json:"type_color,omitempty"`
	IsGlobal    bool   `json:"is_global"`
	WorkspaceID *int   `json:"workspace_id,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type IterationCreateRequest struct {
	Name        string `json:"name" validate:"required"`
	Description string `json:"description,omitempty"`
	StartDate   string `json:"start_date" validate:"required"`
	EndDate     string `json:"end_date" validate:"required"`
	Status      string `json:"status,omitempty"`
	TypeID      *int   `json:"type_id,omitempty"`
	IsGlobal    bool   `json:"is_global,omitempty"`
	WorkspaceID *int   `json:"workspace_id,omitempty"`
}

func (h *IterationHandler) List(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	pagination := restapi.ParsePaginationParams(r)

	rows, err := h.db.Query(`
		SELECT i.id, i.name, i.description, i.start_date, i.end_date, i.status,
		       i.type_id, it.name as type_name, it.color as type_color,
		       i.is_global, i.workspace_id, i.created_at, i.updated_at
		FROM iterations i
		LEFT JOIN iteration_types it ON i.type_id = it.id
		ORDER BY i.start_date DESC
		LIMIT ? OFFSET ?
	`, pagination.Limit, pagination.Offset)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}
	defer rows.Close()

	var iterations []IterationResponse
	for rows.Next() {
		var iter IterationResponse
		var description, typeName, typeColor sql.NullString
		var typeID, workspaceID sql.NullInt64
		rows.Scan(&iter.ID, &iter.Name, &description, &iter.StartDate, &iter.EndDate, &iter.Status,
			&typeID, &typeName, &typeColor, &iter.IsGlobal, &workspaceID, &iter.CreatedAt, &iter.UpdatedAt)
		iter.Description = nullStringValue(description)
		iter.TypeName = nullStringValue(typeName)
		iter.TypeColor = nullStringValue(typeColor)
		if typeID.Valid {
			id := int(typeID.Int64)
			iter.TypeID = &id
		}
		if workspaceID.Valid {
			id := int(workspaceID.Int64)
			iter.WorkspaceID = &id
		}
		iterations = append(iterations, iter)
	}

	var total int
	h.db.QueryRow("SELECT COUNT(*) FROM iterations").Scan(&total)

	restapi.RespondPaginated(w, iterations, restapi.NewPaginationMeta(pagination, total))
}

func (h *IterationHandler) Get(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid iteration ID"))
		return
	}

	var iter IterationResponse
	var description, typeName, typeColor sql.NullString
	var typeID, workspaceID sql.NullInt64
	err = h.db.QueryRow(`
		SELECT i.id, i.name, i.description, i.start_date, i.end_date, i.status,
		       i.type_id, it.name as type_name, it.color as type_color,
		       i.is_global, i.workspace_id, i.created_at, i.updated_at
		FROM iterations i
		LEFT JOIN iteration_types it ON i.type_id = it.id
		WHERE i.id = ?
	`, id).Scan(&iter.ID, &iter.Name, &description, &iter.StartDate, &iter.EndDate, &iter.Status,
		&typeID, &typeName, &typeColor, &iter.IsGlobal, &workspaceID, &iter.CreatedAt, &iter.UpdatedAt)
	if err == sql.ErrNoRows {
		restapi.RespondError(w, r, restapi.ErrNotFound)
		return
	}
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	iter.Description = nullStringValue(description)
	iter.TypeName = nullStringValue(typeName)
	iter.TypeColor = nullStringValue(typeColor)
	if typeID.Valid {
		tid := int(typeID.Int64)
		iter.TypeID = &tid
	}
	if workspaceID.Valid {
		wid := int(workspaceID.Int64)
		iter.WorkspaceID = &wid
	}

	restapi.RespondOK(w, iter)
}

func (h *IterationHandler) Create(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	var req IterationCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid JSON body"))
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeMissingField, "name is required"))
		return
	}

	// Check permission based on whether iteration is global or workspace-scoped
	if req.IsGlobal || req.WorkspaceID == nil {
		hasPermission, _ := h.permissionService.HasGlobalPermission(user.ID, models.PermissionIterationManage)
		if !hasPermission {
			restapi.RespondError(w, r, restapi.NewAPIError(http.StatusForbidden, "FORBIDDEN", "iteration.manage permission required"))
			return
		}
	}
	// Note: Workspace-scoped iterations would need workspace permission checks via workspace role

	status := req.Status
	if status == "" {
		status = "planned"
	}

	result, err := h.db.ExecWrite(`
		INSERT INTO iterations (name, description, start_date, end_date, status, type_id, is_global, workspace_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, req.Name, req.Description, req.StartDate, req.EndDate, status, req.TypeID, req.IsGlobal, req.WorkspaceID)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	id, _ := result.LastInsertId()

	var iter IterationResponse
	h.db.QueryRow(`
		SELECT i.id, i.name, i.description, i.start_date, i.end_date, i.status,
		       i.type_id, it.name as type_name, it.color as type_color,
		       i.is_global, i.workspace_id, i.created_at, i.updated_at
		FROM iterations i
		LEFT JOIN iteration_types it ON i.type_id = it.id
		WHERE i.id = ?
	`, id).Scan(&iter.ID, &iter.Name, &iter.Description, &iter.StartDate, &iter.EndDate, &iter.Status,
		&iter.TypeID, &iter.TypeName, &iter.TypeColor, &iter.IsGlobal, &iter.WorkspaceID, &iter.CreatedAt, &iter.UpdatedAt)

	restapi.RespondCreated(w, iter)
}

func (h *IterationHandler) Update(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid iteration ID"))
		return
	}

	// Check if existing iteration is global
	var existingIsGlobal bool
	err = h.db.QueryRow("SELECT is_global FROM iterations WHERE id = ?", id).Scan(&existingIsGlobal)
	if err == sql.ErrNoRows {
		restapi.RespondError(w, r, restapi.ErrNotFound)
		return
	}
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	var req IterationCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid JSON body"))
		return
	}

	// Check permission based on whether iteration is global or workspace-scoped
	// Need permission if either existing or new state is global
	if existingIsGlobal || req.IsGlobal || req.WorkspaceID == nil {
		hasPermission, _ := h.permissionService.HasGlobalPermission(user.ID, models.PermissionIterationManage)
		if !hasPermission {
			restapi.RespondError(w, r, restapi.NewAPIError(http.StatusForbidden, "FORBIDDEN", "iteration.manage permission required"))
			return
		}
	}

	_, err = h.db.ExecWrite(`
		UPDATE iterations SET name = ?, description = ?, start_date = ?, end_date = ?,
		       status = ?, type_id = ?, is_global = ?, workspace_id = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, req.Name, req.Description, req.StartDate, req.EndDate, req.Status, req.TypeID, req.IsGlobal, req.WorkspaceID, id)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	var iter IterationResponse
	h.db.QueryRow(`
		SELECT i.id, i.name, i.description, i.start_date, i.end_date, i.status,
		       i.type_id, it.name as type_name, it.color as type_color,
		       i.is_global, i.workspace_id, i.created_at, i.updated_at
		FROM iterations i
		LEFT JOIN iteration_types it ON i.type_id = it.id
		WHERE i.id = ?
	`, id).Scan(&iter.ID, &iter.Name, &iter.Description, &iter.StartDate, &iter.EndDate, &iter.Status,
		&iter.TypeID, &iter.TypeName, &iter.TypeColor, &iter.IsGlobal, &iter.WorkspaceID, &iter.CreatedAt, &iter.UpdatedAt)

	restapi.RespondOK(w, iter)
}

func (h *IterationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid iteration ID"))
		return
	}

	// Check if existing iteration is global
	var isGlobal bool
	err = h.db.QueryRow("SELECT is_global FROM iterations WHERE id = ?", id).Scan(&isGlobal)
	if err == sql.ErrNoRows {
		restapi.RespondError(w, r, restapi.ErrNotFound)
		return
	}
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	// Check permission based on whether iteration is global
	if isGlobal {
		hasPermission, _ := h.permissionService.HasGlobalPermission(user.ID, models.PermissionIterationManage)
		if !hasPermission {
			restapi.RespondError(w, r, restapi.NewAPIError(http.StatusForbidden, "FORBIDDEN", "iteration.manage permission required"))
			return
		}
	}

	_, err = h.db.ExecWrite("DELETE FROM iterations WHERE id = ?", id)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	restapi.RespondNoContent(w)
}

// ========================================
// Projects Handler
// ========================================

type ProjectHandler struct {
	db database.Database
}

func NewProjectHandler(db database.Database) *ProjectHandler {
	return &ProjectHandler{db: db}
}

type ProjectResponse struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	Active        bool   `json:"active"`
	WorkspaceID   *int   `json:"workspace_id,omitempty"`
	WorkspaceName string `json:"workspace_name,omitempty"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type ProjectCreateRequest struct {
	Name        string `json:"name" validate:"required"`
	Description string `json:"description,omitempty"`
	WorkspaceID *int   `json:"workspace_id,omitempty"`
	Active      *bool  `json:"active,omitempty"`
}

func (h *ProjectHandler) List(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	pagination := restapi.ParsePaginationParams(r)

	rows, err := h.db.Query(`
		SELECT p.id, p.name, p.description, p.active, p.workspace_id,
		       w.name as workspace_name, p.created_at, p.updated_at
		FROM projects p
		LEFT JOIN workspaces w ON p.workspace_id = w.id
		ORDER BY p.name
		LIMIT ? OFFSET ?
	`, pagination.Limit, pagination.Offset)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}
	defer rows.Close()

	var projects []ProjectResponse
	for rows.Next() {
		var p ProjectResponse
		var description, workspaceName sql.NullString
		var workspaceID sql.NullInt64
		rows.Scan(&p.ID, &p.Name, &description, &p.Active, &workspaceID, &workspaceName, &p.CreatedAt, &p.UpdatedAt)
		p.Description = nullStringValue(description)
		p.WorkspaceName = nullStringValue(workspaceName)
		if workspaceID.Valid {
			id := int(workspaceID.Int64)
			p.WorkspaceID = &id
		}
		projects = append(projects, p)
	}

	var total int
	h.db.QueryRow("SELECT COUNT(*) FROM projects").Scan(&total)

	restapi.RespondPaginated(w, projects, restapi.NewPaginationMeta(pagination, total))
}

func (h *ProjectHandler) Get(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid project ID"))
		return
	}

	var p ProjectResponse
	var description, workspaceName sql.NullString
	var workspaceID sql.NullInt64
	err = h.db.QueryRow(`
		SELECT p.id, p.name, p.description, p.active, p.workspace_id,
		       w.name as workspace_name, p.created_at, p.updated_at
		FROM projects p
		LEFT JOIN workspaces w ON p.workspace_id = w.id
		WHERE p.id = ?
	`, id).Scan(&p.ID, &p.Name, &description, &p.Active, &workspaceID, &workspaceName, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		restapi.RespondError(w, r, restapi.ErrNotFound)
		return
	}
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	p.Description = nullStringValue(description)
	p.WorkspaceName = nullStringValue(workspaceName)
	if workspaceID.Valid {
		wid := int(workspaceID.Int64)
		p.WorkspaceID = &wid
	}

	restapi.RespondOK(w, p)
}

func (h *ProjectHandler) Create(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	var req ProjectCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid JSON body"))
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeMissingField, "name is required"))
		return
	}

	active := true
	if req.Active != nil {
		active = *req.Active
	}

	result, err := h.db.ExecWrite(`
		INSERT INTO projects (name, description, workspace_id, active)
		VALUES (?, ?, ?, ?)
	`, req.Name, req.Description, req.WorkspaceID, active)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	id, _ := result.LastInsertId()

	var p ProjectResponse
	h.db.QueryRow(`
		SELECT p.id, p.name, p.description, p.active, p.workspace_id,
		       w.name as workspace_name, p.created_at, p.updated_at
		FROM projects p
		LEFT JOIN workspaces w ON p.workspace_id = w.id
		WHERE p.id = ?
	`, id).Scan(&p.ID, &p.Name, &p.Description, &p.Active, &p.WorkspaceID, &p.WorkspaceName, &p.CreatedAt, &p.UpdatedAt)

	restapi.RespondCreated(w, p)
}

func (h *ProjectHandler) Update(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid project ID"))
		return
	}

	var req ProjectCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid JSON body"))
		return
	}

	active := true
	if req.Active != nil {
		active = *req.Active
	}

	_, err = h.db.ExecWrite(`
		UPDATE projects SET name = ?, description = ?, workspace_id = ?, active = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, req.Name, req.Description, req.WorkspaceID, active, id)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	var p ProjectResponse
	h.db.QueryRow(`
		SELECT p.id, p.name, p.description, p.active, p.workspace_id,
		       w.name as workspace_name, p.created_at, p.updated_at
		FROM projects p
		LEFT JOIN workspaces w ON p.workspace_id = w.id
		WHERE p.id = ?
	`, id).Scan(&p.ID, &p.Name, &p.Description, &p.Active, &p.WorkspaceID, &p.WorkspaceName, &p.CreatedAt, &p.UpdatedAt)

	restapi.RespondOK(w, p)
}

func (h *ProjectHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid project ID"))
		return
	}

	_, err = h.db.ExecWrite("DELETE FROM projects WHERE id = ?", id)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	restapi.RespondNoContent(w)
}
