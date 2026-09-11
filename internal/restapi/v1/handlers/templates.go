package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/restapi"
	"windshift/internal/services"
)

// TemplateHandler exposes workspace work-item-template CRUD on the bearer-token
// v1 surface (the item-templates:* scopes). The read side lets agents discover
// the scaffold a type enforces; the write side backs programmatic provisioning.
// Workspace view/edit is enforced in-handler (404-not-403 on permission failure
// to avoid leaking template / workspace existence).
type TemplateHandler struct {
	BaseHandler
	service *services.ItemTemplateService
}

// NewTemplateHandler constructs a v1 TemplateHandler.
func NewTemplateHandler(db database.Database, permissionService *services.PermissionService) *TemplateHandler {
	return &TemplateHandler{
		BaseHandler: NewBaseHandler(db, permissionService),
		service:     services.NewItemTemplateService(db),
	}
}

// --- request payloads ---

type templateCreateRequest struct {
	Name            string `json:"name"`
	DescriptionBody string `json:"description_body"`
	Mode            string `json:"mode"`
	IsActive        *bool  `json:"is_active,omitempty"`
	ItemTypeIDs     []int  `json:"item_type_ids"`
}

type templateUpdateRequest struct {
	Name            *string `json:"name,omitempty"`
	DescriptionBody *string `json:"description_body,omitempty"`
	Mode            *string `json:"mode,omitempty"`
	IsActive        *bool   `json:"is_active,omitempty"`
	ItemTypeIDs     *[]int  `json:"item_type_ids,omitempty"`
}

// --- response shapes ---

type templateListResponse struct {
	Items []models.ItemTemplate `json:"items"`
	// MandatoryTemplateID is set when the request filtered by item_type_id and
	// that type has an active mandatory template, so callers (create modal, CLI)
	// can flag it without re-deriving the invariant.
	MandatoryTemplateID *int `json:"mandatory_template_id,omitempty"`
}

// ListForWorkspace handles GET /rest/api/v1/workspaces/{id}/templates
//
// @Summary      List workspace work-item templates
// @Description  Returns templates defined in the workspace. With ?item_type_id=N, returns only the templates a creator may pick for that type (type-targeted + global) and flags the type's mandatory template.
// @Tags         templates
// @Produce      json
// @Security     BearerAuth
// @Param        id            path      int  true   "Workspace ID"
// @Param        item_type_id  query     int  false  "Filter to templates valid for this item type"
// @Success      200  {object}  handlers.templateListResponse
// @Failure      400  {object}  handlers.ErrorResponse  "Invalid workspace or item_type_id"
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the item-templates:read scope"
// @Failure      404  {object}  handlers.ErrorResponse  "Workspace not found or not visible to caller"
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/templates [get]
func (h *TemplateHandler) ListForWorkspace(w http.ResponseWriter, r *http.Request) {
	wsID, ok := h.RequireWorkspaceViewAccess(w, r)
	if !ok {
		return
	}

	if raw := r.URL.Query().Get("item_type_id"); raw != "" {
		typeID, err := strconv.Atoi(raw)
		if err != nil {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid item_type_id"))
			return
		}
		templates, err := h.service.List(wsID, &typeID)
		if err != nil {
			h.RespondInternalError(w, r)
			return
		}
		resp := templateListResponse{Items: templates}
		if mandatory, merr := h.service.GetMandatory(wsID, typeID); merr == nil {
			resp.MandatoryTemplateID = &mandatory.ID
		} else if !errors.Is(merr, repository.ErrNotFound) {
			h.RespondInternalError(w, r)
			return
		}
		h.RespondOK(w, resp)
		return
	}

	templates, err := h.service.List(wsID, nil)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	h.RespondOK(w, templateListResponse{Items: templates})
}

