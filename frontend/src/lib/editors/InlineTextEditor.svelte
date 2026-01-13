<script>
  import { createEventDispatcher, tick } from 'svelte';
  import { Check, X, Loader2 } from 'lucide-svelte';
  
  const dispatch = createEventDispatcher();
  
  export let value = '';
  export let placeholder = 'Enter text...';
  export let disabled = false;
  export let required = false;
  export let maxLength = 255;
  export let className = '';
  export let editingClass = 'editing-input';
  export let displayClass = 'display-text cursor-text';
  export let enableSingleClick = false; // When true, single click triggers custom action
  export let enableDoubleClick = false; // When true, double click triggers edit
  
  let editing = false;
  let editValue = '';
  let inputElement;
  let saving = false;
  let error = '';
  let clickTimeout = null;
  
  function startEditing() {
    if (disabled) return;
    
    editing = true;
    editValue = value || '';
    error = '';
    
    // Focus input after DOM update
    tick().then(() => {
      if (inputElement) {
        inputElement.focus();
        inputElement.select();
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
    
    const trimmedValue = editValue.trim();
    
    // Validation
    if (required && !trimmedValue) {
      error = 'This field is required';
      return;
    }
    
    if (trimmedValue.length > maxLength) {
      error = `Must be less than ${maxLength} characters`;
      return;
    }
    
    // Check if value actually changed
    if (trimmedValue === (value || '')) {
      cancelEditing();
      return;
    }
    
    try {
      saving = true;
      error = '';
      
      // Dispatch save event
      dispatch('save', { value: trimmedValue });
      
      // Wait for parent to confirm save (parent should call confirmSave or rejectSave)
      
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
          dispatch('click');
        }, 200); // 200ms delay to detect double click
      }
    } else if (enableSingleClick) {
      // Only single click enabled - dispatch immediately
      dispatch('click');
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
        {placeholder}
        {maxLength}
        class="w-full px-2 py-1 text-sm border rounded {editingClass} {className}"
        class:border-red-500={error}
        disabled={saving}
        onkeydown={handleKeydown}
        onblur={handleBlur}
        style="background-color: var(--ds-surface); color: var(--ds-text); border-color: var(--ds-border-focused);"
      />
      {#if error}
        <div class="absolute top-full left-0 mt-1 text-xs px-2 py-1 border rounded shadow-sm z-10" style="color: var(--ds-text-danger); background-color: var(--ds-surface-raised); border-color: var(--ds-border-danger);">
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
          class="p-1 rounded save-btn"
          title="Save (Enter)"
        >
          <Check class="w-4 h-4" />
        </button>
        <button
          type="button"
          onclick={cancelEditing}
          class="p-1 rounded cancel-btn"
          title="Cancel (Escape)"
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
    class="text-left w-full px-2 py-1 text-sm rounded transition-colors {displayClass} {className}"
    class:placeholder-text={!value}
    class:cursor-pointer={enableSingleClick || enableDoubleClick}
    {disabled}
    style="color: var(--ds-text);"
  >
    {value || placeholder}
  </button>
{/if}

<style>
  .display-text:hover {
    background-color: var(--ds-surface-hovered);
  }

  .editing-input {
    border-color: var(--ds-border-focused);
    box-shadow: 0 0 0 1px var(--ds-border-focused);
  }

  .save-btn {
    color: var(--ds-text-success);
  }

  .save-btn:hover {
    background-color: var(--ds-background-success-subtle);
  }

  .cancel-btn {
    color: var(--ds-text-subtle);
  }

  .cancel-btn:hover {
    background-color: var(--ds-surface-hovered);
  }

  .placeholder-text {
    color: var(--ds-text-subtlest) !important;
  }
</style>