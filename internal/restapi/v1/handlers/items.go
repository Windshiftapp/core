package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/restapi"
	"windshift/internal/restapi/v1/dto"
	"windshift/internal/services"
	"windshift/internal/utils"
	"windshift/internal/validation"
)

// ItemHandler handles public API requests for items
type ItemHandler struct {
	BaseHandler
	itemRepo     *repository.ItemRepository
	itemCRUD     *services.ItemCRUDService
	itemUpdate   *services.ItemUpdateService
	commentSvc   *services.CommentService
	workflowSvc  *services.WorkflowService
	conditionSvc *services.ConditionService
	approvalSvc  *services.ApprovalService
}

// NewItemHandler creates a new item handler
func NewItemHandler(db database.Database, permissionService *services.PermissionService) *ItemHandler {
	workflowSvc := services.NewWorkflowService(db)
	leaveRepo := repository.NewLeaveRepository(db)
	return &ItemHandler{
		BaseHandler:  NewBaseHandler(db, permissionService),
		itemRepo:     repository.NewItemRepository(db),
		itemCRUD:     services.NewItemCRUDService(db),
		itemUpdate:   services.NewItemUpdateService(db).WithPermissionService(permissionService),
		commentSvc:   services.NewCommentService(db),
		workflowSvc:  workflowSvc,
		conditionSvc: services.NewConditionService(db, permissionService, services.NewScriptEngine()),
		approvalSvc:  services.NewApprovalService(db, permissionService, leaveRepo, workflowSvc),
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

	// Parse pagination
	pagination := h.ParsePagination(r)

	// Get accessible workspace IDs for the user
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

	// Build filters from query parameters
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

	// Use service layer for listing items
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

	items, total, err := h.itemCRUD.List(params)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	// Convert to DTOs
	baseURL := getBaseURL(r)
	itemResponses := dto.MapItemsToResponse(items, baseURL)

	h.RespondPaginated(w, itemResponses, pagination, total)
}

// Get handles GET /rest/api/v1/items/{id}
//
// @Summary      Get an item by ID
// @Description  Returns 404 (not 403) when the item exists but isn't visible to the caller — item existence is never leaked.
// @Tags         items
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Item ID"
// @Success      200  {object}  dto.ItemResponse
// @Failure      400  {object}  handlers.ErrorResponse  "Invalid item ID"
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the items:read scope"
// @Failure      404  {object}  handlers.ErrorResponse  "Item not found or not visible to caller"
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /items/{id} [get]
func (h *ItemHandler) Get(w http.ResponseWriter, r *http.Request) {
	item, _, ok := h.requireItemAccess(w, r, true, h.Perms.CanViewWorkspace)
	if !ok {
		return
	}

	itemID := item.ID

	// Convert to DTO
	baseURL := getBaseURL(r)
	response := dto.MapItemToResponse(item, baseURL)

	// Handle expand parameter
	expand := restapi.ParseExpand(r)
	if expand.Comments {
		if comments, err := h.commentSvc.GetByItemID(itemID); err == nil {
			response.Comments = dto.MapCommentsToResponse(comments)
		}
	}
	if expand.History {
		if history, err := h.itemCRUD.GetHistory(itemID); err == nil {
			response.History = dto.MapHistoryToResponses(history)
		}
	}
	if expand.Attachments {
		if attachments, err := h.itemCRUD.GetAttachments(itemID); err == nil {
			response.Attachments = dto.MapAttachmentsToResponse(attachments, baseURL)
		}
	}
	if expand.Transitions {
		if item.StatusID != nil {
			if transitions, err := h.workflowSvc.GetTransitionsFromStatus(*item.StatusID); err == nil {
				response.Transitions = dto.MapServiceTransitionsToResponse(transitions)
			}
		} else {
			response.Transitions = []dto.TransitionResponse{}
		}
	}

	h.RespondOK(w, response)
}

// GetByKeyAndNumber handles GET /rest/api/v1/workspaces/{ws_key}/items/{number}.
// Looks up an item by its stable (workspace_key, workspace_item_number) pair —
// the form embedding clients should persist instead of the volatile numeric id.
//
// @Summary      Get an item by workspace key and per-workspace number
// @Description  Resolves an item by its stable (workspace_key, workspace_item_number) pair. Returns 404 (not 403) when the item exists but isn't visible to the caller — item existence is never leaked.
// @Tags         items
// @Produce      json
// @Security     BearerAuth
// @Param        ws_key  path      string  true  "Workspace key (e.g. PROJ)"
// @Param        number  path      int     true  "Per-workspace item number"
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

	baseURL := getBaseURL(r)
	response := dto.MapItemToResponse(item, baseURL)

	expand := restapi.ParseExpand(r)
	if expand.Comments {
		if comments, err := h.commentSvc.GetByItemID(itemID); err == nil {
			response.Comments = dto.MapCommentsToResponse(comments)
		}
	}
	if expand.History {
		if history, err := h.itemCRUD.GetHistory(itemID); err == nil {
			response.History = dto.MapHistoryToResponses(history)
		}
	}
	if expand.Attachments {
		if attachments, err := h.itemCRUD.GetAttachments(itemID); err == nil {
			response.Attachments = dto.MapAttachmentsToResponse(attachments, baseURL)
		}
	}
	if expand.Transitions {
		if item.StatusID != nil {
			if transitions, err := h.workflowSvc.GetTransitionsFromStatus(*item.StatusID); err == nil {
				response.Transitions = dto.MapServiceTransitionsToResponse(transitions)
			}
		} else {
			response.Transitions = []dto.TransitionResponse{}
		}
	}

	h.RespondOK(w, response)
}

