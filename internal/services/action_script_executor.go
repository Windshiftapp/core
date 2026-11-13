package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/grafana/sobek"

	"windshift/internal/models"
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

// reservedScriptGlobals are names a script must not shadow: they carry the
// host identity, the read-only context, or the bindings themselves.
var reservedScriptGlobals = map[string]bool{
	"windshift":       true,
	"console":         true,
	"context":         true,
	"item_id":         true,
	"workspace_id":    true,
	"actor_user_id":   true,
	"trigger_user_id": true,
}

// ExecuteScriptExecutor implements the execute_script node type. Each run gets
// a fresh Sobek runtime bound to the effective action executor; REST calls go
// through a short-lived bearer token minted for that executor and revoked in
// cleanup.
//
// Trusted authors only: Sobek runs in-process and has no hard per-script heap
// limit, so time and output budgets do not make a hostile allocation safe.
// Restricting authorship to action managers and requiring action.set_actor for
// executable edits of privileged actions is the current mitigation; process
// isolation would be a prerequisite before accepting untrusted authors.
//
// Side effects are not transactional. A script that mutates state and then
// fails has already committed those writes, matching every other action node.
// Durable replay reuses the first attempt's execution log, but a crash between
// a write and the log completion can replay the script; operators must treat
// partial script side effects as reconciliation-required rather than assume
// whole-script atomicity.
type ExecuteScriptExecutor struct {
	tokens     ActionScriptTokenManager
	dispatcher ScriptAPIDispatcher
	logger     *slog.Logger
}

// NewExecuteScriptExecutor wires the executor with the shared token manager
// and the canonical v2 dispatcher. Both are required; Execute fails closed when
// either is missing.
func NewExecuteScriptExecutor(tokens ActionScriptTokenManager, dispatcher ScriptAPIDispatcher) *ExecuteScriptExecutor {
	return &ExecuteScriptExecutor{
		tokens:     tokens,
		dispatcher: dispatcher,
		logger:     slog.Default(),
	}
}

// NodeType pins the dispatch key.
func (e *ExecuteScriptExecutor) NodeType() models.ActionNodeType {
	return models.ActionNodeExecuteScript
}

// scriptRun holds the mutable state of one node execution. It is created per
// Execute call and never shared across executions, so scripts cannot observe
// each other's state.
type scriptRun struct {
	vm           *sobek.Runtime
	runCtx       context.Context
	tokens       ActionScriptTokenManager
	dispatcher   ScriptAPIDispatcher
	actorUserID  int
	maxAPICalls  int
	apiCalls     int
	console      []string
	consoleBytes int
	consoleCut   bool
	fatal        error
	cred         *ActionScriptCredential
	credOnce     sync.Once
	credErr      error
}

