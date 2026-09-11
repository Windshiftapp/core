// Package services hosts coding-agent harness orchestration. RunService owns
// per-process run admission, dispatch, events, and finalization; Runners spawn
// containers independently for testability.
package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"windshift/internal/auth"
	"windshift/internal/models"
	"windshift/internal/repoprep"
	"windshift/internal/repository"
)

// EventSink is what a Runner uses to stream events into agent_run_events
// while a run is in flight. Returning an error from the sink does not abort
// the run by itself — the runner decides what to do.
type EventSink func(eventType, payloadJSON string) error

// RunnerResult is the terminal verdict a Runner returns when it exits.
// Status must be one of the terminal agent-run states (see
// models.IsAgentRunTerminal). ContainerID is recorded for audit / forensics
// and may be empty in skeleton runners.
type RunnerResult struct {
	ContainerID string
	Status      string
	Error       string
	// Branch and BaseCommit are prepared worktree refs used for PR creation.
	Branch     string
	BaseCommit string
	// Summary is the bounded, sanitized agent finish summary used as the PR note.
	Summary string
	// Repos carries per-repo push results, primary first; it supersedes scalar refs when present.
	Repos []RunnerRepoResult
}

// RunnerRepoResult is one repo's push result reported by a (remote) runner that
// prepared its own checkouts (WI-449).
type RunnerRepoResult struct {
	RepoSlug   string
	Branch     string
	BaseCommit string
}

// RunInput is what RunService hands to a Runner when work is admitted:
// the run id, the host path containing the prepared worktree (empty if no
// repo was attached to the request), and any orchestrator-supplied env
// vars to forward into the container.
type RunInput struct {
	RunID         int
	WorkspacePath string
	Env           map[string]string
	InitialPrompt string
	Kind          string
	Image         string
	Repo          *JobRepo
	// Repos lists every checkout, primary first.
	Repos []JobRepo
}

// Runner executes the actual work of a run: spawning a container, driving
// the JSONL agent contract, and streaming events back through the sink. The skeleton
// uses a func adapter; the container-backed implementation lives in later
// phases.
type Runner interface {
	Run(ctx context.Context, input RunInput, emit EventSink) RunnerResult
}

// RunnerFunc adapts a plain function to the Runner interface.
type RunnerFunc func(ctx context.Context, input RunInput, emit EventSink) RunnerResult

// Run implements Runner for RunnerFunc.
func (f RunnerFunc) Run(ctx context.Context, input RunInput, emit EventSink) RunnerResult {
	return f(ctx, input, emit)
}

// BindingID is the optional id stamped on PostRunInfo so the hook can
// look the binding up without re-running the assignee match. The binding
// trigger sets it; manual run starts leave it 0.

// RunRequest starts a run with optional checkouts, a TTL-bound ws token, and
// runner environment. Orchestrator-owned environment keys override caller values
// to preserve the run identity.
type RunRequest struct {
	WorkspaceID int
	ItemID      *int
	BindingID   int
	// Repo is the deprecated single-repo input; an empty Repos list preserves it.
	Repo *repoprep.RepoSpec
	// Repos is the full checkout set, primary first.
	Repos []*repoprep.RepoSpec
	Token *TokenSpec
	Env   map[string]string
	// Grants are snapshotted at claim and bound to the run token.
	Grants              *models.RunGrants
	JobKind             string
	JobImage            string
	InitialPrompt       string
	Ephemeral           bool
	TargetPoolID        *int
	InitialPromptSuffix string
	TriggeredByUserID   int
	Trigger             *models.RunTrigger
}

// TokenSpec is the per-run input to RunTokenService.Mint. Phase 4-5 wire
// this from a binding row; for now callers populate it directly.
type TokenSpec struct {
	ActingUserID int
	Scopes       []string
	TTL          time.Duration
	Name         string
}

// RunServiceOptions controls construction. GlobalCap caps the number of
// runs in-flight across the whole process — it sizes the in-process worker
// pool (decision #7), which replaced the old admission semaphore. Start
// enqueues onto a buffered job queue and returns without blocking the HTTP
// handler that called it. Preparer is optional and only
// required when callers actually attach Repo to a RunRequest. Tokens is
// optional and only required when callers attach a TokenSpec. PostRunHook
// is optional and fires once per run after the terminal status is
// finalized — that's where WI-90's PR creation + ItemSCMLink writeback
// live.
type RunServiceOptions struct {
	GlobalCap     int
	Runner        Runner
	Preparer      *repoprep.Preparer
	Tokens        *RunTokenService
	PostRunHook   PostRunHook
	InitialPrompt string
	Now           func() time.Time // injected for deterministic tests
	Logger        *log.Logger
}

