package services

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"windshift/internal/models"
	"windshift/internal/repoprep"
)

// RunService is the local OrchestratorClient: its workers run the same
// claim/execute/report loop as remote agents through direct calls. claimNext
// prepares a run; Report finalizes it and cleans up.
var _ OrchestratorClient = (*RunService)(nil)

// queuedJob is an admitted run sent from Start to a local worker.
type queuedJob struct {
	runID int
	req   RunRequest
}

// claimState retains claim-to-report bookkeeping. Repos and checkouts are
// primary-first; scalar refs mirror the primary checkout for legacy paths.
type claimState struct {
	req           RunRequest
	repos         []*repoprep.RepoSpec
	checkouts     []*repoprep.Prepared
	path          string
	branch        string
	baseCommit    string
	workspaceRoot string
	ephemeral     bool
	cancel        context.CancelFunc
}

// runRepos returns Repos or the legacy single Repo, primary first.
func runRepos(req RunRequest) []*repoprep.RepoSpec {
	if len(req.Repos) > 0 {
		return req.Repos
	}
	if req.Repo != nil {
		return []*repoprep.RepoSpec{req.Repo}
	}
	return nil
}

// repoDirNames maps each repo to a unique on-disk subdir name for the multi-repo
// workspace layout (WI-449): the slug's last segment ("owner/core-tests" ->
// "core-tests"), disambiguated with a numeric suffix if two repos share a name.
func repoDirNames(repos []*repoprep.RepoSpec) []string {
	slugs := make([]string, len(repos))
	for i, repo := range repos {
		slugs[i] = repo.RepoSlug
	}
	return repoWorkspaceDirNames(slugs)
}

func repoWorkspaceDirNames(slugs []string) []string {
	names := make([]string, len(slugs))
	seen := make(map[string]int, len(slugs))
	for i, slug := range slugs {
		base := slug
		if idx := strings.LastIndex(base, "/"); idx >= 0 && idx < len(base)-1 {
			base = base[idx+1:]
		}
		if base == "" {
			base = fmt.Sprintf("repo%d", i)
		}
		name := base
		if n, ok := seen[base]; ok {
			name = fmt.Sprintf("%s-%d", base, n+1)
		}
		seen[base]++
		names[i] = name
	}
	return names
}

// queueBuffer sizes the in-process job queue. It is generous relative to
// the concurrency cap so Start does not block under normal load; a queue
// this deep only fills under pathological backpressure.
func queueBuffer(capacity int) int {
	b := capacity * 128
	if b < 1024 {
		b = 1024
	}
	return b
}

// claimNext admits queued work; preamble failures finalize in place, and shutdown drains queued runs.
func (s *RunService) claimNext() *ClaimedJob {
	for {
		job, ok := s.dequeueClaimJob()
		if !ok {
			return nil
		}

		runCtx, cancel, ok := s.beginClaim(job)
		if !ok {
			continue
		}

		st := claimState{req: job.req, ephemeral: job.req.Ephemeral, cancel: cancel}
		if err := s.prepareClaimRepos(runCtx, job, &st); err != nil {
			s.failClaim(job, cancel, "prepare checkout failed", true)
			continue
		}

		env, err := s.buildClaimEnv(runCtx, job, &st)
		if err != nil {
			s.logger.Printf("run service: mint ws token run=%d: %v", job.runID, err)
			s.failClaim(job, cancel, fmt.Sprintf("mint ws token: %v", err), false)
			continue
		}
		if err := pinSandboxWSEnvironment(env); err != nil {
			s.logger.Printf("run service: pin ws destination run=%d: %v", job.runID, err)
			s.failClaim(job, cancel, "pin ws destination failed", true)
			continue
		}
		return s.finishClaim(runCtx, job, &st, env)
	}
}

func (s *RunService) dequeueClaimJob() (queuedJob, bool) {
	select {
	case job := <-s.queue:
		return job, true
	case <-s.shutdownCh:
		for {
			select {
			case job := <-s.queue:
				s.finalize(job.runID, models.AgentRunStatusCanceled, "shutdown before admission")
				s.wg.Done()
			default:
				return queuedJob{}, false
			}
		}
	}
}

func (s *RunService) beginClaim(job queuedJob) (runCtx context.Context, cancel context.CancelFunc, claimed bool) {
	runCtx, cancel = context.WithCancel(context.Background())
	s.registerCancel(job.runID, cancel)
	go func() {
		select {
		case <-s.shutdownCh:
			cancel()
		case <-runCtx.Done():
		}
	}()

	transitioned, err := s.repo.MarkRunningIfQueued(runCtx, job.runID, "", s.now())
	if err != nil {
		s.logger.Printf("run service: mark running run=%d: %v", job.runID, err)
		s.failClaim(job, cancel, fmt.Sprintf("mark running: %v", err), false)
		return nil, nil, false
	}
	if !transitioned {
		s.logger.Printf("run service: skipping run=%d: no longer queued at dequeue", job.runID)
		cancel()
		s.unregisterCancel(job.runID)
		s.wg.Done()
		return nil, nil, false
	}
	if err := s.repo.AppendEvent(runCtx, job.runID, "lifecycle", `{"phase":"running"}`); err != nil {
		s.logger.Printf("run service: append running event run=%d: %v", job.runID, err)
	}
	return runCtx, cancel, true
}

