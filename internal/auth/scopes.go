package auth

import (
	"errors"
	"fmt"
	"strings"
)

var ErrAgentScopesNotPermitted = errors.New("scopes not permitted for coding-agent tokens")

// Granular resource:action scope constants
const (
	// Items (includes comments, attachments, history)
	ScopeItemsRead   = "items:read"
	ScopeItemsWrite  = "items:write"
	ScopeItemsDelete = "items:delete"

	// Workspaces
	ScopeWorkspacesRead   = "workspaces:read"
	ScopeWorkspacesWrite  = "workspaces:write"
	ScopeWorkspacesDelete = "workspaces:delete"

	// Workspace configuration
	ScopeStatusesRead     = "statuses:read"
	ScopeStatusesWrite    = "statuses:write"
	ScopeWorkflowsRead    = "workflows:read"
	ScopeWorkflowsWrite   = "workflows:write"
	ScopeItemTypesRead    = "item-types:read"
	ScopeItemTypesWrite   = "item-types:write"
	ScopePrioritiesRead   = "priorities:read"
	ScopePrioritiesWrite  = "priorities:write"
	ScopeCustomFieldsRead = "custom-fields:read"

	// Users
	ScopeUsersRead = "users:read"

	// Per-user preferences (the token owner's own preferences document,
	// e.g. the SSH TUI's theme/layout persistence)
	ScopeUserPreferencesRead  = "user-preferences:read"
	ScopeUserPreferencesWrite = "user-preferences:write"

	// Milestones
	ScopeMilestonesRead   = "milestones:read"
	ScopeMilestonesWrite  = "milestones:write"
	ScopeMilestonesDelete = "milestones:delete"

	// Iterations
	ScopeIterationsRead   = "iterations:read"
	ScopeIterationsWrite  = "iterations:write"
	ScopeIterationsDelete = "iterations:delete"

	// MCP — single binary scope; the MCP server exposes both read and write
	// tools so we don't split this into :read/:write. It only opens the
	// transport: every tool still declares the resource scopes it needs and
	// they are enforced at dispatch (internal/mcp/tools_registry.go).
	ScopeMCPAccess = "mcp:access"

	// Collections
	ScopeCollectionsRead   = "collections:read"
	ScopeCollectionsWrite  = "collections:write"
	ScopeCollectionsDelete = "collections:delete"

	// Actions (automation graphs — node-catalog discovery, action CRUD).
	// :write is required for create/update; :read covers catalog + list/get.
	// Both are additionally gated in-handler by the workspace-scoped
	// action.manage permission, which only the workspace Administrator role
	// carries and which is never granted implicitly on open workspaces — so
	// holding the scope alone never permits editing another workspace's
	// automations.
	ScopeActionsRead  = "actions:read"
	ScopeActionsWrite = "actions:write"

	// Cross-resource links. Entity-specific link reads use the entity scope;
	// these scopes cover link-type discovery and generic search/mutations.
	ScopeLinksRead  = "links:read"
	ScopeLinksWrite = "links:write"

	// Pages (workspace knowledge / wiki). :write covers create + edit + move;
	// :delete is required for archive. Per-page ACLs still apply in-handler.
	ScopePagesRead   = "pages:read"
	ScopePagesWrite  = "pages:write"
	ScopePagesDelete = "pages:delete"

	// Test management (folders, cases, labels, sets, runs, run templates,
	// reports, result↔item links). :write covers mutations; the per-workspace
	// test.view / test.execute / test.manage role gates individual actions
	// in-handler.
	ScopeTestsRead  = "tests:read"
	ScopeTestsWrite = "tests:write"

	// Assets (sets, types, categories, statuses, asset entities). :write
	// covers mutations to asset entities and CSV import; :delete is required
	// for hard-delete. Mutating sets / types / categories / statuses / actions
	// stays admin-UI-only. The per-set asset role (Viewer / Editor /
	// Administrator, asset.view / edit / delete / admin keys) still gates
	// individual actions in-handler via AssetPermissionService — token scope
	// alone never grants access to a set the user can't see.
	ScopeAssetsRead   = "assets:read"
	ScopeAssetsWrite  = "assets:write"
	ScopeAssetsDelete = "assets:delete"

	// Time tracking (worklogs, timers, time projects). :write covers create/edit;
	// :delete is required for worklog deletion. Per-project membership is enforced
	// in-handler via TimePermService — token scope alone never grants access to a
	// project the user isn't a member of.
	ScopeTimeRead   = "time:read"
	ScopeTimeWrite  = "time:write"
	ScopeTimeDelete = "time:delete"

	// Agent skills (WI-258): the per-workspace library of markdown knowledge
	// packs a run's prompt indexes and the agent reads via `ws skill get`.
	// Authoring additionally requires the workspace administration permission;
	// run tokens remain read-only because their allowlist omits :write.
	ScopeAgentSkillsRead  = "agent-skills:read"
	ScopeAgentSkillsWrite = "agent-skills:write"

	// Approvals. Workflow configuration uses workflows:*; these scopes cover
	// live approval requests and decisions.
	ScopeApprovalsRead  = "approvals:read"
	ScopeApprovalsWrite = "approvals:write"

	// Work item templates (WI-438): workspace-defined reusable bodies that
	// pre-fill a new item's description. :read covers list/get (agents discover
	// the scaffold a type enforces); :write covers admin/programmatic CRUD.
	ScopeItemTemplatesRead  = "item-templates:read"
	ScopeItemTemplatesWrite = "item-templates:write"

	// Admin scopes (require system admin role AND scope on token)
	ScopeAdminUsersRead         = "admin:users:read"
	ScopeAdminUsersWrite        = "admin:users:write"
	ScopeAdminGroupsRead        = "admin:groups:read"
	ScopeAdminGroupsWrite       = "admin:groups:write"
	ScopeAdminAuditLogsRead     = "admin:audit-logs:read"
	ScopeAdminAPITokensRead     = "admin:api-tokens:read"
	ScopeAdminAPITokensWrite    = "admin:api-tokens:write"
	ScopeAdminTranslationsRead  = "admin:object-translations:read"
	ScopeAdminTranslationsWrite = "admin:object-translations:write"
)

