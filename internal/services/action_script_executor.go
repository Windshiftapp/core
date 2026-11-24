package services

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/grafana/sobek"

	"windshift/internal/models"
	"windshift/internal/operatorcontrols"
	"windshift/internal/services/actioncatalog"
)

// Budgets for one execute_script node. Source length reuses maxScriptLength
// (10 KB) from the shared script engine. The values are server-enforced hard
// caps; node config may narrow them but never widen them.
const (
	defaultScriptAPITimeoutMs     = 5000
	maxScriptAPITimeoutMs         = 30000
	maxScriptAPICalls             = 50
	defaultScriptAPIResponseBytes = 256 * 1024
	maxScriptRequestBodyBytes     = 256 * 1024
	maxScriptResultBytes          = 64 * 1024
	maxScriptResultDepth          = 20
	maxScriptConsoleBytes         = 16 * 1024
	maxScriptConsoleEntries       = 200
	maxScriptConsoleEntryBytes    = 1024
	defaultScriptOutputVariable   = "script_result"
)

// ActionScriptWriteMarker persists a write-ahead marker before a potentially
// mutating script request is dispatched. If the worker dies between the commit
// and the terminal execution-log update, the durable log then reads
// reconciliation_required instead of running, so recovery blocks automatic
// replay of an already-applied write.
type ActionScriptWriteMarker interface {
	MarkScriptWritePending(executionLogID int) error
}

// ExecuteScriptExecutor implements the execute_script node type. Each run gets
// a fresh Sobek runtime in a dedicated worker process, so a hostile allocation
// is bounded by the worker's heap budget instead of the API server's. REST calls
// go through a short-lived bearer token minted for the effective executor in the
// parent and revoked in cleanup; the worker never sees the token or the
// database.
//
// The worker enforces a wall-clock limit and a heap budget, and on Linux the
// parent kills it if its resident set passes the budget. A script that exceeds
// either budget fails the node with a bounded error.
//
// Side effects are not transactional. A script that mutates state and then
// fails has already committed those writes, matching every other action node.
// Durable replay reuses the first attempt's execution log, but a crash between
// a write and the log completion can replay the script; operators must treat
// partial script side effects as reconciliation-required rather than assume
// whole-script atomicity.
type ExecuteScriptExecutor struct {
	controls   *operatorcontrols.Controls
	tokens     ActionScriptTokenManager
	dispatcher ScriptAPIDispatcher
	marker     ActionScriptWriteMarker
	logger     *slog.Logger
	launcher   ScriptWorkerLauncher
}

// NewExecuteScriptExecutor wires the executor with the shared token manager
// and the canonical v2 dispatcher. Both are required; Execute fails closed when
// either is missing.
func NewExecuteScriptExecutor(tokens ActionScriptTokenManager, dispatcher ScriptAPIDispatcher) *ExecuteScriptExecutor {
	return &ExecuteScriptExecutor{
		tokens:     tokens,
		dispatcher: dispatcher,
		logger:     slog.Default(),
		launcher:   defaultScriptWorkerLauncher,
	}
}

// WithWriteMarker wires the write-ahead reconciliation marker. Without one,
// crash recovery cannot distinguish an in-flight write from an untouched
// running execution, so mutating scripts fail closed.
func (e *ExecuteScriptExecutor) WithWriteMarker(marker ActionScriptWriteMarker) *ExecuteScriptExecutor {
	e.marker = marker
	return e
}

// WithLauncher overrides the isolated worker launcher. Tests use it to run the
// worker in-process; production uses the subprocess launcher.
func (e *ExecuteScriptExecutor) WithLauncher(launcher ScriptWorkerLauncher) *ExecuteScriptExecutor {
	e.launcher = launcher
	return e
}

// NodeType pins the dispatch key.
func (e *ExecuteScriptExecutor) NodeType() models.ActionNodeType {
	return models.ActionNodeExecuteScript
}

