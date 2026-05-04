package handlers

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"windshift/internal/repository"
	"windshift/internal/services"
)

// GetAvailableStatusTransitions returns the valid status transitions for a work item
func (h *ItemHandler) GetAvailableStatusTransitions(w http.ResponseWriter, r *http.Request) {
	itemID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Require authentication
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Get the item to find its current status, workspace, and item type
	item, err := repository.NewItemRepository(h.db).FindByID(itemID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "item")
			return
		}
		respondInternalError(w, r, err)
		return
	}
	workspaceID := item.WorkspaceID
	currentStatusID := sql.NullInt64{}
	if item.StatusID != nil {
		currentStatusID = sql.NullInt64{Int64: int64(*item.StatusID), Valid: true}
	}
	itemTypeID := sql.NullInt64{}
	if item.ItemTypeID != nil {
		itemTypeID = sql.NullInt64{Int64: int64(*item.ItemTypeID), Valid: true}
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
	var pendingApproval *services.PendingApprovalSummary

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
			SELECT wt.id, s.id, s.name, sc.color
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

		// Collect transitions with their IDs for condition filtering
		type rawTransition struct {
			transitionID  int
			statusID      int
			statusName    string
			categoryColor sql.NullString
		}
		var rawTransitions []rawTransition

		for rows.Next() {
			var rt rawTransition
			if err := rows.Scan(&rt.transitionID, &rt.statusID, &rt.statusName, &rt.categoryColor); err != nil {
				continue
			}
			rawTransitions = append(rawTransitions, rt)
		}

		// Apply approval gating: drop transitions whose ID is the approve or
		// deny target of an in-flight approval on this item.
		if h.approvalService != nil {
			gatedIDs, summary, gErr := h.approvalService.GetGatedTransitionsForItem(itemID, user.ID)
			if gErr != nil {
				slog.Warn("approval gating lookup failed, returning unfiltered transitions",
					slog.Int("item_id", itemID),
					slog.Any("error", gErr))
			} else if len(gatedIDs) > 0 {
				gated := map[int]bool{}
				for _, id := range gatedIDs {
					gated[id] = true
				}
				kept := rawTransitions[:0]
				for _, rt := range rawTransitions {
					if !gated[rt.transitionID] {
						kept = append(kept, rt)
					}
				}
				rawTransitions = kept
			}
			pendingApproval = summary
		}

		// Apply condition filtering if condition service is available
		if h.conditionService != nil {
			conditionSetID, csErr := h.conditionService.GetConditionSetIDForItem(workspaceID, itemTypeIDPtr)
			if csErr == nil && conditionSetID != nil {
				// Build item context for condition evaluation
				itemCtx := services.BuildItemContext(h.db, itemID, workspaceID, currentStatusID, itemTypeID)

				// Convert to TransitionWithID for filtering
				var twids []services.TransitionWithID
				for _, rt := range rawTransitions {
					color := ""
					if rt.categoryColor.Valid {
						color = rt.categoryColor.String
					}
					twids = append(twids, services.TransitionWithID{
						TransitionID:  rt.transitionID,
						StatusID:      rt.statusID,
						StatusName:    rt.statusName,
						CategoryColor: color,
					})
				}

				filtered, filterErr := h.conditionService.FilterTransitionsByConditions(
					r.Context(), *conditionSetID, twids, user.ID, itemCtx,
				)
				if filterErr != nil {
					slog.Warn("condition filtering failed, returning unfiltered transitions",
						slog.Int("item_id", itemID),
						slog.Int("condition_set_id", *conditionSetID),
						slog.Any("error", filterErr))
				} else {
					// Rebuild rawTransitions from filtered results
					rawTransitions = nil
					for _, f := range filtered {
						var cc sql.NullString
						if f.CategoryColor != "" {
							cc = sql.NullString{String: f.CategoryColor, Valid: true}
						}
						rawTransitions = append(rawTransitions, rawTransition{
							transitionID:  f.TransitionID,
							statusID:      f.StatusID,
							statusName:    f.StatusName,
							categoryColor: cc,
						})
					}
				}
			}
		}

		// Track IDs we've already added to avoid duplicates
		addedIDs := map[int]bool{}
		if currentStatusID.Valid {
			addedIDs[int(currentStatusID.Int64)] = true
		}

		for _, rt := range rawTransitions {
			if !addedIDs[rt.statusID] {
				transition := map[string]interface{}{
					"id":    rt.statusID,
					"name":  rt.statusName,
					"value": strings.ToLower(strings.ReplaceAll(rt.statusName, " ", "_")),
				}
				if rt.categoryColor.Valid {
					transition["category_color"] = rt.categoryColor.String
				}
				availableTransitions = append(availableTransitions, transition)
				addedIDs[rt.statusID] = true
			}
		}
	}

	response := map[string]interface{}{
		"current_status":        currentStatusName,
		"available_transitions": availableTransitions,
		"pending_approval":      pendingApproval,
	}

	respondJSONOK(w, response)
}