// ScopeInfo describes one entry in the token scope catalog. Every surface that
// grants, validates, or renders a scope reads it from here so the pickers, the
// OAuth consent screens, and the server-side allowlist cannot drift apart —
// the drift that previously left time:* ungrantable from the token UI (WI-957).
type ScopeInfo struct {
	Scope       string `json:"scope"`
	Resource    string `json:"resource"`
	Action      string `json:"action"`
	Label       string `json:"label"`
	Description string `json:"description"`
	// ResourceLabel names the resource group a picker renders this scope
	// under. Every scope sharing a Resource repeats the same label.
	ResourceLabel string `json:"resource_label"`
	// Admin marks scopes that additionally require the system admin role.
	Admin bool `json:"admin"`
	// AgentDefault marks membership in DefaultAgentScopes — the set granted
	// to agent/CLI/MCP tokens minted without an explicit scope list.
	AgentDefault bool `json:"agent_default"`
}

// scopeCatalog is the authoritative, ordered list of every scope the server
// recognizes. AllValidScopes and DefaultAgentScopes are both derived from it,
// and GET /api-tokens/scope-catalog serves it to the frontend pickers.
//
// Ordering is presentation order: resources are grouped, reads precede writes
// precede deletes, and admin scopes come last.
var scopeCatalog = []ScopeInfo{
	{Scope: ScopeItemsRead, Resource: "items", ResourceLabel: "Items", Action: "read", Label: "Read items", Description: "Read work items, comments, attachments, and history.", AgentDefault: true},
	{Scope: ScopeItemsWrite, Resource: "items", ResourceLabel: "Items", Action: "write", Label: "Create and update items", Description: "Create and update work items, comments, and attachments.", AgentDefault: true},
	{Scope: ScopeItemsDelete, Resource: "items", ResourceLabel: "Items", Action: "delete", Label: "Delete items", Description: "Delete work items and comments. Destructive — opt in deliberately."},

	{Scope: ScopeWorkspacesRead, Resource: "workspaces", ResourceLabel: "Workspaces", Action: "read", Label: "Read workspaces", Description: "Read workspaces and their configuration.", AgentDefault: true},
	{Scope: ScopeWorkspacesWrite, Resource: "workspaces", ResourceLabel: "Workspaces", Action: "write", Label: "Modify workspaces", Description: "Create and update workspaces and their configuration.", AgentDefault: true},
	{Scope: ScopeWorkspacesDelete, Resource: "workspaces", ResourceLabel: "Workspaces", Action: "delete", Label: "Delete workspaces", Description: "Delete workspaces. Destructive — opt in deliberately."},

	{Scope: ScopePagesRead, Resource: "pages", ResourceLabel: "Pages", Action: "read", Label: "Read pages", Description: "Read knowledge pages. Per-page ACLs still apply.", AgentDefault: true},
	{Scope: ScopePagesWrite, Resource: "pages", ResourceLabel: "Pages", Action: "write", Label: "Create and update pages", Description: "Create, edit, and move knowledge pages.", AgentDefault: true},
	{Scope: ScopePagesDelete, Resource: "pages", ResourceLabel: "Pages", Action: "delete", Label: "Archive pages", Description: "Archive knowledge pages.", AgentDefault: true},

	{Scope: ScopeTimeRead, Resource: "time", ResourceLabel: "Time tracking", Action: "read", Label: "Read time entries", Description: "Read worklogs, timers, and time projects you are a member of.", AgentDefault: true},
	{Scope: ScopeTimeWrite, Resource: "time", ResourceLabel: "Time tracking", Action: "write", Label: "Log time", Description: "Create and edit worklogs, and start or stop timers.", AgentDefault: true},
	{Scope: ScopeTimeDelete, Resource: "time", ResourceLabel: "Time tracking", Action: "delete", Label: "Delete worklogs", Description: "Delete worklogs. Destructive — opt in deliberately."},

	{Scope: ScopeTestsRead, Resource: "tests", ResourceLabel: "Test management", Action: "read", Label: "Read tests", Description: "Read test cases, sets, runs, and results.", AgentDefault: true},
	{Scope: ScopeTestsWrite, Resource: "tests", ResourceLabel: "Test management", Action: "write", Label: "Manage tests", Description: "Create and update test cases, runs, and results.", AgentDefault: true},

	{Scope: ScopeAssetsRead, Resource: "assets", ResourceLabel: "Assets", Action: "read", Label: "Read assets", Description: "Read asset sets, types, and entities you have a role on.", AgentDefault: true},
	{Scope: ScopeAssetsWrite, Resource: "assets", ResourceLabel: "Assets", Action: "write", Label: "Update assets", Description: "Create and update asset entities, including CSV import.", AgentDefault: true},
	{Scope: ScopeAssetsDelete, Resource: "assets", ResourceLabel: "Assets", Action: "delete", Label: "Delete assets", Description: "Hard-delete asset entities. Destructive — opt in deliberately."},

	{Scope: ScopeActionsRead, Resource: "actions", ResourceLabel: "Automations", Action: "read", Label: "Read automations", Description: "Read automation graphs and the node catalog. Requires the workspace Administrator role.", AgentDefault: true},
	{Scope: ScopeActionsWrite, Resource: "actions", ResourceLabel: "Automations", Action: "write", Label: "Edit automations", Description: "Create and update automations. Requires the workspace Administrator role.", AgentDefault: true},

	{Scope: ScopeLinksRead, Resource: "links", ResourceLabel: "Links", Action: "read", Label: "Discover links", Description: "Read link types and search resources that can be linked.", AgentDefault: true},
	{Scope: ScopeLinksWrite, Resource: "links", ResourceLabel: "Links", Action: "write", Label: "Manage links", Description: "Create and remove links between accessible resources.", AgentDefault: true},

	{Scope: ScopeMilestonesRead, Resource: "milestones", ResourceLabel: "Milestones", Action: "read", Label: "Read milestones", Description: "Read milestones and their progress.", AgentDefault: true},
	{Scope: ScopeMilestonesWrite, Resource: "milestones", ResourceLabel: "Milestones", Action: "write", Label: "Manage milestones", Description: "Create and update milestones."},
	{Scope: ScopeMilestonesDelete, Resource: "milestones", ResourceLabel: "Milestones", Action: "delete", Label: "Delete milestones", Description: "Delete milestones. Destructive — opt in deliberately."},

	{Scope: ScopeIterationsRead, Resource: "iterations", ResourceLabel: "Iterations", Action: "read", Label: "Read iterations", Description: "Read iterations and their contents.", AgentDefault: true},
	{Scope: ScopeIterationsWrite, Resource: "iterations", ResourceLabel: "Iterations", Action: "write", Label: "Manage iterations", Description: "Create and update iterations."},
	{Scope: ScopeIterationsDelete, Resource: "iterations", ResourceLabel: "Iterations", Action: "delete", Label: "Delete iterations", Description: "Delete iterations. Destructive — opt in deliberately."},

	{Scope: ScopeItemTemplatesRead, Resource: "item-templates", ResourceLabel: "Work item templates", Action: "read", Label: "Read templates", Description: "Read the reusable bodies that pre-fill new items.", AgentDefault: true},
	{Scope: ScopeItemTemplatesWrite, Resource: "item-templates", ResourceLabel: "Work item templates", Action: "write", Label: "Manage templates", Description: "Create and update work item templates."},

	{Scope: ScopeCollectionsRead, Resource: "collections", ResourceLabel: "Collections", Action: "read", Label: "Read collections", Description: "Read collections and their reports."},
	{Scope: ScopeCollectionsWrite, Resource: "collections", ResourceLabel: "Collections", Action: "write", Label: "Manage collections", Description: "Create, update, and share collections."},
	{Scope: ScopeCollectionsDelete, Resource: "collections", ResourceLabel: "Collections", Action: "delete", Label: "Delete collections", Description: "Delete owned collections. Destructive — opt in deliberately."},

	{Scope: ScopeUsersRead, Resource: "users", ResourceLabel: "Users", Action: "read", Label: "Read user directory", Description: "Read the user directory.", AgentDefault: true},

	{Scope: ScopeUserPreferencesRead, Resource: "user-preferences", ResourceLabel: "Your preferences", Action: "read", Label: "Read your preferences", Description: "Read the token owner's own preferences document."},
	{Scope: ScopeUserPreferencesWrite, Resource: "user-preferences", ResourceLabel: "Your preferences", Action: "write", Label: "Update your preferences", Description: "Update the token owner's own preferences document."},

	{Scope: ScopeStatusesRead, Resource: "statuses", ResourceLabel: "Statuses", Action: "read", Label: "Read statuses", Description: "Read workspace statuses.", AgentDefault: true},
	{Scope: ScopeStatusesWrite, Resource: "statuses", ResourceLabel: "Statuses", Action: "write", Label: "Manage statuses", Description: "Create, update, and delete statuses and status categories."},
	{Scope: ScopeWorkflowsRead, Resource: "workflows", ResourceLabel: "Workflows", Action: "read", Label: "Read workflows", Description: "Read workflows, transitions, conditions, and approval-set configuration.", AgentDefault: true},
	{Scope: ScopeWorkflowsWrite, Resource: "workflows", ResourceLabel: "Workflows", Action: "write", Label: "Manage workflows", Description: "Create, update, and delete workflows, transitions, conditions, and approval-set configuration."},
	{Scope: ScopeItemTypesRead, Resource: "item-types", ResourceLabel: "Item types", Action: "read", Label: "Read item types", Description: "Read the configured item types.", AgentDefault: true},
	{Scope: ScopeItemTypesWrite, Resource: "item-types", ResourceLabel: "Item types", Action: "write", Label: "Manage item types", Description: "Create, update, and delete item types."},
	{Scope: ScopePrioritiesRead, Resource: "priorities", ResourceLabel: "Priorities", Action: "read", Label: "Read priorities", Description: "Read the configured priorities.", AgentDefault: true},
	{Scope: ScopePrioritiesWrite, Resource: "priorities", ResourceLabel: "Priorities", Action: "write", Label: "Manage priorities", Description: "Create, update, and delete priorities."},
	{Scope: ScopeCustomFieldsRead, Resource: "custom-fields", ResourceLabel: "Custom fields", Action: "read", Label: "Read custom fields", Description: "Read custom field definitions.", AgentDefault: true},
	{Scope: ScopeAgentSkillsRead, Resource: "agent-skills", ResourceLabel: "Agent skills", Action: "read", Label: "Read agent skills", Description: "Read the workspace library of agent knowledge packs.", AgentDefault: true},
	{Scope: ScopeAgentSkillsWrite, Resource: "agent-skills", ResourceLabel: "Agent skills", Action: "write", Label: "Manage agent skills", Description: "Create, update, and delete workspace agent skills."},

	{Scope: ScopeApprovalsRead, Resource: "approvals", ResourceLabel: "Approvals", Action: "read", Label: "Read approvals", Description: "Read approval requests visible to the token owner."},
	{Scope: ScopeApprovalsWrite, Resource: "approvals", ResourceLabel: "Approvals", Action: "write", Label: "Act on approvals", Description: "Decide, cancel, delegate, refresh, or escalate approval requests."},

	{Scope: ScopeMCPAccess, Resource: "mcp", ResourceLabel: "MCP server", Action: "access", Label: "Connect to the MCP server", Description: "Open the MCP transport. Each tool still requires the resource scopes above.", AgentDefault: true},

	{Scope: ScopeAdminUsersRead, Resource: "admin:users", ResourceLabel: "Users (admin)", Action: "read", Label: "Read all users", Description: "Read every user account.", Admin: true},
	{Scope: ScopeAdminUsersWrite, Resource: "admin:users", ResourceLabel: "Users (admin)", Action: "write", Label: "Manage all users", Description: "Create, update, and deactivate user accounts.", Admin: true},
	{Scope: ScopeAdminGroupsRead, Resource: "admin:groups", ResourceLabel: "Groups (admin)", Action: "read", Label: "Read all groups", Description: "Read every group and its membership.", Admin: true},
	{Scope: ScopeAdminGroupsWrite, Resource: "admin:groups", ResourceLabel: "Groups (admin)", Action: "write", Label: "Manage all groups", Description: "Create, update, and delete groups.", Admin: true},
	{Scope: ScopeAdminAuditLogsRead, Resource: "admin:audit-logs", ResourceLabel: "Audit logs (admin)", Action: "read", Label: "Read audit logs", Description: "Read the central audit log.", Admin: true},
	{Scope: ScopeAdminAPITokensRead, Resource: "admin:api-tokens", ResourceLabel: "API tokens (admin)", Action: "read", Label: "Read all API tokens", Description: "List API tokens belonging to any user.", Admin: true},
	{Scope: ScopeAdminAPITokensWrite, Resource: "admin:api-tokens", ResourceLabel: "API tokens (admin)", Action: "write", Label: "Revoke all API tokens", Description: "Revoke API tokens belonging to any user.", Admin: true},
	{Scope: ScopeAdminTranslationsRead, Resource: "admin:object-translations", ResourceLabel: "Object translations (admin)", Action: "read", Label: "Read object translations", Description: "Read instance-wide labels for configurable objects.", Admin: true},
	{Scope: ScopeAdminTranslationsWrite, Resource: "admin:object-translations", ResourceLabel: "Object translations (admin)", Action: "write", Label: "Manage object translations", Description: "Create, update, and delete instance-wide labels for configurable objects.", Admin: true},
}