// PostRunInfo is what RunService hands to PostRunHook.AfterRun once the
// terminal status has been finalized. Branch + BaseCommit are populated
// only when a worktree was prepared; BindingID is populated only when
// the caller attached it to the request (the binding trigger does).
type PostRunInfo struct {
	RunID       int
	WorkspaceID int
	ItemID      *int
	BindingID   int
	Status      string
	Error       string
	Branch      string
	BaseCommit  string
	// TriggeredByUserID is the audit actor and OAuth credential principal.
	TriggeredByUserID int
	// Summary is already sanitized and bounded before reaching the PR hook.
	Summary string
	// Trigger identifies continuation runs so the PR hook updates the existing PR.
	Trigger *models.RunTrigger
	// Repos carries per-repo push outcomes, primary first; scalar refs remain legacy fallbacks.
	Repos []PostRunRepo
}

// PostRunRepo is one repo's push result handed to the PR hook (WI-449). The run
// service fills only what it knows — RepoSlug + the pushed Branch/BaseCommit;
// the SCM connection and primary flag are resolved by the hook from the binding.
type PostRunRepo struct {
	RepoSlug   string
	Branch     string // empty when the repo had no new commits
	BaseCommit string
}

// BindingInputsResolver derives a binding-backed run's per-run token spec,
// access grants, and runner env at remote claim time, so a remote claim
// prepares the run the same way the local Start path does (WI-195).
// BindingService implements it. It returns (nil, nil, env, nil) for a run
// whose binding mints no token, and (nil, nil, nil, nil) for a run with no
// binding (e.g. action_container) — neither gets token/grant enrichment.
type BindingInputsResolver interface {
	ResolveRunInputs(ctx context.Context, run *models.AgentRun) (*RunInputs, error)
}

// RunInputs bundles everything a binding-backed run needs at remote claim
// time: the per-run token spec, broker grants, repo-prep coordinates, runner
// env, and the per-binding prompt suffix (instructions + skills index,
// WI-258). Nil means "no binding" — the claim proceeds without enrichment.
type RunInputs struct {
	Token  *TokenSpec
	Grants *models.RunGrants
	Repo   *JobRepo
	// Repos is every repository a remote runner must check out, primary first.
	Repos        []JobRepo
	Env          map[string]string
	PromptSuffix string
}

// PostRunHook is the optional post-finalize callback. Errors are logged
// and swallowed by RunService — a misbehaving hook must not affect the
// run's recorded status.
type PostRunHook interface {
	AfterRun(ctx context.Context, info PostRunInfo)
}

// PostRunHookFunc adapts a plain function to PostRunHook.
type PostRunHookFunc func(ctx context.Context, info PostRunInfo)

// AfterRun implements PostRunHook for PostRunHookFunc.
func (f PostRunHookFunc) AfterRun(ctx context.Context, info PostRunInfo) { f(ctx, info) }

const defaultGlobalCap = 8

// ErrShuttingDown is returned from Start once Shutdown has been called.
var ErrShuttingDown = errors.New("run service is shutting down")

// ErrLocalRunnerDisabled is returned from Start for a local (non-pool) run when
// the service runs orchestration-only (no in-process runner). All execution
// happens on remote runner pools; a binding that resolves to a local run on
// such a server is a misconfiguration. The run row is left untouched.
var ErrLocalRunnerDisabled = errors.New("run service: in-process runner is disabled; route this binding to a remote runner pool")

// RunService orchestrates agent runs against the agent_runs table.
type RunService struct {
	repo          *repository.AgentRunRepository
	runner        Runner
	preparer      *repoprep.Preparer
	tokens        *RunTokenService
	postRunHook   PostRunHook
	queue         chan queuedJob
	now           func() time.Time
	logger        *log.Logger
	initialPrompt string

	mu         sync.Mutex
	shutdown   bool
	wg         sync.WaitGroup // counts runs (queued + in-flight)
	workerWG   sync.WaitGroup // counts in-process pool workers
	shutdownCh chan struct{}
	inflightMu sync.Mutex
	inflight   map[int]context.CancelFunc
	claimsMu   sync.Mutex
	claims     map[int]*claimState

	// bindingInputs derives token/grants/env for a binding-backed run at
	// remote claim time (WI-195). Optional; set via SetBindingInputsResolver
	// after construction to break the BindingService<->RunService cycle.
	bindingInputs BindingInputsResolver
}

