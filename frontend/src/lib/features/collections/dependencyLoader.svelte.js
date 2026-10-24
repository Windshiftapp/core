import { links } from '../../api/configuration.js';
import { BACKGROUND_READ } from '../../api/readScheduler.js';

// Board/roadmap dependency links load in 100-anchor batches. At most two
// batches are submitted at once; the shared read scheduler bounds their
// physical concurrency further. A chunk that fails is never cached as an
// empty result: it becomes an error entry that is retryable after a cooldown,
// while a chunk that succeeds commits independently so an unrelated failure
// cannot discard useful earlier work.
export const DEPENDENCY_LINK_CHUNK = 100;
const MAX_CONCURRENT_CHUNKS = 2;
const RETRY_COOLDOWN_MS = 5000;

/**
 * @typedef {{
 *   status: 'pending'|'ready'|'partial'|'stale'|'error',
 *   hasData: boolean,
 *   outgoing: object[],
 *   incoming: object[],
 *   hasMoreLinks: boolean,
 *   nextCursor: string|null,
 *   fetchedAt: number,
 *   failedAt: number,
 * }} DependencyEntry
 */

/**
 * Create a scope-owned dependency loader. Call `reset()` on scope change to
 * synchronously invalidate pending writes.
 */
export function createDependencyLoader(options = {}) {
  const includeCustomFields = options.includeCustomFields === true;
  const chunkSize = options.chunkSize ?? DEPENDENCY_LINK_CHUNK;
  const maxConcurrentChunks = options.maxConcurrentChunks ?? MAX_CONCURRENT_CHUNKS;
  const retryCooldownMs = options.retryCooldownMs ?? RETRY_COOLDOWN_MS;
  const now = options.now ?? (() => Date.now());
  const fetchGroups =
    options.fetchGroups ??
    ((ids) => links.getForItems(ids, { includeCustomFields, ...BACKGROUND_READ }));

  let entries = $state(/** @type {Record<number, DependencyEntry>} */ ({}));
  let scopeGeneration = 0;
  const inFlightIds = new Set();
  /** @type {Map<number, Array<(settled: boolean) => void>>} */
  const waiters = new Map();
  /** @type {number[][]} */
  const chunkQueue = [];
  let activeChunks = 0;

  function waiterFor(id) {
    return new Promise((resolve) => {
      const list = waiters.get(id) ?? [];
      list.push(resolve);
      waiters.set(id, list);
    });
  }

  function resolveWaiters(id, settled) {
    const list = waiters.get(id);
    if (!list) return;
    waiters.delete(id);
    for (const resolve of list) resolve(settled);
  }

  function applyChunk(generation, chunk, groups, error) {
    if (generation !== scopeGeneration) return;
    const next = { ...entries };
    const settled = new Map();
    const timestamp = now();

    for (const id of chunk) {
      inFlightIds.delete(id);
      const prior = entries[id];
      // hasData marks real successful data worth retaining across failures.
      const priorUsable = prior?.hasData === true;
      const group = error ? undefined : groups?.[id];

      if (!group) {
        // A failed chunk or an omitted group is unknown, never a confirmed
        // empty list. Retain prior successful data as stale.
        next[id] = priorUsable
          ? { ...prior, status: 'stale', failedAt: timestamp }
          : {
              status: 'error',
              hasData: false,
              outgoing: prior?.outgoing ?? [],
              incoming: prior?.incoming ?? [],
              hasMoreLinks: prior?.hasMoreLinks ?? false,
              nextCursor: prior?.nextCursor ?? null,
              fetchedAt: prior?.fetchedAt ?? 0,
              failedAt: timestamp,
            };
        settled.set(id, false);
        continue;
      }

      const outgoing = Array.isArray(group.outgoing) ? group.outgoing : [];
      const incoming = Array.isArray(group.incoming) ? group.incoming : [];
      const hasMoreLinks = Boolean(group.has_more_links);
      next[id] = {
        status: hasMoreLinks ? 'partial' : 'ready',
        hasData: true,
        outgoing,
        incoming,
        hasMoreLinks,
        nextCursor: group.next_cursor ?? null,
        fetchedAt: timestamp,
        failedAt: 0,
      };
      settled.set(id, true);
    }

    entries = next;
    for (const [id, ok] of settled) resolveWaiters(id, ok);
  }

  function pump() {
    while (activeChunks < maxConcurrentChunks && chunkQueue.length > 0) {
      const chunk = chunkQueue.shift();
      activeChunks += 1;
      const generation = scopeGeneration;
      Promise.resolve()
        .then(() => fetchGroups(chunk))
        .then(
          (groups) => applyChunk(generation, chunk, groups, null),
          (error) => applyChunk(generation, chunk, null, error)
        )
        .finally(() => {
          activeChunks -= 1;
          pump();
        });
    }
  }

  function ensure(ids) {
    const unique = [
      ...new Set((ids ?? []).map(Number).filter((id) => Number.isInteger(id) && id > 0)),
    ];
    const timestamp = now();
    const eligible = [];
    const pending = [];

    for (const id of unique) {
      const entry = entries[id];
      if (entry?.status === 'pending' || inFlightIds.has(id)) {
        pending.push(waiterFor(id));
        continue;
      }
      if (entry?.status === 'error') {
        if (timestamp - (entry.failedAt ?? 0) < retryCooldownMs) {
          pending.push(Promise.resolve(false));
          continue;
        }
        eligible.push(id);
        continue;
      }
      if (entry?.status === 'stale') {
        // Retained data is stale; an explicit reconciliation refetches it.
        eligible.push(id);
        continue;
      }
      if (entry) {
        // ready/partial data is retained; only an explicit invalidate refetches.
        continue;
      }
      eligible.push(id);
    }

    if (eligible.length > 0) {
      const next = { ...entries };
      for (const id of eligible) {
        next[id] = {
          status: 'pending',
          hasData: entries[id]?.hasData ?? false,
          outgoing: entries[id]?.outgoing ?? [],
          incoming: entries[id]?.incoming ?? [],
          hasMoreLinks: entries[id]?.hasMoreLinks ?? false,
          nextCursor: entries[id]?.nextCursor ?? null,
          fetchedAt: entries[id]?.fetchedAt ?? 0,
          failedAt: 0,
        };
        inFlightIds.add(id);
      }
      entries = next;

      for (let i = 0; i < eligible.length; i += chunkSize) {
        chunkQueue.push(eligible.slice(i, i + chunkSize));
      }
      pump();
      for (const id of eligible) pending.push(waiterFor(id));
    }

    return Promise.all(pending).then(() => undefined);
  }

  function get(id) {
    return entries[Number(id)] ?? null;
  }

  function merged(id) {
    const entry = get(id);
    if (!entry) return [];
    return [...entry.outgoing, ...entry.incoming];
  }

  /** Mark retained data stale so the next ensure refetches it. */
  function invalidate(ids) {
    const next = { ...entries };
    for (const id of ids ?? []) {
      const key = Number(id);
      if (next[key]) next[key] = { ...next[key], status: 'stale' };
    }
    entries = next;
  }

  /** Synchronously invalidate pending writes; in-flight results are dropped. */
  function reset() {
    scopeGeneration += 1;
    chunkQueue.length = 0;
    inFlightIds.clear();
    entries = {};
    for (const [id, list] of waiters) {
      waiters.delete(id);
      for (const resolve of list) resolve(false);
    }
  }

  return {
    get entries() {
      return entries;
    },
    ensure,
    get,
    merged,
    invalidate,
    reset,
  };
}
