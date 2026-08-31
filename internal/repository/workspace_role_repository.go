package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// WorkspaceRoleRepository persists workspace_roles and the user/group role
// assignment join tables (user_workspace_roles / group_workspace_roles).
// Permission-cache invalidation stays outside this layer: the handler calls
// services.PermissionService after a mutation lands.
type WorkspaceRoleRepository struct {
	db database.Database
}

// NewWorkspaceRoleRepository creates a WorkspaceRoleRepository.
func NewWorkspaceRoleRepository(db database.Database) *WorkspaceRoleRepository {
	return &WorkspaceRoleRepository{db: db}
}

const workspaceRoleSelectColumns = "id, COALESCE(builtin_key, ''), name, description, is_system, permissions_enabled, display_order, created_at, updated_at"

// List returns all workspace roles ordered by display_order then name.
func (r *WorkspaceRoleRepository) List() ([]models.WorkspaceRole, error) {
	rows, err := r.db.Query(
		"SELECT " + workspaceRoleSelectColumns + " FROM workspace_roles ORDER BY display_order ASC, name ASC",
	)
	if err != nil {
		return nil, fmt.Errorf("list workspace_roles: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var roles []models.WorkspaceRole
	for rows.Next() {
		var role models.WorkspaceRole
		if err := rows.Scan(&role.ID, &role.BuiltinKey, &role.Name, &role.Description, &role.IsSystem,
			&role.PermissionsEnabled, &role.DisplayOrder, &role.CreatedAt, &role.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan workspace_role: %w", err)
		}
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace_roles: %w", err)
	}
	if roles == nil {
		roles = []models.WorkspaceRole{}
	}
	return roles, nil
}

// GetByID returns a single workspace role's metadata (no Permissions).
// Returns ErrNotFound when missing.
func (r *WorkspaceRoleRepository) GetByID(id int) (*models.WorkspaceRole, error) {
	var role models.WorkspaceRole
	err := r.db.QueryRow(
		"SELECT "+workspaceRoleSelectColumns+" FROM workspace_roles WHERE id = ?", id,
	).Scan(&role.ID, &role.BuiltinKey, &role.Name, &role.Description, &role.IsSystem,
		&role.PermissionsEnabled, &role.DisplayOrder, &role.CreatedAt, &role.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get workspace_role %d: %w", id, err)
	}
	return &role, nil
}

// GetPermissions returns the permissions attached to a role, ordered by
// scope then name. Rows that fail to scan are skipped (legacy behavior).
// The returned slice is always non-nil.
func (r *WorkspaceRoleRepository) GetPermissions(roleID int) ([]models.Permission, error) {
	rows, err := r.db.Query(`
		SELECT p.id, p.permission_key, p.permission_name, p.description, p.scope, p.is_system, p.created_at, p.updated_at
		FROM permissions p
		JOIN role_permissions rp ON p.id = rp.permission_id
		WHERE rp.role_id = ?
		ORDER BY p.scope, p.permission_name
	`, roleID)
	if err != nil {
		return nil, fmt.Errorf("list role permissions for role %d: %w", roleID, err)
	}
	defer func() { _ = rows.Close() }()

	permissions := []models.Permission{}
	for rows.Next() {
		var perm models.Permission
		err := rows.Scan(&perm.ID, &perm.PermissionKey, &perm.PermissionName,
			&perm.Description, &perm.Scope, &perm.IsSystem, &perm.CreatedAt, &perm.UpdatedAt)
		if err == nil {
			permissions = append(permissions, perm)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate role permissions for role %d: %w", roleID, err)
	}
	return permissions, nil
}

// Exists reports whether a workspace role with the given id exists.
func (r *WorkspaceRoleRepository) Exists(roleID int) (bool, error) {
	var exists bool
	err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM workspace_roles WHERE id = ?)", roleID).Scan(&exists)
	return exists, err
}

// NameExists reports whether a workspace role with the given name exists
// (workspace_roles.name is UNIQUE).
func (r *WorkspaceRoleRepository) NameExists(name string) (bool, error) {
	var exists bool
	err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM workspace_roles WHERE name = ?)", name).Scan(&exists)
	return exists, err
}

// NameExistsExcept reports whether another workspace role uses the name.
func (r *WorkspaceRoleRepository) NameExistsExcept(name string, id int) (bool, error) {
	var exists bool
	err := r.db.QueryRow(
		"SELECT EXISTS(SELECT 1 FROM workspace_roles WHERE name = ? AND id <> ?)",
		name,
		id,
	).Scan(&exists)
	return exists, err
}

// CountManualActionRestrictions returns how many action allowlists reference
// the role. Deleting such a role would otherwise turn a restricted manual
// action into an unrestricted one.
func (r *WorkspaceRoleRepository) CountManualActionRestrictions(roleID int) (int, error) {
	var count int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM action_allowed_roles WHERE role_id = ?`, roleID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count manual action restrictions for role %d: %w", roleID, err)
	}
	return count, nil
}

// GroupExists reports whether a group with the given id exists.
func (r *WorkspaceRoleRepository) GroupExists(groupID int) (bool, error) {
	var exists bool
	err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM groups WHERE id = ?)", groupID).Scan(&exists)
	return exists, err
}

// WorkspaceActive returns the workspace's active flag.
// Returns ErrNotFound when the workspace does not exist.
func (r *WorkspaceRoleRepository) WorkspaceActive(workspaceID int) (bool, error) {
	var active bool
	err := r.db.QueryRow("SELECT active FROM workspaces WHERE id = ?", workspaceID).Scan(&active)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("get workspace %d active flag: %w", workspaceID, err)
	}
	return active, nil
}

// CountAssignmentsPreAssign returns the assignment count consulted before an
// assign operation. Preserved legacy quirk: the UNION ALL query yields two
// rows (user count, group count) but only the first row is scanned, so the
// result is effectively the user_workspace_roles count. Kept as-is so the
// "first assignment → full cache reset" trigger behaves identically.
func (r *WorkspaceRoleRepository) CountAssignmentsPreAssign(workspaceID, roleID int) (int, error) {
	var count int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM user_workspace_roles WHERE workspace_id = ? AND role_id = ?
		UNION ALL
		SELECT COUNT(*) FROM group_workspace_roles WHERE workspace_id = ? AND role_id = ?
	`, workspaceID, roleID, workspaceID, roleID).Scan(&count)
	return count, err
}

