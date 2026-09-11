<script>
  import { AlertTriangle, X, Trash2, Users } from '@lucide/svelte';
  import Button from '../components/Button.svelte';
  import ModalBackdrop from '../components/ModalBackdrop.svelte';
  import Spinner from '../components/Spinner.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import FormField from '../components/FormField.svelte';
  import Input from '../components/Input.svelte';
  import Radio from '../components/Radio.svelte';
  import ItemPicker from '../pickers/ItemPicker.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { api } from '../api';

  let {
    show = $bindable(false),
    item = null, // { id, title, parentId }
    ondeleted = null, // callback when deletion completes
    onerror = null, // callback on error
    oncancel = null
  } = $props();

  // Internal state
  let loading = $state(false);
  let loadingInfo = $state(false);
  let loadingCandidates = $state(false);
  let deleteInfo = $state(null);
  let selectedMode = $state('deleteAll'); // 'deleteAll' | 'reparent'
  let confirmText = $state('');
  let error = $state(null);
  let reparentCandidates = $state([]);
  let selectedNewParentId = $state(null);
  let requestedItemId = $state(null);

  // Derived values
  const hasChildren = $derived(deleteInfo?.has_children || false);
  const descendantCount = $derived(deleteInfo?.descendant_count || 0);
  const totalCount = $derived(descendantCount + 1);
  const canConfirmDelete = $derived(
    (deleteInfo && !hasChildren) ||
    (selectedMode === 'reparent' && (selectedNewParentId !== null || deleteInfo?.parent_id === null)) ||
    (selectedMode === 'deleteAll' && confirmText.trim() === item?.title?.trim())
  );

  // ItemPicker config for reparent candidates
  const reparentPickerConfig = {
    primary: {
      text: (item) => item.title
    },
    secondary: {
      text: (item) => `${item.workspace_key || 'WORK'}-${item.workspace_item_number || item.id}`
    },
    searchFields: ['title', 'workspace_key'],
    getValue: (item) => item.id,
    getLabel: (item) => item.title
  };

  // Load delete info when dialog opens
  $effect(() => {
    if (!show) {
      // Reset state when dialog closes
      requestedItemId = null;
      deleteInfo = null;
      selectedMode = 'deleteAll';
      confirmText = '';
      error = null;
      reparentCandidates = [];
      selectedNewParentId = null;
      return;
    }

    // Item detail may refresh from SSE while this dialog is open. Only reload
    // delete metadata when the selected item actually changes so a same-item
    // refresh cannot invalidate confirmation text the user already entered.
    if (item?.id && requestedItemId !== item.id) {
      requestedItemId = item.id;
      loadDeleteInfo(item.id);
    }
  });

  // Load reparent candidates when reparent mode is selected and we have children
  $effect(() => {
    if (selectedMode === 'reparent' && hasChildren && deleteInfo?.hierarchy_level != null) {
      loadReparentCandidates();
    }
  });

  async function loadDeleteInfo(itemId) {
    loadingInfo = true;
    error = null;
    try {
      const result = await api.items.getDeleteInfo(itemId);
      if (!show || requestedItemId !== itemId) return;
      deleteInfo = result;
      // If item has a parent, default to that as the new parent
      if (deleteInfo?.parent_id) {
        selectedNewParentId = deleteInfo.parent_id;
      }
    } catch (err) {
      if (!show || requestedItemId !== itemId) return;
      console.error('Failed to load delete info:', err);
      error = err.message;
    } finally {
      if (requestedItemId === itemId) loadingInfo = false;
    }
  }

  async function loadReparentCandidates() {
    if (!deleteInfo?.workspace_id || deleteInfo?.hierarchy_level == null) {
      reparentCandidates = [];
      return;
    }

    loadingCandidates = true;
    try {
      // Get items at the same hierarchy level in the same workspace
      // These are valid candidates for reparenting (siblings at the same level)
      const response = await api.items.getAll({
        workspace_id: deleteInfo.workspace_id,
        level: deleteInfo.hierarchy_level,
        limit: 100
      });

      const items = response?.data ?? [];
      // Filter out the item being deleted and its descendants
      reparentCandidates = items.filter(i => i.id !== item.id);
    } catch (err) {
      console.error('Failed to load reparent candidates:', err);
      reparentCandidates = [];
    } finally {
      loadingCandidates = false;
    }
  }

  async function handleDelete() {
    if (!canConfirmDelete) return;

    loading = true;
    error = null;

    try {
      if (selectedMode === 'reparent') {
        // Move children before deleting their parent.
        await api.items.reparentChildren(item.id, selectedNewParentId);
        await api.items.delete(item.id);
        ondeleted?.({ mode: 'reparent', deletedCount: 1, newParentId: selectedNewParentId });
      } else {
        await api.items.deleteCascade(item.id);
        ondeleted?.({ mode: 'deleteAll' });
      }
      show = false;
    } catch (err) {
      console.error('Delete failed:', err);
      error = err.message;
      onerror?.(err);
    } finally {
      loading = false;
    }
  }

  function handleNewParentSelect(selected) {
    selectedNewParentId = selected?.id || null;
  }

  function handleCancel() {
    oncancel?.();
    show = false;
  }