func (s *RunService) prepareClaimRepos(ctx context.Context, job queuedJob, state *claimState) error {
	repos := runRepos(job.req)
	if len(repos) == 0 {
		return nil
	}

	multi := len(repos) > 1
	if multi {
		state.workspaceRoot = s.preparer.RunWorkspaceDir(job.runID)
	}
	dirNames := repoDirNames(repos)
	for i, repo := range repos {
		spec := *repo
		if multi {
			spec.DestDir = filepath.Join(state.workspaceRoot, dirNames[i])
		}
		prepared, err := s.preparer.Prepare(ctx, spec, job.runID)
		if err != nil {
			s.logger.Printf("run service: prepare checkout run=%d repo=%s: %v", job.runID, repo.RepoSlug, err)
			for _, checkout := range state.checkouts {
				_ = s.preparer.Cleanup(context.Background(), checkout)
			}
			if state.workspaceRoot != "" {
				s.preparer.CleanupWorkspaceDir(job.runID)
			}
			return err
		}
		state.repos = append(state.repos, repo)
		state.checkouts = append(state.checkouts, prepared)
		if err := writeSandboxWSConfig(prepared.Path, job.req.Env); err != nil {
			for _, checkout := range state.checkouts {
				_ = s.preparer.Cleanup(context.Background(), checkout)
			}
			if state.workspaceRoot != "" {
				s.preparer.CleanupWorkspaceDir(job.runID)
			}
			return fmt.Errorf("pin checkout ws config: %w", err)
		}
		_ = s.repo.AppendEvent(ctx, job.runID, "lifecycle", fmt.Sprintf(
			`{"phase":"worktree_ready","repo":%q,"path":%q,"branch":%q,"base_commit":%q}`,
			repo.RepoSlug, prepared.Path, prepared.Branch, prepared.BaseCommit))
	}

	primary := state.checkouts[0]
	state.branch = primary.Branch
	state.baseCommit = primary.BaseCommit
	state.path = primary.Path
	if multi {
		state.path = state.workspaceRoot
		if err := writeSandboxWSConfig(state.workspaceRoot, job.req.Env); err != nil {
			for _, checkout := range state.checkouts {
				_ = s.preparer.Cleanup(context.Background(), checkout)
			}
			s.preparer.CleanupWorkspaceDir(job.runID)
			return fmt.Errorf("pin workspace ws config: %w", err)
		}
	}
	return nil
}

func (s *RunService) buildClaimEnv(ctx context.Context, job queuedJob, state *claimState) (map[string]string, error) {
	env := make(map[string]string, len(job.req.Env)+1)
	for key, value := range job.req.Env {
		env[key] = value
	}
	if job.req.Token == nil {
		return env, nil
	}

	refByRepo := make(map[string]string, len(state.checkouts))
	for i, checkout := range state.checkouts {
		refByRepo[state.repos[i].RepoSlug] = checkout.Branch
	}
	token, err := s.mintTokenAndGrants(ctx, job.runID, *job.req.Token, job.req.Grants, refByRepo)
	if err != nil {
		return nil, err
	}
	env["WS_TOKEN"] = token
	applyLLMProxyEnv(env, job.req.Grants, job.runID, token)
	return env, nil
}

func (s *RunService) finishClaim(ctx context.Context, job queuedJob, state *claimState, env map[string]string) *ClaimedJob {
	s.claimsMu.Lock()
	s.claims[job.runID] = state
	s.claimsMu.Unlock()

	initialPrompt := s.initialPrompt
	if job.req.InitialPrompt != "" {
		initialPrompt = job.req.InitialPrompt
	}
	initialPrompt += job.req.InitialPromptSuffix
	return &ClaimedJob{
		Spec: JobSpec{
			RunID: job.runID, WorkspacePath: state.path, Env: env,
			InitialPrompt: initialPrompt, Kind: job.req.JobKind, Image: job.req.JobImage,
		},
		Ctx: ctx,
	}
}

// failClaim records a terminal failed status for a run whose preamble
// failed, releases its accounting, and (for the cases that warrant it)
// fires the post-run hook. It mirrors the early-return finalize paths the
// old inline execute used.
func (s *RunService) failClaim(job queuedJob, cancel context.CancelFunc, msg string, hook bool) {
	s.finalize(job.runID, models.AgentRunStatusFailed, msg)
	if hook {
		s.invokePostRunHook(PostRunInfo{
			RunID:             job.runID,
			WorkspaceID:       job.req.WorkspaceID,
			ItemID:            job.req.ItemID,
			BindingID:         job.req.BindingID,
			Status:            models.AgentRunStatusFailed,
			TriggeredByUserID: job.req.TriggeredByUserID,
		})
	}
	cancel()
	s.unregisterCancel(job.runID)
	s.wg.Done()
}

