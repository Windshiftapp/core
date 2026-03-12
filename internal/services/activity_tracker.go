package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"windshift/internal/database"

	"github.com/allegro/bigcache/v3"
)

// ActivityType represents types of user activities
type ActivityType string

const (
	ActivityView    ActivityType = "view"
	ActivityEdit    ActivityType = "edit"
	ActivityComment ActivityType = "comment"
)

// ActivityTrackerConfig represents configuration for the activity tracker
type ActivityTrackerConfig struct {
	TTL                    time.Duration `json:"ttl"`                      // Cache TTL, default: 24h
	MaxCacheSize           int           `json:"max_cache_size"`           // Default: 128MB
	FlushInterval          time.Duration `json:"flush_interval"`           // Default: 5min
	MaxWorkspaceVisits     int           `json:"max_workspace_visits"`     // Default: 10
	MaxItemActivities      int           `json:"max_item_activities"`      // Default: 50 per type
	RetentionDays          int           `json:"retention_days"`           // Default: 90
	ImmediateFlushActivity bool          `json:"immediate_flush_activity"` // Flush edits/comments immediately
}

// DefaultActivityTrackerConfig returns default configuration
func DefaultActivityTrackerConfig() ActivityTrackerConfig {
	return ActivityTrackerConfig{
		TTL:                    24 * time.Hour,
		MaxCacheSize:           128, // 128MB
		FlushInterval:          5 * time.Minute,
		MaxWorkspaceVisits:     10,
		MaxItemActivities:      50,
		RetentionDays:          90,
		ImmediateFlushActivity: true,
	}
}

// ActivityTracker handles user activity tracking with caching
type ActivityTracker struct {
	cache  *bigcache.BigCache
	db     database.Database
	config ActivityTrackerConfig

	// Write batchers for DB persistence
	visitBatcher    *WriteBatcher[WorkspaceVisit]
	activityBatcher *WriteBatcher[ItemActivity]

	// Shadow maps for read-your-writes (visible before batcher flush)
	pendingWorkspaceVisits map[string]*WorkspaceVisit // key: userID:workspaceID
	pendingItemActivities  map[string]*ItemActivity   // key: userID:itemID:activityType
	pendingMu              sync.RWMutex

	// Cache statistics
	hits    int64
	misses  int64
	errors  int64
	flushes int64
}

// WorkspaceVisit tracks a workspace visit
type WorkspaceVisit struct {
	UserID      int
	WorkspaceID int
	VisitedAt   time.Time
	VisitCount  int
}

// ItemActivity tracks item activity
type ItemActivity struct {
	UserID        int
	ItemID        int
	ActivityType  ActivityType
	ActivityAt    time.Time
	ActivityCount int
}

// UserActivityCache stores cached activity data for a user
type UserActivityCache struct {
	UserID          int                             `json:"user_id"`
	WorkspaceVisits []WorkspaceVisit                `json:"workspace_visits"` // Last 10
	ItemActivities  map[ActivityType][]ItemActivity `json:"item_activities"`  // Last 50 per type
	ItemWatches     []int                           `json:"item_watches"`     // All active watches
	CachedAt        time.Time                       `json:"cached_at"`
	ExpiresAt       time.Time                       `json:"expires_at"`
}

