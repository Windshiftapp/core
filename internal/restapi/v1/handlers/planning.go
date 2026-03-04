package handlers

import (
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
	planningService   *services.PlanningService
	itemCRUD          *services.ItemCRUDService
}

func NewMilestoneHandler(db database.Database, permissionService *services.PermissionService) *MilestoneHandler {
	return &MilestoneHandler{
		db:                db,
		permissionService: permissionService,
		planningService:   services.NewPlanningService(db),
		itemCRUD:          services.NewItemCRUDService(db),
	}
}

// SetPlanningService allows injecting a configured planning service
func (h *MilestoneHandler) SetPlanningService(ps *services.PlanningService) {
	h.planningService = ps
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

	results, total, err := h.planningService.ListMilestones(services.MilestoneListParams{
		Limit:  pagination.Limit,
		Offset: pagination.Offset,
	})
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	var milestones []MilestoneResponse
	for _, m := range results {
		milestones = append(milestones, MilestoneResponse{
			ID:            m.ID,
			Name:          m.Name,
			Description:   m.Description,
			TargetDate:    m.TargetDate,
			Status:        m.Status,
			CategoryID:    m.CategoryID,
			CategoryName:  m.CategoryName,
			CategoryColor: m.CategoryColor,
			CreatedAt:     m.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:     m.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	if milestones == nil {
		milestones = []MilestoneResponse{}
	}

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

	m, err := h.planningService.GetMilestone(id)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrNotFound)
		return
	}

	restapi.RespondOK(w, MilestoneResponse{
		ID:            m.ID,
		Name:          m.Name,
		Description:   m.Description,
		TargetDate:    m.TargetDate,
		Status:        m.Status,
		CategoryID:    m.CategoryID,
		CategoryName:  m.CategoryName,
		CategoryColor: m.CategoryColor,
		CreatedAt:     m.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:     m.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

func (h *MilestoneHandler) Create(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	// Check milestone.create permission (REST API v1 milestones are global)
	hasPermission, err := h.permissionService.HasGlobalPermission(user.ID, models.PermissionMilestoneCreate)
	if err != nil || !hasPermission {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusForbidden, "FORBIDDEN", "milestone.create permission required"))
		return
	}

	var req MilestoneCreateRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid JSON body"))
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeMissingField, "name is required"))
		return
	}

	var targetDate *string
	if req.TargetDate != "" {
		targetDate = &req.TargetDate
	}

	m, err := h.planningService.CreateMilestone(services.CreateMilestoneParams{
		Name:        req.Name,
		Description: req.Description,
		TargetDate:  targetDate,
		Status:      req.Status,
		CategoryID:  req.CategoryID,
	})
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	restapi.RespondCreated(w, MilestoneResponse{
		ID:            m.ID,
		Name:          m.Name,
		Description:   m.Description,
		TargetDate:    m.TargetDate,
		Status:        m.Status,
		CategoryID:    m.CategoryID,
		CategoryName:  m.CategoryName,
		CategoryColor: m.CategoryColor,
		CreatedAt:     m.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:     m.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
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
	hasPermission, err := h.permissionService.HasGlobalPermission(user.ID, models.PermissionMilestoneCreate)
	if err != nil || !hasPermission {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusForbidden, "FORBIDDEN", "milestone.create permission required"))
		return
	}

	var req MilestoneCreateRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid JSON body"))
		return
	}

	var updateTargetDate *string
	if req.TargetDate != "" {
		updateTargetDate = &req.TargetDate
	}

	m, err := h.planningService.UpdateMilestone(services.UpdateMilestoneParams{
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
		TargetDate:  updateTargetDate,
		Status:      req.Status,
		CategoryID:  req.CategoryID,
	})
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	restapi.RespondOK(w, MilestoneResponse{
		ID:            m.ID,
		Name:          m.Name,
		Description:   m.Description,
		TargetDate:    m.TargetDate,
		Status:        m.Status,
		CategoryID:    m.CategoryID,
		CategoryName:  m.CategoryName,
		CategoryColor: m.CategoryColor,
		CreatedAt:     m.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:     m.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
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
	hasPermission, err := h.permissionService.HasGlobalPermission(user.ID, models.PermissionMilestoneCreate)
	if err != nil || !hasPermission {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusForbidden, "FORBIDDEN", "milestone.create permission required"))
		return
	}

	err = h.planningService.DeleteMilestone(id)
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

	items, total, err := h.itemCRUD.List(services.ItemListParams{
		Filters: services.ItemFilters{
			MilestoneID: &milestoneID,
		},
		Pagination: services.PaginationParams{
			Limit:  pagination.Limit,
			Offset: pagination.Offset,
		},
		SortBy:  "created_at",
		SortAsc: false,
	})
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	response := dto.MapItemsToResponse(items, baseURL)
	restapi.RespondPaginated(w, response, restapi.NewPaginationMeta(pagination, total))
}

// ========================================
// Iterations Handler
// ========================================

type IterationHandler struct {
	db                database.Database
	permissionService *services.PermissionService
	planningService   *services.PlanningService
}

func NewIterationHandler(db database.Database, permissionService *services.PermissionService) *IterationHandler {
	return &IterationHandler{
		db:                db,
		permissionService: permissionService,
		planningService:   services.NewPlanningService(db),
	}
}

