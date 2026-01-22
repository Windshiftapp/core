package handlers

import (
	"fmt"
	"windshift/internal/models"
)

// Helper functions for permission checking

// canViewWorkspace checks if a user can view a workspace (has item.view permission)
func (h *WorkspaceHandler) canViewWorkspace(userID, workspaceID int) (bool, error) {
	if h.permissionService == nil {
		// If permission service is not available, allow access (backward compatibility)
		return true, nil
	}

	// HasWorkspacePermission now handles checking if workspace has restrictions
	// and returns true if workspace has no restrictions (accessible to all)
	return h.permissionService.HasWorkspacePermission(userID, workspaceID, models.PermissionItemView)
}

// canAdminWorkspace checks if a user can administer a workspace (has workspace.admin permission)
func (h *WorkspaceHandler) canAdminWorkspace(userID, workspaceID int) (bool, error) {
	if h.permissionService == nil {
		// If permission service is not available, allow access (backward compatibility)
		return true, nil
	}

	return h.permissionService.HasWorkspacePermission(userID, workspaceID, models.PermissionWorkspaceAdmin)
}

// canAccessInactiveWorkspace checks if a user can access an inactive workspace
// Returns true if user is system admin OR has workspace.admin permission for the workspace
func (h *WorkspaceHandler) canAccessInactiveWorkspace(user *models.User, workspaceID int) (bool, error) {
	// System admins can always access inactive workspaces
	if h.permissionService != nil {
		isSystemAdmin, err := h.permissionService.IsSystemAdmin(user.ID)
		if err == nil && isSystemAdmin {
			return true, nil
		}
	}

	// Check if user has workspace admin permission for this specific workspace
	if h.permissionService == nil {
		// If permission service is not available, deny access to inactive workspaces for non-system-admins
		return false, nil
	}

	return h.permissionService.HasWorkspacePermission(user.ID, workspaceID, models.PermissionWorkspaceAdmin)
}

// canCreateWorkspace checks if a user can create workspaces (has global workspace.create permission)
func (h *WorkspaceHandler) canCreateWorkspace(userID int) (bool, error) {
	if h.permissionService == nil {
		// If permission service is not available, allow access (backward compatibility)
		return true, nil
	}

	return h.permissionService.HasGlobalPermission(userID, models.PermissionWorkspaceCreate)
}

// filterWorkspacesByPermissions filters a list of workspaces based on user's view permissions
func (h *WorkspaceHandler) filterWorkspacesByPermissions(userID int, workspaces []models.Workspace) ([]models.Workspace, error) {
	if h.permissionService == nil {
		// No permission service, return all workspaces (backward compatibility)
		return workspaces, nil
	}

	// Filter workspaces based on permissions
	// HasWorkspacePermission handles checking if workspace has restrictions
	filtered := make([]models.Workspace, 0)
	for _, ws := range workspaces {
		// IMPORTANT: For inactive workspaces, pass them through for now
		// They will be filtered later by canAccessInactiveWorkspace
		// We need to include them here so they can be checked properly
		if !ws.Active {
			// Include inactive workspaces in the filtered list
			// They will be filtered out later unless user has admin access
			filtered = append(filtered, ws)
			continue
		}

		// For active workspaces, check normal view permission
		hasPermission, err := h.permissionService.HasWorkspacePermission(userID, ws.ID, models.PermissionItemView)
		if err != nil {
			return nil, fmt.Errorf("error checking permission for workspace %d: %w", ws.ID, err)
		}
		if hasPermission {
			filtered = append(filtered, ws)
		}
	}

	return filtered, nil
}
