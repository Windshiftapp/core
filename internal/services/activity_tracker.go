package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"windshift/internal/cacheutil"
	"windshift/internal/database"
	"windshift/internal/repository"

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
	MaxCacheSize           int           `json:"max_cache_size"`           // Default: 50MB
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
		MaxCacheSize:           50,
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
	flushRevision          uint64 // guarded by pendingMu

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
	cache, err := cacheutil.New("activity", cacheutil.BigCacheOptions{
		TTL:               config.TTL,
		MaxCacheMB:        config.MaxCacheSize,
		MaxEntrySize:      16384, // 16KB per entry (larger for activity data)
		Shards:            32,
		InitialCapacityMB: 4,
	})
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
		FlushInterval:  30 * time.Second,
		MaxBatchSize:   100,
		OverflowPolicy: WriteBatcherDropNewest,
		Name:           "workspace_visits",
	}
	tracker.visitBatcher = NewCoalescingWriteBatcher(visitConfig, workspaceVisitKey, mergeWorkspaceVisits, tracker.flushWorkspaceVisitBatch)
	tracker.visitBatcher.Start()

	activityConfig := WriteBatcherConfig{
		FlushInterval:  30 * time.Second,
		MaxBatchSize:   100,
		OverflowPolicy: WriteBatcherDropNewest,
		Name:           "item_activities",
	}
	tracker.activityBatcher = NewCoalescingWriteBatcher(activityConfig, itemActivityKey, mergeItemActivities, tracker.flushItemActivityBatch)
	tracker.activityBatcher.Start()

	slog.Debug("ActivityTracker initialized", slog.String("component", "activity"), slog.Duration("flush_interval", visitConfig.FlushInterval))

	return tracker, nil
}

func workspaceVisitKey(visit WorkspaceVisit) string {
	return fmt.Sprintf("%d:%d", visit.UserID, visit.WorkspaceID)
}

func mergeWorkspaceVisits(existing, incoming WorkspaceVisit) WorkspaceVisit {
	existing.VisitCount += incoming.VisitCount
	if incoming.VisitedAt.After(existing.VisitedAt) {
		existing.VisitedAt = incoming.VisitedAt
	}
	return existing
}

func itemActivityKey(activity ItemActivity) string {
	return fmt.Sprintf("%d:%d:%s", activity.UserID, activity.ItemID, activity.ActivityType)
}

func mergeItemActivities(existing, incoming ItemActivity) ItemActivity {
	existing.ActivityCount += incoming.ActivityCount
	if incoming.ActivityAt.After(existing.ActivityAt) {
		existing.ActivityAt = incoming.ActivityAt
	}
	return existing
}

// getCacheKey generates a cache key for a user's activities
func (at *ActivityTracker) getCacheKey(userID int) string {
	return fmt.Sprintf("activity:user:%d", userID)
}

// TrackWorkspaceVisit records a workspace visit
func (at *ActivityTracker) TrackWorkspaceVisit(userID, workspaceID int) error {
	now := time.Now()
	key := fmt.Sprintf("%d:%d", userID, workspaceID)

	// Update the shadow map and queue under one lock. If the bounded queue
	// rejects a new unique key, roll the shadow update back so the shadow map is
	// bounded by the accepted queue rather than growing throughout an outage.
	at.pendingMu.Lock()
	previous, existed := at.pendingWorkspaceVisits[key]
	var previousValue WorkspaceVisit
	if existed {
		previousValue = *previous
	}
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
	accepted := at.visitBatcher.Add(WorkspaceVisit{
		UserID:      userID,
		WorkspaceID: workspaceID,
		VisitedAt:   now,
		VisitCount:  1,
	})
	if !accepted {
		if existed {
			at.pendingWorkspaceVisits[key] = &previousValue
		} else {
			delete(at.pendingWorkspaceVisits, key)
		}
		atomic.AddInt64(&at.errors, 1)
	}
	at.pendingMu.Unlock()

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
	previous, existed := at.pendingItemActivities[key]
	var previousValue ItemActivity
	if existed {
		previousValue = *previous
	}
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
	accepted := at.activityBatcher.Add(ItemActivity{
		UserID:        userID,
		ItemID:        itemID,
		ActivityType:  activityType,
		ActivityAt:    now,
		ActivityCount: 1,
	})
	if !accepted {
		if existed {
			at.pendingItemActivities[key] = &previousValue
		} else {
			delete(at.pendingItemActivities, key)
		}
		atomic.AddInt64(&at.errors, 1)
	}
	at.pendingMu.Unlock()

	// Invalidate cache for this user
	_ = at.InvalidateUserCache(userID)

	return nil
}

