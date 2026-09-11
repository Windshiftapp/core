package handlers

import (
	"log/slog"
)

// Helper functions for permission checking

// The can* helpers fail closed when the permission service is unavailable, then
// delegate the actual permission resolution to the shared authz.Authz primitives
// so the semantic mapping (view→item.view, admin→workspace.admin, …) lives in one
// place. The nil-guard stays here on purpose: authz falls back to a permissive
// legacy SQL check when its permission service is nil, which would defeat the
// fail-closed guarantee these handlers rely on.

// canViewWorkspace checks if a user can view a workspace (has item.view permission)
func (h *WorkspaceHandler) canViewWorkspace(userID, workspaceID int) (bool, error) {
	if h.permissionService == nil {
		slog.Error("permission service unavailable, denying access", slog.String("component", "workspaces"))
		return false, nil
	}
	return h.authz.CanViewWorkspace(userID, workspaceID)
}
