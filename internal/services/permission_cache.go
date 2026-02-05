package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/allegro/bigcache/v3"
	"windshift/internal/database"
	"windshift/internal/models"
)

// PermissionService handles cached permission resolution
type PermissionService struct {
	cache *bigcache.BigCache
	db    database.Database
	mu    sync.RWMutex

	// Cache statistics
	hits      int64
	misses    int64
	errors    int64
	loadTimes []int64 // For calculating average load time

	// Configuration
	ttl       time.Duration
	batchSize int

	// Static data cached at startup
	allPermissionKeys []string
}

// PermissionCacheConfig represents configuration for the permission cache
type PermissionCacheConfig struct {
	TTL             time.Duration `json:"ttl"`               // Default: 15min
	MaxCacheSize    int           `json:"max_cache_size"`    // Default: 256MB
	WarmupOnStartup bool          `json:"warmup_on_startup"` // Default: true
	PreWarmActive   bool          `json:"pre_warm_active"`   // Default: true
	BatchSize       int           `json:"batch_size"`        // Default: 100
}

// DefaultPermissionCacheConfig returns default configuration
func DefaultPermissionCacheConfig() PermissionCacheConfig {
	return PermissionCacheConfig{
		TTL:             15 * time.Minute,
		MaxCacheSize:    256, // 256MB
		WarmupOnStartup: true,
		PreWarmActive:   true,
		BatchSize:       100,
	}
}

// NewPermissionService creates a new permission service with caching
func NewPermissionService(db database.Database, config PermissionCacheConfig) (*PermissionService, error) {
	// Configure BigCache
	cacheConfig := bigcache.Config{
		Shards:             1024,
		LifeWindow:         config.TTL,
		CleanWindow:        5 * time.Minute,
		MaxEntriesInWindow: 1000 * 10 * 60, // 10 minutes * 1000 entries per minute
		MaxEntrySize:       8192,           // 8KB per entry (larger for permission data)
		Verbose:            false,
		HardMaxCacheSize:   config.MaxCacheSize, // Configurable MB
		OnRemove:           nil,
	}

	cache, err := bigcache.New(context.Background(), cacheConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create BigCache for permissions: %w", err)
	}

	service := &PermissionService{
		cache:     cache,
		db:        db,
		ttl:       config.TTL,
		batchSize: config.BatchSize,
		loadTimes: make([]int64, 0, 1000), // Track last 1000 load times
	}

	// Pre-load all permission keys (static data)
	if err := service.loadAllPermissionKeys(); err != nil {
		slog.Warn("Failed to pre-load permission keys; will lazy-load on first cache build",
			slog.String("component", "permissions"),
			slog.Any("error", err))
	}

	// Warm up cache if configured
	if config.WarmupOnStartup {
		go service.WarmCache()
	}

	return service, nil
}

// getCacheKey generates a cache key for a user's permissions
func (ps *PermissionService) getCacheKey(userID int) string {
	return fmt.Sprintf("permissions:user:%d", userID)
}

// HasWorkspacePermission checks if user has a specific workspace permission
// Returns true if:
// 1. User is system admin, OR
// 2. User has the specified permission on the workspace, OR
// 3. Workspace has NO permission restrictions (accessible to all logged-in users)
func (ps *PermissionService) HasWorkspacePermission(userID, workspaceID int, permission string) (bool, error) {
	startTime := time.Now()
	defer func() {
		loadTime := time.Since(startTime).Milliseconds()
		ps.mu.Lock()
		if len(ps.loadTimes) >= 1000 {
			ps.loadTimes = ps.loadTimes[1:]
		}
		ps.loadTimes = append(ps.loadTimes, loadTime)
		ps.mu.Unlock()
	}()

	// Try cache first
	cached, err := ps.getUserPermissionCache(userID)
	if err == nil {
		atomic.AddInt64(&ps.hits, 1)

		// Check if user is system admin
		if cached.IsSystemAdmin {
			return true, nil
		}

		// Check Everyone assignment first (fast path)
		if everyonePerms, exists := cached.WorkspaceEveryone[workspaceID]; exists {
			if everyonePerms[permission] {
				return true, nil
			}
		}

		// Check workspace-specific permissions
		if workspacePerms, exists := cached.WorkspacePermissions[workspaceID]; exists {
			hasIt := workspacePerms[permission]
			return hasIt, nil
		}
		// No matching permission
		return false, nil
	}

	// Cache miss - load from database
	atomic.AddInt64(&ps.misses, 1)
	return ps.loadUserPermissionAndCheck(userID, workspaceID, permission)
}

