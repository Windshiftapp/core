package handlers

import (
	"errors"
	"net/http"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/restapi"
	"windshift/internal/scheduler"
	"windshift/internal/services"
)

// RecurrenceHandler adapts bearer-token requests to the shared recurrence
// application service. Token scopes remain enforced in the v1 router.
type RecurrenceHandler struct {
	BaseHandler
	service  *services.RecurrenceService
	itemRepo *repository.ItemRepository
}

// NewRecurrenceHandler constructs a v1 RecurrenceHandler. Its scheduler is
// used only for on-demand generation and is never started here.
func NewRecurrenceHandler(db database.Database, permissionService *services.PermissionService) *RecurrenceHandler {
	generator := scheduler.NewRecurrenceScheduler(db, services.NewWorkflowService(db))
	return &RecurrenceHandler{
		BaseHandler: NewBaseHandler(db, permissionService),
		service:     services.NewRecurrenceService(repository.NewRecurrenceRepository(db), generator, logger.NewAuditor(db)),
		itemRepo:    repository.NewItemRepository(db),
	}
}

// requireItem keeps bearer auth and 404 existence masking explicit in this
// transport adapter.
func (h *RecurrenceHandler) requireItem(w http.ResponseWriter, r *http.Request, permission string) (*models.Item, *models.User, bool) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return nil, nil, false
	}
	itemID, ok := h.ParsePathID(w, r, "id", "item ID")
	if !ok {
		return nil, nil, false
	}
	item, err := h.itemRepo.FindByID(itemID)
	if errors.Is(err, repository.ErrNotFound) {
		h.RespondError(w, r, restapi.ErrItemNotFound)
		return nil, nil, false
	}
	if err != nil {
		h.RespondInternalError(w, r)
		return nil, nil, false
	}
	allowed, err := h.Perms.HasWorkspacePermission(user.ID, item.WorkspaceID, permission)
	if err != nil || !allowed {
		h.RespondError(w, r, restapi.ErrItemNotFound)
		return nil, nil, false
	}
	return item, user, true
}

func (h *RecurrenceHandler) respondServiceError(w http.ResponseWriter, r *http.Request, err error) bool {
	validationErr, ok := services.AsRecurrenceValidationError(err)
	if !ok {
		return false
	}
	code := restapi.ErrCodeValidationFailed
	if validationErr.Kind == services.RecurrenceMissingField {
		code = restapi.ErrCodeMissingField
	}
	h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, code, validationErr.Message))
	return true
}

// GetRecurrence handles GET /rest/api/v1/items/{id}/recurrence.
//
// @Summary      Get the recurrence rule on an item
// @Tags         recurrence
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  int  true  "Item ID"
// @Success      200  {object}  models.RecurrenceRule  "null when no recurrence is configured"
// @Failure      400  {object}  handlers.ErrorResponse
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse
// @Failure      404  {object}  handlers.ErrorResponse
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /items/{id}/recurrence [get]
func (h *RecurrenceHandler) GetRecurrence(w http.ResponseWriter, r *http.Request) {
	item, _, ok := h.requireItem(w, r, models.PermissionItemView)
	if !ok {
		return
	}
	rule, err := h.service.Get(item.ID)
	if errors.Is(err, repository.ErrNotFound) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("null"))
		return
	}
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	h.RespondOK(w, rule)
}

// CreateRecurrence handles POST /rest/api/v1/items/{id}/recurrence.
//
// @Summary      Create a recurrence rule on an item
// @Tags         recurrence
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path  int                             true  "Item ID"
// @Param        body  body  models.CreateRecurrenceRequest  true  "Recurrence rule"
// @Success      201  {object}  models.RecurrenceRule
// @Failure      400  {object}  handlers.ErrorResponse
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse
// @Failure      404  {object}  handlers.ErrorResponse
// @Failure      409  {object}  handlers.ErrorResponse
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /items/{id}/recurrence [post]
func (h *RecurrenceHandler) CreateRecurrence(w http.ResponseWriter, r *http.Request) {
	item, user, ok := h.requireItem(w, r, models.PermissionItemEdit)
	if !ok {
		return
	}
	var req models.CreateRecurrenceRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}
	rule, err := h.service.Create(item.ID, item.WorkspaceID, user.ID, req, h.AuditActor(r, user))
	if errors.Is(err, services.ErrRecurrenceConflict) {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusConflict, restapi.ErrCodeConflict, "Recurrence rule already exists for this item"))
		return
	}
	if errors.Is(err, services.ErrRecurrenceWorkspaceLimit) {
		h.RespondError(w, r, restapi.NewAPIError(
			http.StatusConflict,
			restapi.ErrCodeConflict,
			services.RecurrenceWorkspaceLimitMessage(),
		))
		return
	}
	if h.respondServiceError(w, r, err) {
		return
	}
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	h.RespondCreated(w, rule)
}

