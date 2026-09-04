<script>
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';
  import BasePicker from '../../pickers/BasePicker.svelte';
  import { api } from '../../api.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { isSelfOrDescendant } from './pageHierarchy.js';

  /** Page reparenting dialog. It can point the subtree at another accessible
   * workspace; the backend remains the permission and cycle authority. */
  let {
    isOpen = $bindable(false),
    workspaceId,
    page,
    onMoved = null,
  } = $props();

  let candidates = $state([]);
  let sourceTree = $state([]);
  let workspaces = $state([]);
  let loading = $state(false);
  let loadingWorkspaces = $state(false);
  let saving = $state(false);
  let error = $state('');

  /** Root and no selection both bind as null, so selectionMade disambiguates. */
  let pickedWorkspaceId = $state(null);
  let pickedParentId = $state(null);
  let selectionMade = $state(false);

  $effect(() => {
    if (isOpen && page) {
      pickedWorkspaceId = workspaceId;
      loadDialogData();
    }
    if (!isOpen) {
      pickedWorkspaceId = null;
      pickedParentId = null;
      selectionMade = false;
      error = '';
    }
  });

  async function loadDialogData() {
    loadingWorkspaces = true;
    const workspaceRequest = api.workspaces.getAll();
    await Promise.all([loadCandidates(workspaceId, true), workspaceRequest.then((items) => {
      workspaces = (items || []).filter((item) => item.active !== false);
    }).catch((err) => {
      error = err?.message || t('pages.errorLoadWorkspaces');
      workspaces = [];
    }).finally(() => {
      loadingWorkspaces = false;
    })]);
  }

  async function loadCandidates(destinationWorkspaceId, rememberSource = false) {
    loading = true;
    error = '';
    try {
      const all = (await api.pages.getAll(destinationWorkspaceId)) || [];
      if (rememberSource) sourceTree = all;
      candidates = destinationWorkspaceId === workspaceId
        ? all.filter((candidate) => {
            if (isSelfOrDescendant(candidate, page)) return false;
            if (candidate.id === page.parent_id) return false;
            return true;
          })
        : all;
    } catch (err) {
      error = err?.message || t('pages.errorLoadTree');
      candidates = [];
    } finally {
      loading = false;
    }
  }

  async function onWorkspacePick(item) {
    if (!item) return;
    pickedWorkspaceId = item.id;
    pickedParentId = null;
    selectionMade = false;
    await loadCandidates(item.id);
  }

  function onPick(item) {
    // BasePicker fires onSelect with the chosen item, or null when the
    // user picks the "Workspace root" (showUnassigned) option.
    selectionMade = true;
    pickedParentId = item ? item.id : null;
  }

  // Show the "Workspace root" option only when moving there would
  // actually change something. If the page is already at the root it'd
  // be a confusing no-op.
  const crossWorkspace = $derived(
    pickedWorkspaceId != null && pickedWorkspaceId !== workspaceId
  );
  const rootAvailable = $derived(crossWorkspace || page?.parent_id != null);
  const subtreeCount = $derived(
    sourceTree.filter((candidate) => isSelfOrDescendant(candidate, page)).length || 1
  );

  async function confirmMove() {
    if (!selectionMade || saving) return;
    saving = true;
    error = '';
    try {
      const moved = await api.pages.movePage(workspaceId, page.id, pickedParentId, {
        destinationWorkspaceId: pickedWorkspaceId,
      });
      isOpen = false;
      onMoved?.(moved);
    } catch (err) {
      error = err?.message || t('pages.errorMove');
    } finally {
      saving = false;
    }
  }
</script>

<Modal bind:isOpen maxWidth="max-w-lg" onSubmit={confirmMove} submitDisabled={!selectionMade || saving}>
  <ModalHeader
    title={t('pages.moveTitle', { title: page?.title || '' })}
    subtitle={t('pages.moveSubtitle')}
    onClose={() => (isOpen = false)}
  />
  <div class="dialog">
    {#if error}
      <div class="error" role="alert">{error}</div>
    {/if}

    <BasePicker
      id="page-move-workspace-picker"
      bind:value={pickedWorkspaceId}
      items={workspaces}
      loading={loadingWorkspaces}
      label={t('pages.moveWorkspaceLabel')}
      placeholder={t('pages.moveWorkspacePlaceholder')}
      searchFields={['name', 'key', 'description']}
      getValue={(item) => item.id}
      getLabel={(item) => item.name}
      optionTestid={(option) => `page-move-workspace-option-${option.value}`}
      onSelect={onWorkspacePick}
    />

    <BasePicker
      id="page-move-picker"
      bind:value={pickedParentId}
      items={candidates}
      {loading}
      label={t('pages.moveParentLabel')}
      placeholder={t('pages.moveSearchPlaceholder')}
      showUnassigned={rootAvailable}
      unassignedLabel={t('pages.moveRoot')}
      searchFields={['title', 'path']}
      getValue={(p) => p.id}
      getLabel={(p) => p.title}
      optionTestid={(option) => option.isUnassigned
        ? 'page-move-parent-root'
        : `page-move-parent-option-${option.value}`}
      onSelect={onPick}
    />

    {#if crossWorkspace}
      <div class="move-policy" data-testid="page-move-cross-workspace-preview">
        <strong>{t('pages.moveCrossWorkspaceSummary', { count: subtreeCount })}</strong>
        <p>{t('pages.moveCrossWorkspacePolicy')}</p>
      </div>
    {/if}
  </div>
  <DialogFooter
    cancelLabel={t('pages.moveCancel')}
    confirmLabel={t('pages.moveButton')}
    confirmTestid="page-move-confirm"
    cancelTestid="page-move-cancel"
    confirmDisabled={!selectionMade}
    loading={saving}
    showKeyboardHint={true}
    onCancel={() => (isOpen = false)}
    onConfirm={confirmMove}
  />
</Modal>

<style>
  .dialog {
    padding: 1rem 1.25rem;
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  .error {
    padding: 0.625rem 0.875rem;
    background: var(--ds-status-danger-bg, #fef2f2);
    color: var(--ds-text-danger, #b91c1c);
    border-radius: 0.25rem;
    font-size: 0.875rem;
  }

  .move-policy {
    padding: 0.75rem 0.875rem;
    background: var(--ds-background-neutral, #f4f5f7);
    border: 1px solid var(--ds-border, #dfe1e6);
    border-radius: 0.5rem;
    color: var(--ds-text, #172b4d);
    font-size: 0.8125rem;
    line-height: 1.45;
  }

  .move-policy strong {
    display: block;
    font-weight: 600;
  }

  .move-policy p {
    margin: 0.25rem 0 0;
    color: var(--ds-text-subtle, #44546f);
  }
</style>
