<script>
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';
  import Label from '../../components/Label.svelte';
  import DescriptionText from '../../components/DescriptionText.svelte';
  import ItemPicker from '../../pickers/ItemPicker.svelte';
  import { links } from '../../api/configuration.js';
  import { items } from '../../api/items.js';
  import { t } from '../../stores/i18n.svelte.js';

  let { isOpen = $bindable(false), item = null, preselected = null, onMerged = null } = $props();

  let duplicateId = $state(null);
  let selectedDuplicate = $state(null);
  let options = $state([]);
  let searchLoading = $state(false);
  let merging = $state(false);
  let error = $state('');
  let searchVersion = 0;

  // Seed the selection when the dialog opens (from the actions menu it is
  // empty; from the duplicates panel the candidate is passed in) and clear
  // everything on close.
  let wasOpen = false;
  $effect(() => {
    if (isOpen && !wasOpen) {
      duplicateId = preselected?.id ?? null;
      selectedDuplicate = preselected ?? null;
      options = preselected ? [preselected] : [];
      error = '';
      merging = false;
      searchVersion += 1;
    } else if (!isOpen && wasOpen) {
      duplicateId = null;
      selectedDuplicate = null;
      options = [];
      error = '';
      merging = false;
      searchVersion += 1;
    }
    wasOpen = isOpen;
  });

  const duplicatePickerConfig = {
    primary: { text: (entry) => entry.title || '' },
    secondary: {
      text: (entry) => {
        if (entry.workspace_key && entry.workspace_item_number) {
          return `${entry.workspace_key}-${entry.workspace_item_number}`;
        }
        return entry.workspace_name || '';
      }
    },
    getValue: (entry) => entry.id,
    getLabel: (entry) => entry.title || ''
  };

  async function searchDuplicates(query) {
    const trimmed = (query || '').trim();
    const version = ++searchVersion;

    if (trimmed.length < 2) {
      options = selectedDuplicate ? [selectedDuplicate] : [];
      searchLoading = false;
      return;
    }

    searchLoading = true;
    try {
      const results = await links.search(trimmed, 'item', 20);
      if (version !== searchVersion) return;
      const list = (Array.isArray(results) ? results : []).filter(
        (entry) => Number(entry.id) !== Number(item?.id)
      );
      const keepSelected =
        selectedDuplicate &&
        !list.some((entry) => Number(entry.id) === Number(selectedDuplicate.id));
      options = keepSelected ? [selectedDuplicate, ...list] : list;
    } catch {
      if (version !== searchVersion) return;
      options = selectedDuplicate ? [selectedDuplicate] : [];
    } finally {
      if (version === searchVersion) searchLoading = false;
    }
  }

  function handleSelect(selected) {
    if (selected && !Array.isArray(selected)) {
      selectedDuplicate = selected;
    } else if (selected == null) {
      selectedDuplicate = null;
    }
  }

  // Merge folds a different requester's ticket in a degraded way: comments
  // move but become private, and files, email thread, and participants stay
  // on the duplicate (WI-1566). Warn before the agent commits.
  function requesterID(entry) {
    const value = entry?.creator_portal_customer_id;
    return value == null ? null : Number(value);
  }

  let differentRequester = $derived(
    !!selectedDuplicate && requesterID(item) !== requesterID(selectedDuplicate)
  );

  function close() {
    isOpen = false;
  }

  async function handleMerge() {
    if (merging || !item || duplicateId == null) return;
    if (Number(duplicateId) === Number(item.id)) {
      error = t('items.mergeSelfError');
      return;
    }
    try {
      merging = true;
      error = '';
      const result = await items.mergeInto(item.id, [duplicateId]);
      close();
      onMerged?.(result);
    } catch (err) {
      error = err?.message || t('items.mergeFailed');
    } finally {
      merging = false;
    }
  }
</script>

<Modal bind:isOpen onclose={close} maxWidth="max-w-md" dataTestid="item-merge-dialog" onSubmit={handleMerge} submitDisabled={merging || duplicateId == null}>
  {#snippet children(submitHint)}
  <ModalHeader
    title={t('items.mergeTitle')}
    subtitle={item ? `${item.workspace_key || ''}-${item.workspace_item_number}` : ''}
    showCloseButton={false}
  />
  <div class="p-6 space-y-4">
    <div>
      <Label color="default" class="mb-2">{t('items.mergeDuplicateLabel')}</Label>
      <ItemPicker
        bind:value={duplicateId}
        items={options}
        loading={searchLoading}
        config={duplicatePickerConfig}
        placeholder={t('placeholders.searchWorkItems')}
        allowClear={true}
        onSearchChange={searchDuplicates}
        onSelect={handleSelect}
        searchTestid="item-merge-duplicate-search"
        optionTestid={(option) => `item-merge-duplicate-option-${option.value}`}
      />
      <DescriptionText>{t('items.mergeHelp')}</DescriptionText>
    </div>
    {#if differentRequester}
      <div
        class="text-sm rounded p-2"
        style="background: var(--ds-surface-warning, rgba(234, 88, 12, 0.1)); color: var(--ds-text-warning, var(--ds-text));"
        data-testid="item-merge-requester-warning"
      >
        {t('items.mergeDifferentRequester')}
      </div>
    {/if}
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
    confirmDisabled={merging || duplicateId == null}
    loading={merging}
    showKeyboardHint={true}
    confirmKeyboardHint={submitHint}
    onCancel={close}
    onConfirm={handleMerge}
  />
  {/snippet}
</Modal>