// CreateInWorkspace handles POST /rest/api/v1/workspaces/{id}/templates
//
// @Summary      Create a workspace work-item template
// @Tags         templates
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                          true  "Workspace ID"
// @Param        body  body      handlers.templateCreateRequest true  "Template to create"
// @Success      201   {object}  models.ItemTemplate
// @Failure      400   {object}  handlers.ErrorResponse  "Invalid body, mode, or item type"
// @Failure      401   {object}  handlers.ErrorResponse
// @Failure      403   {object}  handlers.ErrorResponse  "Token lacks the item-templates:write scope"
// @Failure      404   {object}  handlers.ErrorResponse  "Workspace not found or not editable by caller"
// @Failure      409   {object}  handlers.ErrorResponse  "Name taken, or a mandatory template already exists for the type"
// @Failure      500   {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/templates [post]
func (h *TemplateHandler) CreateInWorkspace(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}
	// Template catalog writes require workspace.admin (a workspace-configuration
	// concern), not merely item-edit — matching the cookie surface.
	wsID, ok := h.RequireWorkspaceAdminAccess(w, r)
	if !ok {
		return
	}
	var req templateCreateRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}
	created, err := h.service.Create(wsID, user.ID, services.ItemTemplateInput{
		Name: req.Name, DescriptionBody: req.DescriptionBody, Mode: req.Mode,
		IsActive: req.IsActive, ItemTypeIDs: req.ItemTypeIDs,
	})
	if err != nil {
		h.respondTemplateWriteError(w, r, err)
		return
	}
	h.Auditor.Log(r, user, logger.ActionTemplateCreate, logger.ResourceItemTemplate, &created.ID, created.Name)
	h.RespondCreated(w, created)
}

// GetInWorkspace handles GET /rest/api/v1/workspaces/{id}/templates/{templateId}
//
// @Summary      Get a workspace work-item template (with full body)
// @Tags         templates
// @Produce      json
// @Security     BearerAuth
// @Param        id          path      int  true  "Workspace ID"
// @Param        templateId  path      int  true  "Template ID"
// @Success      200  {object}  models.ItemTemplate
// @Failure      400  {object}  handlers.ErrorResponse
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the item-templates:read scope"
// @Failure      404  {object}  handlers.ErrorResponse  "Template not found in this workspace or not visible to caller"
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/templates/{templateId} [get]
func (h *TemplateHandler) GetInWorkspace(w http.ResponseWriter, r *http.Request) {
	wsID, templateID, ok := h.resolveWorkspaceTemplateAccess(w, r, h.Perms.CanViewWorkspace)
	if !ok {
		return
	}
	tmpl, err := h.service.Get(templateID)
	if errors.Is(err, repository.ErrNotFound) || (err == nil && tmpl.WorkspaceID != wsID) {
		h.RespondNotFound(w, r)
		return
	}
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	h.RespondOK(w, tmpl)
}

// UpdateInWorkspace handles PUT /rest/api/v1/workspaces/{id}/templates/{templateId}
//
// @Summary      Update a workspace work-item template
// @Description  Partial update: only supplied fields are touched.
// @Tags         templates
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id          path      int                          true  "Workspace ID"
// @Param        templateId  path      int                          true  "Template ID"
// @Param        body        body      handlers.templateUpdateRequest true  "Fields to update"
// @Success      200  {object}  models.ItemTemplate
// @Failure      400  {object}  handlers.ErrorResponse
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the item-templates:write scope"
// @Failure      404  {object}  handlers.ErrorResponse  "Template not found in this workspace or not editable by caller"
// @Failure      409  {object}  handlers.ErrorResponse  "Name taken, or a mandatory template already exists for the type"
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/templates/{templateId} [put]
func (h *TemplateHandler) UpdateInWorkspace(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}
	wsID, templateID, ok := h.resolveWorkspaceTemplateAccess(w, r, h.Perms.CanAdminWorkspace)
	if !ok {
		return
	}
	existing, err := h.service.Get(templateID)
	if errors.Is(err, repository.ErrNotFound) || (err == nil && existing.WorkspaceID != wsID) {
		h.RespondNotFound(w, r)
		return
	}
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	var req templateUpdateRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}

	result, err := h.service.Update(templateID, user.ID, services.ItemTemplatePatch{
		Name: req.Name, DescriptionBody: req.DescriptionBody, Mode: req.Mode,
		IsActive: req.IsActive, ItemTypeIDs: req.ItemTypeIDs,
	})
	if err != nil {
		h.respondTemplateWriteError(w, r, err)
		return
	}
	h.Auditor.Log(r, user, logger.ActionTemplateUpdate, logger.ResourceItemTemplate, &templateID, result.Name)
	h.RespondOK(w, result)
}

