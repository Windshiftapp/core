package handlers

import (
	"errors"
	"net/http"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/restapi"
	"windshift/internal/services"
)

// PageLabelHandler exposes workspace page-label CRUD and page↔label
// attachment endpoints on the bearer-token v1 surface so the ws CLI can
// drive them. Mirrors the cookie-auth handlers/page_labels.go in shape
// but goes through bearer auth + token scopes.
type PageLabelHandler struct {
	BaseHandler
	labels   *services.PageLabelService
	pageAuth *services.PagePermissionService
}

// NewPageLabelHandler constructs a v1 PageLabelHandler.
func NewPageLabelHandler(db database.Database, permissionService *services.PermissionService) *PageLabelHandler {
	repo := repository.NewPageLabelRepository(db)
	return &PageLabelHandler{
		BaseHandler: NewBaseHandler(db, permissionService),
		labels:      services.NewPageLabelService(repo),
		pageAuth:    services.NewPagePermissionService(db, permissionService),
	}
}

// --- request payloads ---

type pageLabelCreateRequest struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

type pageLabelUpdateRequest struct {
	Name  *string `json:"name,omitempty"`
	Color *string `json:"color,omitempty"`
}

type pageLabelSetRequest struct {
	LabelIDs []int `json:"label_ids"`
}

type pageLabelAddRequest struct {
	LabelID int `json:"label_id"`
}

// --- response shapes ---

type pageLabelListResponse struct {
	Items []models.PageLabel `json:"items"`
}

type pageListWithLabelsResponse struct {
	Items []models.PageLabel `json:"items"`
}

// --- endpoints ---

// ListLabels handles GET /rest/api/v1/workspaces/{id}/page-labels
//
// ListLabels returns every page label in the workspace.
//
// @Summary      List page labels in a workspace
// @Tags         pages, labels
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Workspace ID"
// @Success      200  {object}  handlers.pageLabelListResponse
// @Failure      400  {object}  handlers.ErrorResponse  "Invalid workspace ID"
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      404  {object}  handlers.ErrorResponse  "Workspace not found or caller lacks page.view"
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/page-labels [get]
func (h *PageLabelHandler) ListLabels(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}
	wsID, ok := h.ParsePathID(w, r, "id", "workspace ID")
	if !ok {
		return
	}
	if !h.checkWorkspacePerm(w, r, user.ID, wsID, models.PermissionPageView) {
		return
	}

	labels, err := h.labels.List(wsID)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	h.RespondOK(w, pageLabelListResponse{Items: labels})
}

// GetLabel handles GET /rest/api/v1/workspaces/{id}/page-labels/{labelId}
//
// GetLabel returns a single page label by id.
//
// @Summary      Get a page label by ID
// @Tags         pages, labels
// @Produce      json
// @Security     BearerAuth
// @Param        id       path      int  true  "Workspace ID"
// @Param        labelId  path      int  true  "Page-label ID"
// @Success      200      {object}  models.PageLabel
// @Failure      400      {object}  handlers.ErrorResponse  "Invalid workspace or label ID"
// @Failure      401      {object}  handlers.ErrorResponse
// @Failure      404      {object}  handlers.ErrorResponse  "Label not found in this workspace or caller lacks page.view"
// @Failure      500      {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/page-labels/{labelId} [get]
func (h *PageLabelHandler) GetLabel(w http.ResponseWriter, r *http.Request) {
	wsID, labelID, user, ok := h.resolveWorkspaceLabel(w, r)
	if !ok {
		return
	}
	if !h.checkWorkspacePerm(w, r, user.ID, wsID, models.PermissionPageView) {
		return
	}
	label, err := h.labels.Get(wsID, labelID)
	if h.isLabelNotFound(err) {
		h.RespondNotFound(w, r)
		return
	}
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	h.RespondOK(w, label)
}

