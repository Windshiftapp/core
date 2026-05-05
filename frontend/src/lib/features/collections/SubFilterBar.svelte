<script>
  import { Filter, Plus, X } from 'lucide-svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { collectionStore } from '../../stores/collectionContext.svelte.js';
  import { QLBuilder } from '../../utils/ql.js';
  import DynamicFieldFilter from '../items/DynamicFieldFilter.svelte';
  import Button from '../../components/Button.svelte';

  let { workspaceId } = $props();

  let showFilters = $state(false);
  // Hydrate from the store so the builder reflects the active subfilter when
  // this view is mounted (e.g. after switching from List to Board on the same
  // collection). Cloned so in-progress edits don't mutate store state before Apply.
  let filters = $state(JSON.parse(JSON.stringify(collectionStore.subFilterRows ?? [])));

  let activeFilterCount = $derived(
    filters.filter(f => f.field && (f.value || (f.values && f.values.length > 0))).length
  );

  function addFilter() {
    filters = [...filters, { field: null, operator: '=', value: '', values: [] }];
  }

  function handleFilterChange(index, data) {
    filters = filters.map((f, i) => i === index ? data : f);
  }

  function handleFilterRemove(index) {
    filters = filters.filter((_, i) => i !== index);
    // If we removed the last filter and had an active sub-filter, clear it
    if (filters.length === 0 && collectionStore.subFilterQL) {
      collectionStore.clearSubFilter();
    }
  }

  function applyFilters() {
    const ql = QLBuilder.buildQuery({ dynamicFields: filters });
    if (ql) {
      collectionStore.setSubFilter(ql, JSON.parse(JSON.stringify(filters)));
    } else {
      collectionStore.clearSubFilter();
    }
    showFilters = false;
  }

  function clearAll() {
    filters = [];
    collectionStore.clearSubFilter();
  }

  function toggleFilters() {
    showFilters = !showFilters;
    if (showFilters && filters.length === 0) {
      addFilter();
    }
  }
</script>

<div class="relative">
  <!-- Toggle Button: design-system Button with `selected` variant when active. -->
  <Button
    variant={activeFilterCount > 0 ? 'selected' : 'ghost'}
    size="medium"
    icon={Filter}
    onclick={toggleFilters}
  >
    <span>{t('common.filter') || 'Filter'}</span>
    {#if activeFilterCount > 0}
      <span
        class="inline-flex items-center justify-center min-w-[1.25rem] h-5 px-1.5 text-xs font-semibold rounded-full bg-white/25"
      >
        {activeFilterCount}
      </span>
    {/if}
  </Button>

  <!-- Filter Panel -->
  {#if showFilters}
    <div
      class="absolute left-0 top-full mt-2 z-20 rounded-lg border shadow-lg p-3 min-w-[400px]"
      style="background-color: var(--ds-surface-overlay); border-color: var(--ds-border);"
    >
      <!-- Filter Rows -->
      <div class="flex flex-col gap-2">
        {#each filters as filter, index (index)}
          <DynamicFieldFilter
            {filter}
            compact={true}
            onchange={(data) => handleFilterChange(index, data)}
            onremove={() => handleFilterRemove(index)}
            onexecute={applyFilters}
          />
        {/each}
      </div>

      <!-- Actions -->
      <div class="flex items-center justify-between mt-3 pt-3 border-t" style="border-color: var(--ds-border);">
        <Button variant="ghost" size="sm" icon={Plus} onclick={addFilter}>
          {t('common.addFilter') || 'Add filter'}
        </Button>

        <div class="flex items-center gap-2">
          {#if activeFilterCount > 0 || collectionStore.subFilterQL}
            <Button variant="ghost" size="sm" icon={X} onclick={clearAll}>
              {t('common.clear') || 'Clear'}
            </Button>
          {/if}
          <Button variant="primary" size="sm" onclick={applyFilters}>
            {t('common.apply') || 'Apply'}
          </Button>
        </div>
      </div>
    </div>
  {/if}
</div>

<!-- Click-outside to close -->
{#if showFilters}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="fixed inset-0 z-10"
    onmousedown={() => showFilters = false}
  ></div>
{/if}
