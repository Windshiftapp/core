package handlers

import (
	"net/http"
	"strings"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/restapi"
	"windshift/internal/restapi/v1/dto"
	"windshift/internal/services"
)

// WorkspaceHandler handles public API requests for workspaces
type WorkspaceHandler struct {
	BaseHandler
	db               database.Database
	workspaceService *services.WorkspaceService
	itemCRUD         *services.ItemCRUDService
}

// NewWorkspaceHandler creates a new workspace handler
func NewWorkspaceHandler(db database.Database, permissionService *services.PermissionService) *WorkspaceHandler {
	return &WorkspaceHandler{
		BaseHandler:      NewBaseHandler(db, permissionService),
		db:               db,
		workspaceService: services.NewWorkspaceService(db),
		itemCRUD:         services.NewItemCRUDService(db),
	}
}

// WorkspaceResponse is the public API representation of a Workspace
type WorkspaceResponse struct {
	ID                      int    `json:"id"`
	Name                    string `json:"name"`
	Key                     string `json:"key"`
	Description             string `json:"description"`
	Active                  bool   `json:"active"`
	IsPersonal              bool   `json:"is_personal"`
	InternalCommentsEnabled bool   `json:"internal_comments_enabled"`
	Icon                    string `json:"icon,omitempty"`
	Color                   string `json:"color,omitempty"`
	CreatedAt               string `json:"created_at"`
	UpdatedAt               string `json:"updated_at"`
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

func toWorkspaceResponse(ws *services.WorkspaceListResult) WorkspaceResponse {
	return WorkspaceResponse{
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
	}
}

// List handles GET /rest/api/v1/workspaces
//
// @Summary      List workspaces visible to the caller
// @Tags         workspaces
// @Produce      json
// @Security     BearerAuth
// @Param        page   query     int     false  "Page number (1-based)"
// @Param        limit  query     int     false  "Items per page (max 100)"
// @Param        sort   query     string  false  "Sort field"
// @Param        order  query     string  false  "Sort order: asc or desc"
// @Success      200    {object}  restapi.PaginatedResponse{data=[]handlers.WorkspaceResponse}
// @Failure      401    {object}  restapi.ErrorResponse
// @Failure      403    {object}  restapi.ErrorResponse  "Token lacks the workspaces:read scope"
// @Failure      500    {object}  restapi.ErrorResponse
// @Router       /workspaces [get]
func (h *WorkspaceHandler) List(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	pagination := h.ParsePagination(r)

	results, total, err := h.workspaceService.List(services.WorkspaceListParams{
		UserID: user.ID,
		Limit:  pagination.Limit,
		Offset: pagination.Offset,
	})
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	var workspaces []WorkspaceResponse
	for _, ws := range results {
		workspaces = append(workspaces, toWorkspaceResponse(&ws))
	}

	if workspaces == nil {
		workspaces = []WorkspaceResponse{}
	}

	h.RespondPaginated(w, workspaces, pagination, total)
}

// Get handles GET /rest/api/v1/workspaces/{id}
//
// @Summary      Get a workspace by ID
// @Description  Returns 404 (not 403) when the workspace exists but isn't visible to the caller — workspace existence is never leaked.
// @Tags         workspaces
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Workspace ID"
// @Success      200  {object}  handlers.WorkspaceResponse
// @Failure      400  {object}  restapi.ErrorResponse  "Invalid workspace ID"
// @Failure      401  {object}  restapi.ErrorResponse
// @Failure      403  {object}  restapi.ErrorResponse  "Token lacks the workspaces:read scope"
// @Failure      404  {object}  restapi.ErrorResponse  "Workspace not found or not visible to caller"
// @Failure      500  {object}  restapi.ErrorResponse
// @Router       /workspaces/{id} [get]
func (h *WorkspaceHandler) Get(w http.ResponseWriter, r *http.Request) {
	wsID, ok := h.RequireWorkspaceViewAccess(w, r)
	if !ok {
		return
	}

	ws, err := h.workspaceService.GetByID(wsID)
	if err != nil {
		h.RespondError(w, r, restapi.ErrWorkspaceNotFound)
		return
	}

	h.RespondOK(w, toWorkspaceResponse(ws))
}

// Create handles POST /rest/api/v1/workspaces
//
// @Summary      Create a workspace
// @Description  Requires the global `workspace.create` permission in addition to the workspaces:write token scope.
// @Tags         workspaces
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      handlers.WorkspaceCreateRequest  true  "Workspace to create"
// @Success      201   {object}  handlers.WorkspaceResponse
// @Failure      400   {object}  restapi.ErrorResponse  "Invalid request body or missing required field"
// @Failure      401   {object}  restapi.ErrorResponse
// @Failure      403   {object}  restapi.ErrorResponse  "Token lacks workspaces:write or caller lacks workspace.create"
// @Failure      409   {object}  restapi.ErrorResponse  "A workspace with this key already exists"
// @Failure      500   {object}  restapi.ErrorResponse
// @Router       /workspaces [post]
func (h *WorkspaceHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	if !h.RequireGlobalPermission(w, r, user.ID, models.PermissionWorkspaceCreate, "workspace.create") {
		return
	}

	var req WorkspaceCreateRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}

	if !h.ValidateRequiredString(w, r, req.Name, "name") {
		return
	}
	if !h.ValidateRequiredString(w, r, req.Key, "key") {
		return
	}

	keyExists, err := h.workspaceService.KeyExists(req.Key)
	if err == nil && keyExists {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusConflict, restapi.ErrCodeAlreadyExists, "Workspace key already exists"))
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
		h.RespondInternalError(w, r)
		return
	}

	h.RespondCreated(w, toWorkspaceResponse(result.Workspace))
}

