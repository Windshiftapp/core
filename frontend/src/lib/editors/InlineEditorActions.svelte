<script>
  import Check from '@lucide/svelte/icons/check';
  import X from '@lucide/svelte/icons/x';
  import Spinner from '../components/Spinner.svelte';
  import { t } from '../stores/i18n.svelte.js';

  // Save / cancel controls shared by the inline editors. Shows a spinner while
  // the parent confirms the save.
  let { saving = false, onsave, oncancel } = $props();
</script>

{#if saving}
  <Spinner size="sm" />
{:else}
  <button
    type="button"
    onclick={onsave}
    class="p-1 rounded inline-editor-save"
    title={t('editors.saveEnter')}
  >
    <Check class="w-4 h-4" />
  </button>
  <button
    type="button"
    onclick={oncancel}
    class="p-1 rounded inline-editor-cancel"
    title={t('editors.cancelEscape')}
  >
    <X class="w-4 h-4" />
  </button>
{/if}

<style>
  .inline-editor-save {
    color: var(--ds-text-success);
  }
  .inline-editor-save:hover {
    background-color: var(--ds-background-success-subtle);
  }
  .inline-editor-cancel {
    color: var(--ds-text-subtle);
  }
  .inline-editor-cancel:hover {
    background-color: var(--ds-background-neutral-hovered);
  }
</style>
