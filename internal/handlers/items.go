package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"windshift/internal/cql"
	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"
	"windshift/internal/validation"
	"windshift/internal/webhook"
)

type ItemHandler struct {
	db                database.Database
	hierarchyService  *services.HierarchyService
	permissionService *services.PermissionService
	itemCache         *services.ItemCacheService
	activityTracker   *services.ActivityTracker
	idResolver        *services.IDResolverService
	mentionService    *services.MentionService // Mention service for processing @mentions (optional, can be nil)
	notificationService interface {
		EmitEvent(event *services.NotificationEvent)
	} // Notification service for async notification processing (optional, can be nil)
	actionService interface {
		EmitActionEvent(event *models.ActionEvent)
	} // Action service for automation workflows (optional, can be nil)
	webhookSender *webhook.WebhookSender // Webhook sender for dispatching webhook events (optional, can be nil)
}

func NewItemHandler(db database.Database, permissionService *services.PermissionService, activityTracker *services.ActivityTracker, notificationService interface {
	EmitEvent(event *services.NotificationEvent)
}) *ItemHandler {
	// Initialize item cache service
	itemCache, err := services.NewItemCacheService(db, services.DefaultItemCacheConfig())
	if err != nil {
		slog.Warn("failed to initialize item cache, continuing without cache", slog.Any("error", err))
		// Continue without cache, will fall back to direct queries
		itemCache = nil
	}

	return &ItemHandler{
		db:                  db,
		hierarchyService:    services.NewHierarchyService(db),
		permissionService:   permissionService,
		itemCache:           itemCache,
		activityTracker:     activityTracker,
		idResolver:          services.NewIDResolverService(db),
		notificationService: notificationService,
	}
}

// SetWebhookSender sets the webhook sender for dispatching webhook events
func (h *ItemHandler) SetWebhookSender(sender *webhook.WebhookSender) {
	h.webhookSender = sender
}

// SetMentionService sets the mention service for processing @mentions
func (h *ItemHandler) SetMentionService(mentionService *services.MentionService) {
	h.mentionService = mentionService
}

// SetActionService sets the action service for automation workflows
func (h *ItemHandler) SetActionService(actionService interface {
	EmitActionEvent(event *models.ActionEvent)
}) {
	h.actionService = actionService
}

