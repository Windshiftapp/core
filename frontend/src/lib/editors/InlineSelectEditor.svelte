<script>
  import { tick } from 'svelte';
  import { ChevronDown, Check, X, Loader2 } from 'lucide-svelte';
  import { t } from '../stores/i18n.svelte.js';

  let {
    value = null, options = [], placeholder = '', disabled = false,
    required = false, className = '', displayClass = 'hover-bg cursor-pointer',
    allowClear = false, onsave = null
  } = $props();

  const effectivePlaceholder = $derived(placeholder || t('common.select') + '...');

  let editing = $state(false);
  let selectElement = $state(null);
  let saving = $state(false);
  let error = $state('');
  let selectedValue = $state(value);

  function startEditing() {
    if (disabled) return;

    editing = true;
    selectedValue = value;
    error = '';

    // Focus select after DOM update
    tick().then(() => {
      if (selectElement) {
        selectElement.focus();
      }
    });
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

  function handleChange() {
    // Auto-save on change for dropdowns
    saveValue();
  }

  function handleBlur() {
    // Small delay to allow clicking save/cancel buttons
    setTimeout(() => {
      if (editing && !saving) {
        saveValue();
      }
    }, 100);
  }

  // Get display info for current value
  const currentOption = $derived(options.find(opt => opt.value === value));
  const displayText = $derived(currentOption?.label || effectivePlaceholder);
  const displayColor = $derived(currentOption?.color);
</script>

{#if editing}
  <div class="inline-flex items-center gap-1 w-full">
    <div class="flex-1 relative">
      <select
        bind:this={selectElement}
        bind:value={selectedValue}
        class="w-full px-2 py-1 text-sm border rounded border-blue-500 ring-1 ring-blue-500 {className}"
        class:border-red-500={error}
        disabled={saving}
        onkeydown={handleKeydown}
        onchange={handleChange}
        onblur={handleBlur}
      >
        {#if allowClear || !required}
          <option value={null}>
            {effectivePlaceholder}
          </option>
        {/if}
        {#each options as option}
          <option value={option.value}>
            {option.label}
          </option>
        {/each}
      </select>

      {#if error}
        <div class="absolute top-full left-0 mt-1 text-xs text-red-600 bg-white px-2 py-1 border border-red-200 rounded shadow-sm z-10">
          {error}
        </div>
      {/if}
    </div>

    <div class="flex items-center gap-1">
      {#if saving}
        <Loader2 class="w-4 h-4 animate-spin" style="color: var(--ds-text-subtle);" />
      {:else}
        <button
          type="button"
          onclick={saveValue}
          class="p-1 text-green-600 hover:bg-green-50 rounded"
          title={t('editors.saveEnter')}
        >
          <Check class="w-4 h-4" />
        </button>
        <button
          type="button"
          onclick={cancelEditing}
          class="p-1 rounded hover-bg"
          style="color: var(--ds-text-subtle);"
          title={t('editors.cancelEscape')}
        >
          <X class="w-4 h-4" />
        </button>
      {/if}
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
