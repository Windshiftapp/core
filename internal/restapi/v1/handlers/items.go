package handlers

import (
	"encoding/json"
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
		MilestoneID:           req.MilestoneID,
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

	// Build update data map for service
	updateData := make(map[string]interface{})
	if req.Title != nil {
		updateData["title"] = utils.StripHTMLTags(*req.Title)
	}
	if req.Description != nil {
		updateData["description"] = utils.SanitizeCommentContent(*req.Description)
	}
	if req.PriorityID != nil {
		updateData["priority_id"] = *req.PriorityID
	}
	if req.ItemTypeID != nil {
		updateData["item_type_id"] = *req.ItemTypeID
	}
	if req.AssigneeID != nil {
		updateData["assignee_id"] = *req.AssigneeID
	}
	if req.ParentID != nil {
		updateData["parent_id"] = *req.ParentID
	}
	if req.MilestoneID != nil {
		updateData["milestone_id"] = *req.MilestoneID
	}
	if req.IterationID != nil {
		updateData["iteration_id"] = *req.IterationID
	}
	if req.ProjectID != nil {
		updateData["project_id"] = *req.ProjectID
	}
	if req.DueDate != nil {
		updateData["due_date"] = *req.DueDate
	}
	if req.StartDate != nil {
		updateData["start_date"] = *req.StartDate
	}
	if req.EndDate != nil {
		updateData["end_date"] = *req.EndDate
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
