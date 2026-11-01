package services

import (
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"

	"windshift/internal/models"
	"windshift/internal/repository"
)

const activeWorkspaceCacheTTL = 5 * time.Second

type activeWorkspaceSet struct {
	epoch     uint64
	expiresAt time.Time
	pairs     []repository.IDKey
}

type activeWorkspaceLoad struct {
	epoch uint64
	pairs []repository.IDKey
}

type workspaceAccessCache struct {
	active atomic.Pointer[activeWorkspaceSet]
	loads  singleflight.Group
	epoch  atomic.Uint64
	hits   atomic.Uint64
	misses atomic.Uint64
}

// PermissionWorkspaceAccessStats exposes identifier-free counters for the
// active-workspace and effective-permission snapshots used by list hot paths.
type PermissionWorkspaceAccessStats struct {
	ActiveWorkspaceCacheHits   uint64
	ActiveWorkspaceCacheMisses uint64
	PermissionSnapshotDecodes  uint64
}

func newWorkspaceAccessCache() *workspaceAccessCache {
	return &workspaceAccessCache{}
}

// AccessibleWorkspaceIDs returns the IDs of active workspaces on which the
// user has item.view permission. It performs one active-set cache read and one
// effective-permission cache read/decode, independent of workspace count.
func (ps *PermissionService) AccessibleWorkspaceIDs(userID int) ([]int, error) {
	pairs, err := ps.accessibleWorkspacePairs(userID)
	if err != nil {
		return nil, err
	}
	ids := make([]int, len(pairs))
	for i := range pairs {
		ids[i] = pairs[i].ID
	}
	return ids, nil
}

// AccessibleWorkspaceIDKeys returns active accessible workspace IDs and keys
// from the same permission snapshot, avoiding a second permission pass for
// consumers that need both forms.
func (ps *PermissionService) AccessibleWorkspaceIDKeys(userID int) ([]repository.IDKey, error) {
	pairs, err := ps.accessibleWorkspacePairs(userID)
	if err != nil {
		return nil, err
	}
	return append([]repository.IDKey(nil), pairs...), nil
}

func (ps *PermissionService) accessibleWorkspacePairs(userID int) ([]repository.IDKey, error) {
	activePairs, err := ps.activeWorkspacePairs()
	if err != nil {
		return nil, err
	}

	permissions, err := ps.effectivePermissionSnapshot(userID)
	if err != nil {
		// Preserve the previous fail-closed behavior: a permission-cache build
		// failure skipped every workspace rather than widening access.
		slog.Error("error loading effective permissions for workspace access",
			slog.String("component", "permissions"),
			slog.Int("user_id", userID),
			slog.Any("error", err))
		return []repository.IDKey{}, nil
	}

	out := make([]repository.IDKey, 0, len(activePairs))
	for _, pair := range activePairs {
		if ps.workspacePermission(permissions, pair.ID, models.PermissionItemView) {
			out = append(out, pair)
		}
	}
	return out, nil
}

func (ps *PermissionService) activeWorkspacePairs() ([]repository.IDKey, error) {
	cache := ps.workspaceAccess
	if cache == nil {
		return repository.NewWorkspaceRepository(ps.db).ListActiveIDKeys()
	}

	for {
		currentEpoch := cache.epoch.Load()
		if current := cache.active.Load(); current != nil && current.epoch == currentEpoch && time.Now().Before(current.expiresAt) {
			cache.hits.Add(1)
			return current.pairs, nil
		}
		cache.misses.Add(1)

		loadEpoch := cache.epoch.Load()
		result, err, _ := cache.loads.Do("active-workspaces", func() (any, error) {
			if current := cache.active.Load(); current != nil && current.epoch == loadEpoch && time.Now().Before(current.expiresAt) {
				return &activeWorkspaceLoad{epoch: loadEpoch, pairs: current.pairs}, nil
			}
			pairs, err := repository.NewWorkspaceRepository(ps.db).ListActiveIDKeys()
			if err != nil {
				return nil, err
			}
			immutablePairs := append([]repository.IDKey(nil), pairs...)
			if loadEpoch == cache.epoch.Load() {
				cache.active.Store(&activeWorkspaceSet{
					epoch:     loadEpoch,
					expiresAt: time.Now().Add(activeWorkspaceCacheTTL),
					pairs:     immutablePairs,
				})
			}
			return &activeWorkspaceLoad{epoch: loadEpoch, pairs: immutablePairs}, nil
		})
		if err != nil {
			return nil, err
		}
		load, ok := result.(*activeWorkspaceLoad)
		if !ok {
			return nil, fmt.Errorf("unexpected active-workspace cache result type %T", result)
		}
		if load.epoch != cache.epoch.Load() {
			continue
		}
		return load.pairs, nil
	}
}

// WorkspaceVisibility evaluates many per-workspace view/admin decisions for
// one user against a single decoded permission snapshot, so paged directory
// and search requests pay one snapshot decode instead of one per candidate.
type WorkspaceVisibility interface {
	// CanView reports whether the user holds item.view on the workspace.
	CanView(workspaceID int) (bool, error)
	// CanAdmin reports whether the user holds workspace.admin on the workspace.
	CanAdmin(workspaceID int) (bool, error)
}

// snapshotWorkspaceVisibility answers from an already-decoded snapshot;
// evaluation is fail-closed map lookups and cannot error.
type snapshotWorkspaceVisibility struct {
	service     *PermissionService
	permissions *models.UserPermissionCache
}

func (v snapshotWorkspaceVisibility) CanView(workspaceID int) (bool, error) {
	return v.service.workspacePermission(v.permissions, workspaceID, models.PermissionItemView), nil
}

func (v snapshotWorkspaceVisibility) CanAdmin(workspaceID int) (bool, error) {
	return v.service.workspacePermission(v.permissions, workspaceID, models.PermissionWorkspaceAdmin), nil
}

// WorkspaceVisibility acquires the request-scoped evaluator. A snapshot build
// failure is returned so callers keep failing closed; revocation stays
// governed by the snapshot's own TTL and invalidation, exactly as for
// per-check HasWorkspacePermission calls.
func (ps *PermissionService) WorkspaceVisibility(userID int) (WorkspaceVisibility, error) {
	permissions, err := ps.effectivePermissionSnapshot(userID)
	if err != nil {
		return nil, err
	}
	return snapshotWorkspaceVisibility{service: ps, permissions: permissions}, nil
}

// InvalidateActiveWorkspaceCache makes local workspace create/update/delete
// mutations visible immediately. The TTL bounds changes made by other replicas.
func (ps *PermissionService) InvalidateActiveWorkspaceCache() {
	if ps == nil {
		return
	}
	ps.everyoneGeneration.Add(1)
	if ps.workspaceAccess != nil {
		ps.workspaceAccess.epoch.Add(1)
		ps.workspaceAccess.active.Store(nil)
	}
}

// GetWorkspaceAccessStats returns aggregate hot-path counters.
func (ps *PermissionService) GetWorkspaceAccessStats() PermissionWorkspaceAccessStats {
	if ps == nil {
		return PermissionWorkspaceAccessStats{}
	}
	stats := PermissionWorkspaceAccessStats{
		PermissionSnapshotDecodes: ps.permissionSnapshotDecodes.Load(),
	}
	if ps.workspaceAccess != nil {
		stats.ActiveWorkspaceCacheHits = ps.workspaceAccess.hits.Load()
		stats.ActiveWorkspaceCacheMisses = ps.workspaceAccess.misses.Load()
	}
	return stats
}
