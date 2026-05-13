package services

import (
	"testing"
	"time"
)

func cacheLen(s *WorkflowService) int {
	n := 0
	s.initialStatusCache.Range(func(_, _ any) bool {
		n++
		return true
	})
	return n
}

func TestMaybeSweepInitialStatusCacheDropsExpired(t *testing.T) {
	s := &WorkflowService{}
	now := time.Now()
	for i := 0; i < 100; i++ {
		key := initialStatusCacheKey(i, nil)
		s.initialStatusCache.Store(key, &initialStatusCacheEntry{
			expiresAt: now.Add(-time.Second),
		})
	}
	for i := 100; i < 110; i++ {
		key := initialStatusCacheKey(i, nil)
		s.initialStatusCache.Store(key, &initialStatusCacheEntry{
			expiresAt: now.Add(time.Hour),
		})
	}

	s.maybeSweepInitialStatusCache(now)

	if got := cacheLen(s); got != 10 {
		t.Fatalf("after sweep, want 10 live entries, got %d", got)
	}
}

func TestMaybeSweepInitialStatusCacheThrottles(t *testing.T) {
	s := &WorkflowService{}
	now := time.Now()
	for i := 0; i < 5; i++ {
		key := initialStatusCacheKey(i, nil)
		s.initialStatusCache.Store(key, &initialStatusCacheEntry{
			expiresAt: now.Add(-time.Second),
		})
	}

	s.maybeSweepInitialStatusCache(now)
	if got := cacheLen(s); got != 0 {
		t.Fatalf("first sweep should evict, got %d", got)
	}

	for i := 5; i < 10; i++ {
		key := initialStatusCacheKey(i, nil)
		s.initialStatusCache.Store(key, &initialStatusCacheEntry{
			expiresAt: now.Add(-time.Second),
		})
	}

	s.maybeSweepInitialStatusCache(now.Add(time.Second))
	if got := cacheLen(s); got != 5 {
		t.Fatalf("second sweep within throttle window should be a no-op, got %d", got)
	}

	s.maybeSweepInitialStatusCache(now.Add(initialStatusSweepInterval + time.Second))
	if got := cacheLen(s); got != 0 {
		t.Fatalf("third sweep past throttle window should evict, got %d", got)
	}
}

func TestSlugifyStatusName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"In Review", "in-review"},
		{"IN REVIEW", "in-review"},
		{"in-review", "in-review"},
		{"in--review", "in-review"},
		{"  in review  ", "in-review"},
		{"start review / ready", "start-review-ready"},
		{"#close", "close"},
		{"", ""},
		{"   ", ""},
		{"Done!", "done"},
		{"v1.0", "v1-0"},
	}
	for _, c := range cases {
		if got := slugifyStatusName(c.in); got != c.want {
			t.Errorf("slugifyStatusName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
