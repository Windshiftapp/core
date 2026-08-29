package handlers

import (
	"errors"
	"net/http"

	"windshift/internal/authz"
	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/restapi"
	"windshift/internal/restapi/v1/dto"
	"windshift/internal/sanitize"
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
		workspaceService: services.NewWorkspaceServiceWithAccess(db, authz.New(db, permissionService)),
		itemCRUD:         services.NewItemCRUDService(db),
	}
}

// WorkspaceResponse is the public API representation of a Workspace.
// Warnings carries user-facing strings for any field the handler had
// to sanitize at decode time; the frontend toasts them at info
// severity. omitempty when nothing was modified.
type WorkspaceResponse struct {
	ID                      int      `json:"id"`
	Name                    string   `json:"name"`
	Key                     string   `json:"key"`
	Description             string   `json:"description"`
	Active                  bool     `json:"active"`
	IsPersonal              bool     `json:"is_personal"`
	IsTemplate              bool     `json:"is_template"`
	InternalCommentsEnabled bool     `json:"internal_comments_enabled"`
	Icon                    string   `json:"icon,omitempty"`
	Color                   string   `json:"color,omitempty"`
	CreatedAt               string   `json:"created_at"`
	UpdatedAt               string   `json:"updated_at"`
	Warnings                []string `json:"warnings,omitempty"`
}

// WorkspaceCreateRequest is the request body for creating a workspace
// `template_workspace_id` optionally names a visible template workspace whose
// configuration-set assignment, work-item templates, and seed items are cloned
// into the new workspace.
type WorkspaceCreateRequest struct {
	Name                string `json:"name" validate:"required,max=100"`
	Key                 string `json:"key" validate:"required,min=2,max=10,alphanum"`
	Description         string `json:"description,omitempty"`
	Icon                string `json:"icon,omitempty"`
	Color               string `json:"color,omitempty"`
	TemplateWorkspaceID *int   `json:"template_workspace_id,omitempty"`
}

// WorkspaceUpdateRequest is the request body for updating a workspace
type WorkspaceUpdateRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Active      *bool   `json:"active,omitempty"`
	Icon        *string `json:"icon,omitempty"`
	Color       *string `json:"color,omitempty"`
	IsTemplate  *bool   `json:"is_template,omitempty"`
}

// WorkspaceTemplateSummaryResponse is the public API representation of a
// workspace usable as a creation template.
type WorkspaceTemplateSummaryResponse struct {
	ID                   int    `json:"id"`
	Name                 string `json:"name"`
	Description          string `json:"description"`
	Icon                 string `json:"icon,omitempty"`
	Color                string `json:"color,omitempty"`
	ConfigurationSetName string `json:"configuration_set_name,omitempty"`
	TemplateCount        int    `json:"template_count"`
	ItemCount            int    `json:"item_count"`
}