// CreateLabel handles POST /rest/api/v1/workspaces/{id}/page-labels
//
// CreateLabel inserts a new label. Requires workspace-level page.edit.
//
// @Summary      Create a page label
// @Description  Creates a new page label in the workspace. `color` defaults to a neutral blue if omitted.
// @Tags         pages, labels
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                              true  "Workspace ID"
// @Param        body  body      handlers.pageLabelCreateRequest  true  "Label to create"
// @Success      201   {object}  models.PageLabel
// @Failure      400   {object}  handlers.ErrorResponse  "Invalid request body or missing required field"
// @Failure      401   {object}  handlers.ErrorResponse
// @Failure      404   {object}  handlers.ErrorResponse  "Workspace not found or caller lacks page.edit"
// @Failure      409   {object}  handlers.ErrorResponse  "A page label with this name already exists in this workspace"
// @Failure      500   {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/page-labels [post]
func (h *PageLabelHandler) CreateLabel(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}
	wsID, ok := h.ParsePathID(w, r, "id", "workspace ID")
	if !ok {
		return
	}
	if !h.checkWorkspacePerm(w, r, user.ID, wsID, models.PermissionPageEdit) {
		return
	}

	var req pageLabelCreateRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}
	label, err := h.labels.Create(wsID, req.Name, req.Color)
	if err != nil {
		h.respondLabelMutationError(w, r, err)
		return
	}
	h.Auditor.Log(r, user, logger.ActionPageLabelCreate, logger.ResourcePageLabel, &label.ID, label.Name)
	h.RespondCreated(w, label)
}

// UpdateLabel handles PUT /rest/api/v1/workspaces/{id}/page-labels/{labelId}
//
// UpdateLabel changes name and/or color. Partial update: only supplied
// fields are touched.
//
// @Summary      Update a page label
// @Description  Partial update: only fields supplied in the body are touched. Pass `color` as `""` to reset to the default neutral blue.
// @Tags         pages, labels
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path      int                              true  "Workspace ID"
// @Param        labelId  path      int                              true  "Page-label ID"
// @Param        body     body      handlers.pageLabelUpdateRequest  true  "Fields to update"
// @Success      200      {object}  models.PageLabel
// @Failure      400      {object}  handlers.ErrorResponse  "Invalid request body or invalid field"
// @Failure      401      {object}  handlers.ErrorResponse
// @Failure      404      {object}  handlers.ErrorResponse  "Label not found in this workspace or caller lacks page.edit"
// @Failure      409      {object}  handlers.ErrorResponse  "Another page label in this workspace already has the new name"
// @Failure      500      {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/page-labels/{labelId} [put]
func (h *PageLabelHandler) UpdateLabel(w http.ResponseWriter, r *http.Request) {
	wsID, labelID, user, ok := h.resolveWorkspaceLabel(w, r)
	if !ok {
		return
	}
	if !h.checkWorkspacePerm(w, r, user.ID, wsID, models.PermissionPageEdit) {
		return
	}

	var req pageLabelUpdateRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}

	updated, err := h.labels.Update(wsID, labelID, services.PageLabelUpdate{Name: req.Name, Color: req.Color})
	if err != nil {
		h.respondLabelMutationError(w, r, err)
		return
	}
	h.Auditor.Log(r, &models.User{ID: user.ID, Username: user.Username}, logger.ActionPageLabelUpdate, logger.ResourcePageLabel, &labelID, updated.Name)
	h.RespondOK(w, updated)
}

// DeleteLabel handles DELETE /rest/api/v1/workspaces/{id}/page-labels/{labelId}
//
// DeleteLabel removes a label and cascades the page assignments.
//
// @Summary      Delete a page label
// @Description  Cascade-removes the label from every page it was attached to.
// @Tags         pages, labels
// @Security     BearerAuth
// @Param        id       path  int  true  "Workspace ID"
// @Param        labelId  path  int  true  "Page-label ID"
// @Success      204      "Label deleted"
// @Failure      400      {object}  handlers.ErrorResponse  "Invalid workspace or label ID"
// @Failure      401      {object}  handlers.ErrorResponse
// @Failure      404      {object}  handlers.ErrorResponse  "Label not found in this workspace or caller lacks page.edit"
// @Failure      500      {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/page-labels/{labelId} [delete]
func (h *PageLabelHandler) DeleteLabel(w http.ResponseWriter, r *http.Request) {
	wsID, labelID, user, ok := h.resolveWorkspaceLabel(w, r)
	if !ok {
		return
	}
	if !h.checkWorkspacePerm(w, r, user.ID, wsID, models.PermissionPageEdit) {
		return
	}

	existing, err := h.labels.Delete(wsID, labelID)
	if err != nil {
		h.respondLabelMutationError(w, r, err)
		return
	}
	h.Auditor.Log(r, &models.User{ID: user.ID, Username: user.Username}, logger.ActionPageLabelDelete, logger.ResourcePageLabel, &labelID, existing.Name)
	h.RespondNoContent(w)
}

