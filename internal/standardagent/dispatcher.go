// Package standardagent implements the durable, in-process Agent Studio
// runtime for Standard profiles.
package standardagent

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"windshift/internal/aitooladapter"
	"windshift/internal/aitools"
	"windshift/internal/database"
	"windshift/internal/itemevents"
	"windshift/internal/llm"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

const (
	maxChainDepth        = 8
	defaultRunTimeout    = 5 * time.Minute
	defaultMaxIterations = 10
)

var (
	ErrAdmissionDenied = errors.New("standard agent: trigger requires item.edit permission")
	ErrLineageMissing  = errors.New("standard agent: agent trigger has no active parent run")
	ErrChainLimit      = errors.New("standard agent: maximum agent chain depth reached")
)

type LLMResolver interface {
	Resolve(connectionID int) (llm.Client, error)
}

type Options struct {
	DB                     database.Database
	Runs                   *repository.AgentRunRepository
	Bindings               *repository.WorkspaceAgentBindingRepository
	LLMs                   LLMResolver
	Permissions            *services.PermissionService
	TimePermissions        *services.TimePermissionService
	Timers                 *services.TimerService
	Comments               *services.CommentService
	Approvals              *services.ApprovalService
	Notifications          *services.NotificationService
	ActionService          *services.ActionService
	PageApplicationService *services.PageApplicationService
	PageDiagramService     *services.PageDiagramService
	Registry               *aitools.Registry
	RunTimeout             time.Duration
}

// ProfileSnapshot is deliberately limited to non-secret execution inputs.
// A queued run does not silently adopt later profile edits.
type ProfileSnapshot struct {
	BindingID        int      `json:"binding_id"`
	ProfileVersion   int      `json:"profile_version"`
	ActingUserID     int      `json:"acting_user_id"`
	ActingName       string   `json:"acting_name"`
	LLMConnectionID  int      `json:"llm_connection_id"`
	Instructions     string   `json:"instructions"`
	Purpose          string   `json:"purpose"`
	CapabilityGroups []string `json:"capability_groups"`
	ToolNames        []string `json:"tool_names"`
}

type Dispatcher struct {
	opts Options

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu         sync.Mutex
	workers    map[repository.StandardQueueKey]bool
	runCancels map[int]context.CancelFunc
	closed     bool
}

