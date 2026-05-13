package middleware

import (
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func mapLen(rl *RateLimiter) int {
	n := 0
	rl.limiters.Range(func(_, _ any) bool {
		n++
		return true
	})
	return n
}

func TestRateLimiterEvictIdleDropsStaleBuckets(t *testing.T) {
	rl := NewRateLimiter(100)
	defer rl.Stop()

	for i := 0; i < 200; i++ {
		bucket := rl.getBucket("tok-" + strconv.Itoa(i))
		bucket.allow(rl.rate, rl.window)
	}
	if got := mapLen(rl); got != 200 {
		t.Fatalf("after seeding, want 200 entries, got %d", got)
	}

	rl.evictIdle(time.Now().Add(idleEvictAfter + time.Second))

	if got := mapLen(rl); got != 0 {
		t.Fatalf("after evictIdle with all buckets idle, want 0, got %d", got)
	}
}

func TestRateLimiterEvictIdleKeepsActiveBuckets(t *testing.T) {
	rl := NewRateLimiter(100)
	defer rl.Stop()

	for i := 0; i < 50; i++ {
		bucket := rl.getBucket("active-" + strconv.Itoa(i))
		bucket.allow(rl.rate, rl.window)
	}
	rl.evictIdle(time.Now())

	if got := mapLen(rl); got != 50 {
		t.Fatalf("active buckets should not be evicted, want 50, got %d", got)
	}
}

func TestRateLimiterStopIsIdempotent(t *testing.T) {
	rl := NewRateLimiter(10)
	rl.Stop()
	rl.Stop()
}

// Ensure the sweep goroutine actually wakes up and prunes — relies on a
// shortened ticker via a private hook test (so we don't sit on real sweep
// intervals). We adjust the bucket's lastSeen and call evictIdle directly,
// then verify a subsequent getBucket sees a fresh entry.
func TestRateLimiterSweepRecreatesAfterEviction(t *testing.T) {
	rl := NewRateLimiter(10)
	defer rl.Stop()

	bucket := rl.getBucket("tok-A")
	bucket.allow(rl.rate, rl.window)

	bucket.mu.Lock()
	bucket.lastSeen = time.Now().Add(-2 * idleEvictAfter)
	bucket.mu.Unlock()

	rl.evictIdle(time.Now())
	if got := mapLen(rl); got != 0 {
		t.Fatalf("want bucket evicted, got %d entries", got)
	}

	freshBucket := rl.getBucket("tok-A")
	if freshBucket == bucket {
		t.Fatal("getBucket returned the old (evicted) bucket pointer")
	}
}

// Sanity: getBucket under concurrent load with lots of distinct tokens does
// not race after eviction. Just checks the eviction path is safe to run while
// new buckets are being created.
func TestRateLimiterEvictIdleConcurrentSafe(t *testing.T) {
	rl := NewRateLimiter(100)
	defer rl.Stop()

	var stop atomic.Bool
	done := make(chan struct{})
	go func() {
		i := 0
		for !stop.Load() {
			b := rl.getBucket("tok-" + strconv.Itoa(i))
			b.allow(rl.rate, rl.window)
			i++
		}
		close(done)
	}()

	for i := 0; i < 20; i++ {
		rl.evictIdle(time.Now().Add(idleEvictAfter + time.Second))
	}
	stop.Store(true)
	<-done
}
