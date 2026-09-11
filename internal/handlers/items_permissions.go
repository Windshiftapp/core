package handlers

import (
	"fmt"
	"log/slog"

	"windshift/internal/models"
)

// The can* helpers fail closed when the permission service is unavailable, then
// delegate to the shared authz.Authz primitives so the semantic mapping lives in
// one place (mirrors WorkspaceHandler). The nil-guard stays here on purpose:
// authz falls back to a permissive legacy SQL check when its permission service
// is nil, which would defeat the fail-closed guarantee.

// canViewItem checks if a user can view an item in a specific workspace
func (h *ItemHandler) canViewItem(userID, workspaceID int) (bool, error) {
	if h.permissionService == nil {
		// Fail closed: deny access if permission service is unavailable
		slog.Error("permission service unavailable, denying view access", slog.String("component", "items_permissions"))
		return false, nil
	}
	return h.authz.CanViewWorkspace(userID, workspaceID)
}

// canEditItem checks if a user can edit an item in a specific workspace
func (h *ItemHandler) canEditItem(userID, workspaceID int) (bool, error) {
	if h.permissionService == nil {
		// Fail closed: deny access if permission service is unavailable
		slog.Error("permission service unavailable, denying edit access", slog.String("component", "items_permissions"))
		return false, nil
	}
	return h.authz.CanEditWorkspace(userID, workspaceID)
}

// filterItemsByPermissions filters a list of items based on user's workspace view permissions
func (h *ItemHandler) filterItemsByPermissions(userID int, items []models.Item) ([]models.Item, error) {
	if h.permissionService == nil {
		// Fail closed: return empty list if permission service is unavailable
		slog.Error("permission service unavailable, denying access to all items", slog.String("component", "items_permissions"))
		return []models.Item{}, nil
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

// getAccessibleWorkspaceIDs returns all workspace IDs the user can access
func (h *ItemHandler) getAccessibleWorkspaceIDs(user *models.User) ([]int, error) {
	return GetAccessibleWorkspaceIDs(user, h.db, h.permissionService)
}
