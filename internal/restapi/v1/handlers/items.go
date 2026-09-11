package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"windshift/internal/cql"
	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/markdown"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/restapi"
	"windshift/internal/restapi/v1/dto"
	"windshift/internal/services"
	"windshift/internal/validation"
)

// ItemHandler handles public API requests for items
type ItemHandler struct {
	BaseHandler
	itemRepo     *repository.ItemRepository
	itemCRUD     *services.ItemCRUDService
	itemCreation *services.ItemCreationService
	itemUpdate   *services.ItemUpdateApplicationService
	itemDeletion *services.ItemDeletionApplicationService
	commentSvc   *services.CommentService
	workflowSvc  *services.WorkflowService
	conditionSvc *services.ConditionService
	approvalSvc  *services.ApprovalService
	permSvc      *services.PermissionService
	auditor      *logger.Auditor
}

// NewItemHandler creates a new item handler. commentService is shared with the
// cookie-auth handler so item comments created through the bearer-token surface
// fire the same notifications/mentions/webhooks (WI-434); when nil a bare
// service is created that persists comments but skips side effects.
func NewItemHandler(db database.Database, permissionService *services.PermissionService, commentService *services.CommentService, creationServices ...*services.ItemCreationService) *ItemHandler {
	if commentService == nil {
		commentService = services.NewCommentService(db)
	}
	itemCreation := services.NewItemCreationService(db, permissionService)
	if len(creationServices) > 0 && creationServices[0] != nil {
		itemCreation = creationServices[0]
	}
	workflowSvc := services.NewWorkflowService(db)
	leaveRepo := repository.NewLeaveRepository(db)
	return &ItemHandler{
		BaseHandler:  NewBaseHandler(db, permissionService),
		itemRepo:     repository.NewItemRepository(db),
		itemCRUD:     services.NewItemCRUDService(db),
		itemCreation: itemCreation,
		itemUpdate:   services.NewItemUpdateApplicationService(db, permissionService),
		itemDeletion: services.NewItemDeletionApplicationService(db, permissionService),
		commentSvc:   commentService,
		workflowSvc:  workflowSvc,
		conditionSvc: services.NewConditionService(db, permissionService, services.NewScriptEngine()),
		approvalSvc:  services.NewApprovalService(db, leaveRepo, workflowSvc),
		permSvc:      permissionService,
		auditor:      logger.NewAuditor(db),
	}
}

// SetItemUpdateApplicationService installs the fully wired user-facing update
// pipeline shared with the cookie-auth handler.
func (h *ItemHandler) SetItemUpdateApplicationService(service *services.ItemUpdateApplicationService) {
	if service != nil {
		h.itemUpdate = service
	}
}

// SetItemDeletionApplicationService installs the fully wired user-facing
// deletion pipeline shared with the cookie-auth handler and MCP.
func (h *ItemHandler) SetItemDeletionApplicationService(service *services.ItemDeletionApplicationService) {
	if service != nil {
		h.itemDeletion = service
	}
}

// parseIDList parses a comma-separated list of integer IDs from a query
// parameter. Empty/non-numeric tokens are silently dropped — callers should
// treat a zero-length result as "no usable filter values supplied".
func parseIDList(raw string) []int {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	ids := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if id, err := strconv.Atoi(p); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

// requireItemAccess authenticates the user, parses the item ID from the path,
// loads the item, and checks workspace permission. Returns the item and user on success.
// When needDetails is true, loads the item with joined details (FindByIDWithDetails);
// otherwise uses the lighter FindByID.
// permCheck should be h.Perms.CanViewWorkspace or h.Perms.CanEditWorkspace.
func (h *ItemHandler) requireItemAccess(w http.ResponseWriter, r *http.Request, needDetails bool, permCheck func(int, int) (bool, error)) (*models.Item, *models.User, bool) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return nil, nil, false
	}

	itemID, ok := h.ParsePathID(w, r, "id", "item ID")
	if !ok {
		return nil, nil, false
	}

	var item *models.Item
	var err error
	if needDetails {
		item, err = h.itemRepo.FindByIDWithDetails(itemID)
	} else {
		item, err = h.itemRepo.FindByID(itemID)
	}
	if err != nil {
		if err == repository.ErrNotFound {
			h.RespondError(w, r, restapi.ErrItemNotFound)
			return nil, nil, false
		}
		h.RespondInternalError(w, r)
		return nil, nil, false
	}

	allowed, err := permCheck(user.ID, item.WorkspaceID)
	if err != nil || !allowed {
		h.RespondError(w, r, restapi.ErrItemNotFound)
		return nil, nil, false
	}

	return item, user, true
}

// allowUnlessPersonalExcluded enforces the exclude_personal query parameter
// on single-item fetches: when set and the item's workspace is personal, the
// item is reported as not found (mirroring the visibility 404 contract).
func (h *ItemHandler) allowUnlessPersonalExcluded(w http.ResponseWriter, r *http.Request, workspaceID int) bool {
	if !ExcludePersonal(r) {
		return true
	}
	personal, err := repository.IsPersonalWorkspace(h.DB, workspaceID)
	if err != nil {
		h.RespondInternalError(w, r)
		return false
	}
	if personal {
		h.RespondError(w, r, restapi.ErrItemNotFound)
		return false
	}
	return true
}

