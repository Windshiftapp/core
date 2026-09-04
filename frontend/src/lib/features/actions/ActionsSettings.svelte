<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { successToast, errorToast } from '../../stores/toasts.svelte.js';
  import { statusCategoriesStore } from '../../stores/statusCategories.svelte.js';
  import { workspacePermissions } from '../../stores';
  import { navigate } from '../../router.js';
  import ActionsManager from './ActionsManager.svelte';
  import ActionFlowEditor from './ActionFlowEditor.svelte';
  import ActionLogs from './ActionLogs.svelte';
  import ActionTemplatePicker from './ActionTemplatePicker.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import Button from '../../components/Button.svelte';
  import Input from '../../components/Input.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import UnauthorizedAccess from '../../pages/UnauthorizedAccess.svelte';

  let { workspaceId, actionId = 0 } = $props();

  // Permission check
  let canManageActions = $derived(workspacePermissions.canManageActions(workspaceId));

  let actions = $state([]);
  let statuses = $state([]);
  let loading = $state(true);
  let editingAction = $state(null);
  let viewingLogsAction = $state(null);
  let showCreateModal = $state(false);
  let showTemplatePicker = $state(false);

  // URL is the source of truth for which action is being edited, so the id is
  // recoverable on refresh / share and visible to the AI chat context builder.
  // Sync editingAction from the route param.
  $effect(() => {
    const id = Number(actionId) || 0;
    if (id === 0) {
      editingAction = null;
      return;
    }
    if (editingAction?.id === id) return;
    let cancelled = false;
    api.actions
      .get(workspaceId, id)
      .then((full) => {
        if (!cancelled) editingAction = full;
      })
      .catch((err) => {
        if (cancelled) return;
        console.error('Failed to load action details:', err);
        errorToast(t('errors.failedToLoad'));
        navigate(`/workspaces/${workspaceId}/actions`, { replace: true });
      });
    return () => {
      cancelled = true;
    };
  });

  // New action form data
  let newActionName = $state('');
  let newActionDescription = $state('');

  function handleFromTemplate() {
    showTemplatePicker = true;
  }

  async function handleTemplateApplied(result) {
    showTemplatePicker = false;
    successToast(t('common.created'));
    await loadActions();
    // Open the freshly created action in the editor. The URL effect will fetch
    // and hydrate editingAction once the route updates.
    if (result?.action_id) {
      navigate(`/workspaces/${workspaceId}/actions/${result.action_id}`);
    }
  }

  onMount(async () => {
    await Promise.all([loadActions(), loadStatuses(), statusCategoriesStore.init()]);
    loading = false;
  });

  async function loadActions() {
    try {
      actions = await api.actions.getAll(workspaceId) || [];
    } catch (error) {
      if (error?.name === 'AbortError') return;
      console.error('Failed to load actions:', error);
      errorToast(t('errors.failedToLoad'));
      actions = [];
    }
  }

  async function loadStatuses() {
    try {
      statuses = await api.workspaces.getStatuses(workspaceId) || [];
    } catch (error) {
      console.error('Failed to load statuses:', error);
      statuses = [];
    }
  }

  function handleCreate() {
    showCreateModal = true;
    newActionName = '';
    newActionDescription = '';
  }

  async function createAction() {
    if (!newActionName.trim()) {
      errorToast(t('validation.required', { field: t('common.name') }));
      return;
    }

    try {
      const newAction = await api.actions.create(workspaceId, {
        name: newActionName.trim(),
        description: newActionDescription.trim(),
        trigger_type: 'status_transition',
        is_enabled: false
      });

      showCreateModal = false;
      successToast(t('common.created'));
      await loadActions();
      navigate(`/workspaces/${workspaceId}/actions/${newAction.id}`);
    } catch (error) {
      console.error('Failed to create action:', error);
      errorToast(t('errors.failedToCreate'));
    }
  }

  function handleEdit(action) {
    navigate(`/workspaces/${workspaceId}/actions/${action.id}`);
  }

  async function handleToggle(action) {
    try {
      await api.actions.update(workspaceId, action.id, { is_enabled: !action.is_enabled });
      await loadActions();
      successToast(action.is_enabled ? t('actions.disabled') : t('actions.enabled'));
    } catch (error) {
      console.error('Failed to toggle action:', error);
      errorToast(t('errors.failedToUpdate'));
    }
  }

  async function handleDelete(action) {
    try {
      await api.actions.delete(workspaceId, action.id);
      await loadActions();
      successToast(t('common.deleted'));
    } catch (error) {
      console.error('Failed to delete action:', error);
      errorToast(t('errors.failedToDelete'));
    }
  }

  function handleViewLogs(action) {
    viewingLogsAction = action;
  }

  function handleBackFromLogs() {
    viewingLogsAction = null;
  }

  async function handleSaveAction(updatedAction) {
    try {
      await api.actions.update(workspaceId, updatedAction.id, updatedAction);
      await loadActions();
      successToast(t('common.saved'));
      navigate(`/workspaces/${workspaceId}/actions`);
    } catch (error) {
      console.error('Failed to save action:', error);
      errorToast(t('errors.failedToSave'));
      throw error;
    }
  }

  function handleCancelEdit() {
    navigate(`/workspaces/${workspaceId}/actions`);
  }
