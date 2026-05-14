package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
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

	req, ok := decodeJSON[ScheduleCalendarRequest](w, r)
	if !ok {
		return
	}

	// Require authentication
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Self-scheduling only: reject mismatched user_id (parallels UnscheduleItem).
	if req.UserID != 0 && req.UserID != user.ID {
		respondForbidden(w, r)
		return
	}

	// Get current calendar data and workspace_id for permission check
	calendarDataJSON, workspaceID, err := repository.NewItemRepository(h.db).GetCalendarData(id)
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
		if entry.UserID != user.ID {
			filteredData = append(filteredData, entry)
		}
	}

	// Add new schedule entry
	newEntry := models.CalendarScheduleEntry{
		UserID:          user.ID,
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
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Get current calendar data and workspace_id for permission check
	calendarDataJSON, workspaceID, err := repository.NewItemRepository(h.db).GetCalendarData(id)
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
	user, ok := RequireAuth(w, r)
	if !ok {
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

	itemsWithCalendar, err := repository.NewItemRepository(h.db).ListItemsWithCalendarData(accessibleWorkspaceIDs)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// First collect all items with calendar data
	allItems := make([]models.Item, 0, len(itemsWithCalendar))
	itemCalendarData := make(map[int][]models.CalendarScheduleEntry) // item.ID -> calendar entries
	itemIsPersonal := make(map[int]bool)

	for _, result := range itemsWithCalendar {
		allItems = append(allItems, result.Item)
		itemCalendarData[result.Item.ID] = result.CalendarEntries
		itemIsPersonal[result.Item.ID] = result.IsPersonal
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