// setParentSummary fills the parent's display key and title on the response,
// but only when the caller is allowed to see the parent. A parent may live in
// a different workspace than its child; revealing its key or title across a
// workspace boundary the caller can't view would leak information, so a
// cross-workspace parent is gated on item-view permission — mirroring
// validateParentHierarchy. Same-workspace parents are already covered by the
// item's own workspace view check. The raw parent_id is left as-is
// (pre-existing behavior); only the renderable fields are withheld, so an
// unauthorized caller sees a parent_id it cannot resolve but no key/title.
//
// The key is built from the parent's own workspace key (not the child's), so
// it is correct even when the parent lives in another workspace.
func (h *ItemHandler) setParentSummary(userID int, item *models.Item, resp *dto.ItemResponse) {
	if item == nil || resp == nil || item.ParentID == nil || item.ParentWorkspaceItemNumber == nil {
		return
	}

	parentWorkspaceID, err := h.itemRepo.GetWorkspaceID(*item.ParentID)
	if err != nil {
		// Fail closed: withhold the renderable fields on lookup failure.
		return
	}

	// The parent's workspace key — same as the child's for the common
	// in-workspace hierarchy, looked up only when the parent is elsewhere.
	parentWorkspaceKey := item.WorkspaceKey
	if parentWorkspaceID != item.WorkspaceID {
		// Cross-workspace parent: gate on view permission before revealing anything.
		if allowed, perr := h.Perms.CanViewWorkspace(userID, parentWorkspaceID); perr != nil || !allowed {
			return
		}
		key, kerr := repository.NewWorkspaceRepository(h.DB).GetKey(parentWorkspaceID)
		if kerr != nil {
			return
		}
		parentWorkspaceKey = key
	}

	resp.ParentKey = fmt.Sprintf("%s-%d", parentWorkspaceKey, *item.ParentWorkspaceItemNumber)
	resp.ParentTitle = item.ParentTitle
}

// buildExpandedItemResponse maps an item and applies the optional related
// collections requested by the caller. Both single-item read routes use this
// path so their response contracts stay identical.
func (h *ItemHandler) buildExpandedItemResponse(r *http.Request, userID int, item *models.Item) *dto.ItemResponse {
	baseURL := getBaseURL(r)
	response := dto.MapItemToResponse(item, baseURL)
	h.setParentSummary(userID, item, response)

	expand := restapi.ParseExpand(r)
	if expand.Comments {
		if comments, _, err := h.commentSvc.GetByItemIDPaginated(item.ID, services.DefaultCommentFeedLimit, 0, false); err == nil {
			response.Comments = dto.MapCommentsToResponse(comments)
		}
	}
	if expand.History {
		if history, err := h.itemCRUD.GetHistory(item.ID); err == nil {
			response.History = dto.MapHistoryToResponses(history)
		}
	}
	if expand.Attachments {
		if attachments, err := h.itemCRUD.GetAttachments(item.ID); err == nil {
			response.Attachments = dto.MapAttachmentsToResponse(attachments, baseURL)
		}
	}
	if expand.Transitions {
		if item.StatusID != nil {
			if transitions, err := h.workflowSvc.GetTransitionsForItem(item.WorkspaceID, item.ItemTypeID, *item.StatusID); err == nil {
				response.Transitions = dto.MapServiceTransitionsToResponse(transitions)
			}
		} else {
			response.Transitions = []dto.TransitionResponse{}
		}
	}

	return response
}

// List handles GET /rest/api/v1/items
//
// @Summary      List items visible to the caller
// @Description  Paginated list of items across every workspace the caller can view. Filterable by workspace, status, priority, assignee, parent, creator and item type.
// @Tags         items
// @Produce      json
// @Security     BearerAuth
// @Param        page          query     int     false  "Page number (1-based)"
// @Param        limit         query     int     false  "Items per page (max 100)"
// @Param        sort          query     string  false  "Sort field"
// @Param        order         query     string  false  "Sort order: asc or desc"
// @Param        workspace_id  query     int     false  "Filter to a single workspace"
// @Param        status_id     query     string  false  "Filter by status ID (single value or comma-separated list)"
// @Param        status_id_not query     string  false  "Exclude items with these status IDs (single value or comma-separated list)"
// @Param        priority_id   query     int     false  "Filter by priority ID"
// @Param        assignee_id   query     int     false  "Filter by assignee user ID"
// @Param        item_type_id  query     int     false  "Filter by item type ID"
// @Param        creator_id    query     int     false  "Filter by creator user ID"
// @Param        parent_id     query     string  false  "Filter by parent item ID; pass `null` or `0` for top-level items"
// @Success      200           {object}  handlers.PaginatedResponse{data=[]dto.ItemResponse}
// @Failure      400           {object}  handlers.ErrorResponse  "Invalid query parameter"
// @Failure      401           {object}  handlers.ErrorResponse
// @Failure      403           {object}  handlers.ErrorResponse  "Token lacks the items:read scope"
// @Failure      500           {object}  handlers.ErrorResponse
// @Router       /items [get]
func (h *ItemHandler) List(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	pagination := h.ParsePagination(r)

	accessibleWorkspaceIDs, err := h.Perms.GetAccessibleWorkspaceIDs(user.ID)
	if err != nil {
		h.RespondError(w, r, restapi.ErrInternalError.WithDetails(map[string]string{
			"message": "Failed to get accessible workspaces",
		}))
		return
	}

	if len(accessibleWorkspaceIDs) == 0 {
		h.RespondPaginated(w, []dto.ItemResponse{}, pagination, 0)
		return
	}

	filters := services.ItemFilters{}
	if wsID := r.URL.Query().Get("workspace_id"); wsID != "" {
		if id, parseErr := strconv.Atoi(wsID); parseErr == nil {
			filters.WorkspaceID = &id
		}
	}
	if statusID := r.URL.Query().Get("status_id"); statusID != "" {
		if ids := parseIDList(statusID); len(ids) > 1 {
			filters.StatusIDs = ids
		} else if len(ids) == 1 {
			id := ids[0]
			filters.StatusID = &id
		}
	}
	if statusIDNot := r.URL.Query().Get("status_id_not"); statusIDNot != "" {
		if ids := parseIDList(statusIDNot); len(ids) > 1 {
			filters.StatusIDsNot = ids
		} else if len(ids) == 1 {
			id := ids[0]
			filters.StatusIDNot = &id
		}
	}
	if priorityID := r.URL.Query().Get("priority_id"); priorityID != "" {
		if id, parseErr := strconv.Atoi(priorityID); parseErr == nil {
			filters.PriorityID = &id
		}
	}
	if assigneeID := r.URL.Query().Get("assignee_id"); assigneeID != "" {
		if id, parseErr := strconv.Atoi(assigneeID); parseErr == nil {
			filters.AssigneeID = &id
		}
	}
	if itemTypeID := r.URL.Query().Get("item_type_id"); itemTypeID != "" {
		if id, parseErr := strconv.Atoi(itemTypeID); parseErr == nil {
			filters.ItemTypeID = &id
		}
	}
	if creatorID := r.URL.Query().Get("creator_id"); creatorID != "" {
		if id, parseErr := strconv.Atoi(creatorID); parseErr == nil {
			filters.CreatorID = &id
		}
	}
	if parentID := r.URL.Query().Get("parent_id"); parentID != "" {
		if parentID == "null" || parentID == "0" {
			zero := 0
			filters.ParentID = &zero
			filters.ParentIDIsSet = true
		} else if id, parseErr := strconv.Atoi(parentID); parseErr == nil {
			filters.ParentID = &id
			filters.ParentIDIsSet = true
		}
	}

	params := services.ItemListParams{
		WorkspaceIDs: accessibleWorkspaceIDs,
		Filters:      filters,
		Pagination: services.PaginationParams{
			Limit:  pagination.Limit,
			Offset: pagination.Offset,
		},
		SortBy:  pagination.SortBy,
		SortAsc: pagination.SortAsc,
	}

	items, total, err := h.itemCRUD.ListContext(r.Context(), params)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	if err := repository.NewMilestoneAttachRepository(h.DB).LoadForItemsContext(r.Context(), items); err != nil {
		h.RespondInternalError(w, r)
		return
	}

	h.maskProjectNames(user.ID, items)

	baseURL := getBaseURL(r)
	itemResponses := dto.MapItemsToResponse(items, baseURL)

	h.RespondPaginated(w, itemResponses, pagination, total)
}

