package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"windshift/internal/cacheutil"
	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"

	"github.com/allegro/bigcache/v3"
)

// PermissionService handles cached permission resolution
type PermissionService struct {
	cache           *bigcache.BigCache
	db              database.Database
	cacheCommitMu   sync.RWMutex
	cacheGeneration atomic.Uint64
	workspaceAccess *workspaceAccessCache
	closeOnce       sync.Once
	closeErr        error

	hits                      int64
	misses                    int64
	errors                    int64
	permissionCheckCount      int64
	permissionCheckNanos      int64
	permissionSnapshotDecodes atomic.Uint64

	ttl       time.Duration
	batchSize int

	allPermissionKeys []string
}

// PermissionCacheConfig represents configuration for the permission cache
type PermissionCacheConfig struct {
	TTL             time.Duration `json:"ttl"`               // Default: 15min
	MaxCacheSize    int           `json:"max_cache_size"`    // Default: 123MB
	WarmupOnStartup bool          `json:"warmup_on_startup"` // Default: true
	PreWarmActive   bool          `json:"pre_warm_active"`   // Default: true
	BatchSize       int           `json:"batch_size"`        // Default: 100
}

// DefaultPermissionCacheConfig returns default configuration
// deadcode-keep: called by core-tests test fixtures (invitations_test.go, items_test.go, iterations_test.go and others)
func DefaultPermissionCacheConfig() PermissionCacheConfig {
	return PermissionCacheConfig{
		TTL:             15 * time.Minute,
		MaxCacheSize:    123,
		WarmupOnStartup: true,
		PreWarmActive:   true,
		BatchSize:       100,
	}
}

// NewPermissionService creates a new permission service with caching
func NewPermissionService(db database.Database, config PermissionCacheConfig) (*PermissionService, error) {
	cache, err := cacheutil.New("permissions", cacheutil.BigCacheOptions{
		TTL:               config.TTL,
		MaxCacheMB:        config.MaxCacheSize,
		MaxEntrySize:      8192, // 8KB per entry (larger for permission data)
		Shards:            64,
		InitialCapacityMB: 4,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create BigCache for permissions: %w", err)
	}

	service := &PermissionService{
		cache:           cache,
		db:              db,
		workspaceAccess: newWorkspaceAccessCache(),
		ttl:             config.TTL,
		batchSize:       config.BatchSize,
	}

	if err := service.loadAllPermissionKeys(); err != nil {
		slog.Warn("Failed to pre-load permission keys; will lazy-load on first cache build",
			slog.String("component", "permissions"),
			slog.Any("error", err))
	}

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
		atomic.AddInt64(&ps.permissionCheckCount, 1)
		atomic.AddInt64(&ps.permissionCheckNanos, time.Since(startTime).Nanoseconds())
	}()

	cached, err := ps.effectivePermissionSnapshot(userID)
	if err != nil {
		return false, err
	}
	return workspacePermissionFromSnapshot(cached, workspaceID, permission), nil
}

func workspacePermissionFromSnapshot(cached *models.UserPermissionCache, workspaceID int, permission string) bool {
	if cached == nil {
		return false
	}
	if cached.IsSystemAdmin {
		return true
	}
	if everyonePerms := cached.WorkspaceEveryone[workspaceID]; everyonePerms[permission] {
		return true
	}
	return cached.WorkspacePermissions[workspaceID][permission]
}

func (ps *PermissionService) effectivePermissionSnapshot(userID int) (*models.UserPermissionCache, error) {
	cached, err := ps.getUserPermissionCache(userID)
	if err == nil {
		atomic.AddInt64(&ps.hits, 1)
		return cached, nil
	}

	atomic.AddInt64(&ps.misses, 1)
	cached, err = ps.buildAndStoreUserPermissionCache(userID)
	if err != nil {
		atomic.AddInt64(&ps.errors, 1)
		return nil, err
	}
	return cached, nil
}