// SetPlanningService allows injecting a configured planning service
func (h *IterationHandler) SetPlanningService(ps *services.PlanningService) {
	h.planningService = ps
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

	results, total, err := h.planningService.ListIterations(services.IterationListParams{
		Limit:  pagination.Limit,
		Offset: pagination.Offset,
	})
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	var iterations []IterationResponse
	for _, iter := range results {
		iterations = append(iterations, IterationResponse{
			ID:          iter.ID,
			Name:        iter.Name,
			Description: iter.Description,
			StartDate:   iter.StartDate,
			EndDate:     iter.EndDate,
			Status:      iter.Status,
			TypeID:      iter.TypeID,
			TypeName:    iter.TypeName,
			TypeColor:   iter.TypeColor,
			IsGlobal:    iter.IsGlobal,
			WorkspaceID: iter.WorkspaceID,
			CreatedAt:   iter.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:   iter.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	if iterations == nil {
		iterations = []IterationResponse{}
	}

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

	iter, err := h.planningService.GetIteration(id)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrNotFound)
		return
	}

	restapi.RespondOK(w, IterationResponse{
		ID:          iter.ID,
		Name:        iter.Name,
		Description: iter.Description,
		StartDate:   iter.StartDate,
		EndDate:     iter.EndDate,
		Status:      iter.Status,
		TypeID:      iter.TypeID,
		TypeName:    iter.TypeName,
		TypeColor:   iter.TypeColor,
		IsGlobal:    iter.IsGlobal,
		WorkspaceID: iter.WorkspaceID,
		CreatedAt:   iter.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   iter.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
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

	iter, err := h.planningService.CreateIteration(services.CreateIterationParams{
		Name:        req.Name,
		Description: req.Description,
		StartDate:   req.StartDate,
		EndDate:     req.EndDate,
		Status:      req.Status,
		TypeID:      req.TypeID,
		IsGlobal:    req.IsGlobal,
		WorkspaceID: req.WorkspaceID,
	})
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	restapi.RespondCreated(w, IterationResponse{
		ID:          iter.ID,
		Name:        iter.Name,
		Description: iter.Description,
		StartDate:   iter.StartDate,
		EndDate:     iter.EndDate,
		Status:      iter.Status,
		TypeID:      iter.TypeID,
		TypeName:    iter.TypeName,
		TypeColor:   iter.TypeColor,
		IsGlobal:    iter.IsGlobal,
		WorkspaceID: iter.WorkspaceID,
		CreatedAt:   iter.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   iter.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
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
	existingIsGlobal, _, err := h.planningService.IsIterationGlobal(id)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrNotFound)
		return
	}

	var req IterationCreateRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
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

	iter, err := h.planningService.UpdateIteration(services.UpdateIterationParams{
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
		StartDate:   req.StartDate,
		EndDate:     req.EndDate,
		Status:      req.Status,
		TypeID:      req.TypeID,
		IsGlobal:    req.IsGlobal,
		WorkspaceID: req.WorkspaceID,
	})
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	restapi.RespondOK(w, IterationResponse{
		ID:          iter.ID,
		Name:        iter.Name,
		Description: iter.Description,
		StartDate:   iter.StartDate,
		EndDate:     iter.EndDate,
		Status:      iter.Status,
		TypeID:      iter.TypeID,
		TypeName:    iter.TypeName,
		TypeColor:   iter.TypeColor,
		IsGlobal:    iter.IsGlobal,
		WorkspaceID: iter.WorkspaceID,
		CreatedAt:   iter.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   iter.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
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
	isGlobal, _, err := h.planningService.IsIterationGlobal(id)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrNotFound)
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

	err = h.planningService.DeleteIteration(id)
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
	db              database.Database
	planningService *services.PlanningService
}

func NewProjectHandler(db database.Database) *ProjectHandler {
	return &ProjectHandler{
		db:              db,
		planningService: services.NewPlanningService(db),
	}
}

// SetPlanningService allows injecting a configured planning service
func (h *ProjectHandler) SetPlanningService(ps *services.PlanningService) {
	h.planningService = ps
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

	results, total, err := h.planningService.ListProjects(services.ProjectListParams{
		Limit:  pagination.Limit,
		Offset: pagination.Offset,
	})
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	var projects []ProjectResponse
	for _, p := range results {
		projects = append(projects, ProjectResponse{
			ID:            p.ID,
			Name:          p.Name,
			Description:   p.Description,
			Active:        p.Active,
			WorkspaceID:   p.WorkspaceID,
			WorkspaceName: p.WorkspaceName,
			CreatedAt:     p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:     p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	if projects == nil {
		projects = []ProjectResponse{}
	}

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

	p, err := h.planningService.GetProject(id)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrNotFound)
		return
	}

	restapi.RespondOK(w, ProjectResponse{
		ID:            p.ID,
		Name:          p.Name,
		Description:   p.Description,
		Active:        p.Active,
		WorkspaceID:   p.WorkspaceID,
		WorkspaceName: p.WorkspaceName,
		CreatedAt:     p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:     p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
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

	p, err := h.planningService.CreateProject(services.CreateProjectParams{
		Name:        req.Name,
		Description: req.Description,
		WorkspaceID: req.WorkspaceID,
		Active:      active,
	})
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	restapi.RespondCreated(w, ProjectResponse{
		ID:            p.ID,
		Name:          p.Name,
		Description:   p.Description,
		Active:        p.Active,
		WorkspaceID:   p.WorkspaceID,
		WorkspaceName: p.WorkspaceName,
		CreatedAt:     p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:     p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
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
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid JSON body"))
		return
	}

	active := true
	if req.Active != nil {
		active = *req.Active
	}

	p, err := h.planningService.UpdateProject(services.UpdateProjectParams{
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
		WorkspaceID: req.WorkspaceID,
		Active:      active,
	})
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	restapi.RespondOK(w, ProjectResponse{
		ID:            p.ID,
		Name:          p.Name,
		Description:   p.Description,
		Active:        p.Active,
		WorkspaceID:   p.WorkspaceID,
		WorkspaceName: p.WorkspaceName,
		CreatedAt:     p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:     p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
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

	err = h.planningService.DeleteProject(id)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	restapi.RespondNoContent(w)
}