func toWorkspaceResponse(ws *models.Workspace) WorkspaceResponse {
	return WorkspaceResponse{
		ID:                      ws.ID,
		Name:                    ws.Name,
		Key:                     ws.Key,
		Description:             ws.Description,
		Active:                  ws.Active,
		IsPersonal:              ws.IsPersonal,
		IsTemplate:              ws.IsTemplate,
		InternalCommentsEnabled: ws.InternalCommentsEnabled,
		Icon:                    ws.Icon,
		Color:                   ws.Color,
		CreatedAt:               ws.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:               ws.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
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
// @Success      200    {object}  handlers.PaginatedResponse{data=[]handlers.WorkspaceResponse}
// @Failure      401    {object}  handlers.ErrorResponse
// @Failure      403    {object}  handlers.ErrorResponse  "Token lacks the workspaces:read scope"
// @Failure      500    {object}  handlers.ErrorResponse
// @Router       /workspaces [get]
func (h *WorkspaceHandler) List(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	pagination := h.ParsePagination(r)
	accessibleWorkspaceIDs, err := h.Perms.GetAccessibleWorkspaceIDs(user.ID)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	results, total, err := h.workspaceService.List(services.WorkspaceListParams{
		WorkspaceIDs: accessibleWorkspaceIDs,
		Limit:        pagination.Limit,
		Offset:       pagination.Offset,
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
// @Failure      400  {object}  handlers.ErrorResponse  "Invalid workspace ID"
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the workspaces:read scope"
// @Failure      404  {object}  handlers.ErrorResponse  "Workspace not found or not visible to caller"
// @Failure      500  {object}  handlers.ErrorResponse
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
// @Description  Requires the global `workspace.create` permission in addition to the workspaces:write token scope. Optionally pass template_workspace_id to clone a visible template workspace's configuration-set assignment, work-item templates, and seed items.
// @Tags         workspaces
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      handlers.WorkspaceCreateRequest  true  "Workspace to create"
// @Success      201   {object}  handlers.WorkspaceResponse
// @Failure      400   {object}  handlers.ErrorResponse  "Invalid request body or missing required field"
// @Failure      401   {object}  handlers.ErrorResponse
// @Failure      403   {object}  handlers.ErrorResponse  "Token lacks workspaces:write or caller lacks workspace.create"
// @Failure      404   {object}  handlers.ErrorResponse  "TEMPLATE_WORKSPACE_NOT_FOUND: template workspace missing or not visible"
// @Failure      409   {object}  handlers.ErrorResponse  "A workspace with this key already exists"
// @Failure      422   {object}  handlers.ErrorResponse  "INVALID_WORKSPACE_TEMPLATE or WORKSPACE_TEMPLATE_TOO_LARGE"
// @Failure      500   {object}  handlers.ErrorResponse
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
	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: &req.Name, Policy: sanitize.PlainTextField, Label: "Workspace name"},
		sanitize.Pair{Target: &req.Key, Policy: sanitize.ShortIdentifier, Label: "Workspace key"},
		sanitize.Pair{Target: &req.Description, Policy: sanitize.RichText, Label: "Description"},
		sanitize.Pair{Target: &req.Icon, Policy: sanitize.ShortIdentifier, Label: "Icon"},
		sanitize.Pair{Target: &req.Color, Policy: sanitize.ShortIdentifier, Label: "Color"},
	)

	if !h.ValidateRequiredString(w, r, req.Name, "name") {
		return
	}
	if !h.ValidateRequiredString(w, r, req.Key, "key") {
		return
	}

	result, err := h.workspaceService.Create(r.Context(), services.CreateWorkspaceParams{
		Name:                req.Name,
		Key:                 req.Key,
		Description:         req.Description,
		Icon:                req.Icon,
		Color:               req.Color,
		CreatorID:           user.ID,
		TemplateWorkspaceID: req.TemplateWorkspaceID,
	})
	if err != nil {
		if errors.Is(err, repository.ErrDuplicateEntry) {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusConflict, restapi.ErrCodeAlreadyExists, "Workspace key already exists"))
			return
		}
		if errors.Is(err, services.ErrTemplateWorkspaceNotFound) {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeTemplateWorkspaceNotFound, "Template workspace not found or not visible"))
			return
		}
		if errors.Is(err, services.ErrInvalidWorkspaceTemplate) {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusUnprocessableEntity, restapi.ErrCodeInvalidWorkspaceTemplate, "Workspace cannot be used as a template"))
			return
		}
		if errors.Is(err, services.ErrWorkspaceTemplateTooLarge) {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusUnprocessableEntity, restapi.ErrCodeWorkspaceTemplateTooLarge, "Template workspace exceeds the seed item limit"))
			return
		}
		h.RespondInternalError(w, r)
		return
	}

	if h.PermissionService != nil {
		h.PermissionService.InvalidateActiveWorkspaceCache()
		h.PermissionService.OnEveryoneAccessChanged()
	}

	if req.TemplateWorkspaceID != nil {
		h.Auditor.LogWithDetails(r, user, logger.ActionWorkspaceCreateFromTemplate, logger.ResourceWorkspace,
			&result.Workspace.ID, result.Workspace.Name, map[string]any{
				"source_workspace_id":         result.SourceWorkspaceID,
				"config_set_attached":         result.ConfigSetAttached,
				"templates_copied":            result.TemplatesCopied,
				"items_copied":                result.ItemsCopied,
				"omitted_custom_field_values": result.OmittedCustomFieldValues,
			})
	} else {
		h.Auditor.Log(r, user, logger.ActionWorkspaceCreate, logger.ResourceWorkspace, &result.Workspace.ID, result.Workspace.Name)
	}
	resp := toWorkspaceResponse(result.Workspace)
	resp.Warnings = warnings
	h.RespondCreated(w, resp)
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
// @Failure      400   {object}  handlers.ErrorResponse  "Invalid workspace ID or request body"
// @Failure      401   {object}  handlers.ErrorResponse
// @Failure      403   {object}  handlers.ErrorResponse  "Token lacks the workspaces:write scope"
// @Failure      404   {object}  handlers.ErrorResponse  "Workspace not found or caller cannot edit it"
// @Failure      500   {object}  handlers.ErrorResponse
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

	// Renaming/deactivating the workspace record is a workspace-administration
	// action, not item editing — require workspace.admin, not item.edit.
	canAdmin, err := h.Perms.CanAdminWorkspace(user.ID, wsID)
	if err != nil || !canAdmin {
		h.RespondError(w, r, restapi.ErrWorkspaceNotFound)
		return
	}

	var req WorkspaceUpdateRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}
	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: req.Name, Policy: sanitize.PlainTextField, Label: "Workspace name"},
		sanitize.Pair{Target: req.Description, Policy: sanitize.RichText, Label: "Description"},
		sanitize.Pair{Target: req.Icon, Policy: sanitize.ShortIdentifier, Label: "Icon"},
		sanitize.Pair{Target: req.Color, Policy: sanitize.ShortIdentifier, Label: "Color"},
	)

	ws, err := h.workspaceService.Update(services.UpdateWorkspaceParams{
		ID:          wsID,
		Name:        req.Name,
		Description: req.Description,
		Active:      req.Active,
		Icon:        req.Icon,
		Color:       req.Color,
		IsTemplate:  req.IsTemplate,
	})
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			h.RespondError(w, r, restapi.ErrWorkspaceNotFound)
			return
		}
		if errors.Is(err, services.ErrPersonalWorkspaceTemplate) {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusUnprocessableEntity, restapi.ErrCodeInvalidWorkspaceTemplate, "Personal workspaces cannot be templates"))
			return
		}
		h.RespondInternalError(w, r)
		return
	}

	h.Auditor.Log(r, user, logger.ActionWorkspaceUpdate, logger.ResourceWorkspace, &ws.ID, ws.Name)
	resp := toWorkspaceResponse(ws)
	resp.Warnings = warnings
	h.RespondOK(w, resp)
}

