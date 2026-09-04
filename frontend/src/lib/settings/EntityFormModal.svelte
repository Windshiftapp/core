<script>
  import Button from '../components/Button.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let {
    title,
    onclose,
    onsubmit,
    disabled = false,
    saving = false,
    confirmLabel = t('common.save'),
    submitId = undefined,
    maxWidth = 'max-w-lg',
    fields,
  } = $props();
</script>

<Modal isOpen={true} {maxWidth} {onclose} onSubmit={onsubmit} submitDisabled={disabled || saving}>
  {#snippet children(submitHint)}
    <ModalHeader {title} {onclose} />
    <div class="p-4 space-y-4">
      {@render fields()}
      <div class="flex justify-end gap-2 pt-2 border-t" style="border-color: var(--ds-border);">
        <Button variant="secondary" onclick={onclose} keyboardHint="Esc" dataTestid="entity-form-cancel">
          {t('common.cancel')}
        </Button>
        <Button
          id={submitId}
          variant="primary"
          onclick={onsubmit}
          loading={saving}
          disabled={disabled || saving}
          keyboardHint={submitHint}
          dataTestid="entity-form-confirm"
        >
          {confirmLabel}
        </Button>
      </div>
    </div>
  {/snippet}
</Modal>
