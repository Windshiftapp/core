<script>
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';
  import BasePicker from '../../pickers/BasePicker.svelte';
  import NativeSelect from '../../components/NativeSelect.svelte';
  import { api } from '../../api.js';
  import { workspacePermissions, workspacesStore } from '../../stores';
  import { t } from '../../stores/i18n.svelte.js';

  let {
    isOpen = $bindable(false),
    item,
    onMoved = null,
  } = $props();

  let workspaces = $state([]);
  let destinationWorkspaceId = $state(null);
  let targetItemTypeId = $state(null);
  let targetStatusId = $state(null);
  let targetPriorityId = $state(null);
  let preview = $state(null);
  let loadingWorkspaces = $state(false);
  let loadingPreview = $state(false);
  let saving = $state(false);
  let error = $state('');
  let previewVersion = 0;

  $effect(() => {
    if (isOpen && item) loadWorkspaces();
    if (!isOpen) reset();
  });

  function reset() {
    destinationWorkspaceId = null;
    targetItemTypeId = null;
    targetStatusId = null;
    targetPriorityId = null;
    preview = null;
    error = '';
    previewVersion += 1;
  }

  async function loadWorkspaces() {
    loadingWorkspaces = true;
    error = '';
    try {
      const all = await workspacesStore.load();
      workspaces = (all || []).filter((workspace) =>
        workspace.active !== false &&
        Number(workspace.id) !== Number(item.workspace_id) &&
        workspacePermissions.canCreate(workspace.id)
      );
    } catch (err) {
      workspaces = [];
      error = err?.message || t('items.moveWorkspaceLoadError');
    } finally {
      loadingWorkspaces = false;
    }
  }

  async function selectWorkspace(workspace) {
    if (!workspace) return;
    destinationWorkspaceId = workspace.id;
    targetItemTypeId = null;
    targetStatusId = null;
    targetPriorityId = null;
    await loadPreview(true);
  }

  async function loadPreview(useRecommendation = false) {
    if (!destinationWorkspaceId) return;
    const version = ++previewVersion;
    loadingPreview = true;
    error = '';
    try {
      const payload = { destination_workspace_id: Number(destinationWorkspaceId) };
      if (!useRecommendation) {
        payload.target_item_type_id = Number(targetItemTypeId);
        payload.target_status_id = Number(targetStatusId);
        payload.target_priority_id = targetPriorityId === '' || targetPriorityId == null
          ? null
          : Number(targetPriorityId);
      }
      const result = await api.items.previewWorkspaceMove(item.id, payload);
      if (version !== previewVersion) return;
      preview = result;
      targetItemTypeId = result.target_item_type_id;
      targetStatusId = result.target_status_id;
      targetPriorityId = result.target_priority_id;
    } catch (err) {
      if (version !== previewVersion) return;
      preview = null;
      error = err?.message || t('items.moveWorkspacePreviewError');
    } finally {
      if (version === previewVersion) loadingPreview = false;
    }
  }

  async function changeItemType() {
    targetStatusId = null;
    await loadPreview();
  }

  async function confirmMove() {
    if (!preview || saving) return;
    saving = true;
    error = '';
    try {
      const result = await api.items.moveWorkspace(item.id, {
        destination_workspace_id: Number(destinationWorkspaceId),
        target_item_type_id: Number(targetItemTypeId),
        target_status_id: Number(targetStatusId),
        target_priority_id: targetPriorityId === '' || targetPriorityId == null
          ? null
          : Number(targetPriorityId),
      });
      isOpen = false;
      onMoved?.(result);
    } catch (err) {
      error = err?.message || t('items.moveWorkspaceError');
    } finally {
      saving = false;
    }
  }
</script>