// GetUserActivity retrieves comprehensive activity data for a user
func (at *ActivityTracker) GetUserActivity(userID int) (*UserActivityCache, error) {
	for {
		at.pendingMu.RLock()
		revision := at.flushRevision
		cached, err := at.getUserActivityCache(userID)
		if err == nil {
			atomic.AddInt64(&at.hits, 1)
			at.mergePendingActivities(userID, cached)
			at.pendingMu.RUnlock()
			return cached, nil
		}
		at.pendingMu.RUnlock()

		atomic.AddInt64(&at.misses, 1)
		result, err := at.loadUserActivityFromDB(userID)
		if err != nil {
			return nil, err
		}
		at.pendingMu.RLock()
		// A concurrent flush may have retired pending data this snapshot missed.
		if revision != at.flushRevision {
			at.pendingMu.RUnlock()
			continue
		}
		_ = at.storeUserActivityCache(userID, result)
		at.mergePendingActivities(userID, result)
		at.pendingMu.RUnlock()
		return result, nil
	}
}

// mergePendingActivities adds pending activity. The caller holds pendingMu.
func (at *ActivityTracker) mergePendingActivities(userID int, cached *UserActivityCache) {
	for _, visit := range at.pendingWorkspaceVisits {
		if visit.UserID != userID {
			continue
		}

		found := false
		for i, existing := range cached.WorkspaceVisits {
			if existing.WorkspaceID != visit.WorkspaceID {
				continue
			}
			if visit.VisitedAt.After(existing.VisitedAt) {
				cached.WorkspaceVisits[i].VisitedAt = visit.VisitedAt
			}
			cached.WorkspaceVisits[i].VisitCount += visit.VisitCount
			found = true
			break
		}
		if !found {
			cached.WorkspaceVisits = append(cached.WorkspaceVisits, *visit)
		}
	}
	sort.Slice(cached.WorkspaceVisits, func(i, j int) bool {
		return cached.WorkspaceVisits[i].VisitedAt.After(cached.WorkspaceVisits[j].VisitedAt)
	})
	if len(cached.WorkspaceVisits) > at.config.MaxWorkspaceVisits {
		cached.WorkspaceVisits = cached.WorkspaceVisits[:at.config.MaxWorkspaceVisits]
	}

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
	if err := workspaceRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace visits: %w", err)
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
		if err := activityRows.Err(); err != nil {
			slog.Error("Failed to iterate item activities", slog.String("component", "activity"), slog.String("activity_type", string(activityType)), slog.Any("error", err))
		}
		_ = activityRows.Close()

		cached.ItemActivities[activityType] = activities
	}

	// Load active watches through the owning item repository.
	itemIDs, err := repository.NewItemRepository(at.db).GetUserWatchedItems(userID)
	if err != nil {
		slog.Error("Failed to load watches", slog.String("component", "activity"), slog.Any("error", err))
	} else {
		cached.ItemWatches = itemIDs
	}

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
func (at *ActivityTracker) flushWorkspaceVisitBatch(ctx context.Context, visits []WorkspaceVisit) error {
	expiresAt := time.Now().AddDate(0, 0, at.config.RetentionDays)

	tx, err := at.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin workspace visit transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := func() error {
		for _, visit := range visits {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO user_workspace_visits (user_id, workspace_id, last_visited_at, visit_count, expires_at)
				SELECT ?, ?, ?, ?, ?
				WHERE EXISTS (SELECT 1 FROM users WHERE id = ?)
				  AND EXISTS (SELECT 1 FROM workspaces WHERE id = ?)
				ON CONFLICT(user_id, workspace_id) DO UPDATE SET
					last_visited_at = CASE WHEN excluded.last_visited_at > user_workspace_visits.last_visited_at THEN excluded.last_visited_at ELSE user_workspace_visits.last_visited_at END,
					visit_count = user_workspace_visits.visit_count + excluded.visit_count,
					expires_at = ?,
					updated_at = CURRENT_TIMESTAMP
			`, visit.UserID, visit.WorkspaceID, visit.VisitedAt, visit.VisitCount, expiresAt,
				visit.UserID, visit.WorkspaceID,
				expiresAt); err != nil {
				return fmt.Errorf("flush workspace visit: %w", err)
			}
		}
		return nil
	}(); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit workspace visits: %w", err)
	}

	users := make(map[int]struct{})
	at.pendingMu.Lock()
	for _, visit := range visits {
		users[visit.UserID] = struct{}{}
		key := fmt.Sprintf("%d:%d", visit.UserID, visit.WorkspaceID)
		if existing, ok := at.pendingWorkspaceVisits[key]; ok {
			if !existing.VisitedAt.After(visit.VisitedAt) {
				delete(at.pendingWorkspaceVisits, key)
			}
		}
	}
	at.invalidateFlushedUsers(users)
	at.pendingMu.Unlock()

	atomic.AddInt64(&at.flushes, 1)
	return nil
}

// flushItemActivityBatch persists a batch of item activities to the database.
// Called by WriteBatcher every 30s or when 100 items are queued.
func (at *ActivityTracker) flushItemActivityBatch(ctx context.Context, activities []ItemActivity) error {
	expiresAt := time.Now().AddDate(0, 0, at.config.RetentionDays)

	tx, err := at.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin item activity transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := func() error {
		for _, activity := range activities {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO user_item_activities (user_id, item_id, activity_type, last_activity_at, activity_count, expires_at)
				SELECT ?, ?, ?, ?, ?, ?
				WHERE EXISTS (SELECT 1 FROM users WHERE id = ?)
				  AND EXISTS (SELECT 1 FROM items WHERE id = ?)
				ON CONFLICT(user_id, item_id, activity_type) DO UPDATE SET
					last_activity_at = CASE WHEN excluded.last_activity_at > user_item_activities.last_activity_at THEN excluded.last_activity_at ELSE user_item_activities.last_activity_at END,
					activity_count = user_item_activities.activity_count + excluded.activity_count,
					expires_at = ?,
					updated_at = CURRENT_TIMESTAMP
			`, activity.UserID, activity.ItemID, activity.ActivityType, activity.ActivityAt, activity.ActivityCount, expiresAt,
				activity.UserID, activity.ItemID,
				expiresAt); err != nil {
				return fmt.Errorf("flush item activity: %w", err)
			}
		}
		return nil
	}(); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit item activities: %w", err)
	}

	users := make(map[int]struct{})
	at.pendingMu.Lock()
	for _, activity := range activities {
		users[activity.UserID] = struct{}{}
		key := fmt.Sprintf("%d:%d:%s", activity.UserID, activity.ItemID, activity.ActivityType)
		if existing, ok := at.pendingItemActivities[key]; ok {
			if !existing.ActivityAt.After(activity.ActivityAt) {
				delete(at.pendingItemActivities, key)
			}
		}
	}
	at.invalidateFlushedUsers(users)
	at.pendingMu.Unlock()

	atomic.AddInt64(&at.flushes, 1)
	return nil
}

