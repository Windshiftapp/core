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
		LEFT JOIN workspace_permissions wp ON w.id = wp.workspace_id AND wp.user_id = ?
		WHERE w.id = ? AND (w.active = 1 OR wp.role = 'admin' OR (w.is_personal = 1 AND w.owner_id = ?))
	`, userID, workspaceID, userID).Scan(&exists)
	return err == nil, nil
}

// CanEditWorkspace checks if a user can edit items in a workspace
func (p *PermissionHelper) CanEditWorkspace(userID, workspaceID int) (bool, error) {
	if p.permissionService != nil {
		return p.permissionService.HasWorkspacePermission(userID, workspaceID, "edit_items")
	}
	// Fallback query when permission service is not available
	var role string
	err := p.db.QueryRow(`
		SELECT wp.role FROM workspace_permissions wp
		WHERE wp.workspace_id = ? AND wp.user_id = ?
	`, workspaceID, userID).Scan(&role)
	if err != nil {
		return false, err
	}
	return role == "admin" || role == "editor", nil
}

// GetAccessibleWorkspaceIDs returns all workspace IDs the user can access
func (p *PermissionHelper) GetAccessibleWorkspaceIDs(userID int) ([]int, error) {
	// Query for accessible workspaces based on permissions
	rows, err := p.db.Query(`
		SELECT DISTINCT w.id
		FROM workspaces w
		LEFT JOIN workspace_permissions wp ON w.id = wp.workspace_id AND wp.user_id = ?
		WHERE w.active = 1
		   OR (w.active = 0 AND wp.role = 'admin')
		   OR (w.is_personal = 1 AND w.owner_id = ?)
	`, userID, userID)
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
