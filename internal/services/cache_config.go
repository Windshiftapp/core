package services

import (
	"time"

	"github.com/allegro/bigcache/v3"
)

// BigCacheOptions configures a BigCache instance via NewBigCacheConfig.
type BigCacheOptions struct {
	TTL             time.Duration // LifeWindow — how long entries live
	MaxCacheMB      int           // HardMaxCacheSize in megabytes
	Shards          int           // Number of shards (default: 1024)
	MaxEntrySize    int           // Max size per entry in bytes (default: 4096)
	MaxEntriesInWin int           // MaxEntriesInWindow (default: 600000)
	CleanWindow     time.Duration // How often to clean expired entries (default: 5m)
}

// NewBigCacheConfig creates a bigcache.Config from the given options,
// filling in sensible defaults for unset fields.
func NewBigCacheConfig(opts BigCacheOptions) bigcache.Config {
	shards := opts.Shards
	if shards == 0 {
		shards = 1024
	}
	maxEntrySize := opts.MaxEntrySize
	if maxEntrySize == 0 {
		maxEntrySize = 4096
	}
	maxEntries := opts.MaxEntriesInWin
	if maxEntries == 0 {
		maxEntries = 1000 * 10 * 60 // 600,000
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
	}
}
