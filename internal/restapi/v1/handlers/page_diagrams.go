package handlers

import (
	"errors"
	"net/http"

	"windshift/internal/models"
	"windshift/internal/restapi"
	"windshift/internal/restapi/v1/middleware"
	"windshift/internal/services"
)

type PageDiagramHandler struct {
	BaseHandler
	service *services.PageDiagramService
	pages   *services.PageService
}

func NewPageDiagramHandler(
	base BaseHandler,
	service *services.PageDiagramService,
	pages *services.PageService,
) *PageDiagramHandler {
	return &PageDiagramHandler{BaseHandler: base, service: service, pages: pages}
}

type pageDiagramListResponse struct {
	Items []services.PageDiagram `json:"items"`
}

// List handles GET /rest/api/v1/workspaces/{id}/pages/{pageId}/diagrams.
//
// @Summary      List diagrams embedded in a Page
// @Tags         page-diagrams
// @Produce      json
// @Security     BearerAuth
// @Param        id      path  int  true  "Workspace ID"
// @Param        pageId  path  int  true  "Page ID"
// @Success      200  {object}  handlers.pageDiagramListResponse
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks pages:read"
// @Failure      404  {object}  handlers.ErrorResponse  "Page not found or page.view denied"
// @Router       /workspaces/{id}/pages/{pageId}/diagrams [get]
func (h *PageDiagramHandler) List(w http.ResponseWriter, r *http.Request) {
	pageID, user, ok := h.requireWorkspacePage(w, r)
	if !ok {
		return
	}
	items, err := h.service.List(h.auditActor(r, user), pageID)
	if err != nil {
		h.respondServiceError(w, r, err)
		return
	}
	h.RespondOK(w, pageDiagramListResponse{Items: items})
}

// Get handles GET /rest/api/v1/workspaces/{id}/pages/{pageId}/diagrams/{attachmentId}.
//
// @Summary      Get an embedded Page diagram
// @Tags         page-diagrams
// @Produce      json
// @Security     BearerAuth
// @Param        id            path  int  true  "Workspace ID"
// @Param        pageId        path  int  true  "Page ID"
// @Param        attachmentId  path  int  true  "Page attachment ID"
// @Success      200  {object}  services.PageDiagram
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks pages:read"
// @Failure      404  {object}  handlers.ErrorResponse  "Diagram not found, wrong Page, or page.view denied"
// @Router       /workspaces/{id}/pages/{pageId}/diagrams/{attachmentId} [get]
func (h *PageDiagramHandler) Get(w http.ResponseWriter, r *http.Request) {
	pageID, user, ok := h.requireWorkspacePage(w, r)
	if !ok {
		return
	}
	attachmentID, ok := h.ParsePathID(w, r, "attachmentId", "attachment ID")
	if !ok {
		return
	}
	diagram, err := h.service.Get(h.auditActor(r, user), pageID, attachmentID)
	if err != nil {
		h.respondServiceError(w, r, err)
		return
	}
	h.RespondOK(w, diagram)
}

// Create handles POST /rest/api/v1/workspaces/{id}/pages/{pageId}/diagrams.
//
// @Summary      Create and embed a Page diagram
// @Tags         page-diagrams
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id      path  int  true  "Workspace ID"
// @Param        pageId  path  int  true  "Page ID"
// @Param        body  body  services.CreatePageDiagramInput  true  "Diagram payload and placement"
// @Success      201  {object}  services.PageDiagram
// @Failure      400  {object}  handlers.ErrorResponse  "Invalid scene, name, or placement"
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks pages:write"
// @Failure      404  {object}  handlers.ErrorResponse  "Page not found or page.edit denied"
// @Failure      409  {object}  handlers.ErrorResponse  "Stale content hash"
// @Router       /workspaces/{id}/pages/{pageId}/diagrams [post]
func (h *PageDiagramHandler) Create(w http.ResponseWriter, r *http.Request) {
	pageID, user, ok := h.requireWorkspacePage(w, r)
	if !ok {
		return
	}
	var input services.CreatePageDiagramInput
	if !h.DecodeBodyOrRespond(w, r, &input) {
		return
	}
	input.PageID = pageID
	diagram, err := h.service.Create(h.auditActor(r, user), input)
	if err != nil {
		h.respondServiceError(w, r, err)
		return
	}
	h.RespondCreated(w, diagram)
}