func New(opts Options) (*Dispatcher, error) {
	if opts.DB == nil || opts.Runs == nil || opts.Bindings == nil || opts.LLMs == nil ||
		opts.Permissions == nil || opts.Comments == nil {
		return nil, errors.New("standard agent: db, runs, bindings, llms, permissions, and comments are required")
	}
	if opts.Registry == nil {
		opts.Registry = aitools.Default
	}
	if opts.RunTimeout <= 0 {
		opts.RunTimeout = defaultRunTimeout
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Dispatcher{
		opts:       opts,
		ctx:        ctx,
		cancel:     cancel,
		workers:    make(map[repository.StandardQueueKey]bool),
		runCancels: make(map[int]context.CancelFunc),
	}, nil
}

// Resume repairs interrupted in-process calls and starts every durable queue.
func (d *Dispatcher) Resume(ctx context.Context) error {
	now := time.Now().UTC()
	orphaned, err := d.opts.Runs.ListRunningStandard(ctx)
	if err != nil {
		return err
	}
	if _, err := d.opts.Runs.FailOrphanedStandardRuns(ctx, now); err != nil {
		return err
	}
	for _, run := range orphaned {
		message := "The Standard agent was interrupted by a server restart."
		_ = d.opts.Runs.AppendEvent(ctx, run.ID, "failed", marshalSafe(map[string]any{"message": message}))
		if run.RootInitiatorUserID != nil && run.ItemID != nil && run.ActingUserID != nil {
			d.notifyFailure(*run.RootInitiatorUserID, run.WorkspaceID, *run.ItemID, *run.ActingUserID,
				"Agent run interrupted", message)
		}
	}
	keys, err := d.opts.Runs.ListQueuedStandardKeys(ctx)
	if err != nil {
		return err
	}
	for _, key := range keys {
		d.kick(key)
	}
	return nil
}

func (d *Dispatcher) StartItemRun(ctx context.Context, binding *models.WorkspaceAgentBinding, workspaceID, itemID, triggeredByUserID int, trigger *models.RunTrigger) error {
	if binding == nil || binding.ProfileType != models.AgentProfileStandard ||
		binding.Lifecycle != models.AgentLifecycleReady || binding.WorkspaceID != workspaceID {
		return services.ErrBindingUnavailable
	}
	canEdit, err := d.opts.Permissions.HasWorkspacePermission(triggeredByUserID, workspaceID, models.PermissionItemEdit)
	if err != nil {
		return fmt.Errorf("standard agent admission: %w", err)
	}
	if !canEdit {
		return ErrAdmissionDenied
	}

	rootID, immediateID, parentID, depth, err := d.lineage(ctx, triggeredByUserID, itemID)
	if err != nil {
		return err
	}
	if depth > maxChainDepth {
		d.notifyFailure(rootID, workspaceID, itemID, binding.ActingUserID,
			"Agent chain stopped", "The maximum Standard-agent chain depth was reached.")
		return ErrChainLimit
	}

	entries := aitooladapter.EntriesForStandard(d.opts.Registry, binding.CapabilityGroups)
	toolNames := make([]string, 0, len(entries))
	for _, entry := range entries {
		toolNames = append(toolNames, entry.Name)
	}
	snapshot := ProfileSnapshot{
		BindingID:        binding.ID,
		ProfileVersion:   binding.ProfileVersion,
		ActingUserID:     binding.ActingUserID,
		ActingName:       d.userDisplayName(ctx, binding.ActingUserID),
		Instructions:     binding.Instructions,
		Purpose:          binding.Purpose,
		CapabilityGroups: append([]string(nil), binding.CapabilityGroups...),
		ToolNames:        toolNames,
	}
	if binding.LLMConnectionID != nil {
		snapshot.LLMConnectionID = *binding.LLMConnectionID
	}
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("snapshot Standard profile: %w", err)
	}
	grantsJSON, err := json.Marshal(map[string]any{
		"workspace_ids": []int{workspaceID},
		"tools":         toolNames,
	})
	if err != nil {
		return fmt.Errorf("snapshot Standard grants: %w", err)
	}

	bindingID := binding.ID
	actingID := binding.ActingUserID
	run := &models.AgentRun{
		WorkspaceID:            workspaceID,
		ItemID:                 &itemID,
		BindingID:              &bindingID,
		Status:                 models.AgentRunStatusQueued,
		JobKind:                models.JobKindStandardAgent,
		TriggeredByUserID:      &immediateID,
		ActingUserID:           &actingID,
		RootInitiatorUserID:    &rootID,
		ImmediateTriggerUserID: &immediateID,
		ParentRunID:            parentID,
		ChainDepth:             depth,
		ProfileVersion:         binding.ProfileVersion,
		GrantsJSON:             string(grantsJSON),
		ProfileSnapshotJSON:    string(snapshotJSON),
		Trigger:                trigger,
	}
	runID, err := d.opts.Runs.Insert(ctx, run)
	if err != nil {
		return err
	}
	_ = d.opts.Runs.AppendEvent(ctx, runID, "queued", marshalSafe(map[string]any{
		"job_kind":        models.JobKindStandardAgent,
		"profile_version": binding.ProfileVersion,
		"chain_depth":     depth,
	}))
	d.kick(repository.StandardQueueKey{BindingID: binding.ID, ItemID: itemID})
	return nil
}

