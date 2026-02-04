<script>
  import { tick } from 'svelte';
  import { Check, X, Loader2, Calendar } from 'lucide-svelte';
  import { t } from '../stores/i18n.svelte.js';

  let {
    value = '', placeholder = '', disabled = false, required = false,
    className = '', editingClass = 'border-blue-500 ring-1 ring-blue-500',
    displayClass = 'hover-bg cursor-text', enableSingleClick = false,
    enableDoubleClick = false, onsave = null, onclick: onclickProp = null
  } = $props();

  const effectivePlaceholder = $derived(placeholder || t('editors.selectDate'));

  let editing = $state(false);
  let editValue = $state('');
  let inputElement = $state(null);
  let saving = $state(false);
  let error = $state('');
  let clickTimeout = $state(null);

  // Format date for display
  const displayValue = $derived(value ? formatDisplayDate(value) : '');

  function formatDisplayDate(dateStr) {
    if (!dateStr) return '';
    try {
      const date = new Date(dateStr);
      return date.toLocaleDateString('en-US', {
        year: 'numeric',
        month: 'short',
        day: 'numeric'
      });
    } catch (e) {
      return dateStr;
    }
  }

  function formatInputDate(dateStr) {
    if (!dateStr) return '';
    try {
      const date = new Date(dateStr);
      return date.toISOString().split('T')[0]; // YYYY-MM-DD format for input[type="date"]
    } catch (e) {
      return '';
    }
  }

  function startEditing() {
    if (disabled) return;

    editing = true;
    editValue = formatInputDate(value);
    error = '';

    // Focus input after DOM update
    tick().then(() => {
      if (inputElement) {
        inputElement.focus();
      }
    });
  }

  function cancelEditing() {
    editing = false;
    editValue = '';
    error = '';
  }

  async function saveValue() {
    if (saving) return;

    // Validation
    if (required && !editValue) {
      error = t('validation.required');
      return;
    }

    // Check if value actually changed
    if (editValue === formatInputDate(value)) {
      cancelEditing();
      return;
    }

    try {
      saving = true;
      error = '';

      // Convert to ISO string for storage
      const dateValue = editValue ? new Date(editValue).toISOString() : '';

      // Call save callback
      onsave?.({ value: dateValue });

    } catch (err) {
      error = err.message || 'Failed to save';
      saving = false;
    }
  }

  // External methods that parent can call
  export function confirmSave(newValue) {
    value = newValue;
    editing = false;
    editValue = '';
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

  function handleBlur() {
    // Small delay to allow clicking save/cancel buttons
    setTimeout(() => {
      if (editing && !saving) {
        saveValue();
      }
    }, 100);
  }

  function handleClick() {
    if (enableDoubleClick && enableSingleClick) {
      // When both are enabled, delay single click to check for double click
      if (clickTimeout) {
        // This is a double click - clear the timeout and start editing
        clearTimeout(clickTimeout);
        clickTimeout = null;
        startEditing();
      } else {
        // This might be a single click - wait to see if it becomes a double click
        clickTimeout = setTimeout(() => {
          clickTimeout = null;
          onclickProp?.();
        }, 200);
      }
    } else if (enableSingleClick) {
      // Only single click enabled - dispatch immediately
      onclickProp?.();
    } else if (enableDoubleClick) {
      // Only double click enabled - do nothing on single click
      return;
    } else {
      // Default behavior - edit on single click
      startEditing();
    }
  }

  function handleDoubleClick() {
    if (enableDoubleClick && !enableSingleClick) {
      // Only double click enabled
      startEditing();
    }
    // If both are enabled, double click is handled in handleClick
  }
</script>

{#if editing}
  <div class="inline-flex items-center gap-1 w-full">
    <div class="flex-1 relative">
      <input
        bind:this={inputElement}
        bind:value={editValue}
        type="date"
        class="w-full px-2 py-1 text-sm border rounded {editingClass} {className}"
        class:border-red-500={error}
        disabled={saving}
        onkeydown={handleKeydown}
        onblur={handleBlur}
      />
      {#if error}
        <div class="absolute top-full left-0 mt-1 text-xs text-red-600 px-2 py-1 border border-red-200 rounded shadow-sm z-10" style="background-color: var(--ds-surface);">
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
    onclick={handleClick}
    ondblclick={handleDoubleClick}
    class="text-left w-full px-2 py-1 text-sm rounded transition-colors flex items-center gap-2 {displayClass} {className}"
    style={!value ? 'color: var(--ds-text-subtle);' : ''}
    class:cursor-pointer={enableSingleClick || enableDoubleClick}
    {disabled}
  >
    <Calendar class="w-4 h-4" style="color: var(--ds-text-subtle);" />
    {displayValue || effectivePlaceholder}
  </button>
{/if}

<style>
  .hover-bg:hover {
    background-color: var(--ds-background-neutral-hovered);
  }
</style>