// ListForPage handles GET /rest/api/v1/workspaces/{id}/pages/{pageId}/labels
//
// ListForPage returns the labels currently attached to a page.
//
// @Summary      List labels attached to a page
// @Tags         pages, labels
// @Produce      json
// @Security     BearerAuth
// @Param        id      path      int  true  "Workspace ID"
// @Param        pageId  path      int  true  "Page ID"
// @Success      200     {object}  handlers.pageListWithLabelsResponse
// @Failure      400     {object}  handlers.ErrorResponse  "Invalid workspace or page ID"
// @Failure      401     {object}  handlers.ErrorResponse
// @Failure      404     {object}  handlers.ErrorResponse  "Page not found or you lack page.view"
// @Failure      500     {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/pages/{pageId}/labels [get]
func (h *PageLabelHandler) ListForPage(w http.ResponseWriter, r *http.Request) {
	_, pageID, ok := h.requireWorkspacePageOp(w, r, services.PageOpView)
	if !ok {
		return
	}
	labels, err := h.labels.ListForPage(pageID)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	h.RespondOK(w, pageListWithLabelsResponse{Items: labels})
}

// SetForPage handles PUT /rest/api/v1/workspaces/{id}/pages/{pageId}/labels
//
// SetForPage atomically replaces the label set on a page.
//
// @Summary      Replace the label set on a page
// @Description  Atomically replaces every label attached to the page. Labels must belong to the same workspace.
// @Tags         pages, labels
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id      path      int                           true  "Workspace ID"
// @Param        pageId  path      int                           true  "Page ID"
// @Param        body    body      handlers.pageLabelSetRequest  true  "Replacement label set"
// @Success      200     {object}  handlers.pageListWithLabelsResponse
// @Failure      400     {object}  handlers.ErrorResponse  "Invalid request body"
// @Failure      401     {object}  handlers.ErrorResponse
// @Failure      404     {object}  handlers.ErrorResponse  "Page not found, you lack page.edit, or a label doesn't belong to the workspace"
// @Failure      500     {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/pages/{pageId}/labels [put]
func (h *PageLabelHandler) SetForPage(w http.ResponseWriter, r *http.Request) {
	wsID, pageID, ok := h.requireWorkspacePageOp(w, r, services.PageOpEdit)
	if !ok {
		return
	}
	var req pageLabelSetRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}
	labels, err := h.labels.SetForPage(wsID, pageID, req.LabelIDs)
	if err != nil {
		h.respondLabelAssignmentError(w, r, err)
		return
	}
	h.RespondOK(w, pageListWithLabelsResponse{Items: labels})
}

// AddToPage handles POST /rest/api/v1/workspaces/{id}/pages/{pageId}/labels
//
// AddToPage attaches a single label to a page.
//
// @Summary      Attach a single label to a page
// @Tags         pages, labels
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id      path      int                           true  "Workspace ID"
// @Param        pageId  path      int                           true  "Page ID"
// @Param        body    body      handlers.pageLabelAddRequest  true  "Label to attach"
// @Success      200     {object}  handlers.pageListWithLabelsResponse
// @Failure      400     {object}  handlers.ErrorResponse  "Invalid request body or missing required field"
// @Failure      401     {object}  handlers.ErrorResponse
// @Failure      404     {object}  handlers.ErrorResponse  "Page not found, you lack page.edit, or label doesn't belong to the workspace"
// @Failure      409     {object}  handlers.ErrorResponse  "Label is already attached to this page"
// @Failure      500     {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/pages/{pageId}/labels [post]
func (h *PageLabelHandler) AddToPage(w http.ResponseWriter, r *http.Request) {
	wsID, pageID, ok := h.requireWorkspacePageOp(w, r, services.PageOpEdit)
	if !ok {
		return
	}
	var req pageLabelAddRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}
	labels, err := h.labels.AddToPage(wsID, pageID, req.LabelID)
	if err != nil {
		h.respondLabelAssignmentError(w, r, err)
		return
	}
	h.RespondOK(w, pageListWithLabelsResponse{Items: labels})
}

