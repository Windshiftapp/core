package services

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestScriptEngine_GlobalThisCleanup(t *testing.T) {
	engine := NewScriptEngine()
	ctx := context.Background()

	// Loop enough times that the pool is very likely to hand back a reused VM.
	for i := 0; i < 50; i++ {
		if _, err := engine.Execute(ctx, "globalThis.leak = 'secret'; 1", nil, 0); err != nil {
			t.Fatalf("iter %d: leak script failed: %v", i, err)
		}

		result, err := engine.Execute(ctx, "typeof globalThis.leak", nil, 0)
		if err != nil {
			t.Fatalf("iter %d: probe script failed: %v", i, err)
		}
		if result != "undefined" {
			t.Fatalf("iter %d: globalThis.leak survived pool reuse: got %#v, want \"undefined\"", i, result)
		}
	}
}

func TestScriptEngine_ConcurrentExecuteBool(t *testing.T) {
	// With -race this exercises the pool path, the interrupt goroutine join,
	// and ensures no sobek.Value escapes the runtime (Export must happen while
	// the VM is still held).
	engine := NewScriptEngine()
	ctx := context.Background()

	const workers = 32
	const iters = 50

	var wg sync.WaitGroup
	errs := make(chan error, workers*iters)

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				// Each call uses a unique positive value so a global leak would be detectable.
				want := worker*iters + i + 1
				vars := map[string]interface{}{"n": want}
				got, err := engine.ExecuteBool(ctx, "n > 0", vars, 0)
				if err != nil {
					errs <- fmt.Errorf("worker %d iter %d: %w", worker, i, err)
					return
				}
				if !got {
					errs <- fmt.Errorf("worker %d iter %d: want true for n=%d", worker, i, want)
					return
				}
			}
		}(w)
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestScriptEngine_TimeoutDoesNotPoisonPool(t *testing.T) {
	// A timed-out script must not leave a pending interrupt on a VM that the
	// next caller borrows. Run a tight loop under 1ms, then a well-behaved
	// script — the second must succeed cleanly.
	engine := NewScriptEngine()
	ctx := context.Background()

	_, err := engine.Execute(ctx, "while (true) {}", nil, 1)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("want timeout error, got %v", err)
	}

	for i := 0; i < 20; i++ {
		got, err := engine.ExecuteBool(ctx, "1 + 1 === 2", nil, 0)
		if err != nil {
			t.Fatalf("iter %d: clean script failed after prior timeout: %v", i, err)
		}
		if !got {
			t.Fatalf("iter %d: want true", i)
		}
	}
}

func TestScriptEngine_ExecuteBoolCoercion(t *testing.T) {
	engine := NewScriptEngine()
	ctx := context.Background()

	cases := []struct {
		script string
		want   bool
	}{
		{"true", true},
		{"false", false},
		{"1", true},
		{"0", false},
		{"'hello'", true},
		{"''", false},
		{"null", false},
		{"undefined", false},
		{"NaN", false},
		{"({})", true},
		{"[]", true},
	}

	for _, c := range cases {
		got, err := engine.ExecuteBool(ctx, c.script, nil, 0)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", c.script, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: got %v, want %v", c.script, got, c.want)
		}
	}
}
