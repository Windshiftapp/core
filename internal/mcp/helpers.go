package mcp

import (
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"windshift/internal/models"
	"windshift/internal/repository"
)

// accessibleWorkspaceIDs returns the workspace IDs the user can read.
//
// Mirrors handlers.GetAccessibleWorkspaceIDs: enumerate active workspaces
// via WorkspaceRepository, then keep only those where the user has
// item.view permission. We deliberately do NOT use
// repository.GetAccessibleWorkspaceIDs here — that helper returns every
// active non-personal workspace unconditionally, which is wrong for
// "gated" workspaces (any explicit role assignment flips a workspace into
// gated mode and a non-member should not see its contents). Re-checking
// HasWorkspacePermission per-workspace re-establishes the legacy gate
// that the deleted MCP per-family handlers enforced via canViewItem.
//
// Called once per MCP request when building the aitools.Env.
func (ms *MCPServer) accessibleWorkspaceIDs(userID int) ([]int, error) {
	ids, err := repository.NewWorkspaceRepository(ms.deps.DB).ListActiveIDs()
	if err != nil {
		return nil, err
	}
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		hasView, err := ms.deps.PermissionService.HasWorkspacePermission(userID, id, models.PermissionItemView)
		if err != nil {
			slog.Error("mcp: error checking item.view permission",
				slog.Int("workspace_id", id), slog.Int("user_id", userID), slog.Any("error", err))
			continue
		}
		if hasView {
			out = append(out, id)
		}
	}
	return out, nil
}

// errNoAuth returns a standard auth error for tool handlers.
func errNoAuth() (*mcp.CallToolResult, any, error) {
	return toolError("authentication required")
}

// errInternal returns a tool error for internal failures.
func errInternal(op string, err error) (*mcp.CallToolResult, any, error) {
	return toolErrorf("failed to %s: %v", op, err)
}