// maxBatchItemIDs caps how many ids GetBatch accepts in one request, bounding
// the IN-clause size. Clients chunk larger sets across multiple requests.
const maxBatchItemIDs = 500

// GetBatch handles GET /rest/api/v1/items/batch
//
// @Summary      Get many items by id
// @Description  Returns full item objects for the comma-separated `ids`. Items the caller cannot view (or that don't exist) are silently omitted — existence is never leaked. Cap 500 ids per request.
// @Tags         items
// @Produce      json
// @Security     BearerAuth
// @Param        ids    query     string  true   "Comma-separated item ids, e.g. 1,2,3 (max 500)"
// @Success      200    {array}   dto.ItemResponse
// @Failure      400    {object}  handlers.ErrorResponse  "Too many ids"
// @Failure      401    {object}  handlers.ErrorResponse
// @Failure      403    {object}  handlers.ErrorResponse  "Token lacks the items:read scope"
// @Failure      500    {object}  handlers.ErrorResponse
// @Router       /items/batch [get]
func (h *ItemHandler) GetBatch(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	ids := parseIDList(r.URL.Query().Get("ids"))
	if len(ids) == 0 {
		h.RespondOK(w, []dto.ItemResponse{})
		return
	}
	if len(ids) > maxBatchItemIDs {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput,
			fmt.Sprintf("too many ids (max %d per request)", maxBatchItemIDs)))
		return
	}

	accessibleWorkspaceIDs, err := h.Perms.GetAccessibleWorkspaceIDs(user.ID)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	accessible := make(map[int]struct{}, len(accessibleWorkspaceIDs))
	for _, id := range accessibleWorkspaceIDs {
		accessible[id] = struct{}{}
	}

	loaded, err := h.itemRepo.FindByIDsWithDetails(ids)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	// Keep only items in workspaces the caller can view; drop the rest silently
	// (404-no-leak contract, mirroring requireItemAccess for the single fetch).
	items := make([]models.Item, 0, len(loaded))
	for _, it := range loaded {
		if _, allowed := accessible[it.WorkspaceID]; allowed {
			items = append(items, *it)
		}
	}

	h.maskProjectNames(user.ID, items)

	baseURL := getBaseURL(r)
	h.RespondOK(w, dto.MapItemsToResponse(items, baseURL))
}

// Get handles GET /rest/api/v1/items/{id}
//
// @Summary      Get an item by ID
// @Description  Returns 404 (not 403) when the item exists but isn't visible to the caller — item existence is never leaked. When `expand=comments` is requested, the response embeds the 25 newest comments; use the comments link for additional pages.
// @Tags         items
// @Produce      json
// @Security     BearerAuth
// @Param        id                path      int   true   "Item ID"
// @Param        exclude_personal  query     bool  false  "Treat items in the caller's personal workspaces as not found"
// @Success      200  {object}  dto.ItemResponse
// @Failure      400  {object}  handlers.ErrorResponse  "Invalid item ID"
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the items:read scope"
// @Failure      404  {object}  handlers.ErrorResponse  "Item not found or not visible to caller"
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /items/{id} [get]
func (h *ItemHandler) Get(w http.ResponseWriter, r *http.Request) {
	item, user, ok := h.requireItemAccess(w, r, true, h.Perms.CanViewWorkspace)
	if !ok {
		return
	}
	if !h.allowUnlessPersonalExcluded(w, r, item.WorkspaceID) {
		return
	}

	h.maskProjectNamesOne(user.ID, item)

	h.RespondOK(w, h.buildExpandedItemResponse(r, user.ID, item))
}

