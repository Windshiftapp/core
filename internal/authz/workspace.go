// Package authz provides shared authorization primitives used by both
// the cookie-auth API (internal/handlers, internal/middleware) and the
// bearer-token v1 API (internal/restapi/v1/handlers).
//
// The intent is single-source-of-truth for "can this user act on this
// workspace" — adding a check in one surface cannot be missed in the
// other because both surfaces call the same primitive.
//
// Token-scope checks (the bearer-token "can this token do <category>"
// gate) are NOT in scope here — they live in
// internal/restapi/v1/middleware/auth.go and are intentionally
// orthogonal to user/workspace permissions.
package authz

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

// Authz wraps the permission service and exposes the user/workspace
// authorization primitives used by HTTP handlers and middleware.
type Authz struct {
	db                database.Database
	permissionService *services.PermissionService
}

// New returns an Authz bound to the given database + permission service.
// The permission service handles its own system-admin bypass (admins
// satisfy every workspace and global permission check), so callers do
// not need to short-circuit on admin status before calling these methods.
func New(db database.Database, permissionService *services.PermissionService) *Authz {
	return &Authz{db: db, permissionService: permissionService}
}

// HasWorkspacePermission checks whether the user holds the named
// permission on the given workspace. Generic primitive — prefer the
// CanView/CanEdit convenience methods when the call site knows the
// semantic action.
func (a *Authz) HasWorkspacePermission(userID, workspaceID int, permission string) (bool, error) {
	if a.permissionService != nil {
		return a.permissionService.HasWorkspacePermission(userID, workspaceID, permission)
	}
	return a.canViewWorkspaceFallback(userID, workspaceID), nil
}

// CanViewWorkspace checks if a user can view items in a workspace.
// Equivalent to HasWorkspacePermission(userID, workspaceID, PermissionItemView).
func (a *Authz) CanViewWorkspace(userID, workspaceID int) (bool, error) {
	if a.permissionService != nil {
		return a.permissionService.HasWorkspacePermission(userID, workspaceID, models.PermissionItemView)
	}
	return a.canViewWorkspaceFallback(userID, workspaceID), nil
}

// CanEditWorkspace checks if a user can edit items in a workspace.
// Equivalent to HasWorkspacePermission(userID, workspaceID, PermissionItemEdit).
func (a *Authz) CanEditWorkspace(userID, workspaceID int) (bool, error) {
	if a.permissionService != nil {
		return a.permissionService.HasWorkspacePermission(userID, workspaceID, models.PermissionItemEdit)
	}
	return a.canEditWorkspaceFallback(userID, workspaceID)
}

// CanAdminWorkspace checks if a user can administer a workspace — manage its
// configuration (labels, work item templates, etc.). Equivalent to
// HasWorkspacePermission(userID, workspaceID, PermissionWorkspaceAdmin).
func (a *Authz) CanAdminWorkspace(userID, workspaceID int) (bool, error) {
	if a.permissionService != nil {
		return a.permissionService.HasWorkspacePermission(userID, workspaceID, models.PermissionWorkspaceAdmin)
	}
	// No permission service configured (open-by-default test setups): fall back
	// to the edit check, which is the strictest fallback available here.
	return a.canEditWorkspaceFallback(userID, workspaceID)
}

// HasGlobalPermission checks if a user has a global permission
// (e.g. PermissionMilestoneCreate, PermissionIterationManage).
func (a *Authz) HasGlobalPermission(userID int, permission string) (bool, error) {
	if a.permissionService != nil {
		return a.permissionService.HasGlobalPermission(userID, permission)
	}
	return false, nil
}

// IsSystemAdmin checks if the user has the system.admin permission.
// Most callers don't need this directly — HasWorkspacePermission and
// HasGlobalPermission already short-circuit for admins.
func (a *Authz) IsSystemAdmin(userID int) (bool, error) {
	if a.permissionService != nil {
		return a.permissionService.IsSystemAdmin(userID)
	}
	return false, nil
}

