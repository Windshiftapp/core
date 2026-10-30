<script>
  import { items as itemsApi } from '../../api/items.js';
  import { collectionStore } from '../../stores/collectionContext.svelte.js';
  import { collectionPreferences } from '../../stores/collectionPreferences.svelte.js';
  import { workspaceDataStore } from '../../stores/workspaceDataStore.svelte.js';
  import { GENERIC_SUBTASK_HIERARCHY_LEVEL } from '../../utils/hierarchy.js';
  import HierarchyLevelPicker from './HierarchyLevelPicker.svelte';

  /**
   * Level filter for the hierarchy views. Builds the level options from the
   * workspace's item types (one option per level, every item type shown) plus
   * the filtered counts, resolves the active level, and persists the choice.
   *
   * @type {{
   *   workspaceId: number|null,
   *   collectionId?: number|null,
   *   view?: string,
   * }}
   */
  let { workspaceId, collectionId = null, view = 'tree' } = $props();

  let counts = $state([]);
  let loadingCounts = $state(false);
  let requestId = 0;

  const itemTypes = $derived(workspaceDataStore.itemTypes);

  const levels = $derived.by(() => {
    const byLevel = new Map();
    for (const type of itemTypes) {
      const level = Number(type.hierarchy_level ?? 0);
      if (level === GENERIC_SUBTASK_HIERARCHY_LEVEL) continue;
      if (!byLevel.has(level)) byLevel.set(level, []);
      byLevel.get(level).push(type);
    }
    const countByLevel = new Map(counts.map((entry) => [Number(entry.level), entry.total]));
    return [...byLevel.entries()]
      .map(([level, types]) => ({ level, types, total: countByLevel.get(level) ?? 0 }))
      .sort((a, b) => a.level - b.level);
  });

  // Counts follow the same collection/sub-filter scope as the item list. A
  // response for a stale scope is discarded.
  $effect(() => {
    const wsId = workspaceId;
    const colId = collectionId;
    const subQl = collectionStore.effectiveSubFilterQL;
    const id = ++requestId;
    if (!wsId && !colId) return;
    loadingCounts = true;
    itemsApi
      .getLevelCounts(
        {
          workspace_id: wsId || undefined,
          collection_id: colId || undefined,
          sub_ql: subQl || undefined,
        },
        { readPolicy: { priority: 'background' } }
      )
      .then((result) => {
        if (id !== requestId) return;
        counts = Array.isArray(result) ? result : [];
      })
      .catch(() => {
        if (id !== requestId) return;
        counts = [];
      })
      .finally(() => {
        if (id === requestId) loadingCounts = false;
      });
  });

  // Keep the store's level when it still exists; otherwise use the shallowest
  // level with matches, falling back to the shallowest level present.
  $effect(() => {
    if (levels.length === 0) return;
    const current = collectionStore.hierarchyLevel;
    if (levels.some((entry) => entry.level === current)) return;
    const withMatches = levels.find((entry) => entry.total > 0);
    const next = (withMatches ?? levels[0]).level;
    if (next !== current) collectionStore.setHierarchyLevel(next);
  });

  function handleChange(level) {
    if (level == null) return;
    collectionPreferences.patchViewState(workspaceId, collectionId, view, {
      hierarchy_level: level,
    });
    collectionStore.setHierarchyLevel(level);
  }
</script>

<HierarchyLevelPicker
  {levels}
  value={collectionStore.hierarchyLevel}
  loading={loadingCounts}
  onchange={handleChange}
/>