</script>

<ModalBackdrop bind:show closeOnClick={!loading} closeOnEscape={!loading} zIndex={60} onclose={handleCancel} ariaLabelledBy="dialog-title">
    <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
    <div
      role="presentation"
      data-testid="delete-item-dialog"
      class="bg-white rounded-lg shadow-xl max-w-md w-full transform transition-all"
      style="background-color: var(--ds-surface-raised);"
      onclick={(e) => e.stopPropagation()}
    >
      <!-- Header -->
      <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
        <div class="flex items-center gap-3">
          <div class="flex-shrink-0">
            <AlertTriangle
              class="w-6 h-6"
              style="color: var(--ds-icon-danger);"
            />
          </div>
          <h3
            id="dialog-title"
            class="text-lg font-medium flex-1"
            style="color: var(--ds-text);"
          >
            {hasChildren ? t('items.deleteItemWithChildren') : t('items.deleteWorkItem')}
          </h3>
          <Button
            variant="ghost"
            icon={X}
            onclick={handleCancel}
            disabled={loading}
            title={t('common.close')}
          />
        </div>
      </div>

      <!-- Body -->
      <div class="px-6 py-4">
        {#if loadingInfo}
          <div class="flex items-center justify-center py-8">
            <Spinner size="medium" />
          </div>
        {:else if hasChildren}
          <!-- Has children - show options -->
          <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
            {descendantCount === 1
              ? t('items.itemHasChildrenSingular')
              : t('items.itemHasChildren', { count: descendantCount })}
          </p>

          <!-- Delete options -->
          <div class="space-y-3 mb-4">
            <!-- Delete All option -->
            <label
              class="flex items-start gap-3 p-3 rounded-lg border cursor-pointer transition-colors {selectedMode === 'deleteAll'
                ? 'border-ds-border-danger bg-ds-danger-subtle'
                : 'border-ds-border'}"
            >
              <Radio
                name="deleteMode"
                value="deleteAll"
                bind:groupValue={selectedMode}
                disabled={loading}
                class="mt-1"
              />
              <div class="flex-1">
                <div class="flex items-center gap-2">
                  <Trash2 class="w-4 h-4" style="color: var(--ds-icon-danger);" />
                  <span class="font-medium" style="color: var(--ds-text);">
                    {t('items.deleteAllOption', { count: totalCount })}
                  </span>
                </div>
                <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
                  {t('items.deleteAllDescription')}
                </p>
              </div>
            </label>

            <!-- Reparent option -->
            <label
              class="flex items-start gap-3 p-3 rounded-lg border cursor-pointer transition-colors {selectedMode === 'reparent'
                ? 'border-ds-interactive bg-ds-surface-selected'
                : 'border-ds-border'}"
            >
              <Radio
                name="deleteMode"
                value="reparent"
                dataTestid="item-delete-reparent"
                bind:groupValue={selectedMode}
                disabled={loading}
                class="mt-1"
              />
              <div class="flex-1">
                <div class="flex items-center gap-2">
                  <Users class="w-4 h-4" style="color: var(--ds-icon);" />
                  <span class="font-medium" style="color: var(--ds-text);">
                    {t('items.reparentOption')}
                  </span>
                </div>
                <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
                  {t('items.reparentDescription')}
                </p>
              </div>
            </label>
          </div>

          <!-- New parent picker for reparent mode -->
          {#if selectedMode === 'reparent'}
            <div class="mt-4">
              <span class="block text-sm font-medium mb-2" style="color: var(--ds-text);">
                {t('items.selectNewParent')}
              </span>
              {#if loadingCandidates}
                <div class="flex items-center gap-2 p-3 rounded-lg border" style="border-color: var(--ds-border); background-color: var(--ds-background-input);">
                  <Spinner size="small" />
                  <span class="text-sm" style="color: var(--ds-text-subtle);">{t('common.loading')}</span>
                </div>
              {:else}
                <ItemPicker
                  bind:value={selectedNewParentId}
                  items={reparentCandidates}
                  config={reparentPickerConfig}
                  placeholder={t('items.selectNewParentPlaceholder')}
                  showUnassigned={true}
                  unassignedLabel={t('items.makeRootItem')}
                  disabled={loading}
                  onSelect={handleNewParentSelect}
                />
                <p class="text-xs mt-1.5" style="color: var(--ds-text-subtle);">
                  {reparentCandidates.length > 0
                    ? t('items.reparentLevelHint')
                    : t('items.noOtherItemsAtLevel')}
                </p>
              {/if}
            </div>
          {/if}

          <!-- Confirmation text input for delete all -->
          {#if selectedMode === 'deleteAll'}
            <FormField label={t('items.typeToConfirm', { title: item?.title })} id="delete-confirm-input" class="mt-4">
              <Input
                id="delete-confirm-input"
                dataTestid="delete-item-confirmation"
                type="text"
                bind:value={confirmText}
                placeholder={t('items.confirmationPlaceholder')}
                disabled={loading}
                size="small"
              />
            </FormField>
          {/if}
        {:else}
          <!-- No children - simple confirmation -->
          <p class="text-sm leading-relaxed" style="color: var(--ds-text-subtle);">
            {t('items.confirmDeleteItem', { title: item?.title || '' })}
          </p>
        {/if}

        <!-- Error message -->
        {#if error}
          <AlertBox variant="error" message={error} class="mt-4" />
        {/if}
      </div>

      <!-- Footer -->
      <div class="px-6 py-4 border-t flex justify-end gap-3" style="border-color: var(--ds-border);">
        <Button
          variant="default"
          onclick={handleCancel}
          size="small"
          disabled={loading}
        >
          {t('common.cancel')}
        </Button>

        {#if loadingInfo}
          <!-- Waiting for info to load -->
        {:else if hasChildren}
          <Button
            id="item-delete-confirm"
            dataTestid="delete-item-confirm"
            variant="danger"
            onclick={handleDelete}
            size="small"
            disabled={!canConfirmDelete || loading}
            {loading}
          >
            {selectedMode === 'deleteAll'
              ? t('items.deleteAllItems')
              : t('items.reparentAndDelete')}
          </Button>
        {:else}
          <Button
            id="item-delete-confirm"
            dataTestid="delete-item-confirm"
            variant="danger"
            onclick={handleDelete}
            size="small"
            disabled={loading}
            {loading}
          >
            {t('common.delete')}
          </Button>
        {/if}
      </div>
    </div>
</ModalBackdrop>
