package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/restapi"
	"windshift/internal/restapi/v1/dto"
	"windshift/internal/restapi/v1/middleware"
	"windshift/internal/restapi/v1/shared"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// ItemHandler handles public API requests for items
type ItemHandler struct {
	db          database.Database
	itemRepo    *repository.ItemRepository
	perms       *shared.PermissionHelper
	itemCRUD    *services.ItemCRUDService
	itemUpdate  *services.ItemUpdateService
	commentSvc  *services.CommentService
	workflowSvc *services.WorkflowService
}

// NewItemHandler creates a new item handler
func NewItemHandler(db database.Database, permissionService *services.PermissionService) *ItemHandler {
	return &ItemHandler{
		db:          db,
		itemRepo:    repository.NewItemRepository(db),
		perms:       shared.NewPermissionHelper(db, permissionService),
		itemCRUD:    services.NewItemCRUDService(db),
		itemUpdate:  services.NewItemUpdateService(db),
		commentSvc:  services.NewCommentService(db),
		workflowSvc: services.NewWorkflowService(db),
	}
}

// List handles GET /rest/api/v1/items
func (h *ItemHandler) List(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	// Parse pagination
	pagination := restapi.ParsePaginationParams(r)

	// Get accessible workspace IDs for the user
	accessibleWorkspaceIDs, err := h.perms.GetAccessibleWorkspaceIDs(user.ID)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError.WithDetails(map[string]string{
			"message": "Failed to get accessible workspaces",
		}))
		return
	}

	if len(accessibleWorkspaceIDs) == 0 {
		restapi.RespondPaginated(w, []dto.ItemResponse{}, restapi.NewPaginationMeta(pagination, 0))
		return
	}

	// Build filters from query parameters
	filters := services.ItemFilters{}
	if wsID := r.URL.Query().Get("workspace_id"); wsID != "" {
		id, _ := strconv.Atoi(wsID)
		filters.WorkspaceID = &id
	}
	if statusID := r.URL.Query().Get("status_id"); statusID != "" {
		id, _ := strconv.Atoi(statusID)
		filters.StatusID = &id
	}
	if priorityID := r.URL.Query().Get("priority_id"); priorityID != "" {
		id, _ := strconv.Atoi(priorityID)
		filters.PriorityID = &id
	}
	if assigneeID := r.URL.Query().Get("assignee_id"); assigneeID != "" {
		id, _ := strconv.Atoi(assigneeID)
		filters.AssigneeID = &id
	}
	if itemTypeID := r.URL.Query().Get("item_type_id"); itemTypeID != "" {
		id, _ := strconv.Atoi(itemTypeID)
		filters.ItemTypeID = &id
	}
	if creatorID := r.URL.Query().Get("creator_id"); creatorID != "" {
		id, _ := strconv.Atoi(creatorID)
		filters.CreatorID = &id
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
		restapi.RespondError(w, r, restapi.ErrInternalError.WithDetails(map[string]string{
			"message": err.Error(),
		}))
		return
	}

	// Convert to DTOs
	baseURL := getBaseURL(r)
	itemResponses := dto.MapItemsToResponse(items, baseURL)

	restapi.RespondPaginated(w, itemResponses, restapi.NewPaginationMeta(pagination, total))
}

// Get handles GET /rest/api/v1/items/{id}
func (h *ItemHandler) Get(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	itemID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid item ID"))
		return
	}

	// Load item with details
	item, err := h.itemRepo.FindByIDWithDetails(itemID)
	if err != nil {
		if err == repository.ErrNotFound {
			restapi.RespondError(w, r, restapi.ErrItemNotFound)
			return
		}
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	// Check permission
	canView, err := h.perms.CanViewWorkspace(user.ID, item.WorkspaceID)
	if err != nil || !canView {
		restapi.RespondError(w, r, restapi.ErrInsufficientPermission)
		return
	}

	// Convert to DTO
	baseURL := getBaseURL(r)
	response := dto.MapItemToResponse(item, baseURL)

	// Handle expand parameter
	expand := restapi.ParseExpand(r)
	if expand.Comments {
		comments, _ := h.commentSvc.GetByItemID(itemID)
		response.Comments = dto.MapCommentsToResponse(comments)
	}
	if expand.History {
		history, _ := h.itemCRUD.GetHistory(itemID)
		response.History = dto.MapHistoryToResponses(history)
	}
	if expand.Attachments {
		attachments, _ := h.itemCRUD.GetAttachments(itemID)
		response.Attachments = dto.MapAttachmentsToResponse(attachments, baseURL)
	}
	if expand.Transitions {
		if item.StatusID != nil {
			transitions, _ := h.workflowSvc.GetTransitionsFromStatus(*item.StatusID)
			response.Transitions = dto.MapServiceTransitionsToResponse(transitions)
		} else {
			response.Transitions = []dto.TransitionResponse{}
		}
	}

	restapi.RespondOK(w, response)
}

