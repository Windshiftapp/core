package handlers

import (
	"encoding/json"
	"net/http"

	"windshift/internal/logger"
)

// GetEveryoneRole returns the everyone default role for a set
func (h *AssetHandler) GetEveryoneRole(w http.ResponseWriter, r *http.Request) {
	_, setID, ok := h.requireSetAdminByID(w, r)
	if !ok {
		return
	}

	everyoneRole, err := h.repo.GetEveryoneRoleDetailed(setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, everyoneRole)
}

// SetEveryoneRoleRequest represents the request body for setting everyone role
type SetEveryoneRoleRequest struct {
	RoleID *int `json:"role_id"` // null to remove everyone access
}

// SetEveryoneRole sets or removes the everyone default role for a set
func (h *AssetHandler) SetEveryoneRole(w http.ResponseWriter, r *http.Request) {
	currentUser, setID, ok := h.requireSetAdminByID(w, r)
	if !ok {
		return
	}

	req, ok := decodeJSON[SetEveryoneRoleRequest](w, r)
	if !ok {
		return
	}

	if req.RoleID != nil {
		exists, err := h.repo.AssetRoleExists(*req.RoleID)
		if err != nil || !exists {
			respondInvalidID(w, r, "role ID")
			return
		}
	}

	if err := h.repo.SetEveryoneRole(setID, req.RoleID, currentUser.ID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	actionType := logger.ActionAssetSetRoleRevoke
	if req.RoleID != nil {
		actionType = logger.ActionAssetSetRoleAssign
	}
	logAudit(h.db, r, currentUser, actionType, logger.ResourceAssetSetRole, &setID, "")

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