// Delete handles DELETE /rest/api/v1/workspaces/{id}
//
// @Summary      Delete a workspace
// @Tags         workspaces
// @Security     BearerAuth
// @Param        id   path  int  true  "Workspace ID"
// @Success      204  "Workspace deleted"
// @Failure      400  {object}  handlers.ErrorResponse  "Invalid workspace ID"
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the workspaces:delete scope"
// @Failure      404  {object}  handlers.ErrorResponse  "Workspace not found or caller cannot delete it"
// @Failure      500  {object}  handlers.ErrorResponse
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

	// Deleting the workspace is a destructive administration action —
	// require workspace.admin, not item.edit.
	canAdmin, _ := h.Perms.CanAdminWorkspace(user.ID, wsID)
	if !canAdmin {
		h.RespondError(w, r, restapi.ErrWorkspaceNotFound)
		return
	}

	err := h.workspaceService.Delete(wsID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			h.RespondError(w, r, restapi.ErrWorkspaceNotFound)
			return
		}
		h.RespondInternalError(w, r)
		return
	}

	h.Auditor.Log(r, user, logger.ActionWorkspaceDelete, logger.ResourceWorkspace, &wsID, "")
	h.RespondNoContent(w)
}

// ListTemplates handles GET /rest/api/v1/workspace-templates
//
// @Summary      List workspaces usable as creation templates
// @Description  Active, non-personal workspaces marked as templates and visible to the caller, with configuration-set name and copy counts.
// @Tags         workspaces
// @Produce      json
// @Security     BearerAuth
// @Success      200  {array}   handlers.WorkspaceTemplateSummaryResponse
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the workspaces:read scope"
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /workspace-templates [get]
func (h *WorkspaceHandler) ListTemplates(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	summaries, err := h.workspaceService.ListTemplateSummaries(r.Context())
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	visible := make([]WorkspaceTemplateSummaryResponse, 0, len(summaries))
	for _, summary := range summaries {
		canView, err := h.Perms.HasWorkspacePermission(user.ID, summary.ID, models.PermissionItemView)
		if err != nil {
			h.RespondInternalError(w, r)
			return
		}
		if !canView {
			continue
		}
		visible = append(visible, WorkspaceTemplateSummaryResponse{
			ID:                   summary.ID,
			Name:                 summary.Name,
			Description:          summary.Description,
			Icon:                 summary.Icon,
			Color:                summary.Color,
			ConfigurationSetName: summary.ConfigurationSetName,
			TemplateCount:        summary.TemplateCount,
			ItemCount:            summary.ItemCount,
		})
	}

	h.RespondOK(w, visible)
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
// @Success      200    {object}  handlers.PaginatedResponse{data=[]dto.ItemResponse}
// @Failure      400    {object}  handlers.ErrorResponse  "Invalid workspace ID"
// @Failure      401    {object}  handlers.ErrorResponse
// @Failure      403    {object}  handlers.ErrorResponse  "Token lacks the items:read scope"
// @Failure      404    {object}  handlers.ErrorResponse  "Workspace not found or not visible to caller"
// @Failure      500    {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/items [get]
func (h *WorkspaceHandler) GetItems(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}
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

	h.maskProjectNames(user.ID, items)

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
// @Failure      400  {object}  handlers.ErrorResponse  "Invalid workspace ID"
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the workspaces:read scope"
// @Failure      404  {object}  handlers.ErrorResponse  "Workspace not found or not visible to caller"
// @Failure      500  {object}  handlers.ErrorResponse
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
// @Failure      400  {object}  handlers.ErrorResponse  "Invalid workspace ID"
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the workspaces:read scope"
// @Failure      404  {object}  handlers.ErrorResponse  "Workspace not found or not visible to caller"
// @Failure      500  {object}  handlers.ErrorResponse
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
// @Failure      400  {object}  handlers.ErrorResponse  "Invalid workspace ID"
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the workspaces:read scope"
// @Failure      404  {object}  handlers.ErrorResponse  "Workspace not found or not visible to caller"
// @Failure      500  {object}  handlers.ErrorResponse
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

// GetWorkflows handles GET /rest/api/v1/workspaces/{id}/workflows
//
// @Summary      List workflows effective for a workspace
// @Description  Returns the distinct workflows selected by the workspace configuration set and its item-type overrides.
// @Tags         workspaces, workflows
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Workspace ID"
// @Success      200  {array}   handlers.WorkflowResponse
// @Failure      400  {object}  handlers.ErrorResponse  "Invalid workspace ID"
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the workspaces:read scope"
// @Failure      404  {object}  handlers.ErrorResponse  "Workspace not found or not visible to caller"
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/workflows [get]
func (h *WorkspaceHandler) GetWorkflows(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := h.RequireWorkspaceViewAccess(w, r)
	if !ok {
		return
	}

	results, err := services.NewWorkflowService(h.db).ListForWorkspace(workspaceID)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	workflows := make([]WorkflowResponse, 0, len(results))
	for _, wf := range results {
		workflows = append(workflows, WorkflowResponse{
			ID:          wf.ID,
			BuiltinKey:  wf.BuiltinKey,
			Name:        wf.Name,
			Description: wf.Description,
			IsDefault:   wf.IsDefault,
			CreatedAt:   wf.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:   wf.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	h.RespondOK(w, workflows)
}

// GetPriorities handles GET /rest/api/v1/workspaces/{id}/priorities
//
// @Summary      List priorities configured for a workspace
// @Description  Returns the priorities enabled for the workspace's configuration set. Falls back to all priorities when the workspace has no configuration set.
// @Tags         workspaces, priorities
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Workspace ID"
// @Success      200  {array}   handlers.PriorityResponse
// @Failure      400  {object}  handlers.ErrorResponse  "Invalid workspace ID"
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the workspaces:read scope"
// @Failure      404  {object}  handlers.ErrorResponse  "Workspace not found or not visible to caller"
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/priorities [get]
func (h *WorkspaceHandler) GetPriorities(w http.ResponseWriter, r *http.Request) {
	wsID, ok := h.RequireWorkspaceViewAccess(w, r)
	if !ok {
		return
	}

	priorities, err := h.workspaceService.GetPriorities(wsID)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	var result []PriorityResponse
	for _, p := range priorities {
		result = append(result, PriorityResponse{
			ID:          p.ID,
			Name:        p.Name,
			Description: p.Description,
			Icon:        p.Icon,
			Color:       p.Color,
			SortOrder:   p.SortOrder,
			IsDefault:   p.IsDefault,
		})
	}

	if result == nil {
		result = []PriorityResponse{}
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
