package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// PermissionRepository persists the permission catalog plus the global
// grant tables (user_global_permissions / group_global_permissions) and
// the read queries behind a user's permission summary. Cache invalidation
// and audit logging live outside this layer (the handler calls
// services.PermissionService / logger.Auditor after a mutation lands).
type PermissionRepository struct {
	db database.Database
}

// NewPermissionRepository creates a PermissionRepository.
func NewPermissionRepository(db database.Database) *PermissionRepository {
	return &PermissionRepository{db: db}
}

const permissionColumns = "id, permission_key, permission_name, description, scope, is_system, created_at, updated_at"

// SystemAdminGrantQuery matches a user's system.admin global permission,
// either granted directly or via an active group. Used by the permission
// cache and authz probes; keep aligned with handlers/auth_policy.go.
const SystemAdminGrantQuery = `
	SELECT EXISTS(
		SELECT 1 FROM user_global_permissions ugp
		JOIN permissions p ON ugp.permission_id = p.id
		WHERE ugp.user_id = ? AND p.permission_key = 'system.admin'
		UNION
		SELECT 1 FROM group_members gm
		JOIN groups g ON g.id = gm.group_id
		JOIN group_global_permissions ggp ON ggp.group_id = gm.group_id
		JOIN permissions p ON p.id = ggp.permission_id
		WHERE gm.user_id = ? AND p.permission_key = 'system.admin' AND g.is_active = true
	)
`

// GroupGlobalGrant is one row of the group_global_permissions table as
// surfaced by the admin "all group permissions" listing.
type GroupGlobalGrant struct {
	GroupID      int    `json:"group_id"`
	PermissionID int    `json:"permission_id"`
	GrantedBy    *int   `json:"granted_by"`
	GrantedAt    string `json:"granted_at"`
}

// UserEffectiveGlobalGrant is the compact effective-global-permission shape
// consumed by the permission manager. Direct and active-group grants are
// deduplicated by (user_id, permission_id).
type UserEffectiveGlobalGrant struct {
	UserID       int `json:"user_id"`
	PermissionID int `json:"permission_id"`
}

