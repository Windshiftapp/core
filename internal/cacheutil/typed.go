package cacheutil

import (
	"sync"
	"sync/atomic"
	"time"
)

// TypedCacheOptions configures a TypedCache.
type TypedCacheOptions struct {
	TTL           time.Duration // Per-entry time-to-live. Required.
	MaxEntries    int           // Total entry bound; <= 0 means unbounded.
	Shards        int           // Number of shards (default: 32).
	SweepInterval time.Duration // Expired-entry sweep cadence (default: 5m).
}

type typedEntry[V any] struct {
	value     V
	expiresAt time.Time
}

type typedShard[V any] struct {
	mu      sync.RWMutex
	entries map[int]typedEntry[V]
}

// TypedCache is a sharded, count-bounded in-memory cache for pointer-shaped
// values. It never serializes: Get returns the stored value and callers must
// treat it as immutable. Expiry is lazy on read and reclaimed by a
// low-frequency background sweep.
type TypedCache[V any] struct {
	ttl         time.Duration
	perShardMax int
	shards      []typedShard[V]

	entries   atomic.Int64
	hits      atomic.Uint64
	misses    atomic.Uint64
	evictions atomic.Uint64

	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
}

// NewTyped creates and registers a typed cache for process diagnostics. A
// non-positive MaxEntries leaves the cache unbounded; production callers always
// pass a bound.
func NewTyped[V any](name string, opts TypedCacheOptions) *TypedCache[V] {
	shards := opts.Shards
	if shards <= 0 {
		shards = 32
	}
	perShardMax := 0
	if opts.MaxEntries > 0 {
		perShardMax = (opts.MaxEntries + shards - 1) / shards
	}
	sweepInterval := opts.SweepInterval
	if sweepInterval <= 0 {
		sweepInterval = 5 * time.Minute
	}

	cache := &TypedCache[V]{
		ttl:         opts.TTL,
		perShardMax: perShardMax,
		shards:      make([]typedShard[V], shards),
		stop:        make(chan struct{}),
		done:        make(chan struct{}),
	}
	for i := range cache.shards {
		cache.shards[i].entries = make(map[int]typedEntry[V])
	}
	registerCache(name, cache)
	go cache.runSweep(sweepInterval)
	return cache
}

func (c *TypedCache[V]) shardFor(key int) *typedShard[V] {
	index := key % len(c.shards)
	if index < 0 {
		index += len(c.shards)
	}
	return &c.shards[index]
}

// Get returns the cached value for key. Expired entries are treated as a miss
// and removed.
func (c *TypedCache[V]) Get(key int) (V, bool) {
	shard := c.shardFor(key)
	shard.mu.RLock()
	entry, ok := shard.entries[key]
	shard.mu.RUnlock()
	if !ok {
		c.misses.Add(1)
		var zero V
		return zero, false
	}
	if time.Now().After(entry.expiresAt) {
		shard.mu.Lock()
		if current, ok := shard.entries[key]; ok && time.Now().After(current.expiresAt) {
			delete(shard.entries, key)
			c.entries.Add(-1)
		}
		shard.mu.Unlock()
		c.misses.Add(1)
		var zero V
		return zero, false
	}
	c.hits.Add(1)
	return entry.value, true
}

// Set stores value under key with the cache TTL, evicting from the shard when
// it is at capacity.
func (c *TypedCache[V]) Set(key int, value V) {
	shard := c.shardFor(key)
	now := time.Now()
	shard.mu.Lock()
	_, exists := shard.entries[key]
	if !exists && c.perShardMax > 0 && len(shard.entries) >= c.perShardMax {
		c.dropExpiredLocked(shard, now)
		if len(shard.entries) >= c.perShardMax {
			c.evictSoonestLocked(shard)
		}
	}
	if _, exists := shard.entries[key]; !exists {
		c.entries.Add(1)
	}
	shard.entries[key] = typedEntry[V]{value: value, expiresAt: now.Add(c.ttl)}
	shard.mu.Unlock()
}

// Delete removes key and reports whether it was present.
func (c *TypedCache[V]) Delete(key int) bool {
	shard := c.shardFor(key)
	shard.mu.Lock()
	_, ok := shard.entries[key]
	if ok {
		delete(shard.entries, key)
		c.entries.Add(-1)
	}
	shard.mu.Unlock()
	return ok
}

// Reset removes every entry.
func (c *TypedCache[V]) Reset() {
	for i := range c.shards {
		c.shards[i].mu.Lock()
		c.shards[i].entries = make(map[int]typedEntry[V])
		c.shards[i].mu.Unlock()
	}
	c.entries.Store(0)
}

// Len returns the current entry count, including not-yet-swept expired entries.
func (c *TypedCache[V]) Len() int {
	return int(c.entries.Load())
}

// Close stops the background sweep. It is safe to call more than once.
func (c *TypedCache[V]) Close() error {
	c.closeOnce.Do(func() {
		close(c.stop)
		<-c.done
	})
	return nil
}

func (c *TypedCache[V]) dropExpiredLocked(shard *typedShard[V], now time.Time) {
	for key, entry := range shard.entries {
		if now.After(entry.expiresAt) {
			delete(shard.entries, key)
			c.entries.Add(-1)
		}
	}
}

func (c *TypedCache[V]) evictSoonestLocked(shard *typedShard[V]) {
	var victim int
	var earliest time.Time
	first := true
	for key, entry := range shard.entries {
		if first || entry.expiresAt.Before(earliest) {
			victim = key
			earliest = entry.expiresAt
			first = false
		}
	}
	if !first {
		delete(shard.entries, victim)
		c.entries.Add(-1)
		c.evictions.Add(1)
	}
}

func (c *TypedCache[V]) runSweep(interval time.Duration) {
	defer close(c.done)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-c.stop:
			return
		case <-ticker.C:
			c.sweep(time.Now())
		}
	}
}

func (c *TypedCache[V]) sweep(now time.Time) {
	for i := range c.shards {
		shard := &c.shards[i]
		shard.mu.Lock()
		c.dropExpiredLocked(shard, now)
		shard.mu.Unlock()
	}
}

func (c *TypedCache[V]) cacheSnapshot(name string) Snapshot {
	return Snapshot{
		Name:             name,
		Entries:          c.Len(),
		Hits:             int64(c.hits.Load()),   //nolint:gosec // G115: counters cannot exceed MaxInt64.
		Misses:           int64(c.misses.Load()), //nolint:gosec // G115: counters cannot exceed MaxInt64.
		NoSpaceEvictions: c.evictions.Load(),
	}
}
