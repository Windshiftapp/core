<script>
  import { logbookActions } from '../../api/logbookActions.js';
  import { successToast, errorToast } from '../../stores/toasts.svelte.js';
  import LogbookActionsManager from './LogbookActionsManager.svelte';
  import LogbookActionFlowEditor from './LogbookActionFlowEditor.svelte';
  import LogbookActionLogs from './LogbookActionLogs.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import Button from '../../components/Button.svelte';
  import { DocumentPicker } from '../../pickers';
  import ActionsSettingsBase from '../actions/shared/ActionsSettingsBase.svelte';

  let { bucketId } = $props();

  // Logbook-only flow: pick a document and run an action against it manually.
  let executingAction = $state(null);
  let selectedDocumentId = $state(null);

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
</script>

<ActionsSettingsBase
  parentId={bucketId}
  api={logbookActions}
  defaultTriggerType="document_classified"
  modalTitle="New Action"
  namePlaceholder="e.g. Create ticket from invoice"
>
  {#snippet flowEditorView({ action, onSave, onCancel })}
    <div class="h-full">
      <LogbookActionFlowEditor {action} {onSave} {onCancel} />
    </div>
  {/snippet}

  {#snippet logsView({ action, onBack })}
    <div class="p-6">
      <LogbookActionLogs {bucketId} {action} {onBack} />
    </div>
  {/snippet}

  {#snippet managerView({ actions, loading, handleCreate, handleEdit, handleToggle, handleDelete, handleViewLogs })}
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
  {/snippet}

  {#snippet extraUI()}
    <!-- Execute Action: Document Picker Modal -->
    <Modal
      isOpen={!!executingAction}
      onSubmit={confirmExecute}
      submitDisabled={!selectedDocumentId}
      maxWidth="max-w-md"
      onclose={() => {
        executingAction = null;
        selectedDocumentId = null;
      }}
    >
      {#snippet children(submitHint)}
        <div class="p-6">
          <h2 class="text-lg font-semibold mb-1 modal-title">Run Action</h2>
          <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
            Select a document to run <strong>{executingAction?.name}</strong> against.
          </p>

          <div>
            <label for="doc-picker" class="block text-sm font-medium mb-1 modal-label">Document</label>
            <DocumentPicker bind:value={selectedDocumentId} {bucketId} allowClear={false} />
          </div>

          <div class="flex justify-end gap-3 mt-6">
            <Button
              variant="default"
              onclick={() => {
                executingAction = null;
                selectedDocumentId = null;
              }}
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
  {/snippet}
</ActionsSettingsBase>

<style>
  .modal-title {
    color: var(--ds-text);
  }

  .modal-label {
    color: var(--ds-text);
  }
</style>
