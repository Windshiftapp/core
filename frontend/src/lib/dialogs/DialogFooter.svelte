<script>
  import Button from '../components/Button.svelte';
  import { t } from '../stores/i18n.svelte.js';

  /**
   * DialogFooter - Standard footer for modal dialogs with cancel/confirm buttons
   *
   * @example
   * <DialogFooter
   *   onCancel={() => showModal = false}
   *   onConfirm={handleSave}
   *   confirmLabel="Save"
   * />
   *
   * @example with extra content
   * <DialogFooter onCancel={close} onConfirm={save}>
   *   {#snippet extra()}
   *     <Button variant="ghost" onclick={resetForm}>Reset</Button>
   *   {/snippet}
   * </DialogFooter>
   */
  let {
    cancelLabel = null,
    confirmLabel = null,
    variant = 'primary',
    loading = false,
    disabled = false,
    confirmDisabled = false,
    showCancel = true,
    onCancel = null,
    onConfirm = null,
    extra = null,
    confirmType = 'button',
    showKeyboardHint = false,
    cancelKeyboardHint = 'Esc',
    confirmKeyboardHint = '⏎',
    loadingLabel = null,
    confirmTestid = 'dialog-confirm',
    cancelTestid = 'dialog-cancel',
    class: className = ''
  } = $props();
  const isConfirmDisabled = $derived(disabled || confirmDisabled);
</script>

<div class="px-6 py-4 border-t flex items-center {className}" style="border-color: var(--ds-border);">
  {#if extra}
    <div class="flex-1">
      {@render extra()}
    </div>
  {:else}
    <div class="flex-1"></div>
  {/if}

  <div class="flex items-center gap-3">
    {#if showCancel && onCancel}
      <Button
        variant="ghost"
        onclick={onCancel}
        disabled={loading}
        keyboardHint={showKeyboardHint ? cancelKeyboardHint : undefined}
        dataTestid={cancelTestid}
      >
        {cancelLabel ?? t('dialogs.cancel')}
      </Button>
    {/if}
    {#if onConfirm}
      <Button
        type={confirmType}
        {variant}
        onclick={onConfirm}
        {loading}
        disabled={isConfirmDisabled || loading}
        keyboardHint={showKeyboardHint ? confirmKeyboardHint : undefined}
        dataTestid={confirmTestid}
      >
        {loading && loadingLabel ? loadingLabel : (confirmLabel ?? t('dialogs.confirm'))}
      </Button>
    {/if}
  </div>
</div>
