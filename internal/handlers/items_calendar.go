package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/models"
)

// ScheduleCalendarRequest represents the request to schedule an item on calendar
type ScheduleCalendarRequest struct {
	UserID          int    `json:"user_id"`
	WorkspaceID     int    `json:"workspace_id"`               // User's personal workspace ID
	ScheduledDate   string `json:"scheduled_date"`             // YYYY-MM-DD format
	ScheduledTime   string `json:"scheduled_time,omitempty"`   // HH:MM format, optional
	DurationMinutes int    `json:"duration_minutes,omitempty"` // Duration in minutes, optional
	Notes           string `json:"notes,omitempty"`
}

// ScheduleItem adds an item to a user's calendar
func (h *ItemHandler) ScheduleItem(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var req ScheduleCalendarRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Require authentication
	user := h.getUserFromContext(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	// Get current calendar data and workspace_id for permission check
	var calendarDataJSON sql.NullString
	var workspaceID int
	err := h.db.QueryRow("SELECT calendar_data, workspace_id FROM items WHERE id = ?", id).Scan(&calendarDataJSON, &workspaceID)
	if err != nil {
		respondNotFound(w, r, "item")
		return
	}

	// Check if user has permission to edit items in this workspace
	canEdit, permErr := h.canEditItem(user.ID, workspaceID)
	if permErr != nil {
		respondInternalError(w, r, permErr)
		return
	}
	if !canEdit {
		respondNotFound(w, r, "Item")
		return
	}

	// Parse existing calendar data
	var calendarData []models.CalendarScheduleEntry
	if calendarDataJSON.Valid && calendarDataJSON.String != "" {
		if err = json.Unmarshal([]byte(calendarDataJSON.String), &calendarData); err != nil {
			calendarData = []models.CalendarScheduleEntry{}
		}
	}

	// Remove existing schedule for this user if any
	filteredData := []models.CalendarScheduleEntry{}
	for _, entry := range calendarData {
		if entry.UserID != req.UserID {
			filteredData = append(filteredData, entry)
		}
	}

	// Add new schedule entry
	newEntry := models.CalendarScheduleEntry{
		UserID:          req.UserID,
		WorkspaceID:     req.WorkspaceID,
		ScheduledDate:   req.ScheduledDate,
		ScheduledTime:   req.ScheduledTime,
		DurationMinutes: req.DurationMinutes,
		Notes:           req.Notes,
		CreatedAt:       time.Now().UTC().Format(time.RFC3339),
	}
	filteredData = append(filteredData, newEntry)

	// Marshal back to JSON
	updatedJSON, err := json.Marshal(filteredData)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to update calendar data: %w", err))
		return
	}

	// Update the database
	_, err = h.db.ExecWrite("UPDATE items SET calendar_data = ?, updated_at = ? WHERE id = ?",
		string(updatedJSON), time.Now().UTC(), id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, map[string]interface{}{
		"status":   "success",
		"message":  "Item scheduled successfully",
		"schedule": newEntry,
	})
}

