package handlers

import (
	"errors"
	"net/http"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/restapi"
	"windshift/internal/restapi/v1/middleware"
	"windshift/internal/services"
)

// LabelHandler exposes the global item-label catalog and item assignments on
// the bearer-token v1 surface. Workspace paths provide the authorization
// context for catalog access.
type LabelHandler struct {
	BaseHandler
	application *services.LabelApplicationService
	itemRepo    *repository.ItemRepository
}

// NewLabelHandler constructs a v1 LabelHandler.
func NewLabelHandler(db database.Database, permissionService *services.PermissionService) *LabelHandler {
	return &LabelHandler{
		BaseHandler: NewBaseHandler(db, permissionService),
		application: services.NewLabelApplicationService(db),
		itemRepo:    repository.NewItemRepository(db),
	}
}

// --- request payloads ---

type labelCreateRequest struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

type labelUpdateRequest struct {
	Name  *string `json:"name,omitempty"`
	Color *string `json:"color,omitempty"`
}

type itemLabelSetRequest struct {
	LabelIDs []int `json:"label_ids"`
}

type itemLabelAddRequest struct {
	LabelID int `json:"label_id"`
}

// --- response shapes ---

type labelListResponse struct {
	Items []models.Label `json:"items"`
}

// --- global catalog with workspace authorization context ---

// ListForWorkspace handles GET /rest/api/v1/workspaces/{id}/labels
//
// @Summary      List global labels
// @Description  Returns the global item-label catalog alphabetically. The workspace path controls access.
// @Tags         labels
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Workspace ID"
// @Success      200  {object}  handlers.labelListResponse
// @Failure      400  {object}  handlers.ErrorResponse  "Invalid workspace ID"
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the items:read scope"
// @Failure      404  {object}  handlers.ErrorResponse  "Workspace not found or not visible to caller"
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/labels [get]
func (h *LabelHandler) ListForWorkspace(w http.ResponseWriter, r *http.Request) {
	_, ok := h.RequireWorkspaceViewAccess(w, r)
	if !ok {
		return
	}
	labels, err := h.application.List()
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	h.RespondOK(w, labelListResponse{Items: labels})
}

// CreateInWorkspace handles POST /rest/api/v1/workspaces/{id}/labels
//
// @Summary      Create a global label
// @Description  Creates a global item label. The workspace path controls access; `color` defaults to neutral blue.
// @Tags         labels
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                       true  "Workspace ID"
// @Param        body  body      handlers.labelCreateRequest true  "Label to create"
// @Success      201   {object}  models.Label
// @Failure      400   {object}  handlers.ErrorResponse  "Invalid request body or missing required field"
// @Failure      401   {object}  handlers.ErrorResponse
// @Failure      403   {object}  handlers.ErrorResponse  "Token lacks the items:write scope"
// @Failure      404   {object}  handlers.ErrorResponse  "Workspace not found or not editable by caller"
// @Failure      409   {object}  handlers.ErrorResponse  "A global label with this name already exists"
// @Failure      500   {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/labels [post]
func (h *LabelHandler) CreateInWorkspace(w http.ResponseWriter, r *http.Request) {
	_, ok := h.RequireWorkspaceEditAccess(w, r)
	if !ok {
		return
	}
	var req labelCreateRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}
	label, err := h.application.Create(h.AuditActor(r, middleware.GetUser(r.Context())), req.Name, req.Color)
	if err != nil {
		h.respondCatalogMutationError(w, r, err)
		return
	}
	h.RespondCreated(w, label)
}

// GetInWorkspace handles GET /rest/api/v1/workspaces/{id}/labels/{labelId}
//
// @Summary      Get a global label by ID
// @Tags         labels
// @Produce      json
// @Security     BearerAuth
// @Param        id       path      int  true  "Workspace ID"
// @Param        labelId  path      int  true  "Label ID"
// @Success      200      {object}  models.Label
// @Failure      400      {object}  handlers.ErrorResponse  "Invalid workspace or label ID"
// @Failure      401      {object}  handlers.ErrorResponse
// @Failure      403      {object}  handlers.ErrorResponse  "Token lacks the items:read scope"
// @Failure      404      {object}  handlers.ErrorResponse  "Workspace not visible or label not found"
// @Failure      500      {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/labels/{labelId} [get]
func (h *LabelHandler) GetInWorkspace(w http.ResponseWriter, r *http.Request) {
	labelID, ok := h.resolveWorkspaceLabelAccess(w, r, h.Perms.CanViewWorkspace)
	if !ok {
		return
	}
	label, err := h.application.Get(labelID)
	if errors.Is(err, repository.ErrNotFound) {
		h.RespondNotFound(w, r)
		return
	}
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	h.RespondOK(w, label)
}