// SetBindingInputsResolver wires the binding-input resolver used to enrich
// remote claims. Called once at boot after both services are constructed.
func (s *RunService) SetBindingInputsResolver(r BindingInputsResolver) {
	s.bindingInputs = r
}

// NewRunService constructs a RunService bound to the given repo. The
// returned service holds no background goroutines until Start is invoked.
func NewRunService(repo *repository.AgentRunRepository, opts RunServiceOptions) (*RunService, error) {
	if repo == nil {
		return nil, errors.New("run service: repo is required")
	}
	capacity := opts.GlobalCap
	if capacity <= 0 {
		capacity = defaultGlobalCap
	}
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	logger := opts.Logger
	if logger == nil {
		logger = log.Default()
	}
	s := &RunService{
		repo:          repo,
		runner:        opts.Runner,
		preparer:      opts.Preparer,
		tokens:        opts.Tokens,
		postRunHook:   opts.PostRunHook,
		initialPrompt: opts.InitialPrompt,
		queue:         make(chan queuedJob, queueBuffer(capacity)),
		now:           now,
		logger:        logger,
		shutdownCh:    make(chan struct{}),
		inflight:      make(map[int]context.CancelFunc),
		claims:        make(map[int]*claimState),
	}
	// Orchestration-only mode queues and finalizes remote runs without local workers.
	if opts.Runner == nil {
		return s, nil
	}
	// Local workers run the shared RunWorker loop; capacity is the concurrency cap.
	for i := 0; i < capacity; i++ {
		s.workerWG.Add(1)
		go func() {
			defer s.workerWG.Done()
			RunWorker(context.Background(), s, s.runner, s.logger)
		}()
	}
	return s, nil
}

// Cancel marks an in-flight run for cancellation. Returns true if the run
// was actually in flight and got its ctx canceled; false (with no error)
// if the run is no longer in flight (already terminal, never started, or
// the worker already exited). The terminal status is set by the worker's
// own canceled-by-ctx path, not here, so the DB state always reflects what
// the runner actually saw.
func (s *RunService) Cancel(runID int) bool {
	s.inflightMu.Lock()
	cancel, ok := s.inflight[runID]
	s.inflightMu.Unlock()
	if !ok {
		return false
	}
	cancel()
	return true
}

// CancelForBinding requests ordinary cancellation for every queued or active
// run owned by a profile being archived.
func (s *RunService) CancelForBinding(ctx context.Context, bindingID int) error {
	runIDs, err := s.repo.ListActiveIDsForBinding(ctx, bindingID)
	if err != nil {
		return err
	}
	now := s.now()
	for _, runID := range runIDs {
		run, err := s.repo.Get(ctx, runID)
		if err != nil {
			return fmt.Errorf("load run %d for binding archive: %w", runID, err)
		}
		if run.Status == models.AgentRunStatusQueued {
			canceled, err := s.repo.CancelQueued(ctx, runID, now)
			if err != nil {
				return err
			}
			if canceled {
				_ = s.repo.AppendEvent(ctx, runID, "lifecycle", `{"phase":"canceled","reason":"agent profile archived"}`)
			}
			continue
		}
		if run.RunnerID != nil {
			if err := s.repo.RequestCancel(ctx, runID, now); err != nil {
				return err
			}
			continue
		}
		s.Cancel(runID)
	}
	return nil
}

func (s *RunService) registerCancel(runID int, cancel context.CancelFunc) {
	s.inflightMu.Lock()
	defer s.inflightMu.Unlock()
	s.inflight[runID] = cancel
}

func (s *RunService) unregisterCancel(runID int) {
	s.inflightMu.Lock()
	defer s.inflightMu.Unlock()
	delete(s.inflight, runID)
}

func newQueuedAgentRun(req RunRequest) *models.AgentRun {
	run := &models.AgentRun{
		WorkspaceID: req.WorkspaceID,
		ItemID:      req.ItemID,
		Status:      models.AgentRunStatusQueued,
		Trigger:     req.Trigger,
		IsEphemeral: req.Ephemeral,
	}
	if req.BindingID > 0 {
		bindingID := req.BindingID
		run.BindingID = &bindingID
	}
	if req.TriggeredByUserID > 0 {
		userID := req.TriggeredByUserID
		run.TriggeredByUserID = &userID
	}
	if req.TargetPoolID != nil {
		run.TargetPoolID = req.TargetPoolID
		run.JobKind = req.JobKind
		// An empty image lets the remote runner use its default.
		run.JobImage = req.JobImage
	}
	return run
}

