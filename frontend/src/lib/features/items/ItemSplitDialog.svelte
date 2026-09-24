<script>
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';
  import TextField from '../../components/TextField.svelte';
  import Checkbox from '../../components/Checkbox.svelte';
  import { api } from '../../api.js';
  import { t } from '../../stores/i18n.svelte.js';

  let { isOpen = $bindable(false), item = null, onSplit = null } = $props();

  let title = $state('');
  let comments = $state([]);
  let selected = $state(new Set());
  let splitting = $state(false);
  let loadingComments = $state(false);
  let error = $state('');

  $effect(() => {
    if (isOpen && item) loadComments();
    if (!isOpen) reset();
  });

  function reset() {
    title = '';
    comments = [];
    selected = new Set();
    splitting = false;
    error = '';
  }

  function close() {
    isOpen = false;
  }

  async function loadComments() {
    loadingComments = true;
    error = '';
    try {
      const result = await api.getComments(item.id);
      comments = result?.comments || result || [];
    } catch (err) {
      comments = [];
      error = err?.message || t('items.splitLoadError');
    } finally {
      loadingComments = false;
    }
  }

  function toggleComment(id) {
    const next = new Set(selected);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    selected = next;
  }

  async function handleSplit() {
    if (splitting || !item) return;
    if (!title.trim()) {
      error = t('items.splitTitleRequired');
      return;
    }
    if (selected.size === 0) {
      error = t('items.splitSelectionRequired');
      return;
    }
    try {
      splitting = true;
      error = '';
      const result = await api.items.split(item.id, {
        title: title.trim(),
        comment_ids: [...selected],
      });
      close();
      onSplit?.(result);
    } catch (err) {
      error = err?.message || t('items.splitFailed');
    } finally {
      splitting = false;
    }
  }
</script>

<Modal bind:isOpen onclose={close} maxWidth="max-w-lg" onSubmit={handleSplit} submitDisabled={splitting || !title.trim() || selected.size === 0}>
  {#snippet children(submitHint)}
  <ModalHeader
    title={t('items.splitTitle')}
    subtitle={item ? `${item.workspace_key || ''}-${item.workspace_item_number}` : ''}
    showCloseButton={false}
  />
  <div class="p-6 space-y-4">
    <TextField
      label={t('items.splitNewTitleLabel')}
      labelColor="default"
      placeholder={t('items.splitNewTitlePlaceholder')}
      bind:value={title}
      dataTestid="item-split-title-input"
    />

    <div>
      <div class="text-sm font-medium mb-2" style="color: var(--ds-text);">
        {t('items.splitSelectComments')}
      </div>
      {#if loadingComments}
        <div class="text-sm" style="color: var(--ds-text-subtle);">{t('common.loading')}</div>
      {:else if comments.length === 0}
        <div class="text-sm" style="color: var(--ds-text-subtle);">{t('items.splitNoComments')}</div>
      {:else}
        <div class="space-y-1.5 max-h-56 overflow-y-auto p-1">
          {#each comments as comment (comment.id)}
            <label class="flex items-start gap-2 p-2 rounded cursor-pointer" style="background: var(--ds-surface-raised);">
              <input
                type="checkbox"
                class="mt-0.5"
                checked={selected.has(comment.id)}
                onchange={() => toggleComment(comment.id)}
                data-testid="item-split-comment-{comment.id}"
              />
              <span class="text-sm line-clamp-2" style="color: var(--ds-text);">
                {comment.content}
              </span>
            </label>
          {/each}
        </div>
      {/if}
    </div>

    {#if error}
      <div class="text-sm rounded p-2" style="background: var(--ds-surface-danger, rgba(239, 68, 68, 0.1)); color: var(--ds-text-danger);" data-testid="item-split-error">
        {error}
      </div>
    {/if}
  </div>
  <DialogFooter
    cancelLabel={t('common.cancel')}
    confirmLabel={t('items.splitConfirm')}
    loadingLabel={t('items.splitWorking')}
    confirmTestid="item-split-confirm"
    cancelTestid="item-split-cancel"
    confirmDisabled={splitting || !title.trim() || selected.size === 0}
    loading={splitting}
    showKeyboardHint={true}
    confirmKeyboardHint={submitHint}
    onCancel={close}
    onConfirm={handleSplit}
  />
  {/snippet}
</Modal>
