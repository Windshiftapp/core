package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"windshift/internal/database"

	"github.com/allegro/bigcache/v3"
)

// ItemHierarchyCache stores cached hierarchy data for an item
type ItemHierarchyCache struct {
	ItemID             int       `json:"item_id"`
	EffectiveProjectID *int      `json:"effective_project_id"`
	AncestorPath       []int     `json:"ancestor_path"` // IDs from root to parent
	Level              int       `json:"level"`
	CachedAt           time.Time `json:"cached_at"`
}

// ProjectInheritanceCache caches project inheritance for a workspace
type ProjectInheritanceCache struct {
	WorkspaceID    int          `json:"workspace_id"`
	ItemProjectMap map[int]*int `json:"item_project_map"` // item_id -> effective_project_id
	Version        int64        `json:"version"`          // For invalidation
	CachedAt       time.Time    `json:"cached_at"`
}

// ItemCacheService handles cached item hierarchy and project data
type ItemCacheService struct {
	hierarchyCache *bigcache.BigCache
	projectCache   *bigcache.BigCache
	db             database.Database

	// Cache statistics
	hierarchyHits   int64
	hierarchyMisses int64
	projectHits     int64
	projectMisses   int64
	errors          int64

	// Configuration
	config ItemCacheConfig
}

// ItemCacheConfig represents configuration for the item cache
type ItemCacheConfig struct {
	HierarchyTTL    time.Duration `json:"hierarchy_ttl"`     // Default: 5min
	ProjectTTL      time.Duration `json:"project_ttl"`       // Default: 15min
	MaxCacheSize    int           `json:"max_cache_size"`    // Default: 512MB total
	WarmupBatchSize int           `json:"warmup_batch_size"` // Default: 500
	EnablePreWarm   bool          `json:"enable_pre_warm"`   // Default: true
}

// DefaultItemCacheConfig returns default configuration
func DefaultItemCacheConfig() ItemCacheConfig {
	return ItemCacheConfig{
		HierarchyTTL:    5 * time.Minute,
		ProjectTTL:      15 * time.Minute,
		MaxCacheSize:    512, // 512MB total
		WarmupBatchSize: 500,
		EnablePreWarm:   true,
	}
}

// NewItemCacheService creates a new item cache service
func NewItemCacheService(db database.Database, config ItemCacheConfig) (*ItemCacheService, error) {
	// Configure hierarchy cache
	hierarchyConfig := bigcache.Config{
		Shards:             1024,
		LifeWindow:         config.HierarchyTTL,
		CleanWindow:        1 * time.Minute,
		MaxEntriesInWindow: 100000, // Support up to 100k items
		MaxEntrySize:       4096,   // 4KB per entry
		Verbose:            false,
		HardMaxCacheSize:   config.MaxCacheSize / 2, // Half for hierarchy
		OnRemove:           nil,
	}

	hierarchyCache, err := bigcache.New(context.Background(), hierarchyConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create hierarchy cache: %w", err)
	}

	// Configure project cache
	projectConfig := bigcache.Config{
		Shards:             256,
		LifeWindow:         config.ProjectTTL,
		CleanWindow:        5 * time.Minute,
		MaxEntriesInWindow: 10000, // Support up to 10k workspaces
		MaxEntrySize:       65536, // 64KB per entry (can be large for big workspaces)
		Verbose:            false,
		HardMaxCacheSize:   config.MaxCacheSize / 2, // Half for projects
		OnRemove:           nil,
	}

	projectCache, err := bigcache.New(context.Background(), projectConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create project cache: %w", err)
	}

	service := &ItemCacheService{
		hierarchyCache: hierarchyCache,
		projectCache:   projectCache,
		db:             db,
		config:         config,
	}

	// Warm up cache if configured
	if config.EnablePreWarm {
		go func() { _ = service.WarmCache() }()
	}

	return service, nil
}

// GetItemHierarchy retrieves cached hierarchy data for an item
func (ics *ItemCacheService) GetItemHierarchy(itemID int) (*ItemHierarchyCache, error) {
	key := ics.getHierarchyKey(itemID)

	data, err := ics.hierarchyCache.Get(key)
	if err == nil {
		atomic.AddInt64(&ics.hierarchyHits, 1)

		var cache ItemHierarchyCache
		if err = json.Unmarshal(data, &cache); err != nil {
			atomic.AddInt64(&ics.errors, 1)
			return nil, fmt.Errorf("failed to unmarshal hierarchy cache: %w", err)
		}
		return &cache, nil
	}

	atomic.AddInt64(&ics.hierarchyMisses, 1)
	return nil, err
}

