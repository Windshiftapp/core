<script>
  import { BasePicker } from '.';
  import { createEventDispatcher, onMount } from 'svelte';
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';

  const dispatch = createEventDispatcher();

  let {
    value = null,
    placeholder = '',
    class: className = '',
    disabled = false,
    workspaceId = null
  } = $props();

  const resolvedPlaceholder = $derived(placeholder || t('pickers.allLabels'));

  let labels = $state([]);
  let loading = $state(false);
  let error = $state(null);

  // Load labels on mount and when workspaceId changes
  onMount(() => loadLabels());

  $effect(() => {
    if (workspaceId) {
      loadLabels();
    }
  });

  async function loadLabels() {
    if (!workspaceId) return;

    loading = true;
    error = null;
    try {
      const response = await api.tests.testLabels.getAll(workspaceId);
      labels = response || [];
    } catch (err) {
      console.error('Failed to load test labels:', err);
      error = err.message || 'Failed to load labels';
      labels = [];
    } finally {
      loading = false;
    }
  }

  function handleSelect(event) {
    const label = event.detail;
    dispatch('select', {
      value: label ? label.id : null,
      label: label
    });
  }

  function handleCancel() {
    dispatch('cancel');
  }
</script>

<BasePicker
  bind:value
  items={labels}
  {loading}
  {error}
  placeholder={resolvedPlaceholder}
  {disabled}
  class={className}
  allowClear={true}
  searchFields={['name', 'description']}
  getValue={(label) => label?.id}
  getLabel={(label) => label?.name ?? ''}
  on:select={handleSelect}
  on:cancel={handleCancel}
>
  {#snippet itemSnippet({ item: label, isSelected })}
    <div class="flex items-center gap-3 flex-1 min-w-0">
      <!-- Color indicator -->
      <div class="flex-shrink-0">
        <div
          class="w-3 h-3 rounded-full"
          style="background-color: {label.color || '#9CA3AF'};"
        ></div>
      </div>

      <!-- Label name -->
      <span class="font-medium truncate">{label.name}</span>
    </div>
  {/snippet}
</BasePicker>
