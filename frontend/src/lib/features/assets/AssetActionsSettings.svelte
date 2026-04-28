<script>
  import { assetActions } from '../../api/assetActions.js';
  import AssetActionsManager from './AssetActionsManager.svelte';
  import AssetActionFlowEditor from './AssetActionFlowEditor.svelte';
  import AssetActionLogs from './AssetActionLogs.svelte';
  import ActionsSettingsBase from '../actions/shared/ActionsSettingsBase.svelte';

  let { assetSetId } = $props();
</script>

<ActionsSettingsBase
  parentId={assetSetId}
  api={assetActions}
  defaultTriggerType="asset_created"
  modalTitle="New Asset Action"
  namePlaceholder="e.g. Create ticket when asset added"
>
  {#snippet flowEditorView({ action, onSave, onCancel })}
    <div class="h-full">
      <AssetActionFlowEditor {action} {onSave} {onCancel} />
    </div>
  {/snippet}

  {#snippet logsView({ action, onBack })}
    <div class="p-6">
      <AssetActionLogs {assetSetId} {action} {onBack} />
    </div>
  {/snippet}

  {#snippet managerView({ actions, loading, handleCreate, handleEdit, handleToggle, handleDelete, handleViewLogs })}
    <div>
      <AssetActionsManager
        {actions}
        {loading}
        oncreate={handleCreate}
        onedit={handleEdit}
        ontoggle={handleToggle}
        ondelete={handleDelete}
        onviewlogs={handleViewLogs}
      />
    </div>
  {/snippet}
</ActionsSettingsBase>