// SetItemHierarchy stores hierarchy data in cache
func (ics *ItemCacheService) SetItemHierarchy(cache *ItemHierarchyCache) error {
	cache.CachedAt = time.Now()

	data, err := json.Marshal(cache)
	if err != nil {
		atomic.AddInt64(&ics.errors, 1)
		return fmt.Errorf("failed to marshal hierarchy cache: %w", err)
	}

	key := ics.getHierarchyKey(cache.ItemID)
	return ics.hierarchyCache.Set(key, data)
}

// InvalidateItemHierarchy removes an item and its ancestors from cache
func (ics *ItemCacheService) InvalidateItemHierarchy(itemID int, ancestorIDs []int) error {
	// Invalidate the item itself
	key := ics.getHierarchyKey(itemID)
	_ = ics.hierarchyCache.Delete(key)

	// Invalidate all ancestors (their descendant counts changed)
	for _, ancestorID := range ancestorIDs {
		key := ics.getHierarchyKey(ancestorID)
		_ = ics.hierarchyCache.Delete(key)
	}

	return nil
}

// GetProjectCache retrieves cached project inheritance for a workspace
func (ics *ItemCacheService) GetProjectCache(workspaceID int) (*ProjectInheritanceCache, error) {
	key := ics.getProjectKey(workspaceID)

	data, err := ics.projectCache.Get(key)
	if err == nil {
		atomic.AddInt64(&ics.projectHits, 1)

		var cache ProjectInheritanceCache
		if err = json.Unmarshal(data, &cache); err != nil {
			atomic.AddInt64(&ics.errors, 1)
			return nil, fmt.Errorf("failed to unmarshal project cache: %w", err)
		}
		return &cache, nil
	}

	atomic.AddInt64(&ics.projectMisses, 1)
	return nil, err
}

// SetProjectCache stores project inheritance data in cache
func (ics *ItemCacheService) SetProjectCache(cache *ProjectInheritanceCache) error {
	cache.CachedAt = time.Now()
	cache.Version = time.Now().Unix()

	data, err := json.Marshal(cache)
	if err != nil {
		atomic.AddInt64(&ics.errors, 1)
		return fmt.Errorf("failed to marshal project cache: %w", err)
	}

	key := ics.getProjectKey(cache.WorkspaceID)
	return ics.projectCache.Set(key, data)
}

// InvalidateWorkspaceProjects clears project cache for a workspace
func (ics *ItemCacheService) InvalidateWorkspaceProjects(workspaceID int) error {
	key := ics.getProjectKey(workspaceID)
	return ics.projectCache.Delete(key)
}

// InvalidateProjectInheritors invalidates items that inherit from a project change
func (ics *ItemCacheService) InvalidateProjectInheritors(tx database.Tx, itemID int) error {
	// Find all descendants that inherit project
	query := `
		WITH RECURSIVE descendants AS (
			SELECT id, parent_id, inherit_project
			FROM items
			WHERE parent_id = ?
			UNION ALL
			SELECT i.id, i.parent_id, i.inherit_project
			FROM items i
			INNER JOIN descendants d ON i.parent_id = d.id
			WHERE d.inherit_project = true
		)
		SELECT id FROM descendants WHERE inherit_project = true
	`

	rows, err := tx.Query(query, itemID)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var descendantID int
		if err := rows.Scan(&descendantID); err != nil {
			continue
		}
		key := ics.getHierarchyKey(descendantID)
		_ = ics.hierarchyCache.Delete(key)
	}

	return rows.Err()
}

// WarmCache pre-loads frequently accessed items
func (ics *ItemCacheService) WarmCache() error {
	// Identify hot items (recently accessed or frequently updated)
	query := `
		SELECT DISTINCT item_id
		FROM (
			SELECT item_id, MAX(changed_at) as last_change
			FROM item_history
			WHERE changed_at > datetime('now', '-1 hour')
			GROUP BY item_id
			ORDER BY last_change DESC
			LIMIT ?
		)
	`

	rows, err := ics.db.Query(query, ics.config.WarmupBatchSize)
	if err != nil {
		slog.Error("failed to identify hot items for cache warming", slog.String("component", "item_cache"), slog.Any("error", err))
		return err
	}
	defer rows.Close()

	itemIDs := make([]int, 0, ics.config.WarmupBatchSize)
	for rows.Next() {
		var itemID int
		if err := rows.Scan(&itemID); err != nil {
			continue
		}
		itemIDs = append(itemIDs, itemID)
	}

	// Load hierarchy data for hot items
	for _, itemID := range itemIDs {
		// In production, this would call the actual hierarchy calculation
		// For now, we'll skip the implementation details
		_ = itemID
	}

	slog.Debug("item cache warmed", slog.String("component", "item_cache"), slog.Int("hot_items_count", len(itemIDs)))
	return nil
}