func (h *ItemHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	// Get user from context
	user := h.getUserFromContext(r)
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	// Get accessible workspace IDs (includes active workspaces and inactive ones where user has admin access)
	accessibleWorkspaceIDs, err := h.getAccessibleWorkspaceIDs(user)
	if err != nil {
		http.Error(w, "Failed to get accessible workspaces: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// If user has no accessible workspaces, return empty list
	if len(accessibleWorkspaceIDs) == 0 {
		respondJSONOK(w, map[string]interface{}{
			"items":       []models.Item{},
			"total_count": 0,
			"page":        1,
			"limit":       50,
		})
		return
	}

	// Build query components separately for reuse
	// No CTE needed for GetAll - effective_project is only calculated on detailed GET
	selectClause := `SELECT
			i.id, i.workspace_id, i.workspace_item_number, i.item_type_id, i.title, i.description, i.status_id, i.priority_id, i.due_date, i.is_task,
		    i.milestone_id, i.iteration_id, i.project_id, i.inherit_project, i.time_project_id, i.assignee_id, i.creator_id, i.custom_field_values, i.calendar_data, i.parent_id,
		    i.frac_index, i.created_at, i.updated_at,
		    w.name as workspace_name, w.key as workspace_key, it.name as item_type_name,
		    p.title as parent_title, m.name as milestone_name, iter.name as iteration_name, proj.name as project_name, tp.name as time_project_name,
		    assignee.first_name || ' ' || assignee.last_name as assignee_name, assignee.email as assignee_email, assignee.avatar_url as assignee_avatar,
		    creator.first_name || ' ' || creator.last_name as creator_name, creator.email as creator_email,
		    st.name as status_name, pri.name as priority_name, pri.icon as priority_icon, pri.color as priority_color
		`

	fromClause := `FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN item_types it ON i.item_type_id = it.id
		LEFT JOIN items p ON i.parent_id = p.id
		LEFT JOIN milestones m ON i.milestone_id = m.id
		LEFT JOIN iterations iter ON i.iteration_id = iter.id
		LEFT JOIN time_projects proj ON i.project_id = proj.id
		LEFT JOIN time_projects tp ON i.time_project_id = tp.id
		LEFT JOIN users assignee ON i.assignee_id = assignee.id
		LEFT JOIN users creator ON i.creator_id = creator.id
		LEFT JOIN statuses st ON i.status_id = st.id
		LEFT JOIN priorities pri ON i.priority_id = pri.id
		`

	whereClause := "WHERE 1=1"
	args := []interface{}{}

	// Filter by accessible workspaces (respects workspace active status)
	if len(accessibleWorkspaceIDs) > 0 {
		placeholders := make([]string, len(accessibleWorkspaceIDs))
		for i, id := range accessibleWorkspaceIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		whereClause += " AND i.workspace_id IN (" + strings.Join(placeholders, ",") + ")"
	}

	// Check for QL query parameter
	if qlQuery := r.URL.Query().Get("ql"); qlQuery != "" {
		// Build workspace mapping for QL evaluation
		workspaceMap, err := h.buildWorkspaceMap()
		if err != nil {
			http.Error(w, "Failed to load workspace mapping: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Create QL evaluator and generate SQL
		evaluator := cql.NewEvaluator(workspaceMap)
		qlSQL, qlArgs, err := evaluator.EvaluateToSQL(qlQuery)
		if err != nil {
			http.Error(w, "QL query error: "+err.Error(), http.StatusBadRequest)
			return
		}

		if qlSQL != "" {
			whereClause += " AND (" + qlSQL + ")"
			args = append(args, qlArgs...)
		}
	} else {
		// Add filters based on query parameters (fallback for non-QL queries)
		// Note: workspace_id permission check already done at the start of the function
		if workspaceID := r.URL.Query().Get("workspace_id"); workspaceID != "" {
			whereClause += " AND i.workspace_id = ?"
			args = append(args, workspaceID)
		}

		if status := r.URL.Query().Get("status"); status != "" {
			whereClause += " AND i.status_id = ?"
			args = append(args, status)
		}

		if priorityID := r.URL.Query().Get("priority_id"); priorityID != "" {
			whereClause += " AND i.priority_id = ?"
			args = append(args, priorityID)
		}

		if assigneeID := r.URL.Query().Get("assignee_id"); assigneeID != "" {
			whereClause += " AND i.assignee_id = ?"
			args = append(args, assigneeID)
		}

		// Hierarchy filters
		if parentID := r.URL.Query().Get("parent_id"); parentID != "" {
			if parentID == "null" || parentID == "0" {
				whereClause += " AND i.parent_id IS NULL"
			} else {
				whereClause += " AND i.parent_id = ?"
				args = append(args, parentID)
			}
		}

		if level := r.URL.Query().Get("level"); level != "" {
			levelInt, err := strconv.Atoi(level)
			if err != nil {
				http.Error(w, "Invalid level parameter: must be an integer", http.StatusBadRequest)
				return
			}
			whereClause += " AND COALESCE(it.hierarchy_level, 0) = ?"
			args = append(args, levelInt)
		}

		if maxLevel := r.URL.Query().Get("max_level"); maxLevel != "" {
			maxLevelInt, err := strconv.Atoi(maxLevel)
			if err != nil {
				http.Error(w, "Invalid max_level parameter: must be an integer", http.StatusBadRequest)
				return
			}
			whereClause += " AND COALESCE(it.hierarchy_level, 0) <= ?"
			args = append(args, maxLevelInt)
		}

		// Date filters
		if createdSince := r.URL.Query().Get("created_since"); createdSince != "" {
			whereClause += " AND i.created_at >= ?"
			args = append(args, createdSince)
		}
	}

	// ID filter (applies to both QL and non-QL queries)
	if id := r.URL.Query().Get("id"); id != "" {
		whereClause += " AND i.id = ?"
		args = append(args, id)
	}

	// Add ordering - support multiple ordering strategies
	orderBy := r.URL.Query().Get("order_by")
	var orderByClause string

	if orderBy == "created_at" {
		// Sort by creation time only
		orderByClause = ` ORDER BY
			i.created_at DESC`
	} else {
		// Default: prioritize frac_index over creation time
		orderByClause = ` ORDER BY
			CASE WHEN i.frac_index IS NULL THEN 1 ELSE 0 END,
			i.frac_index ASC,
			i.created_at DESC`
	}

	// Parse pagination parameters
	page := 1
	limit := 50      // Default items per page
	maxLimit := 1000 // Maximum items that can be returned from API

	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
			if limit > maxLimit {
				limit = maxLimit
			}
		}
	}

	// Build count query (no ORDER BY needed for count)
	countQuery := "SELECT COUNT(DISTINCT i.id) " + fromClause + whereClause

	var totalCount int
	err = h.db.QueryRow(countQuery, args...).Scan(&totalCount)
	if err != nil {
		http.Error(w, "Failed to get total count: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Build data query with ordering and pagination
	offset := (page - 1) * limit
	dataQuery := selectClause + fromClause + whereClause + orderByClause + fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)

	rows, err := h.db.Query(dataQuery, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var items []models.Item
	for rows.Next() {
		var item models.Item
		var customFieldValuesJSON, calendarDataJSON sql.NullString
		var itemTypeID, parentID, milestoneID, iterationID, projectID, timeProjectID, assigneeID, creatorID, statusID, priorityID sql.NullInt64
		var dueDate sql.NullTime
		var itemTypeName, parentTitle, milestoneName, iterationName, projectName, timeProjectName sql.NullString
		var assigneeName, assigneeEmail, assigneeAvatar, creatorName, creatorEmail, statusName sql.NullString
		var priorityName, priorityIcon, priorityColor sql.NullString
		var fracIndex sql.NullString
		var inheritProject bool

		err := rows.Scan(&item.ID, &item.WorkspaceID, &item.WorkspaceItemNumber, &itemTypeID, &item.Title, &item.Description,
			&statusID, &priorityID, &dueDate, &item.IsTask, &milestoneID, &iterationID, &projectID, &inheritProject, &timeProjectID, &assigneeID, &creatorID, &customFieldValuesJSON, &calendarDataJSON, &parentID,
			&fracIndex, &item.CreatedAt, &item.UpdatedAt, &item.WorkspaceName, &item.WorkspaceKey, &itemTypeName, &parentTitle, &milestoneName, &iterationName, &projectName, &timeProjectName,
			&assigneeName, &assigneeEmail, &assigneeAvatar, &creatorName, &creatorEmail, &statusName, &priorityName, &priorityIcon, &priorityColor)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Handle nullable fields
		item.ItemTypeID = utils.NullInt64ToPtr(itemTypeID)
		item.ParentID = utils.NullInt64ToPtr(parentID)
		item.DueDate = utils.NullTimeToPtr(dueDate)
		item.MilestoneID = utils.NullInt64ToPtr(milestoneID)
		item.ItemTypeName = itemTypeName.String
		item.ParentTitle = parentTitle.String
		item.MilestoneName = milestoneName.String
		item.IterationID = utils.NullInt64ToPtr(iterationID)
		item.IterationName = iterationName.String
		item.StatusID = utils.NullInt64ToPtr(statusID)
		item.StatusName = statusName.String
		item.ProjectID = utils.NullInt64ToPtr(projectID)
		item.InheritProject = inheritProject
		item.ProjectName = projectName.String
		item.TimeProjectID = utils.NullInt64ToPtr(timeProjectID)
		item.TimeProjectName = timeProjectName.String
		item.FracIndex = utils.NullStringToPtr(fracIndex)
		item.PriorityID = utils.NullInt64ToPtr(priorityID)
		item.PriorityName = priorityName.String
		item.PriorityIcon = priorityIcon.String
		item.PriorityColor = priorityColor.String
		item.AssigneeID = utils.NullInt64ToPtr(assigneeID)
		item.CreatorID = utils.NullInt64ToPtr(creatorID)
		item.AssigneeName = assigneeName.String
		item.AssigneeEmail = assigneeEmail.String
		item.AssigneeAvatar = assigneeAvatar.String
		item.CreatorName = creatorName.String
		item.CreatorEmail = creatorEmail.String
		// Note: effective_project fields are NOT calculated on list operations for performance

		// Parse custom field values JSON
		if customFieldValuesJSON.Valid && customFieldValuesJSON.String != "" {
			if err := json.Unmarshal([]byte(customFieldValuesJSON.String), &item.CustomFieldValues); err != nil {
				item.CustomFieldValues = make(map[string]interface{})
			}
		} else {
			item.CustomFieldValues = make(map[string]interface{})
		}

		// Parse calendar data JSON
		if calendarDataJSON.Valid && calendarDataJSON.String != "" {
			if err := json.Unmarshal([]byte(calendarDataJSON.String), &item.CalendarData); err != nil {
				item.CalendarData = []models.CalendarScheduleEntry{}
			}
		} else {
			item.CalendarData = []models.CalendarScheduleEntry{}
		}

		items = append(items, item)
	}

	// Always return an array, even if empty
	if items == nil {
		items = []models.Item{}
	}

	// Filter items based on user permissions
	filteredItems, err := h.filterItemsByPermissions(user.ID, items)
	if err != nil {
		slog.Error("permission check failed", slog.Int("user_id", user.ID), slog.String("operation", "GetAll"), slog.Any("error", err))
		http.Error(w, "Permission check failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	items = filteredItems

	// Create paginated response
	response := models.PaginatedItemsResponse{
		Items: items,
		Pagination: models.PaginationMeta{
			Page:       page,
			Limit:      limit,
			Total:      totalCount,
			TotalPages: (totalCount + limit - 1) / limit,
		},
	}

	respondJSONOK(w, response)
}

func (h *ItemHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Require authentication
	user := h.getUserFromContext(r)
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	// Get item with all details using service
	crudService := services.NewItemCRUDService(h.db)
	result, err := crudService.GetByIDWithWorkspaceStatus(id)
	if err != nil {
		if err == repository.ErrNotFound {
			http.Error(w, "Item not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	item := result.Item

	// Check if user has permission to view this item
	canView, err := h.canViewItem(user.ID, item.WorkspaceID)
	if err != nil {
		http.Error(w, "Permission check failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if !canView {
		http.Error(w, "Insufficient permissions to view this item", http.StatusForbidden)
		return
	}

	// Check if workspace is inactive and user has permission to access it
	if !result.WorkspaceActive {
		canAccess, err := h.canAccessInactiveWorkspace(user, item.WorkspaceID)
		if err != nil {
			http.Error(w, "Error checking workspace access: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if !canAccess {
			http.Error(w, "This workspace is inactive and you don't have permission to access it", http.StatusForbidden)
			return
		}
	}

	// Get effective project from cache
	if h.itemCache != nil {
		effectiveProjectID, projectInheritanceMode, err := h.itemCache.GetEffectiveProjectForItem(id, item.WorkspaceID)
		if err == nil && effectiveProjectID != nil {
			item.EffectiveProjectID = effectiveProjectID
			item.ProjectInheritanceMode = projectInheritanceMode
			var epName sql.NullString
			h.db.QueryRow("SELECT name FROM time_projects WHERE id = ?", *effectiveProjectID).Scan(&epName)
			item.EffectiveProjectName = epName.String
		}
	}

	// Track item view activity
	if h.activityTracker != nil {
		if err := h.activityTracker.TrackItemActivity(user.ID, item.ID, services.ActivityView); err != nil {
			slog.Warn("failed to track item view activity", slog.Int("user_id", user.ID), slog.Int("item_id", item.ID), slog.Any("error", err))
		}
	}

	respondJSONOK(w, item)
}

func (h *ItemHandler) Create(w http.ResponseWriter, r *http.Request) {
	slog.Debug("item create request received")
	// Performance profiling
	createStart := time.Now()

	var item models.Item
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	slog.Debug("item decoded", slog.Int("workspace_id", item.WorkspaceID), slog.String("title", item.Title))

	// Require authentication
	user := h.getUserFromContext(r)
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}
	slog.Debug("user authenticated", slog.String("username", user.Username))

	// Set creator to the authenticated user
	item.CreatorID = &user.ID

	// Check if user has permission to create items in this workspace
	slog.Debug("checking permissions")
	canEdit, err := h.canEditItem(user.ID, item.WorkspaceID)
	if err != nil {
		http.Error(w, "Permission check failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	slog.Debug("permission check complete", slog.Bool("can_edit", canEdit))
	if !canEdit {
		http.Error(w, "Insufficient permissions to create items in this workspace", http.StatusForbidden)
		return
	}

	// Sanitize user input to prevent XSS
	item.Title = utils.SanitizeTitle(item.Title)
	item.Description = utils.SanitizeDescription(item.Description)

	// Default to status_id = 1 (Open) if not provided
	if item.StatusID == nil {
		defaultStatusID := 1
		item.StatusID = &defaultStatusID
	}

	// Default to the default priority if not provided
	if item.PriorityID == nil {
		var defaultPriorityID int
		err := h.db.QueryRow("SELECT id FROM priorities WHERE is_default = true LIMIT 1").Scan(&defaultPriorityID)
		if err == nil {
			item.PriorityID = &defaultPriorityID
		}
		// If no default priority exists, leave as NULL (graceful fallback)
	}

	// Validate required fields
	if strings.TrimSpace(item.Title) == "" {
		http.Error(w, "Title is required", http.StatusBadRequest)
		return
	}

	// Validate workspace exists
	slog.Debug("validating workspace exists", slog.Int("workspace_id", item.WorkspaceID))
	var workspaceExists bool
	err = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM workspaces WHERE id = ?)", item.WorkspaceID).Scan(&workspaceExists)
	if err != nil {
		slog.Error("workspace validation query failed", slog.Int("workspace_id", item.WorkspaceID), slog.Any("error", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	slog.Debug("workspace validation complete", slog.Bool("exists", workspaceExists))
	if !workspaceExists {
		http.Error(w, "Workspace not found", http.StatusBadRequest)
		return
	}

	// Task-specific validation and defaults
	if item.IsTask {
		// Tasks have simplified status - only status_id 1 (Open) or 5 (Done) allowed
		if item.StatusID != nil && *item.StatusID != 1 && *item.StatusID != 5 {
			http.Error(w, "Tasks can only have status 'Open' (1) or 'Done' (5)", http.StatusBadRequest)
			return
		}
		// Ensure status_id is set (should be from above, but double-check)
		if item.StatusID == nil {
			defaultStatusID := 1 // Open
			item.StatusID = &defaultStatusID
		}
	}

	// Set default project inheritance based on parent relationship
	if item.ProjectID == nil && !item.InheritProject {
		if item.ParentID != nil && *item.ParentID != 0 {
			// Has parent: default to inherit
			item.InheritProject = true
		}
		// If no parent: leave as NULL (none) and InheritProject = false
	}

	// Validate parent item if specified
	// Handle parent-child relationship and hierarchy validation
	if item.ParentID != nil && *item.ParentID != 0 {
		var parentItemTypeID sql.NullInt64
		var parentItemTypeHierarchyLevel int
		err := h.db.QueryRow(`
			SELECT i.item_type_id, COALESCE(it.hierarchy_level, 0)
			FROM items i
			LEFT JOIN item_types it ON i.item_type_id = it.id
			WHERE i.id = ?
		`, *item.ParentID).Scan(&parentItemTypeID, &parentItemTypeHierarchyLevel)

		if err == sql.ErrNoRows {
			http.Error(w, "Parent item not found", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Validate hierarchy relationship if item type is specified
		if item.ItemTypeID != nil && *item.ItemTypeID != 0 {
			var itemTypeHierarchyLevel int
			var itemTypeName string
			err := h.db.QueryRow(`
				SELECT hierarchy_level, name FROM item_types 
				WHERE id = ?
			`, *item.ItemTypeID).Scan(&itemTypeHierarchyLevel, &itemTypeName)

			if err == sql.ErrNoRows {
				http.Error(w, "Item type not found", http.StatusBadRequest)
				return
			}
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			// Check if child hierarchy level is exactly one more than parent
			if itemTypeHierarchyLevel != parentItemTypeHierarchyLevel+1 {
				http.Error(w, fmt.Sprintf("Item type '%s' (hierarchy level %d) cannot be a child of an item at hierarchy level %d",
					itemTypeName, itemTypeHierarchyLevel, parentItemTypeHierarchyLevel), http.StatusBadRequest)
				return
			}
		}
	} else {
		// Root level item - no hierarchy level restrictions when no parent is specified
		item.ParentID = nil
	}

	// Validate related_work_item_id if provided
	if item.RelatedWorkItemID != nil {
		// Verify workspace is personal and belongs to the user
		var isPersonal bool
		var ownerID *int
		err := h.db.QueryRow(`
			SELECT is_personal, owner_id FROM workspaces WHERE id = ?
		`, item.WorkspaceID).Scan(&isPersonal, &ownerID)

		if err != nil {
			http.Error(w, "Failed to validate workspace: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if !isPersonal || ownerID == nil || *ownerID != user.ID {
			http.Error(w, "Personal tasks must be created in your own personal workspace", http.StatusBadRequest)
			return
		}

		// Verify the related work item exists
		var relatedWorkspaceID int
		err = h.db.QueryRow("SELECT workspace_id FROM items WHERE id = ?", *item.RelatedWorkItemID).Scan(&relatedWorkspaceID)
		if err != nil {
			http.Error(w, "Related work item not found or access denied", http.StatusForbidden)
			return
		}
	}

	// Profiling: validation complete
	validationTime := time.Since(createStart)

	// Generate fractional index for manual ordering
	fracIndexStart := time.Now()
	fracIndex, err := services.GenerateFracIndexForNewItem(h.db.GetDB(), item.WorkspaceID, item.ParentID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to generate frac_index: %v", err), http.StatusInternalServerError)
		return
	}
	fracIndexTime := time.Since(fracIndexStart)

	// Convert custom field values to JSON
	var customFieldValuesJSON string
	if item.CustomFieldValues != nil {
		customFieldValuesBytes, err := json.Marshal(item.CustomFieldValues)
		if err != nil {
			http.Error(w, "Invalid custom field values", http.StatusBadRequest)
			return
		}
		customFieldValuesJSON = string(customFieldValuesBytes)
	}

	now := time.Now()

	// Insert item with workspace-specific item number
	// PostgreSQL: inline nextval() in INSERT - no separate round-trip, avoids connection pool contention
	// SQLite: get next number via MAX+1 BEFORE starting transaction to avoid deadlock
	var id int64
	var nextWorkspaceItemNumber int

	// For SQLite, get the next workspace item number BEFORE starting the transaction
	// This avoids deadlock between the transaction lock and writeConn lock on Linux/ZFS
	if h.db.GetDriverName() != "postgres" {
		slog.Debug("getting next workspace item number")
		var err error
		nextWorkspaceItemNumber, err = h.db.NextWorkspaceItemNumber(int64(item.WorkspaceID))
		if err != nil {
			slog.Error("failed to generate workspace item number", slog.Int("workspace_id", item.WorkspaceID), slog.Any("error", err))
			http.Error(w, fmt.Sprintf("Failed to generate workspace item number: %v", err), http.StatusInternalServerError)
			return
		}
		slog.Debug("got workspace item number", slog.Int("workspace_item_number", nextWorkspaceItemNumber))
	}

	// Start transaction for hierarchy updates
	slog.Debug("beginning transaction")
	txStart := time.Now()
	tx, err := h.db.Begin()
	if err != nil {
		slog.Error("failed to begin transaction", slog.Any("error", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	beginTime := time.Since(txStart)
	slog.Debug("transaction started", slog.Duration("elapsed", beginTime))

	insertStart := time.Now()
	if h.db.GetDriverName() == "postgres" {
		// PostgreSQL: inline nextval() directly in INSERT - sequence name built in Go to avoid type issues
		seqName := fmt.Sprintf("workspace_%d_item_seq", item.WorkspaceID)
		err = tx.QueryRow(`
			INSERT INTO items (workspace_id, workspace_item_number, item_type_id, title, description, status_id, priority_id, due_date, is_task,
			                  milestone_id, iteration_id, project_id, inherit_project, assignee_id, creator_id, custom_field_values, parent_id,
			                  frac_index, related_work_item_id, created_at, updated_at)
			VALUES ($1, nextval($2), $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
			RETURNING id, workspace_item_number
		`, item.WorkspaceID, seqName, item.ItemTypeID, item.Title, item.Description, item.StatusID, item.PriorityID, item.DueDate, item.IsTask,
			item.MilestoneID, item.IterationID, item.ProjectID, item.InheritProject, item.AssigneeID, item.CreatorID, customFieldValuesJSON, item.ParentID,
			fracIndex, item.RelatedWorkItemID, now, now).Scan(&id, &nextWorkspaceItemNumber)
	} else {
		// SQLite: use the workspace item number we got earlier
		slog.Debug("executing INSERT", slog.Int("workspace_item_number", nextWorkspaceItemNumber))
		err = tx.QueryRow(`
			INSERT INTO items (workspace_id, workspace_item_number, item_type_id, title, description, status_id, priority_id, due_date, is_task,
			                  milestone_id, iteration_id, project_id, inherit_project, assignee_id, creator_id, custom_field_values, parent_id,
			                  frac_index, related_work_item_id, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
		`, item.WorkspaceID, nextWorkspaceItemNumber, item.ItemTypeID, item.Title, item.Description, item.StatusID, item.PriorityID, item.DueDate, item.IsTask,
			item.MilestoneID, item.IterationID, item.ProjectID, item.InheritProject, item.AssigneeID, item.CreatorID, customFieldValuesJSON, item.ParentID,
			fracIndex, item.RelatedWorkItemID, now, now).Scan(&id)
	}

	if err != nil {
		slog.Error("failed to insert item", slog.Any("error", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	insertTime := time.Since(insertStart)

	// Commit transaction
	commitStart := time.Now()
	if err = tx.Commit(); err != nil {
		slog.Error("failed to commit transaction", slog.Any("error", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	commitTime := time.Since(commitStart)
	txTime := time.Since(txStart) // Time for TX operations (Begin -> Commit)

	// Record item creation history
	updateService := services.NewItemUpdateService(h.db)
	if err := updateService.RecordItemCreationHistory(h.db, int(id), user.ID); err != nil {
		slog.Warn("failed to record item creation history", slog.Int64("item_id", id), slog.Any("error", err))
		// Don't fail the request, just log the error
	}

	// Profiling: post-insert query
	postQueryStart := time.Now()

	// Return the created item with basic data (NO effective project calculation on writes)
	var createdItem models.Item
	var returnCustomFieldValuesJSON sql.NullString
	var itemTypeID, parentID, statusID, returnMilestoneID, returnProjectID sql.NullInt64
	var itemTypeName, parentTitle, returnMilestoneName, returnProjectName sql.NullString
	var returnInheritProject bool

	var createdFracIndex sql.NullString

	// Simple query without CTE - much faster!
	var returnPriorityID sql.NullInt64
	var returnPriorityName, returnPriorityIcon, returnPriorityColor sql.NullString
	var returnDueDate sql.NullTime
	err = h.db.QueryRow(`
		SELECT i.id, i.workspace_id, i.workspace_item_number, i.item_type_id, i.title, i.description, i.status_id, i.priority_id, i.due_date, i.is_task,
		       i.milestone_id, i.project_id, i.inherit_project, i.custom_field_values, i.parent_id,
		       i.frac_index, i.created_at, i.updated_at,
		       w.name as workspace_name, w.key as workspace_key, it.name as item_type_name, p.title as parent_title, m.name as milestone_name, proj.name as project_name,
		       pri.name as priority_name, pri.icon as priority_icon, pri.color as priority_color
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN item_types it ON i.item_type_id = it.id
		LEFT JOIN items p ON i.parent_id = p.id
		LEFT JOIN milestones m ON i.milestone_id = m.id
		LEFT JOIN iterations iter ON i.iteration_id = iter.id
		LEFT JOIN time_projects proj ON i.project_id = proj.id
		LEFT JOIN priorities pri ON i.priority_id = pri.id
		WHERE i.id = ?
	`, id).Scan(&createdItem.ID, &createdItem.WorkspaceID, &createdItem.WorkspaceItemNumber, &itemTypeID, &createdItem.Title, &createdItem.Description,
		&statusID, &returnPriorityID, &returnDueDate, &createdItem.IsTask, &returnMilestoneID, &returnProjectID, &returnInheritProject, &returnCustomFieldValuesJSON, &parentID,
		&createdFracIndex, &createdItem.CreatedAt, &createdItem.UpdatedAt, &createdItem.WorkspaceName, &createdItem.WorkspaceKey, &itemTypeName, &parentTitle, &returnMilestoneName, &returnProjectName,
		&returnPriorityName, &returnPriorityIcon, &returnPriorityColor)
	selectQueryTime := time.Since(postQueryStart)

	// Handle nullable fields
	createdItem.ItemTypeID = utils.NullInt64ToPtr(itemTypeID)
	createdItem.ParentID = utils.NullInt64ToPtr(parentID)
	createdItem.MilestoneID = utils.NullInt64ToPtr(returnMilestoneID)
	createdItem.StatusID = utils.NullInt64ToPtr(statusID)
	createdItem.ProjectID = utils.NullInt64ToPtr(returnProjectID)
	createdItem.ItemTypeName = itemTypeName.String
	createdItem.ParentTitle = parentTitle.String
	createdItem.MilestoneName = returnMilestoneName.String
	createdItem.ProjectName = returnProjectName.String
	createdItem.FracIndex = utils.NullStringToPtr(createdFracIndex)
	createdItem.PriorityID = utils.NullInt64ToPtr(returnPriorityID)
	createdItem.PriorityName = returnPriorityName.String
	createdItem.PriorityIcon = returnPriorityIcon.String
	createdItem.PriorityColor = returnPriorityColor.String
	createdItem.DueDate = utils.NullTimeToPtr(returnDueDate)

	// Handle inherit_project field
	createdItem.InheritProject = returnInheritProject

	// Note: effective_project fields are NOT calculated on writes for performance
	// Clients should use GET /api/items/{id} if they need effective project data

	if err != nil {
		slog.Error("failed to query created item", slog.Int64("item_id", id), slog.Any("error", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Parse custom field values JSON
	if returnCustomFieldValuesJSON.Valid && returnCustomFieldValuesJSON.String != "" {
		if err := json.Unmarshal([]byte(returnCustomFieldValuesJSON.String), &createdItem.CustomFieldValues); err != nil {
			createdItem.CustomFieldValues = make(map[string]interface{})
		}
	} else {
		createdItem.CustomFieldValues = make(map[string]interface{})
	}

	// Emit notification event
	notifyStart := time.Now()
	if h.notificationService != nil {
		// Construct the item key (e.g., "TST-1")
		itemKey := fmt.Sprintf("%s-%d", createdItem.WorkspaceKey, createdItem.WorkspaceItemNumber)

		h.notificationService.EmitEvent(&services.NotificationEvent{
			EventType:   models.EventItemCreated,
			WorkspaceID: createdItem.WorkspaceID,
			ActorUserID: user.ID,
			ItemID:      createdItem.ID,
			AssigneeID:  createdItem.AssigneeID,
			CreatorID:   &user.ID,
			Title:       "New Item Created",
			TemplateData: map[string]interface{}{
				"item.title":     createdItem.Title,
				"item.key":       itemKey,
				"item.id":        createdItem.ID,
				"user.name":      user.Username,
				"workspace.name": createdItem.WorkspaceName,
				"workspace.key":  createdItem.WorkspaceKey,
			},
		})
	}

	// Emit action event for automation
	if h.actionService != nil {
		h.actionService.EmitActionEvent(&models.ActionEvent{
			EventType:   models.ActionTriggerItemCreated,
			WorkspaceID: createdItem.WorkspaceID,
			ItemID:      createdItem.ID,
			ActorUserID: user.ID,
			NewValues: map[string]interface{}{
				"title":       createdItem.Title,
				"status_id":   createdItem.StatusID,
				"item_type_id": createdItem.ItemTypeID,
				"assignee_id": createdItem.AssigneeID,
				"creator_id":  createdItem.CreatorID,
				"priority_id": createdItem.PriorityID,
			},
		})
	}
	notifyTime := time.Since(notifyStart)

	// Dispatch webhook event for item creation
	if h.webhookSender != nil {
		go h.webhookSender.DispatchEvent("item.created", &createdItem)
	}

	// Profiling: log timing summary (all times in milliseconds for easy parsing)
	totalTime := time.Since(createStart)
	measuredTime := validationTime + fracIndexTime + txTime + selectQueryTime + notifyTime
	gapTime := totalTime - measuredTime // Time spent in scheduler/unmeasured code
	slog.Debug("item creation performance",
		slog.Int("item_id", createdItem.ID),
		slog.Group("timings_ms",
			slog.Float64("validation", float64(validationTime.Microseconds())/1000.0),
			slog.Float64("frac_index", float64(fracIndexTime.Microseconds())/1000.0),
			slog.Float64("transaction", float64(txTime.Microseconds())/1000.0),
			slog.Float64("tx_begin", float64(beginTime.Microseconds())/1000.0),
			slog.Float64("tx_insert", float64(insertTime.Microseconds())/1000.0),
			slog.Float64("tx_commit", float64(commitTime.Microseconds())/1000.0),
			slog.Float64("query", float64(selectQueryTime.Microseconds())/1000.0),
			slog.Float64("notify", float64(notifyTime.Microseconds())/1000.0),
			slog.Float64("gap", float64(gapTime.Microseconds())/1000.0),
			slog.Float64("total", float64(totalTime.Microseconds())/1000.0),
		))

	respondJSONCreated(w, createdItem)
}

func (h *ItemHandler) Update(w http.ResponseWriter, r *http.Request) {
	// Parse request and validate item ID
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Parse update data from request body
	var updateData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updateData); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Require authentication
	user := h.getUserFromContext(r)
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	// Load item to check permissions (we need workspace_id, status_id, and item_type_id for workflow resolution)
	var workspaceID int
	var currentStatusID sql.NullInt64
	var itemTypeID sql.NullInt64
	err := h.db.QueryRow("SELECT workspace_id, status_id, item_type_id FROM items WHERE id = ?", id).Scan(&workspaceID, &currentStatusID, &itemTypeID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Item not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check if user has permission to edit items in this workspace
	canEdit, err := h.canEditItem(user.ID, workspaceID)
	if err != nil {
		http.Error(w, "Permission check failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if !canEdit {
		http.Error(w, "Insufficient permissions to edit items in this workspace", http.StatusForbidden)
		return
	}

	// Validate status transition if status_id is being changed
	if newStatusID, ok := updateData["status_id"]; ok && newStatusID != nil {
		var toStatusID int64
		switch v := newStatusID.(type) {
		case float64:
			toStatusID = int64(v)
		case int64:
			toStatusID = v
		case int:
			toStatusID = int64(v)
		default:
			http.Error(w, "Invalid status_id format", http.StatusBadRequest)
			return
		}

		// Only validate if current status exists and is different from new status
		if currentStatusID.Valid && currentStatusID.Int64 != toStatusID {
			// Use WorkflowService for proper item type workflow resolution
			workflowService := services.NewWorkflowService(h.db)
			itemTypeIDPtr := utils.NullInt64ToPtr(itemTypeID)
			valid, err := workflowService.IsValidStatusTransition(workspaceID, itemTypeIDPtr, currentStatusID.Int64, toStatusID)
			if err != nil {
				http.Error(w, "Failed to validate status transition: "+err.Error(), http.StatusInternalServerError)
				return
			}
			if !valid {
				http.Error(w, "Invalid status transition: this status change is not allowed by the workflow", http.StatusBadRequest)
				return
			}
		}
	}

	// Track item edit activity
	if h.activityTracker != nil {
		if err := h.activityTracker.TrackItemActivity(user.ID, id, services.ActivityEdit); err != nil {
			slog.Warn("failed to track item edit activity", slog.Int("user_id", user.ID), slog.Int("item_id", id), slog.Any("error", err))
			// Don't fail the request, just log the error
		}
	}

	// Call update service to handle all business logic
	updateService := services.NewItemUpdateService(h.db)
	result, err := updateService.UpdateItem(services.UpdateItemRequest{
		ItemID:     id,
		UpdateData: updateData,
		UserID:     user.ID,
	})

	if err != nil {
		// Check if it's a validation error
		if valErr, ok := err.(*validation.ValidationError); ok {
			http.Error(w, valErr.Error(), http.StatusBadRequest)
			return
		}
		// Generic error
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get original and updated items for event emission
	originalItem := result.OriginalItem
	updatedItem := result.Item

	w.Header().Set("Content-Type", "application/json")

	// Check if assignee changed (compare originalItem with updatedItem) - needed for both notifications and webhooks
	assigneeChanged := false
	if originalItem.AssigneeID == nil && updatedItem.AssigneeID != nil {
		assigneeChanged = true
	} else if originalItem.AssigneeID != nil && updatedItem.AssigneeID == nil {
		assigneeChanged = true
	} else if originalItem.AssigneeID != nil && updatedItem.AssigneeID != nil && *originalItem.AssigneeID != *updatedItem.AssigneeID {
		assigneeChanged = true
	}

	// Emit notification events
	if h.notificationService != nil && user != nil {
		slog.Debug("checking for notification events",
			slog.Int("item_id", updatedItem.ID),
			slog.Int("user_id", user.ID),
			slog.String("username", user.Username))

		// Log assignee change details
		if assigneeChanged {
			if originalItem.AssigneeID == nil && updatedItem.AssigneeID != nil {
				slog.Debug("assignee added", slog.Int("new_assignee_id", *updatedItem.AssigneeID))
			} else if originalItem.AssigneeID != nil && updatedItem.AssigneeID == nil {
				slog.Debug("assignee removed", slog.Int("old_assignee_id", *originalItem.AssigneeID))
			} else if originalItem.AssigneeID != nil && updatedItem.AssigneeID != nil {
				slog.Debug("assignee changed", slog.Int("old_assignee_id", *originalItem.AssigneeID), slog.Int("new_assignee_id", *updatedItem.AssigneeID))
			}
		} else {
			// Log why it didn't change
			originalAssignee := "nil"
			updatedAssignee := "nil"
			if originalItem.AssigneeID != nil {
				originalAssignee = fmt.Sprintf("%d", *originalItem.AssigneeID)
			}
			if updatedItem.AssigneeID != nil {
				updatedAssignee = fmt.Sprintf("%d", *updatedItem.AssigneeID)
			}
			slog.Debug("assignee not changed", slog.String("original", originalAssignee), slog.String("updated", updatedAssignee))
		}

		// Get status name if status changed
		var statusName string
		if result.StatusChanged && updatedItem.StatusID != nil {
			h.db.QueryRow("SELECT name FROM statuses WHERE id = ?", *updatedItem.StatusID).Scan(&statusName)
		}

		// Emit status changed notification
		if result.StatusChanged {
			slog.Debug("emitting EventStatusChanged",
				slog.Int("item_id", updatedItem.ID),
				slog.String("status", statusName),
				slog.Any("assignee_id", updatedItem.AssigneeID),
				slog.Any("creator_id", originalItem.CreatorID))

			// Construct the item key (e.g., "TST-1")
			itemKey := fmt.Sprintf("%s-%d", updatedItem.WorkspaceKey, updatedItem.WorkspaceItemNumber)

			h.notificationService.EmitEvent(&services.NotificationEvent{
				EventType:   models.EventStatusChanged,
				WorkspaceID: updatedItem.WorkspaceID,
				ActorUserID: user.ID,
				ItemID:      updatedItem.ID,
				AssigneeID:  updatedItem.AssigneeID,
				CreatorID:   originalItem.CreatorID,
				Title:       "Status Changed",
				TemplateData: map[string]interface{}{
					"item.title":  updatedItem.Title,
					"item.key":    itemKey,
					"item.id":     updatedItem.ID,
					"status.name": statusName,
					"user.name":   user.Username,
				},
			})

			slog.Debug("successfully emitted EventStatusChanged", slog.Int("item_id", updatedItem.ID))
		}

		// Emit assignee changed notification
		if assigneeChanged {
			slog.Debug("emitting EventItemAssigned",
				slog.Int("item_id", updatedItem.ID),
				slog.Any("new_assignee_id", updatedItem.AssigneeID),
				slog.Any("old_assignee_id", originalItem.AssigneeID),
				slog.Any("creator_id", originalItem.CreatorID))

			// Construct the item key (e.g., "TST-1")
			itemKey := fmt.Sprintf("%s-%d", updatedItem.WorkspaceKey, updatedItem.WorkspaceItemNumber)

			h.notificationService.EmitEvent(&services.NotificationEvent{
				EventType:   models.EventItemAssigned,
				WorkspaceID: updatedItem.WorkspaceID,
				ActorUserID: user.ID,
				ItemID:      updatedItem.ID,
				AssigneeID:  updatedItem.AssigneeID,
				CreatorID:   originalItem.CreatorID,
				Title:       "Item Assigned",
				TemplateData: map[string]interface{}{
					"item.title": updatedItem.Title,
					"item.key":   itemKey,
					"item.id":    updatedItem.ID,
					"user.name":  user.Username,
				},
			})

			slog.Debug("successfully emitted EventItemAssigned", slog.Int("item_id", updatedItem.ID))
		}

		// Emit item updated notification (always, unless only status or assignee changed)
		if !result.StatusChanged && !assigneeChanged {
			// Construct the item key (e.g., "TST-1")
			itemKey := fmt.Sprintf("%s-%d", updatedItem.WorkspaceKey, updatedItem.WorkspaceItemNumber)

			h.notificationService.EmitEvent(&services.NotificationEvent{
				EventType:   models.EventItemUpdated,
				WorkspaceID: updatedItem.WorkspaceID,
				ActorUserID: user.ID,
				ItemID:      updatedItem.ID,
				AssigneeID:  updatedItem.AssigneeID,
				CreatorID:   originalItem.CreatorID,
				Title:       "Item Updated",
				TemplateData: map[string]interface{}{
					"item.title": updatedItem.Title,
					"item.key":   itemKey,
					"item.id":    updatedItem.ID,
					"user.name":  user.Username,
				},
			})
		}
	}

	// Emit action events for automation
	if h.actionService != nil && user != nil {
		// Emit status transition event
		if result.StatusChanged {
			h.actionService.EmitActionEvent(&models.ActionEvent{
				EventType:   models.ActionTriggerStatusTransition,
				WorkspaceID: updatedItem.WorkspaceID,
				ItemID:      updatedItem.ID,
				ActorUserID: user.ID,
				OldValues: map[string]interface{}{
					"status_id": originalItem.StatusID,
				},
				NewValues: map[string]interface{}{
					"status_id":   updatedItem.StatusID,
					"title":       updatedItem.Title,
					"assignee_id": updatedItem.AssigneeID,
					"creator_id":  updatedItem.CreatorID,
				},
			})
		}

		// Emit item updated event for other changes
		if !result.StatusChanged {
			h.actionService.EmitActionEvent(&models.ActionEvent{
				EventType:   models.ActionTriggerItemUpdated,
				WorkspaceID: updatedItem.WorkspaceID,
				ItemID:      updatedItem.ID,
				ActorUserID: user.ID,
				OldValues: map[string]interface{}{
					"status_id":   originalItem.StatusID,
					"assignee_id": originalItem.AssigneeID,
					"title":       originalItem.Title,
					"priority_id": originalItem.PriorityID,
				},
				NewValues: map[string]interface{}{
					"status_id":   updatedItem.StatusID,
					"assignee_id": updatedItem.AssigneeID,
					"title":       updatedItem.Title,
					"priority_id": updatedItem.PriorityID,
					"creator_id":  updatedItem.CreatorID,
				},
			})
		}
	}

	// Dispatch webhook events for item updates
	if h.webhookSender != nil {
		// Dispatch specific events based on what changed
		if result.StatusChanged {
			go h.webhookSender.DispatchEvent("status.changed", updatedItem)
		}
		if assigneeChanged {
			go h.webhookSender.DispatchEvent("item.assigned", updatedItem)
		}
		// Always dispatch item.updated for any update
		go h.webhookSender.DispatchEvent("item.updated", updatedItem)
	}

	// Process @mentions in description if it changed
	if h.mentionService != nil && originalItem.Description != updatedItem.Description {
		if err := h.mentionService.ProcessMentions(services.ProcessMentionsParams{
			SourceType:  "item_description",
			SourceID:    updatedItem.ID,
			Content:     updatedItem.Description,
			ItemID:      updatedItem.ID,
			WorkspaceID: updatedItem.WorkspaceID,
			ActorUserID: user.ID,
		}); err != nil {
			slog.Warn("failed to process description mentions", slog.Int("item_id", updatedItem.ID), slog.Any("error", err))
			// Don't fail the request if mention processing fails
		}
	}

	respondJSONOK(w, updatedItem)
}

func (h *ItemHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Require authentication
	user := h.getUserFromContext(r)
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	// Get item details before deletion (for permission check and notifications)
	repo := repository.NewItemRepository(h.db)
	item, err := repo.FindByID(id)
	if err != nil {
		if err == repository.ErrNotFound {
			http.Error(w, "Item not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to fetch item: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Check permission
	canDelete, err := h.canDeleteItem(user.ID, item.WorkspaceID)
	if err != nil {
		http.Error(w, "Permission check failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if !canDelete {
		http.Error(w, "Insufficient permissions to delete items in this workspace", http.StatusForbidden)
		return
	}

	// Delete using repository
	tx, err := h.db.Begin()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	if err := repo.DeleteItemLinks(tx, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := repo.ClearWorklogItemReferences(tx, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := repo.Delete(tx, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Emit notification
	if h.notificationService != nil {
		h.notificationService.EmitEvent(&services.NotificationEvent{
			EventType:   models.EventItemDeleted,
			WorkspaceID: item.WorkspaceID,
			ActorUserID: user.ID,
			ItemID:      id,
			AssigneeID:  item.AssigneeID,
			CreatorID:   item.CreatorID,
			Title:       "Item Deleted",
			TemplateData: map[string]interface{}{
				"item.title": item.Title,
				"item.id":    id,
				"user.name":  user.Username,
			},
		})
	}

	// Dispatch webhook event for item deletion
	if h.webhookSender != nil {
		go h.webhookSender.DispatchEvent("item.deleted", item)
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetDeleteInfo returns information needed before deleting an item (descendant count, parent info)
func (h *ItemHandler) GetDeleteInfo(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Require authentication
	user := h.getUserFromContext(r)
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	repo := repository.NewItemRepository(h.db)
	item, err := repo.FindByID(id)
	if err != nil {
		if err == repository.ErrNotFound {
			http.Error(w, "Item not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to fetch item: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Check permission - need at least view access
	canEdit, err := h.canEditItem(user.ID, item.WorkspaceID)
	if err != nil {
		http.Error(w, "Permission check failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if !canEdit {
		http.Error(w, "Insufficient permissions", http.StatusForbidden)
		return
	}

	// Get descendant IDs
	descendantIDs, err := repo.GetDescendantIDs(id)
	if err != nil {
		http.Error(w, "Failed to get descendants: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Get hierarchy level for the item type (needed for filtering reparent candidates)
	var hierarchyLevel sql.NullInt64
	if item.ItemTypeID != nil {
		h.db.QueryRow("SELECT hierarchy_level FROM item_types WHERE id = ?", *item.ItemTypeID).Scan(&hierarchyLevel)
	}

	response := map[string]interface{}{
		"hasChildren":     len(descendantIDs) > 0,
		"descendantCount": len(descendantIDs),
		"parentId":        item.ParentID,
		"title":           item.Title,
		"itemTypeId":      item.ItemTypeID,
		"workspaceId":     item.WorkspaceID,
		"hierarchyLevel":  utils.NullInt64ToPtr(hierarchyLevel),
	}

	respondJSONOK(w, response)
}

// ReparentChildren moves all direct children of an item to a new parent
func (h *ItemHandler) ReparentChildren(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Require authentication
	user := h.getUserFromContext(r)
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	var req struct {
		NewParentID *int `json:"newParentId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	repo := repository.NewItemRepository(h.db)
	item, err := repo.FindByID(id)
	if err != nil {
		if err == repository.ErrNotFound {
			http.Error(w, "Item not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to fetch item: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Check permission
	canEdit, err := h.canEditItem(user.ID, item.WorkspaceID)
	if err != nil {
		http.Error(w, "Permission check failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if !canEdit {
		http.Error(w, "Insufficient permissions to modify items in this workspace", http.StatusForbidden)
		return
	}

	// If new parent is specified, verify it exists and is in the same workspace
	if req.NewParentID != nil {
		newParent, err := repo.FindByID(*req.NewParentID)
		if err != nil {
			if err == repository.ErrNotFound {
				http.Error(w, "New parent item not found", http.StatusNotFound)
				return
			}
			http.Error(w, "Failed to fetch new parent: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if newParent.WorkspaceID != item.WorkspaceID {
			http.Error(w, "New parent must be in the same workspace", http.StatusBadRequest)
			return
		}
	}

	// Get direct children
	children, err := repo.GetChildren(id)
	if err != nil {
		http.Error(w, "Failed to get children: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if len(children) == 0 {
		respondJSONOK(w, map[string]interface{}{"reparentedCount": 0})
		return
	}

	// Start transaction
	tx, err := h.db.Begin()
	if err != nil {
		http.Error(w, "Failed to begin transaction: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// Update parent_id for all direct children
	for _, child := range children {
		if err := repo.UpdateParent(tx, child.ID, req.NewParentID); err != nil {
			http.Error(w, "Failed to update child parent: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, "Failed to commit transaction: "+err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSONOK(w, map[string]interface{}{"reparentedCount": len(children)})
}

// DeleteCascade deletes an item and all its descendants
func (h *ItemHandler) DeleteCascade(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Require authentication
	user := h.getUserFromContext(r)
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	// Get item details before deletion (for permission check and notifications)
	repo := repository.NewItemRepository(h.db)
	item, err := repo.FindByID(id)
	if err != nil {
		if err == repository.ErrNotFound {
			http.Error(w, "Item not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to fetch item: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Check permission
	canDelete, err := h.canDeleteItem(user.ID, item.WorkspaceID)
	if err != nil {
		http.Error(w, "Permission check failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if !canDelete {
		http.Error(w, "Insufficient permissions to delete items in this workspace", http.StatusForbidden)
		return
	}

	// Use the CRUD service for cascade delete
	crudService := services.NewItemCRUDService(h.db)
	result, err := crudService.Delete(id)
	if err != nil {
		http.Error(w, "Failed to delete item: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Emit notification for the main item
	if h.notificationService != nil {
		h.notificationService.EmitEvent(&services.NotificationEvent{
			EventType:   models.EventItemDeleted,
			WorkspaceID: item.WorkspaceID,
			ActorUserID: user.ID,
			ItemID:      id,
			AssigneeID:  item.AssigneeID,
			CreatorID:   item.CreatorID,
			Title:       "Item Deleted",
			TemplateData: map[string]interface{}{
				"item.title":  item.Title,
				"item.id":     id,
				"user.name":   user.Username,
				"descendants": result.DeletedCount - 1,
			},
		})
	}

	// Dispatch webhook event for item deletion
	if h.webhookSender != nil {
		go h.webhookSender.DispatchEvent("item.deleted", item)
	}

	respondJSONOK(w, map[string]interface{}{
		"deletedCount": result.DeletedCount,
	})
}

func (h *ItemHandler) Copy(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Require authentication
	user := h.getUserFromContext(r)
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	// Get the original item using repository
	repo := repository.NewItemRepository(h.db)
	originalItem, err := repo.FindByID(id)
	if err != nil {
		if err == repository.ErrNotFound {
			http.Error(w, "Item not found", http.StatusNotFound)
		} else {
			http.Error(w, fmt.Sprintf("Failed to get original item: %v", err), http.StatusInternalServerError)
		}
		return
	}

	// Check permission
	canEdit, err := h.canEditItem(user.ID, originalItem.WorkspaceID)
	if err != nil {
		http.Error(w, "Permission check failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if !canEdit {
		http.Error(w, "Insufficient permissions to copy items in this workspace", http.StatusForbidden)
		return
	}

	// Create copy title
	copyTitle := utils.SanitizeTitle(fmt.Sprintf("COPY - %s", originalItem.Title))

	// Generate frac_index for the copy
	newFracIndex, err := services.GenerateFracIndexForNewItem(h.db.GetDB(), originalItem.WorkspaceID, originalItem.ParentID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to generate frac_index: %v", err), http.StatusInternalServerError)
		return
	}

	// Create the copy in a transaction
	tx, err := h.db.Begin()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	nextNum, err := repo.GetNextWorkspaceItemNumber(tx, originalItem.WorkspaceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	newItem := &models.Item{
		WorkspaceID:         originalItem.WorkspaceID,
		WorkspaceItemNumber: nextNum,
		ItemTypeID:          originalItem.ItemTypeID,
		Title:               copyTitle,
		Description:         originalItem.Description,
		StatusID:            originalItem.StatusID,
		PriorityID:          originalItem.PriorityID,
		DueDate:             originalItem.DueDate,
		MilestoneID:         originalItem.MilestoneID,
		AssigneeID:          originalItem.AssigneeID,
		CreatorID:           &user.ID,
		ParentID:            originalItem.ParentID,
		TimeProjectID:       originalItem.TimeProjectID,
		CustomFieldValues:   originalItem.CustomFieldValues,
		FracIndex:           &newFracIndex,
	}

	copiedItemID, err := repo.Create(tx, newItem)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create copy: %v", err), http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Record item creation history for the copied item
	updateService := services.NewItemUpdateService(h.db)
	if err := updateService.RecordItemCreationHistory(h.db, int(copiedItemID), user.ID); err != nil {
		slog.Warn("failed to record copied item creation history", slog.Int("item_id", copiedItemID), slog.Any("error", err))
		// Don't fail request, just log the error
	}

	// Return the copied item
	newItem.ID = copiedItemID
	respondJSONOK(w, newItem)
}

// GetCacheStats returns cache performance statistics
// GET /api/items/cache-stats
func (h *ItemHandler) GetCacheStats(w http.ResponseWriter, r *http.Request) {
	if h.itemCache == nil {
		http.Error(w, "Item cache is not enabled", http.StatusServiceUnavailable)
		return
	}

	stats := h.itemCache.GetStats()

	respondJSONOK(w, map[string]interface{}{
		"cache_enabled": true,
		"statistics":    stats,
		"timestamp":     time.Now().Format(time.RFC3339),
	})
}