// Claim implements OrchestratorClient: the in-process transport for the
// shared RunWorker loop. It blocks on the in-memory queue (honoring
// shutdown) and returns (nil, nil) when the service is shutting down. The
// per-run abort context rides on ClaimedJob.Ctx.
func (s *RunService) Claim(_ context.Context) (*ClaimedJob, error) {
	return s.claimNext(), nil
}

// Emit implements OrchestratorClient: it appends one event to the run's
// agent_run_events stream.
func (s *RunService) Emit(ctx context.Context, runID int, eventType, payloadJSON string) error {
	return s.repo.AppendEvent(ctx, runID, eventType, payloadJSON)
}

// Report implements OrchestratorClient: it records the runner's terminal
// verdict, emits the terminal lifecycle event, cleans up the worktree,
// fires the post-run hook, and releases the run's accounting.
func (s *RunService) Report(ctx context.Context, runID int, result RunnerResult) error {
	s.claimsMu.Lock()
	st := s.claims[runID]
	delete(s.claims, runID)
	s.claimsMu.Unlock()

	status := s.normalizeRunnerResult(ctx, runID, &result)
	pushedRepos, status := s.pushClaimRepos(ctx, runID, st, status, &result)
	// Legacy scalar branch fields mirror the primary repo's no-change outcome.
	noChanges := len(pushedRepos) > 0 && pushedRepos[0].Branch == ""

	s.finalize(runID, status, result.Error)
	if err := s.repo.AppendEvent(ctx, runID, "lifecycle", fmt.Sprintf(`{"phase":%q}`, status)); err != nil {
		s.logger.Printf("run service: append terminal event run=%d: %v", runID, err)
	}

	var (
		req        RunRequest
		branch     string
		baseCommit string
		cancel     context.CancelFunc
	)
	if st != nil {
		req = st.req
		if !noChanges {
			branch = st.branch
			baseCommit = st.baseCommit
		}
		cancel = st.cancel
		for _, pw := range st.checkouts {
			if err := s.preparer.Cleanup(context.Background(), pw); err != nil {
				s.logger.Printf("run service: cleanup checkout run=%d: %v", runID, err)
			}
		}
		if st.workspaceRoot != "" {
			s.preparer.CleanupWorkspaceDir(runID)
		}
	}

	// Ephemeral (binding "test") runs never feed the PR hook: there is no item
	// to link and no branch should reach the remote (the push above is skipped
	// too), so opening a PR would be wrong.
	if st == nil || !st.ephemeral {
		s.invokePostRunHook(PostRunInfo{
			RunID:             runID,
			WorkspaceID:       req.WorkspaceID,
			ItemID:            req.ItemID,
			BindingID:         req.BindingID,
			Status:            status,
			Branch:            branch,
			BaseCommit:        baseCommit,
			TriggeredByUserID: req.TriggeredByUserID,
			Summary:           result.Summary,
			Trigger:           req.Trigger,
			Repos:             pushedRepos,
		})
	}

	if cancel != nil {
		cancel()
	}
	s.unregisterCancel(runID)
	s.wg.Done()
	return nil
}

// pushClaimRepos delivers local checkout branches before the run is finalized.
func (s *RunService) pushClaimRepos(ctx context.Context, runID int, st *claimState, status string, result *RunnerResult) (repos []PostRunRepo, finalStatus string) {
	if status != models.AgentRunStatusSucceeded || st == nil || st.ephemeral || len(st.checkouts) == 0 || s.preparer == nil {
		return nil, status
	}
	repos = make([]PostRunRepo, 0, len(st.checkouts))
	for i, prepared := range st.checkouts {
		repo := st.repos[i]
		pushed := PostRunRepo{RepoSlug: repo.RepoSlug, Branch: prepared.Branch, BaseCommit: prepared.BaseCommit}
		switch err := s.preparer.Push(context.Background(), prepared, repo.Token); {
		case errors.Is(err, repoprep.ErrNoNewCommits):
			pushed.Branch = ""
			if err := s.repo.AppendEvent(ctx, runID, "lifecycle", fmt.Sprintf(`{"phase":"no_changes","repo":%q}`, repo.RepoSlug)); err != nil {
				s.logger.Printf("run service: append no_changes event run=%d: %v", runID, err)
			}
		case err != nil:
			s.logger.Printf("run service: push run branch run=%d repo=%s: %v", runID, repo.RepoSlug, err)
			status = models.AgentRunStatusFailed
			result.Error = fmt.Sprintf("push run branch %s: %v", repo.RepoSlug, err)
		}
		repos = append(repos, pushed)
	}
	return repos, status
}

// Heartbeat implements OrchestratorClient. The in-process worker holds the
// run for its whole lifetime, so there is nothing to renew; remote runners
// override this to keep their lease alive.
func (s *RunService) Heartbeat(_ context.Context, _ int) error { return nil }