// ScopeCatalog returns a copy of the scope catalog in presentation order.
func ScopeCatalog() []ScopeInfo {
	return append([]ScopeInfo(nil), scopeCatalog...)
}

// AllValidScopes is the complete set of valid scope strings, derived from the
// catalog so a new scope becomes grantable everywhere by adding one row above.
var AllValidScopes = allScopes()

// DefaultAgentScopes is the scope set granted when a token is minted without
// an explicit permissions list, and the set the OAuth flows request by default
// so `ws` CLI tokens and MCP tokens land on the same capabilities (WI-957).
// Tuned for agent workflows: full items + workspace + pages access, all
// non-admin planning reads, time tracking, automations, and MCP access.
// Excludes admin scopes and every :delete except pages — mint with an explicit
// permissions list when you need those.
var DefaultAgentScopes = defaultAgentScopes()

func allScopes() []string {
	out := make([]string, 0, len(scopeCatalog))
	for _, s := range scopeCatalog {
		out = append(out, s.Scope)
	}
	return out
}

func defaultAgentScopes() []string {
	out := make([]string, 0, len(scopeCatalog))
	for _, s := range scopeCatalog {
		if s.AgentDefault {
			out = append(out, s.Scope)
		}
	}
	return out
}

// DefaultCodingAgentRunScopes is the narrowed scope set granted to short-lived
// per-run coding-agent tokens when a binding does not explicitly request
// scopes. It is intentionally smaller than DefaultAgentScopes: no workspace
// write, no page deletion, no assets write, no tests write, and no automation
// authoring. Page writes let agents turn completed work into workspace
// documentation while the acting identity's page permissions remain enforced.
var DefaultCodingAgentRunScopes = []string{
	ScopeItemsRead, ScopeItemsWrite,
	ScopeLinksRead, ScopeLinksWrite,
	ScopeWorkspacesRead,
	ScopeUsersRead,
	ScopeItemTypesRead, ScopeWorkflowsRead,
	ScopeStatusesRead, ScopePrioritiesRead, ScopeCustomFieldsRead,
	ScopeItemTemplatesRead,
	ScopeMilestonesRead, ScopeIterationsRead,
	ScopePagesRead, ScopePagesWrite,
	ScopeTestsRead,
	ScopeTimeRead, ScopeTimeWrite,
	ScopeAgentSkillsRead,
	// mcp:access is safe to grant since WI-351: the MCP server enforces
	// each tool's required token scopes at dispatch, so this narrowed set
	// holds on the MCP surface too — e.g. a run token (no pages:delete,
	// no items:delete, no actions:*) is refused page deletion, item deletion,
	// and automation tools there just like on the v1 REST surface.
	ScopeMCPAccess,
}