// Start records a new run in the queued state and dispatches it onto a
// background goroutine. The returned ID can be used to query state via the
// repository. The caller's ctx is used only for the initial DB insert; the
// run itself derives its context from the service so it survives the
// request handler returning.
func (s *RunService) Start(ctx context.Context, req RunRequest) (int, error) {
	if req.WorkspaceID == 0 {
		return 0, errors.New("run service: workspace_id is required")
	}
	// A local (non-pool) run needs the in-process worker pool to execute it.
	// In orchestration-only mode there is no runner and the queue is never
	// drained, so reject before inserting a row that would sit queued forever.
	if req.TargetPoolID == nil && s.runner == nil {
		return 0, ErrLocalRunnerDisabled
	}
	s.mu.Lock()
	if s.shutdown {
		s.mu.Unlock()
		return 0, ErrShuttingDown
	}
	s.mu.Unlock()

	run := newQueuedAgentRun(req)
	if req.Grants != nil {
		grantsJSON, err := json.Marshal(req.Grants)
		if err != nil {
			return 0, fmt.Errorf("run service: marshal grants: %w", err)
		}
		run.GrantsJSON = string(grantsJSON)
	}
	runID, err := s.repo.Insert(ctx, run)
	if err != nil {
		return 0, fmt.Errorf("insert agent_run: %w", err)
	}
	// Lifecycle event is best-effort: failure to record it must not block
	// the run from proceeding. Remote runs record which pool they queued for
	// so a stalled run's event log answers "where was this supposed to run?".
	queuedPayload := `{"phase":"queued"}`
	if req.TargetPoolID != nil {
		queuedPayload = fmt.Sprintf(`{"phase":"queued","target_pool_id":%d}`, *req.TargetPoolID)
	}
	if err := s.repo.AppendEvent(ctx, runID, "lifecycle", queuedPayload); err != nil {
		s.logger.Printf("run service: append queued event: %v", err)
	}

	// Remote pool: the run is now queued for a remote runner to claim. The
	// in-process worker pool must not touch it — enrichment (token, grants,
	// env) happens in PrepareRemoteClaim when a runner claims it (WI-195).
	if req.TargetPoolID != nil {
		return runID, nil
	}

	if (req.Repo != nil || len(req.Repos) > 0) && s.preparer == nil {
		return 0, errors.New("run service: request includes a Repo but no Preparer is configured")
	}
	if req.Token != nil && s.tokens == nil {
		return 0, errors.New("run service: request includes a Token but no RunTokenService is configured")
	}

	s.mu.Lock()
	if s.shutdown {
		s.mu.Unlock()
		// Lost the race with Shutdown after the row was inserted: no
		// worker will claim it, so finalize it canceled rather than
		// leave it dangling in queued.
		s.finalize(runID, models.AgentRunStatusCanceled, "shutting down")
		return 0, ErrShuttingDown
	}
	s.wg.Add(1)
	// Enqueue for the worker pool. The run row is already persisted as
	// queued; a worker claims it (admission), prepares the worktree, mints
	// the token, and drives the runner — the run outlives the caller's
	// request ctx. Holding mu across the send orders the enqueue before any
	// concurrent Shutdown so the job is never orphaned; the queue buffer is
	// sized so this send does not block under normal load.
	s.queue <- queuedJob{runID: runID, req: req}
	s.mu.Unlock()
	return runID, nil
}

// invokePostRunHook isolates hook failures and bounds worker delay while allowing SCM retries.
func (s *RunService) invokePostRunHook(info PostRunInfo) {
	if s.postRunHook == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	func() {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Printf("run service: post-run hook panic run=%d: %v", info.RunID, r)
			}
		}()
		s.postRunHook.AfterRun(ctx, info)
	}()
}

func (s *RunService) finalize(runID int, status, errMsg string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Scrub embedded URL credentials before persistence. errMsg
	// originates from runner output / git CombinedOutput / exec
	// failures, any of which may include a `https://user:pass@host`
	// fragment if a token slipped through somewhere upstream.
	if err := s.repo.Finalize(ctx, runID, status, RedactString(errMsg), s.now()); err != nil {
		s.logger.Printf("run service: finalize run=%d status=%s: %v", runID, status, err)
	}
}

