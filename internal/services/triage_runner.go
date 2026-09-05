package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"windshift/internal/models"
)

// TriageRunner is the remote runner's repo-prep wrapper (WI-215). It owns git
// on the runner host so the untrusted agent container never does: for a run
// carrying a JobRepo it execs the windshift-triage binary to prepare a per-run
// checkout, runs the inner Runner against it, then pushes the run branch back
// through the git-proxy — all without the agent ever holding an SCM credential.
//
// Reads (clone/fetch) and the final push both go through the git-proxy at
// {APIBase}/git-proxy/{ws}/{owner}/{repo}, authenticated by the per-run token
// (WS_TOKEN). The proxy injects the real SCM credential server-side and gates
// the push to the single granted ref (WI-168).
//
// For a run WITHOUT a JobRepo (local runs, where the orchestrator already
// prepared WorkspacePath, and non-repo container jobs) TriageRunner is a pass-
// through to the inner Runner.
type TriageRunner struct {
	Inner     Runner // the real execution (DockerAgentRunner)
	TriageBin string // path to the windshift-triage binary
	CacheRoot string // --root for the bare-clone cache
	APIBase   string // orchestrator base incl. API prefix (for the git-proxy URL)
	Logger    *log.Logger
}

type triagePrepareOut struct {
	CheckoutPath string `json:"checkout_path"`
	Branch       string `json:"branch"`
	BaseCommit   string `json:"base_commit"`
}

// Run implements Runner.
func (t *TriageRunner) Run(ctx context.Context, input RunInput, emit EventSink) RunnerResult {
	// Multi-repo run (WI-449): each bound repo is prepared as a sibling dir
	// under a shared workspace root and pushed independently. Handled in a
	// dedicated path so the single-repo flow below stays byte-identical.
	if len(input.Repos) > 1 {
		return t.runMulti(ctx, input, emit)
	}
	// Normalize a one-element Repos to the single Repo field the path below uses.
	if input.Repo == nil && len(input.Repos) == 1 {
		input.Repo = &input.Repos[0]
	}
	if input.Repo == nil {
		return t.Inner.Run(ctx, input, emit)
	}
	token, err := t.repoRunToken(input)
	if err != nil {
		return RunnerResult{Status: models.AgentRunStatusFailed, Error: err.Error()}
	}
	owner, repo, ok := splitSlug(input.Repo.Slug)
	if !ok {
		return RunnerResult{Status: models.AgentRunStatusFailed, Error: fmt.Sprintf("triage runner: bad repo slug %q", input.Repo.Slug)}
	}

	// The git-proxy URL is the tokenless remote both clone and push target;
	// the token authenticates via askpass/Basic and the proxy injects the SCM
	// credential. {ws} is cosmetic to the proxy (it authorizes by token) but
	// the route requires it.
	proxyURL := fmt.Sprintf("%s/git-proxy/%d/%s/%s",
		strings.TrimRight(t.APIBase, "/"), input.Repo.WorkspaceID, owner, repo)

	tokenFile, cleanupToken, err := writeTokenFile(token, input.RunID)
	if err != nil {
		return RunnerResult{Status: models.AgentRunStatusFailed, Error: fmt.Sprintf("triage runner: token file: %v", err)}
	}
	defer cleanupToken()

	prep, err := t.prepare(ctx, *input.Repo, input.RunID, proxyURL, tokenFile, "")
	if err != nil {
		return RunnerResult{Status: models.AgentRunStatusFailed, Error: fmt.Sprintf("triage prepare: %v", err)}
	}
	// The checkout is throwaway; reclaim it whatever happens.
	defer func() {
		if rmErr := os.RemoveAll(prep.CheckoutPath); rmErr != nil {
			t.logf("triage runner: cleanup %s: %v", prep.CheckoutPath, rmErr)
		}
	}()
	if err := writeSandboxWSConfig(prep.CheckoutPath, input.Env); err != nil {
		return RunnerResult{Status: models.AgentRunStatusFailed, Error: fmt.Sprintf("triage pin ws config: %v", err)}
	}

	input.WorkspacePath = prep.CheckoutPath
	_ = emit("lifecycle", fmt.Sprintf(
		`{"phase":"worktree_ready","path":%q,"branch":%q,"base_commit":%q}`,
		prep.CheckoutPath, prep.Branch, prep.BaseCommit))

	result := t.Inner.Run(ctx, input, emit)

	// Push the agent's commits only on success. A run that didn't succeed has
	// nothing the post-run PR hook should act on, and pushing a half-built
	// branch through the ref-gated proxy adds no value. A commit-less success
	// (head still at base) skips the push: nothing to deliver, no branch on
	// the remote, no PR. Only an actual push stamps Branch/BaseCommit on the
	// result — that is what the orchestrator's PR hook keys on.
	if result.Status == models.AgentRunStatusSucceeded {
		head, skipped, perr := t.push(ctx, prep, proxyURL, tokenFile)
		switch {
		case perr != nil:
			_ = emit("lifecycle", fmt.Sprintf(`{"phase":"push_failed","error":%q}`, RedactString(perr.Error())))
			return RunnerResult{Status: models.AgentRunStatusFailed, Error: fmt.Sprintf("triage push: %v", perr)}
		case skipped:
			_ = emit("lifecycle", `{"phase":"no_changes"}`)
		default:
			_ = emit("lifecycle", fmt.Sprintf(`{"phase":"pushed","branch":%q,"head":%q}`, prep.Branch, head))
			result.Branch = prep.Branch
			result.BaseCommit = prep.BaseCommit
		}
	}
	return result
}