// Execute runs the configured function body exactly once. There is no
// execute-then-retry fallback: a retry after a runtime error could repeat host
// side effects.
func (e *ExecuteScriptExecutor) Execute(node *models.ActionNode, ctx *models.ExecutionContext, stepResult *models.StepResult) error {
	if e == nil || e.tokens == nil {
		return errors.New("execute_script: token manager is not configured")
	}
	if e.dispatcher == nil {
		return errors.New("execute_script: api dispatcher is not configured")
	}
	if ctx == nil {
		return errors.New("execute_script: execution context is required")
	}
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
	if timeoutMs <= 0 || timeoutMs > maxScriptAPITimeoutMs {
		timeoutMs = defaultScriptAPITimeoutMs
	}
	maxCalls := config.MaxAPICalls
	if maxCalls <= 0 || maxCalls > maxScriptAPICalls {
		maxCalls = maxScriptAPICalls
	}

	wrapped := "(function() {\n" + source + "\n})()"
	program, err := sobek.Compile("action_script", wrapped, false)
	if err != nil {
		return fmt.Errorf("execute_script: syntax error: %w", err)
	}

	timeoutCtx, cancel := context.WithTimeout(actionRequestContext(ctx), time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	run := &scriptRun{
		tokens:      e.tokens,
		dispatcher:  e.dispatcher,
		actorUserID: ctx.EffectiveActorID,
		maxAPICalls: maxCalls,
		runCtx:      WithActionScriptProvenance(timeoutCtx, provenanceFromExecution(ctx, node.ID)),
	}

	vm := sobek.New()
	run.vm = vm
	if err := bindScriptGlobals(vm, run, ctx); err != nil {
		return fmt.Errorf("execute_script: bind runtime: %w", err)
	}

	result, runErr := runScriptProgram(vm, program, timeoutCtx)

	if run.cred != nil {
		if revokeErr := RevokeActionScriptCredential(e.tokens, run.cred, ctx.EffectiveActorID); revokeErr != nil {
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
	}

	if run.fatal != nil {
		return run.fatal
	}
	if runErr != nil {
		var interrupted *sobek.InterruptedError
		if errors.As(runErr, &interrupted) && interrupted.Value() == errScriptTimeout {
			return fmt.Errorf("execute_script: timed out after %dms", timeoutMs)
		}
		return fmt.Errorf("execute_script: %w", runErr)
	}

	output, err := strictScriptOutput(result)
	if err != nil {
		return fmt.Errorf("execute_script: %w", err)
	}

	ctx.Variables[outputVar] = output
	if stepResult.Output == nil {
		stepResult.Output = make(map[string]any)
	}
	stepResult.Output[outputVar] = output
	stepResult.Output["result"] = output
	stepResult.Output["api_calls"] = run.apiCalls
	stepResult.Output["console"] = run.console
	if run.consoleCut {
		stepResult.Output["console_truncated"] = true
	}
	return nil
}

// bindScriptGlobals installs the explicit, closed set of bindings a script can
// reach. No Go service, pointer, or shared execution map is exposed.
func bindScriptGlobals(vm *sobek.Runtime, run *scriptRun, ctx *models.ExecutionContext) error {
	bindings := map[string]any{
		"windshift": map[string]any{
			"api": map[string]any{
				"request": run.apiRequest,
			},
		},
		"console": map[string]any{
			"log":   run.consoleFn("log"),
			"info":  run.consoleFn("info"),
			"warn":  run.consoleFn("warn"),
			"error": run.consoleFn("error"),
			"debug": run.consoleFn("debug"),
		},
		"context": scriptContextView(ctx),
	}
	for name, value := range bindings {
		if err := vm.Set(name, value); err != nil {
			return fmt.Errorf("set %s: %w", name, err)
		}
	}
	if ctx.Event != nil {
		if err := vm.Set("item_id", ctx.Event.ItemID); err != nil {
			return err
		}
		if err := vm.Set("workspace_id", ctx.Event.WorkspaceID); err != nil {
			return err
		}
		if err := vm.Set("trigger_user_id", ctx.Event.ActorUserID); err != nil {
			return err
		}
	}
	return vm.Set("actor_user_id", ctx.EffectiveActorID)
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

// apiRequest is the single host call exposed to scripts. It validates the
// method and path, enforces the call budget, lazily mints the executor
// credential, and dispatches through the canonical v2 handler. Policy
// violations are fatal: they interrupt the VM so a try/catch cannot swallow
// them.
func (r *scriptRun) apiRequest(call sobek.FunctionCall) sobek.Value {
	if r.fatal != nil {
		return sobek.Undefined()
	}
	options := call.Argument(0)
	if scriptValueMissing(options) {
		panic(r.vm.NewTypeError("windshift.api.request(options) requires an options object"))
	}
	obj := options.ToObject(r.vm)
	methodVal := scriptObjectGet(obj, "method")
	pathVal := scriptObjectGet(obj, "path")
	if scriptValueMissing(methodVal) || scriptValueMissing(pathVal) {
		panic(r.vm.NewTypeError("windshift.api.request requires method and path"))
	}
	method := strings.TrimSpace(methodVal.String())
	path := pathVal.String()

	var body []byte
	if bodyVal := scriptObjectGet(obj, "body"); !scriptValueMissing(bodyVal) {
		encoded, err := json.Marshal(bodyVal.Export())
		if err != nil {
			panic(r.vm.NewTypeError("body must be JSON-serializable: " + err.Error()))
		}
		if len(encoded) > maxScriptRequestBodyBytes {
			r.fail(scriptAPIPolicyErrorf("request body exceeds %d bytes", maxScriptRequestBodyBytes))
			return sobek.Undefined()
		}
		body = encoded
	}

	r.apiCalls++
	if r.apiCalls > r.maxAPICalls {
		r.fail(scriptAPIPolicyErrorf("api call budget of %d exceeded", r.maxAPICalls))
		return sobek.Undefined()
	}

	// Validate before minting or dispatching so a disallowed route fails closed
	// without spending a credential or a network round trip.
	if _, err := ValidateScriptAPIRequest(method, path); err != nil {
		r.fail(err)
		return sobek.Undefined()
	}

	cred, err := r.ensureCredential()
	if err != nil {
		r.fail(err)
		return sobek.Undefined()
	}

	response, err := r.dispatcher.Dispatch(r.runCtx, cred.Token, ScriptAPIRequest{Method: method, Path: path, Body: body})
	if err != nil {
		var policy *ErrScriptAPIPolicy
		if errors.As(err, &policy) {
			r.fail(err)
			return sobek.Undefined()
		}
		panic(r.vm.NewGoError(fmt.Errorf("windshift.api.request failed: %w", err)))
	}
	return r.responseValue(response)
}

// ensureCredential mints at most one credential per node execution, lazily so a
// script that makes no API calls pays no token cost.
func (r *scriptRun) ensureCredential() (*ActionScriptCredential, error) {
	r.credOnce.Do(func() {
		r.cred, r.credErr = MintActionScriptCredential(r.runCtx, r.tokens, r.actorUserID)
	})
	return r.cred, r.credErr
}

// fail records the first fatal policy error and interrupts the VM. Interrupts
// are uncatchable, so an authorization or budget violation cannot be turned
// into a successful step.
func (r *scriptRun) fail(err error) {
	if r.fatal == nil {
		r.fatal = err
	}
	r.vm.Interrupt(err)
}

// responseValue converts a bounded API response into a detached JS object.
func (r *scriptRun) responseValue(response ScriptAPIResponse) sobek.Value {
	result := r.vm.NewObject()
	_ = result.Set("status", response.Status)
	_ = result.Set("ok", response.Status >= 200 && response.Status < 300)
	_ = result.Set("truncated", response.Truncated)
	var decoded any
	if len(response.Body) > 0 && json.Unmarshal(response.Body, &decoded) == nil {
		_ = result.Set("body", decoded)
	} else {
		_ = result.Set("body", string(response.Body))
	}
	return result
}

// consoleFn returns a bounded console sink. Output is truncated rather than
// dropped silently once the byte budget is exhausted.
func (r *scriptRun) consoleFn(level string) func(sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		parts := make([]string, 0, len(call.Arguments))
		for _, arg := range call.Arguments {
			parts = append(parts, scriptValueString(arg))
		}
		r.appendConsole(level + ": " + strings.Join(parts, " "))
		return sobek.Undefined()
	}
}

func (r *scriptRun) appendConsole(line string) {
	if len(line) > maxScriptConsoleEntryBytes {
		line = line[:maxScriptConsoleEntryBytes] + "…[truncated]"
	}
	if len(r.console) >= maxScriptConsoleEntries || r.consoleBytes+len(line) > maxScriptConsoleBytes {
		r.consoleCut = true
		return
	}
	r.console = append(r.console, line)
	r.consoleBytes += len(line)
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

// runScriptProgram executes the compiled program with a watchdog that
// interrupts a runaway script. The runtime is discarded afterwards, so the
// watchdog goroutine is always joined.
func runScriptProgram(vm *sobek.Runtime, program *sobek.Program, timeoutCtx context.Context) (sobek.Value, error) {
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
	result, err := vm.RunProgram(program)
	close(done)
	<-exited
	return result, err
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
	if reservedScriptGlobals[name] {
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