// GetByKeyAndNumber handles GET /rest/api/v1/workspaces/{ws_key}/items/{number}.
// Looks up an item by its stable (workspace_key, workspace_item_number) pair —
// the form embedding clients should persist instead of the volatile numeric id.
//
// @Summary      Get an item by workspace key and per-workspace number
// @Description  Resolves an item by its stable (workspace_key, workspace_item_number) pair. Returns 404 (not 403) when the item exists but isn't visible to the caller — item existence is never leaked. When `expand=comments` is requested, the response embeds the 25 newest comments; use the comments link for additional pages.
// @Tags         items
// @Produce      json
// @Security     BearerAuth
// @Param        ws_key  path      string  true  "Workspace key (e.g. PROJ)"
// @Param        number            path      int   true   "Per-workspace item number"
// @Param        exclude_personal  query     bool  false  "Treat items in the caller's personal workspaces as not found"
// @Success      200     {object}  dto.ItemResponse
// @Failure      400     {object}  handlers.ErrorResponse  "Invalid workspace key or item number"
// @Failure      401     {object}  handlers.ErrorResponse
// @Failure      403     {object}  handlers.ErrorResponse  "Token lacks the items:read scope"
// @Failure      404     {object}  handlers.ErrorResponse  "Item not found or not visible to caller"
// @Failure      500     {object}  handlers.ErrorResponse
// @Router       /workspaces/{ws_key}/items/{number} [get]
func (h *ItemHandler) GetByKeyAndNumber(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	wsKey := strings.TrimSpace(r.PathValue("ws_key"))
	if wsKey == "" {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid workspace key"))
		return
	}
	number, ok := h.ParsePathID(w, r, "number", "item number")
	if !ok {
		return
	}

	itemID, err := h.itemRepo.FindIDByKeyAndNumber(wsKey, number)
	if err != nil {
		if err == repository.ErrNotFound {
			h.RespondError(w, r, restapi.ErrItemNotFound)
			return
		}
		h.RespondInternalError(w, r)
		return
	}

	item, err := h.itemRepo.FindByIDWithDetails(itemID)
	if err != nil {
		if err == repository.ErrNotFound {
			h.RespondError(w, r, restapi.ErrItemNotFound)
			return
		}
		h.RespondInternalError(w, r)
		return
	}

	allowed, err := h.Perms.CanViewWorkspace(user.ID, item.WorkspaceID)
	if err != nil || !allowed {
		// 404, never 403 — do not leak that the item exists.
		h.RespondError(w, r, restapi.ErrItemNotFound)
		return
	}
	if !h.allowUnlessPersonalExcluded(w, r, item.WorkspaceID) {
		return
	}

	h.maskProjectNamesOne(user.ID, item)

	h.RespondOK(w, h.buildExpandedItemResponse(r, user.ID, item))
}

// Create handles POST /rest/api/v1/items
//
// @Summary      Create an item
// @Description  Creates an item in the workspace specified by `workspace_id`. The caller must have edit permission on that workspace. If `status_id` is omitted, the workflow initial status is used. If `status_id` is provided (for example, board column quick-add), it must be the initial status or directly reachable from the initial status without workflow conditions, validators, or approval gates.
// @Tags         items
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      dto.ItemCreateRequest  true  "Item to create"
// @Success      201   {object}  dto.ItemResponse
// @Failure      400   {object}  handlers.ErrorResponse  "Invalid request body, missing required field, or invalid create-time status_id"
// @Failure      401   {object}  handlers.ErrorResponse
// @Failure      403   {object}  handlers.ErrorResponse  "Token lacks the items:write scope or caller cannot edit the target workspace"
// @Failure      500   {object}  handlers.ErrorResponse
// @Router       /items [post]
func (h *ItemHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}
	var req dto.ItemCreateRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}
	if req.WorkspaceID == 0 {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeMissingField, "workspace_id is required"))
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeMissingField, "title is required"))
		return
	}
	canEdit, err := h.Perms.CanEditWorkspace(user.ID, req.WorkspaceID)
	if err != nil || !canEdit {
		h.RespondError(w, r, restapi.ErrInsufficientPermission)
		return
	}

	result, err := h.itemCreation.Create(user.ID, user.Username, services.ItemCreateInput{
		WorkspaceID:       req.WorkspaceID,
		Title:             req.Title,
		Description:       req.Description,
		StatusID:          req.StatusID,
		PriorityID:        req.PriorityID,
		ItemTypeID:        req.ItemTypeID,
		DueDate:           req.DueDate,
		StartDate:         req.StartDate,
		EndDate:           req.EndDate,
		IsTask:            req.IsTask,
		IterationID:       req.IterationID,
		ProjectID:         req.ProjectID,
		AssigneeID:        req.AssigneeID,
		ParentID:          req.ParentID,
		CustomFieldValues: req.CustomFields,
		MilestoneIDs:      req.MilestoneIDs,
	})
	if err != nil {
		h.respondItemCreationError(w, r, err)
		return
	}

	h.maskProjectNamesOne(user.ID, result.Item)
	response := dto.MapItemToResponse(result.Item, getBaseURL(r))
	if result.MandatoryTemplate.TemplateID != 0 {
		response.EnforcedTemplate = &dto.EnforcedTemplateSummary{
			TemplateID: result.MandatoryTemplate.TemplateID,
			Name:       result.MandatoryTemplate.Name,
			Applied:    result.MandatoryTemplate.Applied,
		}
	}
	h.RespondCreated(w, response)
}

func (h *ItemHandler) respondItemCreationError(w http.ResponseWriter, r *http.Request, err error) {
	var creationErr *services.ItemCreationValidationError
	var transitionErr *services.TransitionRejection
	var validationErr *validation.ValidationError
	switch {
	case errors.As(err, &validationErr):
		h.RespondError(w, r, restapi.NewAPIError(
			http.StatusBadRequest,
			restapi.ErrCodeValidationFailed,
			validationErr.Message,
		).WithDetails(map[string]string{"field": validationErr.Field}))
	case errors.As(err, &creationErr),
		errors.As(err, &transitionErr),
		errors.Is(err, services.ErrMissingItemType),
		errors.Is(err, services.ErrInvalidItemType),
		errors.Is(err, services.ErrProjectNotFound):
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeValidationFailed, err.Error()))
	default:
		h.RespondInternalError(w, r)
	}
}

