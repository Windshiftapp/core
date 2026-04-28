<script>
  import { onMount } from 'svelte';
  import { successToast, errorToast } from '../../../stores/toasts.svelte.js';
  import Modal from '../../../dialogs/Modal.svelte';
  import Button from '../../../components/Button.svelte';

  let {
    parentId,
    // CRUD-shaped api: { getAll, get, create, update, delete, toggle }
    // — all methods take parentId as the first argument.
    api,
    defaultTriggerType,
    modalTitle = 'New Action',
    namePlaceholder = '',
    flowEditorView,   // snippet({ action, onSave, onCancel })
    logsView,         // snippet({ action, onBack })
    managerView,      // snippet({ actions, loading, handleCreate, handleEdit, handleToggle, handleDelete, handleViewLogs })
    extraUI,          // optional snippet({ reload }) — additional modals etc.
  } = $props();

  let actions = $state([]);
  let loading = $state(true);
  let editingAction = $state(null);
  let viewingLogsAction = $state(null);
  let showCreateModal = $state(false);
  let newActionName = $state('');
  let newActionDescription = $state('');

  onMount(async () => {
    await loadActions();
    loading = false;
  });

  async function loadActions() {
    try {
      actions = (await api.getAll(parentId)) || [];
    } catch (error) {
      console.error('Failed to load actions:', error);
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
      const newAction = await api.create(parentId, {
        name: newActionName.trim(),
        description: newActionDescription.trim(),
        trigger_type: defaultTriggerType,
        is_enabled: false,
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
      const fullAction = await api.get(parentId, action.id);
      editingAction = fullAction;
    } catch (error) {
      console.error('Failed to load action details:', error);
      errorToast('Failed to load action details');
    }
  }

  async function handleToggle(action) {
    try {
      await api.toggle(parentId, action.id);
      await loadActions();
      successToast(action.is_enabled ? 'Action disabled' : 'Action enabled');
    } catch (error) {
      console.error('Failed to toggle action:', error);
      errorToast('Failed to toggle action');
    }
  }

  async function handleDelete(action) {
    try {
      await api.delete(parentId, action.id);
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

  async function handleSaveAction(updatedAction) {
    try {
      await api.update(parentId, updatedAction.id, updatedAction);
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
  {@render flowEditorView({ action: editingAction, onSave: handleSaveAction, onCancel: handleCancelEdit })}
{:else if viewingLogsAction}
  {@render logsView({ action: viewingLogsAction, onBack: handleBackFromLogs })}
{:else}
  {@render managerView({
    actions,
    loading,
    handleCreate,
    handleEdit,
    handleToggle,
    handleDelete,
    handleViewLogs,
  })}
{/if}

<!-- Create Action Modal -->
<Modal
  isOpen={showCreateModal}
  onSubmit={createAction}
  submitDisabled={!newActionName.trim()}
  maxWidth="max-w-md"
  onclose={() => (showCreateModal = false)}
>
  {#snippet children(submitHint)}
    <div class="p-6">
      <h2 class="text-lg font-semibold mb-4 modal-title">{modalTitle}</h2>

      <div class="space-y-4">
        <div>
          <label for="action-name" class="block text-sm font-medium mb-1 modal-label">Name</label>
          <input
            id="action-name"
            type="text"
            class="w-full px-3 py-2 border rounded-md text-sm modal-input"
            bind:value={newActionName}
            placeholder={namePlaceholder}
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
        <Button variant="default" onclick={() => (showCreateModal = false)} keyboardHint="Esc">
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

{#if extraUI}
  {@render extraUI({ reload: loadActions })}
{/if}

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
