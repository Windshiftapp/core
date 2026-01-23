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

// allowedSortColumns maps user-provided sort field names to safe SQL column references
// This prevents SQL injection via the sort parameter
var allowedSortColumns = map[string]string{
	"created_at":  "i.created_at",
	"updated_at":  "i.updated_at",
	"title":       "i.title",
	"due_date":    "i.due_date",
	"priority_id": "i.priority_id",
	"status_id":   "i.status_id",
	"rank":        "i.rank",
}

// ItemHandler handles public API requests for items
type ItemHandler struct {
	db       database.Database
	itemRepo *repository.ItemRepository
	perms    *shared.PermissionHelper
}

// NewItemHandler creates a new item handler
func NewItemHandler(db database.Database, permissionService *services.PermissionService) *ItemHandler {
	return &ItemHandler{
		db:       db,
		itemRepo: repository.NewItemRepository(db),
		perms:    shared.NewPermissionHelper(db, permissionService),
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

	// Build base query
	baseURL := getBaseURL(r)
	items, total, err := h.queryItems(r, accessibleWorkspaceIDs, pagination)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError.WithDetails(map[string]string{
			"message": err.Error(),
		}))
		return
	}

	// Convert to DTOs
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
		comments, _ := h.getItemComments(itemID)
		response.Comments = dto.MapCommentsToResponse(comments)
	}
	if expand.History {
		history, _ := h.getItemHistory(itemID)
		response.History = dto.MapHistoryToResponses(history)
	}
	if expand.Attachments {
		attachments, _ := h.getItemAttachments(itemID)
		response.Attachments = dto.MapAttachmentsToResponse(attachments, baseURL)
	}
	if expand.Transitions {
		transitions, _ := h.getItemTransitions(item)
		response.Transitions = dto.MapTransitionsToResponse(transitions)
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
	req.Description = utils.StripHTMLTags(req.Description)

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

	// Load existing item
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

	// Apply updates (sanitize to prevent XSS)
	if req.Title != nil {
		item.Title = utils.StripHTMLTags(*req.Title)
	}
	if req.Description != nil {
		item.Description = utils.StripHTMLTags(*req.Description)
	}
	if req.StatusID != nil {
		item.StatusID = req.StatusID
	}
	if req.PriorityID != nil {
		item.PriorityID = req.PriorityID
	}
	if req.ItemTypeID != nil {
		item.ItemTypeID = req.ItemTypeID
	}
	if req.AssigneeID != nil {
		item.AssigneeID = req.AssigneeID
	}
	if req.ParentID != nil {
		item.ParentID = req.ParentID
	}
	if req.MilestoneID != nil {
		item.MilestoneID = req.MilestoneID
	}
	if req.IterationID != nil {
		item.IterationID = req.IterationID
	}
	if req.ProjectID != nil {
		item.ProjectID = req.ProjectID
	}
	if req.DueDate != nil {
		item.DueDate = req.DueDate
	}
	if req.IsTask != nil {
		item.IsTask = *req.IsTask
	}
	if req.CustomFields != nil {
		item.CustomFieldValues = req.CustomFields
	}

	// Update item
	if err := h.updateItem(item); err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError.WithDetails(map[string]string{
			"message": err.Error(),
		}))
		return
	}

	// Load updated item
	updatedItem, err := h.itemRepo.FindByIDWithDetails(itemID)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	baseURL := getBaseURL(r)
	response := dto.MapItemToResponse(updatedItem, baseURL)

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

	// Delete item
	if err := h.deleteItem(itemID); err != nil {
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

	comments, err := h.getItemComments(itemID)
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

	// Create comment
	comment, err := h.createComment(itemID, user.ID, req.Content)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError.WithDetails(map[string]string{
			"message": err.Error(),
		}))
		return
	}

	response := dto.MapCommentToResponse(comment)
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

	history, err := h.getItemHistory(itemID)
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

	transitions, err := h.getItemTransitions(item)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	response := dto.MapTransitionsToResponse(transitions)
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

	attachments, err := h.getItemAttachments(itemID)
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

	children, err := h.getItemChildren(itemID)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
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

	items, total, err := h.searchItems(query, accessibleWorkspaceIDs, pagination)
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

