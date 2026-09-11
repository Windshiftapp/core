import { api } from '../api.js';
import { BaseCacheStore } from './BaseCacheStore.svelte.js';

const TTL_MS = 10 * 60 * 1000; // 10 minutes
const LINK_BATCH_CHUNK = 100;

/**
 * Caches test case links per item, surviving view switches (singleton store).
 * Each item can have different test case links, so the cache key is itemId.
 * TTL-based expiry (10 minutes) ensures data stays reasonably fresh.
 */
class ItemTestCaseLinksStore extends BaseCacheStore {
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

    const toFetch = itemIds.filter((id) => {
      // Skip if already in-flight
      if (this._pending.has(id)) return false;
      // Skip if cached and not expired
      const entry = this._cache.get(id);
      if (entry && Date.now() - entry.fetchedAt <= TTL_MS) return false;
      return true;
    });

    if (toFetch.length === 0) {
      // Still wait on any in-flight requests for the requested IDs
      const pending = itemIds.map((id) => this._pending.get(id)).filter(Boolean);
      if (pending.length > 0) await Promise.all(pending);
      return;
    }

    // One batched /links/batch request per chunk instead of one
    // /items/{id}/links per item — the per-item fan-out could exhaust the DB
    // connection pool when a large tree is rendered with test cases shown.
    const chunks = [];
    for (let i = 0; i < toFetch.length; i += LINK_BATCH_CHUNK) {
      chunks.push(toFetch.slice(i, i + LINK_BATCH_CHUNK));
    }
    await Promise.all(chunks.map((chunk) => this._fetchChunk(chunk)));
  }

  /** @private Fetch + cache a chunk of item ids via the batch links endpoint. */
  _fetchChunk(chunk) {
    const generation = this._generation;
    const scopedWorkspaceId = this.workspaceId;
    const promise = (async () => {
      try {
        const groups = await api.links.getForItems(chunk);
        if (generation !== this._generation || scopedWorkspaceId !== this.workspaceId) return;
        const now = Date.now();
        for (const itemId of chunk) {
          const group = groups[itemId] || {};
          this._cache.set(itemId, {
            testCases: this._extractTestCases(itemId, group.outgoing, group.incoming),
            fetchedAt: now,
          });
        }
      } catch (err) {
        console.error('ItemTestCaseLinksStore: failed to fetch links for items', chunk, err);
        if (generation !== this._generation || scopedWorkspaceId !== this.workspaceId) return;
        // Cache empty results to avoid repeated failures.
        const now = Date.now();
        for (const itemId of chunk) this._cache.set(itemId, { testCases: [], fetchedAt: now });
      } finally {
        for (const itemId of chunk) {
          if (this._pending.get(itemId) === promise) this._pending.delete(itemId);
        }
      }
    })();

    for (const itemId of chunk) this._pending.set(itemId, promise);
    return promise;
  }

  /** @private Extract linked test cases for itemId from its link group. */
  _extractTestCases(itemId, outgoing, incoming) {
    const allLinks = [...(outgoing || []), ...(incoming || [])];
    return allLinks
      .filter((link) => link.link_type_id === 1)
      .map((link) => {
        const isSource = link.source_type === 'item' && link.source_id === itemId;
        const testCaseData = isSource
          ? { id: link.target_id, title: link.target_title, type: link.target_type }
          : { id: link.source_id, title: link.source_title, type: link.source_type };
        return testCaseData.type === 'test_case' ? testCaseData : null;
      })
      .filter((tc) => tc !== null);
  }

  /**
   * Clear all cached data (e.g. after link changes).
   * Uses inherited invalidateAll() from BaseCacheStore.
   */

  /**
   * Full reset: clear cache and workspace scope.
   * Uses inherited reset() from BaseCacheStore.
   */
}

export const itemTestCaseLinksStore = new ItemTestCaseLinksStore();