// scriptHost holds the parent-side mutable state of one node execution. It is
// created per Execute call and never shared across executions. The Sobek realm
// lives in the isolated worker; this struct owns the credentials, budgets,
// console buffer, and reconciliation flags the worker reports against.
type scriptHost struct {
	runCtx         context.Context
	tokens         ActionScriptTokenManager
	dispatcher     ScriptAPIDispatcher
	marker         ActionScriptWriteMarker
	executionLogID int
	actorUserID    int
	maxAPICalls    int
	apiCalls       int
	console        []string
	consoleBytes   int
	consoleCut     bool
	fatal          error
	reconciliation *bool
	writeMarked    *bool
	cred           *ActionScriptCredential
	credOnce       sync.Once
	credErr        error
}

// Execute runs the configured function body exactly once. There is no
// execute-then-retry fallback: a retry after a runtime error could repeat host
// side effects.
func (e *ExecuteScriptExecutor) Execute(node *models.ActionNode, ctx *models.ExecutionContext, stepResult *models.StepResult) error {
	if e != nil {
		if err := e.controls.Check(operatorcontrols.Scripts); err != nil {
			return err
		}
	}
	if e == nil || e.tokens == nil {
		return errors.New("execute_script: token manager is not configured")
	}
	if e.dispatcher == nil {
		return errors.New("execute_script: api dispatcher is not configured")
	}
	if ctx == nil {
		return errors.New("execute_script: execution context is required")
	}
	release, err := e.controls.Acquire(actionRequestContext(ctx), "scripts", e.scriptConcurrencyLimit())
	if err != nil {
		return err
	}
	defer release()
	var config models.ExecuteScriptNodeConfig
	if err := json.Unmarshal([]byte(node.NodeConfig), &config); err != nil {
		return fmt.Errorf("execute_script: invalid config: %w", err)
	}
	source := strings.TrimSpace(config.Source)
	if source == "" {
		return errors.New("execute_script: source is required")
	}
	if len(config.Source) > maxScriptLength {
		return fmt.Errorf("execute_script: source exceeds %d bytes", maxScriptLength)
	}
	outputVar := strings.TrimSpace(config.OutputVariable)
	if outputVar == "" {
		outputVar = defaultScriptOutputVariable
	}
	if err := validateScriptOutputVariable(outputVar); err != nil {
		return err
	}
	if ctx.EffectiveActorID <= 0 {
		return errors.New("execute_script: no effective executor identity; configure an actor override or trigger with an authenticated user")
	}
	if ctx.Variables == nil {
		ctx.Variables = make(map[string]any)
	}

	timeoutMs := config.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = defaultScriptAPITimeoutMs
	} else if timeoutMs > maxScriptAPITimeoutMs {
		timeoutMs = maxScriptAPITimeoutMs
	}
	if ceiling := e.controls.Limits().ScriptTimeoutMS; ceiling > 0 && timeoutMs > ceiling {
		timeoutMs = ceiling
	}
	maxCalls := config.MaxAPICalls
	if maxCalls <= 0 || maxCalls > maxScriptAPICalls {
		maxCalls = maxScriptAPICalls
	}

	// Compile in the parent so a syntax error fails fast without spawning a
	// worker. The worker compiles the same wrapper again before running it.
	wrapped := "(function() {\n" + source + "\n})()"
	if _, err := sobek.Compile("action_script", wrapped, false); err != nil {
		return fmt.Errorf("execute_script: syntax error: %w", err)
	}

	// One absolute deadline covers worker startup, JS, and REST calls. The same
	// instant reaches the worker, so a slow start cannot hand a script a second
	// full timeout, and the worker is force-killed a short grace later so it can
	// still report its own failure.
	deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
	runCtx, cancelRun := context.WithDeadline(actionRequestContext(ctx), deadline)
	defer cancelRun()
	killCtx, cancelKill := context.WithDeadline(actionRequestContext(ctx), deadline.Add(scriptWorkerKillGrace))
	defer cancelKill()

	host := &scriptHost{
		tokens:         e.tokens,
		dispatcher:     e.dispatcher,
		marker:         e.marker,
		executionLogID: ctx.ExecutionLogID,
		actorUserID:    ctx.EffectiveActorID,
		maxAPICalls:    maxCalls,
		runCtx:         WithActionScriptProvenance(runCtx, provenanceFromExecution(ctx, node.ID)),
		reconciliation: &ctx.ReconciliationRequired,
		writeMarked:    &ctx.ScriptWriteMarked,
	}

	// Revocation is deferred so it runs even if the worker fails. The credential
	// is minted lazily on the first bridge call and must not outlive the node.
	defer func() {
		if host.cred == nil {
			return
		}
		if revokeErr := RevokeActionScriptCredential(e.tokens, host.cred, ctx.EffectiveActorID); revokeErr != nil {
			actionID := 0
			if ctx.Action != nil {
				actionID = ctx.Action.ID
			}
			e.logger.Warn("failed to revoke execute_script credential",
				slog.String("component", "actions"),
				slog.Int("action_id", actionID),
				slog.Any("error", revokeErr),
			)
		}
	}()

	request := scriptWorkerMessage{
		Type:             scriptMessageRun,
		Source:           source,
		TimeoutMS:        timeoutMs,
		DeadlineUnixNano: deadline.UnixNano(),
		MemoryBytes:      e.scriptMemoryBytes(),
		Globals:          scriptGlobals(ctx),
	}
	output, runErr := e.runWorker(killCtx, request, host)

	if stepResult.Output == nil {
		stepResult.Output = make(map[string]any)
	}
	stepResult.Output["api_calls"] = host.apiCalls
	stepResult.Output["console"] = host.console
	if host.consoleCut {
		stepResult.Output["console_truncated"] = true
	}

	if runErr != nil {
		return runErr
	}

	ctx.Variables[outputVar] = output
	stepResult.Output[outputVar] = output
	stepResult.Output["result"] = output
	return nil
}