// CountAssignments returns the combined user + group assignment count for a
// role within a workspace.
func (r *WorkspaceRoleRepository) CountAssignments(workspaceID, roleID int) (int, error) {
	var count int
	err := r.db.QueryRow(`
		SELECT (SELECT COUNT(*) FROM user_workspace_roles WHERE workspace_id = ? AND role_id = ?)
		     + (SELECT COUNT(*) FROM group_workspace_roles WHERE workspace_id = ? AND role_id = ?)
	`, workspaceID, roleID, workspaceID, roleID).Scan(&count)
	return count, err
}

// AssignToUser inserts (or refreshes) a user's role assignment in a workspace.
func (r *WorkspaceRoleRepository) AssignToUser(userID, workspaceID, roleID, grantedBy int) error {
	now := time.Now()
	_, err := r.db.ExecWrite(`
		INSERT INTO user_workspace_roles (user_id, workspace_id, role_id, granted_by, granted_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_id, workspace_id, role_id) DO UPDATE SET granted_by = ?, granted_at = ?
	`, userID, workspaceID, roleID, grantedBy, now, grantedBy, now)
	if err != nil {
		return fmt.Errorf("assign role %d to user %d in workspace %d: %w", roleID, userID, workspaceID, err)
	}
	return nil
}

// RevokeFromUser deletes a user's role assignment and returns the number of
// rows removed (0 when the assignment did not exist).
func (r *WorkspaceRoleRepository) RevokeFromUser(userID, workspaceID, roleID int) (int64, error) {
	result, err := r.db.ExecWrite(`
		DELETE FROM user_workspace_roles
		WHERE user_id = ? AND workspace_id = ? AND role_id = ?
	`, userID, workspaceID, roleID)
	if err != nil {
		return 0, fmt.Errorf("revoke role %d from user %d in workspace %d: %w", roleID, userID, workspaceID, err)
	}
	rowsAffected, _ := result.RowsAffected()
	return rowsAffected, nil
}

// AssignToGroup inserts (or refreshes) a group's role assignment in a workspace.
func (r *WorkspaceRoleRepository) AssignToGroup(groupID, workspaceID, roleID, grantedBy int) error {
	now := time.Now()
	_, err := r.db.ExecWrite(`
		INSERT INTO group_workspace_roles (group_id, workspace_id, role_id, granted_by, granted_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(group_id, workspace_id, role_id) DO UPDATE SET granted_by = ?, granted_at = ?
	`, groupID, workspaceID, roleID, grantedBy, now, grantedBy, now)
	if err != nil {
		return fmt.Errorf("assign role %d to group %d in workspace %d: %w", roleID, groupID, workspaceID, err)
	}
	return nil
}

// RevokeFromGroup deletes a group's role assignment and returns the number of
// rows removed (0 when the assignment did not exist).
func (r *WorkspaceRoleRepository) RevokeFromGroup(groupID, workspaceID, roleID int) (int64, error) {
	result, err := r.db.ExecWrite(`
		DELETE FROM group_workspace_roles
		WHERE group_id = ? AND workspace_id = ? AND role_id = ?
	`, groupID, workspaceID, roleID)
	if err != nil {
		return 0, fmt.Errorf("revoke role %d from group %d in workspace %d: %w", roleID, groupID, workspaceID, err)
	}
	rowsAffected, _ := result.RowsAffected()
	return rowsAffected, nil
}

