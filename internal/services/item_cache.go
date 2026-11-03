package services

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"windshift/internal/cacheutil"
	"windshift/internal/database"
	"windshift/internal/repository"
)

// effectiveProjectCacheEntry is the cached effective-project resolution for one
// item. The cache returns it by value; EffectiveProjectID points at an
// immutable int, so callers never need to copy before reading.
type effectiveProjectCacheEntry struct {
	EffectiveProjectID     *int
	ProjectInheritanceMode string // "direct" | "inherit" | "none"
}

// ItemCacheService caches the resolved effective project for an item so a read
// does not re-walk the parent chain. The store is a count-bounded typed cache
// rather than BigCache: cardinality is items and the value is a small struct, so
// the byte API only added marshal/copy overhead (see WI-1906 for the same call
// on the permission snapshot cache).
type ItemCacheService struct {
	cache     *cacheutil.TypedCache[effectiveProjectCacheEntry]
	db        database.Database
	repo      *repository.ItemRepository
	closeOnce sync.Once
	closeErr  error

	// Cache statistics, reported by GetStats.
	hierarchyHits   int64
	hierarchyMisses int64
	errors          int64

	config ItemCacheConfig
}

// Close stops the cache's background sweep without closing the shared database.
// It may be called more than once.
func (s *ItemCacheService) Close() error {
	s.closeOnce.Do(func() {
		if s.cache != nil {
			s.closeErr = s.cache.Close()
		}
	})
	return s.closeErr
}

// ItemCacheConfig represents configuration for the item cache.
type ItemCacheConfig struct {
	HierarchyTTL    time.Duration `json:"hierarchy_ttl"`     // Default: 30min
	MaxCacheSize    int           `json:"max_cache_size"`    // Legacy MiB budget; seeds MaxEntries when unset
	MaxEntries      int           `json:"max_entries"`       // Entry bound; 0 derives from MaxCacheSize
	WarmupBatchSize int           `json:"warmup_batch_size"` // Default: 500
	EnablePreWarm   bool          `json:"enable_pre_warm"`   // Default: true
}

// itemEffectiveProjectEstimateBytes is the rough working-set cost of one cached
// entry, used to translate the legacy MiB budget into an entry bound.
const itemEffectiveProjectEstimateBytes = 96

// DefaultItemCacheConfig returns default configuration.
func DefaultItemCacheConfig() ItemCacheConfig {
	return ItemCacheConfig{
		// The measured workload revisits a deterministic hot set throughout a
		// qualification window longer than five minutes. Retain those entries;
		// mutation paths explicitly invalidate affected items and descendants.
		HierarchyTTL:    30 * time.Minute,
		MaxCacheSize:    196,
		WarmupBatchSize: 500,
		EnablePreWarm:   true,
	}
}

// itemCacheMaxEntries resolves the entry bound, preferring an explicit
// MaxEntries and otherwise translating the legacy MiB budget.
func itemCacheMaxEntries(config ItemCacheConfig) int {
	if config.MaxEntries > 0 {
		return config.MaxEntries
	}
	if config.MaxCacheSize <= 0 {
		return 0
	}
	return config.MaxCacheSize * 1024 * 1024 / itemEffectiveProjectEstimateBytes
}

// NewItemCacheService creates a new item cache service.
func NewItemCacheService(db database.Database, config ItemCacheConfig) *ItemCacheService {
	service := &ItemCacheService{
		cache: cacheutil.NewTyped[effectiveProjectCacheEntry]("item_effective_project", cacheutil.TypedCacheOptions{
			TTL:        config.HierarchyTTL,
			MaxEntries: itemCacheMaxEntries(config),
			Shards:     64,
		}),
		db:     db,
		repo:   repository.NewItemRepository(db),
		config: config,
	}

	// Warm up the cache in the background so startup is not blocked. The fill is
	// bounded by WarmupBatchSize; on-demand reads cover everything else.
	if config.EnablePreWarm {
		go func() { _ = service.WarmCache() }()
	}

	return service
}