// runWorker starts one isolated worker, sends the run request, and serves the
// bridge until the worker reports a result or failure. The worker owns the
// Sobek realm; this function owns credentials, budgets, and policy.
func (e *ExecuteScriptExecutor) runWorker(ctx context.Context, request scriptWorkerMessage, host *scriptHost) (any, error) {
	launcher := e.launcher
	if launcher == nil {
		launcher = defaultScriptWorkerLauncher
	}
	proc, err := launcher.Launch(ctx)
	if err != nil {
		return nil, fmt.Errorf("execute_script: %w", err)
	}
	defer func() {
		_ = proc.Kill()
		_ = proc.Wait()
	}()

	encoder := json.NewEncoder(proc.Stdin())
	reader := bufio.NewReader(proc.Stdout())
	if err := encoder.Encode(request); err != nil {
		return nil, fmt.Errorf("execute_script: send script to worker: %w", err)
	}

	var memoryExceeded atomic.Bool
	watchdogStop := make(chan struct{})
	defer close(watchdogStop)
	var watchdogOnce sync.Once

	for {
		frame, err := readScriptWorkerFrame(reader, scriptWorkerMaxFrameBytes)
		if err != nil {
			if errors.Is(err, errScriptWorkerFrameTooLarge) {
				return nil, errors.New("execute_script: script worker frame exceeds wire budget")
			}
			if memoryExceeded.Load() {
				return nil, memoryLimitError(request.MemoryBytes)
			}
			if host.fatal != nil {
				return nil, host.fatal
			}
			if ctx.Err() != nil {
				if errors.Is(ctx.Err(), context.DeadlineExceeded) {
					return nil, fmt.Errorf("execute_script: timed out after %dms", request.TimeoutMS)
				}
				return nil, ctx.Err()
			}
			return nil, errors.New("execute_script: script worker exited without a result")
		}
		var msg scriptWorkerMessage
		if err := json.Unmarshal(frame, &msg); err != nil {
			return nil, errors.New("execute_script: script worker sent an invalid frame")
		}
		switch msg.Type {
		case scriptMessageReady:
			if request.MemoryBytes > 0 {
				if rss, rssErr := proc.RSSBytes(); rssErr == nil && rss > 0 {
					watchdogOnce.Do(func() {
						go watchWorkerMemory(proc, rss+request.MemoryBytes+scriptWorkerRSSOverhead, watchdogStop, &memoryExceeded)
					})
				}
			}
		case scriptMessageConsole:
			host.appendConsole(msg.Level + ": " + msg.Message)
		case scriptMessageAPIRequest:
			if err := encoder.Encode(host.handleAPIRequest(msg)); err != nil {
				return nil, fmt.Errorf("execute_script: reply to worker: %w", err)
			}
		case scriptMessageResult:
			var output any
			if len(msg.Output) > 0 {
				if err := json.Unmarshal(msg.Output, &output); err != nil {
					return nil, fmt.Errorf("execute_script: decode worker result: %w", err)
				}
			}
			return output, nil
		case scriptMessageError:
			return nil, host.workerError(msg, request.TimeoutMS, request.MemoryBytes)
		}
	}
}

