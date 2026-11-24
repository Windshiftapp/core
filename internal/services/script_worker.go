package services

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/grafana/sobek"
)

// EnvScriptWorker selects the isolated script-worker mode. The server binary
// and test binaries check it before any other initialization and then run
// RunScriptWorker instead of the normal entry point.
const EnvScriptWorker = "WINDSHIFT_SCRIPT_WORKER"

// Budgets for the worker process. The heap budget is the operator-controlled
// per-script ceiling; the kill grace lets a worker report its own timeout or
// memory failure before the parent force-kills it.
const (
	defaultScriptMemoryMB = 128
	// defaultScriptConcurrency is the action-script admission ceiling when the
	// operator leaves concurrent_scripts unset. Each execution is a process, so
	// an unset limit must not mean unbounded spawning.
	defaultScriptConcurrency = 4
	scriptMemoryPollInterval = 25 * time.Millisecond
	scriptWorkerKillGrace    = 2 * time.Second
	// scriptWorkerRSSOverhead is the non-heap headroom the parent allows on top
	// of the heap budget before the RSS backstop kills a worker, so the worker's
	// own heap monitor normally fires first.
	scriptWorkerRSSOverhead = 32 << 20
	// scriptWorkerMaxFrameBytes bounds one newline-delimited JSON frame on the
	// worker pipe. The parent rejects an over-budget frame before decoding it, so
	// a script cannot make the API process allocate on script-controlled input.
	scriptWorkerMaxFrameBytes = 1 << 20
)

// errScriptWorkerFrameTooLarge marks a protocol frame that exceeded the wire
// budget. The parent fails the node instead of decoding it.
var errScriptWorkerFrameTooLarge = errors.New("script worker frame exceeds wire budget")

// readScriptWorkerFrame reads one newline-terminated JSON frame, failing once
// the accumulated bytes pass max. It never holds more than max+1 bytes of the
// peer's output, so a hostile frame is rejected before json.Unmarshal runs.
func readScriptWorkerFrame(reader *bufio.Reader, limit int) ([]byte, error) {
	frame := make([]byte, 0, 512)
	for {
		chunk, err := reader.ReadSlice('\n')
		if len(frame)+len(chunk) > limit {
			return nil, errScriptWorkerFrameTooLarge
		}
		frame = append(frame, chunk...)
		switch {
		case err == nil:
			return frame, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		default:
			return frame, err
		}
	}
}

// Worker protocol message kinds.
const (
	scriptMessageRun         = "run"
	scriptMessageReady       = "ready"
	scriptMessageConsole     = "console"
	scriptMessageAPIRequest  = "api_request"
	scriptMessageAPIResponse = "api_response"
	scriptMessageAPIFatal    = "api_fatal"
	scriptMessageAPIRecover  = "api_error"
	scriptMessageResult      = "result"
	scriptMessageError       = "error"
)

// Worker failure kinds.
const (
	scriptErrorTimeout = "timeout"
	scriptErrorMemory  = "memory"
	scriptErrorPolicy  = "policy"
	scriptErrorRuntime = "runtime"
)

// errScriptMemory is the interrupt value the in-worker memory monitor raises.
// Like errScriptTimeout it is a bare string so the parent can classify it
// without importing worker internals.
const errScriptMemory = "script memory limit exceeded"

// scriptWorkerMessage is the single wire shape for every worker message. The
// Type field selects which of the remaining fields are populated; unused
// fields are omitted so the encoding stays small and forward compatible.
type scriptWorkerMessage struct {
	Type string `json:"type"`

	// run
	Source           string         `json:"source,omitempty"`
	TimeoutMS        int            `json:"timeout_ms,omitempty"`
	DeadlineUnixNano int64          `json:"deadline_unix_nano,omitempty"`
	MemoryBytes      int64          `json:"memory_bytes,omitempty"`
	Globals          map[string]any `json:"globals,omitempty"`

	// console
	Level   string `json:"level,omitempty"`
	Message string `json:"message,omitempty"`

	// api_request / api_response
	ID        int             `json:"id,omitempty"`
	Method    string          `json:"method,omitempty"`
	Path      string          `json:"path,omitempty"`
	Body      json.RawMessage `json:"body,omitempty"`
	Status    int             `json:"status,omitempty"`
	Response  json.RawMessage `json:"response,omitempty"`
	Truncated bool            `json:"truncated,omitempty"`

	// result / error
	Output    json.RawMessage `json:"output,omitempty"`
	ErrorKind string          `json:"error_kind,omitempty"`
}