// HasGlobalPermission checks if user has a specific global permission
func (ps *PermissionService) HasGlobalPermission(userID int, permission string) (bool, error) {
	// Try cache first
	cached, err := ps.getUserPermissionCache(userID)
	if err == nil {
		atomic.AddInt64(&ps.hits, 1)

		// Check if user is system admin
		if cached.IsSystemAdmin {
			return true, nil
		}

		// Check global permissions
		return cached.GlobalPermissions[permission], nil
	}

	// Cache miss - load from database
	atomic.AddInt64(&ps.misses, 1)
	return ps.loadUserPermissionAndCheckGlobal(userID, permission)
}

// HasWorkspacePermissions checks multiple permissions in single operation
func (ps *PermissionService) HasWorkspacePermissions(userID, workspaceID int, permissions []string) (map[string]bool, error) {
	result := make(map[string]bool)

	// Try cache first
	cached, err := ps.getUserPermissionCache(userID)
	if err == nil {
		atomic.AddInt64(&ps.hits, 1)

		// Check if user is system admin
		if cached.IsSystemAdmin {
			for _, perm := range permissions {
				result[perm] = true
			}
			return result, nil
		}

		// Everyone assignment fast path
		if everyonePerms, exists := cached.WorkspaceEveryone[workspaceID]; exists {
			for _, perm := range permissions {
				if everyonePerms[perm] {
					result[perm] = true
				}
			}
		}

		// Check workspace-specific permissions - merge with Everyone (don't overwrite false)
		if workspacePerms, exists := cached.WorkspacePermissions[workspaceID]; exists {
			for _, perm := range permissions {
				if workspacePerms[perm] {
					result[perm] = true
				}
			}
		}
		return result, nil
	}

	// Cache miss - load from database and check all permissions
	atomic.AddInt64(&ps.misses, 1)
	return ps.loadUserPermissionAndCheckMultiple(userID, workspaceID, permissions)
}

