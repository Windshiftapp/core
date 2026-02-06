package handlers

import (
	"database/sql"
	"net/http"
	"strings"

	"windshift/internal/services"
)

// GetAvailableStatusTransitions returns the valid status transitions for a work item
func (h *ItemHandler) GetAvailableStatusTransitions(w http.ResponseWriter, r *http.Request) {
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

	// Get the item to find its current status, workspace, and item type
	var currentStatusID sql.NullInt64
	var workspaceID int
	var itemTypeID sql.NullInt64

	err := h.db.QueryRow(`
		SELECT status_id, workspace_id, item_type_id
		FROM items
		WHERE id = ?
	`, itemID).Scan(&currentStatusID, &workspaceID, &itemTypeID)

	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "item")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Check if user has permission to view this item's workspace
	canView, permErr := h.canViewItem(user.ID, workspaceID)
	if permErr != nil {
		respondInternalError(w, r, permErr)
		return
	}
	if !canView {
		respondNotFound(w, r, "Item")
		return
	}

	// Get current status name for response
	var currentStatusName string
	if currentStatusID.Valid {
		_ = h.db.QueryRow(`SELECT name FROM statuses WHERE id = ?`, currentStatusID.Int64).Scan(&currentStatusName)
	}

	// Get the workflow using WorkflowService (considers item type override)
	workflowService := services.NewWorkflowService(h.db)
	var itemTypeIDPtr *int
	if itemTypeID.Valid {
		itemTypeIDInt := int(itemTypeID.Int64)
		itemTypeIDPtr = &itemTypeIDInt
	}
	workflowID, err := workflowService.GetWorkflowIDForItem(workspaceID, itemTypeIDPtr)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// No workflow configured - return empty transitions
	if workflowID == nil {
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
	if currentStatusID.Valid {
		var statusName string
		var categoryColor sql.NullString
		err = h.db.QueryRow(`
			SELECT s.name, sc.color
			FROM statuses s
			LEFT JOIN status_categories sc ON s.category_id = sc.id
			WHERE s.id = ?
		`, currentStatusID.Int64).Scan(&statusName, &categoryColor)
		if err == nil {
			transition := map[string]interface{}{
				"id":    int(currentStatusID.Int64),
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
	if currentStatusID.Valid {
		rows, err := h.db.Query(`
			SELECT s.id, s.name, sc.color
			FROM workflow_transitions wt
			JOIN statuses s ON wt.to_status_id = s.id
			LEFT JOIN status_categories sc ON s.category_id = sc.id
			WHERE wt.workflow_id = ? AND wt.from_status_id = ?
		`, *workflowID, currentStatusID.Int64)

		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		defer func() { _ = rows.Close() }()

		// Track IDs we've already added to avoid duplicates
		addedIDs := map[int]bool{}
		if currentStatusID.Valid {
			addedIDs[int(currentStatusID.Int64)] = true
		}

		for rows.Next() {
			var statusID int
			var statusName string
			var categoryColor sql.NullString

			err := rows.Scan(&statusID, &statusName, &categoryColor)
			if err != nil {
				continue
			}

			// Don't add duplicates
			if !addedIDs[statusID] {
				transition := map[string]interface{}{
					"id":    statusID,
					"name":  statusName,
					"value": strings.ToLower(strings.ReplaceAll(statusName, " ", "_")),
				}
				if categoryColor.Valid {
					transition["category_color"] = categoryColor.String
				}
				availableTransitions = append(availableTransitions, transition)
				addedIDs[statusID] = true
			}
		}
	}

	response := map[string]interface{}{
		"current_status":        currentStatusName,
		"available_transitions": availableTransitions,
	}

	respondJSONOK(w, response)
}