// UpdateInWorkspace handles PUT /rest/api/v1/workspaces/{id}/labels/{labelId}
//
// @Summary      Update a global label
// @Description  Updates a global label. The workspace path controls access. Pass `color` as `""` to reset it.
// @Tags         labels
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path      int                         true  "Workspace ID"
// @Param        labelId  path      int                         true  "Label ID"
// @Param        body     body      handlers.labelUpdateRequest true  "Fields to update"
// @Success      200      {object}  models.Label
// @Failure      400      {object}  handlers.ErrorResponse  "Invalid request body or invalid field"
// @Failure      401      {object}  handlers.ErrorResponse
// @Failure      403      {object}  handlers.ErrorResponse  "Token lacks the items:write scope"
// @Failure      404      {object}  handlers.ErrorResponse  "Workspace not editable or label not found"
// @Failure      409      {object}  handlers.ErrorResponse  "Another global label already has the new name"
// @Failure      500      {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/labels/{labelId} [put]
func (h *LabelHandler) UpdateInWorkspace(w http.ResponseWriter, r *http.Request) {
	labelID, ok := h.resolveWorkspaceLabelAccess(w, r, h.Perms.CanEditWorkspace)
	if !ok {
		return
	}
	var req labelUpdateRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}
	user := middleware.GetUser(r.Context())
	updated, err := h.application.Update(h.AuditActor(r, user), labelID, services.LabelUpdate{
		Name: req.Name, Color: req.Color,
	})
	if err != nil {
		h.respondCatalogMutationError(w, r, err)
		return
	}
	h.RespondOK(w, updated)
}

// DeleteInWorkspace handles DELETE /rest/api/v1/workspaces/{id}/labels/{labelId}
//
// @Summary      Delete a global label
// @Description  Cascade-removes the label from every item it was attached to.
// @Tags         labels
// @Security     BearerAuth
// @Param        id       path      int  true  "Workspace ID"
// @Param        labelId  path      int  true  "Label ID"
// @Success      204      "Label deleted"
// @Failure      400      {object}  handlers.ErrorResponse  "Invalid workspace or label ID"
// @Failure      401      {object}  handlers.ErrorResponse
// @Failure      403      {object}  handlers.ErrorResponse  "Token lacks the items:write scope"
// @Failure      404      {object}  handlers.ErrorResponse  "Workspace not editable or label not found"
// @Failure      500      {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/labels/{labelId} [delete]
func (h *LabelHandler) DeleteInWorkspace(w http.ResponseWriter, r *http.Request) {
	labelID, ok := h.resolveWorkspaceLabelAccess(w, r, h.Perms.CanEditWorkspace)
	if !ok {
		return
	}
	user := middleware.GetUser(r.Context())
	if err := h.application.Delete(h.AuditActor(r, user), labelID); err != nil {
		h.respondCatalogMutationError(w, r, err)
		return
	}
	h.RespondNoContent(w)
}

// --- item attachments ---

// ListForItem handles GET /rest/api/v1/items/{id}/labels
//
// @Summary      List labels attached to an item
// @Tags         labels
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Item ID"
// @Success      200  {object}  handlers.labelListResponse
// @Failure      400  {object}  handlers.ErrorResponse  "Invalid item ID"
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the items:read scope"
// @Failure      404  {object}  handlers.ErrorResponse  "Item not found or not visible to caller"
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /items/{id}/labels [get]
func (h *LabelHandler) ListForItem(w http.ResponseWriter, r *http.Request) {
	item, ok := h.resolveItemAccess(w, r, h.Perms.CanViewWorkspace)
	if !ok {
		return
	}
	h.respondItemLabels(w, r, item.ID)
}

// SetForItem handles PUT /rest/api/v1/items/{id}/labels
//
// @Summary      Replace the label set on an item
// @Description  Atomically replaces every global label attached to the item with the supplied IDs.
// @Tags         labels
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                          true  "Item ID"
// @Param        body  body      handlers.itemLabelSetRequest true  "Replacement label set"
// @Success      200   {object}  handlers.labelListResponse
// @Failure      400   {object}  handlers.ErrorResponse  "Invalid request body"
// @Failure      401   {object}  handlers.ErrorResponse
// @Failure      403   {object}  handlers.ErrorResponse  "Token lacks the items:write scope"
// @Failure      404   {object}  handlers.ErrorResponse  "Item not found, not editable, or label not found"
// @Failure      500   {object}  handlers.ErrorResponse
// @Router       /items/{id}/labels [put]
func (h *LabelHandler) SetForItem(w http.ResponseWriter, r *http.Request) {
	item, ok := h.resolveItemAccess(w, r, h.Perms.CanEditWorkspace)
	if !ok {
		return
	}
	var req itemLabelSetRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}
	labels, err := h.application.SetForItem(item.ID, req.LabelIDs)
	if err != nil {
		h.respondAssignmentError(w, r, err)
		return
	}
	h.RespondOK(w, labelListResponse{Items: labels})
}

