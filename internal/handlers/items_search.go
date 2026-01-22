package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"windshift/internal/cql"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// Search filter limits to prevent abuse
const (
	maxSearchQueryLength  = 500 // Maximum characters for search query
	maxWorkspaceFilters   = 50  // Maximum number of workspace IDs in filter
	maxStatusFilters      = 20  // Maximum number of statuses in filter
	maxPriorityFilters    = 10  // Maximum number of priorities in filter
)

// Search items across workspaces with advanced filtering
func (h *ItemHandler) Search(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

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
		respondJSONOK(w, []models.Item{})
		return
	}

	// Get search parameters with input validation
	textQuery := r.URL.Query().Get("q")
	workspaceIDs := r.URL.Query()["workspace_id"] // Allow multiple workspace IDs
	statuses := r.URL.Query()["status"]           // Allow multiple statuses
	priorities := r.URL.Query()["priority"]       // Allow multiple priorities

	// Validate and sanitize inputs
	if len(textQuery) > maxSearchQueryLength {
		http.Error(w, fmt.Sprintf("Search query too long (max %d characters)", maxSearchQueryLength), http.StatusBadRequest)
		return
	}

	// Validate workspace IDs are numeric
	for _, workspaceID := range workspaceIDs {
		if workspaceID == "" {
			continue
		}
		if _, err := strconv.Atoi(workspaceID); err != nil {
			http.Error(w, "Invalid workspace ID format", http.StatusBadRequest)
			return
		}
	}

	// Validate status values against allowed statuses
	allowedStatuses := map[string]bool{
		"open": true, "to_do": true, "in_progress": true, "in_review": true,
		"completed": true, "cancelled": true, "done": true, "closed": true,
	}
	for _, status := range statuses {
		if status != "" && !allowedStatuses[status] {
			http.Error(w, fmt.Sprintf("Invalid status: %s", status), http.StatusBadRequest)
			return
		}
	}

	// Validate priority values
	allowedPriorities := map[string]bool{
		"low": true, "medium": true, "high": true, "critical": true,
	}
	for _, priority := range priorities {
		if priority != "" && !allowedPriorities[priority] {
			http.Error(w, fmt.Sprintf("Invalid priority: %s", priority), http.StatusBadRequest)
			return
		}
	}

	// Limit array sizes to prevent abuse
	if len(workspaceIDs) > maxWorkspaceFilters {
		http.Error(w, fmt.Sprintf("Too many workspace filters (max %d)", maxWorkspaceFilters), http.StatusBadRequest)
		return
	}
	if len(statuses) > maxStatusFilters {
		http.Error(w, fmt.Sprintf("Too many status filters (max %d)", maxStatusFilters), http.StatusBadRequest)
		return
	}
	if len(priorities) > maxPriorityFilters {
		http.Error(w, fmt.Sprintf("Too many priority filters (max %d)", maxPriorityFilters), http.StatusBadRequest)
		return
	}

	// Build the base query
	query := `
		SELECT i.id, i.workspace_id, i.workspace_item_number, i.item_type_id, i.title, i.description, i.status_id, i.priority_id,
		       i.custom_field_values, i.parent_id,
		       i.created_at, i.updated_at,
		       w.name as workspace_name, w.key as workspace_key, it.name as item_type_name,
		       p.title as parent_title
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN item_types it ON i.item_type_id = it.id
		LEFT JOIN items p ON i.parent_id = p.id
		WHERE 1=1`

	args := []interface{}{}

	// Add text search if provided
	if textQuery != "" {
		// Check if the query looks like a work item key (e.g., "OK-40", "ok-40")
		// Pattern: letters followed by hyphen followed by digits
		parts := strings.Split(strings.ToUpper(textQuery), "-")
		isKeyPattern := len(parts) == 2 && len(parts[0]) > 0 && len(parts[1]) > 0

		// Try to parse as workspace key + workspace item number
		var workspaceKey string
		var workspaceItemNumber int
		if isKeyPattern {
			workspaceKey = parts[0]
			var err error
			workspaceItemNumber, err = strconv.Atoi(parts[1])
			if err != nil {
				isKeyPattern = false
			}
		}

		if isKeyPattern && workspaceItemNumber > 0 {
			// Search by workspace key and workspace item number (case-insensitive workspace key)
			query += " AND (LOWER(w.key) = LOWER(?) AND i.workspace_item_number = ?)"
			args = append(args, workspaceKey, workspaceItemNumber)
		} else {
			// Regular text search (case-insensitive)
			query += " AND (LOWER(i.title) LIKE LOWER(?) OR LOWER(i.description) LIKE LOWER(?))"
			searchPattern := "%" + textQuery + "%"
			args = append(args, searchPattern, searchPattern)
		}
	}

	// Filter by accessible workspaces (respects workspace active status)
	// If specific workspace IDs were requested, intersect with accessible workspaces
	finalWorkspaceIDs := accessibleWorkspaceIDs
	if len(workspaceIDs) > 0 {
		// Convert requested workspace IDs to integers for comparison
		requestedIDs := make(map[int]bool)
		for _, wsID := range workspaceIDs {
			if wsID != "" {
				if id, err := strconv.Atoi(wsID); err == nil {
					requestedIDs[id] = true
				}
			}
		}

		// Intersect requested IDs with accessible IDs
		finalWorkspaceIDs = []int{}
		for _, id := range accessibleWorkspaceIDs {
			if requestedIDs[id] {
				finalWorkspaceIDs = append(finalWorkspaceIDs, id)
			}
		}
	}

	// Add workspace filter to query
	if len(finalWorkspaceIDs) > 0 {
		placeholders := make([]string, len(finalWorkspaceIDs))
		for i, id := range finalWorkspaceIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		query += " AND i.workspace_id IN (" + strings.Join(placeholders, ",") + ")"
	}

	// Add status filter if provided
	if len(statuses) > 0 {
		placeholders := strings.Repeat("?,", len(statuses))
		placeholders = strings.TrimSuffix(placeholders, ",")
		query += " AND i.status_id IN (" + placeholders + ")"
		for _, status := range statuses {
			args = append(args, status)
		}
	}

	// Add priority filter if provided
	if len(priorities) > 0 {
		placeholders := strings.Repeat("?,", len(priorities))
		placeholders = strings.TrimSuffix(placeholders, ",")
		query += " AND i.priority_id IN (" + placeholders + ")"
		for _, priority := range priorities {
			args = append(args, priority)
		}
	}

	// Add ordering
	query += " ORDER BY i.updated_at DESC, i.created_at DESC"

	// Add limit to prevent overwhelming results with validation
	limitStr := r.URL.Query().Get("limit")
	var limit int = 100 // Default limit
	if limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err != nil {
			http.Error(w, "Invalid limit format", http.StatusBadRequest)
			return
		}
		// Enforce reasonable limits
		if parsedLimit < 1 {
			limit = 1
		} else if parsedLimit > 1000 {
			limit = 1000 // Max limit to prevent resource exhaustion
		} else {
			limit = parsedLimit
		}
	}
	query += " LIMIT ?"
	args = append(args, limit)

	rows, err := h.db.Query(query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var items []models.Item
	for rows.Next() {
		var item models.Item
		var customFieldValuesJSON sql.NullString
		var itemTypeID, statusID, priorityID, parentID sql.NullInt64
		var parentTitle sql.NullString
		var itemTypeName sql.NullString

		err := rows.Scan(
			&item.ID, &item.WorkspaceID, &item.WorkspaceItemNumber, &itemTypeID, &item.Title, &item.Description,
			&statusID, &priorityID, &customFieldValuesJSON, &parentID,
			&item.CreatedAt, &item.UpdatedAt, &item.WorkspaceName, &item.WorkspaceKey,
			&itemTypeName, &parentTitle,
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Handle nullable fields
		item.ItemTypeID = utils.NullInt64ToPtr(itemTypeID)
		item.StatusID = utils.NullInt64ToPtr(statusID)
		item.PriorityID = utils.NullInt64ToPtr(priorityID)
		item.ParentID = utils.NullInt64ToPtr(parentID)

		// Parse custom field values
		if customFieldValuesJSON.Valid && customFieldValuesJSON.String != "" {
			if err := json.Unmarshal([]byte(customFieldValuesJSON.String), &item.CustomFieldValues); err != nil {
				item.CustomFieldValues = make(map[string]interface{})
			}
		} else {
			item.CustomFieldValues = make(map[string]interface{})
		}

		item.ItemTypeName = itemTypeName.String
		item.ParentTitle = parentTitle.String

		items = append(items, item)
	}

	if err = rows.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Filter items based on user permissions

	filteredItems, err := h.filterItemsByPermissions(user.ID, items)
	if err != nil {
		http.Error(w, "Permission check failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	items = filteredItems

	respondJSONOK(w, items)
}

// UpdateFracIndex updates the frac_index of an item for fractional indexing ordering
func (h *ItemHandler) UpdateFracIndex(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Parse the request body
	var fracIndexRequest struct {
		// Item ID-based ranking (uses prev/next item IDs to calculate frac_index)
		PrevItemID *int `json:"prev_item_id"` // ID of item before in current view
		NextItemID *int `json:"next_item_id"` // ID of item after in current view
	}

	if err := json.NewDecoder(r.Body).Decode(&fracIndexRequest); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Require authentication
	user := h.getUserFromContext(r)
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	// Get workspace_id for permission check
	var workspaceID int
	err := h.db.QueryRow("SELECT workspace_id FROM items WHERE id = ?", id).Scan(&workspaceID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Item not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to fetch item: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Check if user has permission to edit items in this workspace
	canEdit, permErr := h.canEditItem(user.ID, workspaceID)
	if permErr != nil {
		http.Error(w, "Permission check failed: "+permErr.Error(), http.StatusInternalServerError)
		return
	}
	if !canEdit {
		http.Error(w, "Insufficient permissions to reorder items in this workspace", http.StatusForbidden)
		return
	}

	// Generate the new frac_index
	var prevFracIndex, nextFracIndex string

	// Look up frac_index values from item IDs directly (trust the caller's prev/next selection)
	if fracIndexRequest.PrevItemID != nil {
		var prevItemFracIndex sql.NullString
		err = h.db.QueryRow("SELECT frac_index FROM items WHERE id = ?", *fracIndexRequest.PrevItemID).Scan(&prevItemFracIndex)
		if err == nil && prevItemFracIndex.Valid {
			prevFracIndex = prevItemFracIndex.String
		}
	}

	if fracIndexRequest.NextItemID != nil {
		var nextItemFracIndex sql.NullString
		err = h.db.QueryRow("SELECT frac_index FROM items WHERE id = ?", *fracIndexRequest.NextItemID).Scan(&nextItemFracIndex)
		if err == nil && nextItemFracIndex.Valid {
			nextFracIndex = nextItemFracIndex.String
		}
	}

	// Defensive check: if prev and next have the same frac_index, skip update
	if prevFracIndex != "" && nextFracIndex != "" && prevFracIndex == nextFracIndex {
		// Return the item as-is without error
		h.Get(w, r)
		return
	}

	// Get the current item's frac_index to check if update is needed
	var currentFracIndex sql.NullString
	err = h.db.QueryRow("SELECT frac_index FROM items WHERE id = ?", id).Scan(&currentFracIndex)
	if err != nil {
		http.Error(w, "Failed to get current frac_index: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Check if the item is already in the correct position
	if currentFracIndex.Valid {
		current := currentFracIndex.String
		// Item is already correctly positioned if:
		// - It's after prev (or prev is empty)
		// - It's before next (or next is empty)
		isAfterPrev := prevFracIndex == "" || current > prevFracIndex
		isBeforeNext := nextFracIndex == "" || current < nextFracIndex

		if isAfterPrev && isBeforeNext {
			// Return the item as-is without error
			h.Get(w, r)
			return
		}
	}

	newFracIndex, err := services.KeyBetween(prevFracIndex, nextFracIndex)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to generate frac_index: %v", err), http.StatusInternalServerError)
		return
	}

	// Update the item's frac_index
	err = services.UpdateItemFracIndex(h.db.GetDB(), id, newFracIndex)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to update frac_index: %v", err), http.StatusInternalServerError)
		return
	}

	// Return the updated item
	h.Get(w, r)
}

// buildWorkspaceMap creates a mapping of workspace names/keys to IDs for QL evaluation
func (h *ItemHandler) buildWorkspaceMap() (map[string]int, error) {
	workspaceMap := make(map[string]int)

	rows, err := h.db.Query("SELECT id, name, key FROM workspaces")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		var name, key string
		if err := rows.Scan(&id, &name, &key); err != nil {
			return nil, err
		}

		// Map by ID, name (lowercase), and key (lowercase)
		workspaceMap[strconv.Itoa(id)] = id
		workspaceMap[strings.ToLower(name)] = id
		workspaceMap[strings.ToLower(key)] = id
	}

	return workspaceMap, nil
}

// GetBacklogItems returns items whose statuses are not marked as completed for a workspace
func (h *ItemHandler) GetBacklogItems(w http.ResponseWriter, r *http.Request) {
	// Get user from context
	user := h.getUserFromContext(r)
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	workspaceID := r.URL.Query().Get("workspace_id")
	if workspaceID == "" {
		http.Error(w, "workspace_id parameter is required", http.StatusBadRequest)
		return
	}

	// Convert workspace ID to int for validation
	wsID, err := strconv.Atoi(workspaceID)
	if err != nil {
		http.Error(w, "Invalid workspace_id format", http.StatusBadRequest)
		return
	}

	// Get accessible workspace IDs
	accessibleWorkspaceIDs, err := h.getAccessibleWorkspaceIDs(user)
	if err != nil {
		http.Error(w, "Failed to get accessible workspaces: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Check if user can access this workspace
	canAccess := false
	for _, id := range accessibleWorkspaceIDs {
		if id == wsID {
			canAccess = true
			break
		}
	}

	if !canAccess {
		http.Error(w, "You don't have access to this workspace", http.StatusForbidden)
		return
	}

	// Get backlog status IDs - either from board configuration or default to non-Done statuses
	var backlogStatusIDs []int

	// First, check if there's a board configuration with backlog_status_ids for this workspace
	var backlogStatusIDsJSON sql.NullString
	err = h.db.QueryRow(`
		SELECT backlog_status_ids
		FROM board_configurations
		WHERE workspace_id = ?`,
		wsID,
	).Scan(&backlogStatusIDsJSON)

	if err == nil && backlogStatusIDsJSON.Valid && backlogStatusIDsJSON.String != "" {
		// Parse the configured backlog status IDs
		if err := json.Unmarshal([]byte(backlogStatusIDsJSON.String), &backlogStatusIDs); err != nil {
			http.Error(w, "Failed to parse backlog configuration", http.StatusInternalServerError)
			return
		}
	} else {
		// Fall back to default behavior: get status IDs that are not marked as completed
		statusQuery := `
			SELECT DISTINCT s.id
			FROM statuses s
			JOIN status_categories sc ON s.category_id = sc.id
			WHERE COALESCE(sc.is_completed, FALSE) = FALSE`

		statusRows, err := h.db.Query(statusQuery)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer statusRows.Close()

		for statusRows.Next() {
			var statusID int
			if err := statusRows.Scan(&statusID); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			backlogStatusIDs = append(backlogStatusIDs, statusID)
		}
	}

	if len(backlogStatusIDs) == 0 {
		// Return empty array if no statuses found
		respondJSONOK(w, []models.Item{})
		return
	}

	// Build the query with placeholders for all backlog status IDs
	placeholders := strings.Repeat("?,", len(backlogStatusIDs))
	placeholders = placeholders[:len(placeholders)-1] // Remove trailing comma

	// Build base query
	query := fmt.Sprintf(`
		SELECT i.id, i.workspace_id, i.workspace_item_number, i.item_type_id, i.title, i.description, i.status_id, i.is_task,
		       i.milestone_id, i.iteration_id, i.time_project_id, i.assignee_id, i.creator_id, i.custom_field_values, i.calendar_data, i.parent_id,
		       i.created_at, i.updated_at,
		       w.name as workspace_name, it.name as item_type_name,
		       p.title as parent_title, m.name as milestone_name, iter.name as iteration_name, tp.name as time_project_name,
		       assignee.first_name || ' ' || assignee.last_name as assignee_name, assignee.email as assignee_email, assignee.avatar_url as assignee_avatar,
		       creator.first_name || ' ' || creator.last_name as creator_name, creator.email as creator_email,
		       st.name as status_name, pri.name as priority_name, pri.icon as priority_icon, pri.color as priority_color
		FROM items i
		LEFT JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN item_types it ON i.item_type_id = it.id
		LEFT JOIN items p ON i.parent_id = p.id
		LEFT JOIN milestones m ON i.milestone_id = m.id
		LEFT JOIN iterations iter ON i.iteration_id = iter.id
		LEFT JOIN time_projects tp ON i.time_project_id = tp.id
		LEFT JOIN users assignee ON i.assignee_id = assignee.id
		LEFT JOIN users creator ON i.creator_id = creator.id
		LEFT JOIN statuses st ON i.status_id = st.id
		LEFT JOIN priorities pri ON i.priority_id = pri.id
		WHERE i.workspace_id = ? AND i.status_id IN (%s)`, placeholders)

	// Prepare arguments: workspace_id + all backlog status IDs
	args := []interface{}{workspaceID}
	for _, statusID := range backlogStatusIDs {
		args = append(args, statusID)
	}

	// Check for QL query parameter and add additional filtering
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
			query += " AND (" + qlSQL + ")"
			args = append(args, qlArgs...)
		}
	}

	// Add ORDER BY clause
	query += `
		ORDER BY
			CASE WHEN i.frac_index IS NULL THEN 1 ELSE 0 END,
			i.frac_index ASC,
			i.created_at DESC`

	rows, err := h.db.Query(query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var items []models.Item
	for rows.Next() {
		var item models.Item
		var customFieldValues sql.NullString
		var calendarData sql.NullString
		var statusID sql.NullInt64
		var workspaceName, itemTypeName, parentTitle, milestoneName, iterationName, timeProjectName sql.NullString
		var assigneeName, assigneeEmail, assigneeAvatar, creatorName, creatorEmail sql.NullString
		var statusName sql.NullString
		var priorityName, priorityIcon, priorityColor sql.NullString

		err := rows.Scan(
			&item.ID, &item.WorkspaceID, &item.WorkspaceItemNumber, &item.ItemTypeID, &item.Title, &item.Description,
			&statusID, &item.IsTask,
			&item.MilestoneID, &item.IterationID, &item.TimeProjectID, &item.AssigneeID, &item.CreatorID, &customFieldValues, &calendarData, &item.ParentID,
			&item.CreatedAt, &item.UpdatedAt,
			&workspaceName, &itemTypeName, &parentTitle, &milestoneName, &iterationName, &timeProjectName,
			&assigneeName, &assigneeEmail, &assigneeAvatar, &creatorName, &creatorEmail,
			&statusName,
			&priorityName, &priorityIcon, &priorityColor,
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Handle nullable status_id field
		item.StatusID = utils.NullInt64ToPtr(statusID)

		// Handle JSON fields
		if customFieldValues.Valid && customFieldValues.String != "" {
			if err := json.Unmarshal([]byte(customFieldValues.String), &item.CustomFieldValues); err != nil {
				item.CustomFieldValues = make(map[string]interface{})
			}
		}

		if calendarData.Valid && calendarData.String != "" {
			if err := json.Unmarshal([]byte(calendarData.String), &item.CalendarData); err != nil {
				item.CalendarData = []models.CalendarScheduleEntry{}
			}
		}

		// Set joined fields
		item.WorkspaceName = workspaceName.String
		item.ItemTypeName = itemTypeName.String
		item.ParentTitle = parentTitle.String
		item.MilestoneName = milestoneName.String
		item.IterationName = iterationName.String
		item.TimeProjectName = timeProjectName.String
		item.AssigneeName = assigneeName.String
		item.AssigneeEmail = assigneeEmail.String
		item.AssigneeAvatar = assigneeAvatar.String
		item.CreatorName = creatorName.String
		item.CreatorEmail = creatorEmail.String
		item.StatusName = statusName.String
		item.PriorityName = priorityName.String
		item.PriorityIcon = priorityIcon.String
		item.PriorityColor = priorityColor.String

		items = append(items, item)
	}

	// Always return an array, even if empty
	if items == nil {
		items = []models.Item{}
	}

	// Filter items based on user permissions
	filteredItems, err := h.filterItemsByPermissions(user.ID, items)
	if err != nil {
		http.Error(w, "Permission check failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	items = filteredItems

	respondJSONOK(w, items)
}
