// Package aitools provides canonical agent tools shared by in-app chat and MCP
// adapters through one registry.
package aitools

import (
	"fmt"
	"log/slog"

	"windshift/internal/database"
	"windshift/internal/itemevents"
	"windshift/internal/logger"
	"windshift/internal/services"
)

// Source identifies which adapter is invoking a tool. Used by mutating tools
// to tag audit-log entries so cookie-auth writes (HTTP handlers) can be
// distinguished from agent-driven writes (chat / MCP).
const (
	SourceAIChat        = "ai_chat"
	SourceMCP           = "mcp"
	SourceStandardAgent = "standard_agent"
)

// Env provides tools their caller, services, and readable workspaces. Tools
// must gate workspace data through AccessibleWorkspaceIDs regardless of adapter.
type Env struct {
	DB                     database.Database
	UserID                 int
	Username               string // Cached at Env-construction time for audit logs
	Timezone               string // Validated IANA timezone for the acting user
	Source                 string // SourceAIChat | SourceMCP — for audit trail
	AccessibleWorkspaceIDs []int
	// AuditDetails contains adapter-supplied correlation identifiers only.
	// Raw tool arguments and results must never be placed here.
	AuditDetails map[string]any

	PermService     *services.PermissionService
	TimePermService *services.TimePermissionService
	TimerService    *services.TimerService
	CommentService  *services.CommentService
	ApprovalService *services.ApprovalService
	// ItemDeletionService is the fully wired user-facing destructive pipeline.
	// MCP receives the instance shared with cookie and REST v1; chat embeddings
	// may leave it nil and tools construct a side-effect-light fallback.
	ItemDeletionService *services.ItemDeletionApplicationService
	// PageApplicationService is the shared permission-aware page mutation
	// pipeline. MCP receives the production instance used by both HTTP
	// surfaces; chat embeddings may use the nil-safe fallback in pages.go.
	PageApplicationService *services.PageApplicationService
	// PageDiagramService owns immutable Page-diagram attachment mutations.
	PageDiagramService *services.PageDiagramService
	// ActionService is the optional cache-invalidation hook for tools that
	// create or mutate actions. Nil-safe: tools must check before calling
	// InvalidateWorkspaceCache so they degrade to "next periodic refresh"
	// when the adapter (chat handler / MCP server) wasn't wired with one.
	ActionService *services.ActionService
}

// Audit resource types for entities internal/logger doesn't define a
// Resource* constant for (their HTTP surfaces don't write central audit rows
// yet). Kept here so every aitools call site spells them identically.
const (
	resourceDiagram     = "diagram"
	resourceItemLink    = "item_link"
	resourceTimeWorklog = "time_worklog"
	resourceTimer       = "timer"
	resourceTestResult  = "test_result"
)

// AuditWrite records an agent-driven mutation in the central audit log.
// entityType is the audit resource type (logger.Resource* where one exists,
// e.g. "item", "comment", "page"), entityID the mutated row's ID, toolName
// the aitools tool that performed the write (stored as the action type so
// agent writes are queryable per tool), and summary a human-readable
// resource name. The Source field on Env tags which surface initiated the
// write (ai_chat | mcp) via details.source, so the audit trail can
// distinguish agent writes from cookie-auth writes. Best-effort: failures
// are logged, never propagated, so a successful tool call is not broken by
// an audit miss (same policy as the diagram history writes).
func (e *Env) AuditWrite(entityType string, entityID int, toolName, summary string) {
	e.audit(toolName, entityType, entityID, summary, nil)
}

// audit is the single audit choke point for aitools writes — AuditWrite and
// actions.go's emitActionAudit both funnel through it. IP/UserAgent are
// empty by design: the call originates from the chat/MCP adapter, not a
// direct HTTP request.
func (e *Env) audit(actionType, resourceType string, resourceID int, resourceName string, details map[string]any) {
	if details == nil {
		details = map[string]any{}
	}
	for key, value := range e.AuditDetails {
		details[key] = value
	}
	source := e.Source
	if source == "" {
		source = "unknown"
	}
	details["source"] = source
	id := resourceID
	err := logger.LogAudit(e.DB, logger.AuditEvent{
		UserID:       e.UserID,
		Username:     e.Username,
		ActionType:   actionType,
		ResourceType: resourceType,
		ResourceID:   &id,
		ResourceName: resourceName,
		Details:      details,
		Success:      true,
	})
	if err != nil {
		slog.Warn("aitool audit log failed",
			slog.String("component", "aitools"),
			slog.String("action_type", actionType),
			slog.String("resource_type", resourceType),
			slog.Int("resource_id", resourceID),
			slog.Any("error", err),
		)
	}
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

// eventMetadata describes this adapter's writes as agent-actor provenance, so
// the durable event store and the item-history feed can both answer "an agent
// did this on the user's behalf, through which surface?". ActorRef carries the
// human the agent acts as — the identity every tool's permission check already
// used, so the audit trail attributes the change to the same principal.
//
// Source falls back to "agent" when an embedder left it unset: writing
// "application" would be a lie (this is not a direct UI click) and would make
// the history feed's source column non-empty but unmatchable.
func (e *Env) eventMetadata() itemevents.Metadata {
	source := e.Source
	if source == "" {
		source = "agent"
	}
	return itemevents.Agent(fmt.Sprintf("user:%d", e.UserID), source)
}
