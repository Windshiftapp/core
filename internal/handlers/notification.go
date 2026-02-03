package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/allegro/bigcache/v3"
	"windshift/internal/database"
	"windshift/internal/middleware"
	"windshift/internal/models"
)

// NotificationManager handles notification caching and persistence
type NotificationManager struct {
	cache      *bigcache.BigCache
	db         database.Database
	syncTicker *time.Ticker
	stopChan   chan struct{}
	mu         sync.RWMutex
}

// NotificationService interface for cache management
type NotificationService interface {
	ForceRefreshCache() error
}

// NotificationHandler handles HTTP requests for notifications
type NotificationHandler struct {
	manager *NotificationManager
	service NotificationService
}

// NewNotificationManager creates a new notification manager with BigCache
func NewNotificationManager(db database.Database) (*NotificationManager, error) {
	config := bigcache.Config{
		Shards:             1024,
		LifeWindow:         24 * time.Hour, // Keep notifications for 24 hours in cache
		CleanWindow:        5 * time.Minute,
		MaxEntriesInWindow: 1000 * 10 * 60, // 10 minutes * 1000 entries per minute
		MaxEntrySize:       1024,            // 1KB per entry
		Verbose:            false,
		HardMaxCacheSize:   512, // 512 MB
		OnRemove:           nil,
	}

	cache, err := bigcache.New(context.Background(), config)
	if err != nil {
		return nil, fmt.Errorf("failed to create BigCache: %w", err)
	}

	manager := &NotificationManager{
		cache:      cache,
		db:         db,
		syncTicker: time.NewTicker(10 * time.Minute), // Sync every 10 minutes
		stopChan:   make(chan struct{}),
	}

	// Start periodic sync goroutine
	go manager.periodicSync()

	return manager, nil
}

// NewNotificationHandler creates a new notification handler
func NewNotificationHandler(manager *NotificationManager, service NotificationService) *NotificationHandler {
	return &NotificationHandler{
		manager: manager,
		service: service,
	}
}

// getCacheKey generates a cache key for a user's notifications
func (nm *NotificationManager) getCacheKey(userID int) string {
	return fmt.Sprintf("user:%d:notifications", userID)
}

// GetUserNotifications retrieves notifications for a user (cache-first)
func (nm *NotificationManager) GetUserNotifications(userID int, limit int, offset int) ([]models.Notification, error) {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	cacheKey := nm.getCacheKey(userID)
	
	// Try cache first
	if entry, err := nm.cache.Get(cacheKey); err == nil {
		var cache models.NotificationCache
		if err := json.Unmarshal(entry, &cache); err == nil {
			// Apply pagination
			start := offset
			end := offset + limit
			if start > len(cache.Notifications) {
				return []models.Notification{}, nil
			}
			if end > len(cache.Notifications) {
				end = len(cache.Notifications)
			}
			return cache.Notifications[start:end], nil
		}
	}

	// Cache miss, load from database
	return nm.loadNotificationsFromDB(userID, limit, offset)
}

// AddNotification adds a new notification (writes to cache immediately)
func (nm *NotificationManager) AddNotification(notification models.Notification) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	slog.Debug("adding notification", slog.String("component", "notifications"), slog.Int("user_id", notification.UserID), slog.String("type", notification.Type), slog.String("title", notification.Title))

	cacheKey := nm.getCacheKey(notification.UserID)
	
	// Get existing cache or create new
	var cache models.NotificationCache
	if entry, err := nm.cache.Get(cacheKey); err == nil {
		json.Unmarshal(entry, &cache)
	} else {
		cache = models.NotificationCache{
			Notifications: []models.Notification{},
			LastSynced:    time.Now().Add(-time.Hour), // Force sync on first write
			IsDirty:       true,
		}
	}

	// Add notification to beginning of slice (newest first)
	notification.ID = int(time.Now().UnixNano()) // Temporary ID for cache
	notification.CreatedAt = time.Now()
	notification.UpdatedAt = time.Now()
	
	cache.Notifications = append([]models.Notification{notification}, cache.Notifications...)
	cache.IsDirty = true

	// Keep only last 1000 notifications in cache
	if len(cache.Notifications) > 1000 {
		cache.Notifications = cache.Notifications[:1000]
	}

	// Update cache
	cacheData, _ := json.Marshal(cache)
	err := nm.cache.Set(cacheKey, cacheData)
	if err != nil {
		slog.Error("failed to set cache", slog.String("component", "notifications"), slog.Int("user_id", notification.UserID), slog.Any("error", err))
		return err
	}

	slog.Debug("successfully added notification", slog.String("component", "notifications"), slog.Int("user_id", notification.UserID), slog.Int("cache_size", len(cache.Notifications)))
	return nil
}

