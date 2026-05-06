package mcp

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"windshift/internal/repository"
)

// accessibleWorkspaceIDs returns the workspace IDs the user can read.
// Used by the registry adapter when building an aitools.Env per-call.
func (ms *MCPServer) accessibleWorkspaceIDs(userID int) ([]int, error) {
	return repository.GetAccessibleWorkspaceIDs(ms.deps.DB, userID)
}

// errNoAuth returns a standard auth error for tool handlers.
func errNoAuth() (*mcp.CallToolResult, any, error) {
	return toolError("authentication required")
}

// errInternal returns a tool error for internal failures.
func errInternal(op string, err error) (*mcp.CallToolResult, any, error) {
	return toolErrorf("failed to %s: %v", op, err)
}