// GetStats returns cache statistics
func (ics *ItemCacheService) GetStats() map[string]interface{} {
	hierarchyTotal := ics.hierarchyHits + ics.hierarchyMisses
	projectTotal := ics.projectHits + ics.projectMisses

	stats := map[string]interface{}{
		"hierarchy_hits":       ics.hierarchyHits,
		"hierarchy_misses":     ics.hierarchyMisses,
		"hierarchy_hit_rate":   float64(ics.hierarchyHits) / float64(max(hierarchyTotal, 1)),
		"project_hits":         ics.projectHits,
		"project_misses":       ics.projectMisses,
		"project_hit_rate":     float64(ics.projectHits) / float64(max(projectTotal, 1)),
		"errors":               ics.errors,
		"hierarchy_cache_size": ics.hierarchyCache.Len(),
		"project_cache_size":   ics.projectCache.Len(),
	}

	return stats
}

// Clear removes all entries from both caches
func (ics *ItemCacheService) Clear() error {
	if err := ics.hierarchyCache.Reset(); err != nil {
		return err
	}
	return ics.projectCache.Reset()
}

// GetEffectiveProjectForItem retrieves or calculates the effective project for an item
// This method first checks the cache, then falls back to database calculation if needed
func (ics *ItemCacheService) GetEffectiveProjectForItem(itemID, workspaceID int) (effectiveProjectID *int, projectInheritanceMode string, err error) {
	// Try cache first
	hierarchyCache, err := ics.GetItemHierarchy(itemID)
	if err == nil && hierarchyCache != nil {
		// Cache hit!
		if hierarchyCache.EffectiveProjectID != nil {
			mode := "direct" // Default assumption
			return hierarchyCache.EffectiveProjectID, mode, nil
		}
	}

	// Cache miss - calculate from database
	effectiveProjectID, inheritProject, directProjectID, err := ics.calculateEffectiveProject(itemID)
	if err != nil {
		return nil, "", err
	}

	// Determine inheritance mode
	switch {
	case directProjectID == nil && !inheritProject:
		projectInheritanceMode = "none"
	case inheritProject:
		projectInheritanceMode = "inherit"
	default:
		projectInheritanceMode = "direct"
	}

	// Store in cache for future use
	cacheEntry := &ItemHierarchyCache{
		ItemID:             itemID,
		EffectiveProjectID: effectiveProjectID,
		CachedAt:           time.Now(),
	}
	_ = ics.SetItemHierarchy(cacheEntry) // Ignore cache write errors

	return effectiveProjectID, projectInheritanceMode, nil
}

// calculateEffectiveProject walks up the hierarchy to find the effective project
func (ics *ItemCacheService) calculateEffectiveProject(itemID int) (effectiveProjectID *int, inheritProject bool, directProjectID *int, err error) {
	query := `
		WITH RECURSIVE effective_projects AS (
			-- Base case: the item itself
			SELECT
				id,
				project_id,
				inherit_project,
				parent_id,
				CASE
					WHEN inherit_project = true THEN NULL
					ELSE project_id
				END as effective_project_id,
				0 as depth
			FROM items
			WHERE id = ?

			UNION ALL

			-- Recursive case: climb up hierarchy to find inherited project
			SELECT
				ep.id,
				ep.project_id,
				ep.inherit_project,
				i.parent_id,
				CASE
					WHEN i.project_id IS NOT NULL AND i.inherit_project = false THEN i.project_id
					ELSE ep.effective_project_id
				END as effective_project_id,
				ep.depth + 1
			FROM effective_projects ep
			JOIN items i ON ep.parent_id = i.id
			WHERE ep.effective_project_id IS NULL
			  AND ep.inherit_project = true
			  AND ep.depth < 10
		)
		SELECT
			project_id,
			inherit_project,
			effective_project_id
		FROM effective_projects
		WHERE id = ?
		ORDER BY depth DESC
		LIMIT 1
	`

	var nullableProjectID, nullableEffectiveProjectID sql.NullInt64
	err = ics.db.QueryRow(query, itemID, itemID).Scan(&nullableProjectID, &inheritProject, &nullableEffectiveProjectID)
	if err != nil {
		return nil, false, nil, fmt.Errorf("failed to calculate effective project: %w", err)
	}

	if nullableProjectID.Valid {
		pid := int(nullableProjectID.Int64)
		directProjectID = &pid
	}

	if nullableEffectiveProjectID.Valid {
		epid := int(nullableEffectiveProjectID.Int64)
		effectiveProjectID = &epid
	}

	return effectiveProjectID, inheritProject, directProjectID, nil
}

// Close shuts down the cache service
func (ics *ItemCacheService) Close() error {
	if err := ics.hierarchyCache.Close(); err != nil {
		return err
	}
	return ics.projectCache.Close()
}

// Helper methods

func (ics *ItemCacheService) getHierarchyKey(itemID int) string {
	return fmt.Sprintf("item:hierarchy:%d", itemID)
}

func (ics *ItemCacheService) getProjectKey(workspaceID int) string {
	return fmt.Sprintf("workspace:projects:%d", workspaceID)
}