func (s *RunService) normalizeRunnerResult(ctx context.Context, runID int, result *RunnerResult) string {
	if result.ContainerID != "" {
		if err := s.repo.SetContainerID(ctx, runID, result.ContainerID); err != nil {
			s.logger.Printf("run service: set container_id run=%d: %v", runID, err)
		}
	}
	if models.IsAgentRunTerminal(result.Status) {
		return result.Status
	}
	status := models.AgentRunStatusFailed
	if result.Error == "" {
		result.Error = fmt.Sprintf("runner returned non-terminal status %q", result.Status)
	}
	return status
}

// FinalizeRemote records the terminal verdict for a run executed by a remote
// runner (the in-process path uses Report). It normalizes + finalizes the
// status, emits the terminal event, and fires the post-run hook with the
// branch / base commit the runner reported — so remote runs get the same
// PR-creation + ItemSCMLink writeback as local ones (WI-144). Worktree
// cleanup is the runner's responsibility, so there's none here.
func (s *RunService) FinalizeRemote(ctx context.Context, runID int, result RunnerResult, branch, baseCommit string) error {
	run, err := s.repo.Get(ctx, runID)
	if err != nil {
		return fmt.Errorf("finalize remote: load run %d: %w", runID, err)
	}
	status := s.normalizeRunnerResult(ctx, runID, &result)
	// Compare-and-swap finalize (WI-168): a remote runner credential must not
	// be able to rewrite a run that already finalized or was canceled. If this
	// call did not perform the running→terminal transition, treat the report
	// as a no-op and — crucially — do not re-emit the terminal event or re-run
	// the post-run hook (which would create a duplicate PR).
	transitioned, err := s.repo.FinalizeRunning(ctx, runID, status, RedactString(result.Error), s.now())
	if err != nil {
		return fmt.Errorf("finalize remote: run %d: %w", runID, err)
	}
	if !transitioned {
		s.logger.Printf("run service: ignoring remote result for run=%d (not running)", runID)
		return nil
	}
	if err := s.repo.AppendEvent(ctx, runID, "lifecycle", fmt.Sprintf(`{"phase":%q}`, status)); err != nil {
		s.logger.Printf("run service: append terminal event run=%d: %v", runID, err)
	}
	bindingID := 0
	if run.BindingID != nil {
		bindingID = *run.BindingID
	}
	// WI-197 (finding 6): a remote runner prepares its own worktree off-box and
	// reports the branch + base commit it pushed. The PR hook uses the branch as
	// the PR head ref, so these are untrusted assertions, not facts — validate
	// them here, the single point where remote-reported SCM state reaches the
	// hook (the local in-process path derives both server-side and is trusted).
	branch, baseCommit = s.validateRemoteSCMRefs(ctx, runID, branch, baseCommit)
	// Per-repo results (WI-449): each reported branch is an untrusted assertion,
	// validated the same way as the scalar primary branch before it reaches the
	// PR hook. Repos with no branch (no_changes) are dropped.
	var repos []PostRunRepo
	for _, rr := range result.Repos {
		vb, vbc := s.validateRemoteSCMRefs(ctx, runID, rr.Branch, rr.BaseCommit)
		if vb == "" {
			continue
		}
		repos = append(repos, PostRunRepo{RepoSlug: rr.RepoSlug, Branch: vb, BaseCommit: vbc})
	}
	triggeredBy := 0
	if run.TriggeredByUserID != nil {
		triggeredBy = *run.TriggeredByUserID
	}
	if run.IsEphemeral {
		return nil
	}
	s.invokePostRunHook(PostRunInfo{
		RunID:             runID,
		WorkspaceID:       run.WorkspaceID,
		ItemID:            run.ItemID,
		BindingID:         bindingID,
		Status:            status,
		Error:             RedactString(result.Error),
		Branch:            branch,
		BaseCommit:        baseCommit,
		TriggeredByUserID: triggeredBy,
		Summary:           result.Summary,
		Trigger:           run.Trigger,
		Repos:             repos,
	})
	return nil
}

// RecordFailedStart persists a run that could not start at trigger time —
// e.g. the triggering user has no connected SCM account on an OAuth
// connection (WI-275) — directly in the failed state, with the queued and
// failed lifecycle events, so the refused trigger is visible in the runs
// UI instead of vanishing into a server log. Nothing is dispatched and no
// post-run hook fires. Returns the run id.
func (s *RunService) RecordFailedStart(ctx context.Context, req RunRequest, reason string) (int, error) {
	if req.WorkspaceID == 0 {
		return 0, errors.New("run service: workspace_id is required")
	}
	run := newQueuedAgentRun(req)
	runID, err := s.repo.Insert(ctx, run)
	if err != nil {
		return 0, fmt.Errorf("insert agent_run: %w", err)
	}
	red := RedactString(reason)
	if err := s.repo.AppendEvent(ctx, runID, "lifecycle", `{"phase":"queued"}`); err != nil {
		s.logger.Printf("run service: append queued event: %v", err)
	}
	if err := s.repo.AppendEvent(ctx, runID, "lifecycle", fmt.Sprintf(`{"phase":"failed","reason":%q}`, red)); err != nil {
		s.logger.Printf("run service: append failed event: %v", err)
	}
	if err := s.repo.Finalize(ctx, runID, models.AgentRunStatusFailed, red, s.now()); err != nil {
		return runID, fmt.Errorf("finalize failed-start run %d: %w", runID, err)
	}
	return runID, nil
}

