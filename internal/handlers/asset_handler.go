package handlers

import (
	"database/sql"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

// AssetHandler handles asset management operations
type AssetHandler struct {
	db                database.Database
	repo              *repository.AssetRepository
	permissionService *services.PermissionService
	attachmentPath    string
}

// NewAssetHandler creates a new asset handler
func NewAssetHandler(db database.Database, permissionService *services.PermissionService, attachmentPath string) *AssetHandler {
	return &AssetHandler{
		db:                db,
		repo:              repository.NewAssetRepository(db),
		permissionService: permissionService,
		attachmentPath:    attachmentPath,
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

// buildSetMap creates a mapping of asset set names to IDs for CQL evaluation
func (h *AssetHandler) buildSetMap() (map[string]int, error) {
	rows, err := h.db.Query("SELECT id, name FROM asset_management_sets")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

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

// buildCustomFieldMap creates a mapping of lowercase custom field names to field IDs for CQL evaluation.
// This allows CQL queries to use human-readable names (cf_Time Estimate) while the DB stores numeric IDs as JSON keys.
func (h *AssetHandler) buildCustomFieldMap(setID int) (map[string]int, error) {
	rows, err := h.db.Query(`SELECT DISTINCT cfd.id, LOWER(cfd.name)
		FROM custom_field_definitions cfd
		JOIN asset_type_fields atf ON atf.custom_field_id = cfd.id
		JOIN asset_types at2 ON atf.asset_type_id = at2.id
		WHERE at2.set_id = ?`, setID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	cfMap := make(map[string]int)
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		cfMap[name] = id
	}
	return cfMap, nil
}

// buildWorkspaceMap creates a mapping of workspace names/keys to IDs for CQL evaluation
func (h *AssetHandler) buildWorkspaceMap() (map[string]int, error) {
	rows, err := h.db.Query("SELECT id, name, key FROM workspaces")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

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
