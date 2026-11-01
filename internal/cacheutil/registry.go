package cacheutil

import (
	"sort"
	"sync"
	"sync/atomic"

	"github.com/allegro/bigcache/v3"
)

type cacheRegistration struct {
	name      string
	cache     *bigcache.BigCache
	maxBytes  int64
	evictions atomic.Uint64
}

func (r *cacheRegistration) cacheSnapshot(name string) Snapshot {
	stats := r.cache.Stats()
	return Snapshot{
		Name:                   name,
		Entries:                r.cache.Len(),
		AllocatedCapacityBytes: int64(r.cache.Capacity()),
		MaximumCapacityBytes:   r.maxBytes,
		Hits:                   stats.Hits,
		Misses:                 stats.Misses,
		NoSpaceEvictions:       r.evictions.Load(),
	}
}

// Snapshot describes one live process-local cache.
type Snapshot struct {
	Name                   string `json:"name"`
	Entries                int    `json:"entries"`
	AllocatedCapacityBytes int64  `json:"allocated_capacity_bytes"`
	MaximumCapacityBytes   int64  `json:"maximum_capacity_bytes"`
	Hits                   int64  `json:"hits"`
	Misses                 int64  `json:"misses"`
	NoSpaceEvictions       uint64 `json:"no_space_evictions"`
}

// snapshotSource is implemented by every registered cache. Unexported so only
// cacheutil types can register.
type snapshotSource interface {
	cacheSnapshot(name string) Snapshot
}

var cacheRegistry = struct {
	sync.RWMutex
	entries map[string]snapshotSource
}{entries: make(map[string]snapshotSource)}

func registerCache(name string, source snapshotSource) {
	cacheRegistry.Lock()
	cacheRegistry.entries[name] = source
	cacheRegistry.Unlock()
}

// Snapshots returns stable, name-sorted diagnostics for registered caches.
func Snapshots() []Snapshot {
	cacheRegistry.RLock()
	sources := make(map[string]snapshotSource, len(cacheRegistry.entries))
	for name, source := range cacheRegistry.entries {
		sources[name] = source
	}
	cacheRegistry.RUnlock()

	snapshots := make([]Snapshot, 0, len(sources))
	for name, source := range sources {
		snapshots = append(snapshots, source.cacheSnapshot(name))
	}
	sort.Slice(snapshots, func(i, j int) bool { return snapshots[i].Name < snapshots[j].Name })
	return snapshots
}