// validateRemoteSCMRefs permits only a run's canonical push branch and full
// 40/64-character base object IDs before the PR hook. Invalid branches drop
// both refs; invalid bases drop only the base. Empty refs mean no PR; rejected
// values are logged and added to the run timeline.
func (s *RunService) validateRemoteSCMRefs(ctx context.Context, runID int, branch, baseCommit string) (validBranch, validBase string) {
	if branch != "" {
		expected := fmt.Sprintf("agent-runs/run-%d", runID)
		if branch != expected {
			s.logger.Printf("run service: remote run=%d reported branch %q, expected %q; dropping branch + base (no PR)",
				runID, clipForEvent(branch), expected)
			_ = s.repo.AppendEvent(ctx, runID, "warning", fmt.Sprintf(
				`{"phase":"scm_ref_rejected","field":"branch","reported":%q,"expected":%q}`,
				clipForEvent(branch), expected))
			return "", ""
		}
	}
	if baseCommit != "" && !isGitObjectID(baseCommit) {
		s.logger.Printf("run service: remote run=%d reported malformed base commit %q; dropping base",
			runID, clipForEvent(baseCommit))
		_ = s.repo.AppendEvent(ctx, runID, "warning", fmt.Sprintf(
			`{"phase":"scm_ref_rejected","field":"base_commit","reported":%q}`, clipForEvent(baseCommit)))
		return branch, ""
	}
	return branch, baseCommit
}

// isGitObjectID accepts only full SHA-1 or SHA-256 object IDs.
func isGitObjectID(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		isHex := c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
		if !isHex {
			return false
		}
	}
	return true
}

// clipForEvent bounds untrusted runner values before logging or persistence.
func clipForEvent(s string) string {
	const maxLen = 120
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}

// mintTokenAndGrants shares identical local/remote token setup. Optional Git
// grants allow only their prepared ref; grant persistence failure safely denies
// brokered access.
func (s *RunService) mintTokenAndGrants(ctx context.Context, runID int, spec TokenSpec, grants *models.RunGrants, refByRepo map[string]string) (string, error) {
	minted, err := s.tokens.Mint(ctx, MintRequest(spec))
	if err != nil {
		return "", err
	}
	_ = s.repo.AppendEvent(ctx, runID, "lifecycle", fmt.Sprintf(
		`{"phase":"token_minted","token_id":%d,"expires_at":%q}`,
		minted.TokenID, minted.ExpiresAt.Format(time.RFC3339)))
	if grants != nil {
		// Copy grants before assigning per-repo push refs; missing refs stay read-only.
		g := *grants
		if len(g.GitRepos) > 0 {
			repos := make([]models.GitGrant, len(g.GitRepos))
			copy(repos, g.GitRepos)
			for i := range repos {
				if ref := refByRepo[repos[i].Repo]; ref != "" {
					repos[i].Ref = ref
				}
			}
			g.GitRepos = repos
		}
		if g.Git != nil {
			gg := *g.Git
			if ref := refByRepo[gg.Repo]; ref != "" {
				gg.Ref = ref
			}
			g.Git = &gg
		}
		if err := s.repo.SetGrants(ctx, runID, minted.TokenID, &g, s.now()); err != nil {
			s.logger.Printf("run service: set grants run=%d: %v", runID, err)
		}
	}
	return minted.Token, nil
}

