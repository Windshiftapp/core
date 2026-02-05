package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"windshift/internal/models"
)

// GetPersonalTasks handles GET /api/items/{id}/personal-tasks - returns personal tasks related to a work item
func (h *ItemHandler) GetPersonalTasks(w http.ResponseWriter, r *http.Request) {
	workItemID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Require authentication
	user := h.getUserFromContext(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	// Verify the work item exists
	var workItemWorkspaceID int
	err := h.db.QueryRow("SELECT workspace_id FROM items WHERE id = ?", workItemID).Scan(&workItemWorkspaceID)
	if err != nil {
		respondForbidden(w, r)
		return
	}

	// Get user's personal workspace
	var personalWorkspaceID *int
	err = h.db.QueryRow(`
		SELECT id FROM workspaces
		WHERE is_personal = ? AND owner_id = ? AND active = ?
	`, true, user.ID, true).Scan(&personalWorkspaceID)

	if err == sql.ErrNoRows {
		// User has no personal workspace, return empty list
		respondJSONOK(w, []models.Item{})
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Query personal tasks related to this work item
	query := `
		SELECT
			i.id, i.workspace_id, i.workspace_item_number, i.item_type_id, i.title, i.description,
			i.status_id, i.priority_id, i.is_task, i.milestone_id,
			i.project_id, i.inherit_project, i.time_project_id, i.assignee_id, i.creator_id,
			i.calendar_data, i.parent_id,
			i.frac_index, i.related_work_item_id,
			i.created_at, i.updated_at,
			w.name as workspace_name, w.key as workspace_key,
			it.name as item_type_name,
			st.name as status_name,
			pri.name as priority_name, pri.icon as priority_icon, pri.color as priority_color,
			assignee.first_name || ' ' || assignee.last_name as assignee_name,
			assignee.email as assignee_email,
			assignee.avatar_url as assignee_avatar
		FROM items i
		LEFT JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN item_types it ON i.item_type_id = it.id
		LEFT JOIN statuses st ON i.status_id = st.id
		LEFT JOIN priorities pri ON i.priority_id = pri.id
		LEFT JOIN users assignee ON i.assignee_id = assignee.id
		WHERE i.related_work_item_id = ? AND i.workspace_id = ?
		ORDER BY i.created_at DESC
	`

	rows, err := h.db.Query(query, workItemID, personalWorkspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var items []models.Item
	for rows.Next() {
		var item models.Item
		var calendarDataJSON sql.NullString
		var itemTypeName, statusName, priorityName, priorityIcon, priorityColor sql.NullString
		var assigneeName, assigneeEmail, assigneeAvatar sql.NullString

		err = rows.Scan(
			&item.ID, &item.WorkspaceID, &item.WorkspaceItemNumber, &item.ItemTypeID, &item.Title, &item.Description,
			&item.StatusID, &item.PriorityID, &item.IsTask, &item.MilestoneID,
			&item.ProjectID, &item.InheritProject, &item.TimeProjectID, &item.AssigneeID, &item.CreatorID,
			&calendarDataJSON, &item.ParentID,
			&item.FracIndex, &item.RelatedWorkItemID,
			&item.CreatedAt, &item.UpdatedAt,
			&item.WorkspaceName, &item.WorkspaceKey,
			&itemTypeName,
			&statusName,
			&priorityName, &priorityIcon, &priorityColor,
			&assigneeName, &assigneeEmail, &assigneeAvatar,
		)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		// Handle nullable fields from LEFT JOINs
		if itemTypeName.Valid {
			item.ItemTypeName = itemTypeName.String
		}
		if statusName.Valid {
			item.StatusName = statusName.String
		}
		if priorityName.Valid {
			item.PriorityName = priorityName.String
		}
		if priorityIcon.Valid {
			item.PriorityIcon = priorityIcon.String
		}
		if priorityColor.Valid {
			item.PriorityColor = priorityColor.String
		}
		if assigneeName.Valid {
			item.AssigneeName = assigneeName.String
		}
		if assigneeEmail.Valid {
			item.AssigneeEmail = assigneeEmail.String
		}
		if assigneeAvatar.Valid {
			item.AssigneeAvatar = assigneeAvatar.String
		}

		// Parse calendar data JSON
		if calendarDataJSON.Valid && calendarDataJSON.String != "" {
			if err = json.Unmarshal([]byte(calendarDataJSON.String), &item.CalendarData); err != nil {
				item.CalendarData = []models.CalendarScheduleEntry{}
			}
		} else {
			item.CalendarData = []models.CalendarScheduleEntry{}
		}

		items = append(items, item)
	}

	if err = rows.Err(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Return empty array if no results
	if items == nil {
		items = []models.Item{}
	}

	respondJSONOK(w, items)
}

// RemoveRelatedWorkItem handles DELETE /api/items/{id}/related-work-item - removes the relationship
func (h *ItemHandler) RemoveRelatedWorkItem(w http.ResponseWriter, r *http.Request) {
	itemID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Require authentication
	user := h.getUserFromContext(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	// Verify the item exists and belongs to user's personal workspace
	var workspaceID int
	var isPersonal bool
	var ownerID *int
	err := h.db.QueryRow(`
		SELECT i.workspace_id, w.is_personal, w.owner_id
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		WHERE i.id = ?
	`, itemID).Scan(&workspaceID, &isPersonal, &ownerID)

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "item")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Verify it's a personal workspace item owned by the current user
	if !isPersonal || ownerID == nil || *ownerID != user.ID {
		respondForbidden(w, r)
		return
	}

	// Remove the relationship
	_, err = h.db.Exec(`
		UPDATE items
		SET related_work_item_id = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, itemID)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, map[string]interface{}{
		"success": true,
		"message": "Successfully removed work item relationship",
	})
}
