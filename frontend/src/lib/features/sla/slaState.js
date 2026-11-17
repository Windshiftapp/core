import { BACKGROUND_READ } from '../../api/readScheduler.js';
import { sla } from '../../api/sla.js';
import { workItemRefreshTouchesItem } from '../../stores/workItemRefresh.js';

// Item SLA reads are pure and cheap on the server, but a list/board page shows
// many rows. Cache each item's state briefly and dedupe concurrent reads so a
// page issues at most one request per item, and none at all on a re-render.
const STATE_TTL_MS = 30_000;
// Warning thresholds change rarely. Keep them much longer than item state so a
// board's wall-clock tick only re-evaluates states, not configuration.
const THRESHOLD_TTL_MS = 5 * 60 * 1000;
// After a transient threshold failure, do not hammer the endpoint on every
// badge mount; retry after a short cooldown.
const THRESHOLD_FAILURE_COOLDOWN_MS = 5_000;
// Cap the caches so a long-lived page that renders thousands of distinct rows
// cannot retain every entry until an explicit invalidation.
const MAX_CACHE_ENTRIES = 2000;

const stateCache = new Map();
const inFlight = new Map();
const thresholdsCache = new Map();
const thresholdsPending = new Map();
/** @type {Set<(workspaceId: number|string|null) => void>} */
const thresholdListeners = new Set();
// Threshold invalidation is per workspace: bumping one workspace must not
// discard a successful in-flight fetch for another.
let thresholdEpoch = 0;
const thresholdGenerations = new Map();

function pruneCache(cache, now = Date.now()) {
  if (cache.size <= MAX_CACHE_ENTRIES) return;
  for (const [key, entry] of cache) {
    if (now - entry.at >= STATE_TTL_MS) cache.delete(key);
  }
  // Still over the cap: drop oldest-first (Map preserves insertion order).
  while (cache.size > MAX_CACHE_ENTRIES) {
    const oldest = cache.keys().next().value;
    cache.delete(oldest);
  }
}

function pruneThresholdCache(now = Date.now()) {
  if (thresholdsCache.size <= MAX_CACHE_ENTRIES) return;
  for (const [key, entry] of thresholdsCache) {
    if (entry.value === undefined || now - entry.fetchedAt >= THRESHOLD_TTL_MS) {
      thresholdsCache.delete(key);
    }
  }
  while (thresholdsCache.size > MAX_CACHE_ENTRIES) {
    const oldest = thresholdsCache.keys().next().value;
    thresholdsCache.delete(oldest);
  }
}

function normalizeWorkspaceKey(workspaceId) {
  const id = Number(workspaceId);
  return Number.isInteger(id) && id > 0 ? id : String(workspaceId);
}

function thresholdGenerationFor(key) {
  return `${thresholdEpoch}:${thresholdGenerations.get(key) ?? 0}`;
}

// Batch coalescing: badges mounted in the same tick share one workspace
// batch read instead of issuing one request per row (WI-1591). Items whose
// workspace is unknown fall back to per-item reads; a failed batch does not.
const pendingByWorkspace = new Map();

// Server cap for one batch read (WI-1637). Boards can mount more cards than
// this, so ids are split across requests instead of being rejected whole.
const BATCH_ID_LIMIT = 200;

export async function getItemSLA(itemId, workspaceId = null, { force = false } = {}) {
  if (!itemId) return [];
  itemId = Number(itemId);
  if (workspaceId != null) workspaceId = normalizeWorkspaceKey(workspaceId);
  if (force) stateCache.delete(itemId);
  const cached = stateCache.get(itemId);
  const now = Date.now();
  const movedWorkspace = (entry) =>
    entry.workspaceId != null && workspaceId != null && entry.workspaceId !== workspaceId;
  if (cached && !movedWorkspace(cached) && now - cached.at < STATE_TTL_MS) return cached.value;
  if (cached) stateCache.delete(itemId);
  const pending = inFlight.get(itemId);
  if (pending) {
    if (movedWorkspace(pending)) {
      pending.invalidated = true;
      pending.workspaceId = workspaceId;
    }
    return pending.promise;
  }

  if (workspaceId && sla?.getItemSLABatch) {
    return enqueueBatchItemSLA(itemId, workspaceId);
  }
  if (!sla?.getItemSLA) return [];
  return fetchItemSLA(itemId, workspaceId);
}

