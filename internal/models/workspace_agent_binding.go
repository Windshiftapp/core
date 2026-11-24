package models

import (
	"strings"
	"time"
)

type AgentProfileType string

const (
	AgentProfileStandard AgentProfileType = "standard"
	AgentProfileCoding   AgentProfileType = "coding"
	AgentProfileLegacy   AgentProfileType = "legacy"
)

type AgentLifecycle string

const (
	AgentLifecycleDraft    AgentLifecycle = "draft"
	AgentLifecycleReady    AgentLifecycle = "ready"
	AgentLifecyclePaused   AgentLifecycle = "paused"
	AgentLifecycleArchived AgentLifecycle = "archived"
)

type AgentIdentityClass string

const (
	AgentIdentityUserOwned        AgentIdentityClass = "user_owned"
	AgentIdentityCentralized      AgentIdentityClass = "centralized_service"
	AgentIdentityWorkspaceManaged AgentIdentityClass = "workspace_managed"
)

// WorkspaceAgentBinding links a workspace + acting user to the run-shape
// RunService needs when an item is assigned to that user. The acting-
// identity kind is stamped at create time by the WI-87 chokepoint and
// stored verbatim so the trigger path can render audit info without
// re-running the gate.
//
// One binding per (workspace_id, acting_user_id). The Repo* fields are
// nullable so a workspace can ship a "fall through to whatever the
// orchestrator picks" binding while still gating identity.
//
// A binding that wants per-run worktree preparation must reference an
// SCMConnectionID. The clone URL is derived server-side from the
// trusted SCM provider record + RepoSlug — the binding does not store
// a free-form remote URL, so a workspace admin cannot point runs at
// arbitrary hosts (SSRF) or git remote helpers (RCE via ext::).
type WorkspaceAgentBinding struct {
	ID             int                `json:"id"`
	WorkspaceID    int                `json:"workspace_id"`
	ActingUserID   int                `json:"acting_user_id"`
	ActingUserKind string             `json:"acting_user_kind"`
	ProfileType    AgentProfileType   `json:"profile_type"`
	Lifecycle      AgentLifecycle     `json:"lifecycle"`
	ProfileVersion int                `json:"profile_version"`
	IdentityClass  AgentIdentityClass `json:"identity_class"`
	Purpose        string             `json:"purpose,omitempty"`
	// CapabilityGroups stores optional Standard-agent groups. The mandatory
	// Read and comment preset is registry policy and is never persisted as an
	// editable selection.
	CapabilityGroups []string   `json:"capability_groups,omitempty"`
	ArchivedAt       *time.Time `json:"archived_at,omitempty"`
	ArchivedByUserID *int       `json:"archived_by_user_id,omitempty"`
	LastKnownName    string     `json:"last_known_name,omitempty"`
	LastKnownHandle  string     `json:"last_known_handle,omitempty"`
	LastKnownAvatar  string     `json:"last_known_avatar,omitempty"`
	DisplayName      string     `json:"name,omitempty"`
	Handle           string     `json:"handle,omitempty"`
	AvatarURL        string     `json:"avatar_url,omitempty"`
	// RepoSlug/RepoBaseRef/SCMConnectionID are the legacy single-repo scalar
	// fields. Superseded by Repos (WI-449); kept one release as deprecated
	// mirrors of the primary repo so straggler readers still work. New code
	// must read Repos / PrimaryRepo(), not these.
	RepoSlug        string   `json:"repo_slug,omitempty"`
	RepoBaseRef     string   `json:"repo_base_ref,omitempty"`
	LLMConnectionID *int     `json:"llm_connection_id,omitempty"`
	SCMConnectionID *int     `json:"scm_connection_id,omitempty"`
	TokenScopes     []string `json:"token_scopes,omitempty"`
	TokenTTLMinutes int      `json:"token_ttl_minutes"`
	MaxRunsPerDay   int      `json:"max_runs_per_day"`
	// TargetPoolID routes this binding's coding-agent runs to a runner_pool
	// capability (a remote pool) instead of the local in-process pool. NULL =
	// local. The pool's per-run token + grants are derived at claim (WI-195).
	TargetPoolID *int `json:"target_pool_id,omitempty"`
	// RunnerImage overrides the coding-agent container image for this binding's
	// remote (pool) runs — e.g. a Node+Chrome image for Playwright. Empty = the
	// runner's default windshift-agent image. Only honored for pool bindings
	// (TargetPoolID set); the local in-process runner uses its fixed image
	// (WI-450).
	RunnerImage string `json:"runner_image,omitempty"`
	// Instructions is the binding's persona/specialization, appended to the
	// run's standard initial prompt as a "Your role" section (WI-258). It
	// never replaces the operational prompt.
	Instructions string `json:"instructions,omitempty"`
	// CreatedByUserID is a soft audit ref; nil once the creator is deleted.
	CreatedByUserID *int      `json:"created_by_user_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`

	// Repos is the set of repositories this binding checks out (WI-449).
	// Exactly one is IsPrimary. Hydrated from workspace_agent_binding_repos.
	Repos []BindingRepo `json:"repos,omitempty"`
}