// RunScriptWorker is the isolated child entry point. It executes exactly one
// script request from in and writes progress and the terminal result to out.
// It never touches the database, configuration, or any other server state.
func RunScriptWorker(in io.Reader, out io.Writer) {
	encoder := json.NewEncoder(out)
	var writeMu sync.Mutex
	send := func(msg scriptWorkerMessage) {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = encoder.Encode(msg)
	}

	reader := bufio.NewReader(in)
	frame, err := readScriptWorkerFrame(reader, scriptWorkerMaxFrameBytes)
	if err != nil {
		send(scriptWorkerMessage{Type: scriptMessageError, ErrorKind: scriptErrorRuntime, Message: "script worker: invalid run request"})
		return
	}
	var request scriptWorkerMessage
	if err := json.Unmarshal(frame, &request); err != nil || request.Type != scriptMessageRun {
		send(scriptWorkerMessage{Type: scriptMessageError, ErrorKind: scriptErrorRuntime, Message: "script worker: invalid run request"})
		return
	}
	if request.MemoryBytes > 0 {
		debug.SetMemoryLimit(request.MemoryBytes)
	}
	send(scriptWorkerMessage{Type: scriptMessageReady})

	output, err := runWorkerScript(reader, send, request)
	if err != nil {
		kind, message := classifyWorkerError(err)
		send(scriptWorkerMessage{Type: scriptMessageError, ErrorKind: kind, Message: message})
		return
	}
	send(scriptWorkerMessage{Type: scriptMessageResult, Output: output})
}

// runWorkerScript builds a fresh Sobek realm, binds the explicit globals, and
// runs the script under the timeout and memory watchdogs. The realm is never
// reused: sobek cannot reset a mutated realm, and a fresh process per
// execution is what keeps one script from observing another's state.
func runWorkerScript(reader *bufio.Reader, send func(scriptWorkerMessage), request scriptWorkerMessage) (json.RawMessage, error) {
	vm := sobek.New()
	for name, value := range request.Globals {
		if err := vm.Set(name, value); err != nil {
			return nil, fmt.Errorf("bind %s: %w", name, err)
		}
	}

	bridge := &workerBridge{vm: vm, reader: reader, send: send}
	if err := vm.Set("windshift", map[string]any{
		"api": map[string]any{"request": bridge.apiRequest},
	}); err != nil {
		return nil, err
	}
	if err := vm.Set("console", map[string]any{
		"log":   bridge.consoleFn("log"),
		"info":  bridge.consoleFn("info"),
		"warn":  bridge.consoleFn("warn"),
		"error": bridge.consoleFn("error"),
		"debug": bridge.consoleFn("debug"),
	}); err != nil {
		return nil, err
	}

	program, err := sobek.Compile("action_script", "(function() {\n"+request.Source+"\n})()", false)
	if err != nil {
		return nil, fmt.Errorf("syntax error: %w", err)
	}

	// One absolute deadline covers setup, JS, and REST calls. The parent sets
	// it before launch, so a worker that spends its budget initializing cannot
	// still run a fresh full timeout afterwards.
	deadline := time.Now().Add(time.Duration(request.TimeoutMS) * time.Millisecond)
	if request.DeadlineUnixNano > 0 {
		deadline = time.Unix(0, request.DeadlineUnixNano)
	}
	timeoutCtx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	// The deadline is absolute and may already have passed if startup consumed
	// the budget. Fail before running so a slow start cannot still execute JS.
	if err := timeoutCtx.Err(); err != nil {
		return nil, err
	}

	stopMonitor := make(chan struct{})
	defer close(stopMonitor)
	if request.MemoryBytes > 0 {
		go monitorWorkerMemory(vm, request.MemoryBytes, stopMonitor)
	}

	result, err := runScriptProgram(vm, program, timeoutCtx)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("result is not JSON-serializable: %w", err)
	}
	return encoded, nil
}

// monitorWorkerMemory interrupts a runaway script once its live heap passes the
// per-script budget. debug.SetMemoryLimit makes the collector work against the
// same ceiling; this monitor is the hard stop that ends the execution.
func monitorWorkerMemory(vm *sobek.Runtime, budget int64, stop <-chan struct{}) {
	ticker := time.NewTicker(scriptMemoryPollInterval)
	defer ticker.Stop()
	var stats runtime.MemStats
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			runtime.ReadMemStats(&stats)
			if int64(stats.HeapAlloc) > budget { //nolint:gosec // G115: heap sizes are far below MaxInt64
				vm.Interrupt(errScriptMemory)
				return
			}
		}
	}
}