// Update handles PUT /rest/api/v1/workspaces/{id}
//
// @Summary      Update a workspace
// @Tags         workspaces
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                              true  "Workspace ID"
// @Param        body  body      handlers.WorkspaceUpdateRequest  true  "Fields to update"
// @Success      200   {object}  handlers.WorkspaceResponse
// @Failure      400   {object}  restapi.ErrorResponse  "Invalid workspace ID or request body"
// @Failure      401   {object}  restapi.ErrorResponse
// @Failure      403   {object}  restapi.ErrorResponse  "Token lacks the workspaces:write scope"
// @Failure      404   {object}  restapi.ErrorResponse  "Workspace not found or caller cannot edit it"
// @Failure      500   {object}  restapi.ErrorResponse
// @Router       /workspaces/{id} [put]
func (h *WorkspaceHandler) Update(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	wsID, ok := h.ParsePathID(w, r, "id", "workspace ID")
	if !ok {
		return
	}

	canEdit, err := h.Perms.CanEditWorkspace(user.ID, wsID)
	if err != nil || !canEdit {
		h.RespondError(w, r, restapi.ErrWorkspaceNotFound)
		return
	}

	var req WorkspaceUpdateRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
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
			h.RespondError(w, r, restapi.ErrWorkspaceNotFound)
			return
		}
		h.RespondInternalError(w, r)
		return
	}

	h.RespondOK(w, toWorkspaceResponse(ws))
}

// Delete handles DELETE /rest/api/v1/workspaces/{id}
//
// @Summary      Delete a workspace
// @Tags         workspaces
// @Security     BearerAuth
// @Param        id   path  int  true  "Workspace ID"
// @Success      204  "Workspace deleted"
// @Failure      400  {object}  restapi.ErrorResponse  "Invalid workspace ID"
// @Failure      401  {object}  restapi.ErrorResponse
// @Failure      403  {object}  restapi.ErrorResponse  "Token lacks the workspaces:delete scope"
// @Failure      404  {object}  restapi.ErrorResponse  "Workspace not found or caller cannot delete it"
// @Failure      500  {object}  restapi.ErrorResponse
// @Router       /workspaces/{id} [delete]
func (h *WorkspaceHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	wsID, ok := h.ParsePathID(w, r, "id", "workspace ID")
	if !ok {
		return
	}

	canEdit, _ := h.Perms.CanEditWorkspace(user.ID, wsID)
	if !canEdit {
		h.RespondError(w, r, restapi.ErrWorkspaceNotFound)
		return
	}

	err := h.workspaceService.Delete(wsID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			h.RespondError(w, r, restapi.ErrWorkspaceNotFound)
			return
		}
		h.RespondInternalError(w, r)
		return
	}

	h.RespondNoContent(w)
}

// GetItems handles GET /rest/api/v1/workspaces/{id}/items
//
// @Summary      List items in a workspace
// @Description  Paginated list of items belonging to the given workspace. Sorted newest-first by creation date.
// @Tags         workspaces, items
// @Produce      json
// @Security     BearerAuth
// @Param        id     path      int  true   "Workspace ID"
// @Param        page   query     int  false  "Page number (1-based)"
// @Param        limit  query     int  false  "Items per page (max 100)"
// @Success      200    {object}  restapi.PaginatedResponse{data=[]dto.ItemResponse}
// @Failure      400    {object}  restapi.ErrorResponse  "Invalid workspace ID"
// @Failure      401    {object}  restapi.ErrorResponse
// @Failure      403    {object}  restapi.ErrorResponse  "Token lacks the items:read scope"
// @Failure      404    {object}  restapi.ErrorResponse  "Workspace not found or not visible to caller"
// @Failure      500    {object}  restapi.ErrorResponse
// @Router       /workspaces/{id}/items [get]
func (h *WorkspaceHandler) GetItems(w http.ResponseWriter, r *http.Request) {
	wsID, ok := h.RequireWorkspaceViewAccess(w, r)
	if !ok {
		return
	}

	pagination := h.ParsePagination(r)
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
		h.RespondInternalError(w, r)
		return
	}

	response := dto.MapItemsToResponse(items, baseURL)
	h.RespondPaginated(w, response, pagination, total)
}

