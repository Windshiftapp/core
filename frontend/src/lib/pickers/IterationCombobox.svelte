<script>
  import ItemPicker from './ItemPicker.svelte';
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';

  let {
    value = $bindable(null),
    values = $bindable([]),
    multiSelect = false,
    placeholder = '',
    class: className = '',
    disabled = false,
    workspaceId = null,
    showUnassigned = true,
    unassignedLabel = '',
    children = null,
    onSelect = () => {},
    onCancel = () => {}
  } = $props();

  const resolvedPlaceholder = $derived(placeholder || t('pickers.selectIteration') || 'Select iteration...');
  const resolvedUnassignedLabel = $derived(unassignedLabel || t('pickers.noIteration') || 'No iteration');

  let iterations = $state([]);
  let loading = $state(false);

  // Load iterations on mount
  onMount(async () => {
    await loadIterations();
  });

  // Reload when workspaceId changes
  $effect(() => {
    if (workspaceId !== undefined) {
      loadIterations();
    }
  });

  async function loadIterations() {
    loading = true;

    try {
      const filters = {};
      if (workspaceId) {
        filters.workspace_id = workspaceId;
        filters.include_global = true;
      }

      const response = await api.iterations.getAll(filters);
      iterations = response || [];
    } catch (err) {
      console.error('Failed to load iterations:', err);
      iterations = [];
    } finally {
      loading = false;
    }
  }

  function handleSelect(iterationOrValues) {
    if (multiSelect) {
      // In multi-select mode, ItemPicker passes the values array
      onSelect(iterationOrValues);
    } else {
      onSelect({
        value: iterationOrValues ? iterationOrValues.id : null,
        iteration: iterationOrValues
      });
    }
  }

  const config = {
    icon: {
      type: 'color-dot',
      source: (item) => item.type_color || '#9CA3AF',
      size: 'w-2 h-2'
    },
    primary: { text: (item) => item.name || '' },
    searchFields: ['name', 'description'],
    getValue: (item) => item?.id,
    getLabel: (item) => item?.name ?? ''
  };
</script>

<ItemPicker
  bind:value
  bind:values
  {multiSelect}
  items={iterations}
  {config}
  placeholder={resolvedPlaceholder}
  {showUnassigned}
  unassignedLabel={resolvedUnassignedLabel}
  {disabled}
  {loading}
  allowClear={true}
  class={className}
  {children}
  onSelect={handleSelect}
  onCancel={() => onCancel()}
/>