// MarkAsRead marks a notification as read
func (nm *NotificationManager) MarkAsRead(userID int, notificationID int) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	cacheKey := nm.getCacheKey(userID)
	
	// Get existing cache
	var cache models.NotificationCache
	if entry, err := nm.cache.Get(cacheKey); err == nil {
		if err := json.Unmarshal(entry, &cache); err != nil {
			return err
		}
	} else {
		// Load from database if not in cache
		notifications, err := nm.loadNotificationsFromDB(userID, 1000, 0)
		if err != nil {
			return err
		}
		cache = models.NotificationCache{
			Notifications: notifications,
			LastSynced:    time.Now(),
			IsDirty:       false,
		}
	}

	// Find and update notification
	for i := range cache.Notifications {
		if cache.Notifications[i].ID == notificationID {
			cache.Notifications[i].Read = true
			cache.Notifications[i].UpdatedAt = time.Now()
			cache.IsDirty = true
			break
		}
	}

	// Update cache
	cacheData, _ := json.Marshal(cache)
	return nm.cache.Set(cacheKey, cacheData)
}

// loadNotificationsFromDB loads notifications from database and updates cache
func (nm *NotificationManager) loadNotificationsFromDB(userID int, limit int, offset int) ([]models.Notification, error) {
	query := `
		SELECT id, user_id, title, message, type, timestamp, read, avatar, action_url, metadata, created_at, updated_at
		FROM notifications 
		WHERE user_id = ? 
		ORDER BY timestamp DESC 
		LIMIT ? OFFSET ?
	`
	
	rows, err := nm.db.Query(query, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notifications []models.Notification
	for rows.Next() {
		var n models.Notification
		var avatar, actionURL, metadata *string
		
		err := rows.Scan(
			&n.ID, &n.UserID, &n.Title, &n.Message, &n.Type,
			&n.Timestamp, &n.Read, &avatar, &actionURL, &metadata,
			&n.CreatedAt, &n.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		if avatar != nil {
			n.Avatar = *avatar
		}
		if actionURL != nil {
			n.ActionURL = *actionURL
		}
		if metadata != nil {
			n.Metadata = *metadata
		}

		notifications = append(notifications, n)
	}

	// Update cache with loaded data (only if we got the full first page)
	if offset == 0 && len(notifications) > 0 {
		cache := models.NotificationCache{
			Notifications: notifications,
			LastSynced:    time.Now(),
			IsDirty:       false,
		}
		cacheData, _ := json.Marshal(cache)
		cacheKey := nm.getCacheKey(userID)
		nm.cache.Set(cacheKey, cacheData)
	}

	return notifications, nil
}

// periodicSync runs every 10 minutes to sync dirty cache entries to database
func (nm *NotificationManager) periodicSync() {
	for {
		select {
		case <-nm.syncTicker.C:
			nm.syncCacheToDatabase()
		case <-nm.stopChan:
			return
		}
	}
}

// syncCacheToDatabase syncs all dirty cache entries to the database
func (nm *NotificationManager) syncCacheToDatabase() {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	slog.Debug("starting periodic notification sync to database", slog.String("component", "notifications"))
	
	// Get all cache keys (this is a simplified approach - in production you'd want to track dirty keys)
	iterator := nm.cache.Iterator()
	for iterator.SetNext() {
		info, err := iterator.Value()
		if err != nil {
			continue
		}

		var cache models.NotificationCache
		if err := json.Unmarshal(info.Value(), &cache); err != nil {
			continue
		}

		if !cache.IsDirty {
			continue
		}

		// Extract user ID from key
		key := info.Key()
		parts := strings.Split(key, ":")
		if len(parts) < 2 {
			continue
		}
		userID, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}

		// Sync notifications to database
		if err := nm.syncUserNotifications(userID, cache.Notifications); err != nil {
			slog.Error("failed to sync notifications", slog.String("component", "notifications"), slog.Int("user_id", userID), slog.Any("error", err))
			continue
		}

		// Mark cache as clean
		cache.IsDirty = false
		cache.LastSynced = time.Now()
		cacheData, _ := json.Marshal(cache)
		nm.cache.Set(key, cacheData)
	}

	slog.Debug("completed periodic notification sync to database", slog.String("component", "notifications"))
}

// syncUserNotifications syncs a user's notifications to the database
func (nm *NotificationManager) syncUserNotifications(userID int, notifications []models.Notification) error {
	tx, err := nm.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Prepare statements
	insertStmt, err := tx.Prepare(`
		INSERT INTO notifications (user_id, title, message, type, timestamp, read, avatar, action_url, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		read = excluded.read,
		updated_at = excluded.updated_at
	`)
	if err != nil {
		return err
	}
	defer insertStmt.Close()

	for _, notification := range notifications {
		// Skip temporary IDs (created in cache)
		if notification.ID > int(time.Now().Unix()) {
			// This is a temporary ID, do an insert
			_, err := insertStmt.Exec(
				notification.UserID, notification.Title, notification.Message,
				notification.Type, notification.Timestamp, notification.Read,
				nullableString(notification.Avatar), nullableString(notification.ActionURL),
				nullableString(notification.Metadata), notification.CreatedAt, notification.UpdatedAt,
			)
			if err != nil {
				return err
			}
		} else {
			// This is a real ID, do an update
			_, err := tx.Exec(`
				UPDATE notifications 
				SET read = ?, updated_at = ?
				WHERE id = ? AND user_id = ?
			`, notification.Read, notification.UpdatedAt, notification.ID, notification.UserID)
			if err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}


// Stop stops the notification manager
func (nm *NotificationManager) Stop() {
	nm.syncTicker.Stop()
	close(nm.stopChan)
	nm.syncCacheToDatabase() // Final sync
	nm.cache.Close()
}

// HTTP Handlers

// getUserFromContext extracts the authenticated user from request context
func (nh *NotificationHandler) getUserFromContext(r *http.Request) *models.User {
	if user := r.Context().Value(middleware.ContextKeyUser); user != nil {
		if u, ok := user.(*models.User); ok {
			return u
		}
	}
	return nil
}

// GetNotifications handles GET /api/notifications
func (nh *NotificationHandler) GetNotifications(w http.ResponseWriter, r *http.Request) {
	// Get authenticated user from context
	user := nh.getUserFromContext(r)
	if user == nil {
		slog.Debug("no authenticated user in context", slog.String("component", "notifications"))
		respondUnauthorized(w, r)
		return
	}
	userID := user.ID
	slog.Debug("loading notifications", slog.String("component", "notifications"), slog.Int("user_id", userID), slog.String("username", user.Username))

	// Parse pagination parameters
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit := 50 // Default limit
	offset := 0 // Default offset

	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	if offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	notifications, err := nh.manager.GetUserNotifications(userID, limit, offset)
	if err != nil {
		slog.Error("failed to get notifications", slog.String("component", "notifications"), slog.Int("user_id", userID), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	slog.Debug("successfully retrieved notifications", slog.String("component", "notifications"), slog.Int("user_id", userID), slog.Int("count", len(notifications)), slog.Int("limit", limit), slog.Int("offset", offset))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(notifications)
}

// CreateNotification handles POST /api/notifications
func (nh *NotificationHandler) CreateNotification(w http.ResponseWriter, r *http.Request) {
	var notification models.Notification
	if err := json.NewDecoder(r.Body).Decode(&notification); err != nil {
		respondBadRequest(w, r, "Invalid JSON")
		return
	}

	// Set timestamp if not provided
	if notification.Timestamp.IsZero() {
		notification.Timestamp = time.Now()
	}

	if err := nh.manager.AddNotification(notification); err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(notification)
}

// MarkNotificationAsRead handles PATCH /api/notifications/{id}/read
func (nh *NotificationHandler) MarkNotificationAsRead(w http.ResponseWriter, r *http.Request) {
	// Get authenticated user from context
	user := nh.getUserFromContext(r)
	if user == nil {
		slog.Debug("no authenticated user in context", slog.String("component", "notifications"))
		respondUnauthorized(w, r)
		return
	}
	userID := user.ID

	idStr := r.PathValue("id")

	id, err := strconv.Atoi(idStr)
	if err != nil {
		respondInvalidID(w, r, "notification ID")
		return
	}

	slog.Debug("marking notification as read", slog.String("component", "notifications"), slog.Int("user_id", userID), slog.String("username", user.Username), slog.Int("notification_id", id))

	if err := nh.manager.MarkAsRead(userID, id); err != nil {
		slog.Error("failed to mark notification as read", slog.String("component", "notifications"), slog.Int("notification_id", id), slog.Int("user_id", userID), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	slog.Debug("successfully marked notification as read", slog.String("component", "notifications"), slog.Int("notification_id", id), slog.Int("user_id", userID))
	w.WriteHeader(http.StatusOK)
}

// RefreshCache handles POST /api/notifications/refresh-cache (admin only)
func (nh *NotificationHandler) RefreshCache(w http.ResponseWriter, r *http.Request) {
	slog.Debug("admin requested manual cache refresh", slog.String("component", "notifications"))

	if nh.service == nil {
		slog.Warn("notification service not available", slog.String("component", "notifications"))
		respondInternalError(w, r, fmt.Errorf("notification service not available"))
		return
	}

	if err := nh.service.ForceRefreshCache(); err != nil {
		slog.Error("failed to refresh cache", slog.String("component", "notifications"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	slog.Debug("cache refreshed successfully", slog.String("component", "notifications"))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Notification cache refreshed successfully",
	})
}