// Update handles PUT /rest/api/v1/items/{id}
//
// @Summary      Update an item
// @Description  Patches the supplied fields on an existing item. `status_id` cannot be updated here — use POST /items/{id}/transition so workflow + condition rules are enforced.
// @Tags         items
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                    true  "Item ID"
// @Param        body  body      dto.ItemUpdateRequest  true  "Fields to update"
// @Success      200   {object}  dto.ItemResponse
// @Failure      400   {object}  handlers.ErrorResponse  "Invalid request body or attempted to update status_id"
// @Failure      401   {object}  handlers.ErrorResponse
// @Failure      403   {object}  handlers.ErrorResponse  "Token lacks the items:write scope"
// @Failure      404   {object}  handlers.ErrorResponse  "Item not found or not visible to caller"
// @Failure      500   {object}  handlers.ErrorResponse
// @Router       /items/{id} [put]
func (h *ItemHandler) Update(w http.ResponseWriter, r *http.Request) {
	item, user, ok := h.requireItemAccess(w, r, true, h.Perms.CanEditWorkspace)
	if !ok {
		return
	}

	itemID := item.ID

	// Decode once into raw fields so the application service can preserve the
	// distinction between omitted fields and explicit JSON null.
	bodyBytes, err := restapi.ReadJSONBody(w, r)
	if err != nil {
		if restapi.IsRequestBodyTooLarge(err) {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusRequestEntityTooLarge, restapi.ErrCodeRequestTooLarge, "Request body too large"))
			return
		}
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid request body"))
		return
	}

	var rawFields map[string]json.RawMessage
	if err := json.Unmarshal(bodyBytes, &rawFields); err != nil {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid request body"))
		return
	}
	result, err := h.itemUpdate.UpdateJSONFields(user.ID, user.Username, itemID, rawFields)
	if err != nil {
		// Validation errors (e.g. milestone_id refers to a non-existent
		// milestone) must surface as 400 with the field name, not 500.
		var verr *validation.ValidationError
		if errors.As(err, &verr) {
			h.RespondError(w, r, restapi.NewAPIError(
				http.StatusBadRequest,
				restapi.ErrCodeValidationFailed,
				verr.Message,
			).WithDetails(map[string]string{"field": verr.Field}))
			return
		}
		h.RespondInternalError(w, r)
		return
	}

	h.maskProjectNamesOne(user.ID, result.Item)

	baseURL := getBaseURL(r)
	response := dto.MapItemToResponse(result.Item, baseURL)

	h.RespondOK(w, response)
}

// ChangeType handles POST /rest/api/v1/items/{id}/change-type.
// Item type changes use their own service because a target type may imply a
// different workflow. When the current status is not in the target workflow,
// clients must provide target_status_id.
//
// @Summary      Change an item's item type
// @Description  Reassigns an item to a different item type. If the target type's workflow does not contain the current status, the caller must supply target_status_id; otherwise a 409 lists the candidate statuses.
// @Tags         items
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                       true  "Item ID"
// @Param        body  body      dto.ItemTypeChangeRequest true  "Target item type and optional target status"
// @Success      200   {object}  dto.ItemResponse
// @Failure      400   {object}  handlers.ErrorResponse  "Invalid body or missing target_item_type_id"
// @Failure      401   {object}  handlers.ErrorResponse
// @Failure      403   {object}  handlers.ErrorResponse  "Token lacks the items:write scope"
// @Failure      404   {object}  handlers.ErrorResponse  "Item not found or not visible to caller"
// @Failure      409   {object}  handlers.ErrorResponse  "Target status required because the current status is not in the target type's workflow"
// @Failure      500   {object}  handlers.ErrorResponse
// @Router       /items/{id}/change-type [post]
func (h *ItemHandler) ChangeType(w http.ResponseWriter, r *http.Request) {
	item, user, ok := h.requireItemAccess(w, r, true, h.Perms.CanEditWorkspace)
	if !ok {
		return
	}

	var req dto.ItemTypeChangeRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}
	if req.TargetItemTypeID <= 0 {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeMissingField, "target_item_type_id is required"))
		return
	}

	svc := services.NewItemTypeChangeService(h.DB).WithConditionService(h.conditionSvc)
	analysis, err := svc.Analyze(item, req.TargetItemTypeID)
	if err != nil {
		h.respondTypeChangeError(w, r, err)
		return
	}
	if item.ItemTypeID != nil && *item.ItemTypeID == req.TargetItemTypeID && !analysis.RequiresMigration {
		h.maskProjectNamesOne(user.ID, item)
		h.RespondOK(w, dto.MapItemToResponse(item, getBaseURL(r)))
		return
	}

	var nextStatusID *int
	if analysis.RequiresMigration {
		if req.TargetStatusID == nil {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusConflict, restapi.ErrCodeConflict, "A target status is required before changing this item type").WithDetails(map[string]any{
				"reason":   "migration_required",
				"analysis": analysis,
			}))
			return
		}
		if analysis.TargetWorkflowID != nil {
			inWorkflow, err := svc.IsStatusInWorkflow(*req.TargetStatusID, *analysis.TargetWorkflowID)
			if err != nil {
				h.RespondInternalError(w, r)
				return
			}
			if !inWorkflow {
				h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeValidationFailed, "target_status_id is not part of the target item type workflow"))
				return
			}
		}
		if err := svc.ValidateStatusMapping(r.Context(), item, req.TargetItemTypeID, analysis.TargetWorkflowID, req.TargetStatusID); err != nil {
			h.respondTypeChangeError(w, r, err)
			return
		}
		nextStatusID = req.TargetStatusID
	}

	if _, err := svc.ApplyChange(item.ID, user.ID, req.TargetItemTypeID, nextStatusID, item); err != nil {
		h.RespondInternalError(w, r)
		return
	}

	updated, err := h.itemRepo.FindByIDWithDetails(item.ID)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	h.maskProjectNamesOne(user.ID, updated)
	h.RespondOK(w, dto.MapItemToResponse(updated, getBaseURL(r)))
}

func (h *ItemHandler) respondTypeChangeError(w http.ResponseWriter, r *http.Request, err error) {
	var verr *validation.ValidationError
	if errors.As(err, &verr) {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeValidationFailed, verr.Message).WithDetails(map[string]string{"field": verr.Field}))
		return
	}
	h.RespondInternalError(w, r)
}