// UnscheduleItem removes an item from a user's calendar
func (h *ItemHandler) UnscheduleItem(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	userIDStr := r.URL.Query().Get("user_id")
	if userIDStr == "" {
		respondValidationError(w, r, "user_id parameter is required")
		return
	}

	userID, err := strconv.Atoi(userIDStr)
	if err != nil {
		respondInvalidID(w, r, "user_id")
		return
	}

	// Require authentication
	user := h.getUserFromContext(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	// Get current calendar data and workspace_id for permission check
	var calendarDataJSON sql.NullString
	var workspaceID int
	err = h.db.QueryRow("SELECT calendar_data, workspace_id FROM items WHERE id = ?", id).Scan(&calendarDataJSON, &workspaceID)
	if err != nil {
		respondNotFound(w, r, "item")
		return
	}

	// Check if user has permission to edit items in this workspace
	canEdit, err := h.canEditItem(user.ID, workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canEdit {
		respondNotFound(w, r, "Item")
		return
	}

	// Verify the requesting user is the schedule owner (security fix)
	if user.ID != userID {
		respondForbidden(w, r)
		return
	}

	// Parse existing calendar data
	var calendarData []models.CalendarScheduleEntry
	if calendarDataJSON.Valid && calendarDataJSON.String != "" {
		if err = json.Unmarshal([]byte(calendarDataJSON.String), &calendarData); err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	// Remove schedule for this user
	filteredData := []models.CalendarScheduleEntry{}
	found := false
	for _, entry := range calendarData {
		if entry.UserID != userID {
			filteredData = append(filteredData, entry)
		} else {
			found = true
		}
	}

	if !found {
		respondNotFound(w, r, "schedule")
		return
	}

	// Marshal back to JSON
	updatedJSON, err := json.Marshal(filteredData)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Update the database
	_, err = h.db.ExecWrite("UPDATE items SET calendar_data = ?, updated_at = ? WHERE id = ?",
		string(updatedJSON), time.Now().UTC(), id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, map[string]string{
		"status":  "success",
		"message": "Item unscheduled successfully",
	})
}

// GetScheduledItems returns all items scheduled for the authenticated user
func (h *ItemHandler) GetScheduledItems(w http.ResponseWriter, r *http.Request) {
	// Require authentication - use authenticated user's ID only
	user := h.getUserFromContext(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	// Use authenticated user's ID - do not accept user_id parameter for security
	userID := user.ID

	// Get accessible workspace IDs (includes active workspaces and inactive ones where user has admin access)
	accessibleWorkspaceIDs, err := h.getAccessibleWorkspaceIDs(user)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// If user has no accessible workspaces, return empty result
	if len(accessibleWorkspaceIDs) == 0 {
		respondJSONOK(w, map[string][]map[string]interface{}{})
		return
	}

	startDate := r.URL.Query().Get("start_date") // YYYY-MM-DD format
	endDate := r.URL.Query().Get("end_date")     // YYYY-MM-DD format

	// Build workspace filter placeholders
	placeholders := make([]string, len(accessibleWorkspaceIDs))
	args := make([]interface{}, len(accessibleWorkspaceIDs))
	for i, id := range accessibleWorkspaceIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT i.id, i.workspace_id, i.workspace_item_number, i.title, i.description, i.status_id, i.priority_id,
		       i.assignee_id, i.creator_id, i.calendar_data, i.due_date, i.created_at, i.updated_at,
		       w.name as workspace_name, w.key as workspace_key, w.is_personal
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		WHERE i.calendar_data IS NOT NULL AND i.calendar_data != ''
		  AND i.workspace_id IN (%s)
	`, strings.Join(placeholders, ","))

	rows, err := h.db.Query(query, args...)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	// First collect all items with calendar data
	var allItems []models.Item
	itemCalendarData := make(map[int][]models.CalendarScheduleEntry) // item.ID -> calendar entries

	// Track is_personal per item ID for the response
	itemIsPersonal := make(map[int]bool)

	for rows.Next() {
		var item models.Item
		var calendarDataJSON sql.NullString
		var statusID, priorityID, assigneeID, creatorID sql.NullInt64
		var dueDate sql.NullTime
		var isPersonal bool

		err = rows.Scan(&item.ID, &item.WorkspaceID, &item.WorkspaceItemNumber, &item.Title, &item.Description,
			&statusID, &priorityID, &assigneeID, &creatorID, &calendarDataJSON, &dueDate,
			&item.CreatedAt, &item.UpdatedAt, &item.WorkspaceName, &item.WorkspaceKey, &isPersonal)
		if err != nil {
			continue
		}

		itemIsPersonal[item.ID] = isPersonal

		// Handle nullable fields
		if statusID.Valid {
			v := int(statusID.Int64)
			item.StatusID = &v
		}
		if priorityID.Valid {
			v := int(priorityID.Int64)
			item.PriorityID = &v
		}
		if assigneeID.Valid {
			v := int(assigneeID.Int64)
			item.AssigneeID = &v
		}
		if creatorID.Valid {
			v := int(creatorID.Int64)
			item.CreatorID = &v
		}
		if dueDate.Valid {
			item.DueDate = &dueDate.Time
		}

		// Parse calendar data
		var calendarData []models.CalendarScheduleEntry
		if calendarDataJSON.Valid && calendarDataJSON.String != "" {
			if err = json.Unmarshal([]byte(calendarDataJSON.String), &calendarData); err != nil {
				continue
			}
		}

		allItems = append(allItems, item)
		itemCalendarData[item.ID] = calendarData
	}

	// Apply permission filtering
	filteredItems, err := h.filterItemsByPermissions(user.ID, allItems)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Build scheduled items map from filtered items only
	scheduledItems := make(map[string][]map[string]interface{})

	for _, item := range filteredItems {
		calendarData := itemCalendarData[item.ID]

		// Filter by user and date range
		for _, entry := range calendarData {
			if entry.UserID != userID {
				continue
			}

			// Check date range if specified
			if startDate != "" && entry.ScheduledDate < startDate {
				continue
			}
			if endDate != "" && entry.ScheduledDate > endDate {
				continue
			}

			// Add to results grouped by date
			if scheduledItems[entry.ScheduledDate] == nil {
				scheduledItems[entry.ScheduledDate] = []map[string]interface{}{}
			}

			itemWithSchedule := map[string]interface{}{
				"id":                  item.ID,
				"workspace_id":        item.WorkspaceID,
				"title":               item.Title,
				"description":         item.Description,
				"status_id":           item.StatusID,
				"status_name":         item.StatusName,
				"priority_name":       item.PriorityName,
				"assignee_id":         item.AssigneeID,
				"creator_id":          item.CreatorID,
				"workspace_name":      item.WorkspaceName,
				"workspace_key":       item.WorkspaceKey,
				"is_personal":         itemIsPersonal[item.ID],
				"due_date":            item.DueDate,
				"created_at":          item.CreatedAt,
				"updated_at":          item.UpdatedAt,
				"scheduled_time":      entry.ScheduledTime,
				"duration_minutes":    entry.DurationMinutes,
				"notes":               entry.Notes,
				"schedule_created_at": entry.CreatedAt,
			}

			scheduledItems[entry.ScheduledDate] = append(scheduledItems[entry.ScheduledDate], itemWithSchedule)
		}
	}

	respondJSONOK(w, scheduledItems)
}