// GetStatuses handles GET /rest/api/v1/workspaces/{id}/statuses
//
// @Summary      List statuses configured for a workspace
// @Tags         workspaces, statuses
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Workspace ID"
// @Success      200  {array}   dto.StatusSummary
// @Failure      400  {object}  restapi.ErrorResponse  "Invalid workspace ID"
// @Failure      401  {object}  restapi.ErrorResponse
// @Failure      403  {object}  restapi.ErrorResponse  "Token lacks the workspaces:read scope"
// @Failure      404  {object}  restapi.ErrorResponse  "Workspace not found or not visible to caller"
// @Failure      500  {object}  restapi.ErrorResponse
// @Router       /workspaces/{id}/statuses [get]
func (h *WorkspaceHandler) GetStatuses(w http.ResponseWriter, r *http.Request) {
	wsID, ok := h.RequireWorkspaceViewAccess(w, r)
	if !ok {
		return
	}

	statuses, err := h.workspaceService.GetStatuses(wsID)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	result := mapStatusesToDTO(statuses)
	h.RespondOK(w, result)
}

// ListCompletedStatuses handles GET /rest/api/v1/workspaces/{id}/statuses/completed
//
// @Summary      List completed statuses configured for a workspace
// @Description  Same as GET /workspaces/{id}/statuses but filtered to statuses whose category is marked as completed.
// @Tags         workspaces, statuses
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Workspace ID"
// @Success      200  {array}   dto.StatusSummary
// @Failure      400  {object}  restapi.ErrorResponse  "Invalid workspace ID"
// @Failure      401  {object}  restapi.ErrorResponse
// @Failure      403  {object}  restapi.ErrorResponse  "Token lacks the workspaces:read scope"
// @Failure      404  {object}  restapi.ErrorResponse  "Workspace not found or not visible to caller"
// @Failure      500  {object}  restapi.ErrorResponse
// @Router       /workspaces/{id}/statuses/completed [get]
func (h *WorkspaceHandler) ListCompletedStatuses(w http.ResponseWriter, r *http.Request) {
	wsID, ok := h.RequireWorkspaceViewAccess(w, r)
	if !ok {
		return
	}

	statuses, err := h.workspaceService.GetStatuses(wsID)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	// Filter for completed statuses only
	var completed []models.Status
	for _, s := range statuses {
		if s.IsCompleted {
			completed = append(completed, s)
		}
	}

	result := mapStatusesToDTO(completed)
	h.RespondOK(w, result)
}

// GetItemTypes handles GET /rest/api/v1/workspaces/{id}/item-types
//
// @Summary      List item types configured for a workspace
// @Tags         workspaces, item-types
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Workspace ID"
// @Success      200  {array}   handlers.ItemTypeResponse
// @Failure      400  {object}  restapi.ErrorResponse  "Invalid workspace ID"
// @Failure      401  {object}  restapi.ErrorResponse
// @Failure      403  {object}  restapi.ErrorResponse  "Token lacks the workspaces:read scope"
// @Failure      404  {object}  restapi.ErrorResponse  "Workspace not found or not visible to caller"
// @Failure      500  {object}  restapi.ErrorResponse
// @Router       /workspaces/{id}/item-types [get]
func (h *WorkspaceHandler) GetItemTypes(w http.ResponseWriter, r *http.Request) {
	wsID, ok := h.RequireWorkspaceViewAccess(w, r)
	if !ok {
		return
	}

	types, err := h.workspaceService.GetItemTypes(wsID)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	var result []ItemTypeResponse
	for _, t := range types {
		result = append(result, ItemTypeResponse{
			ID:             t.ID,
			Name:           t.Name,
			Description:    t.Description,
			Icon:           t.Icon,
			Color:          t.Color,
			HierarchyLevel: t.HierarchyLevel,
			SortOrder:      t.SortOrder,
			IsDefault:      t.IsDefault,
		})
	}

	if result == nil {
		result = []ItemTypeResponse{}
	}

	h.RespondOK(w, result)
}

// mapStatusesToDTO converts a slice of models.Status to a slice of dto.StatusSummary.
func mapStatusesToDTO(statuses []models.Status) []dto.StatusSummary {
	result := make([]dto.StatusSummary, 0, len(statuses))
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
	return result
}
