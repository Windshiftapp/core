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

// GetAccessibleWorkspaceIDs returns all workspace IDs the user can access.
func (a *Authz) GetAccessibleWorkspaceIDs(userID int) ([]int, error) {
	return repository.GetAccessibleWorkspaceIDs(a.db, userID)
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

// canEditWorkspaceFallback runs the legacy SQL check used when the
// permission service is not available.
func (a *Authz) canEditWorkspaceFallback(userID, workspaceID int) (bool, error) {
	var hasPermission int
	err := a.db.QueryRow(`
		SELECT 1 FROM user_workspace_roles uwr
		JOIN workspace_roles wr ON uwr.role_id = wr.id
		WHERE uwr.workspace_id = ? AND uwr.user_id = ? AND wr.name IN ('Editor', 'Administrator')
		UNION
		SELECT 1 FROM group_workspace_roles gwr
		JOIN workspace_roles wr ON gwr.role_id = wr.id
		JOIN group_members gm ON gwr.group_id = gm.group_id
		WHERE gwr.workspace_id = ? AND gm.user_id = ? AND wr.name IN ('Editor', 'Administrator')
		UNION
		SELECT 1 FROM workspaces WHERE id = ? AND is_personal = true AND owner_id = ?
		LIMIT 1
	`, workspaceID, userID, workspaceID, userID, workspaceID, userID).Scan(&hasPermission)
	if err != nil {
		return false, err
	}
	return hasPermission == 1, nil
}
