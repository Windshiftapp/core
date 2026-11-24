<script>
  import AlertCircle from '@lucide/svelte/icons/alert-circle';
  import { priorityIconMap } from '../utils/icons.js';
  import ItemPicker from './ItemPicker.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { workspaces } from '../api/workspaces.js';
  import { objectDisplayName } from '../utils/systemLabels.js';

  // Props
  let {
    workspaceId,
    items = null,  // Pre-loaded priority items, skips API fetch when provided
    selectedPriorityId = $bindable(null),
    onChange = () => {},
    disabled = false,
    placeholder = '',
    triggerClass = '',
    showUnassigned = true,
    unassignedLabel = '',
    children = null  // Custom trigger snippet
  } = $props();

  const resolvedPlaceholder = $derived(placeholder || t('pickers.selectPriority'));
  const resolvedUnassignedLabel = $derived(unassignedLabel || t('pickers.noPriority'));

  // State
  let priorities = $state([]);
  let loading = $state(true);
  let error = $state(null);

  const pickerConfig = {
    icon: {
      type: 'component',
      source: (item) => getIconComponent(item.icon) || AlertCircle
    },
    primary: {
      text: (item) => objectDisplayName(item, 'priority')
    },
    searchFields: ['display_name', 'name', 'display_description', 'description'],
    getValue: (item) => item.id,
    getLabel: (item) => objectDisplayName(item, 'priority')
  };

  // Reactive: use provided items, otherwise load the target workspace's
  // effective priority catalog from the workspace-priorities API. The API owns
  // configuration-set resolution, so the picker no longer reimplements it and
  // a single effect (no mount duplicate) owns loading.
  $effect(() => {
    if (items) {
      priorities = [...items].sort((a, b) => a.sort_order - b.sort_order);
      loading = false;
      error = null;
      return;
    }
    const id = workspaceId;
    if (!id) {
      priorities = [];
      loading = false;
      return;
    }
    let cancelled = false;
    loading = true;
    error = null;
    workspaces
      .getPriorities(id)
      .then((result) => {
        if (cancelled) return;
        priorities = [...(result || [])].sort((a, b) => a.sort_order - b.sort_order);
      })
      .catch((err) => {
        if (cancelled) return;
        console.error('Failed to load priorities:', err);
        error = 'Failed to load priorities';
        priorities = [];
      })
      .finally(() => {
        if (!cancelled) loading = false;
      });
    return () => {
      cancelled = true;
    };
  });

  function handlePrioritySelect(priority) {
    const id = priority?.id ?? null;
    selectedPriorityId = id;
    onChange(id, priority || null);
  }

  function getIconComponent(iconName) {
    return priorityIconMap[iconName] || AlertCircle;
  }
</script>

{#if loading}
  <div class="w-full px-3 py-2 text-sm text-ds-text-subtle border rounded" style="background-color: var(--ds-background-input); border-color: var(--ds-border);">
    {t('pickers.loadingPriorities')}
  </div>
{:else if error}
  <div class="w-full px-3 py-2 text-sm text-ds-text-danger border rounded" style="background-color: var(--ds-background-input); border-color: var(--ds-border);">
    {error}
  </div>
{:else if priorities.length === 0}
  <div class="w-full px-3 py-2 text-sm text-ds-text-subtle border rounded" style="background-color: var(--ds-background-input); border-color: var(--ds-border);">
    {t('pickers.noPrioritiesConfigured')}
  </div>
{:else}
  <ItemPicker
    value={selectedPriorityId}
    items={priorities}
    config={pickerConfig}
    placeholder={resolvedPlaceholder}
    showUnassigned={showUnassigned}
    unassignedLabel={resolvedUnassignedLabel}
    disabled={disabled}
    class={triggerClass}
    {children}
    onSelect={(item) => handlePrioritySelect(item)}
    onCancel={() => {
      if (selectedPriorityId === null) {
        return;
      }
    }}
  />
{/if}
