package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"windshift/internal/repository"
	"windshift/internal/services"
)

// transitionRequest is the JSON body accepted by POST /items/{id}/transition.
type transitionRequest struct {
	ToStatusID *int `json:"to_status_id"`
}

// Transition performs a workflow status transition on an item. Unlike the
// generic PUT /items/{id} path, this endpoint is gated by both validator-mode
// and condition-mode rules, so it cannot be used to bypass workflow conditions.
func (h *ItemHandler) Transition(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	var req transitionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondValidationError(w, r, err.Error())
		return
	}
	if req.ToStatusID == nil {
		respondValidationError(w, r, "to_status_id is required")
		return
	}

	itemRepo := repository.NewItemRepository(h.db)
	loadedItem, err := itemRepo.FindByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "item")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	canEdit, err := h.canEditItem(user.ID, loadedItem.WorkspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canEdit {
		respondNotFound(w, r, "Item")
		return
	}

	if h.activityTracker != nil {
		if err := h.activityTracker.TrackItemActivity(user.ID, id, services.ActivityEdit); err != nil {
			slog.Warn("failed to track item edit activity",
				slog.Int("user_id", user.ID),
				slog.Int("item_id", id),
				slog.Any("error", err))
		}
	}

	workflowService := services.NewWorkflowService(h.db)
	result, err := workflowService.PerformTransition(r.Context(), services.PerformTransitionRequest{
		ItemID:      id,
		ToStatusID:  *req.ToStatusID,
		ActorUserID: user.ID,
		Modes:       []string{"validator", "condition"},
	}, itemRepo, h.conditionService)
	if err != nil {
		if rej := services.IsTransitionRejection(err); rej != nil {
			respondValidationError(w, r, rej.Message)
			return
		}
		respondInternalError(w, r, err)
		return
	}

	if !result.NoOp && h.eventCoordinator != nil {
		h.eventCoordinator.EmitStatusChanged(result.Item, result.OldStatusID, result.NewStatusID, user.ID, user.Username)
	}

	if !result.NoOp && h.issueSyncService != nil && result.NewStatusID != nil {
		go func(statusID int) {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 30*time.Second)
			defer cancel()
			h.issueSyncService.PushStatusToGitHub(ctx, result.Item.ID, statusID)
		}(*result.NewStatusID)
	}

	respondJSONOK(w, map[string]interface{}{
		"item":          result.Item,
		"old_status_id": result.OldStatusID,
		"new_status_id": result.NewStatusID,
		"no_op":         result.NoOp,
	})
}
