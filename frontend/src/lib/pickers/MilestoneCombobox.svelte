<script>
  import ItemPicker from './ItemPicker.svelte';
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';

  let {
    value = $bindable(null),
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

  const resolvedPlaceholder = $derived(placeholder || t('pickers.selectMilestone'));
  const resolvedUnassignedLabel = $derived(unassignedLabel || t('pickers.noMilestone'));

  let milestones = $state([]);
  let loading = $state(false);

  // Load milestones on mount
  onMount(async () => {
    await loadMilestones();
  });

  // Reload when workspaceId changes
  $effect(() => {
    if (workspaceId !== undefined) {
      loadMilestones();
    }
  });

  async function loadMilestones() {
    loading = true;

    try {
      const filters = {};
      if (workspaceId) {
        filters.workspace_id = workspaceId;
        filters.include_global = true;
      }

      const response = await api.milestones.getAll(filters);
      milestones = response || [];
    } catch (err) {
      console.error('Failed to load milestones:', err);
      milestones = [];
    } finally {
      loading = false;
    }
  }

  function handleSelect(milestone) {
    onSelect({
      value: milestone ? milestone.id : null,
      milestone: milestone
    });
  }

  const config = {
    icon: {
      type: 'color-dot',
      source: (item) => item.category_color || '#9CA3AF',
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
  items={milestones}
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
