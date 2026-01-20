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

	"github.com/allegro/bigcache/v3"
	"windshift/internal/database"
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
	TTL                    time.Duration `json:"ttl"`                       // Cache TTL, default: 24h
	MaxCacheSize           int          `json:"max_cache_size"`             // Default: 128MB
	FlushInterval          time.Duration `json:"flush_interval"`            // Default: 5min
	MaxWorkspaceVisits     int          `json:"max_workspace_visits"`       // Default: 10
	MaxItemActivities      int          `json:"max_item_activities"`        // Default: 50 per type
	RetentionDays          int          `json:"retention_days"`             // Default: 90
	ImmediateFlushActivity bool         `json:"immediate_flush_activity"`   // Flush edits/comments immediately
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
	mu     sync.RWMutex

	// Pending activities (buffered for batch write)
	pendingWorkspaceVisits map[string]*WorkspaceVisit     // key: userID:workspaceID
	pendingItemActivities  map[string]*ItemActivity       // key: userID:itemID:activityType
	pendingMu              sync.RWMutex

	// Cache statistics
	hits   int64
	misses int64
	errors int64
	flushes int64

	// Flush ticker
	flushTicker *time.Ticker
	stopChan    chan struct{}
	wg          sync.WaitGroup
}

// WorkspaceVisit tracks a workspace visit
type WorkspaceVisit struct {
	UserID       int
	WorkspaceID  int
	VisitedAt    time.Time
	VisitCount   int
}

// ItemActivity tracks item activity
type ItemActivity struct {
	UserID       int
	ItemID       int
	ActivityType ActivityType
	ActivityAt   time.Time
	ActivityCount int
}

// UserActivityCache stores cached activity data for a user
type UserActivityCache struct {
	UserID            int                           `json:"user_id"`
	WorkspaceVisits   []WorkspaceVisit              `json:"workspace_visits"`    // Last 10
	ItemActivities    map[ActivityType][]ItemActivity `json:"item_activities"`   // Last 50 per type
	ItemWatches       []int                         `json:"item_watches"`        // All active watches
	CachedAt          time.Time                     `json:"cached_at"`
	ExpiresAt         time.Time                     `json:"expires_at"`
}

// NewActivityTracker creates a new activity tracker with caching
func NewActivityTracker(db database.Database, config ActivityTrackerConfig) (*ActivityTracker, error) {
	// Configure BigCache
	cacheConfig := bigcache.Config{
		Shards:             1024,
		LifeWindow:         config.TTL,
		CleanWindow:        5 * time.Minute,
		MaxEntriesInWindow: 1000 * 10 * 60, // 10 minutes * 1000 entries per minute
		MaxEntrySize:       16384,           // 16KB per entry (larger for activity data)
		Verbose:            false,
		HardMaxCacheSize:   config.MaxCacheSize,
		OnRemove:           nil,
	}

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
		flushTicker:            time.NewTicker(config.FlushInterval),
		stopChan:               make(chan struct{}),
	}

	// Start periodic flush goroutine
	tracker.wg.Add(1)
	go tracker.periodicFlush()

	slog.Debug("ActivityTracker initialized", slog.String("component", "activity"), slog.Duration("flush_interval", config.FlushInterval))

	return tracker, nil
}

// getCacheKey generates a cache key for a user's activities
func (at *ActivityTracker) getCacheKey(userID int) string {
	return fmt.Sprintf("activity:user:%d", userID)
}

// TrackWorkspaceVisit records a workspace visit
func (at *ActivityTracker) TrackWorkspaceVisit(userID, workspaceID int) error {
	key := fmt.Sprintf("%d:%d", userID, workspaceID)

	at.pendingMu.Lock()
	if visit, exists := at.pendingWorkspaceVisits[key]; exists {
		visit.VisitedAt = time.Now()
		visit.VisitCount++
	} else {
		at.pendingWorkspaceVisits[key] = &WorkspaceVisit{
			UserID:      userID,
			WorkspaceID: workspaceID,
			VisitedAt:   time.Now(),
			VisitCount:  1,
		}
	}
	at.pendingMu.Unlock()

	// Invalidate cache for this user
	_ = at.InvalidateUserCache(userID)

	return nil
}

