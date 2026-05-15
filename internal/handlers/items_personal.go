package handlers

import (
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

	itemRepo := repository.NewItemRepository(h.db)

	// Verify the work item exists and check view permission
	workItemWorkspaceID, err := itemRepo.GetWorkspaceID(workItemID)
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
	personalWorkspaceID, err := repository.NewWorkspaceRepository(h.db).GetActivePersonalWorkspaceID(user.ID)
	if errors.Is(err, repository.ErrNotFound) {
		// User has no personal workspace, return empty list
		respondJSONOK(w, []models.Item{})
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	items, err := itemRepo.ListRelatedPersonalItems(workItemID, personalWorkspaceID)
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

	itemRepo := repository.NewItemRepository(h.db)

	// Verify the item exists and belongs to user's personal workspace
	ownership, err := itemRepo.GetItemWorkspaceOwnership(itemID)
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
	if err := itemRepo.ClearRelatedWorkItem(itemID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, map[string]interface{}{
		"success": true,
		"message": "Successfully removed work item relationship",
	})
}
