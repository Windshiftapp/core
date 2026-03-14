<script>
  import { Filter, Plus, X } from 'lucide-svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { collectionStore } from '../../stores/collectionContext.svelte.js';
  import { QLBuilder } from '../../utils/ql.js';
  import DynamicFieldFilter from '../items/DynamicFieldFilter.svelte';
  import Button from '../../components/Button.svelte';

  let { workspaceId, hasGradient = false } = $props();

  let showFilters = $state(false);
  let filters = $state([]);

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
      collectionStore.setSubFilter(ql);
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
  <!-- Toggle Button -->
  <button
    class="flex items-center gap-2 px-3 py-2 text-sm rounded-lg transition-colors"
    style="color: {hasGradient ? 'rgba(255,255,255,0.85)' : 'var(--ds-text-subtle)'}; {hasGradient ? 'background: rgba(255,255,255,0.12);' : ''}"
    onclick={toggleFilters}
  >
    <Filter class="w-4 h-4" />
    <span>{t('common.filter') || 'Filter'}</span>
    {#if activeFilterCount > 0}
      <span
        class="inline-flex items-center justify-center w-5 h-5 text-xs font-medium rounded-full"
        style="background-color: var(--ds-background-brand-bold); color: white;"
      >
        {activeFilterCount}
      </span>
    {/if}
  </button>

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
