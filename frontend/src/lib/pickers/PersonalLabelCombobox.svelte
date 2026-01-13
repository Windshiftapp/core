<script>
  import { BasePicker } from '.';
  import { createEventDispatcher, onMount } from 'svelte';
  import { api } from '../api.js';
  import { Plus, Check } from 'lucide-svelte';

  const dispatch = createEventDispatcher();

  let {
    value = $bindable([]),
    placeholder = 'Select labels...',
    class: className = '',
    disabled = false,
    userId = null
  } = $props();

  let labels = $state([]);
  let loading = $state(false);
  let error = $state(null);
  let pickerRef = $state(null);

  // Convert value (array of names or comma-separated string) to array of names
  const valueAsNames = $derived.by(() => {
    if (!value) return [];
    if (Array.isArray(value)) return value;
    if (typeof value === 'string' && value.trim()) {
      return value.split(',').map(name => name.trim()).filter(name => name);
    }
    return [];
  });

  // Map label names to label IDs for the picker
  const valueAsIds = $derived.by(() => {
    return valueAsNames
      .map(name => labels.find(l => l.name === name)?.id)
      .filter(Boolean);
  });

  onMount(async () => {
    await loadLabels();
  });

  async function loadLabels() {
    loading = true;
    error = null;
    try {
      const response = await api.personalLabels.getAll(userId);
      labels = response || [];
    } catch (err) {
      console.error('Failed to load personal labels:', err);
      error = err.message || 'Failed to load labels';
      labels = [];
    } finally {
      loading = false;
    }
  }

  function handleChange(event) {
    // Convert IDs back to names
    const selectedIds = event.detail || [];
    const selectedNames = selectedIds
      .map(id => labels.find(l => l.id === id)?.name)
      .filter(Boolean);

    value = selectedNames;

    const selectedLabels = selectedIds
      .map(id => labels.find(l => l.id === id))
      .filter(Boolean);

    dispatch('select', {
      value: selectedNames,
      labels: selectedLabels
    });
  }

  function handleCancel() {
    dispatch('cancel');
  }

  async function handleCreate(searchQuery) {
    if (!searchQuery?.trim()) return;

    try {
      const newLabel = await api.personalLabels.create({
        name: searchQuery.trim(),
        user_id: userId
      });

      // Add to local labels array
      labels = [...labels, newLabel];

      // Add the newly created label to selection
      const newValue = [...valueAsNames, newLabel.name];
      value = newValue;

      dispatch('select', {
        value: newValue,
        labels: [...labels.filter(l => valueAsNames.includes(l.name)), newLabel]
      });
    } catch (err) {
      console.error('Failed to create label:', err);
      alert('Failed to create label: ' + err.message);
    }
  }
</script>

<BasePicker
  bind:this={pickerRef}
  value={valueAsIds}
  items={labels}
  {loading}
  {error}
  {placeholder}
  {disabled}
  class={className}
  multiple={true}
  allowCreate={true}
  onCreate={handleCreate}
  searchFields={['name']}
  getValue={(label) => label?.id}
  getLabel={(label) => label?.name ?? ''}
  on:change={handleChange}
  on:cancel={handleCancel}
>
  {#snippet itemSnippet({ item: label, isSelected })}
    <div class="flex items-center gap-3 flex-1 min-w-0">
      <span class="font-medium text-sm" style="color: var(--ds-text);">
        {label.name}
      </span>
    </div>
  {/snippet}

  {#snippet noResultsSnippet({ searchQuery })}
    <div class="p-3 text-sm text-center" style="color: var(--ds-text-subtle);">
      <div class="space-y-2">
        <div>No labels found for "{searchQuery}"</div>
        <button
          type="button"
          class="flex items-center gap-2 px-3 py-1 rounded transition-colors mx-auto"
          style="background-color: var(--ds-background-accent-blue-subtlest); color: var(--ds-interactive);"
          onclick={() => handleCreate(searchQuery)}
        >
          <Plus class="w-4 h-4" />
          Create "{searchQuery}"
        </button>
      </div>
    </div>
  {/snippet}
</BasePicker>
