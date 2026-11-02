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
  // Resolve the default level once per scope, after the counts have loaded, so
  // a premature all-zero snapshot cannot pin the shallowest level.
  let resolvedScope = null;

  const itemTypes = $derived(workspaceDataStore.itemTypes);

  const levels = $derived.by(() => {
    const byLevel = new Map();
    for (const type of itemTypes) {
      const level = Number(type.hierarchy_level ?? 0);
      if (level === GENERIC_SUBTASK_HIERARCHY_LEVEL) continue;
      if (!byLevel.has(level)) byLevel.set(level, { level, types: [], name: '', total: 0 });
      byLevel.get(level).types.push(type);
    }
    // The counts also cover levels with no configured item type (for example an
    // item with no type, which counts at level 0). Include them so those items
    // are reachable.
    for (const entry of counts) {
      const level = Number(entry.level);
      if (level === GENERIC_SUBTASK_HIERARCHY_LEVEL) continue;
      if (!byLevel.has(level)) byLevel.set(level, { level, types: [], name: '', total: 0 });
      const bucket = byLevel.get(level);
      bucket.total = entry.total ?? 0;
      if (!bucket.name) bucket.name = entry.name ?? '';
    }
    return [...byLevel.values()]
      .map((entry) => ({
        ...entry,
        label: entry.types.length
          ? entry.types.map((type) => type.name).join(', ')
          : entry.name || `Level ${entry.level}`,
      }))
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

  // Keep a persisted level that still has data; otherwise use the shallowest
  // level with matches, falling back to the shallowest level present.
  $effect(() => {
    const scope = `${workspaceId}|${collectionId}|${view}`;
    if (levels.length === 0 || loadingCounts) return;
    if (resolvedScope === scope) return;
    resolvedScope = scope;
    const current = collectionStore.hierarchyLevel;
    const currentEntry = levels.find((entry) => entry.level === current);
    if (currentEntry && currentEntry.total > 0) return;
    const withMatches = levels.find((entry) => entry.total > 0);
    collectionStore.setHierarchyLevel((withMatches ?? levels[0]).level);
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
