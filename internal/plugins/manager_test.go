//go:build !noplugins

package plugins

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

// TestPluginNameContext_Isolation confirms that plugin names threaded through
// context.Context stay isolated per-goroutine. This is the direct contract that
// replaced the Manager.currentPluginName shared field, which was susceptible to
// cross-plugin KV leaks under concurrent requests.
func TestPluginNameContext_Isolation(t *testing.T) {
	const workers = 64
	const iters = 200

	var wg sync.WaitGroup
	errs := make(chan error, workers*iters)

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				name := fmt.Sprintf("plugin-%d-%d", worker, i)
				ctx := withPluginName(context.Background(), name)

				// Under the old shared-state design a concurrent goroutine
				// could mutate currentPluginName between set and read. With
				// context-threading each goroutine sees exactly what it set.
				got := pluginNameFromContext(ctx)
				if got != name {
					errs <- fmt.Errorf("worker %d iter %d: got %q, want %q", worker, i, got, name)
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

func TestPluginNameContext_EmptyWhenAbsent(t *testing.T) {
	if got := pluginNameFromContext(context.Background()); got != "" {
		t.Errorf("pluginNameFromContext on empty ctx: got %q, want \"\"", got)
	}
}
