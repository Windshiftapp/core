package handlers

import (
	"net/http"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
)

// RequesterOpenTickets lists the requester's other open tickets for the
// duplicate-candidates panel (WI-1548): when intake creates a fresh ticket
// from a sender who already has open tickets, agents see the candidates and
// merge manually instead of the conversation staying siloed. Computed on
// read — no candidate state is stored.
func (h *ItemHandler) RequesterOpenTickets(w http.ResponseWriter, r *http.Request) {
	itemID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	itemRepo := repository.NewItemRepository(h.db)
	workspaceID, err := itemRepo.GetWorkspaceIDCtx(r.Context(), itemID)
	if err != nil {
		respondNotFound(w, r, "Item")
		return
	}
	allowed, err := h.permissionService.HasWorkspacePermission(user.ID, workspaceID, models.PermissionItemView)
	if err != nil || !allowed {
		// 404, not 403: item existence is not disclosed to non-viewers.
		respondNotFound(w, r, "Item")
		return
	}

	requesterCustomerID, err := itemRepo.RequesterCustomerID(itemID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if requesterCustomerID == nil {
		// No requester (internal creator): no candidates.
		respondJSONOK(w, []map[string]any{})
		return
	}

	rows, err := itemRepo.RequesterOpenTickets(*requesterCustomerID, itemID, 20)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Candidates may live in other workspaces; each is filtered to
	// workspaces the caller can view so cross-workspace candidates never
	// leak.
	type candidate struct {
		ID                      int    `json:"id"`
		WorkspaceID             int    `json:"workspace_id"`
		WorkspaceKey            string `json:"workspace_key"`
		WorkspaceItemNumber     int    `json:"workspace_item_number"`
		Title                   string `json:"title"`
		StatusName              string `json:"status_name"`
		UpdatedAt               string `json:"updated_at"`
		CreatorPortalCustomerID *int   `json:"creator_portal_customer_id,omitempty"`
	}
	candidates := []candidate{}
	checked := map[int]bool{}
	allowedWorkspaces := map[int]bool{}
	for _, row := range rows {
		if !checked[row.WorkspaceID] {
			viewable, err := h.permissionService.HasWorkspacePermission(user.ID, row.WorkspaceID, models.PermissionItemView)
			if err != nil {
				respondInternalError(w, r, err)
				return
			}
			checked[row.WorkspaceID] = true
			allowedWorkspaces[row.WorkspaceID] = viewable
		}
		if !allowedWorkspaces[row.WorkspaceID] {
			continue
		}
		candidates = append(candidates, candidate{
			ID:                      row.ID,
			WorkspaceID:             row.WorkspaceID,
			WorkspaceKey:            row.WorkspaceKey,
			WorkspaceItemNumber:     row.WorkspaceItemNumber,
			Title:                   row.Title,
			StatusName:              row.StatusName,
			UpdatedAt:               row.UpdatedAt.UTC().Format(time.RFC3339),
			CreatorPortalCustomerID: requesterCustomerID,
		})
	}

	respondJSONOK(w, candidates)
}