// Transition handles POST /rest/api/v1/items/{id}/transition.
// Unlike the generic Update endpoint, this hard-blocks on both validator-mode
// and condition-mode workflow conditions — it cannot be used to bypass
// transition rules.
//
// @Summary      Transition an item to a new status
// @Description  Performs a workflow transition with validator-mode and condition-mode rules enforced. Pending/rejected approvals return 409.
// @Tags         items
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                   true  "Item ID"
// @Param        body  body      dto.TransitionRequest true  "Target status and optional approval payload"
// @Success      200   {object}  dto.TransitionResultResponse
// @Failure      400   {object}  handlers.ErrorResponse  "Invalid request body, missing to_status_id, or transition rejected by a validator"
// @Failure      401   {object}  handlers.ErrorResponse
// @Failure      403   {object}  handlers.ErrorResponse  "Token lacks the items:write scope"
// @Failure      404   {object}  handlers.ErrorResponse  "Item not found or not visible to caller"
// @Failure      409   {object}  handlers.ErrorResponse  "Transition blocked by approval state (pending, rejected, or must-decide)"
// @Failure      500   {object}  handlers.ErrorResponse
// @Router       /items/{id}/transition [post]
func (h *ItemHandler) Transition(w http.ResponseWriter, r *http.Request) {
	item, user, ok := h.requireItemAccess(w, r, false, h.Perms.CanEditWorkspace)
	if !ok {
		return
	}

	var req dto.TransitionRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}
	if req.ToStatusID == nil {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeMissingField, "to_status_id is required"))
		return
	}

	result, err := h.workflowSvc.PerformTransition(r.Context(), services.PerformTransitionRequest{
		ItemID:      item.ID,
		ToStatusID:  *req.ToStatusID,
		ActorUserID: user.ID,
		Modes:       []string{"validator", "condition"},
	}, h.itemRepo, h.conditionSvc, h.approvalSvc)
	if err != nil {
		if rej := services.IsTransitionRejection(err); rej != nil {
			status := http.StatusBadRequest
			code := restapi.ErrCodeValidationFailed
			switch rej.Code {
			case "approval_must_decide", "approval_pending", "approval_rejected":
				status = http.StatusConflict
				code = restapi.ErrCodeConflict
			}
			details := map[string]any{"transition_code": rej.Code}
			for k, v := range rej.Details {
				details[k] = v
			}
			h.RespondError(w, r, restapi.NewAPIError(status, code, rej.Message).WithDetails(details))
			return
		}
		h.RespondInternalError(w, r)
		return
	}

	baseURL := getBaseURL(r)
	fullItem, err := h.itemRepo.FindByIDWithDetails(result.Item.ID)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	h.maskProjectNamesOne(user.ID, fullItem)
	h.RespondOK(w, dto.TransitionResultResponse{
		Item:        dto.MapItemToResponse(fullItem, baseURL),
		OldStatusID: result.OldStatusID,
		NewStatusID: result.NewStatusID,
		NoOp:        result.NoOp,
	})
}

// Delete handles DELETE /rest/api/v1/items/{id}
//
// @Summary      Delete an item
// @Description  Cascade-deletes the item along with its descendants, links, history and attachments.
// @Tags         items
// @Security     BearerAuth
// @Param        id   path  int  true  "Item ID"
// @Success      204  "Item deleted"
// @Failure      400  {object}  handlers.ErrorResponse  "Invalid item ID"
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the items:delete scope"
// @Failure      404  {object}  handlers.ErrorResponse  "Item not found or not visible to caller"
// @Failure      409  {object}  handlers.ErrorResponse  "Item or descendant still has a protected integration link"
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /items/{id} [delete]
func (h *ItemHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}
	itemID, ok := h.ParsePathID(w, r, "id", "item ID")
	if !ok {
		return
	}

	result, err := h.itemDeletion.Delete(services.ItemDeletionRequest{
		ItemID:        itemID,
		ActorUserID:   user.ID,
		ActorUsername: user.Username,
		Mode:          services.ItemDeletionCascade,
	})
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) || errors.Is(err, services.ErrItemDeletionForbidden) {
			h.RespondError(w, r, restapi.ErrItemNotFound)
			return
		}
		if errors.Is(err, services.ErrItemHasProtectedIntegrationLinks) {
			h.RespondError(w, r, restapi.NewAPIError(
				http.StatusConflict,
				restapi.ErrCodeConflict,
				"Remove all protected integration links from the affected items before deleting them.",
			))
			return
		}
		h.RespondInternalError(w, r)
		return
	}

	h.auditor.LogWithDetails(r, user, logger.ActionItemDeleteCascade, logger.ResourceItem, &itemID, result.Item.Title, map[string]any{
		"workspace_id":     result.Item.WorkspaceID,
		"item_type_id":     result.Item.ItemTypeID,
		"parent_id":        result.Item.ParentID,
		"status_id":        result.Item.StatusID,
		"assignee_id":      result.Item.AssigneeID,
		"creator_id":       result.Item.CreatorID,
		"deleted_count":    result.DeletedCount,
		"descendant_count": result.DescendantCount,
	})
	h.RespondNoContent(w)
}

// GetComments handles GET /rest/api/v1/items/{id}/comments
//
// @Summary      List comments on an item
// @Tags         items, comments
// @Produce      json
// @Security     BearerAuth
// @Param        id     path      int     true   "Item ID"
// @Param        page   query     int     false  "Page number (1-based)"
// @Param        limit  query     int     false  "Items per page (max 100)"
// @Param        order  query     string  false  "Sort order by creation time: asc or desc"
// @Success      200    {object}  handlers.PaginatedResponse{data=[]dto.CommentResponse}
// @Failure      400    {object}  handlers.ErrorResponse  "Invalid item ID"
// @Failure      401    {object}  handlers.ErrorResponse
// @Failure      403    {object}  handlers.ErrorResponse  "Token lacks the items:read scope"
// @Failure      404    {object}  handlers.ErrorResponse  "Item not found or not visible to caller"
// @Failure      500    {object}  handlers.ErrorResponse
// @Router       /items/{id}/comments [get]
func (h *ItemHandler) GetComments(w http.ResponseWriter, r *http.Request) {
	item, _, ok := h.requireItemAccess(w, r, false, h.Perms.CanViewWorkspace)
	if !ok {
		return
	}

	pagination := h.ParsePagination(r)
	comments, total, err := h.commentSvc.GetByItemIDPaginated(
		item.ID,
		pagination.Limit,
		pagination.Offset,
		pagination.SortAsc,
	)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	response := dto.MapCommentsToResponse(comments)
	h.RespondPaginated(w, response, pagination, total)
}

