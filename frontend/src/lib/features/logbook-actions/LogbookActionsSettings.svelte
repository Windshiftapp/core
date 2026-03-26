<script>
  import { onMount } from 'svelte';
  import { logbookActions } from '../../api/logbookActions.js';
  import { successToast, errorToast } from '../../stores/toasts.svelte.js';
  import LogbookActionsManager from './LogbookActionsManager.svelte';
  import LogbookActionFlowEditor from './LogbookActionFlowEditor.svelte';
  import LogbookActionLogs from './LogbookActionLogs.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import Button from '../../components/Button.svelte';
  import { DocumentPicker } from '../../pickers';

  let { bucketId } = $props();

  let actions = $state([]);
  let loading = $state(true);
  let editingAction = $state(null);
  let viewingLogsAction = $state(null);
  let showCreateModal = $state(false);

  let newActionName = $state('');
  let newActionDescription = $state('');

  let executingAction = $state(null);
  let selectedDocumentId = $state(null);

  onMount(async () => {
    await loadActions();
    loading = false;
  });

  async function loadActions() {
    try {
      actions = (await logbookActions.getAll(bucketId)) || [];
    } catch (error) {
      console.error('Failed to load logbook actions:', error);
      errorToast('Failed to load actions');
      actions = [];
    }
  }

  function handleCreate() {
    showCreateModal = true;
    newActionName = '';
    newActionDescription = '';
  }

  async function createAction() {
    if (!newActionName.trim()) {
      errorToast('Name is required');
      return;
    }

    try {
      const newAction = await logbookActions.create(bucketId, {
        name: newActionName.trim(),
        description: newActionDescription.trim(),
        trigger_type: 'document_classified',
        is_enabled: false
      });

      showCreateModal = false;
      editingAction = newAction;
      successToast('Action created');
      await loadActions();
    } catch (error) {
      console.error('Failed to create action:', error);
      errorToast('Failed to create action');
    }
  }

  async function handleEdit(action) {
    try {
      const fullAction = await logbookActions.get(bucketId, action.id);
      editingAction = fullAction;
    } catch (error) {
      console.error('Failed to load action details:', error);
      errorToast('Failed to load action details');
    }
  }

  async function handleToggle(action) {
    try {
      await logbookActions.toggle(bucketId, action.id);
      await loadActions();
      successToast(action.is_enabled ? 'Action disabled' : 'Action enabled');
    } catch (error) {
      console.error('Failed to toggle action:', error);
      errorToast('Failed to toggle action');
    }
  }

  async function handleDelete(action) {
    try {
      await logbookActions.delete(bucketId, action.id);
      await loadActions();
      successToast('Action deleted');
    } catch (error) {
      console.error('Failed to delete action:', error);
      errorToast('Failed to delete action');
    }
  }

  function handleViewLogs(action) {
    viewingLogsAction = action;
  }

  function handleBackFromLogs() {
    viewingLogsAction = null;
  }

  function handleExecute(action) {
    executingAction = action;
    selectedDocumentId = null;
  }

  async function confirmExecute() {
    if (!executingAction || !selectedDocumentId) return;
    try {
      await logbookActions.execute(bucketId, executingAction.id, selectedDocumentId);
      successToast('Action executed (manual trigger)');
    } catch (error) {
      console.error('Failed to execute action:', error);
      errorToast('Failed to execute action');
    } finally {
      executingAction = null;
      selectedDocumentId = null;
    }
  }

  async function handleSaveAction(updatedAction) {
    try {
      await logbookActions.update(bucketId, updatedAction.id, updatedAction);
      editingAction = null;
      await loadActions();
      successToast('Action saved');
    } catch (error) {
      console.error('Failed to save action:', error);
      errorToast('Failed to save action');
      throw error;
    }
  }

  function handleCancelEdit() {
    editingAction = null;
  }
</script>

{#if editingAction}
  <div class="h-full">
    <LogbookActionFlowEditor
      action={editingAction}
      onSave={handleSaveAction}
      onCancel={handleCancelEdit}
    />
  </div>
{:else if viewingLogsAction}
  <div class="p-6">
    <LogbookActionLogs
      {bucketId}
      action={viewingLogsAction}
      onBack={handleBackFromLogs}
    />
  </div>
{:else}
  <div class="p-6">
    <LogbookActionsManager
      {actions}
      {loading}
      oncreate={handleCreate}
      onedit={handleEdit}
      ontoggle={handleToggle}
      ondelete={handleDelete}
      onviewlogs={handleViewLogs}
      onexecute={handleExecute}
    />
  </div>
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
    <h2 class="text-lg font-semibold mb-4 modal-title">New Action</h2>

    <div class="space-y-4">
      <div>
        <label for="action-name" class="block text-sm font-medium mb-1 modal-label">Name</label>
        <input
          id="action-name"
          type="text"
          class="w-full px-3 py-2 border rounded-md text-sm modal-input"
          bind:value={newActionName}
          placeholder="e.g. Create ticket from invoice"
        />
      </div>

      <div>
        <label for="action-description" class="block text-sm font-medium mb-1 modal-label">Description</label>
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
        Cancel
      </Button>
      <Button
        variant="primary"
        onclick={createAction}
        disabled={!newActionName.trim()}
        keyboardHint={submitHint}
      >
        Create
      </Button>
    </div>
  </div>
  {/snippet}
</Modal>

<!-- Execute Action: Document Picker Modal -->
<Modal
  isOpen={!!executingAction}
  onSubmit={confirmExecute}
  submitDisabled={!selectedDocumentId}
  maxWidth="max-w-md"
  onclose={() => { executingAction = null; selectedDocumentId = null; }}
>
  {#snippet children(submitHint)}
  <div class="p-6">
    <h2 class="text-lg font-semibold mb-1 modal-title">Run Action</h2>
    <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
      Select a document to run <strong>{executingAction?.name}</strong> against.
    </p>

    <div>
      <label for="doc-picker" class="block text-sm font-medium mb-1 modal-label">Document</label>
      <DocumentPicker
        bind:value={selectedDocumentId}
        {bucketId}
        allowClear={false}
      />
    </div>

    <div class="flex justify-end gap-3 mt-6">
      <Button
        variant="default"
        onclick={() => { executingAction = null; selectedDocumentId = null; }}
        keyboardHint="Esc"
      >
        Cancel
      </Button>
      <Button
        variant="primary"
        onclick={confirmExecute}
        disabled={!selectedDocumentId}
        keyboardHint={submitHint}
      >
        Run
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
