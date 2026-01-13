package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"windshift/internal/models"
)

// getUserFromContext extracts the user from the request context
func (h *ItemHandler) getUserFromContext(r *http.Request) *models.User {
	if user := r.Context().Value("user"); user != nil {
		if u, ok := user.(*models.User); ok {
			return u
		}
	}
	return nil
}

// canViewItem checks if a user can view an item in a specific workspace
func (h *ItemHandler) canViewItem(userID, workspaceID int) (bool, error) {
	if h.permissionService == nil {
		// If permission service is not available, allow access (backward compatibility)
		return true, nil
	}

	return h.permissionService.HasWorkspacePermission(userID, workspaceID, models.PermissionItemView)
}

// canEditItem checks if a user can edit an item in a specific workspace
func (h *ItemHandler) canEditItem(userID, workspaceID int) (bool, error) {
	if h.permissionService == nil {
		return true, nil
	}

	return h.permissionService.HasWorkspacePermission(userID, workspaceID, models.PermissionItemEdit)
}

// canDeleteItem checks if a user can delete an item in a specific workspace
func (h *ItemHandler) canDeleteItem(userID, workspaceID int) (bool, error) {
	if h.permissionService == nil {
		return true, nil
	}

	return h.permissionService.HasWorkspacePermission(userID, workspaceID, models.PermissionItemDelete)
}

// filterItemsByPermissions filters a list of items based on user's workspace view permissions
func (h *ItemHandler) filterItemsByPermissions(userID int, items []models.Item) ([]models.Item, error) {
	if h.permissionService == nil {
		// No permission service, return all items (backward compatibility)
		return items, nil
	}

	// Check if user is system admin - they can see everything
	isAdmin, err := h.permissionService.IsSystemAdmin(userID)
	if err != nil {
		return nil, fmt.Errorf("error checking admin status: %w", err)
	}
	if isAdmin {
		return items, nil
	}

	// Build a map of workspace IDs to permission check results
	workspacePermissions := make(map[int]bool)

	// Filter items based on permissions
	filteredItems := make([]models.Item, 0, len(items))
	for _, item := range items {
		// Check cache first
		canView, exists := workspacePermissions[item.WorkspaceID]
		if !exists {
			// Check permission for this workspace
			canView, err = h.canViewItem(userID, item.WorkspaceID)
			if err != nil {
				slog.Error("error checking view permission for workspace", slog.String("component", "items_permissions"), slog.Int("workspace_id", item.WorkspaceID), slog.Any("error", err))
				canView = false
			}
			workspacePermissions[item.WorkspaceID] = canView
		}

		if canView {
			filteredItems = append(filteredItems, item)
		}
	}

	return filteredItems, nil
}

// canAccessInactiveWorkspace checks if a user can access an inactive workspace
// System admins and workspace admins can access inactive workspaces
func (h *ItemHandler) canAccessInactiveWorkspace(user *models.User, workspaceID int) (bool, error) {
	if h.permissionService == nil {
		return false, nil
	}

	// System admins can always access inactive workspaces
	isSystemAdmin, err := h.permissionService.IsSystemAdmin(user.ID)
	if err == nil && isSystemAdmin {
		return true, nil
	}

	// Check if user has workspace admin permission for this specific workspace
	return h.permissionService.HasWorkspacePermission(user.ID, workspaceID, models.PermissionWorkspaceAdmin)
}

// getAccessibleWorkspaceIDs returns all workspace IDs the user can access
// This includes active workspaces and inactive workspaces where user has admin access
func (h *ItemHandler) getAccessibleWorkspaceIDs(user *models.User) ([]int, error) {
	if user == nil {
		return []int{}, nil
	}

	// Get all workspaces
	rows, err := h.db.Query(`SELECT id, active FROM workspaces`)
	if err != nil {
		return nil, fmt.Errorf("failed to query workspaces: %w", err)
	}
	defer rows.Close()

	accessibleIDs := []int{}
	for rows.Next() {
		var id int
		var active bool
		if err := rows.Scan(&id, &active); err != nil {
			return nil, fmt.Errorf("failed to scan workspace: %w", err)
		}

		allowed := false

		if active {
			// Active workspaces: rely on permission service (Everyone fast path) when available
			if h.permissionService != nil {
				hasView, err := h.permissionService.HasWorkspacePermission(user.ID, id, models.PermissionItemView)
				if err != nil {
					slog.Error("error checking view permission for workspace", slog.String("component", "items_permissions"), slog.Int("workspace_id", id), slog.Any("error", err))
				} else if hasView {
					allowed = true
				}
			} else {
				// Fallback to legacy open access if permission service is unavailable
				allowed = true
			}
		} else {
			// Inactive workspaces still require admin or workspace admin access
			canAccess, err := h.canAccessInactiveWorkspace(user, id)
			if err != nil {
				slog.Error("error checking access to inactive workspace", slog.String("component", "items_permissions"), slog.Int("workspace_id", id), slog.Any("error", err))
			} else if canAccess {
				allowed = true
			}
		}

		if allowed {
			accessibleIDs = append(accessibleIDs, id)
		}
	}

	return accessibleIDs, rows.Err()
}