// CreateComment handles POST /rest/api/v1/items/{id}/comments
//
// @Summary      Create a comment on an item
// @Tags         items, comments
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                       true  "Item ID"
// @Param        body  body      dto.CommentCreateRequest  true  "Comment to create"
// @Success      201   {object}  dto.CommentResponse
// @Failure      400   {object}  handlers.ErrorResponse  "Invalid request body or missing required field"
// @Failure      401   {object}  handlers.ErrorResponse
// @Failure      403   {object}  handlers.ErrorResponse  "Token lacks the items:write scope"
// @Failure      404   {object}  handlers.ErrorResponse  "Item not found or not visible to caller"
// @Failure      500   {object}  handlers.ErrorResponse
// @Router       /items/{id}/comments [post]
func (h *ItemHandler) CreateComment(w http.ResponseWriter, r *http.Request) {
	item, user, ok := h.requireItemAccess(w, r, false, h.Perms.CanEditWorkspace)
	if !ok {
		return
	}

	itemID := item.ID

	var req dto.CommentCreateRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}
	if !h.ValidateRequiredString(w, r, req.Content, "content") {
		return
	}

	// Create comment using service
	result, err := h.commentSvc.Create(services.CreateCommentParams{
		ItemID:      itemID,
		AuthorID:    user.ID,
		Content:     req.Content,
		ActorUserID: user.ID,
	})
	if err != nil {
		var validationErr *validation.ValidationError
		if errors.As(err, &validationErr) {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeValidationFailed, validationErr.Message).WithDetails(map[string]string{"field": validationErr.Field}))
			return
		}
		h.RespondInternalError(w, r)
		return
	}
	contentHTML, err := markdown.Render(req.Content)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	// Build response with author info from the authenticated user
	fullName := user.FullName
	if fullName == "" {
		fullName = user.FirstName + " " + user.LastName
	}
	response := dto.CommentResponse{
		ID:          int(result.CommentID),
		ItemID:      itemID,
		Content:     req.Content,
		ContentHTML: contentHTML,
		Author: &dto.UserSummary{
			ID:        user.ID,
			Email:     user.Email,
			Username:  user.Username,
			FirstName: user.FirstName,
			LastName:  user.LastName,
			FullName:  fullName,
			AvatarURL: user.AvatarURL,
		},
	}
	h.RespondCreated(w, response)
}

// GetHistory handles GET /rest/api/v1/items/{id}/history
//
// @Summary      Get the change history of an item
// @Tags         items
// @Produce      json
// @Security     BearerAuth
// @Param        id     path      int     true   "Item ID"
// @Param        page   query     int     false  "Page number (1-based)"
// @Param        limit  query     int     false  "Items per page (max 100)"
// @Param        sort   query     string  false  "Sort field"
// @Param        order  query     string  false  "Sort order: asc or desc"
// @Success      200    {array}   dto.HistoryResponse
// @Failure      400    {object}  handlers.ErrorResponse  "Invalid item ID"
// @Failure      401    {object}  handlers.ErrorResponse
// @Failure      403    {object}  handlers.ErrorResponse  "Token lacks the items:read scope"
// @Failure      404    {object}  handlers.ErrorResponse  "Item not found or not visible to caller"
// @Failure      500    {object}  handlers.ErrorResponse
// @Router       /items/{id}/history [get]
func (h *ItemHandler) GetHistory(w http.ResponseWriter, r *http.Request) {
	item, _, ok := h.requireItemAccess(w, r, false, h.Perms.CanViewWorkspace)
	if !ok {
		return
	}

	history, err := h.itemCRUD.GetHistory(item.ID)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	response := dto.MapHistoryToResponses(history)
	h.RespondOK(w, response)
}

// GetTransitions handles GET /rest/api/v1/items/{id}/transitions
//
// @Summary      List workflow transitions available from the item's current status
// @Tags         items
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Item ID"
// @Success      200  {array}   dto.TransitionResponse
// @Failure      400  {object}  handlers.ErrorResponse  "Invalid item ID"
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the items:read scope"
// @Failure      404  {object}  handlers.ErrorResponse  "Item not found or not visible to caller"
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /items/{id}/transitions [get]
func (h *ItemHandler) GetTransitions(w http.ResponseWriter, r *http.Request) {
	item, _, ok := h.requireItemAccess(w, r, true, h.Perms.CanViewWorkspace)
	if !ok {
		return
	}

	if item.StatusID == nil {
		h.RespondOK(w, []dto.TransitionResponse{})
		return
	}

	transitions, err := h.workflowSvc.GetTransitionsForItem(item.WorkspaceID, item.ItemTypeID, *item.StatusID)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	response := dto.MapServiceTransitionsToResponse(transitions)
	h.RespondOK(w, response)
}

// GetAttachments handles GET /rest/api/v1/items/{id}/attachments
//
// @Summary      List attachments on an item
// @Tags         items
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Item ID"
// @Success      200  {array}   dto.AttachmentResponse
// @Failure      400  {object}  handlers.ErrorResponse  "Invalid item ID"
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the items:read scope"
// @Failure      404  {object}  handlers.ErrorResponse  "Item not found or not visible to caller"
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /items/{id}/attachments [get]
func (h *ItemHandler) GetAttachments(w http.ResponseWriter, r *http.Request) {
	item, _, ok := h.requireItemAccess(w, r, false, h.Perms.CanViewWorkspace)
	if !ok {
		return
	}

	attachments, err := h.itemCRUD.GetAttachments(item.ID)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	baseURL := getBaseURL(r)
	response := dto.MapAttachmentsToResponse(attachments, baseURL)
	h.RespondOK(w, response)
}

