package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"windshift/internal/models"
	"windshift/internal/utils"
)

// GetAssetRoles returns all available asset roles
func (h *AssetHandler) GetAssetRoles(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	rows, err := h.db.Query(`
		SELECT id, name, description, is_system, display_order, created_at, updated_at
		FROM asset_roles
		ORDER BY display_order
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var roles []models.AssetRole
	for rows.Next() {
		var role models.AssetRole
		err := rows.Scan(&role.ID, &role.Name, &role.Description, &role.IsSystem, &role.DisplayOrder, &role.CreatedAt, &role.UpdatedAt)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		roles = append(roles, role)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(roles)
}

// GetAssetRole returns a single asset role with its permissions
func (h *AssetHandler) GetAssetRole(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	roleID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid role ID", http.StatusBadRequest)
		return
	}

	var role models.AssetRole
	err = h.db.QueryRow(`
		SELECT id, name, description, is_system, display_order, created_at, updated_at
		FROM asset_roles WHERE id = ?
	`, roleID).Scan(&role.ID, &role.Name, &role.Description, &role.IsSystem, &role.DisplayOrder, &role.CreatedAt, &role.UpdatedAt)
	if err == sql.ErrNoRows {
		http.Error(w, "Role not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get permissions for this role
	permRows, err := h.db.Query(`
		SELECT ap.id, ap.permission_key, ap.permission_name, ap.description, ap.created_at
		FROM asset_role_permissions arp
		JOIN asset_permissions ap ON arp.permission_id = ap.id
		WHERE arp.role_id = ?
	`, roleID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer permRows.Close()

	for permRows.Next() {
		var perm models.AssetPermission
		err := permRows.Scan(&perm.ID, &perm.PermissionKey, &perm.PermissionName, &perm.Description, &perm.CreatedAt)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		role.Permissions = append(role.Permissions, perm)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(role)
}
