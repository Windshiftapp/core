<script>
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import { t } from '../stores/i18n.svelte.js';
  import Select from '../components/Select.svelte';
  import InlineEditorActions from './InlineEditorActions.svelte';
  import InlineEditorError from './InlineEditorError.svelte';

  let {
    value = null, options = [], placeholder = '', disabled = false,
    required = false, className = '', displayClass = 'hover-bg cursor-pointer',
    allowClear = false, onsave = null
  } = $props();

  const effectivePlaceholder = $derived(placeholder || t('common.select') + '...');

  // Build options for the Select component, including placeholder if needed
  const selectOptions = $derived.by(() => {
    const opts = [];
    if (allowClear || !required) {
      opts.push({ value: null, label: effectivePlaceholder });
    }
    opts.push(...options);
    return opts;
  });

  let editing = $state(false);
  let saving = $state(false);
  let error = $state('');
  // Local edit buffer; resets to the prop value each time editing starts.
  // svelte-ignore state_referenced_locally
  let selectedValue = $state(value);

  function startEditing() {
    if (disabled) return;

    editing = true;
    selectedValue = value;
    error = '';
  }

  function cancelEditing() {
    editing = false;
    selectedValue = value;
    error = '';
  }

  async function saveValue() {
    if (saving) return;

    // Validation
    if (required && (selectedValue === null || selectedValue === undefined || selectedValue === '')) {
      error = t('validation.required');
      return;
    }

    // Check if value actually changed
    if (selectedValue === value) {
      cancelEditing();
      return;
    }

    try {
      saving = true;
      error = '';

      // Call save callback
      onsave?.({ value: selectedValue });

      // Wait for parent to confirm save

    } catch (err) {
      error = err.message || 'Failed to save';
      saving = false;
    }
  }

  // External methods that parent can call
  export function confirmSave(newValue) {
    value = newValue;
    selectedValue = newValue;
    editing = false;
    saving = false;
    error = '';
  }

  export function rejectSave(errorMessage) {
    error = errorMessage || 'Failed to save';
    saving = false;
  }

  function handleKeydown(event) {
    if (event.key === 'Enter') {
      event.preventDefault();
      saveValue();
    } else if (event.key === 'Escape') {
      event.preventDefault();
      cancelEditing();
    }
  }

  function handleSelectChange(val) {
    selectedValue = val;
    // Auto-save on change for dropdowns
    saveValue();
  }

  // Get display info for current value
  const currentOption = $derived(options.find(opt => opt.value === value));
  const displayText = $derived(currentOption?.label || effectivePlaceholder);
  const displayColor = $derived(currentOption?.color);
</script>

{#if editing}
  <div class="inline-flex items-center gap-1 w-full">
    <div class="flex-1 relative">
      <Select
        value={selectedValue}
        options={selectOptions}
        disabled={saving}
        size="small"
        onchange={handleSelectChange}
        class={className}
      />

      <InlineEditorError {error} />
    </div>

    <div class="flex items-center gap-1">
      <InlineEditorActions {saving} onsave={saveValue} oncancel={cancelEditing} />
    </div>
  </div>
{:else}
  <button
    type="button"
    onclick={startEditing}
    class="inline-flex items-center gap-2 px-2 py-1 text-sm rounded transition-colors {displayClass} {className}"
    style={!currentOption ? 'color: var(--ds-text-subtle);' : ''}
    {disabled}
  >
    <span
      class="flex items-center gap-2"
      style={!currentOption ? 'color: var(--ds-text-subtle);' : ''}
    >
      {#if currentOption && displayColor}
        <span
          class="w-2 h-2 rounded-full"
          style="background-color: {displayColor};"
        ></span>
      {/if}
      {displayText}
    </span>
    <ChevronDown class="w-3 h-3" style="color: var(--ds-text-subtle);" />
  </button>
{/if}

<style>
  .hover-bg:hover {
    background-color: var(--ds-background-neutral-hovered);
  }
</style>
