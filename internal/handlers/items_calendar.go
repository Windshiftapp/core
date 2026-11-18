package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
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
	sanitize.ApplyAll(
		sanitize.Pair{Target: &req.Notes, Policy: sanitize.RichText},
		sanitize.Pair{Target: &req.ScheduledDate, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &req.ScheduledTime, Policy: sanitize.ShortIdentifier},
	)

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

	repo := repository.NewItemRepository(h.db)

	// Check if user has permission to edit items in this workspace.
	workspaceID, err := repo.GetWorkspaceIDCtx(r.Context(), id)
	if err != nil {
		respondNotFound(w, r, "item")
		return
	}
	canEdit, permErr := h.canEditItem(user.ID, workspaceID)
	if permErr != nil {
		respondInternalError(w, r, permErr)
		return
	}
	if !canEdit {
		respondNotFound(w, r, "Item")
		return
	}

	// Replace this user's entry and append the new one in one transaction so a
	// concurrent schedule for another user is not lost.
	var newEntry models.CalendarScheduleEntry
	err = repo.MutateCalendarData(r.Context(), id, func(entries []models.CalendarScheduleEntry) ([]models.CalendarScheduleEntry, error) {
		filtered := make([]models.CalendarScheduleEntry, 0, len(entries)+1)
		for _, entry := range entries {
			if entry.UserID != user.ID {
				filtered = append(filtered, entry)
			}
		}
		newEntry = models.CalendarScheduleEntry{
			UserID:          user.ID,
			WorkspaceID:     req.WorkspaceID,
			ScheduledDate:   req.ScheduledDate,
			ScheduledTime:   req.ScheduledTime,
			DurationMinutes: req.DurationMinutes,
			Notes:           req.Notes,
			CreatedAt:       time.Now().UTC().Format(time.RFC3339),
		}
		return append(filtered, newEntry), nil
	})
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "item")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, map[string]any{
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

	repo := repository.NewItemRepository(h.db)

	// Check if user has permission to edit items in this workspace.
	workspaceID, err := repo.GetWorkspaceIDCtx(r.Context(), id)
	if err != nil {
		respondNotFound(w, r, "item")
		return
	}
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

	// Remove this user's entry in one transaction so a concurrent schedule for
	// another user is not lost.
	err = repo.MutateCalendarData(r.Context(), id, func(entries []models.CalendarScheduleEntry) ([]models.CalendarScheduleEntry, error) {
		filtered := make([]models.CalendarScheduleEntry, 0, len(entries))
		found := false
		for _, entry := range entries {
			if entry.UserID != userID {
				filtered = append(filtered, entry)
			} else {
				found = true
			}
		}
		if !found {
			return nil, repository.ErrNotFound
		}
		return filtered, nil
	})
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "schedule")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, map[string]string{
		"status":  "success",
		"message": "Item unscheduled successfully",
	})
}

// excludePersonalQuery reports whether the caller opted out of
// personal-workspace items via the exclude_personal query parameter — same
// contract as the bearer-token v1 API (restapi/v1/handlers.ExcludePersonal).
func excludePersonalQuery(r *http.Request) bool {
	v := r.URL.Query().Get("exclude_personal")
	return v == "true" || v == "1"
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
		respondJSONOK(w, map[string][]map[string]any{})
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

	excludePersonal := excludePersonalQuery(r)
	for _, result := range itemsWithCalendar {
		if excludePersonal && result.IsPersonal {
			continue
		}
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
	scheduledItems := make(map[string][]map[string]any)

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
				scheduledItems[entry.ScheduledDate] = []map[string]any{}
			}

			itemWithSchedule := map[string]any{
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