// buildAndStoreUserPermissionCache prevents a snapshot that started before an
// invalidation from being written back after the invalidation completed.
// Permission builds intentionally happen outside the commit lock so unrelated
// reads remain concurrent; the generation check makes the final write linear.
func (ps *PermissionService) buildAndStoreUserPermissionCache(userID int) (*models.UserPermissionCache, error) {
	for {
		generation := ps.cacheGeneration.Load()
		cached, err := ps.buildUserPermissionCache(userID)
		if err != nil {
			return nil, err
		}
		stored, err := ps.storeUserPermissionCacheIfCurrent(userID, cached, generation)
		if err != nil {
			slog.Warn("failed to store effective permission snapshot",
				slog.String("component", "permissions"),
				slog.Int("user_id", userID),
				slog.Any("error", err))
			return cached, nil
		}
		if stored {
			return cached, nil
		}
	}
}

func (ps *PermissionService) storeUserPermissionCacheIfCurrent(
	userID int,
	cached *models.UserPermissionCache,
	generation uint64,
) (bool, error) {
	ps.cacheCommitMu.RLock()
	defer ps.cacheCommitMu.RUnlock()
	if generation != ps.cacheGeneration.Load() {
		return false, nil
	}
	return true, ps.storeUserPermissionCache(userID, cached)
}

// HasGlobalPermission checks if user has a specific global permission
func (ps *PermissionService) HasGlobalPermission(userID int, permission string) (bool, error) {
	// Cache misses build a complete snapshot so later checks stay in memory.
	cached, err := ps.getUserPermissionCache(userID)
	if err == nil {
		atomic.AddInt64(&ps.hits, 1)

		if cached.IsSystemAdmin {
			return true, nil
		}

		return cached.GlobalPermissions[permission], nil
	}

	atomic.AddInt64(&ps.misses, 1)
	return ps.loadUserPermissionAndCheckGlobal(userID, permission)
}

