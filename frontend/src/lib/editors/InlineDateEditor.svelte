<script>
  import { createEventDispatcher, tick } from 'svelte';
  import { Check, X, Loader2, Calendar } from 'lucide-svelte';
  
  const dispatch = createEventDispatcher();
  
  export let value = '';
  export let placeholder = 'Select date...';
  export let disabled = false;
  export let required = false;
  export let className = '';
  export let editingClass = 'border-blue-500 ring-1 ring-blue-500';
  export let displayClass = 'hover:bg-gray-50 cursor-text';
  export let enableSingleClick = false;
  export let enableDoubleClick = false;
  
  let editing = false;
  let editValue = '';
  let inputElement;
  let saving = false;
  let error = '';
  let clickTimeout = null;
  
  // Format date for display
  $: displayValue = value ? formatDisplayDate(value) : '';
  
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
      error = 'This field is required';
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
      
      // Dispatch save event
      dispatch('save', { value: dateValue });
      
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
        }, 200);
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
        type="date"
        class="w-full px-2 py-1 text-sm border rounded {editingClass} {className}"
        class:border-red-500={error}
        disabled={saving}
        onkeydown={handleKeydown}
        onblur={handleBlur}
      />
      {#if error}
        <div class="absolute top-full left-0 mt-1 text-xs text-red-600 bg-white px-2 py-1 border border-red-200 rounded shadow-sm z-10">
          {error}
        </div>
      {/if}
    </div>
    
    <div class="flex items-center gap-1">
      {#if saving}
        <Loader2 class="w-4 h-4 animate-spin text-gray-400" />
      {:else}
        <button
          type="button"
          onclick={saveValue}
          class="p-1 text-green-600 hover:bg-green-50 rounded"
          title="Save (Enter)"
        >
          <Check class="w-4 h-4" />
        </button>
        <button
          type="button"
          onclick={cancelEditing}
          class="p-1 text-gray-400 hover:bg-gray-50 rounded"
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
    class="text-left w-full px-2 py-1 text-sm rounded transition-colors flex items-center gap-2 {displayClass} {className}"
    class:text-gray-400={!value}
    class:cursor-pointer={enableSingleClick || enableDoubleClick}
    {disabled}
  >
    <Calendar class="w-4 h-4 text-gray-400" />
    {displayValue || placeholder}
  </button>
{/if}