// DefaultCodingAgentPrivateTestScopes is the read-only token surface for
// ephemeral Coding/Legacy profile verification. The MCP scope remains
// available so the normal tool transport can be exercised, while every
// resource-specific write/delete scope is deliberately absent.
var DefaultCodingAgentPrivateTestScopes = []string{
	ScopeItemsRead,
	ScopeLinksRead,
	ScopeWorkspacesRead,
	ScopeUsersRead,
	ScopeItemTypesRead, ScopeWorkflowsRead,
	ScopeStatusesRead, ScopePrioritiesRead, ScopeCustomFieldsRead,
	ScopeItemTemplatesRead,
	ScopeMilestonesRead, ScopeIterationsRead,
	ScopePagesRead,
	ScopeTestsRead,
	ScopeTimeRead,
	ScopeAgentSkillsRead,
	ScopeMCPAccess,
}

// AdminScopes returns the set of scopes that require system admin role.
func AdminScopes() []string {
	out := make([]string, 0, 9)
	for _, s := range scopeCatalog {
		if s.Admin {
			out = append(out, s.Scope)
		}
	}
	return out
}

// NonAdminScopes returns every scope that does not require the system admin
// role. Used by the OAuth surfaces to cap what a dynamically registered client
// may ever be granted.
func NonAdminScopes() []string {
	out := make([]string, 0, len(scopeCatalog))
	for _, s := range scopeCatalog {
		if !s.Admin {
			out = append(out, s.Scope)
		}
	}
	return out
}