// applyLLMProxyEnv wires the agent container to reach the model only through
// the run-scoped llm-proxy broker (WI-238): LLM_BASE_URL points at
// <WS_API_URL>/llm-proxy/<runID>/complete and
// LLM_API_KEY carries the per-run token, which ProxyLLM validates and swaps for
// the real provider credential server-side. No raw provider key ever reaches
// the (untrusted) container. A no-op when the run has no LLM grant, or when no
// API base URL is known (the agent then fails loudly with no LLM_BASE_URL
// rather than silently falling back to a direct provider call). Shared by the
// local claim preamble and the remote claim enrichment so both transports
// build identical LLM env.
func applyLLMProxyEnv(env map[string]string, grants *models.RunGrants, runID int, token string) {
	if grants == nil || grants.LLM == nil {
		return
	}
	base := strings.TrimRight(env["WS_API_URL"], "/")
	if base == "" {
		return
	}
	env["LLM_BASE_URL"] = fmt.Sprintf("%s/llm-proxy/%d/complete", base, runID)
	env["LLM_API_KEY"] = token
}

// FailRemoteClaim marks a just-claimed remote run failed when claim enrichment
// could not complete (e.g. PrepareRemoteClaim errored after ClaimQueuedForRunner already
// moved the run to running). Without this the run would sit in `running` with no
// token or grants, holding a pool slot indefinitely (WI-238 security Phase 8).
// CAS-guarded via FinalizeRunning so it never overwrites a run that already
// reached a terminal state; the reason is redacted before it is persisted or
// emitted. No post-run hook fires — no work was produced.
func (s *RunService) FailRemoteClaim(ctx context.Context, runID int, reason string) {
	red := RedactString(reason)
	transitioned, err := s.repo.FinalizeRunning(ctx, runID, models.AgentRunStatusFailed, red, s.now())
	if err != nil {
		s.logger.Printf("run service: fail remote claim run=%d: %v", runID, err)
		return
	}
	if transitioned {
		if err := s.repo.AppendEvent(ctx, runID, "lifecycle", fmt.Sprintf(`{"phase":"failed","reason":%q}`, red)); err != nil {
			s.logger.Printf("run service: append claim-fail event run=%d: %v", runID, err)
		}
	}
}

// PrepareRemoteClaim enriches a run a remote runner just claimed: it derives
// the run's token + grants from its binding (via the resolver), mints the
// per-run token, persists the grants bound to it (git ref = the run-branch
// namespace the remote runner pushes to), and returns the JobSpec the runner
// executes — with $WS_TOKEN and run/workspace/item context in Env. A run with
// no binding (e.g. action_container) is returned with no enrichment. This is
// the remote counterpart of the local claimNext preamble (WI-195).
func (s *RunService) PrepareRemoteClaim(ctx context.Context, run *models.AgentRun) (JobSpec, error) {
	spec := JobSpec{RunID: run.ID, Kind: run.JobKind, Image: run.JobImage, InitialPrompt: s.initialPrompt}
	if run.IsEphemeral {
		spec.InitialPrompt = DefaultTestRunPrompt
	}
	if s.bindingInputs == nil || s.tokens == nil || run.BindingID == nil {
		return spec, nil
	}
	inputs, err := s.bindingInputs.ResolveRunInputs(ctx, run)
	if err != nil {
		return JobSpec{}, fmt.Errorf("remote claim: resolve run inputs: %w", err)
	}
	if inputs == nil {
		inputs = &RunInputs{}
	}
	spec.InitialPrompt += inputs.PromptSuffix
	env := make(map[string]string, len(inputs.Env)+3)
	for key, value := range inputs.Env {
		env[key] = value
	}
	env["AGENT_RUN_ID"] = fmt.Sprintf("%d", run.ID)
	if err := pinSandboxWSEnvironment(env); err != nil {
		return JobSpec{}, fmt.Errorf("remote claim: pin ws destination: %w", err)
	}
	if inputs.Token != nil {
		tokenSpec := *inputs.Token
		if run.IsEphemeral {
			// Defense in depth at the final mint boundary: even a custom input
			// resolver cannot give a private verification run write scopes.
			tokenSpec.Scopes = append([]string(nil), auth.DefaultCodingAgentPrivateTestScopes...)
		}
		// Per-repo push refs the remote runner will create (WI-449): the fresh
		// per-run branch for each repo, or the continuation head branch for the
		// one repo that continues a PR. Each git grant may push only its ref.
		runBranch := fmt.Sprintf("agent-runs/run-%d", run.ID)
		var refByRepo map[string]string
		if !run.IsEphemeral {
			refByRepo = remoteRefByRepo(inputs.Grants, inputs, runBranch)
		}
		token, err := s.mintTokenAndGrants(ctx, run.ID, tokenSpec, inputs.Grants, refByRepo)
		if err != nil {
			return JobSpec{}, fmt.Errorf("remote claim: mint token run=%d: %w", run.ID, err)
		}
		env["WS_TOKEN"] = token
		applyLLMProxyEnv(env, inputs.Grants, run.ID, token)
	}
	spec.Env = env
	// A remote runner prepares its own checkout(s) from these; the host
	// WorkspacePath stays empty on the wire. Repo mirrors the primary for older
	// runners that read the single field.
	spec.Repo = inputs.Repo
	spec.Repos = inputs.Repos
	return spec, nil
}