// NewActivityTracker creates a new activity tracker with caching
func NewActivityTracker(db database.Database, config ActivityTrackerConfig) (*ActivityTracker, error) {
	// Configure BigCache
	cacheConfig := NewBigCacheConfig(BigCacheOptions{
		TTL:          config.TTL,
		MaxCacheMB:   config.MaxCacheSize,
		MaxEntrySize: 16384, // 16KB per entry (larger for activity data)
	})

	cache, err := bigcache.New(context.Background(), cacheConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create BigCache for activity tracker: %w", err)
	}

	tracker := &ActivityTracker{
		cache:                  cache,
		db:                     db,
		config:                 config,
		pendingWorkspaceVisits: make(map[string]*WorkspaceVisit),
		pendingItemActivities:  make(map[string]*ItemActivity),
	}

	// Create write batchers for DB persistence
	visitConfig := WriteBatcherConfig{
		FlushInterval: 30 * time.Second,
		MaxBatchSize:  100,
		Name:          "workspace_visits",
	}
	tracker.visitBatcher = NewWriteBatcher(visitConfig, tracker.flushWorkspaceVisitBatch)
	tracker.visitBatcher.Start()

	activityConfig := WriteBatcherConfig{
		FlushInterval: 30 * time.Second,
		MaxBatchSize:  100,
		Name:          "item_activities",
	}
	tracker.activityBatcher = NewWriteBatcher(activityConfig, tracker.flushItemActivityBatch)
	tracker.activityBatcher.Start()

	slog.Debug("ActivityTracker initialized", slog.String("component", "activity"), slog.Duration("flush_interval", visitConfig.FlushInterval))

	return tracker, nil
}

// getCacheKey generates a cache key for a user's activities
func (at *ActivityTracker) getCacheKey(userID int) string {
	return fmt.Sprintf("activity:user:%d", userID)
}

// TrackWorkspaceVisit records a workspace visit
func (at *ActivityTracker) TrackWorkspaceVisit(userID, workspaceID int) error {
	now := time.Now()
	key := fmt.Sprintf("%d:%d", userID, workspaceID)

	// Update shadow map for read-your-writes
	at.pendingMu.Lock()
	if visit, exists := at.pendingWorkspaceVisits[key]; exists {
		visit.VisitedAt = now
		visit.VisitCount++
	} else {
		at.pendingWorkspaceVisits[key] = &WorkspaceVisit{
			UserID:      userID,
			WorkspaceID: workspaceID,
			VisitedAt:   now,
			VisitCount:  1,
		}
	}
	at.pendingMu.Unlock()

	// Queue for DB persistence via WriteBatcher
	at.visitBatcher.Add(WorkspaceVisit{
		UserID:      userID,
		WorkspaceID: workspaceID,
		VisitedAt:   now,
		VisitCount:  1,
	})

	// Invalidate cache for this user
	_ = at.InvalidateUserCache(userID)

	return nil
}

// TrackItemActivity records an item activity (view/edit/comment)
func (at *ActivityTracker) TrackItemActivity(userID, itemID int, activityType ActivityType) error {
	now := time.Now()
	key := fmt.Sprintf("%d:%d:%s", userID, itemID, activityType)

	// Update shadow map for read-your-writes
	at.pendingMu.Lock()
	if activity, exists := at.pendingItemActivities[key]; exists {
		activity.ActivityAt = now
		activity.ActivityCount++
	} else {
		at.pendingItemActivities[key] = &ItemActivity{
			UserID:        userID,
			ItemID:        itemID,
			ActivityType:  activityType,
			ActivityAt:    now,
			ActivityCount: 1,
		}
	}
	at.pendingMu.Unlock()

	// Queue for DB persistence via WriteBatcher
	at.activityBatcher.Add(ItemActivity{
		UserID:        userID,
		ItemID:        itemID,
		ActivityType:  activityType,
		ActivityAt:    now,
		ActivityCount: 1,
	})

	// Invalidate cache for this user
	_ = at.InvalidateUserCache(userID)

	return nil
}

// AddWatch adds a watch for an item
func (at *ActivityTracker) AddWatch(userID, itemID int, reason string) error {
	_, err := at.db.ExecWrite(`
		INSERT INTO item_watches (user_id, item_id, is_active, watch_reason, created_at, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(user_id, item_id) DO UPDATE SET
			is_active = ?,
			watch_reason = ?,
			updated_at = CURRENT_TIMESTAMP
	`, userID, itemID, true, reason, true, reason)

	if err != nil {
		return fmt.Errorf("failed to add watch: %w", err)
	}

	// Invalidate cache
	_ = at.InvalidateUserCache(userID)

	return nil
}

