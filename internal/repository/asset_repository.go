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

// ClearDefaultSetExcept clears is_default on every set EXCEPT the provided one.
// Used when promoting an existing set to default without demoting itself.
func (r *AssetRepository) ClearDefaultSetExcept(setID int) error {
	_, err := r.db.ExecWrite(
		`UPDATE asset_management_sets SET is_default = false WHERE is_default = true AND id != ?`,
		setID,
	)
	if err != nil {
		return fmt.Errorf("failed to clear default set: %w", err)
	}
	return nil
}

// HardDeleteSet deletes a set row only, without cascading to child data.
// Callers relying on foreign-key constraints for integrity should prefer DeleteSet.
func (r *AssetRepository) HardDeleteSet(setID int) error {
	result, err := r.db.ExecWrite("DELETE FROM asset_management_sets WHERE id = ?", setID)
	if err != nil {
		return fmt.Errorf("failed to delete asset set: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	return nil
}

// GetAssetRoleIDByName returns the id of an asset role by its name (e.g. "Administrator").
func (r *AssetRepository) GetAssetRoleIDByName(name string) (int, error) {
	var id int
	err := r.db.QueryRow(`SELECT id FROM asset_roles WHERE name = ?`, name).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("failed to find asset role: %w", err)
	}
	return id, nil
}

// GetAssetSetCoreByID returns the basic set fields (no joined creator or counts),
// used after an update to return the fresh row.
func (r *AssetRepository) GetAssetSetCoreByID(setID int) (*models.AssetManagementSet, error) {
	var set models.AssetManagementSet
	err := r.db.QueryRow(`
		SELECT id, name, description, is_default, created_by, created_at, updated_at
		FROM asset_management_sets WHERE id = ?
	`, setID).Scan(&set.ID, &set.Name, &set.Description, &set.IsDefault, &set.CreatedBy, &set.CreatedAt, &set.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to fetch asset set: %w", err)
	}
	return &set, nil
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

// FindSetUserRolesByGrantDate returns user role assignments for a set, ordered by
// when they were granted (most recent first), using LEFT JOINs so orphaned rows
// (deleted user or role) still appear.
func (r *AssetRepository) FindSetUserRolesByGrantDate(setID int) ([]models.UserAssetSetRole, error) {
	rows, err := r.db.Query(`
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
		return nil, fmt.Errorf("failed to query user roles: %w", err)
	}
	defer func() { _ = rows.Close() }()

	roles := make([]models.UserAssetSetRole, 0)
	for rows.Next() {
		var role models.UserAssetSetRole
		var userName, userEmail, roleName, grantedByName sql.NullString
		if err := rows.Scan(
			&role.ID, &role.UserID, &role.SetID, &role.RoleID, &role.GrantedBy, &role.GrantedAt,
			&userName, &userEmail, &roleName, &grantedByName,
		); err != nil {
			return nil, fmt.Errorf("failed to scan user role: %w", err)
		}
		role.UserName = userName.String
		role.UserEmail = userEmail.String
		role.RoleName = roleName.String
		role.GrantedByName = grantedByName.String
		roles = append(roles, role)
	}
	return roles, nil
}

// FindSetGroupRolesByGrantDate returns group role assignments for a set ordered
// by granted_at (most recent first), using LEFT JOINs consistent with the user variant.
func (r *AssetRepository) FindSetGroupRolesByGrantDate(setID int) ([]models.GroupAssetSetRole, error) {
	rows, err := r.db.Query(`
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
		return nil, fmt.Errorf("failed to query group roles: %w", err)
	}
	defer func() { _ = rows.Close() }()

	roles := make([]models.GroupAssetSetRole, 0)
	for rows.Next() {
		var role models.GroupAssetSetRole
		var groupName, roleName, grantedByName sql.NullString
		if err := rows.Scan(
			&role.ID, &role.GroupID, &role.SetID, &role.RoleID, &role.GrantedBy, &role.GrantedAt,
			&groupName, &roleName, &grantedByName,
		); err != nil {
			return nil, fmt.Errorf("failed to scan group role: %w", err)
		}
		role.GroupName = groupName.String
		role.RoleName = roleName.String
		role.GrantedByName = grantedByName.String
		roles = append(roles, role)
	}
	return roles, nil
}

// AssetRoleExists reports whether an asset role exists.
func (r *AssetRepository) AssetRoleExists(roleID int) (bool, error) {
	var exists bool
	err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM asset_roles WHERE id = ?)", roleID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check asset role existence: %w", err)
	}
	return exists, nil
}