// RemoveFromPage handles DELETE /rest/api/v1/workspaces/{id}/pages/{pageId}/labels/{labelId}
//
// RemoveFromPage detaches a single label from a page.
//
// @Summary      Detach a label from a page
// @Tags         pages, labels
// @Security     BearerAuth
// @Param        id       path  int  true  "Workspace ID"
// @Param        pageId   path  int  true  "Page ID"
// @Param        labelId  path  int  true  "Page-label ID"
// @Success      204      "Label detached"
// @Failure      400      {object}  handlers.ErrorResponse  "Invalid workspace, page, or label ID"
// @Failure      401      {object}  handlers.ErrorResponse
// @Failure      404      {object}  handlers.ErrorResponse  "Page not found or you lack page.edit"
// @Failure      500      {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/pages/{pageId}/labels/{labelId} [delete]
func (h *PageLabelHandler) RemoveFromPage(w http.ResponseWriter, r *http.Request) {
	_, pageID, ok := h.requireWorkspacePageOp(w, r, services.PageOpEdit)
	if !ok {
		return
	}
	labelID, ok := h.ParsePathID(w, r, "labelId", "label ID")
	if !ok {
		return
	}
	if err := h.labels.RemoveFromPage(pageID, labelID); err != nil {
		h.RespondInternalError(w, r)
		return
	}
	h.RespondNoContent(w)
}

// --- helpers ---

func (h *PageLabelHandler) resolveWorkspaceLabel(w http.ResponseWriter, r *http.Request) (workspaceID, labelID int, user *userCtx, ok bool) {
	var u *models.User
	u, ok = h.RequireAuth(w, r)
	if !ok {
		return 0, 0, nil, false
	}
	workspaceID, ok = h.ParsePathID(w, r, "id", "workspace ID")
	if !ok {
		return 0, 0, nil, false
	}
	labelID, ok = h.ParsePathID(w, r, "labelId", "label ID")
	if !ok {
		return 0, 0, nil, false
	}
	return workspaceID, labelID, &userCtx{ID: u.ID, Username: u.Username}, true
}

// requireWorkspacePageOp runs the per-page permission check for op and
// pulls {workspaceId} + {pageId}. Page-label attachments don't need the
// authenticated user beyond the permission check, so this helper drops it.
func (h *PageLabelHandler) requireWorkspacePageOp(w http.ResponseWriter, r *http.Request, op string) (workspaceID, pageID int, ok bool) {
	var u *models.User
	u, ok = h.RequireAuth(w, r)
	if !ok {
		return 0, 0, false
	}
	workspaceID, ok = h.ParsePathID(w, r, "id", "workspace ID")
	if !ok {
		return 0, 0, false
	}
	pageID, ok = h.ParsePathID(w, r, "pageId", "page ID")
	if !ok {
		return 0, 0, false
	}
	can, err := h.pageAuth.Can(u.ID, workspaceID, pageID, op)
	if err != nil {
		h.RespondInternalError(w, r)
		return 0, 0, false
	}
	if !can {
		h.RespondNotFound(w, r)
		return 0, 0, false
	}
	return workspaceID, pageID, true
}

func (h *PageLabelHandler) checkWorkspacePerm(w http.ResponseWriter, r *http.Request, userID, workspaceID int, key string) bool {
	has, err := h.pageAuth.HasWorkspacePermissionFor(userID, workspaceID, key)
	if err != nil {
		h.RespondInternalError(w, r)
		return false
	}
	if !has {
		h.RespondNotFound(w, r)
		return false
	}
	return true
}

func (h *PageLabelHandler) isLabelNotFound(err error) bool {
	return errors.Is(err, repository.ErrNotFound) || errors.Is(err, services.ErrPageLabelWorkspaceMismatch)
}

func (h *PageLabelHandler) respondLabelMutationError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case h.isLabelNotFound(err):
		h.RespondNotFound(w, r)
	case errors.Is(err, services.ErrPageLabelNameRequired):
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeMissingField, "name is required"))
	case errors.Is(err, repository.ErrDuplicateEntry):
		h.RespondError(w, r, restapi.NewAPIError(http.StatusConflict, restapi.ErrCodeValidationFailed, "a page label with this name already exists in this workspace"))
	default:
		h.RespondInternalError(w, r)
	}
}

func (h *PageLabelHandler) respondLabelAssignmentError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, services.ErrPageLabelIDRequired):
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeMissingField, "label_id is required"))
	case h.isLabelNotFound(err):
		h.RespondNotFound(w, r)
	case errors.Is(err, repository.ErrDuplicateEntry):
		h.RespondError(w, r, restapi.NewAPIError(http.StatusConflict, restapi.ErrCodeValidationFailed, "label is already attached to this page"))
	default:
		h.RespondInternalError(w, r)
	}
}
