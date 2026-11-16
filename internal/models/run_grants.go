package models

import (
	"net/url"
	"strings"
)

// RunGrants is the set of brokered resources a single agent run may reach
// through the secretless access layer (Initiative WI-141 / WI-144). It is
// snapshotted onto the run at claim time (derived from the binding) and
// stored as agent_runs.grants_json; the broker endpoints (git / llm /
// secrets / http) authorize each request against it. The run's minted token
// (agent_runs.run_token_id) is what binds a presented credential to these
// grants — so a leaked token for run A cannot reach run B's resources.
type RunGrants struct {
	// Git is the deprecated single-repo git grant (WI-449). Superseded by
	// GitRepos; kept one release as the primary repo's grant so older broker
	// code and runs persisted before the change keep working. New grants
	// populate both Git (= primary) and GitRepos (all repos).
	Git *GitGrant `json:"git,omitempty"`
	// GitRepos is one grant per repository a multi-repo run may reach (WI-449).
	// The broker authorizes each git request against the grant whose Repo
	// matches the requested owner/repo — deny-by-default if none matches.
	GitRepos []GitGrant   `json:"git_repos,omitempty"`
	LLM      *LLMGrant    `json:"llm,omitempty"`
	Secrets  []int        `json:"secrets,omitempty"` // ActionCredential ids the run may fetch
	HTTP     []string     `json:"http,omitempty"`    // allowed outbound URL prefixes
	Skills   []SkillGrant `json:"skills,omitempty"`  // immutable skill bodies available to this run
}

type SkillGrant struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Body        string `json:"body"`
	// Error is retained for unavailable grants persisted by older versions.
	// New snapshots fail admission instead of creating an unavailable grant.
	Error string `json:"error,omitempty"`
}

func (g *RunGrants) SkillFor(id int) *SkillGrant {
	if g == nil {
		return nil
	}
	for i := range g.Skills {
		if g.Skills[i].ID == id {
			return &g.Skills[i]
		}
	}
	return nil
}

// GitGrant scopes a run's git access to a single repo and the single ref it
// may push (the agent's run branch). Empty Ref means no push is authorized.
// ConnectionID is the SCM connection whose credential the git broker injects
// server-side when proxying to the provider. UserID is the credential
// principal: on OAuth connections the broker injects this user's personal
// token (the run's triggering user, WI-275); 0 means the connection-level
// credential (PAT / GitHub App connections, and legacy runs).
type GitGrant struct {
	Repo         string `json:"repo"`          // "owner/repo"
	Ref          string `json:"ref,omitempty"` // the branch the run may push
	ConnectionID int    `json:"connection_id"` // SCM connection for credential injection
	UserID       int    `json:"user_id,omitempty"`
}

// LLMGrant scopes a run's model access to one connection with an optional
// per-run output-token quota (0 = unlimited).
type LLMGrant struct {
	ConnectionID int `json:"connection_id"`
	QuotaTokens  int `json:"quota_tokens,omitempty"`
}

// gitGrants returns the effective per-repo grant list: GitRepos when present,
// else the deprecated single Git as a one-element list (WI-449). Returns a
// fresh slice; callers must treat entries as read-only.
func (g *RunGrants) gitGrants() []GitGrant {
	if g == nil {
		return nil
	}
	if len(g.GitRepos) > 0 {
		return g.GitRepos
	}
	if g.Git != nil {
		return []GitGrant{*g.Git}
	}
	return nil
}

// GitGrantFor returns the grant authorizing access to repo ("owner/repo"), or
// nil if the run holds no grant for it (deny-by-default). This is the single
// chokepoint the git broker uses to both authorize a repo and resolve the
// credential principal/connection for it (WI-449).
func (g *RunGrants) GitGrantFor(repo string) *GitGrant {
	grants := g.gitGrants()
	for i := range grants {
		if grants[i].Repo == repo {
			return &grants[i]
		}
	}
	return nil
}

// AllowsGitRepo reports whether the run may access the given owner/repo.
func (g *RunGrants) AllowsGitRepo(repo string) bool {
	return g.GitGrantFor(repo) != nil
}

// AllowsGitPush reports whether the run may push the given ref to repo. The
// push is gated to the single branch named in that repo's grant. git-receive-
// pack always sends the fully-qualified ref ("refs/heads/agent-runs/run-7"),
// while the grant may store that branch either short ("agent-runs/run-7", as
// the run service mints it) or already qualified — so both sides are normalized
// to refs/heads/<branch> before the exact match. A tag or any other ref class
// never collapses onto a branch grant, so this stays a single-branch gate.
func (g *RunGrants) AllowsGitPush(repo, ref string) bool {
	grant := g.GitGrantFor(repo)
	return grant != nil && grant.Ref != "" &&
		qualifyBranchRef(grant.Ref) == qualifyBranchRef(ref)
}

// qualifyBranchRef returns ref in fully-qualified form, treating a bare name as
// a branch (refs/heads/<name>). An already-qualified ref (refs/heads/, refs/tags/,
// any refs/*) is returned unchanged, so a branch grant never matches a tag.
func qualifyBranchRef(ref string) string {
	if ref == "" || strings.HasPrefix(ref, "refs/") {
		return ref
	}
	return "refs/heads/" + ref
}

// AllowsSecret reports whether the run may fetch the credential with the
// given id.
func (g *RunGrants) AllowsSecret(id int) bool {
	if g == nil {
		return false
	}
	for _, s := range g.Secrets {
		if s == id {
			return true
		}
	}
	return false
}

// AllowsHTTP reports whether rawURL matches one of the run's allowed grants.
// Deny-by-default: a nil grant or empty pattern never matches.
//
// Matching is on URL-component boundaries, not raw string prefix (WI-168):
// scheme and host:port must be equal and the target path must be the grant
// path or a "/"-delimited sub-path of it. This prevents a grant such as
// "https://api.example.com" from also permitting "https://api.example.com.evil/"
// or "https://api.example.com@169.254.169.254/". A target carrying userinfo
// (user:pass@host) is always rejected.
func (g *RunGrants) AllowsHTTP(rawURL string) bool {
	if g == nil {
		return false
	}
	target, err := url.Parse(rawURL)
	if err != nil || target.Host == "" || target.User != nil {
		return false
	}
	for _, p := range g.HTTP {
		if p == "" {
			continue
		}
		pat, err := url.Parse(p)
		if err != nil || pat.Host == "" {
			continue
		}
		if !strings.EqualFold(target.Scheme, pat.Scheme) {
			continue
		}
		if !strings.EqualFold(target.Host, pat.Host) { // Host includes :port
			continue
		}
		if pathWithinGrant(target.EscapedPath(), pat.EscapedPath()) {
			return true
		}
	}
	return false
}

// pathWithinGrant reports whether targetPath is the grant path or a
// slash-delimited descendant of it. An empty or "/" grant path matches any
// target path (host-level grant).
func pathWithinGrant(targetPath, grantPath string) bool {
	grantPath = strings.TrimSuffix(grantPath, "/")
	if grantPath == "" {
		return true
	}
	if targetPath == grantPath {
		return true
	}
	return strings.HasPrefix(targetPath, grantPath+"/")
}