// scriptPolicyViolation carries a fatal bridge-policy refusal back to the
// parent as an interrupt value. Interrupts are uncatchable, so a script cannot
// swallow a policy denial.
type scriptPolicyViolation struct{ Reason string }

func (e *scriptPolicyViolation) Error() string { return "script api policy violation: " + e.Reason }

// workerBridge translates the two host calls a script can make into protocol
// messages. Every call is synchronous: the worker blocks until the parent
// answers, so the parent stays the sole authority on credentials, budgets, and
// policy.
type workerBridge struct {
	vm     *sobek.Runtime
	reader *bufio.Reader
	send   func(scriptWorkerMessage)
	nextID int
}

func (b *workerBridge) apiRequest(call sobek.FunctionCall) sobek.Value {
	options := call.Argument(0)
	if scriptValueMissing(options) {
		panic(b.vm.NewTypeError("windshift.api.request(options) requires an options object"))
	}
	obj := options.ToObject(b.vm)
	methodVal := scriptObjectGet(obj, "method")
	pathVal := scriptObjectGet(obj, "path")
	if scriptValueMissing(methodVal) || scriptValueMissing(pathVal) {
		panic(b.vm.NewTypeError("windshift.api.request requires method and path"))
	}
	method := strings.TrimSpace(methodVal.String())
	path := pathVal.String()

	var body json.RawMessage
	if bodyVal := scriptObjectGet(obj, "body"); !scriptValueMissing(bodyVal) {
		encoded, err := json.Marshal(bodyVal.Export())
		if err != nil {
			panic(b.vm.NewTypeError("body must be JSON-serializable: " + err.Error()))
		}
		if len(encoded) > maxScriptRequestBodyBytes {
			b.vm.Interrupt(&scriptPolicyViolation{Reason: fmt.Sprintf("request body exceeds %d bytes", maxScriptRequestBodyBytes)})
			return sobek.Undefined()
		}
		body = encoded
	}

	b.nextID++
	id := b.nextID
	b.send(scriptWorkerMessage{Type: scriptMessageAPIRequest, ID: id, Method: method, Path: path, Body: body})

	var response scriptWorkerMessage
	frame, err := readScriptWorkerFrame(b.reader, scriptWorkerMaxFrameBytes)
	if err != nil {
		b.vm.Interrupt(fmt.Errorf("script worker: api bridge closed: %w", err))
		return sobek.Undefined()
	}
	if err := json.Unmarshal(frame, &response); err != nil {
		b.vm.Interrupt(errors.New("script worker: invalid api bridge message"))
		return sobek.Undefined()
	}
	if response.ID != id {
		b.vm.Interrupt(errors.New("script worker: api bridge out of sync"))
		return sobek.Undefined()
	}
	switch response.Type {
	case scriptMessageAPIResponse:
		return b.responseValue(response)
	case scriptMessageAPIFatal:
		b.vm.Interrupt(&scriptPolicyViolation{Reason: response.Message})
		return sobek.Undefined()
	case scriptMessageAPIRecover:
		// A transient dispatch failure is catchable, matching the in-process
		// executor: only policy violations are uncatchable.
		panic(b.vm.NewGoError(fmt.Errorf("windshift.api.request failed: %s", response.Message)))
	default:
		b.vm.Interrupt(errors.New("script worker: unexpected api bridge message"))
		return sobek.Undefined()
	}
}

// responseValue mirrors the in-process response shape: status, ok, truncated,
// and a body decoded from JSON when it parses, else the raw text.
func (b *workerBridge) responseValue(response scriptWorkerMessage) sobek.Value {
	result := b.vm.NewObject()
	_ = result.Set("status", response.Status)
	_ = result.Set("ok", response.Status >= 200 && response.Status < 300)
	_ = result.Set("truncated", response.Truncated)
	if len(response.Response) > 0 {
		var decoded any
		if err := json.Unmarshal(response.Response, &decoded); err == nil {
			_ = result.Set("body", decoded)
		} else {
			_ = result.Set("body", string(response.Response))
		}
	} else {
		_ = result.Set("body", "")
	}
	return result
}

// consoleFn forwards one bounded console line to the parent. The parent owns
// the entry and byte budgets; the worker only caps the individual line so a
// single call cannot send an unbounded message.
func (b *workerBridge) consoleFn(level string) func(sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		parts := make([]string, 0, len(call.Arguments))
		for _, arg := range call.Arguments {
			parts = append(parts, scriptValueString(arg))
		}
		line := strings.Join(parts, " ")
		// Cap the wire line one byte over the entry budget so the parent still
		// sees an over-budget line and appends its truncation marker.
		if len(line) > maxScriptConsoleEntryBytes+1 {
			line = line[:maxScriptConsoleEntryBytes+1]
		}
		b.send(scriptWorkerMessage{Type: scriptMessageConsole, Level: level, Message: line})
		return sobek.Undefined()
	}
}