// RemoveWatch removes a watch for an item
func (at *ActivityTracker) RemoveWatch(userID, itemID int) error {
	_, err := at.db.ExecWrite(`
		UPDATE item_watches
		SET is_active = ?, updated_at = CURRENT_TIMESTAMP
		WHERE user_id = ? AND item_id = ?
	`, false, userID, itemID)

	if err != nil {
		return fmt.Errorf("failed to remove watch: %w", err)
	}

	// Invalidate cache
	_ = at.InvalidateUserCache(userID)

	return nil
}

// GetUserWatches returns all active watches for a user
func (at *ActivityTracker) GetUserWatches(userID int) ([]int, error) {
	rows, err := at.db.Query(`
		SELECT item_id
		FROM item_watches
		WHERE user_id = ? AND is_active = ?
		ORDER BY created_at DESC
	`, userID, true)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var itemIDs []int
	for rows.Next() {
		var itemID int
		if err := rows.Scan(&itemID); err == nil {
			itemIDs = append(itemIDs, itemID)
		}
	}

	return itemIDs, nil
}

// IsWatching checks if a user is watching an item
func (at *ActivityTracker) IsWatching(userID, itemID int) (bool, error) {
	var isActive bool
	err := at.db.QueryRow(`
		SELECT is_active
		FROM item_watches
		WHERE user_id = ? AND item_id = ?
	`, userID, itemID).Scan(&isActive)

	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return isActive, nil
}

// GetUserActivity retrieves comprehensive activity data for a user
func (at *ActivityTracker) GetUserActivity(userID int) (*UserActivityCache, error) {
	// Try cache first
	cached, err := at.getUserActivityCache(userID)
	if err == nil {
		atomic.AddInt64(&at.hits, 1)
		// Merge pending activities before returning
		at.mergePendingActivities(userID, cached)
		return cached, nil
	}

	// Cache miss - load from database
	atomic.AddInt64(&at.misses, 1)
	result, err := at.loadUserActivityFromDB(userID)
	if err != nil {
		return nil, err
	}

	// Merge pending activities
	at.mergePendingActivities(userID, result)
	return result, nil
}

// mergePendingActivities adds pending buffer activities to the cached result
func (at *ActivityTracker) mergePendingActivities(userID int, cached *UserActivityCache) {
	at.pendingMu.RLock()
	defer at.pendingMu.RUnlock()

	for _, activity := range at.pendingItemActivities {
		if activity.UserID != userID {
			continue
		}

		// Get the appropriate activity type list
		actType := activity.ActivityType
		existing := cached.ItemActivities[actType]

		// Check if this item is already in the list
		found := false
		for i, a := range existing {
			if a.ItemID == activity.ItemID {
				// Update with newer timestamp
				if activity.ActivityAt.After(a.ActivityAt) {
					existing[i].ActivityAt = activity.ActivityAt
					existing[i].ActivityCount = a.ActivityCount + activity.ActivityCount
				}
				found = true
				break
			}
		}

		if !found {
			// Prepend new activity
			cached.ItemActivities[actType] = append(
				[]ItemActivity{*activity},
				existing...,
			)
		}
	}

	// Re-sort by activity time (most recent first) and trim to max
	for actType, activities := range cached.ItemActivities {
		sort.Slice(activities, func(i, j int) bool {
			return activities[i].ActivityAt.After(activities[j].ActivityAt)
		})
		if len(activities) > at.config.MaxItemActivities {
			activities = activities[:at.config.MaxItemActivities]
		}
		cached.ItemActivities[actType] = activities
	}
}

// getUserActivityCache retrieves cached activity data for a user
func (at *ActivityTracker) getUserActivityCache(userID int) (*UserActivityCache, error) {
	cacheKey := at.getCacheKey(userID)

	entry, err := at.cache.Get(cacheKey)
	if err != nil {
		return nil, err
	}

	var cached UserActivityCache
	if err := json.Unmarshal(entry, &cached); err != nil {
		// Remove corrupted cache entry
		_ = at.cache.Delete(cacheKey)
		return nil, err
	}

	// Check if cache entry has expired
	if time.Now().After(cached.ExpiresAt) {
		_ = at.cache.Delete(cacheKey)
		return nil, fmt.Errorf("cache entry expired")
	}

	return &cached, nil
}

