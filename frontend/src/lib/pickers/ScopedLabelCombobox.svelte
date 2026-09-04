<script>
  import { untrack } from 'svelte';
  import { BasePicker } from '.';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import { parseLabelValue, labelIdsForNames } from './labelComboboxUtils.js';
  import LabelCreateRow from './LabelCreateRow.svelte';
  import LabelItemRow from './LabelItemRow.svelte';

  let {
    id = undefined,
    testidPrefix = null,
    value = $bindable(/** @type {string[] | string} */ ([])),
    placeholder = '',
    class: className = '',
    disabled = false,
    labels: providedLabels = null,
    loading: providedLoading = false,
    loadKey = 'default',
    loadLabels,
    createLabel,
    onOpen = null,
    onClose = null,
    onSelect = () => {},
    onCancel = () => {},
  } = $props();

  const resolvedPlaceholder = $derived(placeholder || t('pickers.selectLabels'));
  let loadedLabels = $state([]);
  let createdLabels = $state([]);
  let internalLoading = $state(false);
  let error = $state(null);
  let loadToken = 0;

  const labels = $derived([...(providedLabels ?? loadedLabels), ...createdLabels]);
  const loading = $derived(providedLabels === null ? internalLoading : providedLoading);
  const valueAsNames = $derived.by(() => parseLabelValue(value));
  const valueAsIds = $derived.by(() => labelIdsForNames(valueAsNames, labels));

  $effect(() => {
    loadKey;
    if (providedLabels !== null || !loadLabels) return;
    untrack(() => void refreshLabels());
  });

  async function refreshLabels() {
    const token = ++loadToken;
    internalLoading = true;
    error = null;
    createdLabels = [];
    try {
      const response = await loadLabels();
      if (token === loadToken) loadedLabels = response || [];
    } catch (loadError) {
      if (token !== loadToken) return;
      console.error('Failed to load labels:', loadError);
      error = loadError.message || 'Failed to load labels';
      loadedLabels = [];
    } finally {
      if (token === loadToken) internalLoading = false;
    }
  }

  function handleChange(selectedIds = []) {
    const selectedLabels = selectedIds
      .map((selectedId) => labels.find((label) => label.id === selectedId))
      .filter(Boolean);
    value = selectedLabels.map((label) => label.name);
    onSelect({ value, labels: selectedLabels });
  }

  async function handleCreate(searchQuery) {
    const name = searchQuery?.trim();
    if (!name || !createLabel) return;
    if (name.includes(',')) {
      error = t('pickers.labelCommaNotAllowed');
      return;
    }

    try {
      const newLabel = await createLabel(name);
      if (!newLabel) return;
      createdLabels = [...createdLabels, newLabel];
      const selected = [...valueAsNames, newLabel.name]
        .map((labelName) => labels.find((label) => label.name === labelName))
        .filter(Boolean);
      value = selected.map((label) => label.name);
      onSelect({ value, labels: selected });
    } catch (createError) {
      console.error('Failed to create label:', createError);
      errorToast(t('dialogs.alerts.failedToCreateLabel', { error: createError.message }));
    }
  }
</script>

<BasePicker
  {id}
  value={valueAsIds}
  items={labels}
  {loading}
  {error}
  placeholder={resolvedPlaceholder}
  {disabled}
  class={className}
  multiple
  allowCreate
  onCreate={handleCreate}
  searchFields={['name']}
  getValue={(label) => label?.id}
  getLabel={(label) => label?.name ?? ''}
  optionTestid={testidPrefix ? (option) => `${testidPrefix}-option-${option.value}` : null}
  onOpen={() => onOpen?.()}
  onClose={() => onClose?.()}
  onChange={handleChange}
  onCancel={() => onCancel?.()}
>
  {#snippet itemSnippet({ item: label })}
    <LabelItemRow {label} />
  {/snippet}

  {#snippet noResultsSnippet({ searchQuery })}
    <LabelCreateRow {searchQuery} oncreate={handleCreate} />
  {/snippet}
</BasePicker>