// classifyWorkerError maps a worker-side error to the protocol failure kind and
// a bounded message. It never formats a script-backed value.
func classifyWorkerError(err error) (kind, message string) {
	if errors.Is(err, context.DeadlineExceeded) {
		return scriptErrorTimeout, "script execution timed out"
	}
	var interrupted *sobek.InterruptedError
	if errors.As(err, &interrupted) {
		switch value := interrupted.Value().(type) {
		case string:
			switch value {
			case errScriptTimeout:
				return scriptErrorTimeout, "script execution timed out"
			case errScriptMemory:
				return scriptErrorMemory, "script memory limit exceeded"
			}
		case *scriptPolicyViolation:
			return scriptErrorPolicy, value.Reason
		case error:
			return scriptErrorRuntime, boundedScriptText(value.Error())
		}
	}
	return scriptErrorRuntime, boundedScriptText(err.Error())
}

// ScriptWorkerProcess is one isolated worker process executing one script.
type ScriptWorkerProcess interface {
	Stdin() io.Writer
	Stdout() io.Reader
	RSSBytes() (int64, error)
	Kill() error
	Wait() error
}

// ScriptWorkerLauncher starts an isolated worker for a single execution. It is
// an interface so tests can run the worker in-process and so a pooled or
// out-of-process launcher can replace it without touching the executor.
type ScriptWorkerLauncher interface {
	Launch(ctx context.Context) (ScriptWorkerProcess, error)
}

// SubprocessScriptLauncher runs the worker as a child of the current binary,
// selected by EnvScriptWorker. The child shares the binary but not the address
// space, so a runaway allocation is killed by the OS instead of the server.
type SubprocessScriptLauncher struct{}

var defaultScriptWorkerLauncher ScriptWorkerLauncher = SubprocessScriptLauncher{}

func (SubprocessScriptLauncher) Launch(ctx context.Context) (ScriptWorkerProcess, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve worker executable: %w", err)
	}
	cmd := exec.CommandContext(ctx, executable) //nolint:gosec // G204: the worker is this binary
	cmd.Env = append(withoutEnv(os.Environ(), EnvScriptWorker), EnvScriptWorker+"=1")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start script worker: %w", err)
	}
	proc := &subprocessScriptWorker{cmd: cmd, stdin: stdin, stdout: stdout}
	go proc.drainStderr(stderr)
	return proc, nil
}

type subprocessScriptWorker struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
}

func (w *subprocessScriptWorker) Stdin() io.Writer  { return w.stdin }
func (w *subprocessScriptWorker) Stdout() io.Reader { return w.stdout }

func (w *subprocessScriptWorker) RSSBytes() (int64, error) {
	if w.cmd.Process == nil {
		return 0, nil
	}
	return processRSSBytes(w.cmd.Process.Pid)
}

func (w *subprocessScriptWorker) Kill() error {
	if w.cmd.Process == nil {
		return nil
	}
	_ = w.stdin.Close()
	return w.cmd.Process.Kill()
}

func (w *subprocessScriptWorker) Wait() error { return w.cmd.Wait() }

// drainStderr consumes the child's stderr so a chatty worker cannot block on a
// full pipe. Worker diagnostics are deliberately not surfaced to scripts.
func (w *subprocessScriptWorker) drainStderr(stderr io.Reader) {
	_, _ = io.Copy(io.Discard, stderr)
}

// withoutEnv drops any existing assignment for key so the appended value wins.
func withoutEnv(env []string, key string) []string {
	prefix := key + "="
	filtered := env[:0]
	for _, entry := range env {
		if !strings.HasPrefix(entry, prefix) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

// watchWorkerMemory is the parent-side hard backstop: if the child's resident
// set passes its own baseline plus the budget, the parent kills it. The child's
// in-process monitor normally fires first and reports an explainable failure.
func watchWorkerMemory(proc ScriptWorkerProcess, limit int64, stop <-chan struct{}, exceeded *atomic.Bool) {
	ticker := time.NewTicker(scriptMemoryPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			rss, err := proc.RSSBytes()
			if err != nil || rss <= 0 {
				continue
			}
			if rss > limit {
				exceeded.Store(true)
				_ = proc.Kill()
				return
			}
		}
	}
}