// Create handles POST /rest/api/v1/items
//
// @Summary      Create an item
// @Description  Creates an item in the workspace specified by `workspace_id`. The caller must have edit permission on that workspace.
// @Tags         items
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      dto.ItemCreateRequest  true  "Item to create"
// @Success      201   {object}  dto.ItemResponse
// @Failure      400   {object}  handlers.ErrorResponse  "Invalid request body or missing required field"
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

	// Validate required fields
	if req.WorkspaceID == 0 {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeMissingField, "workspace_id is required"))
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeMissingField, "title is required"))
		return
	}

	// Check workspace permission
	canEdit, err := h.Perms.CanEditWorkspace(user.ID, req.WorkspaceID)
	if err != nil || !canEdit {
		h.RespondError(w, r, restapi.ErrInsufficientPermission)
		return
	}

	// Check item type is allowed in workspace config set
	if req.ItemTypeID != nil && *req.ItemTypeID != 0 {
		allowed, checkErr := services.IsItemTypeAllowedInWorkspace(h.DB, req.WorkspaceID, *req.ItemTypeID)
		if checkErr != nil {
			h.RespondInternalError(w, r)
			return
		}
		if !allowed {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeValidationFailed, "Item type is not allowed in this workspace"))
			return
		}
	}

	// Sanitize user input to prevent XSS
	req.Title = utils.StripHTMLTags(req.Title)
	req.Description = utils.SanitizeCommentContent(req.Description)

	// Convert custom field values to JSON
	var customFieldValuesJSON string
	if req.CustomFields != nil {
		var customFieldValuesBytes []byte
		customFieldValuesBytes, err = json.Marshal(req.CustomFields)
		if err != nil {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid custom field values"))
			return
		}
		customFieldValuesJSON = string(customFieldValuesBytes)
	}

	// Use centralized CreateItem service
	// StatusID and PriorityID can be nil - the service will resolve from workflow/defaults
	itemID, err := services.CreateItem(h.DB, services.ItemCreationParams{
		WorkspaceID:           req.WorkspaceID,
		Title:                 req.Title,
		Description:           req.Description,
		StatusID:              req.StatusID,   // nil = use workflow initial status
		PriorityID:            req.PriorityID, // nil = use default priority
		ItemTypeID:            req.ItemTypeID,
		IsTask:                req.IsTask,
		ParentID:              req.ParentID,
		MilestoneIDs:          req.MilestoneIDs,
		IterationID:           req.IterationID,
		ProjectID:             req.ProjectID,
		AssigneeID:            req.AssigneeID,
		CreatorID:             &user.ID,
		DueDate:               req.DueDate,
		StartDate:             req.StartDate,
		EndDate:               req.EndDate,
		CustomFieldValuesJSON: customFieldValuesJSON,
	})
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	// Load full item details for response
	fullItem, err := h.itemRepo.FindByIDWithDetails(int(itemID))
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	baseURL := getBaseURL(r)
	response := dto.MapItemToResponse(fullItem, baseURL)

	h.RespondCreated(w, response)
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

	// Read body once so we can reject status_id before decoding into the DTO.
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid request body"))
		return
	}

	// status_id must be changed via POST /rest/api/v1/items/{id}/transition so
	// workflow + condition rules are always enforced. Reject it here.
	var rawFields map[string]json.RawMessage
	if err := json.Unmarshal(bodyBytes, &rawFields); err != nil {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid request body"))
		return
	}
	if _, hasStatus := rawFields["status_id"]; hasStatus {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeValidationFailed,
			"status_id may not be set via item update; use POST /rest/api/v1/items/{id}/transition"))
		return
	}

	var req dto.ItemUpdateRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid request body"))
		return
	}

	// Build update data map for service.
	//
	// Pointer + omitempty in the DTO collapses two distinct client intents
	// ("don't change" vs "clear") into the same nil pointer. We disambiguate
	// by consulting rawFields: a key present with explicit JSON null on a
	// nullable FK is forwarded to the service as a typed nil so the
	// validator can null the column.
	updateData := make(map[string]interface{})
	isExplicitNull := func(field string) bool {
		raw, ok := rawFields[field]
		return ok && string(raw) == "null"
	}
	if req.Title != nil {
		updateData["title"] = utils.StripHTMLTags(*req.Title)
	}
	if req.Description != nil {
		updateData["description"] = utils.SanitizeCommentContent(*req.Description)
	}
	if req.PriorityID != nil {
		updateData["priority_id"] = *req.PriorityID
	} else if isExplicitNull("priority_id") {
		updateData["priority_id"] = nil
	}
	if req.ItemTypeID != nil {
		updateData["item_type_id"] = *req.ItemTypeID
	}
	if req.AssigneeID != nil {
		updateData["assignee_id"] = *req.AssigneeID
	} else if isExplicitNull("assignee_id") {
		updateData["assignee_id"] = nil
	}
	if req.ParentID != nil {
		updateData["parent_id"] = *req.ParentID
	} else if isExplicitNull("parent_id") {
		updateData["parent_id"] = nil
	}
	if req.MilestoneIDs != nil {
		// Pointer-to-slice present (including empty slice) means "replace set".
		// Pointer absent (nil) means "leave milestones untouched".
		updateData["milestone_ids"] = *req.MilestoneIDs
	}
	if req.IterationID != nil {
		updateData["iteration_id"] = *req.IterationID
	} else if isExplicitNull("iteration_id") {
		updateData["iteration_id"] = nil
	}
	if req.ProjectID != nil {
		updateData["project_id"] = *req.ProjectID
	} else if isExplicitNull("project_id") {
		updateData["project_id"] = nil
	}
	if req.DueDate != nil {
		updateData["due_date"] = *req.DueDate
	} else if isExplicitNull("due_date") {
		updateData["due_date"] = nil
	}
	if req.StartDate != nil {
		updateData["start_date"] = *req.StartDate
	} else if isExplicitNull("start_date") {
		updateData["start_date"] = nil
	}
	if req.EndDate != nil {
		updateData["end_date"] = *req.EndDate
	} else if isExplicitNull("end_date") {
		updateData["end_date"] = nil
	}
	if req.IsTask != nil {
		updateData["is_task"] = *req.IsTask
	}
	if req.CustomFields != nil {
		updateData["custom_field_values"] = req.CustomFields
	}

	// Use ItemUpdateService for update with history tracking
	result, err := h.itemUpdate.UpdateItem(services.UpdateItemRequest{
		ItemID:     itemID,
		UpdateData: updateData,
		UserID:     user.ID,
	})
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

	baseURL := getBaseURL(r)
	response := dto.MapItemToResponse(result.Item, baseURL)

	h.RespondOK(w, response)
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
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /items/{id} [delete]
func (h *ItemHandler) Delete(w http.ResponseWriter, r *http.Request) {
	item, _, ok := h.requireItemAccess(w, r, false, h.Perms.CanEditWorkspace)
	if !ok {
		return
	}

	// Use ItemCRUDService for cascade delete (handles descendants, links, history, etc.)
	_, err := h.itemCRUD.Delete(item.ID)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

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
// @Param        sort   query     string  false  "Sort field"
// @Param        order  query     string  false  "Sort order: asc or desc"
// @Success      200    {array}   dto.CommentResponse
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

	comments, err := h.commentSvc.GetByItemID(item.ID)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	response := dto.MapCommentsToResponse(comments)
	h.RespondOK(w, response)
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
		h.RespondInternalError(w, r)
		return
	}

	// Build response with author info from the authenticated user
	fullName := user.FullName
	if fullName == "" {
		fullName = user.FirstName + " " + user.LastName
	}
	response := dto.CommentResponse{
		ID:      int(result.CommentID),
		ItemID:  itemID,
		Content: req.Content,
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

	transitions, err := h.workflowSvc.GetTransitionsFromStatus(*item.StatusID)
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
	item, _, ok := h.requireItemAccess(w, r, false, h.Perms.CanViewWorkspace)
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

	baseURL := getBaseURL(r)
	response := dto.MapItemsToResponse(children, baseURL)
	h.RespondOK(w, response)
}

