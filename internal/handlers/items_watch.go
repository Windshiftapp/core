package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

// AddWatch handles POST /api/items/{id}/watch - adds a watch to an item
func (h *ItemHandler) AddWatch(w http.ResponseWriter, r *http.Request) {
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

	// Get item's workspace_id for permission check
	var workspaceID int
	err := h.db.QueryRow("SELECT workspace_id FROM items WHERE id = ?", itemID).Scan(&workspaceID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "item")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Check if user has permission to view this item
	canView, permErr := h.canViewItem(user.ID, workspaceID)
	if permErr != nil {
		respondInternalError(w, r, permErr)
		return
	}
	if !canView {
		respondNotFound(w, r, "item")
		return
	}

	// Parse optional reason from request body
	var reqBody struct {
		Reason string `json:"reason"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&reqBody)
	}
	if reqBody.Reason == "" {
		reqBody.Reason = "User subscribed to item notifications"
	}

	// Add watch using ActivityTracker
	if h.activityTracker != nil {
		if err := h.activityTracker.AddWatch(user.ID, itemID, reqBody.Reason); err != nil {
			slog.Error("error adding watch", slog.String("component", "items_watch"), slog.Int("user_id", user.ID), slog.Int("item_id", itemID), slog.Any("error", err))
			respondInternalError(w, r, err)
			return
		}
	} else {
		respondInternalError(w, r, errors.New("activity tracker not available"))
		return
	}

	respondJSONOK(w, map[string]interface{}{
		"success":  true,
		"watching": true,
		"message":  "Successfully added watch to item",
	})
}

// RemoveWatch handles DELETE /api/items/{id}/watch - removes a watch from an item
func (h *ItemHandler) RemoveWatch(w http.ResponseWriter, r *http.Request) {
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

	// Get item's workspace_id for permission check
	var workspaceID int
	err := h.db.QueryRow("SELECT workspace_id FROM items WHERE id = ?", itemID).Scan(&workspaceID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "item")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Check if user has permission to view this item
	canView, permErr := h.canViewItem(user.ID, workspaceID)
	if permErr != nil {
		respondInternalError(w, r, permErr)
		return
	}
	if !canView {
		respondNotFound(w, r, "item")
		return
	}

	// Remove watch using ActivityTracker
	if h.activityTracker != nil {
		if err := h.activityTracker.RemoveWatch(user.ID, itemID); err != nil {
			slog.Error("error removing watch", slog.String("component", "items_watch"), slog.Int("user_id", user.ID), slog.Int("item_id", itemID), slog.Any("error", err))
			respondInternalError(w, r, err)
			return
		}
	} else {
		respondInternalError(w, r, errors.New("activity tracker not available"))
		return
	}

	respondJSONOK(w, map[string]interface{}{
		"success":  true,
		"watching": false,
		"message":  "Successfully removed watch from item",
	})
}

// GetWatchStatus handles GET /api/items/{id}/watch - checks if user is watching an item
func (h *ItemHandler) GetWatchStatus(w http.ResponseWriter, r *http.Request) {
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

	// Check watch status using ActivityTracker
	var isWatching bool
	var err error
	if h.activityTracker != nil {
		isWatching, err = h.activityTracker.IsWatching(user.ID, itemID)
		if err != nil {
			slog.Error("error checking watch status", slog.String("component", "items_watch"), slog.Int("user_id", user.ID), slog.Int("item_id", itemID), slog.Any("error", err))
			respondInternalError(w, r, err)
			return
		}
	} else {
		respondInternalError(w, r, errors.New("activity tracker not available"))
		return
	}

	respondJSONOK(w, map[string]interface{}{
		"watching": isWatching,
		"item_id":  itemID,
	})
}
