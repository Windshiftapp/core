import { api } from '../api.js';

const TTL_MS = 10 * 60 * 1000; // 10 minutes

/**
 * Caches test case links per item, surviving view switches (singleton store).
 * Each item can have different test case links, so the cache key is itemId.
 * TTL-based expiry (10 minutes) ensures data stays reasonably fresh.
 */
class ItemTestCaseLinksStore {
  workspaceId = $state(null);

  /** @type {Map<number, { testCases: any[], fetchedAt: number }>} */
  _cache = new Map();

  /** @type {Map<number, Promise<void>>} */
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
   * Synchronous lookup. Returns cached test cases array or null if missing/expired.
   */
  get(itemId) {
    if (!itemId) return null;
    const entry = this._cache.get(itemId);
    if (!entry) return null;
    if (Date.now() - entry.fetchedAt > TTL_MS) return null;
    return entry.testCases;
  }

  /**
   * Fetch test case links for a list of item IDs.
   * Only fetches uncached/expired items, deduplicates in-flight requests.
   * Populates the cache; returns void.
   */
  async loadForItems(itemIds) {
    if (!itemIds || itemIds.length === 0) return;

    const toFetch = itemIds.filter(id => {
      // Skip if already in-flight
      if (this._pending.has(id)) return false;
      // Skip if cached and not expired
      const entry = this._cache.get(id);
      if (entry && Date.now() - entry.fetchedAt <= TTL_MS) return false;
      return true;
    });

    if (toFetch.length === 0) {
      // Still wait on any in-flight requests for the requested IDs
      const pending = itemIds
        .map(id => this._pending.get(id))
        .filter(Boolean);
      if (pending.length > 0) await Promise.all(pending);
      return;
    }

    const fetchPromises = toFetch.map(id => this._fetchForItem(id));
    await Promise.all(fetchPromises);
  }

  /** @private */
  async _fetchForItem(itemId) {
    if (this._pending.has(itemId)) {
      return this._pending.get(itemId);
    }

    const promise = (async () => {
      try {
        const links = await api.links.getForItem('items', itemId);
        const allLinks = [...(links.outgoing || []), ...(links.incoming || [])];

        const testCases = allLinks
          .filter(link => link.link_type_id === 1)
          .map(link => {
            const isSource = link.source_type === 'item' && link.source_id === itemId;
            const testCaseData = isSource ? {
              id: link.target_id,
              title: link.target_title,
              type: link.target_type
            } : {
              id: link.source_id,
              title: link.source_title,
              type: link.source_type
            };
            return testCaseData.type === 'test_case' ? testCaseData : null;
          })
          .filter(tc => tc !== null);

        this._cache.set(itemId, { testCases, fetchedAt: Date.now() });
      } catch (err) {
        console.error(`ItemTestCaseLinksStore: failed to fetch links for item ${itemId}`, err);
        // Cache empty result to avoid repeated failures
        this._cache.set(itemId, { testCases: [], fetchedAt: Date.now() });
      } finally {
        this._pending.delete(itemId);
      }
    })();

    this._pending.set(itemId, promise);
    return promise;
  }

  /**
   * Clear all cached data (e.g. after link changes).
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

export const itemTestCaseLinksStore = new ItemTestCaseLinksStore();