// enqueueBatchItemSLA collects item ids per workspace and flushes them on the
// next microtask, so all badges rendered by one page share batched requests.
function enqueueBatchItemSLA(itemId, workspaceId) {
  let queue = pendingByWorkspace.get(workspaceId);
  if (!queue) {
    queue = { ids: new Set(), waiters: new Map(), scheduled: false };
    pendingByWorkspace.set(workspaceId, queue);
  }
  const existing = queue.waiters.get(itemId);
  if (existing) return existing.promise;

  let resolve;
  const promise = new Promise((r) => {
    resolve = r;
  });
  queue.ids.add(itemId);
  const entry = { promise, resolve, workspaceId, started: false, invalidated: false };
  queue.waiters.set(itemId, entry);
  // Claim the in-flight slot so a remount during the flush reuses this read
  // instead of issuing a second batch for the same item.
  inFlight.set(itemId, entry);
  if (!queue.scheduled) {
    queue.scheduled = true;
    queueMicrotask(() => flushBatchItemSLA(workspaceId, queue));
  }
  return promise;
}

async function flushBatchItemSLA(workspaceId, queue) {
  if (pendingByWorkspace.get(workspaceId) === queue) pendingByWorkspace.delete(workspaceId);
  const ids = [...queue.ids];
  for (const entry of queue.waiters.values()) entry.started = true;
  const results = new Map();
  const chunks = [];
  for (let i = 0; i < ids.length; i += BATCH_ID_LIMIT) {
    chunks.push(ids.slice(i, i + BATCH_ID_LIMIT));
  }
  await Promise.all(
    chunks.map(async (chunkIds) => {
      let batch;
      try {
        batch = (await sla.getItemSLABatch(workspaceId, chunkIds, BACKGROUND_READ)) ?? {};
      } catch {
        // A failed chunk resolves unset rather than issuing one request per
        // item, which would amplify load under rate limiting or outages.
        return;
      }
      for (const itemId of chunkIds) {
        results.set(itemId, batch[itemId] ?? batch[String(itemId)] ?? []);
      }
    })
  );
  for (const itemId of ids) {
    const waiter = queue.waiters.get(itemId);
    if (!waiter) continue;
    waiter.resolve(settleItemSLA(itemId, waiter, results.get(itemId) ?? [], results.has(itemId)));
  }
}

// Keep ownership until the transport settles. Invalidated readers share one
// replacement read; a stale response never reaches the cache or mounted badges.
function settleItemSLA(itemId, entry, value, cacheable) {
  if (inFlight.get(itemId) !== entry) return [];
  inFlight.delete(itemId);
  if (entry.invalidated) return getItemSLA(itemId, entry.workspaceId);
  if (cacheable) {
    stateCache.set(itemId, { value, at: Date.now(), workspaceId: entry.workspaceId });
    pruneCache(stateCache);
  }
  return value;
}

function fetchItemSLA(itemId, workspaceId) {
  const entry = { promise: null, workspaceId, started: true, invalidated: false };
  entry.promise = sla
    .getItemSLA(itemId, BACKGROUND_READ)
    .then((value) => {
      return settleItemSLA(itemId, entry, value ?? [], true);
    })
    .catch(() => settleItemSLA(itemId, entry, [], false));

  inFlight.set(itemId, entry);
  return entry.promise;
}

/**
 * Warning thresholds for a workspace. Returns a cached successful list while
 * it is fresh, a retained stale list after a transient failure, or null when
 * no prior value exists (threshold-unavailable). Never caches a failure as an
 * empty list.
 */
