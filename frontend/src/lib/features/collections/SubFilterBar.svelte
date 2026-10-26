<script>
  import { collectionStore } from '../../stores/collectionContext.svelte.js';
  import { QLBuilder } from '../../utils/ql.js';
  import DynamicFieldFilter from '../items/DynamicFieldFilter.svelte';
  import DynamicFilterPopover from '../shared/DynamicFilterPopover.svelte';
  import Toggle from '../../components/Toggle.svelte';
  import SavedFilterMenu from './SavedFilterMenu.svelte';
  import CollectionViewsMenu from './CollectionViewsMenu.svelte';
  import { t } from '../../stores/i18n.svelte.js';

  let {
    workspaceId = null,
    collectionId = null,
    showCompletionToggle = true,
  } = $props();

  let filters = $state(JSON.parse(JSON.stringify(collectionStore.subFilterRows ?? [])));
  let activeQl = $derived(QLBuilder.buildQuery({ dynamicFields: filters }) || '');

  function applyFilters(rows) {
    const ql = QLBuilder.buildQuery({ dynamicFields: rows });
    if (ql) collectionStore.setSubFilter(ql, JSON.parse(JSON.stringify(rows)));
    else collectionStore.clearSubFilter();
  }

  // A saved filter replaces the whole stack, so rehydrate the builder rows
  // from the stored set before applying its query.
  function applySavedFilter(rows, ql) {
    const nextRows = JSON.parse(JSON.stringify(rows ?? []));
    filters = nextRows;
    if (ql) collectionStore.setSubFilter(ql, nextRows);
    else collectionStore.clearSubFilter();
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

  <DynamicFilterPopover
    bind:filters
    applied={Boolean(collectionStore.subFilterQL)}
    onapply={applyFilters}
    onclear={() => collectionStore.clearSubFilter()}
  >
    {#snippet filterRow({ filter, index, onchange, onremove, onexecute })}
      <DynamicFieldFilter
        {filter}
        compact
        testIdPrefix={`collection-dynamic-filter-${index}`}
        {onchange}
        {onremove}
        {onexecute}
      />
    {/snippet}
  </DynamicFilterPopover>

  <SavedFilterMenu
    {workspaceId}
    {collectionId}
    {activeQl}
    activeRows={filters}
    onapply={applySavedFilter}
  />

  <CollectionViewsMenu
    {workspaceId}
    {collectionId}
    {activeQl}
    activeRows={filters}
    onapply={applySavedFilter}
  />
</div>