// workerError maps a worker-reported failure to the node's terminal error. A
// policy failure returns the parent's stored fatal error so callers still see
// the concrete *ErrScriptAPIPolicy; anything else is bounded worker text.
func (h *scriptHost) workerError(msg scriptWorkerMessage, timeoutMS int, memoryBytes int64) error {
	switch msg.ErrorKind {
	case scriptErrorTimeout:
		return fmt.Errorf("execute_script: timed out after %dms", timeoutMS)
	case scriptErrorMemory:
		return memoryLimitError(memoryBytes)
	case scriptErrorPolicy:
		if h.fatal != nil {
			return h.fatal
		}
		return scriptAPIPolicyErrorf("%s", msg.Message)
	default:
		return fmt.Errorf("execute_script: %s", msg.Message)
	}
}

// scriptConcurrencyLimit resolves the instance's script admission ceiling. An
// unset operator limit falls back to a finite default: the per-worker heap
// budget does not bound aggregate CPU or memory, so an unset limit must not
// mean unbounded process spawning.
func (e *ExecuteScriptExecutor) scriptConcurrencyLimit() int {
	if limit := e.controls.Limits().ConcurrentScripts; limit > 0 {
		return limit
	}
	return defaultScriptConcurrency
}

// scriptMemoryBytes resolves the per-script heap budget. Zero means the
// operator left the limit unset, so the default applies.
func (e *ExecuteScriptExecutor) scriptMemoryBytes() int64 {
	memoryMB := defaultScriptMemoryMB
	if limit := e.controls.Limits().ScriptMemoryMB; limit > 0 {
		memoryMB = limit
	}
	return int64(memoryMB) * 1024 * 1024
}

// memoryLimitError is the bounded, explainable failure recorded when a script
// exceeds its heap budget.
func memoryLimitError(memoryBytes int64) error {
	return fmt.Errorf("execute_script: script worker memory limit of %d MiB exceeded", memoryBytes/(1024*1024))
}

// scriptGlobals is the explicit, closed set of values the worker exposes to
// the script. No Go service, pointer, or shared execution map is serialized.
func scriptGlobals(ctx *models.ExecutionContext) map[string]any {
	globals := map[string]any{
		"context":       scriptContextView(ctx),
		"actor_user_id": ctx.EffectiveActorID,
	}
	if ctx.Event != nil {
		globals["item_id"] = ctx.Event.ItemID
		globals["workspace_id"] = ctx.Event.WorkspaceID
		globals["trigger_user_id"] = ctx.Event.ActorUserID
	}
	return globals
}