func (h *ItemHandler) queryItems(r *http.Request, workspaceIDs []int, pagination restapi.PaginationParams) ([]models.Item, int, error) {
	// Build query with filters
	baseQuery := `
		SELECT i.id, i.workspace_id, i.workspace_item_number, i.item_type_id, i.title, i.description,
		       i.status_id, i.priority_id, i.due_date, i.is_task, i.milestone_id, i.iteration_id,
		       i.project_id, i.assignee_id, i.creator_id, i.custom_field_values, i.parent_id,
		       i.created_at, i.updated_at,
		       w.name as workspace_name, w.key as workspace_key,
		       it.name as item_type_name,
		       st.name as status_name,
		       pri.name as priority_name, pri.icon as priority_icon, pri.color as priority_color,
		       COALESCE(assignee.first_name || ' ' || assignee.last_name, '') as assignee_name,
		       COALESCE(assignee.email, '') as assignee_email,
		       COALESCE(assignee.avatar_url, '') as assignee_avatar,
		       COALESCE(creator.first_name || ' ' || creator.last_name, '') as creator_name,
		       COALESCE(creator.email, '') as creator_email,
		       m.name as milestone_name, iter.name as iteration_name, proj.name as project_name
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN item_types it ON i.item_type_id = it.id
		LEFT JOIN statuses st ON i.status_id = st.id
		LEFT JOIN priorities pri ON i.priority_id = pri.id
		LEFT JOIN users assignee ON i.assignee_id = assignee.id
		LEFT JOIN users creator ON i.creator_id = creator.id
		LEFT JOIN milestones m ON i.milestone_id = m.id
		LEFT JOIN iterations iter ON i.iteration_id = iter.id
		LEFT JOIN time_projects proj ON i.project_id = proj.id
	`

	whereClause := "WHERE 1=1"
	args := []interface{}{}

	// Filter by accessible workspaces
	if len(workspaceIDs) > 0 {
		placeholders := make([]string, len(workspaceIDs))
		for i, id := range workspaceIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		whereClause += " AND i.workspace_id IN (" + strings.Join(placeholders, ",") + ")"
	}

	// Apply query filters
	if wsID := r.URL.Query().Get("workspace_id"); wsID != "" {
		whereClause += " AND i.workspace_id = ?"
		args = append(args, wsID)
	}
	if statusID := r.URL.Query().Get("status_id"); statusID != "" {
		whereClause += " AND i.status_id = ?"
		args = append(args, statusID)
	}
	if priorityID := r.URL.Query().Get("priority_id"); priorityID != "" {
		whereClause += " AND i.priority_id = ?"
		args = append(args, priorityID)
	}
	if assigneeID := r.URL.Query().Get("assignee_id"); assigneeID != "" {
		whereClause += " AND i.assignee_id = ?"
		args = append(args, assigneeID)
	}
	if itemTypeID := r.URL.Query().Get("item_type_id"); itemTypeID != "" {
		whereClause += " AND i.item_type_id = ?"
		args = append(args, itemTypeID)
	}
	if creatorID := r.URL.Query().Get("creator_id"); creatorID != "" {
		whereClause += " AND i.creator_id = ?"
		args = append(args, creatorID)
	}

	// Count total
	countQuery := "SELECT COUNT(*) FROM items i " + whereClause
	var total int
	if err := h.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Add ordering and pagination
	orderBy := " ORDER BY i.created_at DESC"
	if pagination.SortBy != "" {
		if col, ok := allowedSortColumns[pagination.SortBy]; ok {
			orderBy = fmt.Sprintf(" ORDER BY %s", col)
			if pagination.SortAsc {
				orderBy += " ASC"
			} else {
				orderBy += " DESC"
			}
		}
		// Invalid sort columns silently fall back to default (created_at DESC)
	}

	fullQuery := baseQuery + whereClause + orderBy + fmt.Sprintf(" LIMIT %d OFFSET %d", pagination.Limit, pagination.Offset)
	rows, err := h.db.Query(fullQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	return scanItems(rows)
}

func (h *ItemHandler) searchItems(query string, workspaceIDs []int, pagination restapi.PaginationParams) ([]models.Item, int, error) {
	searchQuery := "%" + query + "%"

	placeholders := make([]string, len(workspaceIDs))
	args := []interface{}{searchQuery, searchQuery}
	for i, id := range workspaceIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}

	whereClause := fmt.Sprintf(`WHERE (i.title LIKE ? OR i.description LIKE ?) AND i.workspace_id IN (%s)`, strings.Join(placeholders, ","))

	// Count
	countQuery := "SELECT COUNT(*) FROM items i " + whereClause
	var total int
	if err := h.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Query
	baseQuery := `
		SELECT i.id, i.workspace_id, i.workspace_item_number, i.item_type_id, i.title, i.description,
		       i.status_id, i.priority_id, i.due_date, i.is_task, i.milestone_id, i.iteration_id,
		       i.project_id, i.assignee_id, i.creator_id, i.custom_field_values, i.parent_id,
		       i.created_at, i.updated_at,
		       w.name as workspace_name, w.key as workspace_key,
		       it.name as item_type_name,
		       st.name as status_name,
		       pri.name as priority_name, pri.icon as priority_icon, pri.color as priority_color,
		       COALESCE(assignee.first_name || ' ' || assignee.last_name, '') as assignee_name,
		       COALESCE(assignee.email, '') as assignee_email,
		       COALESCE(assignee.avatar_url, '') as assignee_avatar,
		       COALESCE(creator.first_name || ' ' || creator.last_name, '') as creator_name,
		       COALESCE(creator.email, '') as creator_email,
		       m.name as milestone_name, iter.name as iteration_name, proj.name as project_name
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN item_types it ON i.item_type_id = it.id
		LEFT JOIN statuses st ON i.status_id = st.id
		LEFT JOIN priorities pri ON i.priority_id = pri.id
		LEFT JOIN users assignee ON i.assignee_id = assignee.id
		LEFT JOIN users creator ON i.creator_id = creator.id
		LEFT JOIN milestones m ON i.milestone_id = m.id
		LEFT JOIN iterations iter ON i.iteration_id = iter.id
		LEFT JOIN time_projects proj ON i.project_id = proj.id
	`

	fullQuery := baseQuery + whereClause + fmt.Sprintf(" ORDER BY i.created_at DESC LIMIT %d OFFSET %d", pagination.Limit, pagination.Offset)
	rows, err := h.db.Query(fullQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	return scanItems(rows)
}

func scanItems(rows *sql.Rows) ([]models.Item, int, error) {
	var items []models.Item
	for rows.Next() {
		var item models.Item
		var itemTypeID, statusID, priorityID, parentID, milestoneID, iterationID, projectID sql.NullInt64
		var assigneeID, creatorID sql.NullInt64
		var dueDate sql.NullTime
		var customFieldValuesJSON sql.NullString
		var itemTypeName, statusName, priorityName, priorityIcon, priorityColor sql.NullString
		var assigneeName, assigneeEmail, assigneeAvatar, creatorName, creatorEmail sql.NullString
		var milestoneName, iterationName, projectName sql.NullString

		err := rows.Scan(
			&item.ID, &item.WorkspaceID, &item.WorkspaceItemNumber, &itemTypeID, &item.Title, &item.Description,
			&statusID, &priorityID, &dueDate, &item.IsTask, &milestoneID, &iterationID,
			&projectID, &assigneeID, &creatorID, &customFieldValuesJSON, &parentID,
			&item.CreatedAt, &item.UpdatedAt,
			&item.WorkspaceName, &item.WorkspaceKey,
			&itemTypeName, &statusName, &priorityName, &priorityIcon, &priorityColor,
			&assigneeName, &assigneeEmail, &assigneeAvatar, &creatorName, &creatorEmail,
			&milestoneName, &iterationName, &projectName,
		)
		if err != nil {
			continue
		}

		// Handle nullable fields
		if itemTypeID.Valid {
			id := int(itemTypeID.Int64)
			item.ItemTypeID = &id
		}
		if statusID.Valid {
			id := int(statusID.Int64)
			item.StatusID = &id
		}
		if priorityID.Valid {
			id := int(priorityID.Int64)
			item.PriorityID = &id
		}
		if parentID.Valid {
			id := int(parentID.Int64)
			item.ParentID = &id
		}
		if milestoneID.Valid {
			id := int(milestoneID.Int64)
			item.MilestoneID = &id
		}
		if iterationID.Valid {
			id := int(iterationID.Int64)
			item.IterationID = &id
		}
		if projectID.Valid {
			id := int(projectID.Int64)
			item.ProjectID = &id
		}
		if assigneeID.Valid {
			id := int(assigneeID.Int64)
			item.AssigneeID = &id
		}
		if creatorID.Valid {
			id := int(creatorID.Int64)
			item.CreatorID = &id
		}
		if dueDate.Valid {
			item.DueDate = &dueDate.Time
		}

		// Handle nullable strings
		item.ItemTypeName = nullStringValue(itemTypeName)
		item.StatusName = nullStringValue(statusName)
		item.PriorityName = nullStringValue(priorityName)
		item.PriorityIcon = nullStringValue(priorityIcon)
		item.PriorityColor = nullStringValue(priorityColor)
		item.AssigneeName = nullStringValue(assigneeName)
		item.AssigneeEmail = nullStringValue(assigneeEmail)
		item.AssigneeAvatar = nullStringValue(assigneeAvatar)
		item.CreatorName = nullStringValue(creatorName)
		item.CreatorEmail = nullStringValue(creatorEmail)
		item.MilestoneName = nullStringValue(milestoneName)
		item.IterationName = nullStringValue(iterationName)
		item.ProjectName = nullStringValue(projectName)

		// Parse custom fields
		if customFieldValuesJSON.Valid && customFieldValuesJSON.String != "" {
			json.Unmarshal([]byte(customFieldValuesJSON.String), &item.CustomFieldValues)
		}

		items = append(items, item)
	}

	return items, len(items), nil
}

func nullStringValue(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}


func (h *ItemHandler) updateItem(item *models.Item) error {
	customFieldsJSON, _ := json.Marshal(item.CustomFieldValues)

	_, err := h.db.ExecWrite(`
		UPDATE items SET
			title = ?, description = ?, status_id = ?, priority_id = ?, due_date = ?,
			is_task = ?, milestone_id = ?, iteration_id = ?, project_id = ?,
			assignee_id = ?, item_type_id = ?, custom_field_values = ?, parent_id = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, item.Title, item.Description, item.StatusID, item.PriorityID, item.DueDate,
		item.IsTask, item.MilestoneID, item.IterationID, item.ProjectID,
		item.AssigneeID, item.ItemTypeID, string(customFieldsJSON), item.ParentID, item.ID)

	return err
}

func (h *ItemHandler) deleteItem(itemID int) error {
	_, err := h.db.ExecWrite("DELETE FROM items WHERE id = ?", itemID)
	return err
}

func (h *ItemHandler) getItemComments(itemID int) ([]models.Comment, error) {
	rows, err := h.db.Query(`
		SELECT c.id, c.item_id, c.author_id, c.content, c.created_at, c.updated_at,
		       u.first_name || ' ' || u.last_name as author_name, u.email as author_email
		FROM comments c
		LEFT JOIN users u ON c.author_id = u.id
		WHERE c.item_id = ?
		ORDER BY c.created_at DESC
	`, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []models.Comment
	for rows.Next() {
		var c models.Comment
		rows.Scan(&c.ID, &c.ItemID, &c.AuthorID, &c.Content, &c.CreatedAt, &c.UpdatedAt,
			&c.AuthorName, &c.AuthorEmail)
		comments = append(comments, c)
	}
	return comments, nil
}

func (h *ItemHandler) createComment(itemID, userID int, content string) (*models.Comment, error) {
	result, err := h.db.ExecWrite(`
		INSERT INTO comments (item_id, author_id, content)
		VALUES (?, ?, ?)
	`, itemID, userID, content)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	comment := &models.Comment{
		ID:       int(id),
		ItemID:   itemID,
		AuthorID: &userID,
		Content:  content,
	}
	return comment, nil
}

func (h *ItemHandler) getItemHistory(itemID int) ([]models.ItemHistory, error) {
	rows, err := h.db.Query(`
		SELECT h.id, h.item_id, h.user_id, h.changed_at, h.field_name, h.old_value, h.new_value,
		       u.first_name || ' ' || u.last_name as user_name, u.email as user_email
		FROM item_history h
		LEFT JOIN users u ON h.user_id = u.id
		WHERE h.item_id = ?
		ORDER BY h.changed_at DESC
	`, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []models.ItemHistory
	for rows.Next() {
		var h models.ItemHistory
		rows.Scan(&h.ID, &h.ItemID, &h.UserID, &h.ChangedAt, &h.FieldName, &h.OldValue, &h.NewValue,
			&h.UserName, &h.UserEmail)
		history = append(history, h)
	}
	return history, nil
}

func (h *ItemHandler) getItemAttachments(itemID int) ([]models.Attachment, error) {
	rows, err := h.db.Query(`
		SELECT a.id, a.item_id, a.filename, a.original_filename, a.mime_type, a.file_size,
		       a.has_thumbnail, a.uploaded_by, a.created_at,
		       u.first_name || ' ' || u.last_name as uploader_name, u.email as uploader_email
		FROM attachments a
		LEFT JOIN users u ON a.uploaded_by = u.id
		WHERE a.item_id = ?
		ORDER BY a.created_at DESC
	`, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attachments []models.Attachment
	for rows.Next() {
		var a models.Attachment
		var uploaderID sql.NullInt64
		rows.Scan(&a.ID, &a.ItemID, &a.Filename, &a.OriginalFilename, &a.MimeType, &a.FileSize,
			&a.HasThumbnail, &uploaderID, &a.CreatedAt, &a.UploaderName, &a.UploaderEmail)
		if uploaderID.Valid {
			id := int(uploaderID.Int64)
			a.UploadedBy = &id
		}
		attachments = append(attachments, a)
	}
	return attachments, nil
}

func (h *ItemHandler) getItemTransitions(item *models.Item) ([]models.WorkflowTransition, error) {
	if item.StatusID == nil {
		return []models.WorkflowTransition{}, nil
	}

	// Get workflow for this workspace/item
	rows, err := h.db.Query(`
		SELECT wt.id, wt.from_status_id, wt.to_status_id,
		       fs.name as from_status_name, ts.name as to_status_name
		FROM workflow_transitions wt
		LEFT JOIN statuses fs ON wt.from_status_id = fs.id
		JOIN statuses ts ON wt.to_status_id = ts.id
		WHERE wt.from_status_id = ? OR wt.from_status_id IS NULL
		ORDER BY wt.display_order
	`, *item.StatusID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transitions []models.WorkflowTransition
	for rows.Next() {
		var t models.WorkflowTransition
		var fromStatusID sql.NullInt64
		var fromStatusName sql.NullString
		rows.Scan(&t.ID, &fromStatusID, &t.ToStatusID, &fromStatusName, &t.ToStatusName)
		if fromStatusID.Valid {
			id := int(fromStatusID.Int64)
			t.FromStatusID = &id
		}
		if fromStatusName.Valid {
			t.FromStatusName = fromStatusName.String
		}
		transitions = append(transitions, t)
	}
	return transitions, nil
}

func (h *ItemHandler) getItemChildren(parentID int) ([]models.Item, error) {
	rows, err := h.db.Query(`
		SELECT i.id, i.workspace_id, i.workspace_item_number, i.item_type_id, i.title, i.description,
		       i.status_id, i.priority_id, i.due_date, i.is_task, i.milestone_id, i.iteration_id,
		       i.project_id, i.assignee_id, i.creator_id, i.custom_field_values, i.parent_id,
		       i.created_at, i.updated_at,
		       w.name as workspace_name, w.key as workspace_key,
		       it.name as item_type_name,
		       st.name as status_name,
		       pri.name as priority_name, pri.icon as priority_icon, pri.color as priority_color,
		       COALESCE(assignee.first_name || ' ' || assignee.last_name, '') as assignee_name,
		       COALESCE(assignee.email, '') as assignee_email,
		       COALESCE(assignee.avatar_url, '') as assignee_avatar,
		       COALESCE(creator.first_name || ' ' || creator.last_name, '') as creator_name,
		       COALESCE(creator.email, '') as creator_email,
		       m.name as milestone_name, iter.name as iteration_name, proj.name as project_name
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN item_types it ON i.item_type_id = it.id
		LEFT JOIN statuses st ON i.status_id = st.id
		LEFT JOIN priorities pri ON i.priority_id = pri.id
		LEFT JOIN users assignee ON i.assignee_id = assignee.id
		LEFT JOIN users creator ON i.creator_id = creator.id
		LEFT JOIN milestones m ON i.milestone_id = m.id
		LEFT JOIN iterations iter ON i.iteration_id = iter.id
		LEFT JOIN time_projects proj ON i.project_id = proj.id
		WHERE i.parent_id = ?
		ORDER BY i.frac_index, i.created_at
	`, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items, _, err := scanItems(rows)
	return items, err
}

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
