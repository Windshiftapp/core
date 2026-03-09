<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { successToast, errorToast } from '../../stores/toasts.svelte.js';
  import { statusCategoriesStore } from '../../stores/statusCategories.svelte.js';
  import { workspacePermissions } from '../../stores';
  import ActionsManager from './ActionsManager.svelte';
  import ActionFlowEditor from './ActionFlowEditor.svelte';
  import ActionLogs from './ActionLogs.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import Button from '../../components/Button.svelte';
  import { ShieldAlert } from 'lucide-svelte';

  export let workspaceId;

  // Permission check
  $: canManageActions = workspacePermissions.canManageActions(workspaceId);

  let actions = [];
  let statuses = [];
  let loading = true;
  let editingAction = null;
  let viewingLogsAction = null;
  let showCreateModal = false;

  // New action form data
  let newActionName = '';
  let newActionDescription = '';

  onMount(async () => {
    await Promise.all([loadActions(), loadStatuses(), statusCategoriesStore.init()]);
    loading = false;
  });

  async function loadActions() {
    try {
      actions = await api.get(`/workspaces/${workspaceId}/actions`) || [];
    } catch (error) {
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
      const newAction = await api.post(`/workspaces/${workspaceId}/actions`, {
        name: newActionName.trim(),
        description: newActionDescription.trim(),
        trigger_type: 'status_transition',
        is_enabled: false
      });

      showCreateModal = false;
      editingAction = newAction;
      successToast(t('common.created'));
      await loadActions();
    } catch (error) {
      console.error('Failed to create action:', error);
      errorToast(t('errors.failedToCreate'));
    }
  }

  async function handleEdit(event) {
    const action = event.detail;
    try {
      // Fetch full action with nodes and edges
      const fullAction = await api.get(`/workspaces/${workspaceId}/actions/${action.id}`);
      editingAction = fullAction;
    } catch (error) {
      console.error('Failed to load action details:', error);
      errorToast(t('errors.failedToLoad'));
    }
  }

  async function handleToggle(event) {
    const action = event.detail;
    try {
      await api.post(`/workspaces/${workspaceId}/actions/${action.id}/toggle`);
      await loadActions();
      successToast(action.is_enabled ? t('actions.disabled') : t('actions.enabled'));
    } catch (error) {
      console.error('Failed to toggle action:', error);
      errorToast(t('errors.failedToUpdate'));
    }
  }

  async function handleDelete(event) {
    const action = event.detail;
    try {
      await api.delete(`/workspaces/${workspaceId}/actions/${action.id}`);
      await loadActions();
      successToast(t('common.deleted'));
    } catch (error) {
      console.error('Failed to delete action:', error);
      errorToast(t('errors.failedToDelete'));
    }
  }

  function handleViewLogs(event) {
    viewingLogsAction = event.detail;
  }

  function handleBackFromLogs() {
    viewingLogsAction = null;
  }

  async function handleSaveAction(updatedAction) {
    try {
      await api.put(`/workspaces/${workspaceId}/actions/${updatedAction.id}`, updatedAction);
      editingAction = null;
      await loadActions();
      successToast(t('common.saved'));
    } catch (error) {
      console.error('Failed to save action:', error);
      errorToast(t('errors.failedToSave'));
      throw error;
    }
  }

  function handleCancelEdit() {
    editingAction = null;
  }
</script>

{#if !canManageActions}
  <div class="h-full flex items-center justify-center">
    <div class="text-center p-8 max-w-md">
      <div class="w-16 h-16 mx-auto mb-4 rounded-full flex items-center justify-center" style="background-color: var(--ds-background-danger-subtle);">
        <ShieldAlert class="w-8 h-8" style="color: var(--ds-text-danger);" />
      </div>
      <h2 class="text-xl font-semibold mb-2" style="color: var(--ds-text);">{t('errors.accessDenied')}</h2>
      <p class="text-sm" style="color: var(--ds-text-subtle);">
        {t('errors.noPermission')}
      </p>
    </div>
  </div>
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
    on:create={handleCreate}
    on:edit={handleEdit}
    on:toggle={handleToggle}
    on:delete={handleDelete}
    on:viewLogs={handleViewLogs}
  />
{/if}

<!-- Create Action Modal -->
<Modal
  isOpen={showCreateModal}
  onSubmit={createAction}
  submitDisabled={!newActionName.trim()}
  maxWidth="max-w-md"
  onclose={() => showCreateModal = false}
  let:submitHint
>
  <div class="p-6">
    <h2 class="text-lg font-semibold mb-4 modal-title">{t('actions.create')}</h2>

    <div class="space-y-4">
      <div>
        <label for="action-name" class="block text-sm font-medium mb-1 modal-label">{t('common.name')}</label>
        <input
          id="action-name"
          type="text"
          class="w-full px-3 py-2 border rounded-md text-sm modal-input"
          bind:value={newActionName}
          placeholder={t('actions.newAction')}
        />
      </div>

      <div>
        <label for="action-description" class="block text-sm font-medium mb-1 modal-label">{t('common.description')}</label>
        <textarea
          id="action-description"
          class="w-full px-3 py-2 border rounded-md text-sm modal-input"
          rows="2"
          bind:value={newActionDescription}
        ></textarea>
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
</Modal>

<style>
  .modal-title {
    color: var(--ds-text);
  }

  .modal-label {
    color: var(--ds-text);
  }

  .modal-input {
    background-color: var(--ds-surface);
    border-color: var(--ds-border);
    color: var(--ds-text);
  }

  .modal-input:focus {
    border-color: var(--ds-interactive);
    outline: none;
  }
</style>
