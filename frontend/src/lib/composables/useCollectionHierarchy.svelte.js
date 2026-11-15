import { onDestroy, untrack } from 'svelte';
import { items as itemsApi } from '../api/items.js';
import { BACKGROUND_READ } from '../api/readScheduler.js';
import { collectionStore } from '../stores/collectionContext.js';
import { collectionPreferences } from '../stores/collectionPreferences.svelte.js';

const DEFAULT_PER_ROOT_LIMIT = 100;
const RESPONSE_BUDGET = 400;

function compareItems(a, b) {
  const aIndex = a?.frac_index ?? '';
  const bIndex = b?.frac_index ?? '';
  if (aIndex !== bIndex) return aIndex < bIndex ? -1 : 1;
  return (a?.id ?? 0) - (b?.id ?? 0);
}

function mergeById(existing, incoming) {
  const seen = new Set(existing.map((item) => item.id));
  const merged = [...existing];
  for (const item of incoming) {
    if (seen.has(item.id)) continue;
    seen.add(item.id);
    merged.push(item);
  }
  return merged;
}

/**
 * Shared lazy hierarchy loader for the tree, map, and roadmap.
 *
 * The view selects one item-type level (see collectionStore.hierarchyLevel) and
 * the store owns the level-scoped base page. This composable owns everything
 * below it:
 *
 * - loads matching descendants only for expanded roots, in bounded batches,
 * - pages within a root for a per-row "Load more",
 * - flattens a matching descendant whose intermediate parent is filtered out
 *   onto the nearest present ancestor (the node whose subtree fetch returned
 *   it), and
 * - negative-caches a loaded-but-empty subtree so a filter-agnostic
 *   `has_children` cannot leave a dead expander.
 *
 * @param {object} options
 * @param {() => number|null} [options.getWorkspaceId]
 * @param {() => number|null} [options.getCollectionId]
 * @param {() => string|null} [options.getView] View key used to persist expansion.
 * @param {() => number} [options.getMaxDepth] 0 = full matching descendants.
 * @param {() => number} [options.getPerRootLimit]
 * @param {() => string} [options.getSubFilterQL]
 */
