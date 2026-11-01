<script>
  import { collectionStore } from '../../stores/collectionContext.svelte.js';
  import { QLBuilder } from '../../utils/ql.js';
  import Toggle from '../../components/Toggle.svelte';
  import FilterMenu from './FilterMenu.svelte';
  import { t } from '../../stores/i18n.svelte.js';

  let {
    workspaceId = null,
    collectionId = null,
    showCompletionToggle = true,
  } = $props();

  let filters = $state(JSON.parse(JSON.stringify(collectionStore.subFilterRows ?? [])));

  // A saved or shared filter replaces the whole stack, so rehydrate the local
  // rows from the stored set before applying its query.
  function applyFilters(rows, ql) {
    const nextRows = JSON.parse(JSON.stringify(rows ?? []));
    filters = nextRows;
    const query = ql ?? QLBuilder.buildQuery({ dynamicFields: nextRows }) ?? '';
    if (query) collectionStore.setSubFilter(query, nextRows);
    else collectionStore.clearSubFilter();
  }

  function clearFilters() {
    filters = [];
    collectionStore.clearSubFilter();
  }
</script>

<div class="flex flex-wrap items-center gap-3" data-testid="collection-sub-filters">
  {#if showCompletionToggle}
    <Toggle
      checked={!collectionStore.showCompleted}
      onchange={(checked) => collectionStore.setShowCompleted(!checked)}
      label={t('milestones.hideCompleted')}
      labelPosition="left"
      size="small"
      dataTestid="collection-hide-completed"
    />
  {/if}

  <FilterMenu
    {workspaceId}
    {collectionId}
    activeRows={filters}
    applied={Boolean(collectionStore.subFilterQL)}
    onapply={applyFilters}
    onclear={clearFilters}
  />
</div>