// HasGlobalPermissionContext is the request-aware form used by hot read paths.
// Cache hits remain allocation-free; a miss uses one cancellable SQL probe
// instead of building the full permission snapshot after the request is gone.
func (ps *PermissionService) HasGlobalPermissionContext(ctx context.Context, userID int, permission string) (bool, error) {
	if cached, err := ps.getUserPermissionCache(userID); err == nil {
		atomic.AddInt64(&ps.hits, 1)
		return cached.IsSystemAdmin || cached.GlobalPermissions[permission], nil
	}
	atomic.AddInt64(&ps.misses, 1)
	var allowed bool
	err := ps.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM user_global_permissions ugp
			JOIN permissions p ON p.id = ugp.permission_id
			WHERE ugp.user_id = ? AND p.permission_key IN (?, 'system.admin')
			UNION
			SELECT 1 FROM group_members gm
			JOIN groups g ON g.id = gm.group_id AND g.is_active = true
			JOIN group_global_permissions ggp ON ggp.group_id = gm.group_id
			JOIN permissions p ON p.id = ggp.permission_id
			WHERE gm.user_id = ? AND p.permission_key IN (?, 'system.admin')
		)
	`, userID, permission, userID, permission).Scan(&allowed)
	if err != nil {
		atomic.AddInt64(&ps.errors, 1)
		return false, fmt.Errorf("error checking global permission: %w", err)
	}
	return allowed, nil
}

// HasWorkspacePermissions checks multiple permissions in single operation
func (ps *PermissionService) HasWorkspacePermissions(userID, workspaceID int, permissions []string) (map[string]bool, error) {
	cached, err := ps.getUserPermissionCache(userID)
	if err == nil {
		atomic.AddInt64(&ps.hits, 1)
		return workspacePermissionResults(cached, workspaceID, permissions), nil
	}

	atomic.AddInt64(&ps.misses, 1)
	return ps.loadUserPermissionAndCheckMultiple(userID, workspaceID, permissions)
}

// IsSystemAdmin checks if user is system administrator
func (ps *PermissionService) IsSystemAdmin(userID int) (bool, error) {
	cached, err := ps.getUserPermissionCache(userID)
	if err == nil {
		atomic.AddInt64(&ps.hits, 1)
		return cached.IsSystemAdmin, nil
	}

	// Keep this probe aligned with auth_policy.go when the snapshot is absent.
	atomic.AddInt64(&ps.misses, 1)
	var hasPermission bool
	err = ps.db.QueryRow(repository.SystemAdminGrantQuery, userID, userID).Scan(&hasPermission)
	if err != nil {
		atomic.AddInt64(&ps.errors, 1)
		return false, fmt.Errorf("error checking system admin permission: %w", err)
	}

	return hasPermission, nil
}

// IsSystemAdminContext is the request-aware form of IsSystemAdmin.
func (ps *PermissionService) IsSystemAdminContext(ctx context.Context, userID int) (bool, error) {
	if cached, err := ps.getUserPermissionCache(userID); err == nil {
		atomic.AddInt64(&ps.hits, 1)
		return cached.IsSystemAdmin, nil
	}
	atomic.AddInt64(&ps.misses, 1)
	var hasPermission bool
	err := ps.db.QueryRowContext(ctx, repository.SystemAdminGrantQuery, userID, userID).Scan(&hasPermission)
	if err != nil {
		atomic.AddInt64(&ps.errors, 1)
		return false, fmt.Errorf("error checking system admin permission: %w", err)
	}
	return hasPermission, nil
}

// GetItemWorkspaceID returns the current workspace for an item. Item location
// is intentionally not stored in permission snapshots because doing so would
// couple item moves to authorization-cache invalidation.
func (ps *PermissionService) GetItemWorkspaceID(_, itemID int) (int, error) {
	workspaceID, err := repository.NewItemRepository(ps.db).GetWorkspaceID(itemID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return 0, fmt.Errorf("item not found: %d", itemID)
		}
		atomic.AddInt64(&ps.errors, 1)
		return 0, fmt.Errorf("error querying item workspace: %w", err)
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
	ps.permissionSnapshotDecodes.Add(1)

	if time.Now().After(cached.ExpiresAt) {
		_ = ps.cache.Delete(cacheKey)
		return nil, fmt.Errorf("cache entry expired")
	}

	return &cached, nil
}

// loadUserPermissionAndCheckGlobal loads user permissions from DB and checks global permission
func (ps *PermissionService) loadUserPermissionAndCheckGlobal(userID int, permission string) (bool, error) {
	cached, err := ps.buildAndStoreUserPermissionCache(userID)
	if err != nil {
		atomic.AddInt64(&ps.errors, 1)
		return false, err
	}

	if cached.IsSystemAdmin {
		return true, nil
	}

	return cached.GlobalPermissions[permission], nil
}

// loadUserPermissionAndCheckMultiple loads user permissions and checks multiple permissions
func (ps *PermissionService) loadUserPermissionAndCheckMultiple(userID, workspaceID int, permissions []string) (map[string]bool, error) {
	cached, err := ps.buildAndStoreUserPermissionCache(userID)
	if err != nil {
		atomic.AddInt64(&ps.errors, 1)
		return make(map[string]bool), err
	}
	return workspacePermissionResults(cached, workspaceID, permissions), nil
}

func workspacePermissionResults(
	cached *models.UserPermissionCache,
	workspaceID int,
	permissions []string,
) map[string]bool {
	result := make(map[string]bool)
	if cached.IsSystemAdmin {
		for _, permission := range permissions {
			result[permission] = true
		}
		return result
	}
	everyone := cached.WorkspaceEveryone[workspaceID]
	explicit := cached.WorkspacePermissions[workspaceID]
	for _, permission := range permissions {
		result[permission] = everyone[permission] || explicit[permission]
	}
	return result
}

// GetGroupMemberships returns the group IDs for a user, leveraging the permission cache.
// Falls back to a direct DB query on cache miss.
func (ps *PermissionService) GetGroupMemberships(userID int) ([]int, error) {
	cached, err := ps.getUserPermissionCache(userID)
	if err == nil {
		atomic.AddInt64(&ps.hits, 1)
		return cached.GroupMemberships, nil
	}

	atomic.AddInt64(&ps.misses, 1)
	cached, err = ps.buildAndStoreUserPermissionCache(userID)
	if err != nil {
		atomic.AddInt64(&ps.errors, 1)
		return nil, err
	}
	return cached.GroupMemberships, nil
}

// HasWorkspaceRole checks whether a user has a specific role in a workspace.
func (ps *PermissionService) HasWorkspaceRole(userID, workspaceID, roleID int) (bool, error) {
	cache, err := ps.GetUserEffectivePermissions(userID)
	if err != nil {
		return false, err
	}
	for _, rid := range cache.RoleAssignments[workspaceID] {
		if rid == roleID {
			return true, nil
		}
	}
	return false, nil
}

// GetUserEffectivePermissions returns the full effective permission cache for a user,
// including explicit roles, group-based roles, and "Everyone" implicit permissions.
func (ps *PermissionService) GetUserEffectivePermissions(userID int) (*models.UserPermissionCache, error) {
	return ps.effectivePermissionSnapshot(userID)
}

// InvalidateUserCache removes a user's permission cache. If the user owns
// any agents, their caches are invalidated as well so the delegation stays
// consistent after a permission mutation on the owner.
func (ps *PermissionService) InvalidateUserCache(userID int) error {
	return ps.InvalidateMultipleUserCaches([]int{userID})
}

func (ps *PermissionService) invalidateUserCaches(userIDs []int) error {
	ps.cacheCommitMu.Lock()
	defer ps.cacheCommitMu.Unlock()
	ps.cacheGeneration.Add(1)
	var errs []error
	for _, userID := range userIDs {
		if err := ps.cache.Delete(ps.getCacheKey(userID)); err != nil && !errors.Is(err, bigcache.ErrEntryNotFound) {
			errs = append(errs, fmt.Errorf("delete permission cache for user %d: %w", userID, err))
		}
	}
	return errors.Join(errs...)
}

func (ps *PermissionService) ownedAgentIDs(ownerID int) ([]int, error) {
	rows, err := ps.db.Query(
		"SELECT id FROM users WHERE agent_owner_user_id = ?",
		ownerID,
	)
	if err != nil {
		return nil, fmt.Errorf("enumerate agents owned by user %d: %w", ownerID, err)
	}
	defer func() { _ = rows.Close() }()
	var agentIDs []int
	for rows.Next() {
		var agentID int
		if err := rows.Scan(&agentID); err != nil {
			return nil, fmt.Errorf("scan agent owned by user %d: %w", ownerID, err)
		}
		agentIDs = append(agentIDs, agentID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate agents owned by user %d: %w", ownerID, err)
	}
	return agentIDs, nil
}

// InvalidateMultipleUserCaches removes permission caches for multiple users
func (ps *PermissionService) InvalidateMultipleUserCaches(userIDs []int) error {
	allIDs := make(map[int]struct{}, len(userIDs))
	for _, userID := range userIDs {
		allIDs[userID] = struct{}{}
		agentIDs, err := ps.ownedAgentIDs(userID)
		if err != nil {
			return err
		}
		for _, agentID := range agentIDs {
			allIDs[agentID] = struct{}{}
		}
	}
	ids := make([]int, 0, len(allIDs))
	for userID := range allIDs {
		ids = append(ids, userID)
	}
	return ps.invalidateUserCaches(ids)
}

// InvalidateGroupMemberCaches invalidates caches for all members of a group
func (ps *PermissionService) InvalidateGroupMemberCaches(groupID int) error {
	// Invalidation reaches inactive groups too; reactivation must not reuse stale snapshots.
	userIDs, err := ps.getGroupMembers(groupID)
	if err != nil {
		return fmt.Errorf("error getting group members for cache invalidation: %w", err)
	}

	return ps.InvalidateMultipleUserCaches(userIDs)
}

// InvalidateWorkspaceMemberCaches invalidates caches for all members of a workspace
func (ps *PermissionService) InvalidateWorkspaceMemberCaches(workspaceID int) error {
	rows, err := ps.db.Query(`
		SELECT DISTINCT user_id FROM user_workspace_roles WHERE workspace_id = ?
		UNION
		SELECT DISTINCT gm.user_id FROM group_members gm
		JOIN group_workspace_roles gwr ON gm.group_id = gwr.group_id
		WHERE gwr.workspace_id = ?
	`, workspaceID, workspaceID)
	if err != nil {
		return fmt.Errorf("error getting workspace members for cache invalidation: %w", err)
	}
	defer rows.Close()

	userIDs, err := scanIntColumn(rows)
	if err != nil {
		return fmt.Errorf("error iterating workspace members for cache invalidation: %w", err)
	}

	return ps.InvalidateMultipleUserCaches(userIDs)
}

// getGroupMembers returns all user IDs in a group. Used by cache invalidation
// helpers when group permissions or membership change. Not filtered by
// groups.is_active: invalidation must reach members of inactive groups too,
// otherwise reactivating a group leaves stale "no perm" caches in place.
func (ps *PermissionService) getGroupMembers(groupID int) ([]int, error) {
	rows, err := ps.db.Query(`
		SELECT user_id FROM group_members WHERE group_id = ?
	`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanIntColumn(rows)
}

// OnUserPermissionChanged should be called when user permissions are modified
func (ps *PermissionService) OnUserPermissionChanged(userID int) error {
	return ps.resetAfterInvalidationError(ps.InvalidateUserCache(userID))
}

// OnGroupPermissionChanged should be called when group permissions are modified
func (ps *PermissionService) OnGroupPermissionChanged(groupID int) error {
	return ps.resetAfterInvalidationError(ps.InvalidateGroupMemberCaches(groupID))
}

// OnUserGroupMembershipChanged should be called when user is added/removed from group
func (ps *PermissionService) OnUserGroupMembershipChanged(userID, groupID int) error {
	return ps.OnUserPermissionChanged(userID)
}

// OnWorkspacePermissionChanged should be called when workspace-level permissions change
func (ps *PermissionService) OnWorkspacePermissionChanged(workspaceID int) error {
	return ps.resetAfterInvalidationError(ps.InvalidateWorkspaceMemberCaches(workspaceID))
}

// OnRoleChanged should be called when a role's permissions are modified
func (ps *PermissionService) OnRoleChanged(roleID int) error {
	var errs []error
	userIDs, err := ps.getUsersWithRole(roleID)
	if err != nil {
		errs = append(errs, fmt.Errorf("get direct users with role %d: %w", roleID, err))
	} else if err = ps.InvalidateMultipleUserCaches(userIDs); err != nil {
		errs = append(errs, fmt.Errorf("invalidate direct users with role %d: %w", roleID, err))
	}

	groupUserIDs, err := ps.getUsersInGroupsWithRole(roleID)
	if err != nil {
		errs = append(errs, fmt.Errorf("get group users with role %d: %w", roleID, err))
	} else if len(groupUserIDs) > 0 {
		if err := ps.InvalidateMultipleUserCaches(groupUserIDs); err != nil {
			errs = append(errs, fmt.Errorf("invalidate group users with role %d: %w", roleID, err))
		}
	}
	if len(errs) > 0 {
		if resetErr := ps.ResetPermissionCache(); resetErr != nil {
			return errors.Join(append(errs, fmt.Errorf("reset permission cache: %w", resetErr))...)
		}
	}
	return nil
}

// OnPermissionSetChanged should be called when a permission set's permissions are modified
func (ps *PermissionService) OnPermissionSetChanged(permissionSetID int) error {
	configSetIDs, err := ps.getConfigurationSetsUsingPermissionSet(permissionSetID)
	if err != nil {
		slog.Error("Failed to get configuration sets using permission set",
			slog.String("component", "permissions"),
			slog.Int("permission_set_id", permissionSetID),
			slog.Any("error", err))
		return ps.resetAfterInvalidationError(err)
	}

	var errs []error
	for _, configSetID := range configSetIDs {
		if err := ps.invalidateConfigurationSetWorkspaces(configSetID); err != nil {
			errs = append(errs, fmt.Errorf("invalidate configuration set %d: %w", configSetID, err))
		}
	}
	return ps.resetAfterInvalidationError(errors.Join(errs...))
}

// OnEveryoneAccessChanged resets the entire permission cache when the implicit
// "everyone" access level changes (i.e., a role's first assignment is added or
// its last assignment is removed for a workspace).
func (ps *PermissionService) OnEveryoneAccessChanged() {
	if err := ps.ResetPermissionCache(); err != nil {
		slog.Error("Failed to reset permission cache after everyone-access change",
			slog.String("component", "permissions"),
			slog.Any("error", err))
	}
}

// ResetPermissionCache invalidates every permission snapshot and prevents an
// in-flight build from committing an older generation afterward.
func (ps *PermissionService) ResetPermissionCache() error {
	if ps.cache == nil {
		return nil
	}
	ps.cacheCommitMu.Lock()
	defer ps.cacheCommitMu.Unlock()
	ps.cacheGeneration.Add(1)
	return ps.cache.Reset()
}

// OnConfigurationSetChanged should be called when a configuration set is modified or reassigned
func (ps *PermissionService) OnConfigurationSetChanged(configurationSetID int) error {
	return ps.resetAfterInvalidationError(ps.invalidateConfigurationSetWorkspaces(configurationSetID))
}

func (ps *PermissionService) resetAfterInvalidationError(invalidationErr error) error {
	if invalidationErr == nil {
		return nil
	}
	if resetErr := ps.ResetPermissionCache(); resetErr != nil {
		return errors.Join(invalidationErr, fmt.Errorf("reset permission cache: %w", resetErr))
	}
	return nil
}

func (ps *PermissionService) invalidateConfigurationSetWorkspaces(configurationSetID int) error {
	workspaceIDs, err := ps.getWorkspacesUsingConfigurationSet(configurationSetID)
	if err != nil {
		return err
	}
	var errs []error
	for _, workspaceID := range workspaceIDs {
		if err := ps.InvalidateWorkspaceMemberCaches(workspaceID); err != nil {
			errs = append(errs, fmt.Errorf("invalidate workspace %d: %w", workspaceID, err))
		}
	}
	return errors.Join(errs...)
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

	return scanIntColumn(rows)
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

	return scanIntColumn(rows)
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

	return scanIntColumn(rows)
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

	return scanIntColumn(rows)
}

// GetCacheStats returns current cache performance statistics
func (ps *PermissionService) GetCacheStats() models.CacheStats {
	hits := atomic.LoadInt64(&ps.hits)
	misses := atomic.LoadInt64(&ps.misses)
	errCount := atomic.LoadInt64(&ps.errors)
	total := hits + misses

	hitRatio := 0.0
	if total > 0 {
		hitRatio = float64(hits) / float64(total)
	}

	// Calculate the average workspace-permission check time without taking a
	// process-wide lock on the permission hot path.
	avgLoadTime := int64(0)
	checkCount := atomic.LoadInt64(&ps.permissionCheckCount)
	if checkCount > 0 {
		avgLoadTime = atomic.LoadInt64(&ps.permissionCheckNanos) / checkCount / int64(time.Millisecond)
	}

	// Get cache info - BigCache Stats doesn't have Entries field
	// We'll track total users differently or estimate it
	totalUsers := int64(0) // For now, we don't track this precisely
	workspaceAccessStats := ps.GetWorkspaceAccessStats()

	return models.CacheStats{
		Hits:                       hits,
		Misses:                     misses,
		Errors:                     errCount,
		HitRatio:                   hitRatio,
		AvgLoadTime:                avgLoadTime,
		TotalUsers:                 totalUsers,
		PermissionSnapshotDecodes:  workspaceAccessStats.PermissionSnapshotDecodes,
		ActiveWorkspaceCacheHits:   workspaceAccessStats.ActiveWorkspaceCacheHits,
		ActiveWorkspaceCacheMisses: workspaceAccessStats.ActiveWorkspaceCacheMisses,
	}
}

// buildUserPermissionCache loads complete permission profile from database
func (ps *PermissionService) buildUserPermissionCache(userID int) (*models.UserPermissionCache, error) {
	inherited, ok, err := ps.inheritedAgentPermissionCache(userID)
	if err != nil || ok {
		return inherited, err
	}

	builder := newPermissionCacheBuilder(ps, userID)
	complete, err := builder.loadSystemAdmin()
	if err != nil {
		return nil, err
	}
	if complete {
		return builder.cached, nil
	}
	if err := builder.loadEveryonePermissions(); err != nil {
		return nil, err
	}
	if err := builder.loadGlobalPermissions(); err != nil {
		return nil, err
	}
	if err := builder.loadGroupMemberships(); err != nil {
		return nil, err
	}
	if err := builder.loadUserRoleAssignments(); err != nil {
		return nil, err
	}
	if err := builder.loadWorkspaceRolePermissions(); err != nil {
		return nil, err
	}
	if err := builder.loadGroupRolePermissions(); err != nil {
		return nil, err
	}
	if err := builder.loadPersonalWorkspacePermissions(); err != nil {
		return nil, err
	}
	return builder.cached, nil
}

// storeUserPermissionCache stores permission cache data
func (ps *PermissionService) storeUserPermissionCache(userID int, cached *models.UserPermissionCache) error {
	cacheKey := ps.getCacheKey(userID)

	data, err := json.Marshal(cached)
	if err != nil {
		return fmt.Errorf("error marshaling cache data: %w", err)
	}

	return ps.cache.Set(cacheKey, data)
}

// getWorkspaceActiveMap returns a map of workspace_id -> active flag
func (ps *PermissionService) getWorkspaceActiveMap() (map[int]bool, error) {
	rows, err := ps.db.Query(`SELECT id, active FROM workspaces WHERE is_personal = false OR is_personal IS NULL`)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
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
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("scan permission for role %d: %w", roleID, err)
		}
		perms[key] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
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
		if err := rows.Scan(&key); err != nil {
			return fmt.Errorf("scan permission key: %w", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	ps.allPermissionKeys = keys
	return nil
}

// scanIntColumn collects a complete single-int-column result set into a slice.
func scanIntColumn(rows *sql.Rows) ([]int, error) {
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

func clonePermissionSet(src map[string]bool) map[string]bool {
	dst := make(map[string]bool, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func mergePerms(dst, src map[string]bool) {
	for k, v := range src {
		if v {
			dst[k] = true
		}
	}
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
	_, err := ps.buildAndStoreUserPermissionCache(userID)
	return err
}

// getRecentlyActiveUsers returns user IDs who were active in the specified duration
func (ps *PermissionService) getRecentlyActiveUsers(duration time.Duration) ([]int, error) {
	since := time.Now().Add(-duration)

	rows, err := ps.db.Query(`
		SELECT user_id
		FROM user_sessions
		WHERE is_active = true AND expires_at > CURRENT_TIMESTAMP AND created_at > ?
		GROUP BY user_id
		ORDER BY MAX(created_at) DESC
		LIMIT ?
	`, since, ps.batchSize*2) // Limit to prevent excessive warm-up

	if err != nil {
		// If session table doesn't exist or has issues, fall back to basic user list
		rows, err = ps.db.Query(`
			SELECT id FROM users 
			WHERE is_active = true
			ORDER BY updated_at DESC
			LIMIT ?
		`, ps.batchSize)

		if err != nil {
			return nil, err
		}
	}
	defer rows.Close()

	return scanIntColumn(rows)
}

// Close stops the cache worker without closing the shared database.
// Repeated calls are safe, including when no cache was initialized.
func (ps *PermissionService) Close() error {
	ps.closeOnce.Do(func() {
		if ps.cache != nil {
			ps.closeErr = ps.cache.Close()
		}
	})
	return ps.closeErr
}