// GetEffectiveProjectForItem resolves the effective project for an item, serving
// a cached resolution when one is fresh and caching a fresh one otherwise.
// Returns: (effective_project_id, inheritance_mode, error) where mode is
// "none" / "inherit" / "direct".
func (s *ItemCacheService) GetEffectiveProjectForItem(itemID int) (effectiveProjectID *int, projectInheritanceMode string, err error) {
	if entry, ok := s.cache.Get(itemID); ok {
		atomic.AddInt64(&s.hierarchyHits, 1)
		return entry.EffectiveProjectID, entry.ProjectInheritanceMode, nil
	}
	atomic.AddInt64(&s.hierarchyMisses, 1)

	res, err := s.repo.ResolveEffectiveProject(itemID)
	if err != nil {
		atomic.AddInt64(&s.errors, 1)
		return nil, "", err
	}

	// A populated entry always carries the resolved mode (including "none"), so
	// the mode alone gates a hit and "none" items cache instead of recomputing.
	mode := "direct"
	switch {
	case res.DirectProjectID == nil && !res.InheritProject:
		mode = "none"
	case res.InheritProject:
		mode = "inherit"
	}
	s.cache.Set(itemID, effectiveProjectCacheEntry{
		EffectiveProjectID:     res.EffectiveProjectID,
		ProjectInheritanceMode: mode,
	})
	return res.EffectiveProjectID, mode, nil
}

// InvalidateItemHierarchy drops the item and any ids the caller names (ancestors
// whose descendant set changed, or descendants whose inherited project changed).
// Mutation paths call this after a committed project, inherit_project or
// parent_id change.
func (s *ItemCacheService) InvalidateItemHierarchy(itemID int, ancestorIDs []int) error {
	s.cache.Delete(itemID)
	for _, ancestorID := range ancestorIDs {
		s.cache.Delete(ancestorID)
	}
	return nil
}

// WarmCache pre-loads the effective project for recently active items. Bounded
// by WarmupBatchSize and intended to run in the background.
func (s *ItemCacheService) WarmCache() error {
	query := `
		SELECT DISTINCT item_id
		FROM (
			SELECT item_id, MAX(changed_at) as last_change
			FROM item_history
			WHERE changed_at > ?
			GROUP BY item_id
			ORDER BY last_change DESC
			LIMIT ?
		) AS hot
	`

	oneHourAgo := time.Now().Add(-time.Hour).UTC()
	rows, err := s.db.Query(query, oneHourAgo, s.config.WarmupBatchSize)
	if err != nil {
		slog.Error("failed to identify hot items for cache warming", slog.String("component", "item_cache"), slog.Any("error", err))
		return err
	}
	defer rows.Close()

	itemIDs := make([]int, 0, s.config.WarmupBatchSize)
	for rows.Next() {
		var itemID int
		if err := rows.Scan(&itemID); err != nil {
			continue
		}
		itemIDs = append(itemIDs, itemID)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	warmed := 0
	for _, itemID := range itemIDs {
		if _, _, err := s.GetEffectiveProjectForItem(itemID); err != nil {
			// A vanished or unreadable item must not abort the warm-up.
			continue
		}
		warmed++
	}

	slog.Debug("item cache warmed", slog.String("component", "item_cache"), slog.Int("hot_items_count", len(itemIDs)), slog.Int("warmed", warmed))
	return nil
}

// GetStats returns cache statistics.
func (s *ItemCacheService) GetStats() map[string]any {
	hits := atomic.LoadInt64(&s.hierarchyHits)
	misses := atomic.LoadInt64(&s.hierarchyMisses)
	total := hits + misses

	return map[string]any{
		"hierarchy_hits":       hits,
		"hierarchy_misses":     misses,
		"hierarchy_hit_rate":   float64(hits) / float64(max(total, 1)),
		"errors":               atomic.LoadInt64(&s.errors),
		"hierarchy_cache_size": s.cache.Len(),
	}
}