// scriptContextView is the read-only, detached JSON view of the execution. It
// deliberately excludes the raw user model, item model, and event payload so
// private trigger content cannot leak into the VM.
func scriptContextView(ctx *models.ExecutionContext) map[string]any {
	view := map[string]any{"actor_user_id": ctx.EffectiveActorID}
	if ctx.Event != nil {
		view["event_type"] = string(ctx.Event.EventType)
		view["item_id"] = ctx.Event.ItemID
		view["workspace_id"] = ctx.Event.WorkspaceID
		view["trigger_user_id"] = ctx.Event.ActorUserID
	}
	if ctx.Item != nil {
		view["item"] = map[string]any{
			"id":           ctx.Item.ID,
			"key":          fmt.Sprintf("%s-%d", ctx.Item.WorkspaceKey, ctx.Item.WorkspaceItemNumber),
			"title":        ctx.Item.Title,
			"workspace_id": ctx.Item.WorkspaceID,
			"status_id":    ctx.Item.StatusID,
			"item_type_id": ctx.Item.ItemTypeID,
			"assignee_id":  ctx.Item.AssigneeID,
			"priority_id":  ctx.Item.PriorityID,
		}
	}
	return view
}

// handleAPIRequest is the parent half of the bridge. It validates the method
// and path, enforces the call budget, lazily mints the executor credential, and
// dispatches through the canonical v2 handler. Policy violations are returned
// as fatal responses, so the worker interrupts the VM and a try/catch cannot
// swallow them.
func (h *scriptHost) handleAPIRequest(req scriptWorkerMessage) scriptWorkerMessage {
	fatal := func(err error) scriptWorkerMessage {
		if h.fatal == nil {
			h.fatal = err
		}
		return scriptWorkerMessage{Type: scriptMessageAPIFatal, ID: req.ID, Message: err.Error()}
	}
	if h.fatal != nil {
		return scriptWorkerMessage{Type: scriptMessageAPIFatal, ID: req.ID, Message: h.fatal.Error()}
	}
	if err := h.runCtx.Err(); err != nil {
		return fatal(err)
	}
	method := strings.TrimSpace(req.Method)
	path := req.Path
	if len(req.Body) > maxScriptRequestBodyBytes {
		return fatal(scriptAPIPolicyErrorf("request body exceeds %d bytes", maxScriptRequestBodyBytes))
	}

	h.apiCalls++
	if h.apiCalls > h.maxAPICalls {
		return fatal(scriptAPIPolicyErrorf("api call budget of %d exceeded", h.maxAPICalls))
	}

	// Validate before minting or dispatching so a disallowed route fails closed
	// without spending a credential or a network round trip.
	if _, err := ValidateScriptAPIRequest(method, path); err != nil {
		return fatal(err)
	}
	if err := h.runCtx.Err(); err != nil {
		return fatal(err)
	}

	// Persist the write-ahead marker and mark the execution non-replayable
	// before a mutating dispatch. A mutating call can commit a side effect even
	// when it returns an error or the worker dies mid-call, so replay must be
	// blocked from this point unless the flow later completes successfully.
	if isMutatingScriptMethod(method) {
		if err := h.markWritePending(); err != nil {
			return fatal(fmt.Errorf("execute_script: persist write marker: %w", err))
		}
		if h.reconciliation != nil {
			*h.reconciliation = true
		}
	}

	cred, err := h.ensureCredential()
	if err != nil {
		return fatal(err)
	}
	if err := h.runCtx.Err(); err != nil {
		return fatal(err)
	}

	response, err := h.dispatcher.Dispatch(h.runCtx, cred.Token, ScriptAPIRequest{Method: method, Path: path, Body: req.Body})
	if err != nil {
		var policy *ErrScriptAPIPolicy
		if errors.As(err, &policy) {
			return fatal(err)
		}
		return scriptWorkerMessage{Type: scriptMessageAPIRecover, ID: req.ID, Message: err.Error()}
	}
	return scriptWorkerMessage{
		Type:      scriptMessageAPIResponse,
		ID:        req.ID,
		Status:    response.Status,
		Truncated: response.Truncated,
		Response:  scriptResponseBody(response.Body),
	}
}

// scriptResponseBody keeps a valid JSON body verbatim and otherwise encodes it
// as a JSON string, so the worker can rebuild the same JS shape the in-process
// bridge produced.
func scriptResponseBody(body []byte) json.RawMessage {
	if len(body) > 0 && json.Valid(body) {
		return json.RawMessage(body)
	}
	encoded, err := json.Marshal(string(body))
	if err != nil {
		return nil
	}
	return encoded
}