// loadUserActivityFromDB loads user activity data from database
func (at *ActivityTracker) loadUserActivityFromDB(userID int) (*UserActivityCache, error) {
	now := time.Now()

	cached := &UserActivityCache{
		UserID:          userID,
		WorkspaceVisits: []WorkspaceVisit{},
		ItemActivities: map[ActivityType][]ItemActivity{
			ActivityView:    {},
			ActivityEdit:    {},
			ActivityComment: {},
		},
		ItemWatches: []int{},
		CachedAt:    now,
		ExpiresAt:   now.Add(at.config.TTL),
	}

	// Load workspace visits (last 10)
	workspaceRows, err := at.db.Query(`
		SELECT workspace_id, last_visited_at, visit_count
		FROM user_workspace_visits
		WHERE user_id = ?
		ORDER BY last_visited_at DESC
		LIMIT ?
	`, userID, at.config.MaxWorkspaceVisits)
	if err != nil {
		return nil, fmt.Errorf("failed to load workspace visits: %w", err)
	}
	defer func() { _ = workspaceRows.Close() }()

	for workspaceRows.Next() {
		var visit WorkspaceVisit
		visit.UserID = userID
		if err = workspaceRows.Scan(&visit.WorkspaceID, &visit.VisitedAt, &visit.VisitCount); err == nil {
			cached.WorkspaceVisits = append(cached.WorkspaceVisits, visit)
		}
	}

	// Load item activities for each type (last 50 per type)
	for _, activityType := range []ActivityType{ActivityView, ActivityEdit, ActivityComment} {
		var activityRows *sql.Rows
		activityRows, err = at.db.Query(`
			SELECT item_id, last_activity_at, activity_count
			FROM user_item_activities
			WHERE user_id = ? AND activity_type = ?
			ORDER BY last_activity_at DESC
			LIMIT ?
		`, userID, activityType, at.config.MaxItemActivities)
		if err != nil {
			slog.Error("Failed to load item activities", slog.String("component", "activity"), slog.String("activity_type", string(activityType)), slog.Any("error", err))
			continue
		}

		activities := []ItemActivity{}
		for activityRows.Next() {
			var activity ItemActivity
			activity.UserID = userID
			activity.ActivityType = activityType
			if err = activityRows.Scan(&activity.ItemID, &activity.ActivityAt, &activity.ActivityCount); err == nil {
				activities = append(activities, activity)
			}
		}
		_ = activityRows.Close()

		cached.ItemActivities[activityType] = activities
	}

	// Load active watches
	watchRows, err := at.db.Query(`
		SELECT item_id
		FROM item_watches
		WHERE user_id = ? AND is_active = ?
		ORDER BY created_at DESC
	`, userID, true)
	if err != nil {
		slog.Error("Failed to load watches", slog.String("component", "activity"), slog.Any("error", err))
	} else {
		defer func() { _ = watchRows.Close() }()
		for watchRows.Next() {
			var itemID int
			if err := watchRows.Scan(&itemID); err == nil {
				cached.ItemWatches = append(cached.ItemWatches, itemID)
			}
		}
	}

	// Store in cache
	_ = at.storeUserActivityCache(userID, cached)

	return cached, nil
}

// storeUserActivityCache stores activity cache data
func (at *ActivityTracker) storeUserActivityCache(userID int, cached *UserActivityCache) error {
	cacheKey := at.getCacheKey(userID)

	data, err := json.Marshal(cached)
	if err != nil {
		return fmt.Errorf("error marshaling cache data: %w", err)
	}

	return at.cache.Set(cacheKey, data)
}

// InvalidateUserCache removes a user's activity cache
func (at *ActivityTracker) InvalidateUserCache(userID int) error {
	cacheKey := at.getCacheKey(userID)
	return at.cache.Delete(cacheKey)
}