// RunPrivateTest exercises the same LLM loop and canonical adapter as a real
// Standard run, but exposes only read tools and persists no conversation,
// comment, or work-item mutation. It may run while a profile is still Draft.
func (d *Dispatcher) RunPrivateTest(ctx context.Context, binding *models.WorkspaceAgentBinding, workspaceID, triggeredByUserID int, prompt string) (*services.StandardPrivateTestResult, error) {
	if binding == nil || binding.ProfileType != models.AgentProfileStandard ||
		binding.WorkspaceID != workspaceID || binding.Lifecycle == models.AgentLifecycleArchived {
		return nil, services.ErrBindingUnavailable
	}
	if binding.LLMConnectionID == nil {
		return nil, services.ErrLLMConnectionRequired
	}
	required, err := d.opts.Permissions.HasWorkspacePermissions(binding.ActingUserID, workspaceID,
		[]string{models.PermissionItemView, models.PermissionItemComment})
	if err != nil {
		return nil, fmt.Errorf("private Standard test permission check: %w", err)
	}
	if !required[models.PermissionItemView] || !required[models.PermissionItemComment] {
		return nil, errors.New("acting identity no longer has required item permissions")
	}
	client, err := d.opts.LLMs.Resolve(*binding.LLMConnectionID)
	if err != nil {
		return nil, err
	}

	allEntries := aitooladapter.EntriesForStandard(d.opts.Registry, binding.CapabilityGroups)
	entries := make([]aitools.Entry, 0, len(allEntries))
	for _, entry := range allEntries {
		if entry.Access == aitools.AccessRead {
			entries = append(entries, entry)
		}
	}
	timezone, err := services.LookupUserTimezone(d.opts.DB, binding.ActingUserID)
	if err != nil {
		return nil, fmt.Errorf("resolve acting identity timezone: %w", err)
	}
	executor := aitooladapter.NewExecutor(&aitools.Env{
		DB:                     d.opts.DB,
		UserID:                 binding.ActingUserID,
		Username:               d.userDisplayName(ctx, binding.ActingUserID),
		Timezone:               timezone,
		Source:                 aitools.SourceStandardAgent,
		AccessibleWorkspaceIDs: []int{workspaceID},
		AuditDetails: map[string]any{
			"agent_profile_id":       binding.ID,
			"agent_profile_version":  binding.ProfileVersion,
			"root_initiator_user_id": triggeredByUserID,
			"acting_user_id":         binding.ActingUserID,
			"workspace_id":           workspaceID,
			"private_test":           true,
		},
		PermService:            d.opts.Permissions,
		TimePermService:        d.opts.TimePermissions,
		TimerService:           d.opts.Timers,
		CommentService:         d.opts.Comments,
		ApprovalService:        d.opts.Approvals,
		ActionService:          d.opts.ActionService,
		PageApplicationService: d.opts.PageApplicationService,
		PageDiagramService:     d.opts.PageDiagramService,
	}, entries)
	if strings.TrimSpace(prompt) == "" {
		prompt = "Confirm that you can access this workspace and briefly summarize what you can help with. Do not modify anything."
	}
	result, err := llm.RunAgent(ctx, client, llm.AgentConfig{
		SystemPrompt: fmt.Sprintf(`You are %s, a Standard workspace agent in a private administrator test.
Use only the provided read-only tools and only within workspace %d.
Do not create, edit, delete, approve, comment, or otherwise mutate data.

Purpose: %s

Profile instructions:
%s`, d.userDisplayName(ctx, binding.ActingUserID), workspaceID,
			strings.TrimSpace(binding.Purpose), strings.TrimSpace(binding.Instructions)),
		Tools:         aitooladapter.BuildTools(entries),
		MaxIterations: defaultMaxIterations,
		Timeout:       d.opts.RunTimeout,
		MaxTokens:     2048,
		Temperature:   0.2,
	}, prompt, executor.Execute, nil)
	if err != nil {
		return nil, err
	}
	if result.StopReason != llm.StopReasonDone || strings.TrimSpace(result.Answer) == "" {
		return nil, errors.New("private Standard test stopped before producing a final answer")
	}
	for _, call := range result.ToolCalls {
		var body map[string]any
		if json.Unmarshal([]byte(call.Result), &body) == nil && body["error"] != nil {
			return nil, errors.New("one or more private-test tool calls failed")
		}
	}
	return &services.StandardPrivateTestResult{
		Answer:     result.Answer,
		Iterations: result.Iterations,
		ToolCalls:  len(result.ToolCalls),
	}, nil
}

func (d *Dispatcher) lineage(ctx context.Context, triggerUserID, itemID int) (rootID, immediateID int, parentID *int, depth int, err error) {
	var isAgent bool
	if err = d.opts.DB.QueryRowContext(ctx,
		`SELECT COALESCE(is_agent, false) FROM users WHERE id = ?`, triggerUserID).Scan(&isAgent); err != nil {
		return 0, 0, nil, 0, fmt.Errorf("load trigger identity: %w", err)
	}
	if !isAgent {
		return triggerUserID, triggerUserID, nil, 0, nil
	}
	parent, err := d.opts.Runs.RunningStandardForActorItem(ctx, triggerUserID, itemID)
	if err != nil {
		return 0, 0, nil, 0, err
	}
	if parent == nil {
		return 0, 0, nil, 0, ErrLineageMissing
	}
	rootID = triggerUserID
	if parent.RootInitiatorUserID != nil {
		rootID = *parent.RootInitiatorUserID
	}
	id := parent.ID
	return rootID, triggerUserID, &id, parent.ChainDepth + 1, nil
}

