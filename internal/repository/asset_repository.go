package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// AssetRepository provides data access methods for asset management
type AssetRepository struct {
	db database.Database
}

// NewAssetRepository creates a new asset repository
func NewAssetRepository(db database.Database) *AssetRepository {
	return &AssetRepository{db: db}
}

// ============================================================================
// Asset Management Set Operations
// ============================================================================

// ListSetsForUser returns all asset sets accessible by the specified user
func (r *AssetRepository) ListSetsForUser(userID int, isAdmin bool) ([]models.AssetManagementSet, error) {
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
		args = append(args, userID, userID)
	}

	query += ` ORDER BY ams.is_default DESC, ams.name`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list asset sets: %w", err)
	}
	defer func() { _ = rows.Close() }()

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
			return nil, fmt.Errorf("failed to scan asset set: %w", err)
		}

		set.CreatorName = creatorName.String
		set.Description = description.String
		sets = append(sets, set)
	}

	return sets, nil
}

// GetSetByID returns an asset set by ID
func (r *AssetRepository) GetSetByID(setID int) (*models.AssetManagementSet, error) {
	var set models.AssetManagementSet
	var creatorName sql.NullString
	var description sql.NullString

	err := r.db.QueryRow(`
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
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get asset set: %w", err)
	}

	set.CreatorName = creatorName.String
	set.Description = description.String

	return &set, nil
}

// CreateSet creates a new asset management set
func (r *AssetRepository) CreateSet(set *models.AssetManagementSet) (int, error) {
	now := time.Now()
	var id int
	err := r.db.QueryRow(`
		INSERT INTO asset_management_sets (name, description, is_default, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING id
	`, set.Name, set.Description, set.IsDefault, set.CreatedBy, now, now).Scan(&id)

	if err != nil {
		return 0, fmt.Errorf("failed to create asset set: %w", err)
	}

	return id, nil
}

// UpdateSet updates an asset management set
func (r *AssetRepository) UpdateSet(set *models.AssetManagementSet) error {
	now := time.Now()
	result, err := r.db.ExecWrite(`
		UPDATE asset_management_sets SET name = ?, description = ?, is_default = ?, updated_at = ?
		WHERE id = ?
	`, set.Name, set.Description, set.IsDefault, now, set.ID)

	if err != nil {
		return fmt.Errorf("failed to update asset set: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// DeleteSet deletes an asset management set and all associated data
func (r *AssetRepository) DeleteSet(setID int) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Delete all associated data in order (respecting foreign key constraints)
	deletions := []string{
		"DELETE FROM assets WHERE set_id = ?",
		"DELETE FROM asset_categories WHERE set_id = ?",
		"DELETE FROM asset_types WHERE set_id = ?",
		"DELETE FROM asset_statuses WHERE set_id = ?",
		"DELETE FROM user_asset_set_roles WHERE set_id = ?",
		"DELETE FROM group_asset_set_roles WHERE set_id = ?",
		"DELETE FROM asset_set_everyone_roles WHERE set_id = ?",
		"DELETE FROM asset_management_sets WHERE id = ?",
	}

	for _, query := range deletions {
		if _, err := tx.Exec(query, setID); err != nil {
			return fmt.Errorf("failed to delete asset set data: %w", err)
		}
	}

	return tx.Commit()
}

// ClearDefaultSet clears the is_default flag from all sets
func (r *AssetRepository) ClearDefaultSet() error {
	_, err := r.db.ExecWrite(`UPDATE asset_management_sets SET is_default = false`)
	if err != nil {
		return fmt.Errorf("failed to clear default set: %w", err)
	}
	return nil
}

// CreateDefaultStatuses creates default statuses for a new asset set
func (r *AssetRepository) CreateDefaultStatuses(setID int) error {
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
		_, err := r.db.ExecWrite(`
			INSERT INTO asset_statuses (set_id, name, color, is_default, display_order, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, setID, s.Name, s.Color, s.IsDefault, s.DisplayOrder, now, now)
		if err != nil {
			return fmt.Errorf("failed to create default status: %w", err)
		}
	}

	return nil
}

// ============================================================================
// Role & Permission Operations
// ============================================================================

// GetUserSetRole returns the role a user has for an asset set
// Priority: Direct User Role > Group Role > Everyone Default
// Note: System admin check should be done in the handler layer
func (r *AssetRepository) GetUserSetRole(userID, setID int) (*models.AssetRole, error) {
	var role models.AssetRole

	// 1. Check direct user role (OVERRIDE - takes precedence)
	err := r.db.QueryRow(`
		SELECT ar.id, ar.name, ar.description, ar.is_system, ar.display_order
		FROM user_asset_set_roles uasr
		JOIN asset_roles ar ON uasr.role_id = ar.id
		WHERE uasr.set_id = ? AND uasr.user_id = ?
	`, setID, userID).Scan(&role.ID, &role.Name, &role.Description, &role.IsSystem, &role.DisplayOrder)

	if err == nil {
		return &role, nil
	}
	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to get user role: %w", err)
	}

	// 2. Check group roles (get highest by display_order desc = most privileged)
	err = r.db.QueryRow(`
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
		return nil, fmt.Errorf("failed to get group role: %w", err)
	}

	// 3. Check everyone default (FALLBACK)
	var roleID sql.NullInt64
	err = r.db.QueryRow(`
		SELECT role_id FROM asset_set_everyone_roles WHERE set_id = ?
	`, setID).Scan(&roleID)

	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to get everyone role: %w", err)
	}

	if err == sql.ErrNoRows || !roleID.Valid {
		return nil, nil
	}

	// Fetch the everyone role details
	err = r.db.QueryRow(`
		SELECT id, name, description, is_system, display_order
		FROM asset_roles WHERE id = ?
	`, roleID.Int64).Scan(&role.ID, &role.Name, &role.Description, &role.IsSystem, &role.DisplayOrder)

	if err != nil {
		return nil, fmt.Errorf("failed to get role details: %w", err)
	}

	return &role, nil
}

// RoleHasPermission checks if a role has a specific permission
func (r *AssetRepository) RoleHasPermission(roleID int, permissionKey string) (bool, error) {
	// Virtual admin role (-1) has all permissions
	if roleID == -1 {
		return true, nil
	}

	var count int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM asset_role_permissions arp
		JOIN asset_permissions ap ON arp.permission_id = ap.id
		WHERE arp.role_id = ? AND ap.permission_key = ?
	`, roleID, permissionKey).Scan(&count)

	if err != nil {
		return false, fmt.Errorf("failed to check role permission: %w", err)
	}

	return count > 0, nil
}

