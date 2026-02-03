package handlers

import (
	"database/sql"
	"net/http"
	"strings"
	"windshift/internal/services"
)

// GetAvailableStatusTransitions returns the valid status transitions for a work item
func (h *ItemHandler) GetAvailableStatusTransitions(w http.ResponseWriter, r *http.Request) {
	itemId, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Require authentication
	user := h.getUserFromContext(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	// Get the item to find its current status, workspace, and item type
	var currentStatusId sql.NullInt64
	var workspaceId int
	var itemTypeId sql.NullInt64

	err := h.db.QueryRow(`
		SELECT status_id, workspace_id, item_type_id
		FROM items
		WHERE id = ?
	`, itemId).Scan(&currentStatusId, &workspaceId, &itemTypeId)

	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "item")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Check if user has permission to view this item's workspace
	canView, permErr := h.canViewItem(user.ID, workspaceId)
	if permErr != nil {
		respondInternalError(w, r, permErr)
		return
	}
	if !canView {
		respondForbidden(w, r)
		return
	}

	// Get current status name for response
	var currentStatusName string
	if currentStatusId.Valid {
		h.db.QueryRow(`SELECT name FROM statuses WHERE id = ?`, currentStatusId.Int64).Scan(&currentStatusName)
	}

	// Get the workflow using WorkflowService (considers item type override)
	workflowService := services.NewWorkflowService(h.db)
	var itemTypeIdPtr *int
	if itemTypeId.Valid {
		itemTypeIdInt := int(itemTypeId.Int64)
		itemTypeIdPtr = &itemTypeIdInt
	}
	workflowId, err := workflowService.GetWorkflowIDForItem(workspaceId, itemTypeIdPtr)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// No workflow configured - return empty transitions
	if workflowId == nil {
		response := map[string]interface{}{
			"current_status":        currentStatusName,
			"available_transitions": []map[string]interface{}{},
		}
		respondJSONOK(w, response)
		return
	}

	// Build the list of available transitions
	availableTransitions := []map[string]interface{}{}

	// Always include current status first
	if currentStatusId.Valid {
		var statusName string
		var categoryColor sql.NullString
		err = h.db.QueryRow(`
			SELECT s.name, sc.color
			FROM statuses s
			LEFT JOIN status_categories sc ON s.category_id = sc.id
			WHERE s.id = ?
		`, currentStatusId.Int64).Scan(&statusName, &categoryColor)
		if err == nil {
			transition := map[string]interface{}{
				"id":    int(currentStatusId.Int64),
				"name":  statusName,
				"value": strings.ToLower(strings.ReplaceAll(statusName, " ", "_")),
			}
			if categoryColor.Valid {
				transition["category_color"] = categoryColor.String
			}
			availableTransitions = append(availableTransitions, transition)
		}
	}

	// Get valid transitions from current status
	if currentStatusId.Valid {
		rows, err := h.db.Query(`
			SELECT s.id, s.name, sc.color
			FROM workflow_transitions wt
			JOIN statuses s ON wt.to_status_id = s.id
			LEFT JOIN status_categories sc ON s.category_id = sc.id
			WHERE wt.workflow_id = ? AND wt.from_status_id = ?
		`, *workflowId, currentStatusId.Int64)

		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		defer rows.Close()

		// Track IDs we've already added to avoid duplicates
		addedIds := map[int]bool{}
		if currentStatusId.Valid {
			addedIds[int(currentStatusId.Int64)] = true
		}

		for rows.Next() {
			var statusId int
			var statusName string
			var categoryColor sql.NullString

			err := rows.Scan(&statusId, &statusName, &categoryColor)
			if err != nil {
				continue
			}

			// Don't add duplicates
			if !addedIds[statusId] {
				transition := map[string]interface{}{
					"id":    statusId,
					"name":  statusName,
					"value": strings.ToLower(strings.ReplaceAll(statusName, " ", "_")),
				}
				if categoryColor.Valid {
					transition["category_color"] = categoryColor.String
				}
				availableTransitions = append(availableTransitions, transition)
				addedIds[statusId] = true
			}
		}
	}

	response := map[string]interface{}{
		"current_status":        currentStatusName,
		"available_transitions": availableTransitions,
	}

	respondJSONOK(w, response)
}
