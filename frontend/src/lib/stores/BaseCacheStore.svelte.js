/**
 * Base class for cached stores with TTL and workspace scoping.
 * Provides common initialization, invalidation, and reset patterns.
 */
export class BaseCacheStore {
  workspaceId = $state(null);
  _cache = new Map();
  _pending = new Map();

  /**
   * Set workspace scope. Resets cache if workspace changed.
   */
  initialize(workspaceId) {
    const id = typeof workspaceId === 'string' ? parseInt(workspaceId, 10) : workspaceId;
    if (this.workspaceId === id) return;
    this.reset();
    this.workspaceId = id;
  }

  /**
   * Clear all cached data (e.g. after configuration changes).
   */
  invalidateAll() {
    this._cache.clear();
    this._pending.clear();
  }

  /**
   * Full reset: clear cache and workspace scope.
   */
  reset() {
    this._cache.clear();
    this._pending.clear();
    this.workspaceId = null;
  }
}
