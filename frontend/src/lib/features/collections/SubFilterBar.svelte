<script>
  import { collectionStore } from '../../stores/collectionContext.svelte.js';
  import { QLBuilder } from '../../utils/ql.js';
  import DynamicFieldFilter from '../items/DynamicFieldFilter.svelte';
  import DynamicFilterPopover from '../shared/DynamicFilterPopover.svelte';

  let { workspaceId: _workspaceId = null } = $props();

  let filters = $state(JSON.parse(JSON.stringify(collectionStore.subFilterRows ?? [])));

  function applyFilters(rows) {
    const ql = QLBuilder.buildQuery({ dynamicFields: rows });
    if (ql) collectionStore.setSubFilter(ql, JSON.parse(JSON.stringify(rows)));
    else collectionStore.clearSubFilter();
  }
</script>

<DynamicFilterPopover
  bind:filters
  applied={Boolean(collectionStore.subFilterQL)}
  onapply={applyFilters}
  onclear={() => collectionStore.clearSubFilter()}
>
  {#snippet filterRow({ filter, onchange, onremove, onexecute })}
    <DynamicFieldFilter
      {filter}
      compact
      {onchange}
      {onremove}
      {onexecute}
    />
  {/snippet}
</DynamicFilterPopover>