<Modal bind:isOpen maxWidth="max-w-2xl" onSubmit={confirmMove} submitDisabled={!preview || loadingPreview || saving}>
  <ModalHeader
    title={t('items.moveWorkspaceTitle', { title: item?.title || '' })}
    subtitle={t('items.moveWorkspaceSubtitle')}
    onClose={() => (isOpen = false)}
  />

  <div class="flex max-h-[min(68vh,42rem)] flex-col gap-4 overflow-y-auto px-6 pb-5 pt-4">
    {#if error}
      <div
        class="rounded-md bg-[var(--ds-background-danger)] px-3.5 py-2.5 text-sm text-[var(--ds-text-danger)]"
        role="alert"
        data-testid="item-move-workspace-error"
      >{error}</div>
    {/if}

    <BasePicker
      id="item-move-workspace-picker"
      bind:value={destinationWorkspaceId}
      items={workspaces}
      loading={loadingWorkspaces}
      label={t('items.moveWorkspaceDestination')}
      placeholder={t('items.moveWorkspaceDestinationPlaceholder')}
      searchFields={['name', 'key', 'description']}
      getValue={(workspace) => workspace.id}
      getLabel={(workspace) => `${workspace.name} (${workspace.key})`}
      optionTestid={(option) => `item-move-workspace-option-${option.value}`}
      onSelect={selectWorkspace}
    />

    {#if loadingPreview}
      <div
        class="grid min-h-32 place-items-center text-sm text-[var(--ds-text-subtle)]"
        data-testid="item-move-workspace-loading"
      >{t('items.moveWorkspaceLoadingPreview')}</div>
    {:else if preview}
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-3" data-testid="item-move-workspace-mapping-controls">
        <label class="flex flex-col gap-1.5 text-xs font-semibold text-[var(--ds-text-subtle)]">
          <span>{t('items.itemType')}</span>
          <NativeSelect
            options={preview.item_types.map((option) => ({ value: option.id, label: option.name }))}
            dataTestid="item-move-workspace-item-type"
            bind:value={targetItemTypeId}
            onchange={changeItemType}
          />
        </label>
        <label class="flex flex-col gap-1.5 text-xs font-semibold text-[var(--ds-text-subtle)]">
          <span>{t('items.itemStatus')}</span>
          <NativeSelect
            options={preview.statuses.map((option) => ({ value: option.id, label: option.name }))}
            dataTestid="item-move-workspace-status"
            bind:value={targetStatusId}
            onchange={() => loadPreview()}
          />
        </label>
        <label class="flex flex-col gap-1.5 text-xs font-semibold text-[var(--ds-text-subtle)]">
          <span>{t('items.itemPriority')}</span>
          <NativeSelect
            options={[
              { value: null, label: t('items.moveWorkspaceNoPriority') },
              ...preview.priorities.map((option) => ({ value: option.id, label: option.name })),
            ]}
            dataTestid="item-move-workspace-priority"
            bind:value={targetPriorityId}
            onchange={() => loadPreview()}
          />
        </label>
      </div>

      <section class="flex flex-col gap-3" data-testid="item-move-workspace-preview">
        <div class="flex items-start justify-between gap-4">
          <div>
            <h3 class="m-0 text-base font-semibold text-[var(--ds-text)]">{preview.source_key} → {preview.destination_workspace_key}</h3>
            <p class="mb-0 mt-1 text-xs text-[var(--ds-text-subtle)]">{t('items.moveWorkspaceNewKeyHint')}</p>
          </div>
          {#if preview.children_detached > 0}
            <span class="shrink-0 rounded-full bg-[var(--ds-background-warning)] px-2 py-1 text-xs font-semibold text-[var(--ds-text-warning)]">
              {t('items.moveWorkspaceChildrenDetached', { count: preview.children_detached })}
            </span>
          {/if}
        </div>

        <div class="border-t border-[var(--ds-border)]">
          {#each preview.fields as field (field.field)}
            <div class="grid grid-cols-[minmax(6rem,.8fr)_minmax(0,1fr)_auto] items-center gap-2 border-b border-[var(--ds-border)] py-2 text-xs sm:grid-cols-[minmax(7.5rem,.8fr)_minmax(0,1fr)_auto_minmax(0,1fr)_auto]">
              <span class="capitalize text-[var(--ds-text-subtle)]">{field.field.replaceAll('_', ' ')}</span>
              <span class="hidden [overflow-wrap:anywhere] text-[var(--ds-text-subtle)] sm:block">{field.from || '—'}</span>
              <span class="hidden text-[var(--ds-text-subtlest)] sm:block" aria-hidden="true">→</span>
              <span class="[overflow-wrap:anywhere] text-[var(--ds-text)]">{field.to || '—'}</span>
              <span
                class="min-w-[3.25rem] text-right font-semibold capitalize {field.action === 'drop' || field.action === 'detach'
                  ? 'text-[var(--ds-text-danger)]'
                  : field.action === 'partial'
                    ? 'text-[var(--ds-text-warning)]'
                    : 'text-[var(--ds-text-subtle)]'}"
              >{field.action}</span>
            </div>
          {/each}
        </div>

        <p class="m-0 rounded-md bg-[var(--ds-background-warning)] px-3 py-2.5 text-[0.8125rem] leading-[1.45] text-[var(--ds-text-warning)]">
          {t('items.moveWorkspaceDeadKeyWarning', { key: preview.source_key })}
        </p>
      </section>
    {/if}
  </div>

  <DialogFooter
    cancelLabel={t('items.moveWorkspaceCancel')}
    confirmLabel={t('items.moveWorkspaceConfirm')}
    loadingLabel={t('items.moveWorkspaceMoving')}
    confirmTestid="item-move-workspace-confirm"
    cancelTestid="item-move-workspace-cancel"
    confirmDisabled={!preview || loadingPreview}
    loading={saving}
    onCancel={() => (isOpen = false)}
    onConfirm={confirmMove}
  />
</Modal>