export function useCollectionHierarchy({
  getWorkspaceId = () => null,
  getCollectionId = () => null,
  getView = () => null,
  getMaxDepth = () => 0,
  getPerRootLimit = () => DEFAULT_PER_ROOT_LIMIT,
  getSubFilterQL = () => '',
} = {}) {
  // rootId -> { items: HierarchyItem[], total, hasMore }
  let overlayByRoot = $state({});
  // Roots whose subtree has been fetched at least once (drives negative cache).
  let loadedRoots = $state(new Set());
  let loadingRoots = $state(new Set());
  let expanded = $state(new Set());
  // Bumped whenever the base set is replaced. A subtree response whose
  // generation no longer matches is dropped instead of merged, so a fetch for
  // a previous view, level, or filter cannot change the current hierarchy.
  let fetchGeneration = 0;
  let controller = new AbortController();
  let queue = Promise.resolve();
  onDestroy(() => controller.abort());

  let baseSignature = null;
  let previousBaseIds = null;

  const baseItems = $derived(collectionStore.items);
  const baseItemIds = $derived(new Set(baseItems.map((item) => item.id)));

  const itemById = $derived.by(() => {
    const map = new Map();
    for (const item of baseItems) map.set(item.id, item);
    for (const group of Object.values(overlayByRoot)) {
      for (const entry of group.items) map.set(entry.id, entry);
    }
    return map;
  });

  const allItems = $derived.by(() => {
    const seen = new Set(baseItems.map((item) => item.id));
    const merged = [...baseItems];
    for (const group of Object.values(overlayByRoot)) {
      for (const entry of group.items) {
        if (seen.has(entry.id)) continue;
        seen.add(entry.id);
        merged.push(entry);
      }
    }
    return merged;
  });

  const completeDescendants = $derived.by(() => {
    const ids = new Set();
    if (getMaxDepth() !== 0) return ids;
    for (const group of Object.values(overlayByRoot)) {
      if (!group.hasMore) for (const item of group.items) ids.add(item.id);
    }
    return ids;
  });

  // Descendant id -> the root whose subtree fetch returned it. Used to flatten
  // a matching descendant onto its nearest present ancestor when its real
  // parent is filtered out.
  const rootIdByItemId = $derived.by(() => {
    const map = new Map();
    for (const [rootId, group] of Object.entries(overlayByRoot)) {
      for (const entry of group.items) map.set(entry.id, Number(rootId));
    }
    return map;
  });

  const hierarchyIndex = $derived.by(() => {
    const baseIds = new Set(baseItems.map((item) => item.id));
    const ids = new Set(allItems.map((item) => item.id));
    const childrenByParent = new Map();
    const roots = [];
    for (const item of allItems) {
      if (baseIds.has(item.id)) {
        roots.push(item);
        continue;
      }
      const parentId = item.parent_id;
      if (parentId != null && ids.has(parentId)) {
        const children = childrenByParent.get(parentId);
        if (children) children.push(item);
        else childrenByParent.set(parentId, [item]);
        continue;
      }
      // Flatten: attach to the node whose subtree fetch produced this item.
      const rootId = rootIdByItemId.get(item.id);
      if (rootId != null && ids.has(rootId)) {
        const children = childrenByParent.get(rootId);
        if (children) children.push(item);
        else childrenByParent.set(rootId, [item]);
      } else {
        roots.push(item);
      }
    }
    // Base items arrive in the API's rank order, so the root list keeps that
    // order. Only child buckets need a sort: flattening can merge descendants
    // from different depths into one parent, and the subtree read is
    // breadth-first.
    for (const children of childrenByParent.values()) children.sort(compareItems);
    return { roots, childrenByParent };
  });

  function hasChildren(itemId) {
    if (hierarchyIndex.childrenByParent.has(itemId)) return true;
    if (isLoaded(itemId)) return false;
    return itemById.get(itemId)?.has_children === true;
  }

  function getChildren(itemId) {
    return hierarchyIndex.childrenByParent.get(itemId) ?? [];
  }

  function isExpanded(itemId) {
    return expanded.has(itemId);
  }

  function hasMore(itemId) {
    return overlayByRoot[itemId]?.hasMore === true;
  }

  function remaining(itemId) {
    const group = overlayByRoot[itemId];
    if (!group) return 0;
    return Math.max(0, (group.total ?? 0) - group.items.length);
  }

  function isLoading(itemId) {
    return loadingRoots.has(itemId);
  }

  function isLoaded(itemId) {
    return loadedRoots.has(itemId) || completeDescendants.has(itemId);
  }

  function isExposed(itemId) {
    const seen = new Set();
    while (!baseItemIds.has(itemId) && !seen.has(itemId)) {
      seen.add(itemId);
      const actualParent = itemById.get(itemId)?.parent_id;
      const parent = itemById.has(actualParent) ? actualParent : rootIdByItemId.get(itemId);
      if (parent == null) return true;
      if (!expanded.has(parent)) return false;
      itemId = parent;
    }
    return true;
  }

  function enqueue(read) {
    const signal = controller.signal;
    const run = queue.then(async () => {
      if (!signal.aborted) return await read(signal);
    });
    queue = run.then(
      () => {},
      () => {}
    );
    return run;
  }

  async function fetchRoots(rootIds, { force = false } = {}) {
    const ids = [...new Set(rootIds)].filter(
      (id) => !loadingRoots.has(id) && (force || !isLoaded(id))
    );
    if (ids.length === 0) return;
    const generation = fetchGeneration;
    loadingRoots = new Set([...loadingRoots, ...ids]);
    try {
      const perRootLimit = Math.min(RESPONSE_BUDGET, Math.max(1, getPerRootLimit()));
      const rootsPerBatch = Math.max(1, Math.floor(RESPONSE_BUDGET / perRootLimit));
      const scope = {
        workspaceId: getWorkspaceId(),
        collectionId: getCollectionId(),
        subQl: getSubFilterQL() || null,
        maxDepth: getMaxDepth(),
        perRootLimit,
        perRootOffset: 0,
      };
      await enqueue(async (signal) => {
        for (let index = 0; index < ids.length; index += rootsPerBatch) {
          if (signal.aborted || generation !== fetchGeneration) return;
          const batch = ids.slice(index, index + rootsPerBatch);
          // A user may collapse queued roots before their batch starts.
          const needed = batch.filter((id) => expanded.has(id) && isExposed(id));
          if (!needed.length) continue;
          const groups = await itemsApi.getHierarchy(
            { ...scope, rootIds: needed },
            { ...BACKGROUND_READ, signal }
          );
          if (signal.aborted || generation !== fetchGeneration) return;
          const next = { ...overlayByRoot };
          const nextLoaded = new Set(loadedRoots);
          for (const group of groups) {
            next[group.root_id] = {
              items: group.items ?? [],
              total: group.total ?? 0,
              hasMore: group.has_more === true,
            };
            nextLoaded.add(group.root_id);
          }
          overlayByRoot = next;
          loadedRoots = nextLoaded;
          const nested = [...expanded].filter(
            (id) => isExposed(id) && itemById.get(id)?.has_children && !isLoaded(id)
          );
          if (nested.length) void fetchRoots(nested);
        }
      });
    } catch (error) {
      if (
        generation !== fetchGeneration ||
        controller.signal.aborted ||
        error?.name === 'AbortError'
      )
        return;
      console.error('[useCollectionHierarchy] failed to load descendants:', error);
    } finally {
      if (generation === fetchGeneration) {
        const pending = new Set(loadingRoots);
        for (const id of ids) pending.delete(id);
        loadingRoots = pending;
      }
    }
  }

  /** Fetch the next page of a root's descendants (per-row "Load more"). */
  async function loadMore(itemId) {
    const group = overlayByRoot[itemId];
    if (!group?.hasMore || loadingRoots.has(itemId)) return;
    const generation = fetchGeneration;
    loadingRoots = new Set([...loadingRoots, itemId]);
    try {
      const scope = {
        rootIds: [itemId],
        workspaceId: getWorkspaceId(),
        collectionId: getCollectionId(),
        subQl: getSubFilterQL() || null,
        maxDepth: getMaxDepth(),
        perRootLimit: Math.min(RESPONSE_BUDGET, Math.max(1, getPerRootLimit())),
        perRootOffset: group.items.length,
      };
      const groups = await enqueue((signal) =>
        itemsApi.getHierarchy(scope, { ...BACKGROUND_READ, signal })
      );
      if (generation !== fetchGeneration || controller.signal.aborted) return;
      if (!groups) return;
      const next = groups.find((entry) => entry.root_id === itemId);
      if (!next) return;
      overlayByRoot = {
        ...overlayByRoot,
        [itemId]: {
          items: mergeById(group.items, next.items ?? []),
          total: next.total ?? group.total,
          hasMore: next.has_more === true,
        },
      };
    } catch (error) {
      if (
        generation !== fetchGeneration ||
        controller.signal.aborted ||
        error?.name === 'AbortError'
      )
        return;
      console.error('[useCollectionHierarchy] failed to load more descendants:', error);
    } finally {
      if (generation === fetchGeneration) {
        const pending = new Set(loadingRoots);
        pending.delete(itemId);
        loadingRoots = pending;
      }
    }
  }

  /** Load a node's descendants if it has not been fetched yet. */
  function ensureLoaded(itemId) {
    if (isLoaded(itemId) || loadingRoots.has(itemId)) return;
    if (!itemById.get(itemId)?.has_children) return;
    void fetchRoots([itemId]);
  }

  // The expanded ids are stored per scope and view so a reload restores the
  // tree the user left. A stored key (even an empty list) is authoritative over
  // the default "roots with children start expanded".
  const EXPANDED_STATE_KEY = 'expanded_ids';

  function readStoredExpanded() {
    const view = getView();
    if (!view) return null;
    const stored = collectionPreferences.viewStateValue(
      getWorkspaceId(),
      getCollectionId(),
      view,
      EXPANDED_STATE_KEY
    );
    return Array.isArray(stored) ? new Set(stored) : null;
  }

  function persistExpanded() {
    const view = getView();
    if (!view) return;
    const workspaceId = getWorkspaceId();
    const collectionId = getCollectionId();
    if (workspaceId == null && collectionId == null) return;
    collectionPreferences.patchViewState(workspaceId, collectionId, view, {
      [EXPANDED_STATE_KEY]: [...expanded],
    });
  }

  function expand(itemId) {
    expanded.add(itemId);
    expanded = new Set(expanded);
    for (const id of expanded) if (isExposed(id)) ensureLoaded(id);
    persistExpanded();
  }

  function collapse(itemId) {
    expanded.delete(itemId);
    expanded = new Set(expanded);
    persistExpanded();
  }

  function toggleExpanded(itemId) {
    if (expanded.has(itemId)) collapse(itemId);
    else expand(itemId);
  }

  function collapseAll() {
    expanded = new Set();
    persistExpanded();
  }

  function expandAll() {
    const next = new Set(expanded);
    const toLoad = [];
    for (const item of allItems) {
      if (!hasChildren(item.id)) continue;
      next.add(item.id);
      if (!isLoaded(item.id) && !loadingRoots.has(item.id)) toLoad.push(item.id);
    }
    expanded = next;
    if (toLoad.length > 0) void fetchRoots(toLoad);
    persistExpanded();
  }

  function resetReads() {
    controller.abort();
    controller = new AbortController();
    queue = Promise.resolve();
    fetchGeneration += 1;
    loadingRoots = new Set();
  }

  // Refresh ancestors first; exposed expanded children hydrate from fresh results.
  function coveringRoots(ids) {
    const selected = new Set(ids);
    return ids.filter((id) => {
      const seen = new Set([id]);
      for (;;) {
        const actualParent = itemById.get(id)?.parent_id;
        const parent = itemById.has(actualParent) ? actualParent : rootIdByItemId.get(id);
        if (parent == null || seen.has(parent)) return true;
        if (selected.has(parent)) return false;
        seen.add(parent);
        id = parent;
      }
    });
  }

  let lastGeneration = null;
  let lastScope = null;
  $effect(() => {
    const signature = baseItems.map((item) => item.id).join(',');
    const generation = collectionStore.dataGeneration;
    const scope = JSON.stringify([
      getWorkspaceId(),
      getCollectionId(),
      getSubFilterQL(),
      getMaxDepth(),
    ]);
    untrack(() => syncBase(signature, generation, scope));
  });

  function syncBase(signature, generation, scope) {
    const generationChanged = lastGeneration !== null && lastGeneration !== generation;
    lastGeneration = generation;
    const scopeChanged = lastScope !== scope;
    lastScope = scope;

    if (!scopeChanged && signature === baseSignature) {
      if (generationChanged) {
        if (readStoredExpanded() == null) {
          expanded = new Set([
            ...expanded,
            ...baseItems.filter((item) => item.has_children).map((item) => item.id),
          ]);
        }
        const roots = coveringRoots(
          [...expanded].filter((id) => isExposed(id) && itemById.get(id)?.has_children)
        );
        resetReads();
        overlayByRoot = {};
        loadedRoots = new Set();
        void fetchRoots(roots);
      }
      return;
    }
    const nextIds = new Set(baseItems.map((item) => item.id));
    const isAppend =
      !scopeChanged &&
      previousBaseIds !== null &&
      [...previousBaseIds].every((id) => nextIds.has(id));
    // Read without subscribing: preferences are written by the actions below,
    // and a tracked read would re-run this effect on every save.
    const stored = untrack(readStoredExpanded);
    if (!isAppend) {
      overlayByRoot = {};
      loadedRoots = new Set();
      expanded = stored ?? new Set();
      resetReads();
    }
    baseSignature = signature;
    previousBaseIds = nextIds;

    // Base roots with children start expanded until the view has a stored
    // expansion, after which the user's choice is authoritative.
    if (stored == null) {
      const nextExpanded = new Set(expanded);
      for (const item of baseItems) {
        if (item.has_children) nextExpanded.add(item.id);
      }
      expanded = nextExpanded;
      if (expanded.size > 0) persistExpanded();
    }
    const toLoad = baseItems
      .filter((item) => item.has_children && expanded.has(item.id) && !isLoaded(item.id))
      .map((item) => item.id);
    void fetchRoots(toLoad);
  }

  return {
    get allItems() {
      return allItems;
    },
    get roots() {
      return hierarchyIndex.roots;
    },
    get hierarchyIndex() {
      return hierarchyIndex;
    },
    get baseItems() {
      return baseItems;
    },
    get expandedCount() {
      return expanded.size;
    },
    getChildren,
    hasChildren,
    isExpanded,
    toggleExpanded,
    expand,
    collapse,
    expandAll,
    collapseAll,
    ensureLoaded,
    loadMore,
    hasMore,
    remaining,
    isLoading,
    isLoaded,
  };
}