</script>

{#if !canManageActions}
  <UnauthorizedAccess requiredPermission="action.manage" showBackButton={false} />
{:else if editingAction}
  <div class="h-full">
    <ActionFlowEditor
      action={editingAction}
      {statuses}
      onSave={handleSaveAction}
      onCancel={handleCancelEdit}
    />
  </div>
{:else if viewingLogsAction}
  <div class="h-full">
    <ActionLogs
      {workspaceId}
      action={viewingLogsAction}
      onBack={handleBackFromLogs}
    />
  </div>
{:else}
  <ActionsManager
    {workspaceId}
    {actions}
    {loading}
    oncreate={handleCreate}
    onfromtemplate={handleFromTemplate}
    onedit={handleEdit}
    ontoggle={handleToggle}
    ondelete={handleDelete}
    onviewlogs={handleViewLogs}
  />
{/if}

{#if showTemplatePicker}
  <ActionTemplatePicker
    {workspaceId}
    onclose={() => (showTemplatePicker = false)}
    onapplied={handleTemplateApplied}
  />
{/if}

<!-- Create Action Modal -->
<Modal
  isOpen={showCreateModal}
  onSubmit={createAction}
  submitDisabled={!newActionName.trim()}
  maxWidth="max-w-md"
  onclose={() => showCreateModal = false}
>
  {#snippet children(submitHint)}
  <div class="p-6">
    <h2 class="text-lg font-semibold mb-4 modal-title">{t('actions.create')}</h2>

    <div class="space-y-4">
      <div>
        <label for="action-name" class="block text-sm font-medium mb-1 modal-label">{t('common.name')}</label>
        <Input
          id="action-name"
          type="text"
          bind:value={newActionName}
          placeholder={t('actions.newAction')}
          size="small"
        />
      </div>

      <div>
        <label for="action-description" class="block text-sm font-medium mb-1 modal-label">{t('common.description')}</label>
        <Textarea
          id="action-description"
          rows={2}
          bind:value={newActionDescription}
          style="background-color: var(--ds-surface);"
          size="small"
        />
      </div>
    </div>

    <div class="flex justify-end gap-3 mt-6">
      <Button
        variant="default"
        onclick={() => showCreateModal = false}
        keyboardHint="Esc"
      >
        {t('common.cancel')}
      </Button>
      <Button
        variant="primary"
        onclick={createAction}
        disabled={!newActionName.trim()}
        keyboardHint={submitHint}
      >
        {t('common.create')}
      </Button>
    </div>
  </div>
  {/snippet}
</Modal>

<style>
  .modal-title {
    color: var(--ds-text);
  }

  .modal-label {
    color: var(--ds-text);
  }

</style>