// Retire stale cache snapshots together with flushed shadow entries.
// The caller holds pendingMu.
func (at *ActivityTracker) invalidateFlushedUsers(users map[int]struct{}) {
	at.flushRevision++
	for userID := range users {
		_ = at.InvalidateUserCache(userID)
	}
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

	// Workspace visits: keep only the N most-recent per user.
	_, err = at.db.ExecWrite(`
		DELETE FROM user_workspace_visits
		WHERE id IN (
			SELECT id FROM (
				SELECT id,
				       ROW_NUMBER() OVER (PARTITION BY user_id ORDER BY last_visited_at DESC) AS rn
				FROM user_workspace_visits
			) ranked
			WHERE rn > ?
		)
	`, at.config.MaxWorkspaceVisits)
	if err != nil {
		slog.Error("Error enforcing workspace visit limits", slog.String("component", "activity"), slog.Any("error", err))
	}

	// Item activities: keep only the N most-recent per user per activity type.
	for _, activityType := range []ActivityType{ActivityView, ActivityEdit, ActivityComment} {
		_, err = at.db.ExecWrite(`
			DELETE FROM user_item_activities
			WHERE id IN (
				SELECT id FROM (
					SELECT id,
					       ROW_NUMBER() OVER (PARTITION BY user_id ORDER BY last_activity_at DESC) AS rn
					FROM user_item_activities
					WHERE activity_type = ?
				) ranked
				WHERE rn > ?
			)
		`, activityType, at.config.MaxItemActivities)
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
	errCount := atomic.LoadInt64(&at.errors)
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
	visitStats := at.visitBatcher.Stats()
	activityStats := at.activityBatcher.Stats()

	return ActivityTrackerStats{
		Hits:                   hits,
		Misses:                 misses,
		Errors:                 errCount,
		Flushes:                flushes,
		HitRatio:               hitRatio,
		PendingWorkspaceVisits: pendingWorkspaceVisits,
		PendingItemActivities:  pendingItemActivities,
		QueuedWrites:           visitStats.Pending + activityStats.Pending,
		DroppedWrites:          visitStats.ItemsDropped + activityStats.ItemsDropped,
		ExpiredWrites:          visitStats.ItemsExpired + activityStats.ItemsExpired,
		CoalescedWrites:        visitStats.ItemsCoalesced + activityStats.ItemsCoalesced,
		RetryCount:             visitStats.RetryCount + activityStats.RetryCount,
		OldestQueueAge:         max(visitStats.OldestAge, activityStats.OldestAge),
		MaxFlushDuration:       max(visitStats.MaxFlushDuration, activityStats.MaxFlushDuration),
	}
}

// ActivityTrackerStats represents cache statistics
type ActivityTrackerStats struct {
	Hits                   int64         `json:"hits"`
	Misses                 int64         `json:"misses"`
	Errors                 int64         `json:"errors"`
	Flushes                int64         `json:"flushes"`
	HitRatio               float64       `json:"hit_ratio"`
	PendingWorkspaceVisits int           `json:"pending_workspace_visits"`
	PendingItemActivities  int           `json:"pending_item_activities"`
	QueuedWrites           int           `json:"queued_writes"`
	DroppedWrites          int64         `json:"dropped_writes"`
	ExpiredWrites          int64         `json:"expired_writes"`
	CoalescedWrites        int64         `json:"coalesced_writes"`
	RetryCount             int64         `json:"retry_count"`
	OldestQueueAge         time.Duration `json:"oldest_queue_age"`
	MaxFlushDuration       time.Duration `json:"max_flush_duration"`
}

// Close gracefully shuts down the activity tracker
func (at *ActivityTracker) Close() error {
	slog.Debug("Closing ActivityTracker", slog.String("component", "activity"))

	// Stop write batchers under one deadline so database failure cannot hang
	// server shutdown indefinitely.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	visitErr := at.visitBatcher.StopContext(ctx)
	activityErr := at.activityBatcher.StopContext(ctx)
	slog.Debug("Write batchers stopped", slog.String("component", "activity"))

	// Close cache
	err := at.cache.Close()
	slog.Debug("ActivityTracker cache closed", slog.String("component", "activity"))
	return errors.Join(visitErr, activityErr, err)
}