// IsSystemAdmin checks if user is system administrator
func (ps *PermissionService) IsSystemAdmin(userID int) (bool, error) {
	// Try cache first
	cached, err := ps.getUserPermissionCache(userID)
	if err == nil {
		atomic.AddInt64(&ps.hits, 1)
		return cached.IsSystemAdmin, nil
	}

	// Cache miss - check database directly for system.admin permission
	atomic.AddInt64(&ps.misses, 1)
	var hasPermission bool
	err = ps.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM user_global_permissions ugp
			JOIN permissions p ON ugp.permission_id = p.id
			WHERE ugp.user_id = ? AND p.permission_key = 'system.admin'
		)
	`, userID).Scan(&hasPermission)
	if err != nil {
		atomic.AddInt64(&ps.errors, 1)
		return false, fmt.Errorf("error checking system admin permission: %v", err)
	}

	return hasPermission, nil
}

// GetItemWorkspaceID returns the workspace ID for a given item ID using lazy-loaded cache
// This method is thread-safe and will populate the cache on first access
func (ps *PermissionService) GetItemWorkspaceID(userID, itemID int) (int, error) {
	// Try to get from cache first
	cached, err := ps.getUserPermissionCache(userID)
	if err == nil {
		// Check if item workspace mapping exists in cache
		if workspaceID, exists := cached.ItemWorkspaceMap[itemID]; exists {
			atomic.AddInt64(&ps.hits, 1)
			return workspaceID, nil
		}
	}

	// Cache miss or item not in map - query database
	atomic.AddInt64(&ps.misses, 1)

	var workspaceID int
	err = ps.db.QueryRow(`SELECT workspace_id FROM items WHERE id = ?`, itemID).Scan(&workspaceID)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("item not found: %d", itemID)
		}
		atomic.AddInt64(&ps.errors, 1)
		return 0, fmt.Errorf("error querying item workspace: %v", err)
	}

	// Store in cache if we have a valid cached entry
	if cached != nil {
		ps.mu.Lock()
		cached.ItemWorkspaceMap[itemID] = workspaceID
		ps.mu.Unlock()

		// Update cache storage
		if err := ps.storeUserPermissionCache(userID, cached); err != nil {
			slog.Warn("Failed to update cache with item workspace mapping",
				slog.String("component", "permissions"),
				slog.Int("user_id", userID),
				slog.Int("item_id", itemID),
				slog.Any("error", err))
			// Don't fail the request, just log the error
		}
	}

	return workspaceID, nil
}

// getUserPermissionCache retrieves cached permission data for a user
func (ps *PermissionService) getUserPermissionCache(userID int) (*models.UserPermissionCache, error) {
	cacheKey := ps.getCacheKey(userID)

	entry, err := ps.cache.Get(cacheKey)
	if err != nil {
		return nil, err
	}

	var cached models.UserPermissionCache
	if err := json.Unmarshal(entry, &cached); err != nil {
		// Remove corrupted cache entry
		_ = ps.cache.Delete(cacheKey)
		return nil, err
	}

	// Check if cache entry has expired
	if time.Now().After(cached.ExpiresAt) {
		_ = ps.cache.Delete(cacheKey)
		return nil, fmt.Errorf("cache entry expired")
	}

	return &cached, nil
}

// loadUserPermissionAndCheck loads user permissions from DB and checks specific permission
func (ps *PermissionService) loadUserPermissionAndCheck(userID, workspaceID int, permission string) (bool, error) {
	cached, err := ps.buildUserPermissionCache(userID)
	if err != nil {
		atomic.AddInt64(&ps.errors, 1)
		return false, err
	}

	// Store in cache
	_ = ps.storeUserPermissionCache(userID, cached)

	// Check if user is system admin
	if cached.IsSystemAdmin {
		return true, nil
	}

	// Everyone assignment fast path
	if everyonePerms, exists := cached.WorkspaceEveryone[workspaceID]; exists {
		if everyonePerms[permission] {
			return true, nil
		}
	}

	// Check workspace-specific permissions
	if workspacePerms, exists := cached.WorkspacePermissions[workspaceID]; exists {
		return workspacePerms[permission], nil
	}

	return false, nil
}

// loadUserPermissionAndCheckGlobal loads user permissions from DB and checks global permission
func (ps *PermissionService) loadUserPermissionAndCheckGlobal(userID int, permission string) (bool, error) {
	cached, err := ps.buildUserPermissionCache(userID)
	if err != nil {
		atomic.AddInt64(&ps.errors, 1)
		return false, err
	}

	// Store in cache
	_ = ps.storeUserPermissionCache(userID, cached)

	// Check if user is system admin
	if cached.IsSystemAdmin {
		return true, nil
	}

	// Check global permissions
	return cached.GlobalPermissions[permission], nil
}

// loadUserPermissionAndCheckMultiple loads user permissions and checks multiple permissions
func (ps *PermissionService) loadUserPermissionAndCheckMultiple(userID, workspaceID int, permissions []string) (map[string]bool, error) {
	result := make(map[string]bool)

	cached, err := ps.buildUserPermissionCache(userID)
	if err != nil {
		atomic.AddInt64(&ps.errors, 1)
		return result, err
	}

	// Store in cache
	_ = ps.storeUserPermissionCache(userID, cached)

	// Check if user is system admin
	if cached.IsSystemAdmin {
		for _, perm := range permissions {
			result[perm] = true
		}
		return result, nil
	}

	// Everyone assignment fast path
	if everyonePerms, exists := cached.WorkspaceEveryone[workspaceID]; exists {
		for _, perm := range permissions {
			if everyonePerms[perm] {
				result[perm] = true
			}
		}
	}

	// Check workspace-specific permissions
	if workspacePerms, exists := cached.WorkspacePermissions[workspaceID]; exists {
		for _, perm := range permissions {
			result[perm] = workspacePerms[perm]
		}
	}

	return result, nil
}

// InvalidateUserCache removes a user's permission cache
func (ps *PermissionService) InvalidateUserCache(userID int) error {
	cacheKey := ps.getCacheKey(userID)
	return ps.cache.Delete(cacheKey)
}

// InvalidateMultipleUserCaches removes permission caches for multiple users
func (ps *PermissionService) InvalidateMultipleUserCaches(userIDs []int) error {
	for _, userID := range userIDs {
		if err := ps.InvalidateUserCache(userID); err != nil {
			slog.Warn("Failed to invalidate cache for user",
				slog.String("component", "permissions"),
				slog.Int("user_id", userID),
				slog.Any("error", err))
		}
	}
	return nil
}

// InvalidateGroupMemberCaches invalidates caches for all members of a group
func (ps *PermissionService) InvalidateGroupMemberCaches(groupID int) error {
	// Get all group members
	userIDs, err := ps.getGroupMembers(groupID)
	if err != nil {
		return fmt.Errorf("error getting group members for cache invalidation: %v", err)
	}

	return ps.InvalidateMultipleUserCaches(userIDs)
}

// InvalidateWorkspaceMemberCaches invalidates caches for all members of a workspace
func (ps *PermissionService) InvalidateWorkspaceMemberCaches(workspaceID int) error {
	// Get all workspace members via role assignments (both direct and group-based)
	rows, err := ps.db.Query(`
		SELECT DISTINCT user_id FROM user_workspace_roles WHERE workspace_id = ?
		UNION
		SELECT DISTINCT gm.user_id FROM group_members gm
		JOIN group_workspace_roles gwr ON gm.group_id = gwr.group_id
		WHERE gwr.workspace_id = ?
	`, workspaceID, workspaceID)
	if err != nil {
		return fmt.Errorf("error getting workspace members for cache invalidation: %v", err)
	}
	defer rows.Close()

	var userIDs []int
	for rows.Next() {
		var userID int
		if err := rows.Scan(&userID); err == nil {
			userIDs = append(userIDs, userID)
		}
	}

	return ps.InvalidateMultipleUserCaches(userIDs)
}

// getGroupMembers returns all user IDs in a group
func (ps *PermissionService) getGroupMembers(groupID int) ([]int, error) {
	rows, err := ps.db.Query(`
		SELECT user_id FROM user_groups WHERE group_id = ?
	`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var userIDs []int
	for rows.Next() {
		var userID int
		if err := rows.Scan(&userID); err == nil {
			userIDs = append(userIDs, userID)
		}
	}

	return userIDs, nil
}

// OnUserPermissionChanged should be called when user permissions are modified
func (ps *PermissionService) OnUserPermissionChanged(userID int) error {
	// Invalidate user cache - next request will populate fresh from DB
	if err := ps.InvalidateUserCache(userID); err != nil {
		slog.Warn("Failed to invalidate cache for user after permission change",
			slog.String("component", "permissions"),
			slog.Int("user_id", userID),
			slog.Any("error", err))
	}
	return nil
}

// OnGroupPermissionChanged should be called when group permissions are modified
func (ps *PermissionService) OnGroupPermissionChanged(groupID int) error {
	// Invalidate caches for all group members - next request will populate fresh from DB
	if err := ps.InvalidateGroupMemberCaches(groupID); err != nil {
		slog.Warn("Failed to invalidate group member caches",
			slog.String("component", "permissions"),
			slog.Int("group_id", groupID),
			slog.Any("error", err))
	}
	return nil
}

// OnUserGroupMembershipChanged should be called when user is added/removed from group
func (ps *PermissionService) OnUserGroupMembershipChanged(userID, groupID int) error {
	return ps.OnUserPermissionChanged(userID)
}

// OnWorkspacePermissionChanged should be called when workspace-level permissions change
func (ps *PermissionService) OnWorkspacePermissionChanged(workspaceID int) error {
	// Invalidate caches for all workspace members
	if err := ps.InvalidateWorkspaceMemberCaches(workspaceID); err != nil {
		slog.Warn("Failed to invalidate workspace member caches",
			slog.String("component", "permissions"),
			slog.Int("workspace_id", workspaceID),
			slog.Any("error", err))
	}

	return nil
}

// OnRoleChanged should be called when a role's permissions are modified
func (ps *PermissionService) OnRoleChanged(roleID int) error {
	// Get all users with this role in any workspace
	userIDs, err := ps.getUsersWithRole(roleID)
	if err != nil {
		slog.Error("Failed to get users with role",
			slog.String("component", "permissions"),
			slog.Int("role_id", roleID),
			slog.Any("error", err))
		return err
	}

	// Invalidate caches for all affected users
	if err := ps.InvalidateMultipleUserCaches(userIDs); err != nil {
		slog.Warn("Failed to invalidate user caches for role",
			slog.String("component", "permissions"),
			slog.Int("role_id", roleID),
			slog.Any("error", err))
	}

	// Also invalidate caches for users in groups with this role
	groupUserIDs, err := ps.getUsersInGroupsWithRole(roleID)
	if err != nil {
		slog.Warn("Failed to get users in groups with role",
			slog.String("component", "permissions"),
			slog.Int("role_id", roleID),
			slog.Any("error", err))
	} else if len(groupUserIDs) > 0 {
		if err := ps.InvalidateMultipleUserCaches(groupUserIDs); err != nil {
			slog.Warn("Failed to invalidate group user caches for role",
				slog.String("component", "permissions"),
				slog.Int("role_id", roleID),
				slog.Any("error", err))
		}
	}

	return nil
}

// OnPermissionSetChanged should be called when a permission set's permissions are modified
func (ps *PermissionService) OnPermissionSetChanged(permissionSetID int) error {
	// Get all configuration sets using this permission set
	configSetIDs, err := ps.getConfigurationSetsUsingPermissionSet(permissionSetID)
	if err != nil {
		slog.Error("Failed to get configuration sets using permission set",
			slog.String("component", "permissions"),
			slog.Int("permission_set_id", permissionSetID),
			slog.Any("error", err))
		return err
	}

	// For each configuration set, invalidate all workspace members
	for _, configSetID := range configSetIDs {
		workspaceIDs, err := ps.getWorkspacesUsingConfigurationSet(configSetID)
		if err != nil {
			slog.Warn("Failed to get workspaces for configuration set",
				slog.String("component", "permissions"),
				slog.Int("configuration_set_id", configSetID),
				slog.Any("error", err))
			continue
		}

		for _, workspaceID := range workspaceIDs {
			if err := ps.InvalidateWorkspaceMemberCaches(workspaceID); err != nil {
				slog.Warn("Failed to invalidate workspace member caches",
					slog.String("component", "permissions"),
					slog.Int("workspace_id", workspaceID),
					slog.Any("error", err))
			}
		}
	}

	return nil
}

// OnEveryoneRoleChanged clears permission caches when the Everyone assignment changes.
// This is a broad reset because the assignment affects all users.
func (ps *PermissionService) OnEveryoneRoleChanged() {
	if ps.cache != nil {
		if err := ps.cache.Reset(); err != nil {
			slog.Error("Failed to reset permission cache after everyone-role change",
				slog.String("component", "permissions"),
				slog.Any("error", err))
		}
	}
}

// OnConfigurationSetChanged should be called when a configuration set is modified or reassigned
func (ps *PermissionService) OnConfigurationSetChanged(configurationSetID int) error {
	// Get all workspaces using this configuration set
	workspaceIDs, err := ps.getWorkspacesUsingConfigurationSet(configurationSetID)
	if err != nil {
		slog.Error("Failed to get workspaces for configuration set",
			slog.String("component", "permissions"),
			slog.Int("configuration_set_id", configurationSetID),
			slog.Any("error", err))
		return err
	}

	// Invalidate caches for all members of affected workspaces
	for _, workspaceID := range workspaceIDs {
		if err := ps.InvalidateWorkspaceMemberCaches(workspaceID); err != nil {
			slog.Warn("Failed to invalidate workspace member caches",
				slog.String("component", "permissions"),
				slog.Int("workspace_id", workspaceID),
				slog.Any("error", err))
		}
	}

	return nil
}

// Helper functions for cache invalidation

// getUsersWithRole returns all user IDs that have been assigned a specific role
func (ps *PermissionService) getUsersWithRole(roleID int) ([]int, error) {
	rows, err := ps.db.Query(`
		SELECT DISTINCT user_id
		FROM user_workspace_roles
		WHERE role_id = ?
	`, roleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var userIDs []int
	for rows.Next() {
		var userID int
		if err := rows.Scan(&userID); err == nil {
			userIDs = append(userIDs, userID)
		}
	}

	return userIDs, nil
}

// getUsersInGroupsWithRole returns all user IDs in groups that have been assigned a specific role
func (ps *PermissionService) getUsersInGroupsWithRole(roleID int) ([]int, error) {
	rows, err := ps.db.Query(`
		SELECT DISTINCT gm.user_id
		FROM group_workspace_roles gwr
		JOIN group_members gm ON gwr.group_id = gm.group_id
		WHERE gwr.role_id = ?
	`, roleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var userIDs []int
	for rows.Next() {
		var userID int
		if err := rows.Scan(&userID); err == nil {
			userIDs = append(userIDs, userID)
		}
	}

	return userIDs, nil
}

// getConfigurationSetsUsingPermissionSet returns all configuration set IDs using a specific permission set
func (ps *PermissionService) getConfigurationSetsUsingPermissionSet(permissionSetID int) ([]int, error) {
	rows, err := ps.db.Query(`
		SELECT id
		FROM configuration_sets
		WHERE permission_set_id = ?
	`, permissionSetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configSetIDs []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err == nil {
			configSetIDs = append(configSetIDs, id)
		}
	}

	return configSetIDs, nil
}

// getWorkspacesUsingConfigurationSet returns all workspace IDs using a specific configuration set
func (ps *PermissionService) getWorkspacesUsingConfigurationSet(configSetID int) ([]int, error) {
	rows, err := ps.db.Query(`
		SELECT workspace_id
		FROM workspace_configuration_sets
		WHERE configuration_set_id = ?
	`, configSetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workspaceIDs []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err == nil {
			workspaceIDs = append(workspaceIDs, id)
		}
	}

	return workspaceIDs, nil
}

// GetCacheStats returns current cache performance statistics
func (ps *PermissionService) GetCacheStats() models.CacheStats {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	hits := atomic.LoadInt64(&ps.hits)
	misses := atomic.LoadInt64(&ps.misses)
	errors := atomic.LoadInt64(&ps.errors)
	total := hits + misses

	hitRatio := 0.0
	if total > 0 {
		hitRatio = float64(hits) / float64(total)
	}

	// Calculate average load time
	avgLoadTime := int64(0)
	if len(ps.loadTimes) > 0 {
		sum := int64(0)
		for _, t := range ps.loadTimes {
			sum += t
		}
		avgLoadTime = sum / int64(len(ps.loadTimes))
	}

	// Get cache info - BigCache Stats doesn't have Entries field
	// We'll track total users differently or estimate it
	totalUsers := int64(0) // For now, we don't track this precisely

	return models.CacheStats{
		Hits:        hits,
		Misses:      misses,
		Errors:      errors,
		HitRatio:    hitRatio,
		AvgLoadTime: avgLoadTime,
		TotalUsers:  totalUsers,
	}
}

// buildUserPermissionCache loads complete permission profile from database
func (ps *PermissionService) buildUserPermissionCache(userID int) (*models.UserPermissionCache, error) {
	now := time.Now()

	cached := &models.UserPermissionCache{
		UserID:               userID,
		IsSystemAdmin:        false,
		GlobalPermissions:    make(map[string]bool),
		WorkspacePermissions: make(map[int]map[string]bool),
		WorkspaceEveryone:    make(map[int]map[string]bool),
		GroupMemberships:     make([]int, 0),
		RoleAssignments:      make(map[int][]int),
		DirectPermissions:    make(map[int][]string),
		PermissionSources:    make(map[int]map[string]string),
		ItemWorkspaceMap:     make(map[int]int),
		CachedAt:             now,
		ExpiresAt:            now.Add(ps.ttl),
	}

	// Check if user has system.admin permission
	var hasSystemAdmin bool
	err := ps.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM user_global_permissions ugp
			JOIN permissions p ON ugp.permission_id = p.id
			WHERE ugp.user_id = ? AND p.permission_key = 'system.admin'
		)
	`, userID).Scan(&hasSystemAdmin)
	if err != nil {
		if err == sql.ErrNoRows {
			return cached, nil // User not found, return empty permissions
		}
		return nil, fmt.Errorf("error checking system admin permission: %v", err)
	}
	cached.IsSystemAdmin = hasSystemAdmin

	// If system admin, no need to load specific permissions
	if cached.IsSystemAdmin {
		return cached, nil
	}

	// Cache for role permissions (lazy-loaded per role ID)
	rolePermissionCache := make(map[int]map[string]bool)

	// Load workspace active flags once
	activeWorkspaces, err := ps.getWorkspaceActiveMap()
	if err != nil {
		return nil, fmt.Errorf("error loading workspace states: %v", err)
	}

	// Load explicit Everyone role assignments (workspace access must be explicitly granted)
	everyoneRows, err := ps.db.Query(`
		SELECT workspace_id, role_id FROM workspace_everyone_roles
	`)
	if err == nil {
		defer func() { _ = everyoneRows.Close() }()
		for everyoneRows.Next() {
			var workspaceID int
			var roleID sql.NullInt64
			if err := everyoneRows.Scan(&workspaceID, &roleID); err != nil {
				continue
			}

			// Skip inactive workspaces (remain restricted)
			if active, ok := activeWorkspaces[workspaceID]; ok && !active {
				continue
			}

			// NULL role_id means "Everyone has no access" (lock down)
			if !roleID.Valid {
				cached.WorkspaceEveryone[workspaceID] = map[string]bool{}
				continue
			}

			// Resolve permissions for the assigned role (cached per role_id)
			perms, ok := rolePermissionCache[int(roleID.Int64)]
			if !ok {
				perms, err = ps.getRolePermissions(int(roleID.Int64))
				if err != nil {
					continue
				}
				rolePermissionCache[int(roleID.Int64)] = perms
			}
			cached.WorkspaceEveryone[workspaceID] = clonePermissionSet(perms)
		}
	}

	// Load global permissions
	globalRows, err := ps.db.Query(`
		SELECT p.permission_key
		FROM user_global_permissions ugp
		JOIN permissions p ON ugp.permission_id = p.id
		WHERE ugp.user_id = ?
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("error loading global permissions: %v", err)
	}
	defer func() { _ = globalRows.Close() }()

	for globalRows.Next() {
		var permissionKey string
		if err := globalRows.Scan(&permissionKey); err != nil {
			continue
		}
		cached.GlobalPermissions[permissionKey] = true
	}

	// Load group memberships
	groupRows, err := ps.db.Query(`
		SELECT group_id FROM group_members WHERE user_id = ?
	`, userID)
	if err == nil {
		defer func() { _ = groupRows.Close() }()
		for groupRows.Next() {
			var groupID int
			if err := groupRows.Scan(&groupID); err == nil {
				cached.GroupMemberships = append(cached.GroupMemberships, groupID)
			}
		}
	}

	// Load user's role assignments and derive permissions from roles
	roleRows, err := ps.db.Query(`
		SELECT uwr.workspace_id, uwr.role_id, p.permission_key
		FROM user_workspace_roles uwr
		JOIN role_permissions rp ON uwr.role_id = rp.role_id
		JOIN permissions p ON rp.permission_id = p.id
		WHERE uwr.user_id = ?
	`, userID)
	if err == nil {
		defer func() { _ = roleRows.Close() }()
		for roleRows.Next() {
			var workspaceID, roleID int
			var permissionKey string
			if err := roleRows.Scan(&workspaceID, &roleID, &permissionKey); err != nil {
				continue
			}

			// Track role assignment
			if cached.RoleAssignments[workspaceID] == nil {
				cached.RoleAssignments[workspaceID] = []int{}
			}
			// Avoid duplicates
			roleExists := false
			for _, rid := range cached.RoleAssignments[workspaceID] {
				if rid == roleID {
					roleExists = true
					break
				}
			}
			if !roleExists {
				cached.RoleAssignments[workspaceID] = append(cached.RoleAssignments[workspaceID], roleID)
			}

			// Add permission from role
			if cached.WorkspacePermissions[workspaceID] == nil {
				cached.WorkspacePermissions[workspaceID] = make(map[string]bool)
			}
			cached.WorkspacePermissions[workspaceID][permissionKey] = true

			// Track source
			if cached.PermissionSources[workspaceID] == nil {
				cached.PermissionSources[workspaceID] = make(map[string]string)
			}
			if cached.PermissionSources[workspaceID][permissionKey] == "" {
				cached.PermissionSources[workspaceID][permissionKey] = "role"
			}
		}
	}

	// Load group role assignments (permissions granted via group membership)
	if len(cached.GroupMemberships) > 0 {
		// Build group ID list for query
		groupIDList := ""
		for i, gid := range cached.GroupMemberships {
			if i > 0 {
				groupIDList += ","
			}
			groupIDList += fmt.Sprintf("%d", gid)
		}

		groupRoleQuery := fmt.Sprintf(`
			SELECT gwr.workspace_id, p.permission_key
			FROM group_workspace_roles gwr
			JOIN role_permissions rp ON gwr.role_id = rp.role_id
			JOIN permissions p ON rp.permission_id = p.id
			WHERE gwr.group_id IN (%s)
		`, groupIDList)

		groupRoleRows, err := ps.db.Query(groupRoleQuery)
		if err == nil {
			defer func() { _ = groupRoleRows.Close() }()
			for groupRoleRows.Next() {
				var workspaceID int
				var permissionKey string
				if err := groupRoleRows.Scan(&workspaceID, &permissionKey); err != nil {
					continue
				}

				// Add permission from group
				if cached.WorkspacePermissions[workspaceID] == nil {
					cached.WorkspacePermissions[workspaceID] = make(map[string]bool)
				}
				cached.WorkspacePermissions[workspaceID][permissionKey] = true

				// Track source (only if not already set by role or direct)
				if cached.PermissionSources[workspaceID] == nil {
					cached.PermissionSources[workspaceID] = make(map[string]string)
				}
				if cached.PermissionSources[workspaceID][permissionKey] == "" {
					cached.PermissionSources[workspaceID][permissionKey] = "group"
				}
			}
		}
	}

	// Grant all permissions for personal workspaces owned by this user
	personalRows, err := ps.db.Query(`
		SELECT w.id FROM workspaces w WHERE w.is_personal = 1 AND w.owner_id = ? AND w.active = 1
	`, userID)
	if err == nil {
		defer func() { _ = personalRows.Close() }()

		// Lazy-load if startup pre-load failed
		if len(ps.allPermissionKeys) == 0 {
			if err := ps.loadAllPermissionKeys(); err != nil {
				slog.Warn("Failed to lazy-load permission keys for personal workspace grant",
					slog.String("component", "permissions"),
					slog.Int("user_id", userID),
					slog.Any("error", err))
			}
		}

		if len(ps.allPermissionKeys) > 0 {
			for personalRows.Next() {
				var wsID int
				if err := personalRows.Scan(&wsID); err != nil {
					continue
				}
				if cached.WorkspacePermissions[wsID] == nil {
					cached.WorkspacePermissions[wsID] = make(map[string]bool)
				}
				for _, key := range ps.allPermissionKeys {
					cached.WorkspacePermissions[wsID][key] = true
				}
				if cached.PermissionSources[wsID] == nil {
					cached.PermissionSources[wsID] = make(map[string]string)
				}
				cached.PermissionSources[wsID]["_source"] = "personal_owner"
			}
		}
	}

	return cached, nil
}

// storeUserPermissionCache stores permission cache data
func (ps *PermissionService) storeUserPermissionCache(userID int, cached *models.UserPermissionCache) error {
	cacheKey := ps.getCacheKey(userID)

	data, err := json.Marshal(cached)
	if err != nil {
		return fmt.Errorf("error marshaling cache data: %v", err)
	}

	return ps.cache.Set(cacheKey, data)
}

// getWorkspaceActiveMap returns a map of workspace_id -> active flag
func (ps *PermissionService) getWorkspaceActiveMap() (map[int]bool, error) {
	rows, err := ps.db.Query(`SELECT id, active FROM workspaces`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int]bool)
	for rows.Next() {
		var id int
		var active bool
		if err := rows.Scan(&id, &active); err != nil {
			return nil, err
		}
		result[id] = active
	}
	return result, nil
}

// getRolePermissionsByName loads permission keys for a workspace role by name
func (ps *PermissionService) getRolePermissionsByName(name string) (map[string]bool, error) {
	var roleID int
	err := ps.db.QueryRow(`SELECT id FROM workspace_roles WHERE name = ? LIMIT 1`, name).Scan(&roleID)
	if err != nil {
		return nil, err
	}
	return ps.getRolePermissions(roleID)
}

// getRolePermissions loads permission keys for a given workspace role id
func (ps *PermissionService) getRolePermissions(roleID int) (map[string]bool, error) {
	rows, err := ps.db.Query(`
		SELECT p.permission_key
		FROM role_permissions rp
		JOIN permissions p ON rp.permission_id = p.id
		WHERE rp.role_id = ?
	`, roleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	perms := make(map[string]bool)
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err == nil {
			perms[key] = true
		}
	}
	return perms, nil
}

// loadAllPermissionKeys fetches all permission keys from the database and
// stores them on the service. The permissions table is static, so this only
// needs to run once (at startup or lazily on first cache build).
func (ps *PermissionService) loadAllPermissionKeys() error {
	rows, err := ps.db.Query(`SELECT permission_key FROM permissions`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err == nil {
			keys = append(keys, key)
		}
	}
	ps.allPermissionKeys = keys
	return nil
}

func clonePermissionSet(src map[string]bool) map[string]bool {
	dst := make(map[string]bool, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// applyAllViewersInheritance grants permissions from roles without explicit members to all users with Viewer
// This implements the "All Viewers" inheritance model where:
// - Roles without explicit members grant to all users with Viewer permissions
// - Roles with explicit members are restricted to those members only
//
// NOTE: This function is intentionally disabled. The feature was found to grant all permissions
// to viewers when no explicit role assignments exist, which breaks role-based access control.
// If needed in the future, this should be a workspace-level opt-in setting.
func (ps *PermissionService) applyAllViewersInheritance(_ *models.UserPermissionCache, _, _, _ map[string]bool) {
	// Function intentionally disabled - see comment above
}

// WarmCache pre-loads permissions for recently active users
func (ps *PermissionService) WarmCache() {
	slog.Info("Starting permission cache warm-up",
		slog.String("component", "permissions"))

	// Get recently active users (last 24 hours)
	activeUsers, err := ps.getRecentlyActiveUsers(24 * time.Hour)
	if err != nil {
		slog.Error("Error getting recently active users for cache warm-up",
			slog.String("component", "permissions"),
			slog.Any("error", err))
		return
	}

	warmedCount := 0
	for _, userID := range activeUsers {
		if err := ps.preWarmUserCache(userID); err != nil {
			slog.Warn("Error warming cache for user",
				slog.String("component", "permissions"),
				slog.Int("user_id", userID),
				slog.Any("error", err))
			continue
		}
		warmedCount++

		// Add small delay to prevent overwhelming the database
		if warmedCount%ps.batchSize == 0 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	slog.Info("Permission cache warm-up completed",
		slog.String("component", "permissions"),
		slog.Int("users_cached", warmedCount))
}

// preWarmUserCache loads and caches permissions for a specific user
func (ps *PermissionService) preWarmUserCache(userID int) error {
	cached, err := ps.buildUserPermissionCache(userID)
	if err != nil {
		return err
	}

	return ps.storeUserPermissionCache(userID, cached)
}

// getRecentlyActiveUsers returns user IDs who were active in the specified duration
func (ps *PermissionService) getRecentlyActiveUsers(duration time.Duration) ([]int, error) {
	since := time.Now().Add(-duration)

	rows, err := ps.db.Query(`
		SELECT DISTINCT user_id
		FROM user_sessions
		WHERE created_at > ? OR last_activity > ?
		ORDER BY last_activity DESC
		LIMIT ?
	`, since, since, ps.batchSize*2) // Limit to prevent excessive warm-up

	if err != nil {
		// If session table doesn't exist or has issues, fall back to basic user list
		rows, err = ps.db.Query(`
			SELECT id FROM users 
			WHERE role != 'inactive'
			ORDER BY updated_at DESC
			LIMIT ?
		`, ps.batchSize)

		if err != nil {
			return nil, err
		}
	}
	defer rows.Close()

	var userIDs []int
	for rows.Next() {
		var userID int
		if err := rows.Scan(&userID); err == nil {
			userIDs = append(userIDs, userID)
		}
	}

	return userIDs, nil
}

// Close gracefully shuts down the permission service
func (ps *PermissionService) Close() error {
	return ps.cache.Close()
}
