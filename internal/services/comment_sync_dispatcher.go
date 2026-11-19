package services

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

const (
	commentSyncQueueSize   = 256
	commentSyncConcurrency = 4
	commentSyncTimeout     = 30 * time.Second
)

// commentSyncDispatcher owns the best-effort outbound GitHub comment mirroring
// queue. Delivery is best-effort: a full queue drops the push (counted), and
// remote failures are logged by the push implementation with no durable retry.
// Replacing the previous one-goroutine-per-mutation design bounds both
// goroutines and in-flight remote work (WI-2118).
type commentSyncDispatcher struct {
	mu      sync.Mutex
	started bool
	closed  bool
	jobs    chan func(context.Context)
	cancel  context.CancelFunc
	wg      sync.WaitGroup

	queued    atomic.Int64
	active    atomic.Int64
	completed atomic.Int64
	dropped   atomic.Int64
	panics    atomic.Int64
}

var globalCommentSyncDispatcher commentSyncDispatcher

func (d *commentSyncDispatcher) worker(ctx context.Context) {
	defer d.wg.Done()
	for push := range d.jobs {
		d.active.Add(1)
		pushCtx, cancel := context.WithTimeout(ctx, commentSyncTimeout)
		func() {
			defer func() {
				if r := recover(); r != nil {
					d.panics.Add(1)
				}
			}()
			push(pushCtx)
		}()
		cancel()
		d.active.Add(-1)
		d.completed.Add(1)
	}
}

// enqueue schedules a best-effort push, starting the worker pool on first use.
// It returns false when the queue is full or the dispatcher has shut down.
func (d *commentSyncDispatcher) enqueue(push func(context.Context)) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		d.dropped.Add(1)
		return false
	}
	if !d.started {
		ctx, cancel := context.WithCancel(context.Background())
		d.cancel = cancel
		d.jobs = make(chan func(context.Context), commentSyncQueueSize)
		d.started = true
		for i := 0; i < commentSyncConcurrency; i++ {
			d.wg.Add(1)
			go d.worker(ctx)
		}
	}
	select {
	case d.jobs <- push:
		d.queued.Add(1)
		return true
	default:
		d.dropped.Add(1)
		return false
	}
}

// shutdown cancels in-flight pushes and drains the queue with a bounded wait.
func (d *commentSyncDispatcher) shutdown(ctx context.Context) {
	d.mu.Lock()
	if !d.started || d.closed {
		d.mu.Unlock()
		return
	}
	d.closed = true
	cancel := d.cancel
	jobs := d.jobs
	d.mu.Unlock()

	cancel()
	close(jobs)
	done := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// ShutdownCommentSync stops the process-local comment mirroring queue. It is
// safe to call when the queue was never started.
func ShutdownCommentSync(ctx context.Context) {
	globalCommentSyncDispatcher.shutdown(ctx)
}

// enqueueCommentSync schedules a best-effort comment mirror push.
func enqueueCommentSync(push func(context.Context)) {
	globalCommentSyncDispatcher.enqueue(push)
}

// CommentSyncStats exposes queue depth, active work, completions, drops, and
// recovered panics for diagnostics.
func CommentSyncStats() map[string]int64 {
	d := &globalCommentSyncDispatcher
	return map[string]int64{
		"queued":    d.queued.Load(),
		"active":    d.active.Load(),
		"completed": d.completed.Load(),
		"dropped":   d.dropped.Load(),
		"panics":    d.panics.Load(),
	}
}
