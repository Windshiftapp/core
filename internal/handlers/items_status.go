package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

type ItemTransitionSummary struct {
	CurrentStatus        string                           `json:"current_status"`
	AvailableTransitions []map[string]any                 `json:"available_transitions"`
	PendingApproval      *services.PendingApprovalSummary `json:"pending_approval"`
}

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
	// Check if user has permission to view this item's workspace
	canView, permErr := h.canViewItem(user.ID, item.WorkspaceID)
	if permErr != nil {
		respondInternalError(w, r, permErr)
		return
	}
	if !canView {
		respondNotFound(w, r, "Item")
		return
	}

	response, err := h.loadAvailableStatusTransitions(r.Context(), user.ID, item)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, response)
}

func (h *ItemHandler) loadAvailableStatusTransitions(ctx context.Context, userID int, item *models.Item) (ItemTransitionSummary, error) {
	response := ItemTransitionSummary{AvailableTransitions: []map[string]any{}}
	workspaceID := item.WorkspaceID
	currentStatusID := item.StatusID
	itemTypeIDPtr := item.ItemTypeID
	itemID := item.ID
	workflowService := services.NewWorkflowService(h.db)

	// Get current status name for response
	if currentStatusID != nil {
		currentStatusName, err := workflowService.GetStatusName(int64(*currentStatusID))
		if err != nil {
			return ItemTransitionSummary{}, err
		}
		response.CurrentStatus = currentStatusName
	}

	// Get the workflow using WorkflowService (considers item type override)
	workflowID, err := workflowService.GetWorkflowIDForItem(workspaceID, itemTypeIDPtr)
	if err != nil {
		return ItemTransitionSummary{}, err
	}

	// No workflow configured - return empty transitions
	if workflowID == nil {
		return response, nil
	}

	// Build the list of available transitions
	// Always include current status first
	if currentStatusID != nil {
		currentOption, optionErr := workflowService.GetStatusTransitionOption(int64(*currentStatusID))
		if optionErr != nil {
			return ItemTransitionSummary{}, optionErr
		}
		if currentOption != nil {
			response.AvailableTransitions = append(response.AvailableTransitions, transitionOptionResponse(*currentOption))
		}
	}

	// Get valid transitions from current status
	if currentStatusID != nil {
		rawTransitions, listErr := workflowService.ListAvailableTransitionOptions(*workflowID, int64(*currentStatusID))
		if listErr != nil {
			return ItemTransitionSummary{}, listErr
		}

		// Apply approval gating: drop transitions whose ID is the approve or
		// deny target of an in-flight approval on this item.
		if h.approvalService != nil {
			gatedIDs, summary, gErr := h.approvalService.GetGatedTransitionsForItem(ctx, itemID, userID)
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
					if !gated[rt.TransitionID] {
						kept = append(kept, rt)
					}
				}
				rawTransitions = kept
			}
			response.PendingApproval = summary
		}

		// Apply condition filtering if condition service is available
		if h.conditionService != nil {
			conditionSetID, csErr := h.conditionService.GetConditionSetIDForItem(workspaceID, itemTypeIDPtr)
			if csErr == nil && conditionSetID != nil {
				// Build item context for condition evaluation
				itemCtx := services.BuildItemContextFromIDs(h.db, itemID, workspaceID, currentStatusID, itemTypeIDPtr)

				// Convert to TransitionWithID for filtering
				var twids []services.TransitionWithID
				for _, rt := range rawTransitions {
					color := ""
					if rt.CategoryColor != nil {
						color = *rt.CategoryColor
					}
					twids = append(twids, services.TransitionWithID{
						TransitionID:  rt.TransitionID,
						StatusID:      rt.StatusID,
						BuiltinKey:    rt.BuiltinKey,
						StatusName:    rt.StatusName,
						CategoryColor: color,
					})
				}

				filtered, filterErr := h.conditionService.FilterTransitionsByConditions(
					ctx, *conditionSetID, twids, userID, itemCtx,
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
						var categoryColor *string
						if f.CategoryColor != "" {
							color := f.CategoryColor
							categoryColor = &color
						}
						rawTransitions = append(rawTransitions, services.StatusTransitionOption{
							TransitionID:  f.TransitionID,
							StatusID:      f.StatusID,
							BuiltinKey:    f.BuiltinKey,
							StatusName:    f.StatusName,
							CategoryColor: categoryColor,
						})
					}
				}
			}
		}

		// Track IDs we've already added to avoid duplicates.
		// currentStatusID is non-nil in this block.
		addedIDs := map[int]bool{*currentStatusID: true}

		for _, rt := range rawTransitions {
			if !addedIDs[rt.StatusID] {
				response.AvailableTransitions = append(response.AvailableTransitions, transitionOptionResponse(rt))
				addedIDs[rt.StatusID] = true
			}
		}
	}

	return response, nil
}