func (d *Dispatcher) kick(key repository.StandardQueueKey) {
	d.mu.Lock()
	if d.closed || d.workers[key] {
		d.mu.Unlock()
		return
	}
	d.workers[key] = true
	d.wg.Add(1)
	d.mu.Unlock()

	go d.drain(key)
}

func (d *Dispatcher) drain(key repository.StandardQueueKey) {
	defer d.wg.Done()
	defer func() {
		d.mu.Lock()
		delete(d.workers, key)
		closed := d.closed
		d.mu.Unlock()
		if closed {
			return
		}
		// Close the enqueue-vs-worker-exit race without polling or sleeps.
		// The existence check targets this queue's key so a worker exit costs a
		// pointed index lookup, not a whole-table DISTINCT.
		queued, err := d.opts.Runs.HasQueuedStandardRun(d.ctx, key.BindingID, key.ItemID)
		if err != nil {
			slog.Error("failed to recheck Standard queue", "binding_id", key.BindingID, "item_id", key.ItemID, "error", err)
			return
		}
		if queued {
			d.kick(key)
		}
	}()

	for {
		if err := d.ctx.Err(); err != nil {
			return
		}
		run, err := d.opts.Runs.ClaimNextStandard(d.ctx, key.BindingID, key.ItemID, time.Now().UTC())
		if err != nil {
			slog.Error("failed to claim Standard run", "binding_id", key.BindingID, "item_id", key.ItemID, "error", err)
			return
		}
		if run == nil {
			return
		}
		d.execute(run)
	}
}

