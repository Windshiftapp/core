package shared

import (
	"windshift/internal/database"
	"windshift/internal/services"
)

// PermissionHelper provides common permission checking methods for REST API handlers
type PermissionHelper struct {
	db                database.Database
	permissionService *services.PermissionService
}

// NewPermissionHelper creates a new permission helper
func NewPermissionHelper(db database.Database, permissionService *services.PermissionService) *PermissionHelper {
	return &PermissionHelper{
		db:                db,
		permissionService: permissionService,
	}
}

// CanViewWorkspace checks if a user can view items in a workspace
func (p *PermissionHelper) CanViewWorkspace(userID, workspaceID int) (bool, error) {
	if p.permissionService != nil {
		return p.permissionService.HasWorkspacePermission(userID, workspaceID, "view_items")
	}
	// Fallback query when permission service is not available
	var exists int
	err := p.db.QueryRow(`
		SELECT 1 FROM workspaces w
		LEFT JOIN user_workspace_roles uwr ON w.id = uwr.workspace_id AND uwr.user_id = ?
		LEFT JOIN (
			SELECT DISTINCT gwr.workspace_id
			FROM group_workspace_roles gwr
			JOIN group_members gm ON gwr.group_id = gm.group_id
			WHERE gm.user_id = ?
		) grp ON w.id = grp.workspace_id
		WHERE w.id = ? AND (
			w.active = 1
			OR uwr.role_id IS NOT NULL
			OR grp.workspace_id IS NOT NULL
			OR (w.is_personal = 1 AND w.owner_id = ?)
		)
	`, userID, userID, workspaceID, userID).Scan(&exists)
	return err == nil, nil
}

// CanEditWorkspace checks if a user can edit items in a workspace
func (p *PermissionHelper) CanEditWorkspace(userID, workspaceID int) (bool, error) {
	if p.permissionService != nil {
		return p.permissionService.HasWorkspacePermission(userID, workspaceID, "edit_items")
	}
	// Fallback query when permission service is not available
	// Check for Editor or Administrator role via direct assignment or group membership
	var hasPermission int
	err := p.db.QueryRow(`
		SELECT 1 FROM user_workspace_roles uwr
		JOIN workspace_roles wr ON uwr.role_id = wr.id
		WHERE uwr.workspace_id = ? AND uwr.user_id = ? AND wr.name IN ('Editor', 'Administrator')
		UNION
		SELECT 1 FROM group_workspace_roles gwr
		JOIN workspace_roles wr ON gwr.role_id = wr.id
		JOIN group_members gm ON gwr.group_id = gm.group_id
		WHERE gwr.workspace_id = ? AND gm.user_id = ? AND wr.name IN ('Editor', 'Administrator')
		LIMIT 1
	`, workspaceID, userID, workspaceID, userID).Scan(&hasPermission)
	if err != nil {
		return false, nil // No edit permission found
	}
	return hasPermission == 1, nil
}

// HasGlobalPermission checks if a user has a global permission
func (p *PermissionHelper) HasGlobalPermission(userID int, permission string) (bool, error) {
	if p.permissionService != nil {
		return p.permissionService.HasGlobalPermission(userID, permission)
	}
	return false, nil
}

// GetAccessibleWorkspaceIDs returns all workspace IDs the user can access
func (p *PermissionHelper) GetAccessibleWorkspaceIDs(userID int) ([]int, error) {
	// Query for accessible workspaces based on RBAC role assignments
	rows, err := p.db.Query(`
		SELECT DISTINCT w.id
		FROM workspaces w
		LEFT JOIN user_workspace_roles uwr ON w.id = uwr.workspace_id AND uwr.user_id = ?
		LEFT JOIN (
			SELECT DISTINCT gwr.workspace_id
			FROM group_workspace_roles gwr
			JOIN group_members gm ON gwr.group_id = gm.group_id
			WHERE gm.user_id = ?
		) grp ON w.id = grp.workspace_id
		WHERE w.active = 1
		   OR (w.active = 0 AND uwr.role_id IS NOT NULL)
		   OR (w.active = 0 AND grp.workspace_id IS NOT NULL)
		   OR (w.is_personal = 1 AND w.owner_id = ?)
	`, userID, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