// ListUserRoles returns the roles assigned to a user in a workspace, ordered
// by display_order. Rows that fail to scan are skipped (legacy behavior).
// The returned slice is always non-nil.
func (r *WorkspaceRoleRepository) ListUserRoles(userID, workspaceID int) ([]models.WorkspaceRole, error) {
	rows, err := r.db.Query(`
		SELECT wr.id, COALESCE(wr.builtin_key, ''), wr.name, wr.description, wr.is_system, wr.display_order, wr.created_at, wr.updated_at
		FROM workspace_roles wr
		JOIN user_workspace_roles uwr ON wr.id = uwr.role_id
		WHERE uwr.user_id = ? AND uwr.workspace_id = ?
		ORDER BY wr.display_order ASC
	`, userID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list roles for user %d in workspace %d: %w", userID, workspaceID, err)
	}
	defer func() { _ = rows.Close() }()

	var roles []models.WorkspaceRole
	for rows.Next() {
		var role models.WorkspaceRole
		err := rows.Scan(&role.ID, &role.BuiltinKey, &role.Name, &role.Description, &role.IsSystem,
			&role.DisplayOrder, &role.CreatedAt, &role.UpdatedAt)
		if err == nil {
			roles = append(roles, role)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate roles for user %d in workspace %d: %w", userID, workspaceID, err)
	}
	if roles == nil {
		roles = []models.WorkspaceRole{}
	}
	return roles, nil
}

// WorkspaceRoleUserAssignment is one user↔role assignment row within a
// workspace, joined with user and role details.
type WorkspaceRoleUserAssignment struct {
	UserID          int
	Username        string
	Email           string
	FirstName       *string
	LastName        *string
	AvatarURL       *string
	RoleID          int
	RoleBuiltinKey  string
	RoleName        string
	RoleDescription string
	AssignmentID    int
	GrantedAt       time.Time
}

// ListUserAssignments returns all user role assignments for a workspace,
// ordered by username then role display_order. Rows that fail to scan are
// skipped (legacy behavior).
func (r *WorkspaceRoleRepository) ListUserAssignments(workspaceID int) ([]WorkspaceRoleUserAssignment, error) {
	rows, err := r.db.Query(`
		SELECT
			u.id, u.username, u.email, u.first_name, u.last_name, u.avatar_url,
			wr.id, COALESCE(wr.builtin_key, ''), wr.name, wr.description,
			uwr.id, uwr.granted_at
		FROM user_workspace_roles uwr
		JOIN users u ON uwr.user_id = u.id
		JOIN workspace_roles wr ON uwr.role_id = wr.id
		WHERE uwr.workspace_id = ?
		ORDER BY u.username, wr.display_order
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list user role assignments for workspace %d: %w", workspaceID, err)
	}
	defer func() { _ = rows.Close() }()

	var assignments []WorkspaceRoleUserAssignment
	for rows.Next() {
		var a WorkspaceRoleUserAssignment
		err := rows.Scan(
			&a.UserID, &a.Username, &a.Email, &a.FirstName, &a.LastName, &a.AvatarURL,
			&a.RoleID, &a.RoleBuiltinKey, &a.RoleName, &a.RoleDescription,
			&a.AssignmentID, &a.GrantedAt,
		)
		if err != nil {
			continue
		}
		assignments = append(assignments, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user role assignments for workspace %d: %w", workspaceID, err)
	}
	return assignments, nil
}

// WorkspaceRoleGroupAssignment is one group↔role assignment row within a
// workspace, joined with group and role details.
type WorkspaceRoleGroupAssignment struct {
	GroupID          int
	GroupName        string
	GroupDescription *string
	RoleID           int
	RoleBuiltinKey   string
	RoleName         string
	RoleDescription  string
	AssignmentID     int
	GrantedAt        time.Time
}

// ListGroupAssignments returns all group role assignments for a workspace,
// ordered by group name then role display_order. Rows that fail to scan are
// skipped (legacy behavior).
func (r *WorkspaceRoleRepository) ListGroupAssignments(workspaceID int) ([]WorkspaceRoleGroupAssignment, error) {
	rows, err := r.db.Query(`
		SELECT
			g.id, g.name, g.description,
			wr.id, COALESCE(wr.builtin_key, ''), wr.name, wr.description,
			gwr.id, gwr.granted_at
		FROM group_workspace_roles gwr
		JOIN groups g ON gwr.group_id = g.id
		JOIN workspace_roles wr ON gwr.role_id = wr.id
		WHERE gwr.workspace_id = ?
		ORDER BY g.name, wr.display_order
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list group role assignments for workspace %d: %w", workspaceID, err)
	}
	defer func() { _ = rows.Close() }()

	var assignments []WorkspaceRoleGroupAssignment
	for rows.Next() {
		var a WorkspaceRoleGroupAssignment
		err := rows.Scan(
			&a.GroupID, &a.GroupName, &a.GroupDescription,
			&a.RoleID, &a.RoleBuiltinKey, &a.RoleName, &a.RoleDescription,
			&a.AssignmentID, &a.GrantedAt,
		)
		if err != nil {
			continue
		}
		assignments = append(assignments, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate group role assignments for workspace %d: %w", workspaceID, err)
	}
	return assignments, nil
}

// RoleName returns the role's name for audit labels; errors yield "".
func (r *WorkspaceRoleRepository) RoleName(roleID int) string {
	var name string
	_ = r.db.QueryRow("SELECT name FROM workspace_roles WHERE id = ?", roleID).Scan(&name)
	return name
}

// Username returns the user's username for audit labels; errors yield "".
func (r *WorkspaceRoleRepository) Username(userID int) string {
	var name string
	_ = r.db.QueryRow("SELECT username FROM users WHERE id = ?", userID).Scan(&name)
	return name
}

// GroupName returns the group's name for audit labels; errors yield "".
func (r *WorkspaceRoleRepository) GroupName(groupID int) string {
	var name string
	_ = r.db.QueryRow("SELECT name FROM groups WHERE id = ?", groupID).Scan(&name)
	return name
}

// WorkspaceName returns the workspace's name for audit labels; errors yield "".
func (r *WorkspaceRoleRepository) WorkspaceName(workspaceID int) string {
	var name string
	_ = r.db.QueryRow("SELECT name FROM workspaces WHERE id = ?", workspaceID).Scan(&name)
	return name
}

// CreateCustomRole inserts a custom (label-only) workspace role with
// is_system=false and permissions_enabled=false, placed at the end of the
// display order. Returns the new role id.
func (r *WorkspaceRoleRepository) CreateCustomRole(name, description string, now time.Time) (int, error) {
	var id64 int64
	if err := r.db.QueryRow(`
		INSERT INTO workspace_roles (name, description, is_system, permissions_enabled, display_order, created_at, updated_at)
		VALUES (?, ?, false, false, COALESCE((SELECT MAX(display_order) + 1 FROM workspace_roles), 1), ?, ?)
		RETURNING id
	`, name, description, now, now).Scan(&id64); err != nil {
		return 0, fmt.Errorf("create workspace_role %q: %w", name, err)
	}
	return int(id64), nil
}

// UpdateMetadata changes only the canonical label fields.
func (r *WorkspaceRoleRepository) UpdateMetadata(id int, name, description string, now time.Time) error {
	if _, err := r.db.ExecWrite(`
		UPDATE workspace_roles
		SET name = ?, description = ?, updated_at = ?
		WHERE id = ?
	`, name, description, now, id); err != nil {
		return fmt.Errorf("update workspace_role %d: %w", id, err)
	}
	return nil
}

// AffectedUserIDs returns the set of user ids whose permission cache is
// affected by deleting a role: directly assigned users plus members of groups
// holding the role. Query errors are intentionally swallowed (cache
// invalidation is best-effort, matching legacy behavior).
func (r *WorkspaceRoleRepository) AffectedUserIDs(roleID int) map[int]bool {
	affected := make(map[int]bool)
	if rows, err := r.db.Query(`SELECT user_id FROM user_workspace_roles WHERE role_id = ?`, roleID); err == nil {
		for rows.Next() {
			var uid int
			if scanErr := rows.Scan(&uid); scanErr == nil {
				affected[uid] = true
			}
		}
		_ = rows.Err()
		_ = rows.Close()
	}
	if rows, err := r.db.Query(`
		SELECT DISTINCT gm.user_id
		FROM group_workspace_roles gwr
		JOIN group_members gm ON gm.group_id = gwr.group_id
		WHERE gwr.role_id = ?
	`, roleID); err == nil {
		for rows.Next() {
			var uid int
			if scanErr := rows.Scan(&uid); scanErr == nil {
				affected[uid] = true
			}
		}
		_ = rows.Err()
		_ = rows.Close()
	}
	return affected
}

// Delete removes a workspace role. The DELETE cascades to
// user_workspace_roles + group_workspace_roles + role_permissions via FKs.
func (r *WorkspaceRoleRepository) Delete(id int) error {
	if _, err := r.db.ExecWrite(`DELETE FROM workspace_roles WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete workspace_role %d: %w", id, err)
	}
	return nil
}
