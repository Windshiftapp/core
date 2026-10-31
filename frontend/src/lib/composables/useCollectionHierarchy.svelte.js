import { items as itemsApi } from '../api/items.js';
import { collectionStore } from '../stores/collectionContext.js';

const DEFAULT_PER_ROOT_LIMIT = 100;

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
 * - eager-loads each base item's matching descendants (one batched call),
 * - loads a node's descendants on demand when it is expanded,
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
 * @param {() => number} [options.getMaxDepth] 0 = full matching descendants.
 * @param {() => number} [options.getPerRootLimit]
 * @param {() => string} [options.getSubFilterQL]
 */
export function useCollectionHierarchy({
  getWorkspaceId = () => null,
  getCollectionId = () => null,
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

  let baseSignature = null;
  let previousBaseIds = null;

  const baseItems = $derived(collectionStore.items);

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
    roots.sort(compareItems);
    for (const children of childrenByParent.values()) children.sort(compareItems);
    return { roots, childrenByParent };
  });

  function hasChildren(itemId) {
    if (hierarchyIndex.childrenByParent.has(itemId)) return true;
    if (loadedRoots.has(itemId)) return false;
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
    return loadedRoots.has(itemId);
  }

  async function fetchRoots(rootIds, { offsetByRoot = {}, force = false } = {}) {
    const ids = [...new Set(rootIds)].filter(
      (id) => !loadingRoots.has(id) && (force || offsetByRoot[id] != null || !loadedRoots.has(id))
    );
    if (ids.length === 0) return;
    loadingRoots = new Set([...loadingRoots, ...ids]);
    try {
      const groups = await itemsApi.getHierarchy({
        rootIds: ids,
        workspaceId: getWorkspaceId(),
        collectionId: getCollectionId(),
        subQl: getSubFilterQL() || null,
        maxDepth: getMaxDepth(),
        perRootLimit: getPerRootLimit(),
        perRootOffset: 0,
      });
      const next = { ...overlayByRoot };
      const nextLoaded = new Set(loadedRoots);
      for (const group of groups) {
        const offset = offsetByRoot[group.root_id];
        const existing = offset != null ? (next[group.root_id]?.items ?? []) : [];
        next[group.root_id] = {
          items: mergeById(existing, group.items ?? []),
          total: group.total ?? 0,
          hasMore: group.has_more === true,
        };
        nextLoaded.add(group.root_id);
        // A complete unlimited-depth fetch contains every descendant subtree,
        // so those nodes are already loaded and must not refetch on expand.
        if (!group.has_more && getMaxDepth() === 0) {
          for (const entry of group.items ?? []) nextLoaded.add(entry.id);
        }
      }
      overlayByRoot = next;
      loadedRoots = nextLoaded;
    } catch (error) {
      console.error('[useCollectionHierarchy] failed to load descendants:', error);
    } finally {
      const pending = new Set(loadingRoots);
      for (const id of ids) pending.delete(id);
      loadingRoots = pending;
    }
  }

  /** Fetch the next page of a root's descendants (per-row "Load more"). */
  async function loadMore(itemId) {
    const group = overlayByRoot[itemId];
    if (!group?.hasMore || loadingRoots.has(itemId)) return;
    loadingRoots = new Set([...loadingRoots, itemId]);
    try {
      const groups = await itemsApi.getHierarchy({
        rootIds: [itemId],
        workspaceId: getWorkspaceId(),
        collectionId: getCollectionId(),
        subQl: getSubFilterQL() || null,
        maxDepth: getMaxDepth(),
        perRootLimit: getPerRootLimit(),
        perRootOffset: group.items.length,
      });
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
      console.error('[useCollectionHierarchy] failed to load more descendants:', error);
    } finally {
      const pending = new Set(loadingRoots);
      pending.delete(itemId);
      loadingRoots = pending;
    }
  }

  /** Load a node's descendants if it has not been fetched yet. */
  function ensureLoaded(itemId) {
    if (loadedRoots.has(itemId) || loadingRoots.has(itemId)) return;
    if (!itemById.get(itemId)?.has_children) return;
    void fetchRoots([itemId]);
  }

  function expand(itemId) {
    expanded.add(itemId);
    expanded = new Set(expanded);
    ensureLoaded(itemId);
  }

  function collapse(itemId) {
    expanded.delete(itemId);
    expanded = new Set(expanded);
  }

  function toggleExpanded(itemId) {
    if (expanded.has(itemId)) collapse(itemId);
    else expand(itemId);
  }

  function collapseAll() {
    expanded = new Set();
  }

  function expandAll() {
    const next = new Set(expanded);
    const toLoad = [];
    for (const item of allItems) {
      if (!hasChildren(item.id)) continue;
      next.add(item.id);
      if (!loadedRoots.has(item.id) && !loadingRoots.has(item.id)) toLoad.push(item.id);
    }
    expanded = next;
    if (toLoad.length > 0) void fetchRoots(toLoad);
  }

  // Reset the overlay when the base set is replaced (level or scope change) and
  // eager-load the base items' subtrees. Appending a base page keeps the
  // already-loaded subtrees. A store refresh that leaves the base rows
  // unchanged still re-fetches the loaded subtrees so new descendants appear.
  let lastGeneration = null;
  $effect(() => {
    const signature = baseItems.map((item) => item.id).join(',');
    const generation = collectionStore.dataGeneration;
    const generationChanged = lastGeneration !== null && lastGeneration !== generation;
    lastGeneration = generation;

    if (signature === baseSignature) {
      // An authoritative refresh with the same base rows can still change
      // has_children or add descendants, so re-fetch every known root plus any
      // base row that now reports children.
      if (generationChanged) {
        const roots = new Set(loadedRoots);
        for (const item of baseItems) {
          if (item.has_children) roots.add(item.id);
        }
        if (roots.size > 0) void fetchRoots([...roots], { force: true });
      }
      return;
    }
    const nextIds = new Set(baseItems.map((item) => item.id));
    const isAppend =
      previousBaseIds !== null && [...previousBaseIds].every((id) => nextIds.has(id));
    if (!isAppend) {
      overlayByRoot = {};
      loadedRoots = new Set();
      loadingRoots = new Set();
      expanded = new Set();
    }
    baseSignature = signature;
    previousBaseIds = nextIds;

    const toLoad = baseItems
      .filter((item) => item.has_children && !loadedRoots.has(item.id))
      .map((item) => item.id);
    if (toLoad.length > 0) void fetchRoots(toLoad);

    // Base roots with children start expanded, matching the previous default.
    const nextExpanded = new Set(expanded);
    for (const item of baseItems) {
      if (item.has_children) nextExpanded.add(item.id);
    }
    expanded = nextExpanded;
  });

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
