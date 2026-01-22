package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
	"windshift/internal/models"
	"windshift/internal/utils"
)

// GetEveryoneRole returns the everyone default role for a set
func (h *AssetHandler) GetEveryoneRole(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	setID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid set ID", http.StatusBadRequest)
		return
	}

	// Check admin permission
	canAdmin, err := h.canAdminSet(currentUser.ID, setID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !canAdmin {
		http.Error(w, "Admin permission required", http.StatusForbidden)
		return
	}

	var everyoneRole models.AssetSetEveryoneRole
	var roleID sql.NullInt64
	var grantedBy sql.NullInt64
	var roleName, grantedByName sql.NullString

	err = h.db.QueryRow(`
		SELECT aser.set_id, aser.role_id, aser.granted_by, aser.granted_at,
		       ar.name as role_name,
		       COALESCE(u.first_name || ' ' || u.last_name, u.username, '') as granted_by_name
		FROM asset_set_everyone_roles aser
		LEFT JOIN asset_roles ar ON aser.role_id = ar.id
		LEFT JOIN users u ON aser.granted_by = u.id
		WHERE aser.set_id = ?
	`, setID).Scan(&everyoneRole.SetID, &roleID, &grantedBy, &everyoneRole.GrantedAt, &roleName, &grantedByName)

	if err == sql.ErrNoRows {
		// No everyone role configured - return null
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(nil)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	everyoneRole.RoleID = utils.NullInt64ToPtr(roleID)
	everyoneRole.GrantedBy = utils.NullInt64ToPtr(grantedBy)
	everyoneRole.RoleName = roleName.String
	everyoneRole.GrantedByName = grantedByName.String

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(everyoneRole)
}

// SetEveryoneRoleRequest represents the request body for setting everyone role
type SetEveryoneRoleRequest struct {
	RoleID *int `json:"role_id"` // null to remove everyone access
}

// SetEveryoneRole sets or removes the everyone default role for a set
func (h *AssetHandler) SetEveryoneRole(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	setID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid set ID", http.StatusBadRequest)
		return
	}

	// Check admin permission
	canAdmin, err := h.canAdminSet(currentUser.ID, setID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !canAdmin {
		http.Error(w, "Admin permission required", http.StatusForbidden)
		return
	}

	var req SetEveryoneRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	now := time.Now()

	if req.RoleID == nil {
		// Remove everyone role (delete row if exists)
		_, err = h.db.ExecWrite("DELETE FROM asset_set_everyone_roles WHERE set_id = ?", setID)
	} else {
		// Validate role exists
		var roleExists bool
		err = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM asset_roles WHERE id = ?)", *req.RoleID).Scan(&roleExists)
		if err != nil || !roleExists {
			http.Error(w, "Invalid role ID", http.StatusBadRequest)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
