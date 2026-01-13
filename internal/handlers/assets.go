package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
	"windshift/internal/cql"
	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"

)

// AssetHandler handles asset management operations
type AssetHandler struct {
	db                database.Database
	permissionService *services.PermissionService
}

// NewAssetHandler creates a new asset handler
func NewAssetHandler(db database.Database, permissionService *services.PermissionService) *AssetHandler {
	return &AssetHandler{
		db:                db,
		permissionService: permissionService,
	}
}

// Asset permission key constants
const (
	AssetPermissionKeyView   = "asset.view"
	AssetPermissionKeyCreate = "asset.create"
	AssetPermissionKeyEdit   = "asset.edit"
	AssetPermissionKeyDelete = "asset.delete"
	AssetPermissionKeyAdmin  = "asset.admin"
)

// Role name constants
const (
	AssetRoleViewer        = "Viewer"
	AssetRoleEditor        = "Editor"
	AssetRoleAdministrator = "Administrator"
)

// createDefaultStatuses creates default statuses for a new asset set
func (h *AssetHandler) createDefaultStatuses(setID int) error {
	now := time.Now()
	defaultStatuses := []struct {
		Name         string
		Color        string
		IsDefault    bool
		DisplayOrder int
	}{
		{"Active", "#22c55e", true, 0},
		{"Inactive", "#6b7280", false, 1},
		{"Maintenance", "#f59e0b", false, 2},
		{"Retired", "#ef4444", false, 3},
	}

	for _, s := range defaultStatuses {
		_, err := h.db.ExecWrite(`
			INSERT INTO asset_statuses (set_id, name, color, is_default, display_order, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, setID, s.Name, s.Color, s.IsDefault, s.DisplayOrder, now, now)
		if err != nil {
			return err
		}
	}

	return nil
}

// getUserSetRole returns the role a user has for an asset set
// Priority: System Admin > Direct User Role > Group Role > Everyone Default
func (h *AssetHandler) getUserSetRole(userID, setID int) (*models.AssetRole, error) {
	// 1. Check if user is system admin - they have full access (virtual Administrator role)
	isAdmin, err := h.permissionService.HasGlobalPermission(userID, "system.admin")
	if err != nil {
		return nil, err
	}
	if isAdmin {
		// Return virtual Administrator role for system admins
		return &models.AssetRole{
			ID:   -1, // Virtual role
			Name: AssetRoleAdministrator,
		}, nil
	}

	// 2. Check direct user role (OVERRIDE - takes precedence)
	var role models.AssetRole
	err = h.db.QueryRow(`
		SELECT ar.id, ar.name, ar.description, ar.is_system, ar.display_order
		FROM user_asset_set_roles uasr
		JOIN asset_roles ar ON uasr.role_id = ar.id
		WHERE uasr.set_id = ? AND uasr.user_id = ?
	`, setID, userID).Scan(&role.ID, &role.Name, &role.Description, &role.IsSystem, &role.DisplayOrder)

	if err == nil {
		return &role, nil
	}
	if err != sql.ErrNoRows {
		return nil, err
	}

	// 3. Check group roles (OVERRIDE - get highest by display_order desc = most privileged)
	err = h.db.QueryRow(`
		SELECT ar.id, ar.name, ar.description, ar.is_system, ar.display_order
		FROM group_asset_set_roles gasr
		JOIN group_members gm ON gasr.group_id = gm.group_id
		JOIN asset_roles ar ON gasr.role_id = ar.id
		WHERE gasr.set_id = ? AND gm.user_id = ?
		ORDER BY ar.display_order DESC
		LIMIT 1
	`, setID, userID).Scan(&role.ID, &role.Name, &role.Description, &role.IsSystem, &role.DisplayOrder)

	if err == nil {
		return &role, nil
	}
	if err != sql.ErrNoRows {
		return nil, err
	}

	// 4. Check everyone default (FALLBACK)
	var roleID sql.NullInt64
	err = h.db.QueryRow(`
		SELECT role_id FROM asset_set_everyone_roles WHERE set_id = ?
	`, setID).Scan(&roleID)

	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	if err == sql.ErrNoRows || !roleID.Valid {
		// No everyone default configured
		return nil, nil
	}

	// Fetch the everyone role details
	err = h.db.QueryRow(`
		SELECT id, name, description, is_system, display_order
		FROM asset_roles WHERE id = ?
	`, roleID.Int64).Scan(&role.ID, &role.Name, &role.Description, &role.IsSystem, &role.DisplayOrder)

	if err != nil {
		return nil, err
	}

	return &role, nil
}

// roleHasPermission checks if a role has a specific permission
func (h *AssetHandler) roleHasPermission(roleID int, permissionKey string) (bool, error) {
	// Virtual admin role (-1) has all permissions
	if roleID == -1 {
		return true, nil
	}

	var count int
	err := h.db.QueryRow(`
		SELECT COUNT(*) FROM asset_role_permissions arp
		JOIN asset_permissions ap ON arp.permission_id = ap.id
		WHERE arp.role_id = ? AND ap.permission_key = ?
	`, roleID, permissionKey).Scan(&count)

	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// hasAssetPermission checks if a user has a specific asset permission for a set
func (h *AssetHandler) hasAssetPermission(userID, setID int, permissionKey string) (bool, error) {
	role, err := h.getUserSetRole(userID, setID)
	if err != nil {
		return false, err
	}
	if role == nil {
		return false, nil
	}
	return h.roleHasPermission(role.ID, permissionKey)
}

// getUserSetRoleName returns the role name (for API responses)
func (h *AssetHandler) getUserSetRoleName(userID, setID int) (string, error) {
	role, err := h.getUserSetRole(userID, setID)
	if err != nil {
		return "", err
	}
	if role == nil {
		return "", nil
	}
	return role.Name, nil
}

// canViewSet checks if user can view a set
func (h *AssetHandler) canViewSet(userID, setID int) (bool, error) {
	return h.hasAssetPermission(userID, setID, AssetPermissionKeyView)
}

// canEditSet checks if user can edit assets in a set
func (h *AssetHandler) canEditSet(userID, setID int) (bool, error) {
	return h.hasAssetPermission(userID, setID, AssetPermissionKeyEdit)
}

// canAdminSet checks if user can administer a set
func (h *AssetHandler) canAdminSet(userID, setID int) (bool, error) {
	return h.hasAssetPermission(userID, setID, AssetPermissionKeyAdmin)
}

// ============================================================================
// Asset Management Set Handlers
// ============================================================================

// GetAssetSets returns all asset sets the user has access to
func (h *AssetHandler) GetAssetSets(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	// Check if user is system admin
	isAdmin, _ := h.permissionService.HasGlobalPermission(currentUser.ID, "system.admin")

	query := `
		SELECT ams.id, ams.name, ams.description, ams.is_default,
		       ams.created_by, ams.created_at, ams.updated_at,
		       COALESCE(u.first_name || ' ' || u.last_name, u.username, '') as creator_name,
		       (SELECT COUNT(*) FROM asset_types WHERE set_id = ams.id) as asset_type_count,
		       (SELECT COUNT(*) FROM assets WHERE set_id = ams.id) as asset_count
		FROM asset_management_sets ams
		LEFT JOIN users u ON ams.created_by = u.id
	`

	var args []interface{}

	// System admins see all sets, others see only permitted sets
	if !isAdmin {
		query += ` WHERE (
			EXISTS (SELECT 1 FROM user_asset_set_roles WHERE set_id = ams.id AND user_id = ?)
			OR EXISTS (
				SELECT 1 FROM group_asset_set_roles gasr
				JOIN group_members gm ON gasr.group_id = gm.group_id
				WHERE gasr.set_id = ams.id AND gm.user_id = ?
			)
			OR EXISTS (SELECT 1 FROM asset_set_everyone_roles WHERE set_id = ams.id AND role_id IS NOT NULL)
		)`
		args = append(args, currentUser.ID, currentUser.ID)
	}

	query += ` ORDER BY ams.is_default DESC, ams.name`

	rows, err := h.db.Query(query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var sets []models.AssetManagementSet
	for rows.Next() {
		var set models.AssetManagementSet
		var creatorName sql.NullString
		var description sql.NullString

		err := rows.Scan(
			&set.ID, &set.Name, &description, &set.IsDefault,
			&set.CreatedBy, &set.CreatedAt, &set.UpdatedAt,
			&creatorName, &set.AssetTypeCount, &set.AssetCount,
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		set.CreatorName = creatorName.String
		set.Description = description.String

		// Get user's role for this set (stored as UserPermission for backwards compatibility)
		if isAdmin {
			set.UserPermission = AssetRoleAdministrator
		} else {
			set.UserPermission, _ = h.getUserSetRoleName(currentUser.ID, set.ID)
		}

		sets = append(sets, set)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sets)
}

// GetAssetSet returns a single asset set
func (h *AssetHandler) GetAssetSet(w http.ResponseWriter, r *http.Request) {
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

	// Check permission
	canView, err := h.canViewSet(currentUser.ID, setID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !canView {
		http.Error(w, "Access denied", http.StatusForbidden)
		return
	}

	var set models.AssetManagementSet
	var creatorName, description sql.NullString

	err = h.db.QueryRow(`
		SELECT ams.id, ams.name, ams.description, ams.is_default,
		       ams.created_by, ams.created_at, ams.updated_at,
		       COALESCE(u.first_name || ' ' || u.last_name, u.username, '') as creator_name,
		       (SELECT COUNT(*) FROM asset_types WHERE set_id = ams.id) as asset_type_count,
		       (SELECT COUNT(*) FROM assets WHERE set_id = ams.id) as asset_count
		FROM asset_management_sets ams
		LEFT JOIN users u ON ams.created_by = u.id
		WHERE ams.id = ?
	`, setID).Scan(
		&set.ID, &set.Name, &description, &set.IsDefault,
		&set.CreatedBy, &set.CreatedAt, &set.UpdatedAt,
		&creatorName, &set.AssetTypeCount, &set.AssetCount,
	)

	if err == sql.ErrNoRows {
		http.Error(w, "Set not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	set.CreatorName = creatorName.String
	set.Description = description.String

	set.UserPermission, _ = h.getUserSetRoleName(currentUser.ID, setID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(set)
}

// CreateAssetSetRequest represents the request body for creating an asset set
type CreateAssetSetRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsDefault   bool   `json:"is_default"`
}

// CreateAssetSet creates a new asset management set
func (h *AssetHandler) CreateAssetSet(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	// Check if user has asset.manage permission or is system admin
	hasPermission, err := h.permissionService.HasGlobalPermission(currentUser.ID, "system.admin")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !hasPermission {
		hasPermission, err = h.permissionService.HasGlobalPermission(currentUser.ID, "asset.manage")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if !hasPermission {
		http.Error(w, "Permission denied. Requires asset.manage or system.admin permission.", http.StatusForbidden)
		return
	}

	var req CreateAssetSetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}

	now := time.Now()

	// If this set is marked as default, unset any existing default
	if req.IsDefault {
		_, err := h.db.ExecWrite("UPDATE asset_management_sets SET is_default = false WHERE is_default = true")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	var setID int64
	err = h.db.QueryRow(`
		INSERT INTO asset_management_sets (name, description, is_default, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING id
	`, req.Name, req.Description, req.IsDefault, currentUser.ID, now, now).Scan(&setID)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Grant Administrator role to creator
	var adminRoleID int
	err = h.db.QueryRow(`SELECT id FROM asset_roles WHERE name = 'Administrator'`).Scan(&adminRoleID)
	if err != nil {
		http.Error(w, "Failed to find Administrator role", http.StatusInternalServerError)
		return
	}
	_, err = h.db.ExecWrite(`
		INSERT INTO user_asset_set_roles (set_id, user_id, role_id, granted_by, granted_at)
		VALUES (?, ?, ?, ?, ?)
	`, setID, currentUser.ID, adminRoleID, currentUser.ID, now)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Create default statuses for the new set
	if err := h.createDefaultStatuses(int(setID)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the created set
	set := models.AssetManagementSet{
		ID:             int(setID),
		Name:           req.Name,
		Description:    req.Description,
		IsDefault:      req.IsDefault,
		CreatedBy:      &currentUser.ID,
		CreatedAt:      now,
		UpdatedAt:      now,
		UserPermission: "Administrator",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(set)
}

// UpdateAssetSetRequest represents the request body for updating an asset set
type UpdateAssetSetRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsDefault   bool   `json:"is_default"`
}

// UpdateAssetSet updates an asset management set
func (h *AssetHandler) UpdateAssetSet(w http.ResponseWriter, r *http.Request) {
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

	var req UpdateAssetSetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}

	now := time.Now()

	// If this set is marked as default, unset any existing default
	if req.IsDefault {
		_, err := h.db.ExecWrite("UPDATE asset_management_sets SET is_default = false WHERE is_default = true AND id != ?", setID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	result, err := h.db.ExecWrite(`
		UPDATE asset_management_sets
		SET name = ?, description = ?, is_default = ?, updated_at = ?
		WHERE id = ?
	`, req.Name, req.Description, req.IsDefault, now, setID)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Set not found", http.StatusNotFound)
		return
	}

	// Return updated set
	var set models.AssetManagementSet
	h.db.QueryRow(`
		SELECT id, name, description, is_default, created_by, created_at, updated_at
		FROM asset_management_sets WHERE id = ?
	`, setID).Scan(&set.ID, &set.Name, &set.Description, &set.IsDefault, &set.CreatedBy, &set.CreatedAt, &set.UpdatedAt)

	set.UserPermission = "Administrator"

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(set)
}

// DeleteAssetSet deletes an asset management set
func (h *AssetHandler) DeleteAssetSet(w http.ResponseWriter, r *http.Request) {
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

	// Only system admins can delete sets
	isAdmin, err := h.permissionService.HasGlobalPermission(currentUser.ID, "system.admin")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !isAdmin {
		http.Error(w, "System admin permission required to delete sets", http.StatusForbidden)
		return
	}

	result, err := h.db.ExecWrite("DELETE FROM asset_management_sets WHERE id = ?", setID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Set not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ============================================================================
// Asset Role Handlers
// ============================================================================

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

// ============================================================================
// Asset Set Role Assignment Handlers
// ============================================================================

// GetSetRoles returns all role assignments for a set (users, groups, and everyone default)
func (h *AssetHandler) GetSetRoles(w http.ResponseWriter, r *http.Request) {
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

	// Get user role assignments
	userRoleRows, err := h.db.Query(`
		SELECT uasr.id, uasr.user_id, uasr.set_id, uasr.role_id, uasr.granted_by, uasr.granted_at,
		       COALESCE(u.first_name || ' ' || u.last_name, u.username, '') as user_name,
		       u.email as user_email,
		       ar.name as role_name,
		       COALESCE(g.first_name || ' ' || g.last_name, g.username, '') as granted_by_name
		FROM user_asset_set_roles uasr
		LEFT JOIN users u ON uasr.user_id = u.id
		LEFT JOIN asset_roles ar ON uasr.role_id = ar.id
		LEFT JOIN users g ON uasr.granted_by = g.id
		WHERE uasr.set_id = ?
		ORDER BY uasr.granted_at DESC
	`, setID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer userRoleRows.Close()

	var userRoles []models.UserAssetSetRole
	for userRoleRows.Next() {
		var role models.UserAssetSetRole
		var userName, userEmail, roleName, grantedByName sql.NullString

		err := userRoleRows.Scan(
			&role.ID, &role.UserID, &role.SetID, &role.RoleID, &role.GrantedBy, &role.GrantedAt,
			&userName, &userEmail, &roleName, &grantedByName,
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		role.UserName = userName.String
		role.UserEmail = userEmail.String
		role.RoleName = roleName.String
		role.GrantedByName = grantedByName.String

		userRoles = append(userRoles, role)
	}

	// Get group role assignments
	groupRoleRows, err := h.db.Query(`
		SELECT gasr.id, gasr.group_id, gasr.set_id, gasr.role_id, gasr.granted_by, gasr.granted_at,
		       g.name as group_name,
		       ar.name as role_name,
		       COALESCE(u.first_name || ' ' || u.last_name, u.username, '') as granted_by_name
		FROM group_asset_set_roles gasr
		LEFT JOIN groups g ON gasr.group_id = g.id
		LEFT JOIN asset_roles ar ON gasr.role_id = ar.id
		LEFT JOIN users u ON gasr.granted_by = u.id
		WHERE gasr.set_id = ?
		ORDER BY gasr.granted_at DESC
	`, setID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer groupRoleRows.Close()

	var groupRoles []models.GroupAssetSetRole
	for groupRoleRows.Next() {
		var role models.GroupAssetSetRole
		var groupName, roleName, grantedByName sql.NullString

		err := groupRoleRows.Scan(
			&role.ID, &role.GroupID, &role.SetID, &role.RoleID, &role.GrantedBy, &role.GrantedAt,
			&groupName, &roleName, &grantedByName,
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		role.GroupName = groupName.String
		role.RoleName = roleName.String
		role.GrantedByName = grantedByName.String

		groupRoles = append(groupRoles, role)
	}

	// Get everyone default role
	var everyoneRole *models.AssetSetEveryoneRole
	var roleID sql.NullInt64
	var grantedBy sql.NullInt64
	var grantedAt time.Time
	var roleName, grantedByName sql.NullString

	err = h.db.QueryRow(`
		SELECT aser.set_id, aser.role_id, aser.granted_by, aser.granted_at,
		       ar.name as role_name,
		       COALESCE(u.first_name || ' ' || u.last_name, u.username, '') as granted_by_name
		FROM asset_set_everyone_roles aser
		LEFT JOIN asset_roles ar ON aser.role_id = ar.id
		LEFT JOIN users u ON aser.granted_by = u.id
		WHERE aser.set_id = ?
	`, setID).Scan(&setID, &roleID, &grantedBy, &grantedAt, &roleName, &grantedByName)

	if err == nil {
		everyoneRole = &models.AssetSetEveryoneRole{
			SetID:         setID,
			GrantedAt:     grantedAt,
			RoleID:        utils.NullInt64ToPtr(roleID),
			GrantedBy:     utils.NullInt64ToPtr(grantedBy),
			RoleName:      roleName.String,
			GrantedByName: grantedByName.String,
		}
	} else if err != sql.ErrNoRows {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"user_roles":    userRoles,
		"group_roles":   groupRoles,
		"everyone_role": everyoneRole,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// AssignRoleRequest represents the request body for assigning a role
type AssignRoleRequest struct {
	UserID  *int `json:"user_id,omitempty"`
	GroupID *int `json:"group_id,omitempty"`
	RoleID  int  `json:"role_id"`
}

// AssignSetRole assigns a role to a user or group for a set
func (h *AssetHandler) AssignSetRole(w http.ResponseWriter, r *http.Request) {
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

	var req AssignRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate role exists
	var roleExists bool
	err = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM asset_roles WHERE id = ?)", req.RoleID).Scan(&roleExists)
	if err != nil || !roleExists {
		http.Error(w, "Invalid role ID", http.StatusBadRequest)
		return
	}

	// Must specify either user_id or group_id
	if req.UserID == nil && req.GroupID == nil {
		http.Error(w, "Must specify user_id or group_id", http.StatusBadRequest)
		return
	}

	now := time.Now()

	if req.UserID != nil {
		// Assign role to user (upsert)
		_, err = h.db.ExecWrite(`
			INSERT INTO user_asset_set_roles (set_id, user_id, role_id, granted_by, granted_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(user_id, set_id) DO UPDATE SET role_id = excluded.role_id, granted_by = excluded.granted_by, granted_at = excluded.granted_at
		`, setID, *req.UserID, req.RoleID, currentUser.ID, now)
	} else {
		// Assign role to group (upsert)
		_, err = h.db.ExecWrite(`
			INSERT INTO group_asset_set_roles (set_id, group_id, role_id, granted_by, granted_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(group_id, set_id) DO UPDATE SET role_id = excluded.role_id, granted_by = excluded.granted_by, granted_at = excluded.granted_at
		`, setID, *req.GroupID, req.RoleID, currentUser.ID, now)
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// RevokeSetRole revokes a role assignment from a user or group
func (h *AssetHandler) RevokeSetRole(w http.ResponseWriter, r *http.Request) {
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

	roleAssignmentID, err := strconv.Atoi(r.PathValue("assignmentId"))
	if err != nil {
		http.Error(w, "Invalid assignment ID", http.StatusBadRequest)
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

	// Check assignment type from query param
	assignmentType := r.URL.Query().Get("type")

	var result sql.Result
	if assignmentType == "group" {
		result, err = h.db.ExecWrite("DELETE FROM group_asset_set_roles WHERE id = ? AND set_id = ?", roleAssignmentID, setID)
	} else {
		result, err = h.db.ExecWrite("DELETE FROM user_asset_set_roles WHERE id = ? AND set_id = ?", roleAssignmentID, setID)
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Role assignment not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ============================================================================
// Everyone Role Handlers
// ============================================================================

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

// ============================================================================
// Asset CRUD Handlers
// ============================================================================

// buildSetMap creates a mapping of asset set names to IDs for CQL evaluation
func (h *AssetHandler) buildSetMap() (map[string]int, error) {
	rows, err := h.db.Query("SELECT id, name FROM asset_management_sets")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	setMap := make(map[string]int)
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		setMap[strings.ToLower(name)] = id
	}
	return setMap, nil
}

// buildWorkspaceMap creates a mapping of workspace names/keys to IDs for CQL evaluation
func (h *AssetHandler) buildWorkspaceMap() (map[string]int, error) {
	rows, err := h.db.Query("SELECT id, name, key FROM workspaces")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	workspaceMap := make(map[string]int)
	for rows.Next() {
		var id int
		var name, key string
		if err := rows.Scan(&id, &name, &key); err != nil {
			return nil, err
		}
		workspaceMap[strings.ToLower(name)] = id
		workspaceMap[strings.ToLower(key)] = id
	}
	return workspaceMap, nil
}

// GetAssets returns all assets in a set with pagination and subcategory support
func (h *AssetHandler) GetAssets(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	setID, err := strconv.Atoi(r.PathValue("setId"))
	if err != nil {
		http.Error(w, "Invalid set ID", http.StatusBadRequest)
		return
	}

	// Check view permission
	canView, err := h.canViewSet(currentUser.ID, setID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !canView {
		http.Error(w, "Access denied", http.StatusForbidden)
		return
	}

	// Parse pagination parameters
	limit := 25
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	// Build WHERE clause and args (shared between count and main query)
	whereClause := "WHERE a.set_id = ?"
	args := []interface{}{setID}
	ctePrefix := ""

	// Add filters
	if typeID := r.URL.Query().Get("type_id"); typeID != "" {
		whereClause += " AND a.asset_type_id = ?"
		args = append(args, typeID)
	}

	// Category filter with optional subcategory inclusion
	if categoryIDStr := r.URL.Query().Get("category_id"); categoryIDStr != "" {
		includeSubcats := r.URL.Query().Get("include_subcategories") != "false"
		if includeSubcats {
			// Use recursive CTE to get category and all descendants
			ctePrefix = `WITH RECURSIVE category_tree AS (
				SELECT id FROM asset_categories WHERE id = ?
				UNION ALL
				SELECT ac.id FROM asset_categories ac
				INNER JOIN category_tree ct ON ac.parent_id = ct.id
			) `
			whereClause += " AND a.category_id IN (SELECT id FROM category_tree)"
			// Prepend categoryID to args since CTE comes first
			args = append([]interface{}{categoryIDStr}, args...)
		} else {
			whereClause += " AND a.category_id = ?"
			args = append(args, categoryIDStr)
		}
	}

	if statusID := r.URL.Query().Get("status_id"); statusID != "" {
		whereClause += " AND a.status_id = ?"
		args = append(args, statusID)
	}

	if search := r.URL.Query().Get("search"); search != "" {
		whereClause += " AND (a.title LIKE ? OR a.description LIKE ? OR a.asset_tag LIKE ?)"
		searchTerm := "%" + search + "%"
		args = append(args, searchTerm, searchTerm, searchTerm)
	}

	// Check for CQL query parameter
	if cqlQuery := r.URL.Query().Get("cql"); cqlQuery != "" {
		// Build set mapping for CQL evaluation
		setMap, err := h.buildSetMap()
		if err != nil {
			http.Error(w, "Failed to load set mapping: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Build workspace mapping for linkedOf() queries
		workspaceMap, err := h.buildWorkspaceMap()
		if err != nil {
			http.Error(w, "Failed to load workspace mapping: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Create CQL evaluator and generate SQL
		evaluator := cql.NewAssetEvaluator(setMap, workspaceMap)
		cqlSQL, cqlArgs, err := evaluator.EvaluateToSQL(cqlQuery)
		if err != nil {
			http.Error(w, "CQL query error: "+err.Error(), http.StatusBadRequest)
			return
		}

		if cqlSQL != "" {
			whereClause += " AND (" + cqlSQL + ")"
			args = append(args, cqlArgs...)
		}
	}

	// Get total count first (include JOINs for CQL field references)
	countQuery := ctePrefix + `SELECT COUNT(*) FROM assets a
		LEFT JOIN asset_management_sets ams ON a.set_id = ams.id
		LEFT JOIN asset_types at ON a.asset_type_id = at.id
		LEFT JOIN asset_categories ac ON a.category_id = ac.id
		LEFT JOIN asset_statuses ast ON a.status_id = ast.id
		LEFT JOIN users u ON a.created_by = u.id
		` + whereClause
	var total int
	if err := h.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Build main query
	query := ctePrefix + `
		SELECT a.id, a.set_id, a.asset_type_id, a.category_id, a.status_id, a.title, a.description,
		       a.asset_tag, a.custom_field_values, a.frac_index,
		       a.created_by, a.created_at, a.updated_at,
		       ams.name as set_name,
		       at.name as asset_type_name, at.icon as asset_type_icon, at.color as asset_type_color,
		       ac.name as category_name, ac.path as category_path,
		       ast.name as status_name, ast.color as status_color,
		       COALESCE(u.first_name || ' ' || u.last_name, u.username, '') as creator_name,
		       u.email as creator_email,
		       (SELECT COUNT(*) FROM item_links WHERE (source_type = 'asset' AND source_id = a.id) OR (target_type = 'asset' AND target_id = a.id)) as linked_item_count
		FROM assets a
		LEFT JOIN asset_management_sets ams ON a.set_id = ams.id
		LEFT JOIN asset_types at ON a.asset_type_id = at.id
		LEFT JOIN asset_categories ac ON a.category_id = ac.id
		LEFT JOIN asset_statuses ast ON a.status_id = ast.id
		LEFT JOIN users u ON a.created_by = u.id
		` + whereClause + `
		ORDER BY a.frac_index, a.title
		LIMIT ? OFFSET ?
	`
	// Add pagination args
	queryArgs := append(args, limit, offset)

	rows, err := h.db.Query(query, queryArgs...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var assets []models.Asset
	for rows.Next() {
		var asset models.Asset
		var description, assetTag, customFieldValuesJSON, fracIndex sql.NullString
		var categoryID, statusID sql.NullInt64
		var setName, assetTypeName, assetTypeIcon, assetTypeColor sql.NullString
		var categoryName, categoryPath, statusName, statusColor sql.NullString
		var creatorName, creatorEmail sql.NullString

		err := rows.Scan(
			&asset.ID, &asset.SetID, &asset.AssetTypeID, &categoryID, &statusID, &asset.Title, &description,
			&assetTag, &customFieldValuesJSON, &fracIndex,
			&asset.CreatedBy, &asset.CreatedAt, &asset.UpdatedAt,
			&setName, &assetTypeName, &assetTypeIcon, &assetTypeColor,
			&categoryName, &categoryPath, &statusName, &statusColor,
			&creatorName, &creatorEmail, &asset.LinkedItemCount,
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		asset.CategoryID = utils.NullInt64ToPtr(categoryID)
		asset.StatusID = utils.NullInt64ToPtr(statusID)
		asset.Description = description.String
		asset.AssetTag = assetTag.String
		asset.FracIndex = utils.NullStringToPtr(fracIndex)
		asset.SetName = setName.String
		asset.AssetTypeName = assetTypeName.String
		asset.AssetTypeIcon = assetTypeIcon.String
		asset.AssetTypeColor = assetTypeColor.String
		asset.CategoryName = categoryName.String
		asset.CategoryPath = categoryPath.String
		asset.StatusName = statusName.String
		asset.StatusColor = statusColor.String
		asset.CreatorName = creatorName.String
		asset.CreatorEmail = creatorEmail.String

		// Deserialize custom field values
		if customFieldValuesJSON.Valid && customFieldValuesJSON.String != "" {
			if err := json.Unmarshal([]byte(customFieldValuesJSON.String), &asset.CustomFieldValues); err != nil {
				asset.CustomFieldValues = make(map[string]interface{})
			}
		}

		assets = append(assets, asset)
	}

	// Enrich user-type custom fields with current user data
	for i := range assets {
		if err := h.enrichUserCustomFields(&assets[i]); err != nil {
			// Log error but don't fail the request
			continue
		}
	}

	// Return paginated response
	response := map[string]interface{}{
		"assets": assets,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetAsset returns a single asset
func (h *AssetHandler) GetAsset(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	assetID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid asset ID", http.StatusBadRequest)
		return
	}

	// First get the asset to check set permissions
	var setID int
	err = h.db.QueryRow("SELECT set_id FROM assets WHERE id = ?", assetID).Scan(&setID)
	if err == sql.ErrNoRows {
		http.Error(w, "Asset not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check view permission
	canView, err := h.canViewSet(currentUser.ID, setID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !canView {
		http.Error(w, "Access denied", http.StatusForbidden)
		return
	}

	var asset models.Asset
	var description, assetTag, customFieldValuesJSON, fracIndex sql.NullString
	var categoryID, statusID sql.NullInt64
	var setName, assetTypeName, assetTypeIcon, assetTypeColor sql.NullString
	var categoryName, categoryPath, statusName, statusColor sql.NullString
	var creatorName, creatorEmail sql.NullString

	err = h.db.QueryRow(`
		SELECT a.id, a.set_id, a.asset_type_id, a.category_id, a.status_id, a.title, a.description,
		       a.asset_tag, a.custom_field_values, a.frac_index,
		       a.created_by, a.created_at, a.updated_at,
		       ams.name as set_name,
		       at.name as asset_type_name, at.icon as asset_type_icon, at.color as asset_type_color,
		       ac.name as category_name, ac.path as category_path,
		       ast.name as status_name, ast.color as status_color,
		       COALESCE(u.first_name || ' ' || u.last_name, u.username, '') as creator_name,
		       u.email as creator_email,
		       (SELECT COUNT(*) FROM item_links WHERE (source_type = 'asset' AND source_id = a.id) OR (target_type = 'asset' AND target_id = a.id)) as linked_item_count
		FROM assets a
		LEFT JOIN asset_management_sets ams ON a.set_id = ams.id
		LEFT JOIN asset_types at ON a.asset_type_id = at.id
		LEFT JOIN asset_categories ac ON a.category_id = ac.id
		LEFT JOIN asset_statuses ast ON a.status_id = ast.id
		LEFT JOIN users u ON a.created_by = u.id
		WHERE a.id = ?
	`, assetID).Scan(
		&asset.ID, &asset.SetID, &asset.AssetTypeID, &categoryID, &statusID, &asset.Title, &description,
		&assetTag, &customFieldValuesJSON, &fracIndex,
		&asset.CreatedBy, &asset.CreatedAt, &asset.UpdatedAt,
		&setName, &assetTypeName, &assetTypeIcon, &assetTypeColor,
		&categoryName, &categoryPath, &statusName, &statusColor,
		&creatorName, &creatorEmail, &asset.LinkedItemCount,
	)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	asset.CategoryID = utils.NullInt64ToPtr(categoryID)
	asset.StatusID = utils.NullInt64ToPtr(statusID)
	asset.Description = description.String
	asset.AssetTag = assetTag.String
	asset.FracIndex = utils.NullStringToPtr(fracIndex)
	asset.SetName = setName.String
	asset.AssetTypeName = assetTypeName.String
	asset.AssetTypeIcon = assetTypeIcon.String
	asset.AssetTypeColor = assetTypeColor.String
	asset.CategoryName = categoryName.String
	asset.CategoryPath = categoryPath.String
	asset.StatusName = statusName.String
	asset.StatusColor = statusColor.String
	asset.CreatorName = creatorName.String
	asset.CreatorEmail = creatorEmail.String

	// Deserialize custom field values
	if customFieldValuesJSON.Valid && customFieldValuesJSON.String != "" {
		if err := json.Unmarshal([]byte(customFieldValuesJSON.String), &asset.CustomFieldValues); err != nil {
			asset.CustomFieldValues = make(map[string]interface{})
		}
	}

	// Enrich user-type custom fields with current user data
	if err := h.enrichUserCustomFields(&asset); err != nil {
		// Log error but don't fail the request
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(asset)
}

// CreateAssetRequest represents the request body for creating an asset
type CreateAssetRequest struct {
	AssetTypeID       int                    `json:"asset_type_id"`
	CategoryID        *int                   `json:"category_id,omitempty"`
	StatusID          *int                   `json:"status_id,omitempty"`
	Title             string                 `json:"title"`
	Description       string                 `json:"description"`
	AssetTag          string                 `json:"asset_tag,omitempty"`
	CustomFieldValues map[string]interface{} `json:"custom_field_values,omitempty"`
}

// CreateAsset creates a new asset
func (h *AssetHandler) CreateAsset(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	setID, err := strconv.Atoi(r.PathValue("setId"))
	if err != nil {
		http.Error(w, "Invalid set ID", http.StatusBadRequest)
		return
	}

	// Check edit permission
	canEdit, err := h.canEditSet(currentUser.ID, setID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !canEdit {
		http.Error(w, "Edit permission required", http.StatusForbidden)
		return
	}

	var req CreateAssetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Title == "" {
		http.Error(w, "Title is required", http.StatusBadRequest)
		return
	}

	if req.AssetTypeID == 0 {
		http.Error(w, "Asset type is required", http.StatusBadRequest)
		return
	}

	// Validate asset type belongs to this set
	var typeSetID int
	err = h.db.QueryRow("SELECT set_id FROM asset_types WHERE id = ?", req.AssetTypeID).Scan(&typeSetID)
	if err == sql.ErrNoRows {
		http.Error(w, "Asset type not found", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if typeSetID != setID {
		http.Error(w, "Asset type does not belong to this set", http.StatusBadRequest)
		return
	}

	// Sanitize user input to prevent XSS
	req.Title = utils.StripHTMLTags(req.Title)
	req.Description = utils.StripHTMLTags(req.Description)

	// Validate category if provided
	if req.CategoryID != nil {
		var catSetID int
		err = h.db.QueryRow("SELECT set_id FROM asset_categories WHERE id = ?", *req.CategoryID).Scan(&catSetID)
		if err == sql.ErrNoRows {
			http.Error(w, "Category not found", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if catSetID != setID {
			http.Error(w, "Category does not belong to this set", http.StatusBadRequest)
			return
		}
	}

	// Handle status_id - get default if not provided
	var statusID *int
	if req.StatusID != nil {
		// Validate status belongs to this set
		var statusSetID int
		err = h.db.QueryRow("SELECT set_id FROM asset_statuses WHERE id = ?", *req.StatusID).Scan(&statusSetID)
		if err == sql.ErrNoRows {
			http.Error(w, "Status not found", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if statusSetID != setID {
			http.Error(w, "Status does not belong to this set", http.StatusBadRequest)
			return
		}
		statusID = req.StatusID
	} else {
		// Get default status for this set
		var defaultStatusID int
		err = h.db.QueryRow("SELECT id FROM asset_statuses WHERE set_id = ? AND is_default = true LIMIT 1", setID).Scan(&defaultStatusID)
		if err == nil {
			statusID = &defaultStatusID
		}
		// If no default status found, statusID will be nil which is okay
	}

	now := time.Now()

	// Normalize user-type custom field values to store just the ID
	if req.CustomFieldValues != nil {
		if err := h.normalizeUserFieldValues(req.CustomFieldValues, req.AssetTypeID); err != nil {
			http.Error(w, "Failed to process custom field values", http.StatusInternalServerError)
			return
		}
	}

	// Serialize custom field values
	var customFieldValuesJSON string
	if req.CustomFieldValues != nil {
		customFieldValuesBytes, err := json.Marshal(req.CustomFieldValues)
		if err != nil {
			http.Error(w, "Invalid custom field values", http.StatusBadRequest)
			return
		}
		customFieldValuesJSON = string(customFieldValuesBytes)
	}

	var assetID int64
	err = h.db.QueryRow(`
		INSERT INTO assets (set_id, asset_type_id, category_id, status_id, title, description, asset_tag, custom_field_values, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, setID, req.AssetTypeID, req.CategoryID, statusID, req.Title, req.Description, req.AssetTag, customFieldValuesJSON, currentUser.ID, now, now).Scan(&assetID)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return created asset
	asset := models.Asset{
		ID:                int(assetID),
		SetID:             setID,
		AssetTypeID:       req.AssetTypeID,
		CategoryID:        req.CategoryID,
		StatusID:          statusID,
		Title:             req.Title,
		Description:       req.Description,
		AssetTag:          req.AssetTag,
		CustomFieldValues: req.CustomFieldValues,
		CreatedBy:         &currentUser.ID,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(asset)
}

// UpdateAssetRequest represents the request body for updating an asset
type UpdateAssetRequest struct {
	AssetTypeID       int                    `json:"asset_type_id"`
	CategoryID        *int                   `json:"category_id,omitempty"`
	StatusID          *int                   `json:"status_id,omitempty"`
	Title             string                 `json:"title"`
	Description       string                 `json:"description"`
	AssetTag          string                 `json:"asset_tag,omitempty"`
	CustomFieldValues map[string]interface{} `json:"custom_field_values,omitempty"`
}

// UpdateAsset updates an existing asset
func (h *AssetHandler) UpdateAsset(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	assetID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid asset ID", http.StatusBadRequest)
		return
	}

	// Get asset to check permissions
	var setID int
	err = h.db.QueryRow("SELECT set_id FROM assets WHERE id = ?", assetID).Scan(&setID)
	if err == sql.ErrNoRows {
		http.Error(w, "Asset not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check edit permission
	canEdit, err := h.canEditSet(currentUser.ID, setID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !canEdit {
		http.Error(w, "Edit permission required", http.StatusForbidden)
		return
	}

	var req UpdateAssetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Title == "" {
		http.Error(w, "Title is required", http.StatusBadRequest)
		return
	}

	// Sanitize user input to prevent XSS
	req.Title = utils.StripHTMLTags(req.Title)
	req.Description = utils.StripHTMLTags(req.Description)

	// Validate asset type if changing
	if req.AssetTypeID != 0 {
		var typeSetID int
		err = h.db.QueryRow("SELECT set_id FROM asset_types WHERE id = ?", req.AssetTypeID).Scan(&typeSetID)
		if err == sql.ErrNoRows {
			http.Error(w, "Asset type not found", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if typeSetID != setID {
			http.Error(w, "Asset type does not belong to this set", http.StatusBadRequest)
			return
		}
	}

	// Validate category if provided
	if req.CategoryID != nil {
		var catSetID int
		err = h.db.QueryRow("SELECT set_id FROM asset_categories WHERE id = ?", *req.CategoryID).Scan(&catSetID)
		if err == sql.ErrNoRows {
			http.Error(w, "Category not found", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if catSetID != setID {
			http.Error(w, "Category does not belong to this set", http.StatusBadRequest)
			return
		}
	}

	// Validate status_id if provided
	if req.StatusID != nil {
		var statusSetID int
		err = h.db.QueryRow("SELECT set_id FROM asset_statuses WHERE id = ?", *req.StatusID).Scan(&statusSetID)
		if err == sql.ErrNoRows {
			http.Error(w, "Status not found", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if statusSetID != setID {
			http.Error(w, "Status does not belong to this set", http.StatusBadRequest)
			return
		}
	}

	now := time.Now()

	// Normalize user-type custom field values to store just the ID
	if req.CustomFieldValues != nil {
		if err := h.normalizeUserFieldValues(req.CustomFieldValues, req.AssetTypeID); err != nil {
			http.Error(w, "Failed to process custom field values", http.StatusInternalServerError)
			return
		}
	}

	// Serialize custom field values
	var customFieldValuesJSON string
	if req.CustomFieldValues != nil {
		customFieldValuesBytes, err := json.Marshal(req.CustomFieldValues)
		if err != nil {
			http.Error(w, "Invalid custom field values", http.StatusBadRequest)
			return
		}
		customFieldValuesJSON = string(customFieldValuesBytes)
	}

	result, err := h.db.ExecWrite(`
		UPDATE assets
		SET asset_type_id = ?, category_id = ?, status_id = ?, title = ?, description = ?,
		    asset_tag = ?, custom_field_values = ?, updated_at = ?
		WHERE id = ?
	`, req.AssetTypeID, req.CategoryID, req.StatusID, req.Title, req.Description, req.AssetTag, customFieldValuesJSON, now, assetID)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Asset not found", http.StatusNotFound)
		return
	}

	// Return updated asset
	asset := models.Asset{
		ID:                assetID,
		SetID:             setID,
		AssetTypeID:       req.AssetTypeID,
		CategoryID:        req.CategoryID,
		StatusID:          req.StatusID,
		Title:             req.Title,
		Description:       req.Description,
		AssetTag:          req.AssetTag,
		CustomFieldValues: req.CustomFieldValues,
		UpdatedAt:         now,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(asset)
}

// DeleteAsset deletes an asset
func (h *AssetHandler) DeleteAsset(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	assetID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid asset ID", http.StatusBadRequest)
		return
	}

	// Get asset to check permissions
	var setID int
	err = h.db.QueryRow("SELECT set_id FROM assets WHERE id = ?", assetID).Scan(&setID)
	if err == sql.ErrNoRows {
		http.Error(w, "Asset not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check edit permission (edit permission allows delete)
	canEdit, err := h.canEditSet(currentUser.ID, setID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !canEdit {
		http.Error(w, "Edit permission required", http.StatusForbidden)
		return
	}

	// Delete related links first
	_, err = h.db.ExecWrite("DELETE FROM item_links WHERE (source_type = 'asset' AND source_id = ?) OR (target_type = 'asset' AND target_id = ?)", assetID, assetID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	result, err := h.db.ExecWrite("DELETE FROM assets WHERE id = ?", assetID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Asset not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ============================================================================
// Asset Link Handlers
// ============================================================================

// GetAssetLinks returns all links for an asset (incoming and outgoing)
func (h *AssetHandler) GetAssetLinks(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	assetID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid asset ID", http.StatusBadRequest)
		return
	}

	// Get asset to check permissions
	var setID int
	err = h.db.QueryRow("SELECT set_id FROM assets WHERE id = ?", assetID).Scan(&setID)
	if err == sql.ErrNoRows {
		http.Error(w, "Asset not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check view permission
	canView, err := h.canViewSet(currentUser.ID, setID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !canView {
		http.Error(w, "Access denied", http.StatusForbidden)
		return
	}

	// Get outgoing links (where this asset is the source)
	outgoingQuery := `
		SELECT il.id, il.link_type_id, il.source_type, il.source_id, il.target_type, il.target_id,
		       il.created_by, il.created_at,
		       lt.name as link_type_name, lt.color as link_type_color, lt.forward_label, lt.reverse_label,
		       CASE
		           WHEN il.target_type = 'item' THEN (SELECT title FROM items WHERE id = il.target_id)
		           WHEN il.target_type = 'asset' THEN (SELECT title FROM assets WHERE id = il.target_id)
		           WHEN il.target_type = 'test_case' THEN (SELECT title FROM test_cases WHERE id = il.target_id)
		           ELSE ''
		       END as target_title,
		       COALESCE(u.username, '') as created_by_name
		FROM item_links il
		JOIN link_types lt ON il.link_type_id = lt.id
		LEFT JOIN users u ON il.created_by = u.id
		WHERE il.source_type = 'asset' AND il.source_id = ?
		ORDER BY lt.name, il.created_at DESC
	`

	outgoingRows, err := h.db.Query(outgoingQuery, assetID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer outgoingRows.Close()

	var outgoingLinks []models.ItemLink
	for outgoingRows.Next() {
		var link models.ItemLink
		err := outgoingRows.Scan(
			&link.ID, &link.LinkTypeID, &link.SourceType, &link.SourceID,
			&link.TargetType, &link.TargetID, &link.CreatedBy, &link.CreatedAt,
			&link.LinkTypeName, &link.LinkTypeColor, &link.LinkTypeForwardLabel, &link.LinkTypeReverseLabel,
			&link.TargetTitle, &link.CreatedByName,
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		outgoingLinks = append(outgoingLinks, link)
	}

	// Get incoming links (where this asset is the target)
	incomingQuery := `
		SELECT il.id, il.link_type_id, il.source_type, il.source_id, il.target_type, il.target_id,
		       il.created_by, il.created_at,
		       lt.name as link_type_name, lt.color as link_type_color, lt.forward_label, lt.reverse_label,
		       CASE
		           WHEN il.source_type = 'item' THEN (SELECT title FROM items WHERE id = il.source_id)
		           WHEN il.source_type = 'asset' THEN (SELECT title FROM assets WHERE id = il.source_id)
		           WHEN il.source_type = 'test_case' THEN (SELECT title FROM test_cases WHERE id = il.source_id)
		           ELSE ''
		       END as source_title,
		       COALESCE(u.username, '') as created_by_name
		FROM item_links il
		JOIN link_types lt ON il.link_type_id = lt.id
		LEFT JOIN users u ON il.created_by = u.id
		WHERE il.target_type = 'asset' AND il.target_id = ?
		ORDER BY lt.name, il.created_at DESC
	`

	incomingRows, err := h.db.Query(incomingQuery, assetID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer incomingRows.Close()

	var incomingLinks []models.ItemLink
	for incomingRows.Next() {
		var link models.ItemLink
		err := incomingRows.Scan(
			&link.ID, &link.LinkTypeID, &link.SourceType, &link.SourceID,
			&link.TargetType, &link.TargetID, &link.CreatedBy, &link.CreatedAt,
			&link.LinkTypeName, &link.LinkTypeColor, &link.LinkTypeForwardLabel, &link.LinkTypeReverseLabel,
			&link.SourceTitle, &link.CreatedByName,
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		incomingLinks = append(incomingLinks, link)
	}

	response := map[string]interface{}{
		"outgoing": outgoingLinks,
		"incoming": incomingLinks,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// CreateAssetLinkRequest represents the request body for creating an asset link
type CreateAssetLinkRequest struct {
	LinkTypeID int    `json:"link_type_id"`
	TargetType string `json:"target_type"` // item, asset, test_case
	TargetID   int    `json:"target_id"`
}

// CreateAssetLink creates a link from an asset to another entity
func (h *AssetHandler) CreateAssetLink(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	assetID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid asset ID", http.StatusBadRequest)
		return
	}

	// Get asset to check permissions
	var setID int
	err = h.db.QueryRow("SELECT set_id FROM assets WHERE id = ?", assetID).Scan(&setID)
	if err == sql.ErrNoRows {
		http.Error(w, "Asset not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check edit permission
	canEdit, err := h.canEditSet(currentUser.ID, setID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !canEdit {
		http.Error(w, "Edit permission required", http.StatusForbidden)
		return
	}

	var req CreateAssetLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate target type
	validTargetTypes := map[string]bool{"item": true, "asset": true, "test_case": true}
	if !validTargetTypes[req.TargetType] {
		http.Error(w, "Invalid target_type. Must be 'item', 'asset', or 'test_case'", http.StatusBadRequest)
		return
	}

	// Prevent self-links
	if req.TargetType == "asset" && req.TargetID == assetID {
		http.Error(w, "Cannot create link to self", http.StatusBadRequest)
		return
	}

	// Verify link type exists and is active
	var linkTypeActive bool
	err = h.db.QueryRow("SELECT active FROM link_types WHERE id = ?", req.LinkTypeID).Scan(&linkTypeActive)
	if err == sql.ErrNoRows {
		http.Error(w, "Link type not found", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !linkTypeActive {
		http.Error(w, "Link type is not active", http.StatusBadRequest)
		return
	}

	now := time.Now()

	var linkID int64
	err = h.db.QueryRow(`
		INSERT INTO item_links (link_type_id, source_type, source_id, target_type, target_id, created_by, created_at)
		VALUES (?, 'asset', ?, ?, ?, ?, ?) RETURNING id
	`, req.LinkTypeID, assetID, req.TargetType, req.TargetID, currentUser.ID, now).Scan(&linkID)

	if err != nil {
		// Check for unique constraint violation
		if err.Error() == "UNIQUE constraint failed: item_links.link_type_id, item_links.source_type, item_links.source_id, item_links.target_type, item_links.target_id" {
			http.Error(w, "Link already exists", http.StatusConflict)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	response := map[string]interface{}{
		"id":           linkID,
		"link_type_id": req.LinkTypeID,
		"source_type":  "asset",
		"source_id":    assetID,
		"target_type":  req.TargetType,
		"target_id":    req.TargetID,
		"created_at":   now,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// extractUserID extracts user ID from various value formats (int, float64, or map with "id")
func extractUserID(val interface{}) int {
	switch v := val.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case map[string]interface{}:
		if id, ok := v["id"]; ok {
			return extractUserID(id)
		}
	}
	return 0
}

// getUserFieldIDsForAssetType returns a set of custom field IDs that are user-type for a given asset type
func (h *AssetHandler) getUserFieldIDsForAssetType(assetTypeID int) (map[int]bool, error) {
	rows, err := h.db.Query(`
		SELECT cfd.id
		FROM custom_field_definitions cfd
		JOIN asset_type_fields atf ON atf.custom_field_id = cfd.id
		WHERE atf.asset_type_id = ? AND cfd.field_type = 'user'
	`, assetTypeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	fieldIDs := make(map[int]bool)
	for rows.Next() {
		var fieldID int
		if err := rows.Scan(&fieldID); err != nil {
			return nil, err
		}
		fieldIDs[fieldID] = true
	}
	return fieldIDs, nil
}

// enrichUserCustomFields resolves user IDs to full user data for user-type custom fields
func (h *AssetHandler) enrichUserCustomFields(asset *models.Asset) error {
	if asset.CustomFieldValues == nil || len(asset.CustomFieldValues) == 0 {
		return nil
	}

	// Get user-type field IDs for this asset's type
	userFieldIDs, err := h.getUserFieldIDsForAssetType(asset.AssetTypeID)
	if err != nil {
		return err
	}

	if len(userFieldIDs) == 0 {
		return nil
	}

	// Resolve each user field
	for fieldID := range userFieldIDs {
		fieldKey := strconv.Itoa(fieldID)
		val, ok := asset.CustomFieldValues[fieldKey]
		if !ok || val == nil {
			continue
		}

		userID := extractUserID(val)
		if userID <= 0 {
			continue
		}

		// Query user data
		var firstName, lastName, email, avatarURL sql.NullString
		err := h.db.QueryRow(`
			SELECT first_name, last_name, email, avatar_url
			FROM users WHERE id = ?
		`, userID).Scan(&firstName, &lastName, &email, &avatarURL)
		if err != nil {
			if err == sql.ErrNoRows {
				// User doesn't exist, clear the value
				asset.CustomFieldValues[fieldKey] = nil
				continue
			}
			return err
		}

		// Replace with enriched data
		asset.CustomFieldValues[fieldKey] = map[string]interface{}{
			"id":         userID,
			"name":       strings.TrimSpace(firstName.String + " " + lastName.String),
			"email":      email.String,
			"avatar_url": avatarURL.String,
		}
	}

	return nil
}

// normalizeUserFieldValues extracts just the user ID from user-type custom field values before storage
func (h *AssetHandler) normalizeUserFieldValues(customFieldValues map[string]interface{}, assetTypeID int) error {
	if customFieldValues == nil || len(customFieldValues) == 0 {
		return nil
	}

	// Get user-type field IDs for this asset's type
	userFieldIDs, err := h.getUserFieldIDsForAssetType(assetTypeID)
	if err != nil {
		return err
	}

	if len(userFieldIDs) == 0 {
		return nil
	}

	// Normalize each user field to just the ID
	for fieldID := range userFieldIDs {
		fieldKey := strconv.Itoa(fieldID)
		val, ok := customFieldValues[fieldKey]
		if !ok || val == nil {
			continue
		}

		userID := extractUserID(val)
		if userID > 0 {
			customFieldValues[fieldKey] = userID
		} else {
			// Invalid value, remove it
			delete(customFieldValues, fieldKey)
		}
	}

	return nil
}
