<script>
  import Button from '../components/Button.svelte';

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
    cancelLabel = 'Cancel',
    confirmLabel = 'Confirm',
    variant = 'primary', // 'primary' | 'danger'
    loading = false,
    disabled = false,
    showCancel = true,
    onCancel = null,
    onConfirm = null,
    extra = null, // Snippet for extra content (left side)
    confirmType = 'button', // 'button' | 'submit'
    showKeyboardHint = false,
    cancelKeyboardHint = 'Esc',
    loadingLabel = null, // Optional loading text (e.g., "Saving...")
    class: className = ''
  } = $props();
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
      >
        {cancelLabel}
      </Button>
    {/if}
    {#if onConfirm}
      <Button
        type={confirmType}
        {variant}
        onclick={onConfirm}
        {loading}
        disabled={disabled || loading}
        keyboardHint={showKeyboardHint ? '⏎' : undefined}
      >
        {loading && loadingLabel ? loadingLabel : confirmLabel}
      </Button>
    {/if}
  </div>
</div>
