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
	"windshift/internal/restapi/v1/shared"
	"windshift/internal/services"
)

// WorkspaceHandler handles public API requests for workspaces
type WorkspaceHandler struct {
	db               database.Database
	perms            *shared.PermissionHelper
	workspaceService *services.WorkspaceService
	itemCRUD         *services.ItemCRUDService
}

// NewWorkspaceHandler creates a new workspace handler
func NewWorkspaceHandler(db database.Database, permissionService *services.PermissionService) *WorkspaceHandler {
	return &WorkspaceHandler{
		db:               db,
		perms:            shared.NewPermissionHelper(db, permissionService),
		workspaceService: services.NewWorkspaceService(db),
		itemCRUD:         services.NewItemCRUDService(db),
	}
}

// SetWorkspaceService allows injecting a configured workspace service
func (h *WorkspaceHandler) SetWorkspaceService(ws *services.WorkspaceService) {
	h.workspaceService = ws
}

// SetItemCRUDService allows injecting a configured item CRUD service
func (h *WorkspaceHandler) SetItemCRUDService(ic *services.ItemCRUDService) {
	h.itemCRUD = ic
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

	results, total, err := h.workspaceService.List(services.WorkspaceListParams{
		UserID: user.ID,
		Limit:  pagination.Limit,
		Offset: pagination.Offset,
	})
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	// Convert to response format
	var workspaces []WorkspaceResponse
	for _, ws := range results {
		workspaces = append(workspaces, WorkspaceResponse{
			ID:          ws.ID,
			Name:        ws.Name,
			Key:         ws.Key,
			Description: ws.Description,
			Active:      ws.Active,
			IsPersonal:  ws.IsPersonal,
			Icon:        ws.Icon,
			Color:       ws.Color,
			CreatedAt:   ws.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:   ws.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	if workspaces == nil {
		workspaces = []WorkspaceResponse{}
	}

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

	// Check permission first
	canView, _ := h.perms.CanViewWorkspace(user.ID, wsID)
	if !canView {
		restapi.RespondError(w, r, restapi.ErrInsufficientPermission)
		return
	}

	ws, err := h.workspaceService.GetByID(wsID)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrWorkspaceNotFound)
		return
	}

	restapi.RespondOK(w, WorkspaceResponse{
		ID:          ws.ID,
		Name:        ws.Name,
		Key:         ws.Key,
		Description: ws.Description,
		Active:      ws.Active,
		IsPersonal:  ws.IsPersonal,
		Icon:        ws.Icon,
		Color:       ws.Color,
		CreatedAt:   ws.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   ws.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
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

	// Check for duplicate key using service
	keyExists, _ := h.workspaceService.KeyExists(req.Key)
	if keyExists {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusConflict, restapi.ErrCodeAlreadyExists, "Workspace key already exists"))
		return
	}

	result, err := h.workspaceService.Create(services.CreateWorkspaceParams{
		Name:        req.Name,
		Key:         req.Key,
		Description: req.Description,
		Icon:        req.Icon,
		Color:       req.Color,
		CreatorID:   user.ID,
	})
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	ws := result.Workspace
	restapi.RespondCreated(w, WorkspaceResponse{
		ID:          ws.ID,
		Name:        ws.Name,
		Key:         ws.Key,
		Description: ws.Description,
		Active:      ws.Active,
		IsPersonal:  ws.IsPersonal,
		Icon:        ws.Icon,
		Color:       ws.Color,
		CreatedAt:   ws.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   ws.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
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

	ws, err := h.workspaceService.Update(services.UpdateWorkspaceParams{
		ID:          wsID,
		Name:        req.Name,
		Description: req.Description,
		Active:      req.Active,
		Icon:        req.Icon,
		Color:       req.Color,
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			restapi.RespondError(w, r, restapi.ErrWorkspaceNotFound)
			return
		}
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	restapi.RespondOK(w, WorkspaceResponse{
		ID:          ws.ID,
		Name:        ws.Name,
		Key:         ws.Key,
		Description: ws.Description,
		Active:      ws.Active,
		IsPersonal:  ws.IsPersonal,
		Icon:        ws.Icon,
		Color:       ws.Color,
		CreatedAt:   ws.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   ws.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
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

	err = h.workspaceService.Delete(wsID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			restapi.RespondError(w, r, restapi.ErrWorkspaceNotFound)
			return
		}
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

	items, total, err := h.itemCRUD.List(services.ItemListParams{
		WorkspaceIDs: []int{wsID},
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

	statuses, err := h.workspaceService.GetStatuses(wsID)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	// Convert to DTO format
	var result []dto.StatusSummary
	for _, s := range statuses {
		result = append(result, dto.StatusSummary{
			ID:            s.ID,
			Name:          s.Name,
			CategoryID:    s.CategoryID,
			CategoryName:  s.CategoryName,
			CategoryColor: s.CategoryColor,
			IsCompleted:   s.IsCompleted,
		})
	}

	if result == nil {
		result = []dto.StatusSummary{}
	}

	restapi.RespondOK(w, result)
}