// AddToItem handles POST /rest/api/v1/items/{id}/labels
//
// @Summary      Attach a single label to an item
// @Tags         labels
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                          true  "Item ID"
// @Param        body  body      handlers.itemLabelAddRequest true  "Label to attach"
// @Success      200   {object}  handlers.labelListResponse
// @Failure      400   {object}  handlers.ErrorResponse  "Invalid request body or missing required field"
// @Failure      401   {object}  handlers.ErrorResponse
// @Failure      403   {object}  handlers.ErrorResponse  "Token lacks the items:write scope"
// @Failure      404   {object}  handlers.ErrorResponse  "Item not found, not editable, or label not found"
// @Failure      409   {object}  handlers.ErrorResponse  "Label is already attached to this item"
// @Failure      500   {object}  handlers.ErrorResponse
// @Router       /items/{id}/labels [post]
func (h *LabelHandler) AddToItem(w http.ResponseWriter, r *http.Request) {
	item, ok := h.resolveItemAccess(w, r, h.Perms.CanEditWorkspace)
	if !ok {
		return
	}
	var req itemLabelAddRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}
	labels, err := h.application.AddToItem(item.ID, req.LabelID)
	if err != nil {
		h.respondAssignmentError(w, r, err)
		return
	}
	h.RespondOK(w, labelListResponse{Items: labels})
}

// RemoveFromItem handles DELETE /rest/api/v1/items/{id}/labels/{labelId}
//
// @Summary      Detach a label from an item
// @Tags         labels
// @Security     BearerAuth
// @Param        id       path  int  true  "Item ID"
// @Param        labelId  path  int  true  "Label ID"
// @Success      204      "Label detached"
// @Failure      400      {object}  handlers.ErrorResponse  "Invalid item or label ID"
// @Failure      401      {object}  handlers.ErrorResponse
// @Failure      403      {object}  handlers.ErrorResponse  "Token lacks the items:write scope"
// @Failure      404      {object}  handlers.ErrorResponse  "Item not found or not editable by caller"
// @Failure      500      {object}  handlers.ErrorResponse
// @Router       /items/{id}/labels/{labelId} [delete]
func (h *LabelHandler) RemoveFromItem(w http.ResponseWriter, r *http.Request) {
	item, ok := h.resolveItemAccess(w, r, h.Perms.CanEditWorkspace)
	if !ok {
		return
	}
	labelID, ok := h.ParsePathID(w, r, "labelId", "label ID")
	if !ok {
		return
	}
	if err := h.application.RemoveFromItem(item.ID, labelID); err != nil {
		h.RespondInternalError(w, r)
		return
	}
	h.RespondNoContent(w)
}

// --- helpers ---

// resolveWorkspaceLabelAccess parses {id} (workspace) + {labelId} from the
// path and verifies the caller has the given permission on the workspace.
// Permission failure returns 404 so the caller can't probe workspace IDs.
func (h *LabelHandler) resolveWorkspaceLabelAccess(w http.ResponseWriter, r *http.Request, permCheck func(int, int) (bool, error)) (labelID int, ok bool) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return 0, false
	}
	workspaceID, ok := h.ParsePathID(w, r, "id", "workspace ID")
	if !ok {
		return 0, false
	}
	labelID, ok = h.ParsePathID(w, r, "labelId", "label ID")
	if !ok {
		return 0, false
	}
	allowed, err := permCheck(user.ID, workspaceID)
	if err != nil || !allowed {
		h.RespondNotFound(w, r)
		return 0, false
	}
	return labelID, true
}

// resolveItemAccess loads the item from path param `id` and applies the
// workspace permission check. 404 on permission failure so item existence
// isn't leaked through the labels surface.
func (h *LabelHandler) resolveItemAccess(w http.ResponseWriter, r *http.Request, permCheck func(int, int) (bool, error)) (*models.Item, bool) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return nil, false
	}
	itemID, ok := h.ParsePathID(w, r, "id", "item ID")
	if !ok {
		return nil, false
	}
	item, err := h.itemRepo.FindByID(itemID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			h.RespondError(w, r, restapi.ErrItemNotFound)
			return nil, false
		}
		h.RespondInternalError(w, r)
		return nil, false
	}
	allowed, err := permCheck(user.ID, item.WorkspaceID)
	if err != nil || !allowed {
		h.RespondError(w, r, restapi.ErrItemNotFound)
		return nil, false
	}
	return item, true
}

func (h *LabelHandler) respondItemLabels(w http.ResponseWriter, r *http.Request, itemID int) {
	labels, err := h.application.ListForItem(itemID)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	h.RespondOK(w, labelListResponse{Items: labels})
}

func (h *LabelHandler) respondCatalogMutationError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		h.RespondNotFound(w, r)
	case errors.Is(err, repository.ErrDuplicateEntry):
		h.RespondError(w, r, restapi.NewAPIError(
			http.StatusConflict, restapi.ErrCodeValidationFailed, "a label with this name already exists",
		))
	case errors.Is(err, services.ErrLabelNameRequired):
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeMissingField, "name is required"))
	default:
		h.RespondInternalError(w, r)
	}
}

func (h *LabelHandler) respondAssignmentError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		h.RespondNotFound(w, r)
	case errors.Is(err, repository.ErrDuplicateEntry):
		h.RespondError(w, r, restapi.NewAPIError(
			http.StatusConflict, restapi.ErrCodeValidationFailed, "label is already attached to this item",
		))
	case errors.Is(err, services.ErrLabelIDRequired):
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeMissingField, "label_id is required"))
	default:
		h.RespondInternalError(w, r)
	}
}