// FlushPendingActivities flushes both write batchers
func (at *ActivityTracker) FlushPendingActivities() error {
	if err := at.visitBatcher.Flush(); err != nil {
		return fmt.Errorf("flush workspace visits: %w", err)
	}
	if err := at.activityBatcher.Flush(); err != nil {
		return fmt.Errorf("flush item activities: %w", err)
	}
	return nil
}

// flushWorkspaceVisitBatch persists a batch of workspace visits to the database.
// Called by WriteBatcher every 30s or when 100 items are queued.
func (at *ActivityTracker) flushWorkspaceVisitBatch(visits []WorkspaceVisit) error {
	expiresAt := time.Now().AddDate(0, 0, at.config.RetentionDays)

	tx, err := at.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, visit := range visits {
		_, err := tx.Exec(`
			INSERT INTO user_workspace_visits (user_id, workspace_id, last_visited_at, visit_count, expires_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(user_id, workspace_id) DO UPDATE SET
				last_visited_at = CASE WHEN excluded.last_visited_at > user_workspace_visits.last_visited_at THEN excluded.last_visited_at ELSE user_workspace_visits.last_visited_at END,
				visit_count = visit_count + ?,
				expires_at = ?,
				updated_at = CURRENT_TIMESTAMP
		`, visit.UserID, visit.WorkspaceID, visit.VisitedAt, visit.VisitCount, expiresAt,
			visit.VisitCount, expiresAt)
		if err != nil {
			return fmt.Errorf("flush workspace visit: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Clear flushed entries from shadow map
	at.pendingMu.Lock()
	for _, visit := range visits {
		key := fmt.Sprintf("%d:%d", visit.UserID, visit.WorkspaceID)
		if existing, ok := at.pendingWorkspaceVisits[key]; ok {
			if !existing.VisitedAt.After(visit.VisitedAt) {
				delete(at.pendingWorkspaceVisits, key)
			}
		}
	}
	at.pendingMu.Unlock()

	atomic.AddInt64(&at.flushes, 1)
	return nil
}

// flushItemActivityBatch persists a batch of item activities to the database.
// Called by WriteBatcher every 30s or when 100 items are queued.
func (at *ActivityTracker) flushItemActivityBatch(activities []ItemActivity) error {
	expiresAt := time.Now().AddDate(0, 0, at.config.RetentionDays)

	tx, err := at.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, activity := range activities {
		_, err := tx.Exec(`
			INSERT INTO user_item_activities (user_id, item_id, activity_type, last_activity_at, activity_count, expires_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(user_id, item_id, activity_type) DO UPDATE SET
				last_activity_at = CASE WHEN excluded.last_activity_at > user_item_activities.last_activity_at THEN excluded.last_activity_at ELSE user_item_activities.last_activity_at END,
				activity_count = activity_count + ?,
				expires_at = ?,
				updated_at = CURRENT_TIMESTAMP
		`, activity.UserID, activity.ItemID, activity.ActivityType, activity.ActivityAt, activity.ActivityCount, expiresAt,
			activity.ActivityCount, expiresAt)
		if err != nil {
			return fmt.Errorf("flush item activity: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Clear flushed entries from shadow map
	at.pendingMu.Lock()
	for _, activity := range activities {
		key := fmt.Sprintf("%d:%d:%s", activity.UserID, activity.ItemID, activity.ActivityType)
		if existing, ok := at.pendingItemActivities[key]; ok {
			if !existing.ActivityAt.After(activity.ActivityAt) {
				delete(at.pendingItemActivities, key)
			}
		}
	}
	at.pendingMu.Unlock()

	atomic.AddInt64(&at.flushes, 1)
	return nil
}

// CleanupExpiredActivities removes expired activity records
func (at *ActivityTracker) CleanupExpiredActivities() error {
	now := time.Now()

	// Clean up expired workspace visits
	result, err := at.db.ExecWrite(`DELETE FROM user_workspace_visits WHERE expires_at < ?`, now)
	if err != nil {
		return fmt.Errorf("failed to clean up workspace visits: %w", err)
	}
	deleted, _ := result.RowsAffected()
	slog.Debug("Cleaned up expired workspace visits", slog.String("component", "activity"), slog.Int64("deleted", deleted))

	// Clean up expired item activities
	result, err = at.db.ExecWrite(`DELETE FROM user_item_activities WHERE expires_at < ?`, now)
	if err != nil {
		return fmt.Errorf("failed to clean up item activities: %w", err)
	}
	deleted, _ = result.RowsAffected()
	slog.Debug("Cleaned up expired item activities", slog.String("component", "activity"), slog.Int64("deleted", deleted))

	// Also enforce count limits (keep only most recent N records per user)
	// This is a safety measure in case expiration isn't working properly

	// Workspace visits: keep only last 10 per user
	_, err = at.db.ExecWrite(`
		DELETE FROM user_workspace_visits
		WHERE id NOT IN (
			SELECT id FROM (
				SELECT id, user_id
				FROM user_workspace_visits
				ORDER BY user_id, last_visited_at DESC
			) GROUP BY user_id
			LIMIT ?
		)
	`, at.config.MaxWorkspaceVisits)
	if err != nil {
		slog.Error("Error enforcing workspace visit limits", slog.String("component", "activity"), slog.Any("error", err))
	}

	// Item activities: keep only last 50 per user per type
	for _, activityType := range []ActivityType{ActivityView, ActivityEdit, ActivityComment} {
		_, err = at.db.ExecWrite(`
			DELETE FROM user_item_activities
			WHERE activity_type = ? AND id NOT IN (
				SELECT id FROM (
					SELECT id, user_id
					FROM user_item_activities
					WHERE activity_type = ?
					ORDER BY user_id, last_activity_at DESC
				) GROUP BY user_id
				LIMIT ?
			)
		`, activityType, activityType, at.config.MaxItemActivities)
		if err != nil {
			slog.Error("Error enforcing item activity limits", slog.String("component", "activity"), slog.String("activity_type", string(activityType)), slog.Any("error", err))
		}
	}

	return nil
}

// GetCacheStats returns current cache performance statistics
func (at *ActivityTracker) GetCacheStats() ActivityTrackerStats {
	hits := atomic.LoadInt64(&at.hits)
	misses := atomic.LoadInt64(&at.misses)
	errors := atomic.LoadInt64(&at.errors)
	flushes := atomic.LoadInt64(&at.flushes)
	total := hits + misses

	hitRatio := 0.0
	if total > 0 {
		hitRatio = float64(hits) / float64(total)
	}

	at.pendingMu.RLock()
	pendingWorkspaceVisits := len(at.pendingWorkspaceVisits)
	pendingItemActivities := len(at.pendingItemActivities)
	at.pendingMu.RUnlock()

	return ActivityTrackerStats{
		Hits:                   hits,
		Misses:                 misses,
		Errors:                 errors,
		Flushes:                flushes,
		HitRatio:               hitRatio,
		PendingWorkspaceVisits: pendingWorkspaceVisits,
		PendingItemActivities:  pendingItemActivities,
	}
}

// ActivityTrackerStats represents cache statistics
type ActivityTrackerStats struct {
	Hits                   int64   `json:"hits"`
	Misses                 int64   `json:"misses"`
	Errors                 int64   `json:"errors"`
	Flushes                int64   `json:"flushes"`
	HitRatio               float64 `json:"hit_ratio"`
	PendingWorkspaceVisits int     `json:"pending_workspace_visits"`
	PendingItemActivities  int     `json:"pending_item_activities"`
}

// Close gracefully shuts down the activity tracker
func (at *ActivityTracker) Close() error {
	slog.Debug("Closing ActivityTracker", slog.String("component", "activity"))

	// Stop write batchers (flushes remaining items)
	at.visitBatcher.Stop()
	at.activityBatcher.Stop()
	slog.Debug("Write batchers stopped", slog.String("component", "activity"))

	// Close cache
	err := at.cache.Close()
	slog.Debug("ActivityTracker cache closed", slog.String("component", "activity"))
	return err
}
