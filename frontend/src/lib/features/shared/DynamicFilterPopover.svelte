<script>
  import { Filter, Plus, X } from '@lucide/svelte';
  import Button from '../../components/Button.svelte';
  import { t } from '../../stores/i18n.svelte.js';

  let {
    filters = $bindable([]),
    applied = false,
    panelMinWidth = '400px',
    filterRow,
    onapply = () => {},
    onclear = () => {},
  } = $props();

  let showFilters = $state(false);
  const activeFilterCount = $derived(
    filters.filter((filter) => filter.field && (filter.value || filter.values?.length)).length,
  );

  function addFilter() {
    filters = [...filters, { field: null, operator: '=', value: '', values: [] }];
  }

  function updateFilter(index, filter) {
    filters = filters.map((current, currentIndex) => currentIndex === index ? filter : current);
  }

  function removeFilter(index) {
    filters = filters.filter((_, currentIndex) => currentIndex !== index);
    if (filters.length === 0) onclear();
  }

  function applyFilters() {
    onapply(filters);
    showFilters = false;
  }

  function clearFilters() {
    filters = [];
    onclear();
  }

  function toggleFilters() {
    showFilters = !showFilters;
    if (showFilters && filters.length === 0) addFilter();
  }
</script>

<div class="relative">
  <Button
    variant={activeFilterCount > 0 ? 'selected' : 'ghost'}
    size="medium"
    icon={Filter}
    onclick={toggleFilters}
  >
    <span>{t('common.filter')}</span>
    {#if activeFilterCount > 0}
      <span class="inline-flex items-center justify-center min-w-[1.25rem] h-5 px-1.5 text-xs font-semibold rounded-full bg-white/25">
        {activeFilterCount}
      </span>
    {/if}
  </Button>

  {#if showFilters}
    <div
      class="absolute left-0 top-full mt-2 z-20 rounded-lg border shadow-lg p-3"
      style="min-width: {panelMinWidth}; background-color: var(--ds-surface-overlay); border-color: var(--ds-border);"
    >
      <div class="flex flex-col gap-2">
        {#each filters as filter, index (index)}
          {@render filterRow({
            filter,
            index,
            onchange: (updated) => updateFilter(index, updated),
            onremove: () => removeFilter(index),
            onexecute: applyFilters,
          })}
        {/each}
      </div>

      <div class="flex items-center justify-between mt-3 pt-3 border-t" style="border-color: var(--ds-border);">
        <!-- shortcut-guard-exempt: This action is scoped to the open filter popover. -->
        <Button variant="ghost" size="sm" icon={Plus} onclick={addFilter}>
          {t('common.addFilter')}
        </Button>
        <div class="flex items-center gap-2">
          {#if activeFilterCount > 0 || applied}
            <Button variant="ghost" size="sm" icon={X} onclick={clearFilters}>
              {t('common.clear')}
            </Button>
          {/if}
          <Button variant="primary" size="sm" onclick={applyFilters}>
            {t('common.apply')}
          </Button>
        </div>
      </div>
    </div>
  {/if}
</div>

{#if showFilters}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="fixed inset-0 z-10" onmousedown={() => showFilters = false}></div>
{/if}