// UpdateRecurrence handles PUT /rest/api/v1/items/{id}/recurrence.
//
// @Summary      Update a recurrence rule
// @Tags         recurrence
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path  int                             true  "Item ID"
// @Param        body  body  models.UpdateRecurrenceRequest  true  "Fields to update"
// @Success      200  {object}  models.RecurrenceRule
// @Failure      400  {object}  handlers.ErrorResponse
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse
// @Failure      404  {object}  handlers.ErrorResponse
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /items/{id}/recurrence [put]
func (h *RecurrenceHandler) UpdateRecurrence(w http.ResponseWriter, r *http.Request) {
	item, user, ok := h.requireItem(w, r, models.PermissionItemEdit)
	if !ok {
		return
	}
	var req models.UpdateRecurrenceRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}
	rule, err := h.service.Update(item.ID, req, h.AuditActor(r, user))
	if errors.Is(err, repository.ErrNotFound) {
		h.RespondError(w, r, restapi.ErrNotFound)
		return
	}
	if h.respondServiceError(w, r, err) {
		return
	}
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	h.RespondOK(w, rule)
}

// DeleteRecurrence deletes the recurrence rule for an item.
//
// @Summary      Delete a recurrence rule
// @Tags         recurrence
// @Security     BearerAuth
// @Param        id  path  int  true  "Item ID"
// @Success      204
// @Failure      400  {object}  handlers.ErrorResponse
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse
// @Failure      404  {object}  handlers.ErrorResponse
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /items/{id}/recurrence [delete]
func (h *RecurrenceHandler) DeleteRecurrence(w http.ResponseWriter, r *http.Request) {
	item, user, ok := h.requireItem(w, r, models.PermissionItemEdit)
	if !ok {
		return
	}
	err := h.service.Delete(item.ID, h.AuditActor(r, user))
	if errors.Is(err, repository.ErrNotFound) {
		h.RespondError(w, r, restapi.ErrNotFound)
		return
	}
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	h.RespondNoContent(w)
}

type recurrenceInstanceListResponse struct {
	Items      []models.RecurrenceInstance `json:"instances"`
	Pagination restapi.PaginationMeta      `json:"pagination"`
}

// ListInstances handles GET /rest/api/v1/items/{id}/recurrence/instances.
//
// @Summary      List generated recurrence instances
// @Tags         recurrence
// @Produce      json
// @Security     BearerAuth
// @Param        id     path   int  true   "Item ID"
// @Param        page   query  int  false  "Page number"
// @Param        limit  query  int  false  "Page size"
// @Success      200  {object}  handlers.recurrenceInstanceListResponse
// @Failure      400  {object}  handlers.ErrorResponse
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse
// @Failure      404  {object}  handlers.ErrorResponse
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /items/{id}/recurrence/instances [get]
func (h *RecurrenceHandler) ListInstances(w http.ResponseWriter, r *http.Request) {
	item, _, ok := h.requireItem(w, r, models.PermissionItemView)
	if !ok {
		return
	}
	pagination := h.ParsePagination(r)
	result, err := h.service.ListInstances(item.ID, pagination.Limit, pagination.Offset)
	if errors.Is(err, repository.ErrNotFound) {
		h.RespondError(w, r, restapi.ErrNotFound)
		return
	}
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	instances := make([]models.RecurrenceInstance, 0, len(result.Items))
	for _, instance := range result.Items {
		instances = append(instances, *instance)
	}
	h.RespondOK(w, recurrenceInstanceListResponse{
		Items: instances, Pagination: restapi.NewPaginationMeta(pagination, result.Total),
	})
}

type recurrenceForceGenerateResponse struct {
	InstancesGenerated int `json:"instances_generated"`
}

// ForceGenerate handles POST /rest/api/v1/items/{id}/recurrence/generate.
//
// @Summary      Force-generate recurrence instances
// @Tags         recurrence
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  int  true  "Item ID"
// @Success      200  {object}  handlers.recurrenceForceGenerateResponse
// @Failure      400  {object}  handlers.ErrorResponse
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse
// @Failure      404  {object}  handlers.ErrorResponse
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /items/{id}/recurrence/generate [post]
func (h *RecurrenceHandler) ForceGenerate(w http.ResponseWriter, r *http.Request) {
	item, user, ok := h.requireItem(w, r, models.PermissionItemEdit)
	if !ok {
		return
	}
	count, err := h.service.ForceGenerate(item.ID, h.AuditActor(r, user))
	if errors.Is(err, repository.ErrNotFound) {
		h.RespondError(w, r, restapi.ErrNotFound)
		return
	}
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	h.RespondOK(w, recurrenceForceGenerateResponse{InstancesGenerated: count})
}

// PreviewRRule handles POST /rest/api/v1/recurrence-rules/preview.
//
// @Summary      Preview RRULE occurrences
// @Tags         recurrence
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body  models.RRulePreviewRequest  true  "RRULE preview"
// @Success      200  {object}  services.RecurrencePreview
// @Failure      400  {object}  handlers.ErrorResponse
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /recurrence-rules/preview [post]
func (h *RecurrenceHandler) PreviewRRule(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.RequireAuth(w, r); !ok {
		return
	}
	var req models.RRulePreviewRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}
	preview, err := h.service.Preview(req)
	if h.respondServiceError(w, r, err) {
		return
	}
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	h.RespondOK(w, preview)
}