export async function getSLAThresholds(workspaceId) {
  if (!sla?.getWarningThresholds) return [];
  // A missing workspace is unknown, not a configured-empty threshold list.
  if (!workspaceId) return null;
  const key = normalizeWorkspaceKey(workspaceId);
  const now = Date.now();
  const cached = thresholdsCache.get(key);

  if (cached) {
    if (cached.value !== undefined && now - cached.fetchedAt <= THRESHOLD_TTL_MS) {
      return cached.value;
    }
    if (cached.failedAt && now - cached.failedAt < THRESHOLD_FAILURE_COOLDOWN_MS) {
      return cached.value ?? null;
    }
  }

  const pending = thresholdsPending.get(key);
  if (pending) return pending;

  const generation = thresholdGenerationFor(key);
  const promise = sla
    .getWarningThresholds(workspaceId, BACKGROUND_READ)
    .then((result) => {
      const value = Array.isArray(result) ? result : [];
      if (generation === thresholdGenerationFor(key)) {
        thresholdsCache.set(key, { value, fetchedAt: Date.now(), failedAt: 0 });
        pruneThresholdCache();
      }
      return value;
    })
    .catch((error) => {
      if (generation !== thresholdGenerationFor(key)) return null;
      const status = error?.status;
      if (status === 401 || status === 403 || status === 404) {
        // Protected data must not survive an authorization denial.
        thresholdsCache.delete(key);
        return null;
      }
      const previous = thresholdsCache.get(key);
      thresholdsCache.set(key, {
        value: previous?.value,
        fetchedAt: previous?.value !== undefined ? previous.fetchedAt : 0,
        failedAt: Date.now(),
      });
      return previous?.value ?? null;
    })
    .finally(() => {
      if (thresholdsPending.get(key) === promise) thresholdsPending.delete(key);
    });

  thresholdsPending.set(key, promise);
  return promise;
}

/**
 * Invalidate cached item states only. The board's wall-clock tick and
 * refresh-work-items use this so threshold configuration stays cached.
 */
export function invalidateSLAState(detail = {}) {
  for (const [itemId, entry] of stateCache) {
    if (workItemRefreshTouchesItem(detail, itemId, entry.workspaceId)) stateCache.delete(itemId);
  }
  for (const [itemId, entry] of inFlight) {
    if (entry.started && workItemRefreshTouchesItem(detail, itemId, entry.workspaceId)) {
      entry.invalidated = true;
    }
  }
}

/**
 * Invalidate one workspace's threshold cache (or every workspace when called
 * with no id) and notify mounted badges in this tab. A failed mutation must
 * not call this.
 */
export function invalidateSLAThresholds(workspaceId) {
  if (workspaceId == null) {
    thresholdEpoch++;
    thresholdsCache.clear();
    thresholdsPending.clear();
    for (const listener of thresholdListeners) listener(null);
    return;
  }
  const key = normalizeWorkspaceKey(workspaceId);
  thresholdGenerations.set(key, (thresholdGenerations.get(key) ?? 0) + 1);
  thresholdsCache.delete(key);
  thresholdsPending.delete(key);
  for (const listener of thresholdListeners) listener(workspaceId);
}

/** Subscribe to in-tab threshold invalidations. Returns an unsubscribe. */
export function onSLAThresholdsInvalidated(listener) {
  thresholdListeners.add(listener);
  return () => thresholdListeners.delete(listener);
}

/** Clear all state and threshold caches, e.g. on session reset. */
export function resetSLAState() {
  stateCache.clear();
  inFlight.clear();
  pendingByWorkspace.clear();
  invalidateSLAThresholds(null);
}

if (typeof window !== 'undefined') {
  window.addEventListener('refresh-work-items', (event) =>
    invalidateSLAState(/** @type {CustomEvent} */ (event).detail)
  );
}