// GetWorkspaceTransitionMatrix returns the allowed status transitions for every
// (item_type_id, status_id) pair in a workspace, keyed "<itemTypeID>:<statusID>".
// It backs the board's transition preload, which otherwise fired one
// GET /items/{id}/available-status-transitions per unique (item type, status)
// pair — many concurrent requests on a board spanning several types/statuses.
//
// The matrix is for DISPLAY of candidate transitions only: it deliberately
// omits per-item approval gating and condition filtering (both item-specific),
// which still apply when an actual transition is performed. Each pair's value
// matches the per-item endpoint's available_transitions shape (current status
// first, then reachable statuses).
func (h *ItemHandler) GetWorkspaceTransitionMatrix(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	canView, permErr := h.canViewItem(user.ID, workspaceID)
	if permErr != nil {
		respondInternalError(w, r, permErr)
		return
	}
	if !canView {
		respondNotFound(w, r, "Workspace")
		return
	}

	if h.transitionMatrix == nil {
		h.transitionMatrix = services.NewTransitionMatrixService(h.db)
	}
	ctx, cancel := h.requestDBContext(r)
	defer cancel()
	matrix, err := h.transitionMatrix.Load(ctx, workspaceID)
	if err != nil {
		h.respondItemReadError(w, r, err)
		return
	}
	transitions := map[string][]map[string]any{}
	for itemTypeID, byStatus := range matrix.ByItemType {
		for statusID, rawOptions := range byStatus {
			options := make([]map[string]any, 0, len(rawOptions))
			for _, option := range rawOptions {
				options = append(options, transitionOptionResponse(option))
			}
			transitions[strconv.Itoa(itemTypeID)+":"+strconv.Itoa(statusID)] = options
		}
	}

	response := map[string]any{"transitions": transitions}
	responseBytes := 0
	if encoded, marshalErr := json.Marshal(response); marshalErr == nil {
		// respondJSONOK uses json.Encoder, which appends one trailing newline.
		responseBytes = len(encoded) + 1
		h.transitionMatrix.ObserveResponseSize(responseBytes)
	}
	slog.Debug("workspace transition matrix loaded",
		"component", "transition_matrix",
		"workspace_id", workspaceID,
		"item_type_count", matrix.ItemTypeCount,
		"status_count", matrix.StatusCount,
		"workflow_count", matrix.WorkflowCount,
		"sql_count", matrix.SQLCount,
		"query_duration_ms", matrix.QueryDuration.Milliseconds(),
		"response_bytes", responseBytes,
	)
	respondJSONOK(w, response)
}

func transitionOptionResponse(option services.StatusTransitionOption) map[string]any {
	transition := map[string]any{
		"id":          option.StatusID,
		"builtin_key": option.BuiltinKey,
		"name":        option.StatusName,
		"value":       strings.ToLower(strings.ReplaceAll(option.StatusName, " ", "_")),
	}
	if option.CategoryColor != nil {
		transition["category_color"] = *option.CategoryColor
	}
	return transition
}
