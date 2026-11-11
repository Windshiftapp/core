<script>
  import { collectionStore } from '../../stores/collectionContext.js';
  import { workspaceDataStore } from '../../stores/workspaceDataStore.svelte.js';
  import {
    buildHierarchyLevels,
    defaultHierarchyLevel,
  } from './hierarchyLevels.js';
  import HierarchyLevelPicker from './HierarchyLevelPicker.svelte';

  /**
   * Level filter for the hierarchy views. Builds the level options from the
   * workspace's item types (one option per level, every item type shown) plus
   * the filtered counts. Selection and persistence are owned by the store.
   *
   * @type {{
   *   workspaceId: number|null,
   *   collectionId?: number|null,
   * }}
   */
  let { workspaceId, collectionId = null } = $props();

  const itemTypes = $derived(workspaceDataStore.itemTypes);
  const counts = $derived(collectionStore.levelCounts);
  const loadingCounts = $derived(collectionStore.levelCountsLoading);

  const levels = $derived(buildHierarchyLevels(itemTypes, counts));

  // The store owns the counts cache and single-flight, so remounting a
  // hierarchy view does not re-issue the lookup. It also resolves the default
  // level during load, before the first item page.
  $effect(() => {
    const wsId = workspaceId;
    const colId = collectionId;
    const subQl = collectionStore.effectiveSubFilterQL;
    if (!wsId && !colId) return;
    void collectionStore.ensureLevelCounts(wsId, colId, subQl);
  });

  // Fallback for the rare case where the store could not resolve a level (for
  // example the counts request failed and no item types were known yet).
  $effect(() => {
    if (collectionStore.hierarchyLevel != null) return;
    if (loadingCounts || levels.length === 0) return;
    const level = defaultHierarchyLevel(levels);
    if (level != null) collectionStore.setHierarchyLevel(level);
  });

  function handleChange(level) {
    if (level == null) return;
    collectionStore.setHierarchyLevel(level);
  }
</script>

<HierarchyLevelPicker
  {levels}
  value={collectionStore.hierarchyLevel}
  loading={loadingCounts}
  onchange={handleChange}
/>
