<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { AlertCircle } from 'lucide-svelte';
  import { priorityIconMap } from '../utils/icons.js';
  import ItemPicker from './ItemPicker.svelte';

  // Props
  let {
    workspaceId,
    selectedPriorityId = $bindable(null),
    onChange = () => {},
    disabled = false,
    placeholder = 'Select priority',
    triggerClass = '',
    showUnassigned = true,
    unassignedLabel = 'No priority',
    children = null  // Custom trigger snippet
  } = $props();

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
      text: (item) => item.name
    },
    searchFields: ['name', 'description'],
    getValue: (item) => item.id,
    getLabel: (item) => item.name
  };

  // Reactive: Load priorities when workspaceId changes
  $effect(() => {
    if (workspaceId) {
      loadPriorities();
    }
  });

  async function loadPriorities() {
    if (!workspaceId) return;

    try {
      loading = true;
      error = null;

      // Get workspace to find its configuration set
      const workspace = await api.workspaces.get(workspaceId);

      if (workspace.configuration_set_id) {
        // Load priorities from configuration set
        const configSet = await api.configurationSets.get(workspace.configuration_set_id);
        priorities = configSet.priorities_detailed || [];
      } else {
        // No configuration set - load all priorities
        priorities = await api.priorities.getAll();
      }

      // Sort by sort_order
      priorities = priorities.sort((a, b) => a.sort_order - b.sort_order);

    } catch (err) {
      console.error('Failed to load priorities:', err);
      error = 'Failed to load priorities';
      priorities = [];
    } finally {
      loading = false;
    }
  }

  function handlePrioritySelect(priority) {
    const id = priority?.id ?? null;
    selectedPriorityId = id;
    onChange(id, priority || null);
  }

  function getIconComponent(iconName) {
    return priorityIconMap[iconName] || AlertCircle;
  }

  onMount(() => {
    loadPriorities();
  });
</script>

{#if loading}
  <div class="w-full px-3 py-2 text-sm text-gray-500 border rounded" style="background-color: var(--ds-background-input); border-color: var(--ds-border);">
    Loading priorities...
  </div>
{:else if error}
  <div class="w-full px-3 py-2 text-sm text-red-500 border rounded" style="background-color: var(--ds-background-input); border-color: var(--ds-border);">
    {error}
  </div>
{:else if priorities.length === 0}
  <div class="w-full px-3 py-2 text-sm text-gray-500 border rounded" style="background-color: var(--ds-background-input); border-color: var(--ds-border);">
    No priorities configured
  </div>
{:else}
  <ItemPicker
    value={selectedPriorityId}
    items={priorities}
    config={pickerConfig}
    placeholder={placeholder}
    showUnassigned={showUnassigned}
    unassignedLabel={unassignedLabel}
    disabled={disabled}
    class={triggerClass}
    {children}
    on:select={(event) => handlePrioritySelect(event.detail)}
    on:cancel={() => {
      if (selectedPriorityId === null) {
        return;
      }
    }}
  />
{/if}