// isMutatingScriptMethod reports whether a permitted bridge method changes
// state. DELETE is rejected by the path policy, so it is not listed.
func isMutatingScriptMethod(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		return true
	default:
		return false
	}
}

// ensureCredential mints at most one credential per node execution, lazily so a
// script that makes no API calls pays no token cost.
func (h *scriptHost) ensureCredential() (*ActionScriptCredential, error) {
	h.credOnce.Do(func() {
		h.cred, h.credErr = MintActionScriptCredential(h.runCtx, h.tokens, h.actorUserID)
	})
	return h.cred, h.credErr
}

// markWritePending persists the reconciliation marker once per execution. The
// marker is shared across every script node and iterator pass, so the second
// mutating node in a flow reuses the first node's persisted marker instead of
// re-updating a log that is already reconciliation_required. It is a no-op when
// there is no durable execution log to recover.
func (h *scriptHost) markWritePending() error {
	if h.marker == nil || h.executionLogID <= 0 {
		return nil
	}
	if h.writeMarked != nil && *h.writeMarked {
		return nil
	}
	if err := h.marker.MarkScriptWritePending(h.executionLogID); err != nil {
		return err
	}
	if h.writeMarked != nil {
		*h.writeMarked = true
	}
	return nil
}

func (h *scriptHost) appendConsole(line string) {
	if len(line) > maxScriptConsoleEntryBytes {
		line = line[:maxScriptConsoleEntryBytes] + "…[truncated]"
	}
	if len(h.console) >= maxScriptConsoleEntries || h.consoleBytes+len(line) > maxScriptConsoleBytes {
		h.consoleCut = true
		return
	}
	h.console = append(h.console, line)
	h.consoleBytes += len(line)
}

// scriptValueString renders a console argument as bounded text. Objects are
// JSON-encoded so a log line is useful without exposing Go representations.
func scriptValueString(value sobek.Value) string {
	if value == nil {
		return "undefined"
	}
	exported := value.Export()
	if s, ok := exported.(string); ok {
		return s
	}
	if encoded, err := json.Marshal(exported); err == nil {
		return string(encoded)
	}
	return value.String()
}

// runScriptProgram executes the compiled program and converts its return value
// while the watchdog is still armed. Returned getters are evaluated by sobek's
// export, so conversion is script execution too: it must stay inside the
// timeout budget and inside the credential window.
func runScriptProgram(vm *sobek.Runtime, program *sobek.Program, timeoutCtx context.Context) (output any, err error) {
	done := make(chan struct{})
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		select {
		case <-timeoutCtx.Done():
			vm.Interrupt(errScriptTimeout)
		case <-done:
		}
	}()
	defer func() {
		// Convert panics and runtime errors to inert Go text while the watchdog
		// is still armed: formatting a Sobek exception can invoke a script
		// toString, which is itself script code and must stay under the budget.
		if r := recover(); r != nil {
			err = inertScriptPanic(r)
		}
		close(done)
		<-exited
	}()
	result, runErr := vm.RunProgram(program)
	if runErr != nil {
		return nil, inertScriptError(runErr)
	}
	return strictScriptOutput(result)
}

// maxScriptErrorBytes bounds the inert error text a script can influence.
const maxScriptErrorBytes = 1024

// inertScriptError converts a runtime error to a plain Go error without
// retaining any script-backed value whose later formatting could evaluate
// script code. It must be called while the watchdog is armed.
func inertScriptError(runErr error) (err error) {
	var interrupted *sobek.InterruptedError
	if errors.As(runErr, &interrupted) {
		return interrupted
	}
	defer func() {
		if r := recover(); r != nil {
			if ie, ok := r.(*sobek.InterruptedError); ok {
				err = ie
				return
			}
			err = errors.New("script execution error")
		}
	}()
	return errors.New(boundedScriptText(runErr.Error()))
}

