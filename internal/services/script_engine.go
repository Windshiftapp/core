package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/grafana/sobek"
)

const (
	defaultScriptTimeoutMs = 1000  // 1 second
	maxScriptLength        = 10240 // 10KB
)

// pooledVM wraps a sobek.Runtime with a snapshot of its baseline global
// property names. Cleanup deletes any globals not in the baseline so scripts
// cannot leak state to the next caller that borrows the same VM.
type pooledVM struct {
	vm          *sobek.Runtime
	baseGlobals map[string]struct{}
}

// ScriptEngine provides sandboxed JavaScript execution via Sobek (Grafana's goja fork).
// It is general-purpose and can be reused for computed fields, validation rules, automations, etc.
type ScriptEngine struct {
	pool sync.Pool
}

// NewScriptEngine creates a new script engine with a VM pool.
func NewScriptEngine() *ScriptEngine {
	return &ScriptEngine{
		pool: sync.Pool{
			New: func() interface{} {
				vm := sobek.New()
				keys := vm.GlobalObject().Keys()
				base := make(map[string]struct{}, len(keys))
				for _, k := range keys {
					base[k] = struct{}{}
				}
				return &pooledVM{vm: vm, baseGlobals: base}
			},
		},
	}
}

// Execute runs a JavaScript script with the given variables and returns the result
// exported to a plain Go value. The script is executed in a sandboxed VM with no I/O access.
// vars are set as global variables accessible to the script.
// timeoutMs controls execution timeout (0 = default 1s).
//
// The exported Go value is safe to use after this function returns; no sobek.Value
// escapes the pool-managed runtime (which is NOT goroutine-safe).
func (e *ScriptEngine) Execute(ctx context.Context, script string, vars map[string]interface{}, timeoutMs int) (any, error) {
	if len(script) > maxScriptLength {
		return nil, fmt.Errorf("script exceeds maximum length of %d bytes", maxScriptLength)
	}

	if timeoutMs <= 0 {
		timeoutMs = defaultScriptTimeoutMs
	}

	poolObj := e.pool.Get()
	pv, ok := poolObj.(*pooledVM)
	if !ok {
		return nil, fmt.Errorf("unexpected VM type from pool")
	}
	vm := pv.vm
	defer func() {
		// Delete any global not in the baseline snapshot — this removes both
		// vars set below and anything the script may have written to globalThis.
		for _, k := range vm.GlobalObject().Keys() {
			if _, base := pv.baseGlobals[k]; !base {
				_ = vm.GlobalObject().Delete(k)
			}
		}
		e.pool.Put(pv)
	}()

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

	// Interrupt goroutine: must be joined before we touch the VM again, otherwise
	// a delayed Interrupt could fire on a VM already returned to the pool and
	// reused by another goroutine.
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
	vm.ClearInterrupt()

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
func (e *ScriptEngine) ExecuteBool(ctx context.Context, script string, vars map[string]interface{}, timeoutMs int) (bool, error) {
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
