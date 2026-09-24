<script>
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';
  import TextField from '../../components/TextField.svelte';
  import DescriptionText from '../../components/DescriptionText.svelte';
  import { api } from '../../api.js';
  import { t } from '../../stores/i18n.svelte.js';

  let { isOpen = $bindable(false), item = null, onMerged = null } = $props();

  let duplicateKey = $state('');
  let merging = $state(false);
  let error = $state('');

  $effect(() => {
    if (!isOpen) {
      duplicateKey = '';
      merging = false;
      error = '';
    }
  });

  function close() {
    isOpen = false;
  }

  // Accepts "KEY-123" (any workspace the viewer can see) or a bare id.
  async function resolveDuplicate(raw) {
    const trimmed = (raw || '').trim();
    if (!trimmed) {
      throw new Error(t('items.mergeKeyRequired'));
    }
    const keyMatch = trimmed.match(/^([A-Za-z0-9]+)-(\d+)$/);
    if (keyMatch) {
      const resolved = await api.items.getByKey(keyMatch[1], Number(keyMatch[2]));
      return resolved.id;
    }
    if (/^\d+$/.test(trimmed)) {
      return Number(trimmed);
    }
    throw new Error(t('items.mergeKeyInvalid'));
  }

  async function handleMerge() {
    if (merging || !item) return;
    try {
      merging = true;
      error = '';
      const duplicateId = await resolveDuplicate(duplicateKey);
      if (Number(duplicateId) === Number(item.id)) {
        error = t('items.mergeSelfError');
        return;
      }
      const result = await api.items.mergeInto(item.id, [duplicateId]);
      close();
      onMerged?.(result);
    } catch (err) {
      error = err?.message || t('items.mergeFailed');
    } finally {
      merging = false;
    }
  }
</script>

<Modal bind:isOpen onclose={close} maxWidth="max-w-md" onSubmit={handleMerge} submitDisabled={merging || !duplicateKey.trim()}>
  {#snippet children(submitHint)}
  <ModalHeader
    title={t('items.mergeTitle')}
    subtitle={item ? `${item.workspace_key || ''}-${item.workspace_item_number}` : ''}
    showCloseButton={false}
  />
  <div class="p-6 space-y-4">
    <div>
      <TextField
        label={t('items.mergeDuplicateLabel')}
        labelColor="default"
        placeholder="KEY-123"
        bind:value={duplicateKey}
        dataTestid="item-merge-duplicate-input"
        onkeydown={(e) => { if (e.key === 'Enter') handleMerge(); }}
      />
      <DescriptionText>{t('items.mergeHelp')}</DescriptionText>
    </div>
    {#if error}
      <div class="text-sm rounded p-2" style="background: var(--ds-surface-danger, rgba(239, 68, 68, 0.1)); color: var(--ds-text-danger);" data-testid="item-merge-error">
        {error}
      </div>
    {/if}
  </div>
  <DialogFooter
    cancelLabel={t('common.cancel')}
    confirmLabel={t('items.mergeConfirm')}
    loadingLabel={t('items.mergeWorking')}
    confirmTestid="item-merge-confirm"
    cancelTestid="item-merge-cancel"
    confirmDisabled={merging || !duplicateKey.trim()}
    loading={merging}
    showKeyboardHint={true}
    confirmKeyboardHint={submitHint}
    onCancel={close}
    onConfirm={handleMerge}
  />
  {/snippet}
</Modal>
