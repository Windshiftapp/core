<script>
  import { createEventDispatcher } from 'svelte';
  import { fade } from 'svelte/transition';
  import { AlertTriangle, X, Trash2, Users } from 'lucide-svelte';
  import Button from '../components/Button.svelte';
  import Spinner from '../components/Spinner.svelte';
  import ItemPicker from '../pickers/ItemPicker.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { api } from '../api';

  const dispatch = createEventDispatcher();

  let {
    show = $bindable(false),
    item = null, // { id, title, parentId }
    ondeleted = null, // callback when deletion completes
    onerror = null // callback on error
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

  // Derived values
  const hasChildren = $derived(deleteInfo?.hasChildren || false);
  const descendantCount = $derived(deleteInfo?.descendantCount || 0);
  const totalCount = $derived(descendantCount + 1);
  const canConfirmDelete = $derived(
    (selectedMode === 'reparent' && (selectedNewParentId !== null || deleteInfo?.parentId === null)) ||
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
    if (show && item?.id) {
      loadDeleteInfo();
    } else {
      // Reset state when dialog closes
      deleteInfo = null;
      selectedMode = 'deleteAll';
      confirmText = '';
      error = null;
      reparentCandidates = [];
      selectedNewParentId = null;
    }
  });

  // Load reparent candidates when reparent mode is selected and we have children
  $effect(() => {
    if (selectedMode === 'reparent' && hasChildren && deleteInfo?.hierarchyLevel != null) {
      loadReparentCandidates();
    }
  });

  async function loadDeleteInfo() {
    loadingInfo = true;
    error = null;
    try {
      deleteInfo = await api.items.getDeleteInfo(item.id);
      // If item has a parent, default to that as the new parent
      if (deleteInfo?.parentId) {
        selectedNewParentId = deleteInfo.parentId;
      }
    } catch (err) {
      console.error('Failed to load delete info:', err);
      error = err.message;
    } finally {
      loadingInfo = false;
    }
  }

  async function loadReparentCandidates() {
    if (!deleteInfo?.workspaceId || deleteInfo?.hierarchyLevel == null) {
      reparentCandidates = [];
      return;
    }

    loadingCandidates = true;
    try {
      // Get items at the same hierarchy level in the same workspace
      // These are valid candidates for reparenting (siblings at the same level)
      const response = await api.items.getAll({
        workspace_id: deleteInfo.workspaceId,
        level: deleteInfo.hierarchyLevel,
        limit: 100
      });

      const items = response?.items || response || [];
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
        // First reparent children to the selected new parent, then delete the item
        await api.items.reparentChildren(item.id, selectedNewParentId);
        await api.items.delete(item.id);
        ondeleted?.({ mode: 'reparent', deletedCount: 1, newParentId: selectedNewParentId });
      } else {
        // Cascade delete
        const result = await api.items.deleteCascade(item.id);
        ondeleted?.({ mode: 'deleteAll', deletedCount: result.deletedCount });
      }
      dispatch('deleted');
      show = false;
    } catch (err) {
      console.error('Delete failed:', err);
      error = err.message;
      onerror?.(err);
    } finally {
      loading = false;
    }
  }

  function handleNewParentSelect(event) {
    const selected = event.detail;
    selectedNewParentId = selected?.id || null;
  }

  function handleCancel() {
    dispatch('cancel');
    show = false;
  }

  function handleBackdropClick(event) {
    if (event.target === event.currentTarget && !loading) {
      handleCancel();
    }
  }

  function handleKeydown(event) {
    if (event.key === 'Escape' && show && !loading) {
      handleCancel();
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

{#if show}
  <div
    transition:fade={{ duration: 150 }}
    class="modal-backdrop fixed inset-0 z-50 flex items-center justify-center p-4"
    onclick={handleBackdropClick}
    role="dialog"
    aria-modal="true"
    aria-labelledby="dialog-title"
  >
    <div
      class="bg-white rounded-xl shadow-xl max-w-md w-full transform transition-all"
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
              class="flex items-start gap-3 p-3 rounded-lg border cursor-pointer transition-colors {selectedMode === 'deleteAll' ? 'border-red-500' : ''}"
              style="border-color: {selectedMode === 'deleteAll' ? 'var(--ds-border-danger)' : 'var(--ds-border)'}; background-color: {selectedMode === 'deleteAll' ? 'var(--ds-background-danger)' : 'transparent'};"
            >
              <input
                type="radio"
                name="deleteMode"
                value="deleteAll"
                bind:group={selectedMode}
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
              class="flex items-start gap-3 p-3 rounded-lg border cursor-pointer transition-colors"
              style="border-color: {selectedMode === 'reparent' ? 'var(--ds-border-selected)' : 'var(--ds-border)'}; background-color: {selectedMode === 'reparent' ? 'var(--ds-background-selected)' : 'transparent'};"
            >
              <input
                type="radio"
                name="deleteMode"
                value="reparent"
                bind:group={selectedMode}
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
              <label class="block text-sm font-medium mb-2" style="color: var(--ds-text);">
                {t('items.selectNewParent')}
              </label>
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
                  on:select={handleNewParentSelect}
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
            <div class="mt-4">
              <label class="block text-sm font-medium mb-2" style="color: var(--ds-text);">
                {t('items.typeToConfirm', { title: item?.title })}
              </label>
              <input
                type="text"
                bind:value={confirmText}
                placeholder={t('items.confirmationPlaceholder')}
                disabled={loading}
                class="w-full px-3 py-2 rounded-lg border text-sm"
                style="border-color: var(--ds-border); background-color: var(--ds-background-input); color: var(--ds-text);"
              />
            </div>
          {/if}
        {:else}
          <!-- No children - simple confirmation -->
          <p class="text-sm leading-relaxed" style="color: var(--ds-text-subtle);">
            {t('items.confirmDeleteItem', { title: item?.title || '' })}
          </p>
        {/if}

        <!-- Error message -->
        {#if error}
          <div class="mt-4 p-3 rounded-lg" style="background-color: var(--ds-background-danger);">
            <p class="text-sm" style="color: var(--ds-text-danger);">{error}</p>
          </div>
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
  </div>
{/if}

<style>
  .modal-backdrop {
    background-color: rgba(0, 0, 0, 0.5);
    backdrop-filter: blur(2px);
  }
</style>