// BindingRepo is one repository bound to an agent binding (WI-449). Each repo
// carries its own SCM connection so the clone URL stays server-derived from a
// trusted host per repo (the anti-SSRF invariant the binding upholds). Exactly
// one repo per binding is IsPrimary: its PR links to the work item and it is
// the repo used for single-repo backward-compatible behavior.
type BindingRepo struct {
	ID              int    `json:"id"`
	BindingID       int    `json:"binding_id"`
	SCMConnectionID *int   `json:"scm_connection_id,omitempty"`
	RepoSlug        string `json:"repo_slug"`
	RepoBaseRef     string `json:"repo_base_ref,omitempty"`
	IsPrimary       bool   `json:"is_primary"`
	Position        int    `json:"position"`
}

// HasRepo reports whether the binding is configured with enough source-
// control info to ask the repoprep.Preparer for a prepared checkout: at least
// one bound repo. Each repo carries its own SCM connection (enforced at
// create time), which supplies the trusted provider host the clone URL is
// derived from.
//
// As a transitional fallback it also accepts the deprecated scalar
// RepoSlug/SCMConnectionID fields, so a binding loaded by code that predates
// repo hydration (or constructed in-memory from the legacy fields) still
// reports a repo. Hydrated bindings mirror the primary onto those fields, so
// the two checks agree.
func (b *WorkspaceAgentBinding) HasRepo() bool {
	if b == nil {
		return false
	}
	return len(b.Repos) > 0 || (b.RepoSlug != "" && b.SCMConnectionID != nil)
}

// IsMultiRepo reports whether the binding checks out more than one repo.
func (b *WorkspaceAgentBinding) IsMultiRepo() bool {
	return b != nil && len(b.Repos) > 1
}

// PrimaryRepo returns the binding's primary repo: the IsPrimary entry, else
// the first by Position, else nil when no repos are bound.
func (b *WorkspaceAgentBinding) PrimaryRepo() *BindingRepo {
	if b == nil || len(b.Repos) == 0 {
		return nil
	}
	for i := range b.Repos {
		if b.Repos[i].IsPrimary {
			return &b.Repos[i]
		}
	}
	return &b.Repos[0]
}

// HasRepoSlug reports whether any bound repo matches slug. Used by the run
// continuation guards: a continuation is allowed if the open PR's repo is one
// the binding can push to.
func (b *WorkspaceAgentBinding) HasRepoSlug(slug string) bool {
	if b == nil || slug == "" {
		return false
	}
	for i := range b.Repos {
		if b.Repos[i].RepoSlug == slug {
			return true
		}
	}
	return false
}

// DirName returns the on-disk directory name for this repo in a multi-repo
// agent workspace: the segment of the slug after the last "/" (e.g.
// "owner/core-tests" -> "core-tests"). Falls back to the whole slug when it
// has no "/".
func (r BindingRepo) DirName() string {
	if i := strings.LastIndex(r.RepoSlug, "/"); i >= 0 && i < len(r.RepoSlug)-1 {
		return r.RepoSlug[i+1:]
	}
	return r.RepoSlug
}
