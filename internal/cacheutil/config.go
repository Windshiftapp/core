// Package cacheutil holds shared cache helpers usable from any layer.
// Lives below domain/service packages so foundational packages (auth,
// middleware, etc.) can configure caches without depending on services.
package cacheutil

import (
	"context"
	"time"

	"github.com/allegro/bigcache/v3"
)

// BigCacheOptions configures a BigCache instance via NewBigCacheConfig.
type BigCacheOptions struct {
	TTL                time.Duration // LifeWindow — how long entries live
	MaxCacheMB         int           // HardMaxCacheSize in megabytes
	Shards             int           // Number of shards (default: 64)
	MaxEntrySize       int           // Max size per entry in bytes (default: 4096)
	MaxEntriesInWin    int           // MaxEntriesInWindow; overrides InitialCapacityMB
	InitialCapacityMB  int           // Initial queue target (default: 4MB; caches still grow to MaxCacheMB)
	CleanWindow        time.Duration // How often to clean expired entries (default: 5m)
	OnRemoveWithReason func(string, []byte, bigcache.RemoveReason)
}

// NewBigCacheConfig creates a bigcache.Config from the given options,
// filling in sensible defaults for unset fields.
func NewBigCacheConfig(opts BigCacheOptions) bigcache.Config {
	shards := opts.Shards
	if shards == 0 {
		shards = 64
	}
	maxEntrySize := opts.MaxEntrySize
	if maxEntrySize == 0 {
		maxEntrySize = 4096
	}
	maxEntries := opts.MaxEntriesInWin
	if maxEntries == 0 {
		initialCapacityMB := opts.InitialCapacityMB
		if initialCapacityMB == 0 {
			initialCapacityMB = 4
		}
		maxEntries = initialCapacityMB * 1024 * 1024 / maxEntrySize
		if maxEntries < shards*10 {
			maxEntries = shards * 10
		}
	}
	cleanWindow := opts.CleanWindow
	if cleanWindow == 0 {
		cleanWindow = 5 * time.Minute
	}

	return bigcache.Config{
		Shards:             shards,
		LifeWindow:         opts.TTL,
		CleanWindow:        cleanWindow,
		MaxEntriesInWindow: maxEntries,
		MaxEntrySize:       maxEntrySize,
		Verbose:            false,
		HardMaxCacheSize:   opts.MaxCacheMB,
		OnRemove:           nil,
		OnRemoveWithReason: opts.OnRemoveWithReason,
	}
}

// New creates a named BigCache and registers it for process diagnostics.
func New(name string, opts BigCacheOptions) (*bigcache.BigCache, error) {
	registration := &cacheRegistration{name: name, maxBytes: int64(opts.MaxCacheMB) * 1024 * 1024}
	callback := opts.OnRemoveWithReason
	opts.OnRemoveWithReason = func(key string, entry []byte, reason bigcache.RemoveReason) {
		if reason == bigcache.NoSpace {
			registration.evictions.Add(1)
		}
		if callback != nil {
			callback(key, entry, reason)
		}
	}
	cache, err := bigcache.New(context.Background(), NewBigCacheConfig(opts))
	if err != nil {
		return nil, err
	}
	registration.cache = cache
	registerCache(name, registration)
	return cache, nil
}