func (d *Dispatcher) execute(run *models.AgentRun) {
	ctx, cancel := context.WithTimeout(d.ctx, d.opts.RunTimeout)
	d.mu.Lock()
	d.runCancels[run.ID] = cancel
	d.mu.Unlock()
	defer func() {
		cancel()
		d.mu.Lock()
		delete(d.runCancels, run.ID)
		d.mu.Unlock()
	}()

	_ = d.opts.Runs.AppendEvent(ctx, run.ID, "running", marshalSafe(map[string]any{
		"profile_version": run.ProfileVersion,
		"chain_depth":     run.ChainDepth,
	}))

	var snapshot ProfileSnapshot
	if err := json.Unmarshal([]byte(run.ProfileSnapshotJSON), &snapshot); err != nil {
		d.fail(run, fmt.Errorf("invalid profile snapshot: %w", err))
		return
	}
	client, err := d.opts.LLMs.Resolve(snapshot.LLMConnectionID)
	if err != nil {
		d.fail(run, err)
		return
	}
	required, err := d.opts.Permissions.HasWorkspacePermissions(snapshot.ActingUserID, run.WorkspaceID,
		[]string{models.PermissionItemView, models.PermissionItemComment})
	if err != nil || !required[models.PermissionItemView] || !required[models.PermissionItemComment] {
		d.fail(run, errors.New("acting identity no longer has required item permissions"))
		return
	}

	entries := entriesByName(d.opts.Registry, snapshot.ToolNames)
	timezone, err := services.LookupUserTimezone(d.opts.DB, snapshot.ActingUserID)
	if err != nil {
		d.fail(run, fmt.Errorf("resolve acting identity timezone: %w", err))
		return
	}
	executor := aitooladapter.NewExecutor(&aitools.Env{
		DB:                     d.opts.DB,
		UserID:                 snapshot.ActingUserID,
		Username:               snapshot.ActingName,
		Timezone:               timezone,
		Source:                 aitools.SourceStandardAgent,
		RunID:                  run.ID,
		AccessibleWorkspaceIDs: []int{run.WorkspaceID},
		AuditDetails: map[string]any{
			"agent_run_id":              run.ID,
			"agent_profile_id":          snapshot.BindingID,
			"agent_profile_version":     snapshot.ProfileVersion,
			"agent_session_id":          run.SessionID,
			"root_initiator_user_id":    optionalInt(run.RootInitiatorUserID),
			"immediate_trigger_user_id": optionalInt(run.ImmediateTriggerUserID),
			"acting_user_id":            snapshot.ActingUserID,
			"parent_run_id":             optionalInt(run.ParentRunID),
			"chain_depth":               run.ChainDepth,
			"workspace_id":              run.WorkspaceID,
			"item_id":                   optionalInt(run.ItemID),
		},
		PermService:            d.opts.Permissions,
		TimePermService:        d.opts.TimePermissions,
		TimerService:           d.opts.Timers,
		CommentService:         d.opts.Comments,
		ApprovalService:        d.opts.Approvals,
		ActionService:          d.opts.ActionService,
		PageApplicationService: d.opts.PageApplicationService,
		PageDiagramService:     d.opts.PageDiagramService,
	}, entries)

	result, err := llm.RunAgent(ctx, client, llm.AgentConfig{
		SystemPrompt:  systemPrompt(snapshot, run),
		Tools:         aitooladapter.BuildTools(entries),
		TerminalTools: aitooladapter.TerminalTools(entries),
		MaxIterations: defaultMaxIterations,
		Timeout:       d.opts.RunTimeout,
		MaxTokens:     2048,
		Temperature:   0.2,
	}, userPrompt(run), executor.Execute, nil)
	if err != nil {
		d.fail(run, err)
		return
	}
	toolFailed := false
	for _, call := range result.ToolCalls {
		status := "succeeded"
		var body map[string]any
		if json.Unmarshal([]byte(call.Result), &body) == nil && body["error"] != nil {
			status = "failed"
			toolFailed = true
		}
		_ = d.opts.Runs.AppendEvent(ctx, run.ID, "tool", marshalSafe(map[string]any{
			"name":   call.Name,
			"status": status,
		}))
	}
	if toolFailed {
		d.fail(run, errors.New("one or more admitted tool calls failed"))
		return
	}
	if result.StopReason != llm.StopReasonDone || strings.TrimSpace(result.Answer) == "" {
		d.fail(run, errors.New("agent stopped before producing a final answer"))
		return
	}

	private := d.inheritedPrivacy(ctx, run.Trigger)
	comment, err := d.opts.Comments.Create(services.CreateCommentParams{
		ItemID:                optionalInt(run.ItemID),
		AuthorID:              snapshot.ActingUserID,
		Content:               result.Answer,
		IsPrivate:             private,
		ActorUserID:           snapshot.ActingUserID,
		SuppressNotifications: false,
		EventMetadata:         itemevents.Agent(fmt.Sprint(run.ID), "agent"),
	})
	if err != nil {
		d.fail(run, fmt.Errorf("post final comment: %w", err))
		return
	}
	now := time.Now().UTC()
	if _, err := d.opts.Runs.FinalizeRunning(ctx, run.ID, models.AgentRunStatusSucceeded, "", now); err != nil {
		slog.Error("failed to finalize Standard run", "run_id", run.ID, "error", err)
		return
	}
	_ = d.opts.Runs.AppendEvent(ctx, run.ID, "succeeded", marshalSafe(map[string]any{
		"comment_id":         comment.CommentID,
		"iterations":         result.Iterations,
		"prompt_tokens":      result.Usage.PromptTokens,
		"completion_tokens":  result.Usage.CompletionTokens,
		"cache_read_tokens":  result.Usage.CacheReadTokens,
		"cache_write_tokens": result.Usage.CacheWriteTokens,
		"reasoning_tokens":   result.Usage.ReasoningTokens,
	}))
}

func (d *Dispatcher) fail(run *models.AgentRun, cause error) {
	if cause != nil {
		slog.Warn("Standard agent run failed", "run_id", run.ID, "error_type", fmt.Sprintf("%T", cause))
	}
	status := models.AgentRunStatusFailed
	message := "The Standard agent could not complete this run."
	if errors.Is(cause, context.Canceled) {
		status = models.AgentRunStatusCanceled
		message = "The Standard agent run was canceled."
	} else if errors.Is(cause, context.DeadlineExceeded) {
		message = "The Standard agent run timed out."
	}
	now := time.Now().UTC()
	transitioned, err := d.opts.Runs.FinalizeRunning(context.Background(), run.ID, status, message, now)
	if err != nil {
		slog.Error("failed to persist Standard run failure", "run_id", run.ID, "error", err)
		return
	}
	if !transitioned {
		return
	}
	_ = d.opts.Runs.AppendEvent(context.Background(), run.ID, status, marshalSafe(map[string]any{"message": message}))
	if status == models.AgentRunStatusFailed && run.RootInitiatorUserID != nil && run.ItemID != nil && run.ActingUserID != nil {
		d.notifyFailure(*run.RootInitiatorUserID, run.WorkspaceID, *run.ItemID, *run.ActingUserID,
			"Agent run failed", message)
	}
}

