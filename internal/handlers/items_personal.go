package handlers

import (
	"database/sql"
	"errors"
	"net/http"

	"windshift/internal/models"
	"windshift/internal/repository"
)

// GetPersonalTasks handles GET /api/items/{id}/personal-tasks - returns personal tasks related to a work item
func (h *ItemHandler) GetPersonalTasks(w http.ResponseWriter, r *http.Request) {
	workItemID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Require authentication
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Verify the work item exists and check view permission
	workItemWorkspaceID, err := repository.NewItemRepository(h.db).GetWorkspaceID(workItemID)
	if err != nil {
		respondNotFound(w, r, "Item")
		return
	}

	canView, permErr := h.canViewItem(user.ID, workItemWorkspaceID)
	if permErr != nil {
		respondInternalError(w, r, permErr)
		return
	}
	if !canView {
		respondNotFound(w, r, "Item")
		return
	}

	// Get user's personal workspace
	var personalWorkspaceID int
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

	items, err := repository.NewItemRepository(h.db).ListRelatedPersonalItems(workItemID, personalWorkspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
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
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Verify the item exists and belongs to user's personal workspace
	ownership, err := repository.NewItemRepository(h.db).GetItemWorkspaceOwnership(itemID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "item")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Verify it's a personal workspace item owned by the current user
	if !ownership.IsPersonal || ownership.OwnerID == nil || *ownership.OwnerID != user.ID {
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
