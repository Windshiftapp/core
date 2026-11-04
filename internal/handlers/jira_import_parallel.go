package handlers

import (
	"context"
	"sync"
)

// jiraImportDefaultConcurrency bounds how many independent Jira requests an
// import keeps in flight. The client's shared rate limiter is the real cap
// (10 requests/second by default); this only has to be large enough to keep
// that budget busy without queueing an unbounded number of requests.
const jiraImportDefaultConcurrency = 8

// jiraImportMaxConcurrency caps the operator-configurable pool so a
// misconfiguration cannot spawn an unbounded number of goroutines.
const jiraImportMaxConcurrency = 32

// jiraParallel runs fn for every index in [0, n) with at most workers
// goroutines. It preserves no ordering: callers must write to distinct indices
// and keep shared state off the concurrent path. It stops launching work once
// ctx is done and waits for in-flight work before returning.
func jiraParallel(ctx context.Context, workers, n int, fn func(ctx context.Context, i int)) {
	if n <= 0 {
		return
	}
	if workers < 1 {
		workers = 1
	}
	if workers > n {
		workers = n
	}
	if workers == 1 {
		for i := 0; i < n; i++ {
			if ctx.Err() != nil {
				return
			}
			fn(ctx, i)
		}
		return
	}

	slots := make(chan struct{}, workers)
	var group sync.WaitGroup
	for i := 0; i < n; i++ {
		if ctx.Err() != nil {
			break
		}
		slots <- struct{}{}
		group.Add(1)
		go func(i int) {
			defer group.Done()
			defer func() { <-slots }()
			fn(ctx, i)
		}(i)
	}
	group.Wait()
}
