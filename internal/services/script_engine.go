package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/grafana/sobek"
)

const (
	defaultScriptTimeoutMs = 1000  // 1 second
	maxScriptLength        = 10240 // 10KB
)

// ScriptEngine provides sandboxed JavaScript execution via Sobek (Grafana's goja fork).
// It is general-purpose and can be reused for computed fields, validation rules, automations, etc.
//
// Every execution runs in a fresh sobek runtime. Scripts can mutate built-ins
// and their prototypes, and sobek cannot restore a mutated realm, so reusing a
// runtime would let one script change what later executions observe. Creating a
// runtime costs a couple of microseconds, which is cheap enough that pooling is
// not worth the cross-execution isolation risk.
type ScriptEngine struct{}

// NewScriptEngine creates a new script engine.
func NewScriptEngine() *ScriptEngine {
	return &ScriptEngine{}
}

// Execute runs a JavaScript script with the given variables and returns the result
// exported to a plain Go value. The script is executed in a sandboxed VM with no I/O access.
// vars are set as global variables accessible to the script.
// timeoutMs controls execution timeout (0 = default 1s).
//
// The exported Go value is safe to use after this function returns; no sobek.Value
// escapes the runtime (which is NOT goroutine-safe).
func (e *ScriptEngine) Execute(ctx context.Context, script string, vars map[string]any, timeoutMs int) (any, error) {
	if len(script) > maxScriptLength {
		return nil, fmt.Errorf("script exceeds maximum length of %d bytes", maxScriptLength)
	}

	if timeoutMs <= 0 {
		timeoutMs = defaultScriptTimeoutMs
	}

	// A fresh runtime per call is the only way to guarantee that built-ins and
	// their prototypes are pristine; there is no realm reset in sobek.
	vm := sobek.New()

	// Set variables as globals
	for key, value := range vars {
		if err := vm.Set(key, value); err != nil {
			return nil, fmt.Errorf("failed to set variable %q: %w", key, err)
		}
	}

	// Set up timeout using context and vm.Interrupt
	timeout := time.Duration(timeoutMs) * time.Millisecond
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Interrupt goroutine: join it before returning so no goroutine outlives the
	// call, even though the discarded runtime cannot be reused.
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

	result, err := vm.RunString(script)
	// Only retry on non-interrupt errors. An InterruptedError means the
	// watchdog goroutine has already fired its one-shot Interrupt and exited;
	// retrying would run unprotected, letting a malicious script bypass timeouts.
	if err != nil && !isInterruptedError(err) {
		// Retry wrapped in IIFE — handles top-level return statements
		// and other cases where bare expressions need a function context.
		wrapped := "(function() { " + script + " })()"
		if retryResult, retryErr := vm.RunString(wrapped); retryErr == nil {
			result = retryResult
			err = nil
		}
	}
	close(done)
	<-exited

	if err != nil {
		// Check if it was a timeout
		var exception *sobek.InterruptedError
		if errors.As(err, &exception) {
			if exception.Value() == errScriptTimeout {
				return nil, fmt.Errorf("script execution timed out after %dms", timeoutMs)
			}
		}
		return nil, fmt.Errorf("script execution error: %w", err)
	}

	if result == nil || sobek.IsUndefined(result) || sobek.IsNull(result) {
		return nil, nil
	}

	return result.Export(), nil
}

// ExecuteBool runs a script and coerces the result to bool using JS-like truthiness.
func (e *ScriptEngine) ExecuteBool(ctx context.Context, script string, vars map[string]any, timeoutMs int) (bool, error) {
	result, err := e.Execute(ctx, script, vars, timeoutMs)
	if err != nil {
		return false, err
	}
	return toBool(result), nil
}

// toBool mirrors ECMAScript ToBoolean for the types sobek's Export can produce.
func toBool(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case int64:
		return x != 0
	case float64:
		return x != 0 && !math.IsNaN(x)
	default:
		return true
	}
}

var errScriptTimeout = "script execution timeout"

func isInterruptedError(err error) bool {
	var ie *sobek.InterruptedError
	return errors.As(err, &ie)
}