// remoteRefByRepo maps each granted repo to the branch the run may push
// (WI-449): the fresh per-run branch by default, overridden by a continuation
// head branch for the one repo that continues a PR. Keyed off the grants (the
// authoritative set of repos the run can reach) so it works even when the
// resolver supplies grants without per-repo JobRepo prep inputs.
func remoteRefByRepo(grants *models.RunGrants, inputs *RunInputs, runBranch string) map[string]string {
	refs := map[string]string{}
	if grants != nil {
		for _, gg := range grants.GitRepos {
			refs[gg.Repo] = runBranch
		}
		if grants.Git != nil {
			refs[grants.Git.Repo] = runBranch
		}
	}
	// Continuation overrides: the runner lands commits on the existing PR head
	// branch for the repo that continues, not a fresh per-run branch.
	repos := inputs.Repos
	if len(repos) == 0 && inputs.Repo != nil {
		repos = []JobRepo{*inputs.Repo}
	}
	for _, jr := range repos {
		if jr.ContinueBranch != "" {
			refs[jr.Slug] = jr.ContinueBranch
		}
	}
	return refs
}

// Shutdown stops accepting new runs and waits for in-flight runs to drain.
// Cancellation of in-flight runs is propagated through their context.
func (s *RunService) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	if s.shutdown {
		s.mu.Unlock()
		return nil
	}
	s.shutdown = true
	close(s.shutdownCh)
	s.mu.Unlock()

	// Closing shutdownCh makes the workers drain any still-queued runs as
	// canceled and then exit; in-flight runs see their ctx canceled and
	// finalize. wg drops to zero once every run (queued + in-flight) is
	// accounted for; workerWG drops once the pool has exited.
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		s.workerWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Wait blocks until all currently-dispatched runs complete. Used by tests
// to deterministically wait for an end state; production code should use
// Shutdown.
func (s *RunService) Wait() {
	s.wg.Wait()
}

// HasTokens reports whether a RunTokenService is configured. Used by
// upstream callers (BindingService) to know whether to build a TokenSpec
// for the run.
func (s *RunService) HasTokens() bool {
	return s.tokens != nil
}

// LocalExecutionEnabled reports whether the service runs an in-process worker
// pool. It is false on an orchestration-only server, where all runs execute on
// remote runner pools. Callers that can only run locally (binding test runs)
// use it to fail fast instead of queuing a run nothing will pick up.
func (s *RunService) LocalExecutionEnabled() bool {
	return s.runner != nil
}

// CountRunsForBindingSince proxies to the repository so BindingService
// can enforce a binding's max_runs_per_day budget without taking on a
// direct dependency on the agent_runs repo.
func (s *RunService) CountRunsForBindingSince(ctx context.Context, bindingID int, since time.Time) (int, error) {
	return s.repo.CountForBindingSince(ctx, bindingID, since)
}

// CountActiveRunsForBindingItem proxies to the repository for the
// comment-@mention trigger's per-item dedup check (WI-264).
func (s *RunService) CountActiveRunsForBindingItem(ctx context.Context, bindingID, itemID int) (int, error) {
	return s.repo.CountActiveForBindingItem(ctx, bindingID, itemID)
}

// LatestRunForItem returns the most recent run on an item, or nil when the
// item has never had one. Backs the manual "Re-run" trigger, which derives the
// agent to re-run from the last run's binding.
func (s *RunService) LatestRunForItem(ctx context.Context, itemID int) (*models.AgentRun, error) {
	runs, err := s.repo.ListForItem(ctx, itemID, 1, 0)
	if err != nil {
		return nil, err
	}
	if len(runs) == 0 {
		return nil, nil
	}
	return runs[0], nil
}

func (s *RunService) PRContinuationOwner(ctx context.Context, workspaceID int, repoSlug string, prNumber int) (*repository.AgentPROwnership, error) {
	return s.repo.PRContinuationOwner(ctx, workspaceID, repoSlug, prNumber)
}

func (s *RunService) LatestRunForBindingItem(ctx context.Context, bindingID, itemID int) (*models.AgentRun, error) {
	return s.repo.LatestForBindingItem(ctx, bindingID, itemID)
}
