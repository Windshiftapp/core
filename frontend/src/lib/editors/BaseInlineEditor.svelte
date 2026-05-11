<script>
  import { Check, X, Loader2 } from '@lucide/svelte';
  import { t } from '../stores/i18n.svelte.js';

  let {
    disabled = false,
    enableSingleClick = false,
    enableDoubleClick = false,
    onclick: onclickProp = null,
    onsave = null,
    onstartedit = null,
    editingInput,
    displayContent,
  } = $props();

  let editing = $state(false);
  let saving = $state(false);
  let error = $state('');
  let clickTimeout = $state(null);

  export function startEditing() {
    if (disabled) return;
    editing = true;
    error = '';
    onstartedit?.();
  }

  export function cancelEditing() {
    editing = false;
    error = '';
  }

  export function setSaving(val) {
    saving = val;
  }

  export function setError(msg) {
    error = msg;
  }

  export function confirmSave() {
    editing = false;
    saving = false;
    error = '';
  }

  export function rejectSave(errorMessage) {
    error = errorMessage || 'Failed to save';
    saving = false;
  }

  export function isEditing() {
    return editing;
  }

  export function isSaving() {
    return saving;
  }

  function saveValue() {
    if (saving) return;
    onsave?.();
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
    setTimeout(() => {
      if (editing && !saving) {
        saveValue();
      }
    }, 100);
  }

  function handleClick() {
    if (enableDoubleClick && enableSingleClick) {
      if (clickTimeout) {
        clearTimeout(clickTimeout);
        clickTimeout = null;
        startEditing();
      } else {
        clickTimeout = setTimeout(() => {
          clickTimeout = null;
          onclickProp?.();
        }, 200);
      }
    } else if (enableSingleClick) {
      onclickProp?.();
    } else if (enableDoubleClick) {
      return;
    } else {
      startEditing();
    }
  }

  function handleDoubleClick() {
    if (enableDoubleClick && !enableSingleClick) {
      startEditing();
    }
  }
</script>

{#if editing}
  <div class="inline-flex items-center gap-1 w-full">
    <div class="flex-1 relative">
      {@render editingInput({ saving, error, onkeydown: handleKeydown, onblur: handleBlur })}
      {#if error}
        <div class="absolute top-full left-0 mt-1 text-xs px-2 py-1 border rounded shadow-sm z-10 inline-editor-error">
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
          title={t('editors.saveEnter')}
        >
          <Check class="w-4 h-4" />
        </button>
        <button
          type="button"
          onclick={cancelEditing}
          class="p-1 rounded cancel-btn"
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
    class:cursor-pointer={enableSingleClick || enableDoubleClick}
    {disabled}
    style="all: unset; display: block; width: 100%;"
  >
    {@render displayContent()}
  </button>
{/if}

<style>
  .inline-editor-error {
    color: var(--ds-text-danger);
    background-color: var(--ds-surface-raised);
    border-color: var(--ds-border-danger);
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
    background-color: var(--ds-background-neutral-hovered);
  }
</style>
