package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"windshift/internal/logger"
)

// GetEveryoneRole returns the everyone default role for a set
func (h *AssetHandler) GetEveryoneRole(w http.ResponseWriter, r *http.Request) {
	_, setID, ok := h.requireSetAdminByID(w, r)
	if !ok {
		return
	}

	everyoneRole, err := h.queryEveryoneRole(setID)
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

	now := time.Now()

	var err error
	if req.RoleID == nil {
		// Remove everyone role (delete row if exists)
		_, err = h.db.ExecWrite("DELETE FROM asset_set_everyone_roles WHERE set_id = ?", setID)
	} else {
		// Validate role exists
		var roleExists bool
		err = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM asset_roles WHERE id = ?)", *req.RoleID).Scan(&roleExists)
		if err != nil || !roleExists {
			respondInvalidID(w, r, "role ID")
			return
		}

		// Upsert everyone role
		_, err = h.db.ExecWrite(`
			INSERT INTO asset_set_everyone_roles (set_id, role_id, granted_by, granted_at)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(set_id) DO UPDATE SET role_id = excluded.role_id, granted_by = excluded.granted_by, granted_at = excluded.granted_at
		`, setID, *req.RoleID, currentUser.ID, now)
	}

	if err != nil {
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