// GetAccessibleWorkspaceIDs returns the IDs of active workspaces the user can
// view. It is gated-aware: a workspace flipped into gated mode (by any explicit
// role assignment) is hidden from non-members, matching the cookie-API
// handlers.GetAccessibleWorkspaceIDs path. The bearer-token v1 API relies on
// this list as the sole workspace filter when listing/searching items, so it
// MUST honor gated mode — the ungated repository.GetAccessibleWorkspaceIDs is
// only used as a fallback when the permission service is unavailable (test paths).
func (a *Authz) GetAccessibleWorkspaceIDs(userID int) ([]int, error) {
	if a.permissionService != nil {
		return a.permissionService.AccessibleWorkspaceIDs(userID)
	}
	return repository.GetAccessibleWorkspaceIDs(a.db, userID)
}

// CanViewWorkspaceTx applies the same item.view rule as CanViewWorkspace, but
// reads through the caller's transaction so template-source visibility and the
// cloned snapshot come from one consistent database state. It resolves agent
// ownership, system-admin grants, explicit user/group role permissions, and
// the open-role "everyone" fallback exactly like the permission cache build.
func (a *Authz) CanViewWorkspaceTx(ctx context.Context, tx database.Tx, userID, workspaceID int) (bool, error) {
	// Owned agents inherit their owner's permissions.
	var ownerID sql.NullInt64
	var isAgent sql.NullBool
	err := tx.QueryRowContext(ctx,
		"SELECT COALESCE(is_agent, false), agent_owner_user_id FROM users WHERE id = ?", userID,
	).Scan(&isAgent, &ownerID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("resolve template access user: %w", err)
	}
	if isAgent.Valid && isAgent.Bool && ownerID.Valid {
		userID = int(ownerID.Int64)
	}

	// System admins pass every workspace check.
	var hasSystemAdmin bool
	err = tx.QueryRowContext(ctx, repository.SystemAdminGrantQuery, userID, userID).Scan(&hasSystemAdmin)
	if err != nil {
		return false, fmt.Errorf("check template access system admin: %w", err)
	}
	if hasSystemAdmin {
		return true, nil
	}

	// Explicit user or active-group role grants carrying item.view.
	var hasRolePerm bool
	err = tx.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM user_workspace_roles uwr
			JOIN workspace_roles wr ON wr.id = uwr.role_id AND wr.permissions_enabled = true
			JOIN role_permissions rp ON rp.role_id = uwr.role_id
			JOIN permissions p ON p.id = rp.permission_id
			WHERE uwr.workspace_id = ? AND uwr.user_id = ? AND p.permission_key = ?
			UNION
			SELECT 1 FROM group_workspace_roles gwr
			JOIN group_members gm ON gm.group_id = gwr.group_id
			JOIN groups g ON g.id = gm.group_id AND g.is_active = true
			JOIN workspace_roles wr ON wr.id = gwr.role_id AND wr.permissions_enabled = true
			JOIN role_permissions rp ON rp.role_id = gwr.role_id
			JOIN permissions p ON p.id = rp.permission_id
			WHERE gwr.workspace_id = ? AND gm.user_id = ? AND p.permission_key = ?
		)
	`, workspaceID, userID, models.PermissionItemView, workspaceID, userID, models.PermissionItemView).Scan(&hasRolePerm)
	if err != nil {
		return false, fmt.Errorf("check template access role permission: %w", err)
	}
	if hasRolePerm {
		return true, nil
	}

	// Open-role fallback: only active, non-personal workspaces grant the
	// unassigned Viewer/Editor/Tester union to everyone.
	var active, isPersonal bool
	err = tx.QueryRowContext(ctx,
		"SELECT active, COALESCE(is_personal, false) FROM workspaces WHERE id = ?", workspaceID,
	).Scan(&active, &isPersonal)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load template access workspace: %w", err)
	}
	if !active || isPersonal {
		return false, nil
	}

	roleHasPermission := func(roleKey string) (bool, error) {
		var has int
		err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM role_permissions rp
			JOIN permissions p ON p.id = rp.permission_id
			JOIN workspace_roles wr ON wr.id = rp.role_id
			WHERE wr.builtin_key = ? AND p.permission_key = ?
		`, roleKey, models.PermissionItemView).Scan(&has)
		return has > 0, err
	}
	roleAssigned := func(roleKey string) (bool, error) {
		var assigned int
		err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM workspace_roles wr
			WHERE wr.builtin_key = ? AND wr.id IN (
				SELECT role_id FROM user_workspace_roles WHERE workspace_id = ?
				UNION
				SELECT role_id FROM group_workspace_roles WHERE workspace_id = ?
			)
		`, roleKey, workspaceID, workspaceID).Scan(&assigned)
		return assigned > 0, err
	}

	viewerAssigned, err := roleAssigned(models.RoleBuiltinViewer)
	if err != nil {
		return false, fmt.Errorf("check template access viewer gating: %w", err)
	}
	if viewerAssigned {
		return false, nil
	}
	viewerHas, err := roleHasPermission(models.RoleBuiltinViewer)
	if err != nil {
		return false, fmt.Errorf("check template access viewer permission: %w", err)
	}
	if viewerHas {
		return true, nil
	}

	editorAssigned, err := roleAssigned(models.RoleBuiltinEditor)
	if err != nil {
		return false, fmt.Errorf("check template access editor gating: %w", err)
	}
	if !editorAssigned {
		editorHas, err := roleHasPermission(models.RoleBuiltinEditor)
		if err != nil {
			return false, fmt.Errorf("check template access editor permission: %w", err)
		}
		if editorHas {
			return true, nil
		}
		testerAssigned, err := roleAssigned(models.RoleBuiltinTester)
		if err != nil {
			return false, fmt.Errorf("check template access tester gating: %w", err)
		}
		if !testerAssigned {
			testerHas, err := roleHasPermission(models.RoleBuiltinTester)
			if err != nil {
				return false, fmt.Errorf("check template access tester permission: %w", err)
			}
			return testerHas, nil
		}
	}
	return false, nil
}

// canViewWorkspaceFallback runs the legacy SQL check used when the
// permission service is not available (e.g. some test paths).
func (a *Authz) canViewWorkspaceFallback(userID, workspaceID int) bool {
	var exists int
	err := a.db.QueryRow(`
		SELECT 1 FROM workspaces w
		LEFT JOIN user_workspace_roles uwr ON w.id = uwr.workspace_id AND uwr.user_id = ?
		LEFT JOIN (
			SELECT DISTINCT gwr.workspace_id
			FROM group_workspace_roles gwr
			JOIN group_members gm ON gwr.group_id = gm.group_id
			WHERE gm.user_id = ?
		) grp ON w.id = grp.workspace_id
		WHERE w.id = ? AND (
			(w.active = true AND (w.is_personal = false OR w.is_personal IS NULL))
			OR uwr.role_id IS NOT NULL
			OR grp.workspace_id IS NOT NULL
			OR (w.is_personal = true AND w.owner_id = ?)
		)
	`, userID, userID, workspaceID, userID).Scan(&exists)
	return err == nil
}
func (a *Authz) canEditWorkspaceFallback(userID, workspaceID int) (bool, error) {
	var hasPermission int
	err := a.db.QueryRow(`
		SELECT 1 FROM user_workspace_roles uwr
		JOIN workspace_roles wr ON uwr.role_id = wr.id
		WHERE uwr.workspace_id = ? AND uwr.user_id = ? AND wr.builtin_key IN (?, ?)
		UNION
		SELECT 1 FROM group_workspace_roles gwr
		JOIN workspace_roles wr ON gwr.role_id = wr.id
		JOIN group_members gm ON gwr.group_id = gm.group_id
		WHERE gwr.workspace_id = ? AND gm.user_id = ? AND wr.builtin_key IN (?, ?)
		UNION
		SELECT 1 FROM workspaces WHERE id = ? AND is_personal = true AND owner_id = ?
		LIMIT 1
	`, workspaceID, userID, models.RoleBuiltinEditor, models.RoleBuiltinAdministrator,
		workspaceID, userID, models.RoleBuiltinEditor, models.RoleBuiltinAdministrator,
		workspaceID, userID).Scan(&hasPermission)
	if err != nil {
		return false, err
	}
	return hasPermission == 1, nil
}