func (d *Dispatcher) notifyFailure(rootID, workspaceID, itemID, actingID int, title, message string) {
	if d.opts.Notifications == nil || rootID <= 0 {
		return
	}
	if err := d.opts.Notifications.NotifyUsers([]int{rootID}, workspaceID, itemID, actingID,
		"agent_run_failed", title, message); err != nil {
		slog.Warn("failed to notify Standard run initiator", "user_id", rootID, "item_id", itemID, "error", err)
	}
}

func (d *Dispatcher) inheritedPrivacy(ctx context.Context, trigger *models.RunTrigger) bool {
	if trigger == nil || trigger.CommentID <= 0 {
		return false
	}
	var private bool
	if err := d.opts.DB.QueryRowContext(ctx,
		`SELECT COALESCE(is_private, false) FROM comments WHERE id = ?`, trigger.CommentID).Scan(&private); err != nil {
		return false
	}
	return private
}

func (d *Dispatcher) userDisplayName(ctx context.Context, userID int) string {
	var username, firstName, lastName sql.NullString
	if err := d.opts.DB.QueryRowContext(ctx,
		`SELECT username, first_name, last_name FROM users WHERE id = ?`, userID).
		Scan(&username, &firstName, &lastName); err != nil {
		return fmt.Sprintf("Agent #%d", userID)
	}
	if name := strings.TrimSpace(firstName.String + " " + lastName.String); name != "" {
		return name
	}
	return username.String
}

// CancelForBinding cancels queued work durably and signals active LLM calls.
func (d *Dispatcher) CancelForBinding(ctx context.Context, bindingID int) error {
	ids, err := d.opts.Runs.ListActiveStandardIDsForBinding(ctx, bindingID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, id := range ids {
		canceled, err := d.opts.Runs.CancelQueued(ctx, id, now)
		if err != nil {
			return err
		}
		if canceled {
			_ = d.opts.Runs.AppendEvent(ctx, id, "canceled", `{"reason":"profile archived"}`)
			continue
		}
		if err := d.opts.Runs.RequestCancel(ctx, id, now); err != nil {
			return err
		}
		d.mu.Lock()
		cancel := d.runCancels[id]
		d.mu.Unlock()
		if cancel != nil {
			cancel()
		}
	}
	return nil
}

func (d *Dispatcher) Close(ctx context.Context) error {
	d.mu.Lock()
	if !d.closed {
		d.closed = true
		d.cancel()
	}
	d.mu.Unlock()
	done := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func entriesByName(registry *aitools.Registry, names []string) []aitools.Entry {
	allowed := make(map[string]bool, len(names))
	for _, name := range names {
		allowed[name] = true
	}
	var out []aitools.Entry
	for _, entry := range registry.All() {
		if allowed[entry.Name] && entry.Access != aitools.AccessDestructive && entry.Access != aitools.AccessAdmin {
			out = append(out, entry)
		}
	}
	return out
}

func systemPrompt(snapshot ProfileSnapshot, run *models.AgentRun) string {
	return fmt.Sprintf(`You are %s, a Standard workspace agent.
Act only through the provided tools and only within workspace %d.
The work item for this run is %d. Read it before deciding what to do.
Your final response will be posted as your comment on that item. Be concise,
state what you completed, and never claim a mutation succeeded unless its tool
call succeeded.

Purpose: %s

Profile instructions:
%s`, snapshot.ActingName, run.WorkspaceID, optionalInt(run.ItemID),
		strings.TrimSpace(snapshot.Purpose), strings.TrimSpace(snapshot.Instructions))
}

func userPrompt(run *models.AgentRun) string {
	if run.Trigger != nil && strings.TrimSpace(run.Trigger.Instruction) != "" {
		return run.Trigger.Instruction
	}
	kind := "assignment"
	if run.Trigger != nil && run.Trigger.Kind != "" {
		kind = run.Trigger.Kind
	}
	return fmt.Sprintf("Handle work item %d. Trigger: %s.", optionalInt(run.ItemID), kind)
}

func optionalInt(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

func marshalSafe(value any) string {
	body, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(body)
}
