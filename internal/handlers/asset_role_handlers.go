package handlers

import (
	"database/sql"
	"net/http"

	"windshift/internal/models"
)

// GetAssetRoles returns all available asset roles
func (h *AssetHandler) GetAssetRoles(w http.ResponseWriter, r *http.Request) {
	_, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	rows, err := h.db.Query(`
		SELECT id, name, description, is_system, display_order, created_at, updated_at
		FROM asset_roles
		ORDER BY display_order
	`)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var roles []models.AssetRole
	for rows.Next() {
		var role models.AssetRole
		err := rows.Scan(&role.ID, &role.Name, &role.Description, &role.IsSystem, &role.DisplayOrder, &role.CreatedAt, &role.UpdatedAt)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		roles = append(roles, role)
	}

	respondJSONOK(w, roles)
}

// GetAssetRole returns a single asset role with its permissions
func (h *AssetHandler) GetAssetRole(w http.ResponseWriter, r *http.Request) {
	if _, ok := RequireAuth(w, r); !ok {
		return
	}

	roleID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var err error

	var role models.AssetRole
	err = h.db.QueryRow(`
		SELECT id, name, description, is_system, display_order, created_at, updated_at
		FROM asset_roles WHERE id = ?
	`, roleID).Scan(&role.ID, &role.Name, &role.Description, &role.IsSystem, &role.DisplayOrder, &role.CreatedAt, &role.UpdatedAt)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "Role")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
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
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = permRows.Close() }()

	for permRows.Next() {
		var perm models.AssetPermission
		err := permRows.Scan(&perm.ID, &perm.PermissionKey, &perm.PermissionName, &perm.Description, &perm.CreatedAt)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		role.Permissions = append(role.Permissions, perm)
	}

	respondJSONOK(w, role)
}