// DeleteUserRoleAssignment removes a specific user role assignment by id (scoped to a set).
// Returns ErrNotFound when no row matches.
func (r *AssetRepository) DeleteUserRoleAssignment(assignmentID, setID int) error {
	result, err := r.db.ExecWrite(
		"DELETE FROM user_asset_set_roles WHERE id = ? AND set_id = ?",
		assignmentID, setID,
	)
	if err != nil {
		return fmt.Errorf("failed to delete user role assignment: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteGroupRoleAssignment removes a specific group role assignment by id (scoped to a set).
func (r *AssetRepository) DeleteGroupRoleAssignment(assignmentID, setID int) error {
	result, err := r.db.ExecWrite(
		"DELETE FROM group_asset_set_roles WHERE id = ? AND set_id = ?",
		assignmentID, setID,
	)
	if err != nil {
		return fmt.Errorf("failed to delete group role assignment: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	return nil
}

// GetAssignmentRoleID returns the role_id of a specific assignment for use in
// admin-guard checks. kind is "user" or "group" (anything else treated as user).
// Returns sql.ErrNoRows when the assignment does not exist.
func (r *AssetRepository) GetAssignmentRoleID(setID, assignmentID int, kind string) (int, error) {
	var query string
	if kind == "group" {
		query = `SELECT role_id FROM group_asset_set_roles WHERE id = ? AND set_id = ?`
	} else {
		query = `SELECT role_id FROM user_asset_set_roles WHERE id = ? AND set_id = ?`
	}
	var roleID int
	err := r.db.QueryRow(query, assignmentID, setID).Scan(&roleID)
	return roleID, err
}

// GetEveryoneRoleIDValueForSet returns the everyone-role role_id for a set.
// Returns a zero NullInt64 (Valid=false) and no error when no everyone role is configured.
func (r *AssetRepository) GetEveryoneRoleIDValueForSet(setID int) (sql.NullInt64, error) {
	var roleID sql.NullInt64
	err := r.db.QueryRow(`SELECT role_id FROM asset_set_everyone_roles WHERE set_id = ?`, setID).Scan(&roleID)
	if err == sql.ErrNoRows {
		return roleID, nil
	}
	if err != nil {
		return roleID, fmt.Errorf("failed to query everyone role id: %w", err)
	}
	return roleID, nil
}

// CountAdminAssignmentsExcluding returns the count of admin role assignments
// (user + group) for a set, not counting the assignment being revoked.
// excludeKind is "user" or "group"; the assignment with the matching kind+id is skipped.
func (r *AssetRepository) CountAdminAssignmentsExcluding(setID, adminRoleID, excludeID int, excludeKind string) (int, error) {
	var count int
	err := r.db.QueryRow(`
		SELECT
			(SELECT COUNT(*) FROM user_asset_set_roles WHERE set_id = ? AND role_id = ? AND NOT (id = ? AND ? = 'user'))
			+
			(SELECT COUNT(*) FROM group_asset_set_roles WHERE set_id = ? AND role_id = ? AND NOT (id = ? AND ? = 'group'))
	`,
		setID, adminRoleID, excludeID, excludeKind,
		setID, adminRoleID, excludeID, excludeKind,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count admin assignments: %w", err)
	}
	return count, nil
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
// Asset categories
// ============================================================================

// FindAssetCategoriesForSet returns all categories for a set with set/parent names
// and a joined asset count.
func (r *AssetRepository) FindAssetCategoriesForSet(setID int) ([]models.AssetCategory, error) {
	rows, err := r.db.Query(`
		SELECT ac.id, ac.set_id, ac.name, ac.description, ac.parent_id, ac.path,
		       ac.has_children, ac.children_count, ac.descendants_count, ac.frac_index,
		       ac.created_at, ac.updated_at,
		       ams.name as set_name,
		       pc.name as parent_name,
		       (SELECT COUNT(*) FROM assets WHERE category_id = ac.id) as asset_count
		FROM asset_categories ac
		LEFT JOIN asset_management_sets ams ON ac.set_id = ams.id
		LEFT JOIN asset_categories pc ON ac.parent_id = pc.id
		WHERE ac.set_id = ?
		ORDER BY ac.frac_index, ac.name
	`, setID)
	if err != nil {
		return nil, fmt.Errorf("failed to query asset categories: %w", err)
	}
	defer func() { _ = rows.Close() }()

	categories := make([]models.AssetCategory, 0)
	for rows.Next() {
		cat, err := scanAssetCategoryRow(rows)
		if err != nil {
			return nil, err
		}
		categories = append(categories, cat)
	}
	return categories, nil
}

// FindAssetCategoryByID returns a single category with set/parent names and asset count.
func (r *AssetRepository) FindAssetCategoryByID(categoryID int) (*models.AssetCategory, error) {
	row := r.db.QueryRow(`
		SELECT ac.id, ac.set_id, ac.name, ac.description, ac.parent_id, ac.path,
		       ac.has_children, ac.children_count, ac.descendants_count, ac.frac_index,
		       ac.created_at, ac.updated_at,
		       ams.name as set_name,
		       pc.name as parent_name,
		       (SELECT COUNT(*) FROM assets WHERE category_id = ac.id) as asset_count
		FROM asset_categories ac
		LEFT JOIN asset_management_sets ams ON ac.set_id = ams.id
		LEFT JOIN asset_categories pc ON ac.parent_id = pc.id
		WHERE ac.id = ?
	`, categoryID)
	cat, err := scanAssetCategoryRow(row)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &cat, nil
}

// GetAssetCategoryCoreByID returns the basic columns for a category (no joined data)
// — used after a write to return the updated row.
func (r *AssetRepository) GetAssetCategoryCoreByID(categoryID int) (*models.AssetCategory, error) {
	row := r.db.QueryRow(`
		SELECT id, set_id, name, description, parent_id, path,
		       has_children, children_count, descendants_count, frac_index,
		       created_at, updated_at
		FROM asset_categories WHERE id = ?
	`, categoryID)
	cat, err := scanAssetCategoryCoreRow(row)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &cat, nil
}

// GetAssetCategorySetID returns the owning set_id for a category.
func (r *AssetRepository) GetAssetCategorySetID(categoryID int) (int, error) {
	var setID int
	err := r.db.QueryRow("SELECT set_id FROM asset_categories WHERE id = ?", categoryID).Scan(&setID)
	if err == sql.ErrNoRows {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get asset category set: %w", err)
	}
	return setID, nil
}

// GetAssetCategoryParentID returns the parent_id of a category (Valid=false if at root).
func (r *AssetRepository) GetAssetCategoryParentID(categoryID int) (sql.NullInt64, error) {
	var parentID sql.NullInt64
	err := r.db.QueryRow("SELECT parent_id FROM asset_categories WHERE id = ?", categoryID).Scan(&parentID)
	if err == sql.ErrNoRows {
		return parentID, ErrNotFound
	}
	if err != nil {
		return parentID, fmt.Errorf("failed to get parent id: %w", err)
	}
	return parentID, nil
}

// GetAssetCategoryDeletionInfo returns the data a delete-guard needs in one query:
// set_id, has_children flag, parent_id, and the count of assets currently in the category.
func (r *AssetRepository) GetAssetCategoryDeletionInfo(categoryID int) (setID int, hasChildren bool, parentID sql.NullInt64, assetCount int, err error) {
	err = r.db.QueryRow(`
		SELECT set_id, has_children, parent_id,
		       (SELECT COUNT(*) FROM assets WHERE category_id = ?) as asset_count
		FROM asset_categories WHERE id = ?
	`, categoryID, categoryID).Scan(&setID, &hasChildren, &parentID, &assetCount)
	if err == sql.ErrNoRows {
		err = ErrNotFound
		return
	}
	if err != nil {
		err = fmt.Errorf("failed to get category deletion info: %w", err)
	}
	return
}

// CreateAssetCategoryInput is the input for CreateAssetCategory.
type CreateAssetCategoryInput struct {
	SetID       int
	Name        string
	Description string
	ParentID    *int
}

// CreateAssetCategory inserts a new category, updates parent counts if needed,
// and returns the new category id and created_at timestamp.
func (r *AssetRepository) CreateAssetCategory(input CreateAssetCategoryInput) (int, time.Time, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now()
	var id int64
	err = tx.QueryRow(`
		INSERT INTO asset_categories (set_id, name, description, parent_id, path, created_at, updated_at)
		VALUES (?, ?, ?, ?, '/', ?, ?) RETURNING id
	`, input.SetID, input.Name, input.Description, input.ParentID, now, now).Scan(&id)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("failed to create asset category: %w", err)
	}

	if input.ParentID != nil {
		if err := updateCategoryParentCounts(tx, *input.ParentID); err != nil {
			return 0, time.Time{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, time.Time{}, fmt.Errorf("failed to commit category create: %w", err)
	}
	return int(id), now, nil
}

// UpdateAssetCategoryNameDescription patches only the name and description.
// Returns ErrNotFound when no row matches.
func (r *AssetRepository) UpdateAssetCategoryNameDescription(categoryID int, name, description string) error {
	result, err := r.db.ExecWrite(`
		UPDATE asset_categories SET name = ?, description = ?, updated_at = ?
		WHERE id = ?
	`, name, description, time.Now(), categoryID)
	if err != nil {
		return fmt.Errorf("failed to update asset category: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteAssetCategory deletes a category and refreshes its old parent's counts
// in a single transaction. Returns ErrNotFound when no row matches.
func (r *AssetRepository) DeleteAssetCategory(categoryID int, oldParentID sql.NullInt64) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.Exec("DELETE FROM asset_categories WHERE id = ?", categoryID)
	if err != nil {
		return fmt.Errorf("failed to delete asset category: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrNotFound
	}

	if oldParentID.Valid {
		if err := updateCategoryParentCounts(tx, int(oldParentID.Int64)); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// MoveAssetCategory updates a category's parent and refreshes both old and new
// parents' counts in a single transaction.
func (r *AssetRepository) MoveAssetCategory(categoryID int, oldParentID sql.NullInt64, newParentID *int) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(
		"UPDATE asset_categories SET parent_id = ?, updated_at = ? WHERE id = ?",
		newParentID, time.Now(), categoryID,
	); err != nil {
		return fmt.Errorf("failed to move asset category: %w", err)
	}

	if oldParentID.Valid {
		if err := updateCategoryParentCounts(tx, int(oldParentID.Int64)); err != nil {
			return err
		}
	}
	if newParentID != nil {
		if err := updateCategoryParentCounts(tx, *newParentID); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// IsAssetCategoryDescendantOf reports whether `potentialDescendant` is a descendant
// of `ancestor` using a recursive CTE over asset_categories.parent_id.
func (r *AssetRepository) IsAssetCategoryDescendantOf(potentialDescendant, ancestor int) (bool, error) {
	rows, err := r.db.Query(`
		WITH RECURSIVE ancestors AS (
			SELECT parent_id FROM asset_categories WHERE id = ?
			UNION ALL
			SELECT ac.parent_id FROM asset_categories ac
			INNER JOIN ancestors a ON ac.id = a.parent_id
			WHERE ac.parent_id IS NOT NULL
		)
		SELECT 1 FROM ancestors WHERE parent_id = ? LIMIT 1
	`, potentialDescendant, ancestor)
	if err != nil {
		return false, fmt.Errorf("failed to query category ancestors: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return rows.Next(), nil
}

// updateCategoryParentCounts refreshes children_count/has_children on a parent and
// re-computes descendants_count for all its ancestors. Must be called within a transaction.
func updateCategoryParentCounts(tx database.Tx, parentID int) error {
	var childrenCount int
	if err := tx.QueryRow(
		"SELECT COUNT(*) FROM asset_categories WHERE parent_id = ?",
		parentID,
	).Scan(&childrenCount); err != nil {
		return fmt.Errorf("failed to count children: %w", err)
	}

	if _, err := tx.Exec(`
		UPDATE asset_categories
		SET children_count = ?, has_children = ?, updated_at = ?
		WHERE id = ?
	`, childrenCount, childrenCount > 0, time.Now(), parentID); err != nil {
		return fmt.Errorf("failed to update parent counts: %w", err)
	}

	if _, err := tx.Exec(`
		WITH RECURSIVE ancestors AS (
			SELECT parent_id as id FROM asset_categories WHERE id = ? AND parent_id IS NOT NULL
			UNION ALL
			SELECT ac.parent_id as id FROM asset_categories ac
			INNER JOIN ancestors a ON ac.id = a.id
			WHERE ac.parent_id IS NOT NULL
		)
		UPDATE asset_categories
		SET descendants_count = (
			WITH RECURSIVE descendants AS (
				SELECT id FROM asset_categories WHERE parent_id = asset_categories.id
				UNION ALL
				SELECT ac.id FROM asset_categories ac
				INNER JOIN descendants d ON ac.parent_id = d.id
			)
			SELECT COUNT(*) FROM descendants
		)
		WHERE id IN (SELECT id FROM ancestors)
	`, parentID); err != nil {
		return fmt.Errorf("failed to update ancestor descendants: %w", err)
	}

	return nil
}

func scanAssetCategoryRow(scanner interface{ Scan(...interface{}) error }) (models.AssetCategory, error) {
	var cat models.AssetCategory
	var description, path, fracIndex, setName, parentName sql.NullString
	var parentID sql.NullInt64

	if err := scanner.Scan(
		&cat.ID, &cat.SetID, &cat.Name, &description, &parentID, &path,
		&cat.HasChildren, &cat.ChildrenCount, &cat.DescendantsCount, &fracIndex,
		&cat.CreatedAt, &cat.UpdatedAt,
		&setName, &parentName, &cat.AssetCount,
	); err != nil {
		return cat, err
	}
	cat.Description = description.String
	if parentID.Valid {
		v := int(parentID.Int64)
		cat.ParentID = &v
	}
	cat.Path = path.String
	if fracIndex.Valid {
		v := fracIndex.String
		cat.FracIndex = &v
	}
	cat.SetName = setName.String
	cat.ParentName = parentName.String
	return cat, nil
}

func scanAssetCategoryCoreRow(scanner interface{ Scan(...interface{}) error }) (models.AssetCategory, error) {
	var cat models.AssetCategory
	var description, path, fracIndex sql.NullString
	var parentID sql.NullInt64
	if err := scanner.Scan(
		&cat.ID, &cat.SetID, &cat.Name, &description, &parentID, &path,
		&cat.HasChildren, &cat.ChildrenCount, &cat.DescendantsCount, &fracIndex,
		&cat.CreatedAt, &cat.UpdatedAt,
	); err != nil {
		return cat, err
	}
	cat.Description = description.String
	if parentID.Valid {
		v := int(parentID.Int64)
		cat.ParentID = &v
	}
	cat.Path = path.String
	if fracIndex.Valid {
		v := fracIndex.String
		cat.FracIndex = &v
	}
	return cat, nil
}

// ============================================================================
// Asset CRUD
// ============================================================================

// AssetRow captures the full projection returned by the assets list/detail queries.
type AssetRow struct {
	ID                int
	SetID             int
	AssetTypeID       int
	CategoryID        sql.NullInt64
	StatusID          sql.NullInt64
	Title             string
	Description       sql.NullString
	AssetTag          sql.NullString
	CustomFieldValues sql.NullString
	FracIndex         sql.NullString
	CreatedBy         *int
	CreatedAt         time.Time
	UpdatedAt         time.Time
	SetName           sql.NullString
	AssetTypeName     sql.NullString
	AssetTypeIcon     sql.NullString
	AssetTypeColor    sql.NullString
	CategoryName      sql.NullString
	CategoryPath      sql.NullString
	StatusName        sql.NullString
	StatusColor       sql.NullString
	CreatorName       sql.NullString
	CreatorEmail      sql.NullString
	LinkedItemCount   int
}

// AssetListFilter holds the non-CQL filters for the assets list query.
type AssetListFilter struct {
	SetID                int
	AssetTypeID          string // raw string from query param (empty for no filter)
	CategoryID           string // raw string; if set and IncludeSubcategories is true, recursive CTE is used
	IncludeSubcategories bool
	StatusID             string // raw string
	Search               string
	CQLSQL               string
	CQLArgs              []interface{}
	Limit                int
	Offset               int
}

// CountAssets returns the total number of assets matching the filter.
func (r *AssetRepository) CountAssets(f AssetListFilter) (int, error) {
	cte, where, args := buildAssetListWhere(f)
	query := cte + `SELECT COUNT(*) FROM assets a
		LEFT JOIN asset_management_sets ams ON a.set_id = ams.id
		LEFT JOIN asset_types at ON a.asset_type_id = at.id
		LEFT JOIN asset_categories ac ON a.category_id = ac.id
		LEFT JOIN asset_statuses ast ON a.status_id = ast.id
		LEFT JOIN users u ON a.created_by = u.id
		` + where

	var total int
	if err := r.db.QueryRow(query, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("failed to count assets: %w", err)
	}
	return total, nil
}

// ListAssets returns a page of assets matching the filter, with all joined fields
// and the item-link count.
func (r *AssetRepository) ListAssets(f AssetListFilter) ([]AssetRow, error) {
	cte, where, args := buildAssetListWhere(f)
	args = append(args, f.Limit, f.Offset)

	query := cte + `
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
		` + where + `
		ORDER BY a.frac_index, a.title
		LIMIT ? OFFSET ?
	`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list assets: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make([]AssetRow, 0)
	for rows.Next() {
		row, err := scanAssetRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, nil
}

// FindAssetFullByID returns a single asset with all joined fields, matching the
// projection returned by ListAssets. Returns ErrNotFound when missing.
func (r *AssetRepository) FindAssetFullByID(assetID int) (*AssetRow, error) {
	row := r.db.QueryRow(`
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
	`, assetID)
	assetRow, err := scanAssetRow(row)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &assetRow, nil
}

// AssetUpdateSnapshot is what UpdateAsset needs from the existing row to detect status changes.
type AssetUpdateSnapshot struct {
	SetID       int
	StatusID    sql.NullInt64
	AssetTypeID int
}

// GetAssetUpdateSnapshot returns the fields needed by UpdateAsset before applying changes.
func (r *AssetRepository) GetAssetUpdateSnapshot(assetID int) (*AssetUpdateSnapshot, error) {
	var snap AssetUpdateSnapshot
	err := r.db.QueryRow(
		`SELECT set_id, status_id, asset_type_id FROM assets WHERE id = ?`,
		assetID,
	).Scan(&snap.SetID, &snap.StatusID, &snap.AssetTypeID)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to fetch asset snapshot: %w", err)
	}
	return &snap, nil
}

// GetAssetSetAndTitle returns the set_id and title for an asset (used by delete flows for auditing).
func (r *AssetRepository) GetAssetSetAndTitle(assetID int) (setID int, title string, err error) {
	err = r.db.QueryRow(`SELECT set_id, title FROM assets WHERE id = ?`, assetID).Scan(&setID, &title)
	if err == sql.ErrNoRows {
		err = ErrNotFound
		return
	}
	if err != nil {
		err = fmt.Errorf("failed to fetch asset set/title: %w", err)
	}
	return
}

// GetResourceSetID returns set_id from one of the asset-scoped child tables.
// `table` must be one of the allowed values to prevent SQL injection via table name.
func (r *AssetRepository) GetResourceSetID(table string, resourceID int) (int, error) {
	allowed := map[string]bool{
		"asset_types":      true,
		"asset_categories": true,
		"asset_statuses":   true,
	}
	if !allowed[table] {
		return 0, fmt.Errorf("table %q is not a valid asset-scoped resource table", table)
	}
	var setID int
	//nolint:gosec // table name validated via allowlist above
	err := r.db.QueryRow(`SELECT set_id FROM `+table+` WHERE id = ?`, resourceID).Scan(&setID)
	if err == sql.ErrNoRows {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("failed to fetch resource set id: %w", err)
	}
	return setID, nil
}

// CreateAssetInput holds the columns written by a single asset insert.
type CreateAssetInput struct {
	SetID                 int
	AssetTypeID           int
	CategoryID            *int
	StatusID              *int
	Title                 string
	Description           string
	AssetTag              string
	CustomFieldValuesJSON *string
	CreatedBy             int
	CreatedAt             time.Time
}

// CreateAsset inserts a new asset and returns its id.
func (r *AssetRepository) CreateAsset(in CreateAssetInput) (int, error) {
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO assets (set_id, asset_type_id, category_id, status_id, title, description, asset_tag, custom_field_values, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, in.SetID, in.AssetTypeID, in.CategoryID, in.StatusID, in.Title, in.Description, in.AssetTag,
		in.CustomFieldValuesJSON, in.CreatedBy, in.CreatedAt, in.CreatedAt).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create asset: %w", err)
	}
	return int(id), nil
}

// UpdateAssetInput holds the columns written by a single asset update.
type UpdateAssetInput struct {
	AssetTypeID           int
	CategoryID            *int
	StatusID              *int
	Title                 string
	Description           string
	AssetTag              string
	CustomFieldValuesJSON *string
}

// UpdateAsset writes the given columns to an asset. Returns ErrNotFound when no row matches.
func (r *AssetRepository) UpdateAsset(assetID int, in UpdateAssetInput) error {
	result, err := r.db.ExecWrite(`
		UPDATE assets
		SET asset_type_id = ?, category_id = ?, status_id = ?, title = ?, description = ?,
		    asset_tag = ?, custom_field_values = ?, updated_at = ?
		WHERE id = ?
	`, in.AssetTypeID, in.CategoryID, in.StatusID, in.Title, in.Description, in.AssetTag,
		in.CustomFieldValuesJSON, time.Now(), assetID)
	if err != nil {
		return fmt.Errorf("failed to update asset: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteAssetWithLinks deletes an asset and its item_links rows in a single transaction.
// Returns ErrNotFound when the asset does not exist.
func (r *AssetRepository) DeleteAssetWithLinks(assetID int) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(
		`DELETE FROM item_links WHERE (source_type = 'asset' AND source_id = ?) OR (target_type = 'asset' AND target_id = ?)`,
		assetID, assetID,
	); err != nil {
		return fmt.Errorf("failed to delete asset links: %w", err)
	}

	result, err := tx.Exec(`DELETE FROM assets WHERE id = ?`, assetID)
	if err != nil {
		return fmt.Errorf("failed to delete asset: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrNotFound
	}

	return tx.Commit()
}

// scanAssetRow populates an AssetRow from the full joined projection.
func scanAssetRow(scanner interface{ Scan(...interface{}) error }) (AssetRow, error) {
	var row AssetRow
	err := scanner.Scan(
		&row.ID, &row.SetID, &row.AssetTypeID, &row.CategoryID, &row.StatusID, &row.Title, &row.Description,
		&row.AssetTag, &row.CustomFieldValues, &row.FracIndex,
		&row.CreatedBy, &row.CreatedAt, &row.UpdatedAt,
		&row.SetName, &row.AssetTypeName, &row.AssetTypeIcon, &row.AssetTypeColor,
		&row.CategoryName, &row.CategoryPath, &row.StatusName, &row.StatusColor,
		&row.CreatorName, &row.CreatorEmail, &row.LinkedItemCount,
	)
	return row, err
}

// buildAssetListWhere converts the filter into a (cte-prefix, where-clause, args) triple
// shared by CountAssets and ListAssets.
func buildAssetListWhere(f AssetListFilter) (ctePrefix, whereClause string, args []interface{}) {
	whereClause = "WHERE a.set_id = ?"
	args = []interface{}{f.SetID}

	if f.AssetTypeID != "" {
		whereClause += " AND a.asset_type_id = ?"
		args = append(args, f.AssetTypeID)
	}

	if f.CategoryID != "" {
		if f.IncludeSubcategories {
			ctePrefix = `WITH RECURSIVE category_tree AS (
				SELECT id FROM asset_categories WHERE id = ?
				UNION ALL
				SELECT ac.id FROM asset_categories ac
				INNER JOIN category_tree ct ON ac.parent_id = ct.id
			) `
			whereClause += " AND a.category_id IN (SELECT id FROM category_tree)"
			// CTE parameter comes first.
			args = append([]interface{}{f.CategoryID}, args...)
		} else {
			whereClause += " AND a.category_id = ?"
			args = append(args, f.CategoryID)
		}
	}

	if f.StatusID != "" {
		whereClause += " AND a.status_id = ?"
		args = append(args, f.StatusID)
	}

	if f.Search != "" {
		whereClause += " AND (a.title LIKE ? OR a.description LIKE ? OR a.asset_tag LIKE ?)"
		term := "%" + f.Search + "%"
		args = append(args, term, term, term)
	}

	if f.CQLSQL != "" {
		whereClause += " AND (" + f.CQLSQL + ")"
		args = append(args, f.CQLArgs...)
	}
	return ctePrefix, whereClause, args
}

// ============================================================================
// Asset imports
// ============================================================================

// ImportJobRow is the projection used by import-job list/detail queries.
type ImportJobRow struct {
	JobID        string
	Status       sql.NullString
	Phase        sql.NullString
	ProgressJSON sql.NullString
	ErrorMessage sql.NullString
	CreatedAt    sql.NullTime
	StartedAt    sql.NullTime
	CompletedAt  sql.NullTime
}

// CreateImportJob inserts a new import job row in 'queued'/'initializing' state.
func (r *AssetRepository) CreateImportJob(jobID string, setID int, filePath, configJSON string, createdBy int, createdAt time.Time) error {
	_, err := r.db.ExecWrite(`
		INSERT INTO asset_import_jobs (id, set_id, status, phase, file_path, config_json, created_by, created_at)
		VALUES (?, ?, 'queued', 'initializing', ?, ?, ?, ?)
	`, jobID, setID, filePath, configJSON, createdBy, createdAt)
	if err != nil {
		return fmt.Errorf("failed to create import job: %w", err)
	}
	return nil
}

// GetImportJob returns a single import job scoped by set. Returns ErrNotFound if absent.
func (r *AssetRepository) GetImportJob(jobID string, setID int) (*ImportJobRow, error) {
	row := ImportJobRow{JobID: jobID}
	err := r.db.QueryRow(`
		SELECT status, phase, progress_json, error_message, created_at, started_at, completed_at
		FROM asset_import_jobs WHERE id = ? AND set_id = ?
	`, jobID, setID).Scan(&row.Status, &row.Phase, &row.ProgressJSON, &row.ErrorMessage, &row.CreatedAt, &row.StartedAt, &row.CompletedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get import job: %w", err)
	}
	return &row, nil
}

// ListImportJobs returns the most recent import jobs for a set (up to `limit`).
func (r *AssetRepository) ListImportJobs(setID, limit int) ([]ImportJobRow, error) {
	rows, err := r.db.Query(`
		SELECT id, status, phase, progress_json, error_message, created_at, started_at, completed_at
		FROM asset_import_jobs WHERE set_id = ? ORDER BY created_at DESC LIMIT ?
	`, setID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list import jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	jobs := make([]ImportJobRow, 0)
	for rows.Next() {
		var job ImportJobRow
		if err := rows.Scan(&job.JobID, &job.Status, &job.Phase, &job.ProgressJSON, &job.ErrorMessage, &job.CreatedAt, &job.StartedAt, &job.CompletedAt); err != nil {
			continue
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

// ListInterruptedImportJobIDs returns job ids left in 'running' or 'queued'
// state from a previous process (used at startup to reconcile orphans).
func (r *AssetRepository) ListInterruptedImportJobIDs() ([]string, error) {
	rows, err := r.db.Query(`SELECT id FROM asset_import_jobs WHERE status IN ('running', 'queued')`)
	if err != nil {
		return nil, fmt.Errorf("failed to list interrupted import jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan interrupted import job id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// DeleteAssetsFromImportJob rolls back partial inserts left by a crashed import.
func (r *AssetRepository) DeleteAssetsFromImportJob(jobID string) error {
	_, err := r.db.ExecWrite(`DELETE FROM assets WHERE import_job_id = ?`, jobID)
	if err != nil {
		return fmt.Errorf("failed to delete assets for job %s: %w", jobID, err)
	}
	return nil
}

// MarkInterruptedImportsFailed flips every running/queued job to failed and
// returns the number of jobs updated.
func (r *AssetRepository) MarkInterruptedImportsFailed(completedAt time.Time) (int, error) {
	result, err := r.db.ExecWrite(`
		UPDATE asset_import_jobs
		SET status = 'failed',
		    phase = '',
		    error_message = 'Import interrupted by server restart',
		    completed_at = ?
		WHERE status IN ('running', 'queued')
	`, completedAt)
	if err != nil {
		return 0, fmt.Errorf("failed to mark interrupted imports: %w", err)
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// ImportAssetRowInput holds the columns written by a single import row insert.
type ImportAssetRowInput struct {
	SetID                 int
	AssetTypeID           int
	CategoryID            *int
	StatusID              *int
	Title                 string
	Description           string
	AssetTag              string
	CustomFieldValuesJSON *string
	ImportJobID           string
	CreatedBy             int
	CreatedAt             time.Time
}

// InsertImportedAsset inserts a single asset row during CSV import.
func (r *AssetRepository) InsertImportedAsset(in ImportAssetRowInput) error {
	_, err := r.db.ExecWrite(`
		INSERT INTO assets (set_id, asset_type_id, category_id, status_id, title, description, asset_tag, custom_field_values, import_job_id, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, in.SetID, in.AssetTypeID, in.CategoryID, in.StatusID, in.Title, in.Description, in.AssetTag,
		in.CustomFieldValuesJSON, in.ImportJobID, in.CreatedBy, in.CreatedAt, in.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert imported asset: %w", err)
	}
	return nil
}

// GetCustomFieldTypeAndOptions reads a custom field definition's field_type and options JSON.
func (r *AssetRepository) GetCustomFieldTypeAndOptions(fieldID int) (fieldType string, options sql.NullString, err error) {
	err = r.db.QueryRow(
		`SELECT field_type, options FROM custom_field_definitions WHERE id = ?`,
		fieldID,
	).Scan(&fieldType, &options)
	if err == sql.ErrNoRows {
		err = ErrNotFound
		return
	}
	if err != nil {
		err = fmt.Errorf("failed to query custom field definition: %w", err)
	}
	return
}

// StartImportJobRunning flips a job to running and sets started_at=now.
func (r *AssetRepository) StartImportJobRunning(jobID, phase, progressJSON string) error {
	_, err := r.db.ExecWrite(
		`UPDATE asset_import_jobs SET status = 'running', phase = ?, progress_json = ?, started_at = CURRENT_TIMESTAMP WHERE id = ?`,
		phase, progressJSON, jobID,
	)
	if err != nil {
		return fmt.Errorf("failed to start import job: %w", err)
	}
	return nil
}

// FinishImportJob marks a job as completed or failed and sets completed_at=now.
// status must be "completed" or "failed".
func (r *AssetRepository) FinishImportJob(jobID, status, phase, progressJSON, errorMessage string) error {
	_, err := r.db.ExecWrite(
		`UPDATE asset_import_jobs SET status = ?, phase = ?, progress_json = ?, error_message = ?, completed_at = CURRENT_TIMESTAMP WHERE id = ?`,
		status, phase, progressJSON, errorMessage, jobID,
	)
	if err != nil {
		return fmt.Errorf("failed to finish import job: %w", err)
	}
	return nil
}

// UpdateImportJobStatus writes status/phase/progress without touching started_at/completed_at.
func (r *AssetRepository) UpdateImportJobStatus(jobID, status, phase, progressJSON string) error {
	_, err := r.db.ExecWrite(
		`UPDATE asset_import_jobs SET status = ?, phase = ?, progress_json = ? WHERE id = ?`,
		status, phase, progressJSON, jobID,
	)
	if err != nil {
		return fmt.Errorf("failed to update import job status: %w", err)
	}
	return nil
}

// UpdateImportJobProgress writes only phase and progress_json.
func (r *AssetRepository) UpdateImportJobProgress(jobID, phase, progressJSON string) error {
	_, err := r.db.ExecWrite(
		`UPDATE asset_import_jobs SET phase = ?, progress_json = ? WHERE id = ?`,
		phase, progressJSON, jobID,
	)
	if err != nil {
		return fmt.Errorf("failed to update import job progress: %w", err)
	}
	return nil
}

// ImportTypeFieldInput describes one custom field to attach to a type during
// the "create type from import" flow.
type ImportTypeFieldInput struct {
	Name         string
	FieldType    string
	OptionsJSON  *string
	IsRequired   bool
	DisplayOrder int
}

// ImportTypeFieldResult mirrors ImportTypeFieldInput plus the generated ids
// needed by the handler to build the API response.
type ImportTypeFieldResult struct {
	AssetTypeFieldID int
	CustomFieldID    int
}

// CreateAssetTypeWithFields inserts an asset type and links a set of custom fields
// (creating the custom_field_definitions rows when the name/type isn't already present).
// Everything runs in a single transaction. Returns the new type id and, for each
// input field, the generated asset_type_fields id and custom_field_id.
//
// The returned `createdAt` timestamp is the value used for every inserted row.
// On UNIQUE-constraint violations for the type name, ErrConflict is returned.
func (r *AssetRepository) CreateAssetTypeWithFields(setID int, typeCore models.AssetType, fields []ImportTypeFieldInput) (typeID int, createdAt time.Time, results []ImportTypeFieldResult, err error) {
	tx, err := r.db.Begin()
	if err != nil {
		return 0, time.Time{}, nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now()

	var typeID64 int64
	if err = tx.QueryRow(`
		INSERT INTO asset_types (set_id, name, description, icon, color, display_order, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 0, true, ?, ?) RETURNING id
	`, setID, typeCore.Name, typeCore.Description, typeCore.Icon, typeCore.Color, now, now).Scan(&typeID64); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "unique") {
			return 0, time.Time{}, nil, ErrDuplicateEntry
		}
		return 0, time.Time{}, nil, fmt.Errorf("failed to create asset type: %w", err)
	}
	typeID = int(typeID64)

	results = make([]ImportTypeFieldResult, 0, len(fields))
	for _, f := range fields {
		var cfID int
		err = tx.QueryRow(`
			SELECT id FROM custom_field_definitions
			WHERE LOWER(name) = LOWER(?) AND field_type = ?
		`, f.Name, f.FieldType).Scan(&cfID)
		if err == sql.ErrNoRows {
			if err = tx.QueryRow(`
				INSERT INTO custom_field_definitions (name, field_type, options, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?) RETURNING id
			`, f.Name, f.FieldType, f.OptionsJSON, now, now).Scan(&cfID); err != nil {
				return 0, time.Time{}, nil, fmt.Errorf("failed to create custom field definition: %w", err)
			}
		} else if err != nil {
			return 0, time.Time{}, nil, fmt.Errorf("failed to look up custom field: %w", err)
		}

		var atfID int64
		if err = tx.QueryRow(`
			INSERT INTO asset_type_fields (asset_type_id, custom_field_id, is_required, display_order, created_at)
			VALUES (?, ?, ?, ?, ?) RETURNING id
		`, typeID, cfID, f.IsRequired, f.DisplayOrder, now).Scan(&atfID); err != nil {
			return 0, time.Time{}, nil, fmt.Errorf("failed to link field to type: %w", err)
		}

		results = append(results, ImportTypeFieldResult{
			AssetTypeFieldID: int(atfID),
			CustomFieldID:    cfID,
		})
	}

	if err = tx.Commit(); err != nil {
		return 0, time.Time{}, nil, fmt.Errorf("failed to commit asset type creation: %w", err)
	}
	return typeID, now, results, nil
}

// ============================================================================
// Asset custom field resolution
// ============================================================================

// FindCustomFieldIDsByType returns the set of custom field IDs of a given field_type
// (e.g. "user", "asset") attached to an asset type via asset_type_fields.
func (r *AssetRepository) FindCustomFieldIDsByType(assetTypeID int, fieldType string) (map[int]bool, error) {
	rows, err := r.db.Query(`
		SELECT cfd.id
		FROM custom_field_definitions cfd
		JOIN asset_type_fields atf ON atf.custom_field_id = cfd.id
		WHERE atf.asset_type_id = ? AND cfd.field_type = ?
	`, assetTypeID, fieldType)
	if err != nil {
		return nil, fmt.Errorf("failed to query custom field ids by type: %w", err)
	}
	defer func() { _ = rows.Close() }()

	fieldIDs := make(map[int]bool)
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan custom field id: %w", err)
		}
		fieldIDs[id] = true
	}
	return fieldIDs, nil
}

// AssetSummary is the tiny projection used to enrich an asset-reference custom field value.
type AssetSummary struct {
	Title    string
	AssetTag string
}

// GetAssetSummary returns the title and asset_tag for an asset. Returns ErrNotFound
// when the asset does not exist (used to render a "deleted" marker).
func (r *AssetRepository) GetAssetSummary(assetID int) (*AssetSummary, error) {
	var title, assetTag sql.NullString
	err := r.db.QueryRow(`SELECT title, asset_tag FROM assets WHERE id = ?`, assetID).Scan(&title, &assetTag)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get asset summary: %w", err)
	}
	return &AssetSummary{Title: title.String, AssetTag: assetTag.String}, nil
}

// UserBasicInfo is the projection used to enrich a user-reference custom field value.
type UserBasicInfo struct {
	FirstName sql.NullString
	LastName  sql.NullString
	Email     sql.NullString
	AvatarURL sql.NullString
}

// GetUserBasicInfo returns first/last name, email, and avatar URL for a user.
// Returns ErrNotFound when the user does not exist.
func (r *AssetRepository) GetUserBasicInfo(userID int) (*UserBasicInfo, error) {
	var info UserBasicInfo
	err := r.db.QueryRow(`
		SELECT first_name, last_name, email, avatar_url
		FROM users WHERE id = ?
	`, userID).Scan(&info.FirstName, &info.LastName, &info.Email, &info.AvatarURL)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user basic info: %w", err)
	}
	return &info, nil
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
