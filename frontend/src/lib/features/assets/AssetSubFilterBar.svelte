<script>
  import { Filter, Plus, X } from 'lucide-svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { QLBuilder } from '../../utils/ql.js';
  import AssetDynamicFieldFilter from './AssetDynamicFieldFilter.svelte';
  import Button from '../../components/Button.svelte';

  let {
    statuses = [],
    assetTypes = [],
    categories = [],
    customFields = [],
    onApply = () => {}
  } = $props();

  let showFilters = $state(false);
  let filters = $state([]);

  let activeFilterCount = $derived(
    filters.filter(f => f.field && (f.value || (f.values && f.values.length > 0))).length
  );

  // Build asset-specific field groups for FieldSelector
  let assetFieldGroups = $derived([
    {
      category: t('pickers.fieldCategories.basic') || 'Basic',
      fields: [
        { id: 'title', name: t('pickers.fields.title')?.name || t('pickers.fields.title') || 'Title', type: 'text', description: '' },
        { id: 'description', name: t('pickers.fields.description')?.name || t('pickers.fields.description') || 'Description', type: 'text', description: '' },
        { id: 'asset_tag', name: 'Asset Tag', type: 'text', description: '' }
      ]
    },
    {
      category: 'Classification',
      fields: [
        { id: 'status', name: t('pickers.fields.status')?.name || t('pickers.fields.status') || 'Status', type: 'enum', description: '' },
        { id: 'type', name: t('pickers.fields.type')?.name || t('pickers.fields.type') || 'Type', type: 'enum', description: '' },
        { id: 'category', name: t('common.category') || 'Category', type: 'enum', description: '' }
      ]
    },
    {
      category: t('pickers.fieldCategories.dates') || 'Dates',
      fields: [
        { id: 'created_at', name: t('pickers.fields.createdAt')?.name || t('pickers.fields.createdAt') || 'Created', type: 'date', description: '' },
        { id: 'updated_at', name: t('pickers.fields.updatedAt')?.name || t('pickers.fields.updatedAt') || 'Updated', type: 'date', description: '' }
      ]
    },
    {
      category: t('pickers.fieldCategories.people') || 'People',
      fields: [
        { id: 'creator', name: t('common.createdBy') || 'Creator', type: 'user', description: '' }
      ]
    }
  ]);

  // Build custom field items for FieldSelector
  let assetCustomFieldItems = $derived(
    customFields.map(field => ({
      id: `cf_${field.field_name || field.name}`,
      name: field.field_name || field.name,
      type: field.field_type,
      description: '',
      isCustom: true,
      options: field.field_options ? (typeof field.field_options === 'string' ? JSON.parse(field.field_options) : field.field_options) : null
    }))
  );

  function addFilter() {
    filters = [...filters, { field: null, operator: '=', value: '', values: [] }];
  }

  function handleFilterChange(index, updatedFilter) {
    filters = filters.map((f, i) => i === index ? updatedFilter : f);
  }

  function handleFilterRemove(index) {
    filters = filters.filter((_, i) => i !== index);
    if (filters.length === 0) {
      onApply('');
    }
  }

  function applyFilters() {
    const ql = QLBuilder.buildQuery({ dynamicFields: filters });
    onApply(ql || '');
    showFilters = false;
  }

  function clearAll() {
    filters = [];
    onApply('');
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
    style="color: var(--ds-text-subtle); background: var(--ds-background-input); border: 1px solid var(--ds-border);"
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
      class="absolute left-0 top-full mt-2 z-20 rounded-lg border shadow-lg p-3 min-w-[440px]"
      style="background-color: var(--ds-surface-overlay); border-color: var(--ds-border);"
    >
      <!-- Filter Rows -->
      <div class="flex flex-col gap-2">
        {#each filters as filter, index (index)}
          <AssetDynamicFieldFilter
            {filter}
            compact={true}
            {statuses}
            {assetTypes}
            {categories}
            fieldGroups={assetFieldGroups}
            customFieldItems={assetCustomFieldItems}
            onChange={(updated) => handleFilterChange(index, updated)}
            onRemove={() => handleFilterRemove(index)}
            onExecute={applyFilters}
          />
        {/each}
      </div>

      <!-- Actions -->
      <div class="flex items-center justify-between mt-3 pt-3 border-t" style="border-color: var(--ds-border);">
        <Button variant="ghost" size="sm" icon={Plus} onclick={addFilter}>
          {t('common.addFilter') || 'Add filter'}
        </Button>

        <div class="flex items-center gap-2">
          {#if activeFilterCount > 0}
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