// Create handles POST /rest/api/v1/items
func (h *ItemHandler) Create(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	var req dto.ItemCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid JSON body"))
		return
	}

	// Validate required fields
	if req.WorkspaceID == 0 {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeMissingField, "workspace_id is required"))
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeMissingField, "title is required"))
		return
	}

	// Check workspace permission
	canEdit, err := h.perms.CanEditWorkspace(user.ID, req.WorkspaceID)
	if err != nil || !canEdit {
		restapi.RespondError(w, r, restapi.ErrInsufficientPermission)
		return
	}

	// Sanitize user input to prevent XSS
	req.Title = utils.StripHTMLTags(req.Title)
	req.Description = utils.SanitizeCommentContent(req.Description)

	// Convert custom field values to JSON
	var customFieldValuesJSON string
	if req.CustomFields != nil {
		customFieldValuesBytes, err := json.Marshal(req.CustomFields)
		if err != nil {
			restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid custom field values"))
			return
		}
		customFieldValuesJSON = string(customFieldValuesBytes)
	}

	// Use centralized CreateItem service
	// StatusID and PriorityID can be nil - the service will resolve from workflow/defaults
	itemID, err := services.CreateItem(h.db, services.ItemCreationParams{
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
		CustomFieldValuesJSON: customFieldValuesJSON,
	})
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError.WithDetails(map[string]string{
			"message": err.Error(),
		}))
		return
	}

	// Load full item details for response
	fullItem, err := h.itemRepo.FindByIDWithDetails(int(itemID))
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	baseURL := getBaseURL(r)
	response := dto.MapItemToResponse(fullItem, baseURL)

	restapi.RespondCreated(w, response)
}

// Update handles PUT /rest/api/v1/items/{id}
func (h *ItemHandler) Update(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	itemID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid item ID"))
		return
	}

	// Load existing item to check permissions
	item, err := h.itemRepo.FindByIDWithDetails(itemID)
	if err != nil {
		if err == repository.ErrNotFound {
			restapi.RespondError(w, r, restapi.ErrItemNotFound)
			return
		}
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	// Check permission
	canEdit, err := h.perms.CanEditWorkspace(user.ID, item.WorkspaceID)
	if err != nil || !canEdit {
		restapi.RespondError(w, r, restapi.ErrInsufficientPermission)
		return
	}

	var req dto.ItemUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid JSON body"))
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
	if req.StatusID != nil {
		updateData["status_id"] = *req.StatusID
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
		restapi.RespondError(w, r, restapi.ErrInternalError.WithDetails(map[string]string{
			"message": err.Error(),
		}))
		return
	}

	baseURL := getBaseURL(r)
	response := dto.MapItemToResponse(result.Item, baseURL)

	restapi.RespondOK(w, response)
}

// Delete handles DELETE /rest/api/v1/items/{id}
func (h *ItemHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	itemID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid item ID"))
		return
	}

	// Load item to check permissions
	item, err := h.itemRepo.FindByID(itemID)
	if err != nil {
		if err == repository.ErrNotFound {
			restapi.RespondError(w, r, restapi.ErrItemNotFound)
			return
		}
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	// Check permission
	canEdit, err := h.perms.CanEditWorkspace(user.ID, item.WorkspaceID)
	if err != nil || !canEdit {
		restapi.RespondError(w, r, restapi.ErrInsufficientPermission)
		return
	}

	// Use ItemCRUDService for cascade delete (handles descendants, links, history, etc.)
	_, err = h.itemCRUD.Delete(itemID)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError.WithDetails(map[string]string{
			"message": err.Error(),
		}))
		return
	}

	restapi.RespondNoContent(w)
}

// GetComments handles GET /rest/api/v1/items/{id}/comments
func (h *ItemHandler) GetComments(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	itemID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid item ID"))
		return
	}

	// Load item to check permissions
	item, err := h.itemRepo.FindByID(itemID)
	if err != nil {
		if err == repository.ErrNotFound {
			restapi.RespondError(w, r, restapi.ErrItemNotFound)
			return
		}
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	canView, err := h.perms.CanViewWorkspace(user.ID, item.WorkspaceID)
	if err != nil || !canView {
		restapi.RespondError(w, r, restapi.ErrInsufficientPermission)
		return
	}

	comments, err := h.commentSvc.GetByItemID(itemID)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	response := dto.MapCommentsToResponse(comments)
	restapi.RespondOK(w, response)
}

// CreateComment handles POST /rest/api/v1/items/{id}/comments
func (h *ItemHandler) CreateComment(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	itemID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid item ID"))
		return
	}

	// Load item to check permissions
	item, err := h.itemRepo.FindByID(itemID)
	if err != nil {
		if err == repository.ErrNotFound {
			restapi.RespondError(w, r, restapi.ErrItemNotFound)
			return
		}
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	canEdit, err := h.perms.CanEditWorkspace(user.ID, item.WorkspaceID)
	if err != nil || !canEdit {
		restapi.RespondError(w, r, restapi.ErrInsufficientPermission)
		return
	}

	var req dto.CommentCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid JSON body"))
		return
	}

	if strings.TrimSpace(req.Content) == "" {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeMissingField, "content is required"))
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
		restapi.RespondError(w, r, restapi.ErrInternalError.WithDetails(map[string]string{
			"message": err.Error(),
		}))
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
	restapi.RespondCreated(w, response)
}

