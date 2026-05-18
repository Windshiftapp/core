// Package aitools holds the canonical implementations of every tool exposed
// to AI agents. Both the in-app LLM agent (internal/handlers/ai_tools.go)
// and the external MCP server (internal/mcp) are thin adapters over this
// package — they translate protocol-specific request shapes into an Env +
// typed Args, dispatch through the Registry, and translate the returned
// value back to their protocol's response shape. A new tool registers in
// one place here and is automatically available on both surfaces.
package aitools

import (
	"windshift/internal/database"
	"windshift/internal/services"
)

// Env carries everything a tool's Run function needs: the database handle,
// the calling user, their accessible workspace IDs (pre-computed by the
// adapter), and the services it might call into.
//
// AccessibleWorkspaceIDs is the canonical permission gate — tools that
// touch workspace-scoped data must check membership against this set
// before returning anything. The list is populated differently per
// adapter (LLM pre-computes once at chat-start; MCP loads per-call from
// context) but the contract is the same: only IDs the caller can read.
type Env struct {
	DB                     database.Database
	UserID                 int
	AccessibleWorkspaceIDs []int

	PermService     *services.PermissionService
	TimePermService *services.TimePermissionService
	TimerService    *services.TimerService
	CommentService  *services.CommentService
	ApprovalService *services.ApprovalService
	// ActionService is the optional cache-invalidation hook for tools that
	// create or mutate actions. Nil-safe: tools must check before calling
	// InvalidateWorkspaceCache so they degrade to "next periodic refresh"
	// when the adapter (chat handler / MCP server) wasn't wired with one.
	ActionService *services.ActionService
}

// HasWorkspaceAccess reports whether the caller can touch the given workspace.
func (e *Env) HasWorkspaceAccess(workspaceID int) bool {
	for _, id := range e.AccessibleWorkspaceIDs {
		if id == workspaceID {
			return true
		}
	}
	return false
}