// TrackItemActivity records an item activity (view/edit/comment)
func (at *ActivityTracker) TrackItemActivity(userID, itemID int, activityType ActivityType) error {
	key := fmt.Sprintf("%d:%d:%s", userID, itemID, activityType)

	at.pendingMu.Lock()
	if activity, exists := at.pendingItemActivities[key]; exists {
		activity.ActivityAt = time.Now()
		activity.ActivityCount++
	} else {
		at.pendingItemActivities[key] = &ItemActivity{
			UserID:       userID,
			ItemID:       itemID,
			ActivityType: activityType,
			ActivityAt:   time.Now(),
			ActivityCount: 1,
		}
	}
	at.pendingMu.Unlock()

	// Invalidate cache for this user
	_ = at.InvalidateUserCache(userID)

	// All activities are now served from pending buffer via mergePendingActivities()
	// No immediate flush needed - activities batch write every 5 minutes

	return nil
}

// AddWatch adds a watch for an item
func (at *ActivityTracker) AddWatch(userID, itemID int, reason string) error {
	_, err := at.db.Exec(`
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
	_, err := at.db.Exec(`
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
		UserID:         userID,
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
		if err := workspaceRows.Scan(&visit.WorkspaceID, &visit.VisitedAt, &visit.VisitCount); err == nil {
			cached.WorkspaceVisits = append(cached.WorkspaceVisits, visit)
		}
	}

	// Load item activities for each type (last 50 per type)
	for _, activityType := range []ActivityType{ActivityView, ActivityEdit, ActivityComment} {
		activityRows, err := at.db.Query(`
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
			if err := activityRows.Scan(&activity.ItemID, &activity.ActivityAt, &activity.ActivityCount); err == nil {
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

// periodicFlush runs periodic flush of pending activities
func (at *ActivityTracker) periodicFlush() {
	defer at.wg.Done()

	for {
		select {
		case <-at.flushTicker.C:
			if err := at.FlushPendingActivities(); err != nil {
				slog.Error("Error flushing pending activities", slog.String("component", "activity"), slog.Any("error", err))
			}
		case <-at.stopChan:
			slog.Debug("Stopping periodic flush", slog.String("component", "activity"))
			return
		}
	}
}

// FlushPendingActivities writes all pending activities to database
func (at *ActivityTracker) FlushPendingActivities() error {
	at.pendingMu.Lock()

	// Copy and clear pending activities
	workspaceVisits := at.pendingWorkspaceVisits
	itemActivities := at.pendingItemActivities
	at.pendingWorkspaceVisits = make(map[string]*WorkspaceVisit)
	at.pendingItemActivities = make(map[string]*ItemActivity)

	at.pendingMu.Unlock()

	if len(workspaceVisits) == 0 && len(itemActivities) == 0 {
		return nil
	}

	slog.Debug("Flushing activities to database", slog.String("component", "activity"), slog.Int("workspace_visits", len(workspaceVisits)), slog.Int("item_activities", len(itemActivities)))

	// Flush workspace visits
	for _, visit := range workspaceVisits {
		if err := at.flushWorkspaceVisitToDB(visit); err != nil {
			slog.Error("Error flushing workspace visit", slog.String("component", "activity"), slog.Any("error", err))
			atomic.AddInt64(&at.errors, 1)
		}
	}

	// Flush item activities
	for _, activity := range itemActivities {
		if err := at.flushItemActivityToDB(activity); err != nil {
			slog.Error("Error flushing item activity", slog.String("component", "activity"), slog.Any("error", err))
			atomic.AddInt64(&at.errors, 1)
		}
	}

	atomic.AddInt64(&at.flushes, 1)
	return nil
}

// flushWorkspaceVisitToDB writes a workspace visit to database
func (at *ActivityTracker) flushWorkspaceVisitToDB(visit *WorkspaceVisit) error {
	expiresAt := time.Now().AddDate(0, 0, at.config.RetentionDays)

	_, err := at.db.Exec(`
		INSERT INTO user_workspace_visits (user_id, workspace_id, last_visited_at, visit_count, expires_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_id, workspace_id) DO UPDATE SET
			last_visited_at = ?,
			visit_count = visit_count + ?,
			expires_at = ?,
			updated_at = CURRENT_TIMESTAMP
	`, visit.UserID, visit.WorkspaceID, visit.VisitedAt, visit.VisitCount, expiresAt,
		visit.VisitedAt, visit.VisitCount, expiresAt)

	return err
}

// flushItemActivityToDB writes an item activity to database
func (at *ActivityTracker) flushItemActivityToDB(activity *ItemActivity) error {
	expiresAt := time.Now().AddDate(0, 0, at.config.RetentionDays)

	_, err := at.db.Exec(`
		INSERT INTO user_item_activities (user_id, item_id, activity_type, last_activity_at, activity_count, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, item_id, activity_type) DO UPDATE SET
			last_activity_at = ?,
			activity_count = activity_count + ?,
			expires_at = ?,
			updated_at = CURRENT_TIMESTAMP
	`, activity.UserID, activity.ItemID, activity.ActivityType, activity.ActivityAt, activity.ActivityCount, expiresAt,
		activity.ActivityAt, activity.ActivityCount, expiresAt)

	return err
}

// flushItemActivity immediately flushes a specific item activity
func (at *ActivityTracker) flushItemActivity(key string) error {
	at.pendingMu.Lock()
	activity, exists := at.pendingItemActivities[key]
	if !exists {
		at.pendingMu.Unlock()
		return nil
	}
	delete(at.pendingItemActivities, key)
	at.pendingMu.Unlock()

	return at.flushItemActivityToDB(activity)
}

// CleanupExpiredActivities removes expired activity records
func (at *ActivityTracker) CleanupExpiredActivities() error {
	now := time.Now()

	// Clean up expired workspace visits
	result, err := at.db.Exec(`DELETE FROM user_workspace_visits WHERE expires_at < ?`, now)
	if err != nil {
		return fmt.Errorf("failed to clean up workspace visits: %w", err)
	}
	deleted, _ := result.RowsAffected()
	slog.Debug("Cleaned up expired workspace visits", slog.String("component", "activity"), slog.Int64("deleted", deleted))

	// Clean up expired item activities
	result, err = at.db.Exec(`DELETE FROM user_item_activities WHERE expires_at < ?`, now)
	if err != nil {
		return fmt.Errorf("failed to clean up item activities: %w", err)
	}
	deleted, _ = result.RowsAffected()
	slog.Debug("Cleaned up expired item activities", slog.String("component", "activity"), slog.Int64("deleted", deleted))

	// Also enforce count limits (keep only most recent N records per user)
	// This is a safety measure in case expiration isn't working properly

	// Workspace visits: keep only last 10 per user
	_, err = at.db.Exec(`
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
		_, err = at.db.Exec(`
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

	// Stop periodic flush
	close(at.stopChan)
	at.flushTicker.Stop()
	slog.Debug("Waiting for periodic flush goroutine", slog.String("component", "activity"))
	at.wg.Wait()
	slog.Debug("Periodic flush goroutine stopped", slog.String("component", "activity"))

	// Final flush of pending activities
	if err := at.FlushPendingActivities(); err != nil {
		slog.Error("Error during final flush", slog.String("component", "activity"), slog.Any("error", err))
	}
	slog.Debug("Final flush complete, closing cache", slog.String("component", "activity"))

	// Close cache
	err := at.cache.Close()
	slog.Debug("ActivityTracker cache closed", slog.String("component", "activity"))
	return err
}