// runMulti prepares every bound repo as a sibling dir under a shared per-run
// workspace root, runs the agent against that root, then pushes each repo's run
// branch through the git-proxy independently (WI-449). Mirrors the local
// run_service multi-repo path; the broker authorizes each push against that
// repo's grant.
func (t *TriageRunner) runMulti(ctx context.Context, input RunInput, emit EventSink) RunnerResult {
	token, err := t.repoRunToken(input)
	if err != nil {
		return RunnerResult{Status: models.AgentRunStatusFailed, Error: err.Error()}
	}
	tokenFile, cleanupToken, err := writeTokenFile(token, input.RunID)
	if err != nil {
		return RunnerResult{Status: models.AgentRunStatusFailed, Error: fmt.Sprintf("triage runner: token file: %v", err)}
	}
	defer cleanupToken()

	workspaceRoot := filepath.Join(t.CacheRoot, ".workspaces", fmt.Sprintf("run-%d", input.RunID))
	defer func() {
		if rmErr := os.RemoveAll(workspaceRoot); rmErr != nil {
			t.logf("triage runner: cleanup workspace %s: %v", workspaceRoot, rmErr)
		}
	}()

	type preparedRepo struct {
		jr       JobRepo
		out      triagePrepareOut
		proxyURL string
	}
	dirNames := triageDirNames(input.Repos)
	var preps []preparedRepo
	for i, jr := range input.Repos {
		owner, repo, ok := splitSlug(jr.Slug)
		if !ok {
			return RunnerResult{Status: models.AgentRunStatusFailed, Error: fmt.Sprintf("triage runner: bad repo slug %q", jr.Slug)}
		}
		proxyURL := fmt.Sprintf("%s/git-proxy/%d/%s/%s",
			strings.TrimRight(t.APIBase, "/"), jr.WorkspaceID, owner, repo)
		dest := filepath.Join(workspaceRoot, dirNames[i])
		prep, perr := t.prepare(ctx, jr, input.RunID, proxyURL, tokenFile, dest)
		if perr != nil {
			return RunnerResult{Status: models.AgentRunStatusFailed, Error: fmt.Sprintf("triage prepare %s: %v", jr.Slug, perr)}
		}
		if perr := writeSandboxWSConfig(prep.CheckoutPath, input.Env); perr != nil {
			return RunnerResult{Status: models.AgentRunStatusFailed, Error: fmt.Sprintf("triage pin ws config %s: %v", jr.Slug, perr)}
		}
		_ = emit("lifecycle", fmt.Sprintf(
			`{"phase":"worktree_ready","repo":%q,"path":%q,"branch":%q,"base_commit":%q}`,
			jr.Slug, prep.CheckoutPath, prep.Branch, prep.BaseCommit))
		preps = append(preps, preparedRepo{jr: jr, out: prep, proxyURL: proxyURL})
	}
	if err := writeSandboxWSConfig(workspaceRoot, input.Env); err != nil {
		return RunnerResult{Status: models.AgentRunStatusFailed, Error: fmt.Sprintf("triage pin workspace ws config: %v", err)}
	}

	input.WorkspacePath = workspaceRoot
	result := t.Inner.Run(ctx, input, emit)
	if result.Status != models.AgentRunStatusSucceeded {
		return result
	}

	// Push each repo independently. A push error fails the whole run (a
	// half-delivered multi-repo change is worse than a visible failure); a
	// commit-less repo is skipped (no_changes). Only pushed repos carry a
	// branch in the reported result, which is what the PR hook keys on.
	for _, p := range preps {
		head, skipped, perr := t.push(ctx, p.out, p.proxyURL, tokenFile)
		switch {
		case perr != nil:
			_ = emit("lifecycle", fmt.Sprintf(`{"phase":"push_failed","repo":%q,"error":%q}`, p.jr.Slug, RedactString(perr.Error())))
			return RunnerResult{Status: models.AgentRunStatusFailed, Error: fmt.Sprintf("triage push %s: %v", p.jr.Slug, perr)}
		case skipped:
			_ = emit("lifecycle", fmt.Sprintf(`{"phase":"no_changes","repo":%q}`, p.jr.Slug))
			result.Repos = append(result.Repos, RunnerRepoResult{RepoSlug: p.jr.Slug})
		default:
			_ = emit("lifecycle", fmt.Sprintf(`{"phase":"pushed","repo":%q,"branch":%q,"head":%q}`, p.jr.Slug, p.out.Branch, head))
			result.Repos = append(result.Repos, RunnerRepoResult{RepoSlug: p.jr.Slug, Branch: p.out.Branch, BaseCommit: p.out.BaseCommit})
		}
	}
	// Mirror the primary (index 0) onto the scalar fields for back-compat.
	if len(result.Repos) > 0 {
		result.Branch = result.Repos[0].Branch
		result.BaseCommit = result.Repos[0].BaseCommit
	}
	return result
}

