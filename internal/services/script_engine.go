package services

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/grafana/sobek"
)

const (
	defaultScriptTimeoutMs = 1000  // 1 second
	maxScriptLength        = 10240 // 10KB
)

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
				return sobek.New()
			},
		},
	}
}

// Execute runs a JavaScript script with the given variables and returns the result.
// The script is executed in a sandboxed VM with no I/O access.
// vars are set as global variables accessible to the script.
// timeoutMs controls execution timeout (0 = default 1s).
func (e *ScriptEngine) Execute(ctx context.Context, script string, vars map[string]interface{}, timeoutMs int) (sobek.Value, error) {
	if len(script) > maxScriptLength {
		return nil, fmt.Errorf("script exceeds maximum length of %d bytes", maxScriptLength)
	}

	if timeoutMs <= 0 {
		timeoutMs = defaultScriptTimeoutMs
	}

	poolObj := e.pool.Get()
	vm, ok := poolObj.(*sobek.Runtime)
	if !ok {
		return nil, fmt.Errorf("unexpected VM type from pool")
	}
	defer func() {
		// Clear all globals before returning to pool
		for key := range vars {
			_ = vm.GlobalObject().Delete(key)
		}
		e.pool.Put(vm)
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

	// Run interrupt goroutine
	done := make(chan struct{})
	go func() {
		select {
		case <-timeoutCtx.Done():
			vm.Interrupt(errScriptTimeout)
		case <-done:
		}
	}()

	result, err := vm.RunString(script)
	close(done)

	// Clear any pending interrupt
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

	return result, nil
}

// ExecuteBool runs a script and coerces the result to bool.
func (e *ScriptEngine) ExecuteBool(ctx context.Context, script string, vars map[string]interface{}, timeoutMs int) (bool, error) {
	result, err := e.Execute(ctx, script, vars, timeoutMs)
	if err != nil {
		return false, err
	}

	if result == nil || sobek.IsUndefined(result) || sobek.IsNull(result) {
		return false, nil
	}

	return result.ToBoolean(), nil
}

var errScriptTimeout = "script execution timeout"