// Update handles PUT /rest/api/v1/workspaces/{id}/pages/{pageId}/diagrams/{attachmentId}.
//
// @Summary      Replace an embedded Page diagram
// @Description  Creates a new immutable attachment and replaces the one matching Page fence. No delete endpoint is exposed because Page history retains older references.
// @Tags         page-diagrams
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id            path  int  true  "Workspace ID"
// @Param        pageId        path  int  true  "Page ID"
// @Param        attachmentId  path  int  true  "Current Page attachment ID"
// @Param        body  body  services.UpdatePageDiagramInput  true  "Replacement payload"
// @Success      200  {object}  services.PageDiagram
// @Failure      400  {object}  handlers.ErrorResponse  "Invalid scene"
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks pages:write"
// @Failure      404  {object}  handlers.ErrorResponse  "Diagram not found, wrong Page, or page.edit denied"
// @Failure      409  {object}  handlers.ErrorResponse  "Stale content hash or ambiguous reference"
// @Router       /workspaces/{id}/pages/{pageId}/diagrams/{attachmentId} [put]
func (h *PageDiagramHandler) Update(w http.ResponseWriter, r *http.Request) {
	pageID, user, ok := h.requireWorkspacePage(w, r)
	if !ok {
		return
	}
	attachmentID, ok := h.ParsePathID(w, r, "attachmentId", "attachment ID")
	if !ok {
		return
	}
	var input services.UpdatePageDiagramInput
	if !h.DecodeBodyOrRespond(w, r, &input) {
		return
	}
	input.PageID = pageID
	input.AttachmentID = attachmentID
	diagram, err := h.service.Update(h.auditActor(r, user), input)
	if err != nil {
		h.respondServiceError(w, r, err)
		return
	}
	h.RespondOK(w, diagram)
}

func (h *PageDiagramHandler) requireWorkspacePage(w http.ResponseWriter, r *http.Request) (int, *models.User, bool) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return 0, nil, false
	}
	workspaceID, ok := h.ParsePathID(w, r, "id", "workspace ID")
	if !ok {
		return 0, nil, false
	}
	pageID, ok := h.ParsePathID(w, r, "pageId", "page ID")
	if !ok {
		return 0, nil, false
	}
	page, err := h.pages.GetByID(pageID)
	if errors.Is(err, services.ErrPageNotFound) {
		h.RespondNotFound(w, r)
		return 0, nil, false
	}
	if err != nil {
		h.RespondInternalError(w, r)
		return 0, nil, false
	}
	if page.WorkspaceID != workspaceID {
		h.RespondNotFound(w, r)
		return 0, nil, false
	}
	return pageID, user, true
}

func (h *PageDiagramHandler) auditActor(r *http.Request, user *models.User) services.AuditActor {
	return services.NewAuditActorFromRequest(r, user, middleware.GetAPIToken(r.Context()), "bearer")
}

func (h *PageDiagramHandler) respondServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, services.ErrPageDiagramNotFound),
		errors.Is(err, services.ErrPageNotFound),
		errors.Is(err, services.ErrPageAttachmentUploadNotFound):
		h.RespondNotFound(w, r)
	case errors.Is(err, services.ErrPageContentConflict),
		errors.Is(err, services.ErrPageDiagramReferenceConflict):
		h.RespondError(w, r, restapi.NewAPIError(http.StatusConflict, restapi.ErrCodeValidationFailed, err.Error()))
	case errors.Is(err, services.ErrDiagramPayloadTooLarge):
		h.RespondError(w, r, restapi.NewAPIError(http.StatusRequestEntityTooLarge, restapi.ErrCodeValidationFailed, err.Error()))
	case errors.Is(err, services.ErrPageDiagramNameRequired),
		errors.Is(err, services.ErrPageDiagramPlacementInvalid),
		errors.Is(err, services.ErrDiagramPayloadRequired),
		errors.Is(err, services.ErrDiagramPayloadConflict),
		errors.Is(err, services.ErrDiagramPayloadInvalid),
		errors.Is(err, services.ErrPageAttachmentUploadInvalid):
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeValidationFailed, err.Error()))
	case errors.Is(err, services.ErrPageAttachmentUploadDisabled):
		h.RespondError(w, r, restapi.NewAPIError(http.StatusServiceUnavailable, restapi.ErrCodeValidationFailed, "page diagram storage is disabled"))
	default:
		h.RespondInternalError(w, r)
	}
}