// triageDirNames mirrors repoDirNames for the remote runner: a unique sibling
// dir name per repo (last slug segment, numeric-suffixed on collision).
func triageDirNames(repos []JobRepo) []string {
	slugs := make([]string, len(repos))
	for i, repo := range repos {
		slugs[i] = repo.Slug
	}
	return repoWorkspaceDirNames(slugs)
}

func (t *TriageRunner) repoRunToken(input RunInput) (string, error) {
	if t.TriageBin == "" || t.CacheRoot == "" || t.APIBase == "" {
		return "", errors.New("triage runner: TriageBin, CacheRoot and APIBase are required for repo runs")
	}
	token := input.Env["WS_TOKEN"]
	if token == "" {
		return "", errors.New("triage runner: no WS_TOKEN for git-proxy auth")
	}
	return token, nil
}

func (t *TriageRunner) prepare(ctx context.Context, jr JobRepo, runID int, proxyURL, tokenFile, destDir string) (triagePrepareOut, error) {
	args := []string{
		"--root", t.CacheRoot,
		"--workspace-id", fmt.Sprintf("%d", jr.WorkspaceID),
		"--repo", jr.Slug,
		"--remote-url", proxyURL,
		"--base-ref", jr.BaseRef,
		"--run-id", fmt.Sprintf("%d", runID),
		"--token-file", tokenFile,
	}
	// Continuation: check out + push back to this existing PR head branch
	// instead of cutting a fresh per-run branch from --base-ref.
	if jr.ContinueBranch != "" {
		args = append(args, "--continue-branch", jr.ContinueBranch)
	}
	// Multi-repo: place this checkout at the chosen sibling dir (WI-449).
	if destDir != "" {
		args = append(args, "--dest-dir", destDir)
	}
	out, err := t.execTriage(ctx, "prepare", args...)
	if err != nil {
		return triagePrepareOut{}, err
	}
	var p triagePrepareOut
	if jerr := json.Unmarshal(out, &p); jerr != nil {
		return triagePrepareOut{}, fmt.Errorf("decode prepare output: %w", jerr)
	}
	if p.CheckoutPath == "" || p.Branch == "" {
		return triagePrepareOut{}, fmt.Errorf("prepare returned incomplete output: %s", string(out))
	}
	return p, nil
}

func (t *TriageRunner) push(ctx context.Context, prep triagePrepareOut, proxyURL, tokenFile string) (head string, skipped bool, err error) {
	out, err := t.execTriage(ctx, "push",
		"--dest", prep.CheckoutPath,
		"--branch", prep.Branch,
		"--git-transport", "proxy",
		"--proxy-url", proxyURL,
		"--token-file", tokenFile,
		"--skip-if-head", prep.BaseCommit,
	)
	if err != nil {
		return "", false, err
	}
	var p struct {
		HeadSHA string `json:"head_sha"`
		Skipped bool   `json:"skipped"`
	}
	_ = json.Unmarshal(out, &p)
	return p.HeadSHA, p.Skipped, nil
}

// execTriage runs the triage binary and returns its stdout. stderr is folded
// into the error (scrubbed) so a credential can't leak into logs.
func (t *TriageRunner) execTriage(ctx context.Context, sub string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, t.TriageBin, append([]string{sub}, args...)...) //nolint:gosec // G204: args are orchestrator-derived
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w: %s", err, RedactString(strings.TrimSpace(stderr.String())))
	}
	return []byte(stdout.String()), nil
}

func (t *TriageRunner) logf(format string, args ...any) {
	if t.Logger != nil {
		t.Logger.Printf(format, args...)
	}
}

// splitSlug splits "owner/repo" into its parts. Both must be non-empty and
// neither may contain a slash (a multi-segment path is rejected).
func splitSlug(slug string) (owner, repo string, ok bool) {
	owner, repo, found := strings.Cut(slug, "/")
	if !found || owner == "" || repo == "" || strings.Contains(repo, "/") {
		return "", "", false
	}
	return owner, repo, true
}

// writeTokenFile writes the per-run token to a 0600 temp file for triage's
// --token-file, returning the path plus a cleanup func.
func writeTokenFile(token string, runID int) (path string, cleanup func(), err error) {
	f, err := os.CreateTemp("", fmt.Sprintf("windshift-runtoken-%d-*", runID))
	if err != nil {
		return "", func() {}, err
	}
	path = f.Name()
	cleanup = func() { _ = os.Remove(path) }
	if err = f.Chmod(0o600); err != nil {
		_ = f.Close()
		cleanup()
		return "", func() {}, err
	}
	if _, err = f.WriteString(token); err != nil {
		_ = f.Close()
		cleanup()
		return "", func() {}, err
	}
	if err = f.Close(); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return path, cleanup, nil
}