// inertScriptPanic converts a recovered panic value to a plain Go error. It
// must be called while the watchdog is armed.
func inertScriptPanic(recovered any) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if ie, ok := r.(*sobek.InterruptedError); ok {
				err = ie
				return
			}
			err = errors.New("script result conversion failed")
		}
	}()
	switch value := recovered.(type) {
	case *sobek.InterruptedError:
		return value
	case *sobek.Object:
		return errors.New(boundedScriptText(value.String()))
	case error:
		return errors.New(boundedScriptText(value.Error()))
	default:
		return fmt.Errorf("script result conversion failed: %v", value)
	}
}

func boundedScriptText(text string) string {
	if len(text) > maxScriptErrorBytes {
		return text[:maxScriptErrorBytes] + "…[truncated]"
	}
	return text
}

// strictScriptOutput converts a script's return value into detached JSON,
// rejecting promises, functions, and other unsupported values. Depth and size
// are bounded so a hostile result cannot exhaust memory or blow the trace.
func strictScriptOutput(result sobek.Value) (any, error) {
	if scriptValueMissing(result) {
		return nil, nil
	}
	exported := result.Export()
	if _, ok := exported.(*sobek.Promise); ok {
		return nil, errors.New("result is a Promise; asynchronous scripts are not supported")
	}
	safe, err := toJSONValue(exported, 0)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(safe)
	if err != nil {
		return nil, fmt.Errorf("result is not JSON-serializable: %w", err)
	}
	if len(encoded) > maxScriptResultBytes {
		return nil, fmt.Errorf("result exceeds %d bytes", maxScriptResultBytes)
	}
	var detached any
	if err := json.Unmarshal(encoded, &detached); err != nil {
		return nil, fmt.Errorf("result is not valid JSON: %w", err)
	}
	return detached, nil
}

func toJSONValue(value any, depth int) (any, error) {
	if depth > maxScriptResultDepth {
		return nil, fmt.Errorf("result exceeds maximum depth of %d", maxScriptResultDepth)
	}
	switch v := value.(type) {
	case nil, bool, string, int, int64, float64, json.Number:
		return v, nil
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			converted, err := toJSONValue(item, depth+1)
			if err != nil {
				return nil, err
			}
			out[i] = converted
		}
		return out, nil
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			converted, err := toJSONValue(item, depth+1)
			if err != nil {
				return nil, err
			}
			out[key] = converted
		}
		return out, nil
	default:
		return nil, fmt.Errorf("result contains unsupported value of type %T", value)
	}
}

// scriptValueMissing treats a nil sobek.Value as missing. sobek's Object.Get
// returns a nil interface for an absent property, which the IsUndefined /
// IsNull predicates do not catch.
func scriptValueMissing(value sobek.Value) bool {
	return value == nil || sobek.IsUndefined(value) || sobek.IsNull(value)
}

// scriptObjectGet reads a property and normalizes an absent property to
// undefined so callers never call methods on a nil value.
func scriptObjectGet(obj *sobek.Object, name string) sobek.Value {
	value := obj.Get(name)
	if value == nil {
		return sobek.Undefined()
	}
	return value
}

// validateScriptOutputVariable rejects names that would shadow host bindings or
// that are not simple identifiers.
func validateScriptOutputVariable(name string) error {
	if len(name) > 64 {
		return errors.New("execute_script: output_variable is too long")
	}
	if !isScriptIdentifier(name) {
		return fmt.Errorf("execute_script: output_variable %q is not a valid identifier", name)
	}
	if actioncatalog.ReservedScriptGlobals[name] {
		return fmt.Errorf("execute_script: output_variable %q is reserved", name)
	}
	return nil
}

func isScriptIdentifier(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

// ValidateScriptSourceSyntax compiles a function-body script without running
// it. Save-time validation uses this so a stored definition never contains
// code that cannot be compiled.
func ValidateScriptSourceSyntax(source string) error {
	if strings.TrimSpace(source) == "" {
		return errors.New("source is required")
	}
	if len(source) > maxScriptLength {
		return fmt.Errorf("source exceeds %d bytes", maxScriptLength)
	}
	if _, err := sobek.Compile("action_script", "(function() {\n"+source+"\n})()", false); err != nil {
		return err
	}
	return nil
}