// Search handles GET /rest/api/v1/search/items
//
// @Summary      Search items
// @Description  Full-text search over items the caller can view. Requires a non-empty `q` query parameter.
// @Tags         items, search
// @Produce      json
// @Security     BearerAuth
// @Param        q      query     string  true   "Search query"
// @Param        page   query     int     false  "Page number (1-based)"
// @Param        limit  query     int     false  "Items per page (max 100)"
// @Param        sort   query     string  false  "Sort field"
// @Param        order  query     string  false  "Sort order: asc or desc"
// @Success      200    {object}  handlers.PaginatedResponse{data=[]dto.ItemResponse}
// @Failure      400    {object}  handlers.ErrorResponse  "Missing or invalid q parameter"
// @Failure      401    {object}  handlers.ErrorResponse
// @Failure      403    {object}  handlers.ErrorResponse  "Token lacks the items:read scope"
// @Failure      500    {object}  handlers.ErrorResponse
// @Router       /search/items [get]
func (h *ItemHandler) Search(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeMissingField, "q query parameter is required"))
		return
	}

	pagination := h.ParsePagination(r)

	accessibleWorkspaceIDs, err := h.Perms.GetAccessibleWorkspaceIDs(user.ID)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	if len(accessibleWorkspaceIDs) == 0 {
		h.RespondPaginated(w, []dto.ItemResponse{}, pagination, 0)
		return
	}

	// Use service layer for search
	items, total, err := h.itemCRUD.Search(query, accessibleWorkspaceIDs, services.PaginationParams{
		Limit:  pagination.Limit,
		Offset: pagination.Offset,
	})
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

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
	return fmt.Sprintf("%s://%s", scheme, r.Host)
}