// GetHistory handles GET /rest/api/v1/items/{id}/history
func (h *ItemHandler) GetHistory(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	itemID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid item ID"))
		return
	}

	// Load item to check permissions
	item, err := h.itemRepo.FindByID(itemID)
	if err != nil {
		if err == repository.ErrNotFound {
			restapi.RespondError(w, r, restapi.ErrItemNotFound)
			return
		}
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	canView, err := h.perms.CanViewWorkspace(user.ID, item.WorkspaceID)
	if err != nil || !canView {
		restapi.RespondError(w, r, restapi.ErrInsufficientPermission)
		return
	}

	history, err := h.itemCRUD.GetHistory(itemID)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	response := dto.MapHistoryToResponses(history)
	restapi.RespondOK(w, response)
}

// GetTransitions handles GET /rest/api/v1/items/{id}/transitions
func (h *ItemHandler) GetTransitions(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	itemID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid item ID"))
		return
	}

	item, err := h.itemRepo.FindByIDWithDetails(itemID)
	if err != nil {
		if err == repository.ErrNotFound {
			restapi.RespondError(w, r, restapi.ErrItemNotFound)
			return
		}
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	canView, err := h.perms.CanViewWorkspace(user.ID, item.WorkspaceID)
	if err != nil || !canView {
		restapi.RespondError(w, r, restapi.ErrInsufficientPermission)
		return
	}

	if item.StatusID == nil {
		restapi.RespondOK(w, []dto.TransitionResponse{})
		return
	}

	transitions, err := h.workflowSvc.GetTransitionsFromStatus(*item.StatusID)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	response := dto.MapServiceTransitionsToResponse(transitions)
	restapi.RespondOK(w, response)
}

// GetAttachments handles GET /rest/api/v1/items/{id}/attachments
func (h *ItemHandler) GetAttachments(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	itemID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid item ID"))
		return
	}

	item, err := h.itemRepo.FindByID(itemID)
	if err != nil {
		if err == repository.ErrNotFound {
			restapi.RespondError(w, r, restapi.ErrItemNotFound)
			return
		}
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	canView, err := h.perms.CanViewWorkspace(user.ID, item.WorkspaceID)
	if err != nil || !canView {
		restapi.RespondError(w, r, restapi.ErrInsufficientPermission)
		return
	}

	attachments, err := h.itemCRUD.GetAttachments(itemID)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	baseURL := getBaseURL(r)
	response := dto.MapAttachmentsToResponse(attachments, baseURL)
	restapi.RespondOK(w, response)
}

// GetChildren handles GET /rest/api/v1/items/{id}/children
func (h *ItemHandler) GetChildren(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	itemID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid item ID"))
		return
	}

	item, err := h.itemRepo.FindByID(itemID)
	if err != nil {
		if err == repository.ErrNotFound {
			restapi.RespondError(w, r, restapi.ErrItemNotFound)
			return
		}
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	canView, err := h.perms.CanViewWorkspace(user.ID, item.WorkspaceID)
	if err != nil || !canView {
		restapi.RespondError(w, r, restapi.ErrInsufficientPermission)
		return
	}

	// Use service layer for getting children
	childrenPtrs, err := h.itemCRUD.GetChildren(itemID)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	// Convert []*models.Item to []models.Item for DTO mapping
	children := make([]models.Item, len(childrenPtrs))
	for i, child := range childrenPtrs {
		children[i] = *child
	}

	baseURL := getBaseURL(r)
	response := dto.MapItemsToResponse(children, baseURL)
	restapi.RespondOK(w, response)
}

// Search handles GET /rest/api/v1/search/items
func (h *ItemHandler) Search(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeMissingField, "q query parameter is required"))
		return
	}

	pagination := restapi.ParsePaginationParams(r)

	accessibleWorkspaceIDs, err := h.perms.GetAccessibleWorkspaceIDs(user.ID)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	if len(accessibleWorkspaceIDs) == 0 {
		restapi.RespondPaginated(w, []dto.ItemResponse{}, restapi.NewPaginationMeta(pagination, 0))
		return
	}

	// Use service layer for search
	items, total, err := h.itemCRUD.Search(query, accessibleWorkspaceIDs, services.PaginationParams{
		Limit:  pagination.Limit,
		Offset: pagination.Offset,
	})
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError.WithDetails(map[string]string{
			"message": err.Error(),
		}))
		return
	}

	baseURL := getBaseURL(r)
	response := dto.MapItemsToResponse(items, baseURL)
	restapi.RespondPaginated(w, response, restapi.NewPaginationMeta(pagination, total))
}

// Helper methods

func getBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if fwdProto := r.Header.Get("X-Forwarded-Proto"); fwdProto != "" {
		scheme = fwdProto
	}
	return fmt.Sprintf("%s://%s", scheme, r.Host)
}

// nullStringValue safely extracts value from sql.NullString
// Used by multiple handlers in this package
func nullStringValue(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}