// GetChildren handles GET /rest/api/v1/items/{id}/children
//
// @Summary      List child items of an item
// @Tags         items
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Item ID"
// @Success      200  {array}   dto.ItemResponse
// @Failure      400  {object}  handlers.ErrorResponse  "Invalid item ID"
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the items:read scope"
// @Failure      404  {object}  handlers.ErrorResponse  "Item not found or not visible to caller"
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /items/{id}/children [get]
func (h *ItemHandler) GetChildren(w http.ResponseWriter, r *http.Request) {
	item, user, ok := h.requireItemAccess(w, r, false, h.Perms.CanViewWorkspace)
	if !ok {
		return
	}

	// Use service layer for getting children
	childrenPtrs, err := h.itemCRUD.GetChildren(item.ID)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	// Convert []*models.Item to []models.Item for DTO mapping
	children := make([]models.Item, len(childrenPtrs))
	for i, child := range childrenPtrs {
		children[i] = *child
	}

	h.maskProjectNames(user.ID, children)

	baseURL := getBaseURL(r)
	response := dto.MapItemsToResponse(children, baseURL)
	h.RespondOK(w, response)
}

// Search handles GET /rest/api/v1/search/items
//
// It supports two modes over the items the caller can view:
//
//   - Full-text search via `q` (e.g. `q=login bug`).
//   - Structured CQL filtering via `ql` (e.g. `ql=milestone = '0.8.2' AND status != Done`).
//
// When only `q` is supplied it is auto-detected: if it parses as a structured
// CQL filter it is evaluated as CQL, otherwise it is matched as free text. The
// explicit `ql` parameter forces CQL evaluation and returns 400 on a parse
// error, giving callers a way to surface query mistakes.
//
// @Summary      Search items
// @Description  Search items the caller can view by full-text (`q`) or structured CQL filter (`ql`). A `q` value that parses as CQL is evaluated as such.
// @Tags         items, search
// @Produce      json
// @Security     BearerAuth
// @Param        q                 query     string  false  "Full-text search query (auto-detected as CQL when it parses as a filter)"
// @Param        ql                query     string  false  "Structured CQL filter, e.g. milestone = '0.8.2' AND status != Done"
// @Param        page              query     int     false  "Page number (1-based)"
// @Param        limit             query     int     false  "Items per page (max 100)"
// @Param        sort              query     string  false  "Sort field"
// @Param        order             query     string  false  "Sort order: asc or desc"
// @Param        exclude_personal  query     bool    false  "Exclude items from the caller's personal workspaces"
// @Success      200    {object}  handlers.PaginatedResponse{data=[]dto.ItemResponse}
// @Failure      400    {object}  handlers.ErrorResponse  "Missing q/ql, or invalid CQL query"
// @Failure      401    {object}  handlers.ErrorResponse
// @Failure      403    {object}  handlers.ErrorResponse  "Token lacks the items:read scope"
// @Failure      500    {object}  handlers.ErrorResponse
// @Router       /search/items [get]
func (h *ItemHandler) Search(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	textQuery := strings.TrimSpace(r.URL.Query().Get("q"))
	qlQuery := strings.TrimSpace(r.URL.Query().Get("ql"))

	// Resolve the search mode. An explicit `ql` always forces CQL; otherwise a
	// `q` that parses as a structured filter is treated as CQL, and anything
	// else is a full-text term.
	useCQL := qlQuery != ""
	if !useCQL && textQuery != "" && cql.LooksLikeQuery(textQuery) {
		qlQuery = textQuery
		useCQL = true
	}
	if !useCQL && textQuery == "" {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeMissingField, "q or ql query parameter is required"))
		return
	}

	pagination := h.ParsePagination(r)

	accessibleWorkspaceIDs, err := h.Perms.GetAccessibleWorkspaceIDs(user.ID)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	if ExcludePersonal(r) {
		accessibleWorkspaceIDs, err = repository.FilterSharedWorkspaceIDs(h.DB, accessibleWorkspaceIDs)
		if err != nil {
			h.RespondInternalError(w, r)
			return
		}
	}

	if len(accessibleWorkspaceIDs) == 0 {
		h.RespondPaginated(w, []dto.ItemResponse{}, pagination, 0)
		return
	}

	var items []models.Item
	var total int
	if useCQL {
		items, total, err = h.itemCRUD.ListWithQL(services.ListWithQLParams{
			QLQuery:      qlQuery,
			WorkspaceIDs: accessibleWorkspaceIDs,
			UserID:       user.ID,
			Pagination: services.PaginationParams{
				Limit:  pagination.Limit,
				Offset: pagination.Offset,
			},
			SortBy:  pagination.SortBy,
			SortAsc: pagination.SortAsc,
		})
		if errors.Is(err, services.ErrQLQuery) {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeValidationFailed, err.Error()))
			return
		}
	} else {
		items, total, err = h.itemCRUD.Search(textQuery, accessibleWorkspaceIDs, services.PaginationParams{
			Limit:  pagination.Limit,
			Offset: pagination.Offset,
		})
	}
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	h.maskProjectNames(user.ID, items)

	baseURL := getBaseURL(r)
	response := dto.MapItemsToResponse(items, baseURL)
	h.RespondPaginated(w, response, pagination, total)
}

// Helper methods

func getBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if fwdProto := r.Header.Get("X-Forwarded-Proto"); fwdProto == "http" || fwdProto == "https" {
		scheme = fwdProto
	}
	prefix := sanitizedContextPrefix(r.Header.Get("X-Windshift-Context-Path"))
	return fmt.Sprintf("%s://%s%s", scheme, r.Host, prefix)
}

func sanitizedContextPrefix(prefix string) string {
	if prefix == "" || prefix == "/" || !strings.HasPrefix(prefix, "/") || strings.ContainsAny(prefix, "?#\\") || strings.Contains(prefix, "//") || strings.Contains(prefix, "..") {
		return ""
	}
	return strings.TrimSuffix(prefix, "/")
}