// IsAdminScope returns true if the scope is an admin scope.
func IsAdminScope(scope string) bool {
	return strings.HasPrefix(scope, "admin:")
}

// ValidateScopes checks that all provided scopes are valid.
// Returns an error listing any invalid scopes.
// last review: ser, 210426
func ValidateScopes(scopes []string) error {
	valid := make(map[string]bool, len(AllValidScopes))
	for _, s := range AllValidScopes {
		valid[s] = true
	}
	var invalid []string
	for _, s := range scopes {
		if !valid[s] {
			invalid = append(invalid, s)
		}
	}
	if len(invalid) > 0 {
		return fmt.Errorf("invalid scopes: %s", strings.Join(invalid, ", "))
	}
	return nil
}

// ScopesSatisfy reports whether the held scope set grants every scope in
// required, applying the hierarchy that a :write scope implies the matching
// :read. TokenManager.CheckTokenPermissions is the token-shaped wrapper around
// this; surfaces that gate on a scope set without a token in hand (the chat
// tool registry) call it directly so the rule is only implemented once.
func ScopesSatisfy(held, required []string) bool {
	have := make(map[string]bool, len(held))
	for _, s := range held {
		have[s] = true
	}
	for _, want := range required {
		if have[want] {
			continue
		}
		// Hierarchy: write implies read for the same resource, for both
		// plain ("items:read") and admin ("admin:users:read") scopes.
		if strings.HasSuffix(want, ":read") && have[strings.TrimSuffix(want, ":read")+":write"] {
			continue
		}
		return false
	}
	return true
}

// ValidateAgentScopes restricts coding-agent run tokens to the narrowed
// DefaultCodingAgentRunScopes set: no admin:* scopes, no destructive scopes,
// and no broad workspace writes. mcp:access is permitted since WI-351 because
// the MCP server enforces per-tool token scopes, so it cannot widen the set.
// The harness mints tokens that run inside an attacker-reachable container, so
// the surface must stay narrow regardless of what the binding's workspace
// admin requested.
func ValidateAgentScopes(scopes []string) error {
	allowed := make(map[string]bool, len(DefaultCodingAgentRunScopes))
	for _, s := range DefaultCodingAgentRunScopes {
		allowed[s] = true
	}
	var rejected []string
	for _, s := range scopes {
		if !allowed[s] {
			rejected = append(rejected, s)
		}
	}
	if len(rejected) > 0 {
		return fmt.Errorf("%w: %s", ErrAgentScopesNotPermitted, strings.Join(rejected, ", "))
	}
	return nil
}
