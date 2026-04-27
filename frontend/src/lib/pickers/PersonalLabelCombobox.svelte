<script>
  import { BasePicker } from '.';
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { authStore } from '../stores';
  import { Plus, Check } from 'lucide-svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast } from '../stores/toasts.svelte.js';

  // userId semantics:
  //   undefined  → unified mode: load mine ∪ shared; inline-create makes a
  //                personal label owned by the current user.
  //   null       → legacy custom-field mode: load shared (global) labels only;
  //                inline-create makes a shared label.
  //   <number>   → that user's labels (mine ∪ shared); inline-create assigns
  //                user_id = <number>.
  let {
    value = $bindable(/** @type {string[] | string} */ ([])),
    placeholder = '',
    class: className = '',
    disabled = false,
    userId = undefined,
    onSelect = () => {},
    onCancel = () => {}
  } = $props();

  const resolvedPlaceholder = $derived(placeholder || t('pickers.selectLabels'));

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

  function handleChange(selectedIds) {
    // Convert IDs back to names
    selectedIds = selectedIds || [];
    const selectedNames = selectedIds
      .map(id => labels.find(l => l.id === id)?.name)
      .filter(Boolean);

    value = selectedNames;

    const selectedLabels = selectedIds
      .map(id => labels.find(l => l.id === id))
      .filter(Boolean);

    onSelect({
      value: selectedNames,
      labels: selectedLabels
    });
  }

  function handleCancel() {
    onCancel();
  }

  async function handleCreate(searchQuery) {
    if (!searchQuery?.trim()) return;

    if (searchQuery.includes(',')) {
      error = t('pickers.labelCommaNotAllowed');
      return;
    }

    // Resolve who owns the new label:
    //   unified mode (userId === undefined): create as personal for me
    //   legacy/null: create as shared (user_id null)
    //   explicit id: that user
    const createUserId = userId === undefined
      ? (authStore.currentUser?.id ?? null)
      : userId;

    try {
      const newLabel = await api.personalLabels.create({
        name: searchQuery.trim(),
        user_id: createUserId
      });

      // Add to local labels array
      labels = [...labels, newLabel];

      // Add the newly created label to selection
      const newValue = [...valueAsNames, newLabel.name];
      value = newValue;

      const selected = newValue
        .map((name) => labels.find((l) => l.name === name))
        .filter(Boolean);

      onSelect({ value: newValue, labels: selected });
    } catch (err) {
      console.error('Failed to create label:', err);
      errorToast(t('dialogs.alerts.failedToCreateLabel', { error: err.message }));
    }
  }
</script>

<BasePicker
  bind:this={pickerRef}
  value={valueAsIds}
  items={labels}
  {loading}
  {error}
  placeholder={resolvedPlaceholder}
  {disabled}
  class={className}
  multiple={true}
  allowCreate={true}
  onCreate={handleCreate}
  searchFields={['name']}
  getValue={(label) => label?.id}
  getLabel={(label) => label?.name ?? ''}
  onChange={handleChange}
  onCancel={handleCancel}
>
  {#snippet itemSnippet({ item: label, isSelected })}
    <div class="flex items-center gap-3 flex-1 min-w-0">
      <span
        class="inline-block w-3 h-3 rounded-full flex-shrink-0"
        style="background-color: {label.color || '#3B82F6'};"
        aria-hidden="true"
      ></span>
      <span class="font-medium text-sm" style="color: var(--ds-text);">
        {label.name}
      </span>
    </div>
  {/snippet}

  {#snippet noResultsSnippet({ searchQuery })}
    <div class="p-3 text-sm text-center" style="color: var(--ds-text-subtle);">
      <div class="space-y-2">
        <div>{t('pickers.noLabelsFoundFor', { query: searchQuery })}</div>
        <button
          type="button"
          class="flex items-center gap-2 px-3 py-1 rounded transition-colors mx-auto"
          style="background-color: var(--ds-background-accent-blue-subtlest); color: var(--ds-interactive);"
          onclick={() => handleCreate(searchQuery)}
        >
          <Plus class="w-4 h-4" />
          {t('pickers.createItem', { value: searchQuery })}
        </button>
      </div>
    </div>
  {/snippet}
</BasePicker>