// DeleteInWorkspace handles DELETE /rest/api/v1/workspaces/{id}/templates/{templateId}
//
// @Summary      Delete a workspace work-item template
// @Tags         templates
// @Security     BearerAuth
// @Param        id          path  int  true  "Workspace ID"
// @Param        templateId  path  int  true  "Template ID"
// @Success      204  "Template deleted"
// @Failure      400  {object}  handlers.ErrorResponse
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the item-templates:write scope"
// @Failure      404  {object}  handlers.ErrorResponse  "Template not found in this workspace or not editable by caller"
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/templates/{templateId} [delete]
func (h *TemplateHandler) DeleteInWorkspace(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}
	wsID, templateID, ok := h.resolveWorkspaceTemplateAccess(w, r, h.Perms.CanAdminWorkspace)
	if !ok {
		return
	}
	existing, err := h.service.Get(templateID)
	if errors.Is(err, repository.ErrNotFound) || (err == nil && existing.WorkspaceID != wsID) {
		h.RespondNotFound(w, r)
		return
	}
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	if _, err := h.service.Delete(templateID); err != nil {
		h.RespondInternalError(w, r)
		return
	}
	h.Auditor.Log(r, user, logger.ActionTemplateDelete, logger.ResourceItemTemplate, &templateID, existing.Name)
	h.RespondNoContent(w)
}

// --- helpers ---

// resolveWorkspaceTemplateAccess parses {id} (workspace) + {templateId} from
// the path and verifies the caller has the given permission on the workspace.
// Permission failure returns 404 so the caller can't probe workspace IDs.
func (h *TemplateHandler) resolveWorkspaceTemplateAccess(w http.ResponseWriter, r *http.Request, permCheck func(int, int) (bool, error)) (workspaceID, templateID int, ok bool) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return 0, 0, false
	}
	workspaceID, ok = h.ParsePathID(w, r, "id", "workspace ID")
	if !ok {
		return 0, 0, false
	}
	templateID, ok = h.ParsePathID(w, r, "templateId", "template ID")
	if !ok {
		return 0, 0, false
	}
	allowed, err := permCheck(user.ID, workspaceID)
	if err != nil || !allowed {
		h.RespondNotFound(w, r)
		return 0, 0, false
	}
	return workspaceID, templateID, true
}

// respondTemplateWriteError maps repository write errors to client responses.
func (h *TemplateHandler) respondTemplateWriteError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, services.ErrItemTemplateNameRequired):
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeMissingField, "name is required"))
	case errors.Is(err, services.ErrItemTemplateTypeNotFound):
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeValidationFailed, "unknown item type id"))
	case errors.Is(err, repository.ErrInvalidTemplateMode):
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeValidationFailed, "mode must be 'selectable' or 'mandatory'"))
	case errors.Is(err, repository.ErrMandatoryRequiresOneType):
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeValidationFailed, "a mandatory template must target exactly one item type"))
	case errors.Is(err, repository.ErrMandatoryConflict):
		h.RespondError(w, r, restapi.NewAPIError(http.StatusConflict, restapi.ErrCodeValidationFailed, "another active mandatory template already exists for this item type"))
	case errors.Is(err, repository.ErrDuplicateEntry):
		h.RespondError(w, r, restapi.NewAPIError(http.StatusConflict, restapi.ErrCodeValidationFailed, "a template with this name already exists in this workspace"))
	default:
		h.RespondInternalError(w, r)
	}
}