// GetEveryoneRoleForSet returns the everyone role ID for a set
func (r *AssetRepository) GetEveryoneRoleForSet(setID int) (*int, error) {
	var roleID sql.NullInt64
	err := r.db.QueryRow(`
		SELECT role_id FROM asset_set_everyone_roles WHERE set_id = ?
	`, setID).Scan(&roleID)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get everyone role: %w", err)
	}

	if !roleID.Valid {
		return nil, nil
	}

	id := int(roleID.Int64)
	return &id, nil
}

// ListAllRoles returns all available asset roles
func (r *AssetRepository) ListAllRoles() ([]models.AssetRole, error) {
	rows, err := r.db.Query(`
		SELECT id, name, description, is_system, display_order, created_at, updated_at
		FROM asset_roles ORDER BY display_order
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list roles: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var roles []models.AssetRole
	for rows.Next() {
		var role models.AssetRole
		if err := rows.Scan(&role.ID, &role.Name, &role.Description, &role.IsSystem, &role.DisplayOrder, &role.CreatedAt, &role.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan role: %w", err)
		}
		roles = append(roles, role)
	}

	return roles, nil
}

// GetRoleByID returns a role by ID
func (r *AssetRepository) GetRoleByID(roleID int) (*models.AssetRole, error) {
	var role models.AssetRole
	err := r.db.QueryRow(`
		SELECT id, name, description, is_system, display_order, created_at, updated_at
		FROM asset_roles WHERE id = ?
	`, roleID).Scan(&role.ID, &role.Name, &role.Description, &role.IsSystem, &role.DisplayOrder, &role.CreatedAt, &role.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get role: %w", err)
	}

	return &role, nil
}

// GetRolePermissions returns the permissions for a role
func (r *AssetRepository) GetRolePermissions(roleID int) ([]models.AssetPermission, error) {
	rows, err := r.db.Query(`
		SELECT ap.id, ap.permission_key, ap.permission_name, ap.description, ap.created_at
		FROM asset_role_permissions arp
		JOIN asset_permissions ap ON arp.permission_id = ap.id
		WHERE arp.role_id = ?
	`, roleID)
	if err != nil {
		return nil, fmt.Errorf("failed to get role permissions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var permissions []models.AssetPermission
	for rows.Next() {
		var perm models.AssetPermission
		if err := rows.Scan(&perm.ID, &perm.PermissionKey, &perm.PermissionName, &perm.Description, &perm.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan permission: %w", err)
		}
		permissions = append(permissions, perm)
	}

	return permissions, nil
}

// ============================================================================
// Set Role Assignment Operations
// ============================================================================

// GetSetUserRoles returns all user role assignments for a set
func (r *AssetRepository) GetSetUserRoles(setID int) ([]models.UserAssetSetRole, error) {
	rows, err := r.db.Query(`
		SELECT uasr.id, uasr.user_id, uasr.set_id, uasr.role_id, uasr.granted_by, uasr.granted_at,
		       COALESCE(u.first_name || ' ' || u.last_name, u.username, '') as user_name,
		       u.email as user_email,
		       ar.name as role_name,
		       COALESCE(g.first_name || ' ' || g.last_name, g.username, '') as granted_by_name
		FROM user_asset_set_roles uasr
		JOIN users u ON uasr.user_id = u.id
		JOIN asset_roles ar ON uasr.role_id = ar.id
		LEFT JOIN users g ON uasr.granted_by = g.id
		WHERE uasr.set_id = ?
		ORDER BY u.first_name, u.last_name
	`, setID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user roles: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var roles []models.UserAssetSetRole
	for rows.Next() {
		var role models.UserAssetSetRole
		var grantedByName sql.NullString
		if err := rows.Scan(&role.ID, &role.UserID, &role.SetID, &role.RoleID, &role.GrantedBy, &role.GrantedAt,
			&role.UserName, &role.UserEmail, &role.RoleName, &grantedByName); err != nil {
			return nil, fmt.Errorf("failed to scan user role: %w", err)
		}
		role.GrantedByName = grantedByName.String
		roles = append(roles, role)
	}

	return roles, nil
}

// GetSetGroupRoles returns all group role assignments for a set
func (r *AssetRepository) GetSetGroupRoles(setID int) ([]models.GroupAssetSetRole, error) {
	rows, err := r.db.Query(`
		SELECT gasr.id, gasr.group_id, gasr.set_id, gasr.role_id, gasr.granted_by, gasr.granted_at,
		       tg.name as group_name,
		       ar.name as role_name,
		       COALESCE(g.first_name || ' ' || g.last_name, g.username, '') as granted_by_name
		FROM group_asset_set_roles gasr
		JOIN team_groups tg ON gasr.group_id = tg.id
		JOIN asset_roles ar ON gasr.role_id = ar.id
		LEFT JOIN users g ON gasr.granted_by = g.id
		WHERE gasr.set_id = ?
		ORDER BY tg.name
	`, setID)
	if err != nil {
		return nil, fmt.Errorf("failed to get group roles: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var roles []models.GroupAssetSetRole
	for rows.Next() {
		var role models.GroupAssetSetRole
		var grantedByName sql.NullString
		if err := rows.Scan(&role.ID, &role.GroupID, &role.SetID, &role.RoleID, &role.GrantedBy, &role.GrantedAt,
			&role.GroupName, &role.RoleName, &grantedByName); err != nil {
			return nil, fmt.Errorf("failed to scan group role: %w", err)
		}
		role.GrantedByName = grantedByName.String
		roles = append(roles, role)
	}

	return roles, nil
}

// AssignUserRole assigns a role to a user for a set (upsert)
func (r *AssetRepository) AssignUserRole(setID, userID, roleID, grantedBy int) error {
	now := time.Now()
	_, err := r.db.ExecWrite(`
		INSERT INTO user_asset_set_roles (set_id, user_id, role_id, granted_by, granted_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (set_id, user_id) DO UPDATE SET role_id = ?, granted_by = ?, granted_at = ?
	`, setID, userID, roleID, grantedBy, now, roleID, grantedBy, now)

	if err != nil {
		return fmt.Errorf("failed to assign user role: %w", err)
	}
	return nil
}

// AssignGroupRole assigns a role to a group for a set (upsert)
func (r *AssetRepository) AssignGroupRole(setID, groupID, roleID, grantedBy int) error {
	now := time.Now()
	_, err := r.db.ExecWrite(`
		INSERT INTO group_asset_set_roles (set_id, group_id, role_id, granted_by, granted_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (set_id, group_id) DO UPDATE SET role_id = ?, granted_by = ?, granted_at = ?
	`, setID, groupID, roleID, grantedBy, now, roleID, grantedBy, now)

	if err != nil {
		return fmt.Errorf("failed to assign group role: %w", err)
	}
	return nil
}

// RevokeUserRole removes a user's role assignment for a set
func (r *AssetRepository) RevokeUserRole(assignmentID, setID int) error {
	result, err := r.db.ExecWrite(`
		DELETE FROM user_asset_set_roles WHERE id = ? AND set_id = ?
	`, assignmentID, setID)
	if err != nil {
		return fmt.Errorf("failed to revoke user role: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// RevokeGroupRole removes a group's role assignment for a set
func (r *AssetRepository) RevokeGroupRole(assignmentID, setID int) error {
	result, err := r.db.ExecWrite(`
		DELETE FROM group_asset_set_roles WHERE id = ? AND set_id = ?
	`, assignmentID, setID)
	if err != nil {
		return fmt.Errorf("failed to revoke group role: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// SetEveryoneRole sets the everyone role for a set (upsert or delete)
func (r *AssetRepository) SetEveryoneRole(setID int, roleID *int, grantedBy int) error {
	now := time.Now()
	if roleID == nil {
		_, err := r.db.ExecWrite(`DELETE FROM asset_set_everyone_roles WHERE set_id = ?`, setID)
		return err
	}

	_, err := r.db.ExecWrite(`
		INSERT INTO asset_set_everyone_roles (set_id, role_id, granted_by, granted_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (set_id) DO UPDATE SET role_id = ?, granted_by = ?, granted_at = ?
	`, setID, *roleID, grantedBy, now, *roleID, grantedBy, now)

	if err != nil {
		return fmt.Errorf("failed to set everyone role: %w", err)
	}
	return nil
}

// ============================================================================
// Asset Operations
// ============================================================================

// GetAssetByID returns an asset by ID with all joined data
func (r *AssetRepository) GetAssetByID(assetID int) (*models.Asset, error) {
	var asset models.Asset
	var categoryID, statusID, createdBy sql.NullInt64
	var description, assetTag, fracIndex sql.NullString
	var categoryName, categoryPath, statusName, statusColor sql.NullString
	var assetTypeIcon, assetTypeColor sql.NullString
	var creatorName, creatorEmail sql.NullString
	var customFieldValuesJSON sql.NullString

	err := r.db.QueryRow(`
		SELECT a.id, a.set_id, a.asset_type_id, a.category_id, a.status_id,
		       a.title, a.description, a.asset_tag, a.custom_field_values,
		       a.frac_index, a.created_by, a.created_at, a.updated_at,
		       ams.name as set_name,
		       at.name as asset_type_name, at.icon as asset_type_icon, at.color as asset_type_color,
		       ac.name as category_name, ac.path as category_path,
		       ast.name as status_name, ast.color as status_color,
		       COALESCE(u.first_name || ' ' || u.last_name, u.username, '') as creator_name,
		       u.email as creator_email
		FROM assets a
		JOIN asset_management_sets ams ON a.set_id = ams.id
		JOIN asset_types at ON a.asset_type_id = at.id
		LEFT JOIN asset_categories ac ON a.category_id = ac.id
		LEFT JOIN asset_statuses ast ON a.status_id = ast.id
		LEFT JOIN users u ON a.created_by = u.id
		WHERE a.id = ?
	`, assetID).Scan(
		&asset.ID, &asset.SetID, &asset.AssetTypeID, &categoryID, &statusID,
		&asset.Title, &description, &assetTag, &customFieldValuesJSON,
		&fracIndex, &createdBy, &asset.CreatedAt, &asset.UpdatedAt,
		&asset.SetName, &asset.AssetTypeName, &assetTypeIcon, &assetTypeColor,
		&categoryName, &categoryPath, &statusName, &statusColor,
		&creatorName, &creatorEmail,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get asset: %w", err)
	}

	// Handle nullable fields
	if categoryID.Valid {
		id := int(categoryID.Int64)
		asset.CategoryID = &id
	}
	if statusID.Valid {
		id := int(statusID.Int64)
		asset.StatusID = &id
	}
	if createdBy.Valid {
		id := int(createdBy.Int64)
		asset.CreatedBy = &id
	}
	asset.Description = description.String
	asset.AssetTag = assetTag.String
	if fracIndex.Valid {
		asset.FracIndex = &fracIndex.String
	}
	asset.AssetTypeIcon = assetTypeIcon.String
	asset.AssetTypeColor = assetTypeColor.String
	asset.CategoryName = categoryName.String
	asset.CategoryPath = categoryPath.String
	asset.StatusName = statusName.String
	asset.StatusColor = statusColor.String
	asset.CreatorName = creatorName.String
	asset.CreatorEmail = creatorEmail.String

	return &asset, nil
}

// GetAssetSetID returns the set ID for an asset
func (r *AssetRepository) GetAssetSetID(assetID int) (int, error) {
	var setID int
	err := r.db.QueryRow("SELECT set_id FROM assets WHERE id = ?", assetID).Scan(&setID)
	if err == sql.ErrNoRows {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get asset set ID: %w", err)
	}
	return setID, nil
}

// DeleteAsset deletes an asset
func (r *AssetRepository) DeleteAsset(assetID int) error {
	result, err := r.db.ExecWrite("DELETE FROM assets WHERE id = ?", assetID)
	if err != nil {
		return fmt.Errorf("failed to delete asset: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ============================================================================
// Validation Methods
// ============================================================================

// AssetTypeBelongsToSet checks if an asset type belongs to a set
func (r *AssetRepository) AssetTypeBelongsToSet(typeID, setID int) (bool, error) {
	var exists bool
	err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM asset_types WHERE id = ? AND set_id = ?)", typeID, setID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check asset type: %w", err)
	}
	return exists, nil
}

// CategoryBelongsToSet checks if a category belongs to a set
func (r *AssetRepository) CategoryBelongsToSet(categoryID, setID int) (bool, error) {
	var exists bool
	err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM asset_categories WHERE id = ? AND set_id = ?)", categoryID, setID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check category: %w", err)
	}
	return exists, nil
}

// StatusBelongsToSet checks if a status belongs to a set
func (r *AssetRepository) StatusBelongsToSet(statusID, setID int) (bool, error) {
	var exists bool
	err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM asset_statuses WHERE id = ? AND set_id = ?)", statusID, setID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check status: %w", err)
	}
	return exists, nil
}

// GetDefaultStatus returns the default status ID for a set
func (r *AssetRepository) GetDefaultStatus(setID int) (*int, error) {
	var statusID sql.NullInt64
	err := r.db.QueryRow(`
		SELECT id FROM asset_statuses WHERE set_id = ? AND is_default = true LIMIT 1
	`, setID).Scan(&statusID)

	if err == sql.ErrNoRows || !statusID.Valid {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get default status: %w", err)
	}

	id := int(statusID.Int64)
	return &id, nil
}

// RoleExists checks if a role exists
func (r *AssetRepository) RoleExists(roleID int) (bool, error) {
	var exists bool
	err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM asset_roles WHERE id = ?)", roleID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check role: %w", err)
	}
	return exists, nil
}

// ============================================================================
// Asset types
// ============================================================================

// FindAssetTypesForSet returns all asset types for a set with joined set name and an asset count.
func (r *AssetRepository) FindAssetTypesForSet(setID int) ([]models.AssetType, error) {
	rows, err := r.db.Query(`
		SELECT at.id, at.set_id, at.name, at.description, at.icon, at.color,
		       at.display_order, at.is_active, at.created_at, at.updated_at,
		       ams.name as set_name,
		       (SELECT COUNT(*) FROM assets WHERE asset_type_id = at.id) as asset_count
		FROM asset_types at
		LEFT JOIN asset_management_sets ams ON at.set_id = ams.id
		WHERE at.set_id = ?
		ORDER BY at.display_order, at.name
	`, setID)
	if err != nil {
		return nil, fmt.Errorf("failed to query asset types: %w", err)
	}
	defer func() { _ = rows.Close() }()

	types := make([]models.AssetType, 0)
	for rows.Next() {
		at, err := scanAssetTypeRow(rows)
		if err != nil {
			return nil, err
		}
		types = append(types, at)
	}
	return types, nil
}

// FindAssetTypeByID returns a single asset type with set name and asset count.
// Returns ErrNotFound if the type does not exist.
func (r *AssetRepository) FindAssetTypeByID(typeID int) (*models.AssetType, error) {
	row := r.db.QueryRow(`
		SELECT at.id, at.set_id, at.name, at.description, at.icon, at.color,
		       at.display_order, at.is_active, at.created_at, at.updated_at,
		       ams.name as set_name,
		       (SELECT COUNT(*) FROM assets WHERE asset_type_id = at.id) as asset_count
		FROM asset_types at
		LEFT JOIN asset_management_sets ams ON at.set_id = ams.id
		WHERE at.id = ?
	`, typeID)
	at, err := scanAssetTypeRow(row)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &at, nil
}

// GetAssetTypeSetID returns the set_id for an asset type. Returns ErrNotFound if it doesn't exist.
func (r *AssetRepository) GetAssetTypeSetID(typeID int) (int, error) {
	var setID int
	err := r.db.QueryRow("SELECT set_id FROM asset_types WHERE id = ?", typeID).Scan(&setID)
	if err == sql.ErrNoRows {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get asset type set: %w", err)
	}
	return setID, nil
}

// GetAssetTypeSetAndCount returns the set_id and current asset count for an asset type.
func (r *AssetRepository) GetAssetTypeSetAndCount(typeID int) (setID, assetCount int, err error) {
	err = r.db.QueryRow(`
		SELECT set_id, (SELECT COUNT(*) FROM assets WHERE asset_type_id = ?) as asset_count
		FROM asset_types WHERE id = ?
	`, typeID, typeID).Scan(&setID, &assetCount)
	if err == sql.ErrNoRows {
		return 0, 0, ErrNotFound
	}
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get asset type set/count: %w", err)
	}
	return setID, assetCount, nil
}

// CreateAssetType inserts an asset type and returns its id.
func (r *AssetRepository) CreateAssetType(at *models.AssetType) (int, error) {
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO asset_types (set_id, name, description, icon, color, display_order, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, at.SetID, at.Name, at.Description, at.Icon, at.Color, at.DisplayOrder, at.IsActive, at.CreatedAt, at.UpdatedAt).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create asset type: %w", err)
	}
	return int(id), nil
}

// AssetTypeUpdate holds the patchable fields for an asset type update.
// IsActive is nil-able so callers can distinguish "keep current" from "set false".
type AssetTypeUpdate struct {
	Name         string
	Description  string
	Icon         string
	Color        string
	DisplayOrder int
	IsActive     *bool
}

// UpdateAssetType applies a patch to an asset type. Returns ErrNotFound when no row matches.
func (r *AssetRepository) UpdateAssetType(typeID int, patch AssetTypeUpdate) error {
	query := "UPDATE asset_types SET name = ?, description = ?, icon = ?, color = ?, display_order = ?, updated_at = ?"
	args := []interface{}{patch.Name, patch.Description, patch.Icon, patch.Color, patch.DisplayOrder, time.Now()}

	if patch.IsActive != nil {
		query += ", is_active = ?"
		args = append(args, *patch.IsActive)
	}
	query += " WHERE id = ?"
	args = append(args, typeID)

	result, err := r.db.ExecWrite(query, args...)
	if err != nil {
		return fmt.Errorf("failed to update asset type: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteAssetType removes an asset type along with its field assignments in one transaction.
// Returns ErrNotFound when the type does not exist.
func (r *AssetRepository) DeleteAssetType(typeID int) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec("DELETE FROM asset_type_fields WHERE asset_type_id = ?", typeID); err != nil {
		return fmt.Errorf("failed to delete asset type fields: %w", err)
	}

	result, err := tx.Exec("DELETE FROM asset_types WHERE id = ?", typeID)
	if err != nil {
		return fmt.Errorf("failed to delete asset type: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

// GetAssetTypeCoreByID returns only the stored fields of an asset type (no joined set name
// or asset count). Used after an update to return the fresh row.
func (r *AssetRepository) GetAssetTypeCoreByID(typeID int) (*models.AssetType, error) {
	var at models.AssetType
	err := r.db.QueryRow(`
		SELECT id, set_id, name, description, icon, color, display_order, is_active, created_at, updated_at
		FROM asset_types WHERE id = ?
	`, typeID).Scan(
		&at.ID, &at.SetID, &at.Name, &at.Description,
		&at.Icon, &at.Color, &at.DisplayOrder, &at.IsActive,
		&at.CreatedAt, &at.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to fetch asset type: %w", err)
	}
	return &at, nil
}

// FindAssetTypeFields returns the custom field assignments for an asset type with
// field metadata (name, type, description, options) joined in.
func (r *AssetRepository) FindAssetTypeFields(typeID int) ([]models.AssetTypeField, error) {
	rows, err := r.db.Query(`
		SELECT atf.id, atf.asset_type_id, atf.custom_field_id, atf.is_required, atf.display_order, atf.created_at,
		       cfd.name as field_name, cfd.field_type, cfd.description as field_description, cfd.options
		FROM asset_type_fields atf
		JOIN custom_field_definitions cfd ON atf.custom_field_id = cfd.id
		WHERE atf.asset_type_id = ?
		ORDER BY atf.display_order, cfd.name
	`, typeID)
	if err != nil {
		return nil, fmt.Errorf("failed to query asset type fields: %w", err)
	}
	defer func() { _ = rows.Close() }()

	fields := make([]models.AssetTypeField, 0)
	for rows.Next() {
		var field models.AssetTypeField
		var fieldDescription, options sql.NullString
		if err := rows.Scan(
			&field.ID, &field.AssetTypeID, &field.CustomFieldID, &field.IsRequired,
			&field.DisplayOrder, &field.CreatedAt,
			&field.FieldName, &field.FieldType, &fieldDescription, &options,
		); err != nil {
			return nil, fmt.Errorf("failed to scan asset type field: %w", err)
		}
		if fieldDescription.Valid {
			field.FieldDescription = fieldDescription.String
		}
		if options.Valid {
			field.Options = options.String
		}
		fields = append(fields, field)
	}
	return fields, nil
}

// AssetTypeFieldAssignment is the input for ReplaceAssetTypeFields.
type AssetTypeFieldAssignment struct {
	CustomFieldID int
	IsRequired    bool
	DisplayOrder  int
}

// ReplaceAssetTypeFields atomically replaces an asset type's custom field assignments.
// It deletes existing rows and inserts the provided set in a single transaction.
func (r *AssetRepository) ReplaceAssetTypeFields(typeID int, fields []AssetTypeFieldAssignment) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec("DELETE FROM asset_type_fields WHERE asset_type_id = ?", typeID); err != nil {
		return fmt.Errorf("failed to delete existing type fields: %w", err)
	}

	now := time.Now()
	for _, f := range fields {
		if _, err := tx.Exec(`
			INSERT INTO asset_type_fields (asset_type_id, custom_field_id, is_required, display_order, created_at)
			VALUES (?, ?, ?, ?, ?)
		`, typeID, f.CustomFieldID, f.IsRequired, f.DisplayOrder, now); err != nil {
			return fmt.Errorf("failed to insert type field: %w", err)
		}
	}

	return tx.Commit()
}

// scanAssetTypeRow scans a full asset type row (with nullable description and set_name)
// from any scanner (sql.Row or sql.Rows).
func scanAssetTypeRow(scanner interface {
	Scan(dest ...interface{}) error
}) (models.AssetType, error) {
	var at models.AssetType
	var description, setName sql.NullString
	if err := scanner.Scan(
		&at.ID, &at.SetID, &at.Name, &description,
		&at.Icon, &at.Color, &at.DisplayOrder,
		&at.IsActive, &at.CreatedAt, &at.UpdatedAt,
		&setName, &at.AssetCount,
	); err != nil {
		return at, err
	}
	if description.Valid {
		at.Description = description.String
	}
	if setName.Valid {
		at.SetName = setName.String
	}
	return at, nil
}

// ============================================================================
// Asset statuses
// ============================================================================

// FindAssetStatusesForSet returns all asset statuses for a set.
func (r *AssetRepository) FindAssetStatusesForSet(setID int) ([]models.AssetStatus, error) {
	rows, err := r.db.Query(`
		SELECT id, set_id, name, color, description, is_default, display_order, created_at, updated_at
		FROM asset_statuses
		WHERE set_id = ?
		ORDER BY display_order, name
	`, setID)
	if err != nil {
		return nil, fmt.Errorf("failed to query asset statuses: %w", err)
	}
	defer func() { _ = rows.Close() }()

	statuses := make([]models.AssetStatus, 0)
	for rows.Next() {
		status, err := scanAssetStatus(rows)
		if err != nil {
			return nil, err
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

// FindAssetStatusByID returns a single asset status. Returns ErrNotFound if missing.
func (r *AssetRepository) FindAssetStatusByID(statusID int) (*models.AssetStatus, error) {
	row := r.db.QueryRow(`
		SELECT id, set_id, name, color, description, is_default, display_order, created_at, updated_at
		FROM asset_statuses WHERE id = ?
	`, statusID)
	status, err := scanAssetStatus(row)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &status, nil
}

// GetAssetStatusSetID returns the owning set_id for an asset status.
func (r *AssetRepository) GetAssetStatusSetID(statusID int) (int, error) {
	var setID int
	err := r.db.QueryRow("SELECT set_id FROM asset_statuses WHERE id = ?", statusID).Scan(&setID)
	if err == sql.ErrNoRows {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get asset status set: %w", err)
	}
	return setID, nil
}

// ClearDefaultStatuses unsets is_default on every status in a set. Used before
// installing a new default to keep the uniqueness invariant.
func (r *AssetRepository) ClearDefaultStatuses(setID int) error {
	_, err := r.db.ExecWrite("UPDATE asset_statuses SET is_default = false WHERE set_id = ?", setID)
	if err != nil {
		return fmt.Errorf("failed to clear default statuses: %w", err)
	}
	return nil
}

// ClearDefaultStatusesExcept unsets is_default on every status in a set EXCEPT the given id.
// Used when promoting an existing status to default.
func (r *AssetRepository) ClearDefaultStatusesExcept(setID, statusID int) error {
	_, err := r.db.ExecWrite(
		"UPDATE asset_statuses SET is_default = false WHERE set_id = ? AND id != ?",
		setID, statusID,
	)
	if err != nil {
		return fmt.Errorf("failed to clear default statuses: %w", err)
	}
	return nil
}

// CreateAssetStatus inserts a new asset status and returns its id.
func (r *AssetRepository) CreateAssetStatus(s *models.AssetStatus) (int, error) {
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO asset_statuses (set_id, name, color, description, is_default, display_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, s.SetID, s.Name, s.Color, s.Description, s.IsDefault, s.DisplayOrder, s.CreatedAt, s.UpdatedAt).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create asset status: %w", err)
	}
	return int(id), nil
}

// AssetStatusUpdate holds the patchable fields for an asset status update.
type AssetStatusUpdate struct {
	Name         string
	Color        string
	Description  string
	DisplayOrder int
	IsDefault    *bool
}

// UpdateAssetStatus applies a patch to an asset status. Returns ErrNotFound when no row matches.
func (r *AssetRepository) UpdateAssetStatus(statusID int, patch AssetStatusUpdate) error {
	query := "UPDATE asset_statuses SET name = ?, color = ?, description = ?, display_order = ?, updated_at = ?"
	args := []interface{}{patch.Name, patch.Color, patch.Description, patch.DisplayOrder, time.Now()}

	if patch.IsDefault != nil {
		query += ", is_default = ?"
		args = append(args, *patch.IsDefault)
	}
	query += " WHERE id = ?"
	args = append(args, statusID)

	result, err := r.db.ExecWrite(query, args...)
	if err != nil {
		return fmt.Errorf("failed to update asset status: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteAssetStatus removes an asset status. Returns ErrNotFound when missing.
func (r *AssetRepository) DeleteAssetStatus(statusID int) error {
	result, err := r.db.ExecWrite("DELETE FROM asset_statuses WHERE id = ?", statusID)
	if err != nil {
		return fmt.Errorf("failed to delete asset status: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	return nil
}

// CountAssetsUsingStatus returns the number of assets currently assigned the given status.
func (r *AssetRepository) CountAssetsUsingStatus(statusID int) (int, error) {
	var count int
	err := r.db.QueryRow("SELECT COUNT(*) FROM assets WHERE status_id = ?", statusID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count assets using status: %w", err)
	}
	return count, nil
}

func scanAssetStatus(scanner interface{ Scan(...interface{}) error }) (models.AssetStatus, error) {
	var status models.AssetStatus
	var description sql.NullString
	if err := scanner.Scan(
		&status.ID, &status.SetID, &status.Name, &status.Color, &description,
		&status.IsDefault, &status.DisplayOrder, &status.CreatedAt, &status.UpdatedAt,
	); err != nil {
		return status, err
	}
	if description.Valid {
		status.Description = description.String
	}
	return status, nil
}

// ============================================================================
// CQL lookup maps
// ============================================================================

// GetCQLSetMap returns a lowercase-name → id map for asset management sets,
// used by the CQL evaluator.
func (r *AssetRepository) GetCQLSetMap() (map[string]int, error) {
	rows, err := r.db.Query("SELECT id, name FROM asset_management_sets")
	if err != nil {
		return nil, fmt.Errorf("failed to query asset sets: %w", err)
	}
	defer func() { _ = rows.Close() }()

	setMap := make(map[string]int)
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("failed to scan asset set: %w", err)
		}
		setMap[strings.ToLower(name)] = id
	}
	return setMap, nil
}

// GetCQLCustomFieldMap returns a lowercase-name → custom-field-id map for the
// custom fields attached to asset types in a set. Lets CQL queries reference
// human-readable field names even though the DB stores numeric keys.
func (r *AssetRepository) GetCQLCustomFieldMap(setID int) (map[string]int, error) {
	rows, err := r.db.Query(`
		SELECT DISTINCT cfd.id, LOWER(cfd.name)
		FROM custom_field_definitions cfd
		JOIN asset_type_fields atf ON atf.custom_field_id = cfd.id
		JOIN asset_types at2 ON atf.asset_type_id = at2.id
		WHERE at2.set_id = ?
	`, setID)
	if err != nil {
		return nil, fmt.Errorf("failed to query asset custom fields: %w", err)
	}
	defer func() { _ = rows.Close() }()

	cfMap := make(map[string]int)
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("failed to scan custom field: %w", err)
		}
		cfMap[name] = id
	}
	return cfMap, nil
}

// GetEveryoneRoleDetailed returns the everyone-default role assignment for a set,
// with the role name and granter name joined in. Returns nil (with no error) when
// no everyone role is configured for the set.
func (r *AssetRepository) GetEveryoneRoleDetailed(setID int) (*models.AssetSetEveryoneRole, error) {
	var role models.AssetSetEveryoneRole
	var roleID, grantedBy sql.NullInt64
	var roleName, grantedByName sql.NullString

	err := r.db.QueryRow(`
		SELECT aser.set_id, aser.role_id, aser.granted_by, aser.granted_at,
		       ar.name as role_name,
		       COALESCE(u.first_name || ' ' || u.last_name, u.username, '') as granted_by_name
		FROM asset_set_everyone_roles aser
		LEFT JOIN asset_roles ar ON aser.role_id = ar.id
		LEFT JOIN users u ON aser.granted_by = u.id
		WHERE aser.set_id = ?
	`, setID).Scan(&role.SetID, &roleID, &grantedBy, &role.GrantedAt, &roleName, &grantedByName)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query everyone role: %w", err)
	}

	if roleID.Valid {
		v := int(roleID.Int64)
		role.RoleID = &v
	}
	if grantedBy.Valid {
		v := int(grantedBy.Int64)
		role.GrantedBy = &v
	}
	role.RoleName = roleName.String
	role.GrantedByName = grantedByName.String
	return &role, nil
}

// ============================================================================
// Link Operations
// ============================================================================

// DeleteAssetLinks deletes all links associated with an asset
func (r *AssetRepository) DeleteAssetLinks(assetID int) error {
	_, err := r.db.ExecWrite(`
		DELETE FROM item_links
		WHERE (source_type = 'asset' AND source_id = ?)
		   OR (target_type = 'asset' AND target_id = ?)
	`, assetID, assetID)
	if err != nil {
		return fmt.Errorf("failed to delete asset links: %w", err)
	}
	return nil
}
