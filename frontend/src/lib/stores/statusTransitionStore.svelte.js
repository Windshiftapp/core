import { api } from '../api.js';

const TTL_MS = 10 * 60 * 1000; // 10 minutes

/**
 * Caches available status transitions by (itemTypeId, statusId) instead of per-item.
 * Transitions depend only on item type + current status (via workflow config),
 * so a board with 100 items and 5 statuses across 2 item types needs ~10 fetches, not 100.
 *
 * Survives view switches (singleton store, not component-local).
 */
class StatusTransitionStore {
  workspaceId = $state(null);

  /** @type {Map<string, { transitions: any[], fetchedAt: number }>} */
  _cache = new Map();

  /** @type {Map<string, Promise<any[]>>} */
  _pending = new Map();

  /** @private */
  _cacheKey(itemTypeId, statusId) {
    return `${itemTypeId}:${statusId}`;
  }

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
   * Synchronous lookup. Returns cached transitions or null if missing/expired.
   */
  get(itemTypeId, statusId) {
    if (!itemTypeId || !statusId) return null;
    const entry = this._cache.get(this._cacheKey(itemTypeId, statusId));
    if (!entry) return null;
    if (Date.now() - entry.fetchedAt > TTL_MS) return null;
    return entry.transitions;
  }

  /**
   * Synchronous validation check for drag-and-drop.
   */
  isValidTransition(itemTypeId, fromStatusId, toStatusId) {
    if (!fromStatusId || !toStatusId) return false;
    if (fromStatusId === toStatusId) return true;
    const transitions = this.get(itemTypeId, fromStatusId);
    if (!transitions) return false; // fail-safe: deny if unknown
    return transitions.some(t => t.id === toStatusId);
  }

  /**
   * Batch-preload transitions for a list of items.
   * Groups by unique (itemTypeId, statusId), fetches only uncached pairs
   * using one representative item per pair.
   */
  async preloadForItems(items) {
    if (!items || items.length === 0) return;

    // Group items by unique (itemTypeId, statusId), pick one representative per group
    const representatives = new Map();
    for (const item of items) {
      if (!item.item_type_id || !item.status_id) continue;
      const key = this._cacheKey(item.item_type_id, item.status_id);

      // Skip if already cached and not expired
      const existing = this._cache.get(key);
      if (existing && Date.now() - existing.fetchedAt <= TTL_MS) continue;

      // Skip if we already picked a representative for this pair
      if (representatives.has(key)) continue;

      representatives.set(key, item);
    }

    if (representatives.size === 0) return;

    // Fetch all uncached pairs concurrently, deduplicating in-flight requests
    const fetchPromises = [];
    for (const [key, item] of representatives) {
      fetchPromises.push(this._fetchForItem(key, item));
    }

    await Promise.all(fetchPromises);
  }

  /** @private */
  async _fetchForItem(cacheKey, item) {
    // Deduplicate: if already in-flight for this key, wait on existing promise
    if (this._pending.has(cacheKey)) {
      return this._pending.get(cacheKey);
    }

    const promise = (async () => {
      try {
        const result = await api.items.getAvailableStatusTransitions(item.id);
        const transitions = result.available_transitions || [];
        this._cache.set(cacheKey, { transitions, fetchedAt: Date.now() });
        return transitions;
      } catch (err) {
        console.error(`StatusTransitionStore: failed to fetch transitions for key ${cacheKey}`, err);
        return [];
      } finally {
        this._pending.delete(cacheKey);
      }
    })();

    this._pending.set(cacheKey, promise);
    return promise;
  }

  /**
   * Clear all cached transitions (e.g. after workflow configuration changes).
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

export const statusTransitionStore = new StatusTransitionStore();