// ListAll returns every permission ordered by scope then name.
// The result is nil (not an empty slice) when the catalog is empty —
// callers that serialize it directly rely on the historical "null" output.
func (r *PermissionRepository) ListAll() ([]models.Permission, error) {
	rows, err := r.db.Query(
		"SELECT " + permissionColumns + " FROM permissions ORDER BY scope, permission_name",
	)
	if err != nil {
		return nil, fmt.Errorf("list permissions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var permissions []models.Permission
	for rows.Next() {
		var p models.Permission
		if scanErr := rows.Scan(
			&p.ID, &p.PermissionKey, &p.PermissionName,
			&p.Description, &p.Scope, &p.IsSystem,
			&p.CreatedAt, &p.UpdatedAt,
		); scanErr != nil {
			return nil, fmt.Errorf("scan permission: %w", scanErr)
		}
		permissions = append(permissions, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate permissions: %w", err)
	}
	return permissions, nil
}

// GetScope returns the scope of a permission. Returns ErrNotFound when the
// permission does not exist.
func (r *PermissionRepository) GetScope(permissionID int) (string, error) {
	var scope string
	err := r.db.QueryRow("SELECT scope FROM permissions WHERE id = ?", permissionID).Scan(&scope)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get permission scope %d: %w", permissionID, err)
	}
	return scope, nil
}

// GetKey returns a permission's permission_key.
func (r *PermissionRepository) GetKey(permissionID int) (string, error) {
	var key string
	err := r.db.QueryRow("SELECT permission_key FROM permissions WHERE id = ?", permissionID).Scan(&key)
	if err != nil {
		return "", err
	}
	return key, nil
}

// GetName returns a permission's display name (used for audit details).
func (r *PermissionRepository) GetName(permissionID int) (string, error) {
	var name string
	err := r.db.QueryRow("SELECT permission_name FROM permissions WHERE id = ?", permissionID).Scan(&name)
	if err != nil {
		return "", err
	}
	return name, nil
}

// GetUsername returns a user's username (used for audit details).
func (r *PermissionRepository) GetUsername(userID int) (string, error) {
	var username string
	err := r.db.QueryRow("SELECT username FROM users WHERE id = ?", userID).Scan(&username)
	if err != nil {
		return "", err
	}
	return username, nil
}

// GetGroupName returns a group's name (used for audit details).
func (r *PermissionRepository) GetGroupName(groupID int) (string, error) {
	var name string
	err := r.db.QueryRow("SELECT name FROM groups WHERE id = ?", groupID).Scan(&name)
	if err != nil {
		return "", err
	}
	return name, nil
}

// CountSystemAdminGrants returns how many direct user grants of the
// system.admin permission exist (guards the "last admin" revoke case).
func (r *PermissionRepository) CountSystemAdminGrants() (int, error) {
	var count int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM user_global_permissions ugp
		JOIN permissions p ON ugp.permission_id = p.id
		WHERE p.permission_key = 'system.admin'
	`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count system admin grants: %w", err)
	}
	return count, nil
}

// GrantGlobalToUser grants a global permission to a user (no-op when the
// grant already exists).
func (r *PermissionRepository) GrantGlobalToUser(userID, permissionID, grantedBy int) error {
	_, err := r.db.ExecWrite(`
		INSERT INTO user_global_permissions (user_id, permission_id, granted_by, granted_at)
		SELECT ?, ?, ?, ?
		WHERE NOT EXISTS (
			SELECT 1 FROM user_global_permissions
			WHERE user_id = ? AND permission_id = ?
		)
	`, userID, permissionID, grantedBy, time.Now(), userID, permissionID)
	if err != nil {
		return fmt.Errorf("grant global permission %d to user %d: %w", permissionID, userID, err)
	}
	return nil
}

// RevokeGlobalFromUser removes a global permission from a user and returns
// the number of rows deleted.
func (r *PermissionRepository) RevokeGlobalFromUser(userID, permissionID int) (int64, error) {
	result, err := r.db.ExecWrite(`
		DELETE FROM user_global_permissions
		WHERE user_id = ? AND permission_id = ?
	`, userID, permissionID)
	if err != nil {
		return 0, fmt.Errorf("revoke global permission %d from user %d: %w", permissionID, userID, err)
	}
	rowsAffected, _ := result.RowsAffected()
	return rowsAffected, nil
}

// GroupExists reports whether a group with the given id exists.
func (r *PermissionRepository) GroupExists(groupID int) (bool, error) {
	var exists bool
	err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM groups WHERE id = ?)", groupID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check group %d exists: %w", groupID, err)
	}
	return exists, nil
}

// GrantGlobalToGroup grants a global permission to a group (no-op when the
// grant already exists).
func (r *PermissionRepository) GrantGlobalToGroup(groupID, permissionID, grantedBy int) error {
	_, err := r.db.ExecWrite(`
		INSERT INTO group_global_permissions (group_id, permission_id, granted_by, granted_at)
		SELECT ?, ?, ?, ?
		WHERE NOT EXISTS (
			SELECT 1 FROM group_global_permissions
			WHERE group_id = ? AND permission_id = ?
		)
	`, groupID, permissionID, grantedBy, time.Now(), groupID, permissionID)
	if err != nil {
		return fmt.Errorf("grant global permission %d to group %d: %w", permissionID, groupID, err)
	}
	return nil
}

// RevokeGlobalFromGroup removes a global permission from a group and
// returns the number of rows deleted.
func (r *PermissionRepository) RevokeGlobalFromGroup(groupID, permissionID int) (int64, error) {
	result, err := r.db.ExecWrite(`
		DELETE FROM group_global_permissions
		WHERE group_id = ? AND permission_id = ?
	`, groupID, permissionID)
	if err != nil {
		return 0, fmt.Errorf("revoke global permission %d from group %d: %w", permissionID, groupID, err)
	}
	rowsAffected, _ := result.RowsAffected()
	return rowsAffected, nil
}

// GroupMemberUserIDs returns the user ids of a group's members. Rows that
// fail to scan are skipped. When iteration fails partway, the ids collected
// so far are still returned together with iterErr; queryErr is non-nil only
// when the query itself could not run. Callers use the distinction to
// preserve the historical cache-invalidation semantics (query failure is
// silent, iteration failure surfaces a warning but still invalidates the
// collected users).
func (r *PermissionRepository) GroupMemberUserIDs(groupID int) (ids []int, iterErr, queryErr error) {
	rows, err := r.db.Query("SELECT user_id FROM group_members WHERE group_id = ?", groupID)
	if err != nil {
		return nil, nil, fmt.Errorf("list group %d members: %w", groupID, err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var userID int
		if scanErr := rows.Scan(&userID); scanErr == nil {
			ids = append(ids, userID)
		}
	}
	return ids, rows.Err(), nil
}

// ListGroupGlobalGrants returns every group_global_permissions row.
// Rows that fail to scan are skipped. Always returns a non-nil slice on
// success so it serializes to [] instead of null.
func (r *PermissionRepository) ListGroupGlobalGrants() ([]GroupGlobalGrant, error) {
	rows, err := r.db.Query(`
		SELECT ggp.group_id, ggp.permission_id, ggp.granted_by, ggp.granted_at
		FROM group_global_permissions ggp
		ORDER BY ggp.group_id, ggp.permission_id
	`)
	if err != nil {
		return nil, fmt.Errorf("list group global grants: %w", err)
	}
	defer func() { _ = rows.Close() }()

	grants := make([]GroupGlobalGrant, 0)
	for rows.Next() {
		var gp GroupGlobalGrant
		if scanErr := rows.Scan(&gp.GroupID, &gp.PermissionID, &gp.GrantedBy, &gp.GrantedAt); scanErr != nil {
			continue
		}
		grants = append(grants, gp)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate group global grants: %w", err)
	}
	return grants, nil
}

// ListEffectiveUserGlobalGrants returns every user's effective global grants
// in one query. This intentionally excludes workspace permissions: the admin
// permission matrix only renders global assignments.
func (r *PermissionRepository) ListEffectiveUserGlobalGrants() ([]UserEffectiveGlobalGrant, error) {
	rows, err := r.db.Query(`
		SELECT grants.user_id, grants.permission_id
		FROM (
			SELECT ugp.user_id, ugp.permission_id
			FROM user_global_permissions ugp
			UNION
			SELECT gm.user_id, ggp.permission_id
			FROM group_members gm
			JOIN groups g ON g.id = gm.group_id AND g.is_active = true
			JOIN group_global_permissions ggp ON ggp.group_id = gm.group_id
		) grants
		ORDER BY grants.user_id, grants.permission_id
	`)
	if err != nil {
		return nil, fmt.Errorf("list effective user global grants: %w", err)
	}
	defer func() { _ = rows.Close() }()

	grants := make([]UserEffectiveGlobalGrant, 0)
	for rows.Next() {
		var grant UserEffectiveGlobalGrant
		if scanErr := rows.Scan(&grant.UserID, &grant.PermissionID); scanErr != nil {
			return nil, fmt.Errorf("scan effective user global grant: %w", scanErr)
		}
		grants = append(grants, grant)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate effective user global grants: %w", err)
	}
	return grants, nil
}

// GetUserBasic returns the basic user fields needed by the permission
// summary view.
func (r *PermissionRepository) GetUserBasic(userID int) (*models.User, error) {
	var user models.User
	err := r.db.QueryRow(`
		SELECT id, email, username, first_name, last_name, is_active
		FROM users WHERE id = ?
	`, userID).Scan(&user.ID, &user.Email, &user.Username, &user.FirstName, &user.LastName, &user.IsActive)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}
	return &user, nil
}

// ListUserGlobalGrants returns a user's direct global permission grants
// with the Permission populated. Rows that fail to scan are skipped.
func (r *PermissionRepository) ListUserGlobalGrants(userID int) ([]models.UserGlobalPermission, error) {
	rows, err := r.db.Query(`
		SELECT ugp.id, ugp.user_id, ugp.permission_id, ugp.granted_by, ugp.granted_at,
		       p.id, p.permission_key, p.permission_name, p.description, p.scope, p.is_system, p.created_at, p.updated_at
		FROM user_global_permissions ugp
		JOIN permissions p ON ugp.permission_id = p.id
		WHERE ugp.user_id = ?
		ORDER BY p.permission_name
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get global permissions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var grants []models.UserGlobalPermission
	for rows.Next() {
		var ugp models.UserGlobalPermission
		var p models.Permission
		if scanErr := rows.Scan(
			&ugp.ID, &ugp.UserID, &ugp.PermissionID, &ugp.GrantedBy, &ugp.GrantedAt,
			&p.ID, &p.PermissionKey, &p.PermissionName, &p.Description, &p.Scope, &p.IsSystem, &p.CreatedAt, &p.UpdatedAt,
		); scanErr != nil {
			continue
		}
		ugp.Permission = &p
		grants = append(grants, ugp)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return grants, nil
}

// ListUserGroupGlobalGrants returns the global permission grants a user
// inherits from active groups, with UserID set to the queried user and the
// Permission populated. Rows that fail to scan are skipped.
func (r *PermissionRepository) ListUserGroupGlobalGrants(userID int) ([]models.UserGlobalPermission, error) {
	rows, err := r.db.Query(`
		SELECT DISTINCT ggp.id, ggp.permission_id, ggp.granted_by, ggp.granted_at,
		       p.id, p.permission_key, p.permission_name, p.description, p.scope, p.is_system, p.created_at, p.updated_at
		FROM group_members gm
		JOIN group_global_permissions ggp ON gm.group_id = ggp.group_id
		JOIN permissions p ON ggp.permission_id = p.id
		JOIN groups g ON gm.group_id = g.id
		WHERE gm.user_id = ? AND g.is_active = true
		ORDER BY p.permission_name
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get group permissions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var grants []models.UserGlobalPermission
	for rows.Next() {
		var ugp models.UserGlobalPermission
		var p models.Permission
		if scanErr := rows.Scan(
			&ugp.ID, &ugp.PermissionID, &ugp.GrantedBy, &ugp.GrantedAt,
			&p.ID, &p.PermissionKey, &p.PermissionName, &p.Description, &p.Scope, &p.IsSystem, &p.CreatedAt, &p.UpdatedAt,
		); scanErr != nil {
			continue
		}
		// Set UserID to the queried user (not the group)
		ugp.UserID = userID
		ugp.Permission = &p
		grants = append(grants, ugp)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return grants, nil
}

// ListUserWorkspaceRoleGrants returns the workspace permissions a user
// holds through explicit workspace role assignments, with Permission and
// Workspace populated. Aligns with the permission cache by ignoring roles
// with permissions_enabled = false (label-only custom roles); rows that
// fail to scan are skipped.
func (r *PermissionRepository) ListUserWorkspaceRoleGrants(userID int) ([]models.UserWorkspacePermission, error) {
	rows, err := r.db.Query(`
		SELECT uwr.workspace_id, uwr.role_id, uwr.granted_by, uwr.granted_at,
		       p.id, p.permission_key, p.permission_name, p.description, p.scope, p.is_system, p.created_at, p.updated_at,
		       w.id, w.name, w.description, w.key
		FROM user_workspace_roles uwr
		JOIN workspace_roles wr ON wr.id = uwr.role_id AND wr.permissions_enabled = true
		JOIN role_permissions rp ON uwr.role_id = rp.role_id
		JOIN permissions p ON rp.permission_id = p.id
		JOIN workspaces w ON uwr.workspace_id = w.id
		WHERE uwr.user_id = ?
		ORDER BY w.name, p.permission_name
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace permissions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var grants []models.UserWorkspacePermission
	for rows.Next() {
		var uwp models.UserWorkspacePermission
		var p models.Permission
		var w models.Workspace
		var roleID int
		if scanErr := rows.Scan(
			&uwp.WorkspaceID, &roleID, &uwp.GrantedBy, &uwp.GrantedAt,
			&p.ID, &p.PermissionKey, &p.PermissionName, &p.Description, &p.Scope, &p.IsSystem, &p.CreatedAt, &p.UpdatedAt,
			&w.ID, &w.Name, &w.Description, &w.Key,
		); scanErr != nil {
			continue
		}
		uwp.UserID = userID
		uwp.PermissionID = p.ID
		uwp.Permission = &p
		uwp.Workspace = &w
		grants = append(grants, uwp)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return grants, nil
}

// ListUserWorkspacePermissionKeys returns the permission keys granted to a
// user per workspace through explicit permission-enabled role assignments,
// deduplicated into map[workspaceID]set-of-keys. It carries no display
// fields — the compact shape the permission profile serves.
func (r *PermissionRepository) ListUserWorkspacePermissionKeys(userID int) (map[int]map[string]bool, error) {
	rows, err := r.db.Query(`
		SELECT uwr.workspace_id, p.permission_key
		FROM user_workspace_roles uwr
		JOIN workspace_roles wr ON wr.id = uwr.role_id AND wr.permissions_enabled = true
		JOIN role_permissions rp ON uwr.role_id = rp.role_id
		JOIN permissions p ON rp.permission_id = p.id
		WHERE uwr.user_id = ?
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace permission keys: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make(map[int]map[string]bool)
	for rows.Next() {
		var workspaceID int
		var key string
		if scanErr := rows.Scan(&workspaceID, &key); scanErr != nil {
			continue
		}
		if result[workspaceID] == nil {
			result[workspaceID] = make(map[string]bool)
		}
		result[workspaceID][key] = true
	}
	return result, rows.Err()
}

// PermissionsByKey returns the full permission catalog keyed by
// permission_key. Rows that fail to scan are skipped and iteration errors
// are ignored — callers use this as a best-effort lookup table.
func (r *PermissionRepository) PermissionsByKey() (map[string]*models.Permission, error) {
	lookup := make(map[string]*models.Permission)
	rows, err := r.db.Query("SELECT " + permissionColumns + " FROM permissions")
	if err != nil {
		return lookup, fmt.Errorf("list permissions by key: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var p models.Permission
		if scanErr := rows.Scan(&p.ID, &p.PermissionKey, &p.PermissionName, &p.Description, &p.Scope, &p.IsSystem, &p.CreatedAt, &p.UpdatedAt); scanErr == nil {
			cp := p // copy to avoid pointer reuse
			lookup[p.PermissionKey] = &cp
		}
	}
	_ = rows.Err()
	return lookup, nil
}

// ListWorkspacesBasic returns id/name/description/key for every workspace.
// Rows that fail to scan are skipped and iteration errors are ignored —
// callers use this as a best-effort lookup table.
func (r *PermissionRepository) ListWorkspacesBasic() ([]models.Workspace, error) {
	rows, err := r.db.Query("SELECT id, name, description, key FROM workspaces")
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var workspaces []models.Workspace
	for rows.Next() {
		var w models.Workspace
		if scanErr := rows.Scan(&w.ID, &w.Name, &w.Description, &w.Key); scanErr == nil {
			workspaces = append(workspaces, w)
		}
	}
	_ = rows.Err()
	return workspaces, nil
}
