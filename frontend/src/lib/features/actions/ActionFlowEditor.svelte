<script>
  import { Pencil, RefreshCw, MessageSquare, Bell, HelpCircle, Database, PlusSquare } from 'lucide-svelte';
  import { toHotkeyString, getShortcutDisplay } from '../../utils/keyboardShortcuts.js';
  import FieldSelector from '../../pickers/FieldSelector.svelte';
  import TriggerNode from './nodes/TriggerNode.svelte';
  import SetFieldNode from './nodes/SetFieldNode.svelte';
  import SetStatusNode from './nodes/SetStatusNode.svelte';
  import AddCommentNode from './nodes/AddCommentNode.svelte';
  import NotifyUserNode from './nodes/NotifyUserNode.svelte';
  import ConditionNode from './nodes/ConditionNode.svelte';
  import UpdateAssetNode from './nodes/UpdateAssetNode.svelte';
  import CreateAssetNode from './nodes/CreateAssetNode.svelte';
  import UpdateAssetConfigPanel from './UpdateAssetConfigPanel.svelte';
  import CreateAssetConfigPanel from './CreateAssetConfigPanel.svelte';
  import PlaceholderReferenceModal from './PlaceholderReferenceModal.svelte';
  import BaseActionFlowEditor from './shared/BaseActionFlowEditor.svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import Checkbox from '../../components/Checkbox.svelte';
  import Select from '../../components/Select.svelte';
  import UserPicker from '../../pickers/UserPicker.svelte';
  import { actionFlowStore } from '../../stores/actionFlowStore.svelte.js';
  import { permissionStore } from '../../stores';

  let {
    action,
    statuses = [],
    onSave,
    onCancel
  } = $props();

  let showPlaceholderModal = $state(false);

  // Actor override: null means the action runs under the triggering user's
  // permissions. Only users with the global action.set_actor permission can
  // change this; others see a read-only display of the current value.
  let actorUserId = $state(action?.actor_user_id ?? null);
  let canSetActor = $derived(permissionStore.hasPermissionKey('action.set_actor'));

  const nodeTypes = {
    trigger: TriggerNode,
    set_field: SetFieldNode,
    set_status: SetStatusNode,
    add_comment: AddCommentNode,
    notify_user: NotifyUserNode,
    condition: ConditionNode,
    update_asset: UpdateAssetNode,
    create_asset: CreateAssetNode
  };

  // Mirror each node's accentColor in the minimap so the overview reflects
  // the canvas colour coding instead of rendering every node the same grey.
  const nodeTypeAccents = {
    trigger: 'amber',
    set_field: 'purple',
    set_status: 'teal',
    add_comment: 'orange',
    notify_user: 'magenta',
    condition: 'yellow',
    update_asset: 'teal',
    create_asset: 'green',
  };

  function minimapNodeColor(node) {
    const accent = nodeTypeAccents[node.type];
    if (!accent) return 'var(--ds-accent-gray-subtle, #94a3b8)';
    return `var(--ds-accent-${accent}-subtle)`;
  }

  function minimapNodeStroke(node) {
    const accent = nodeTypeAccents[node.type];
    if (!accent) return 'var(--ds-border, #64748b)';
    return `var(--ds-accent-${accent})`;
  }

  // Node palette - available node types to drag
  const nodePalette = [
    { type: 'set_field', label: t('actions.nodes.setField'), icon: Pencil },
    { type: 'set_status', label: t('actions.nodes.setStatus'), icon: RefreshCw },
    { type: 'add_comment', label: t('actions.nodes.addComment'), icon: MessageSquare },
    { type: 'notify_user', label: t('actions.nodes.notifyUser'), icon: Bell },
    { type: 'condition', label: t('actions.nodes.condition'), icon: HelpCircle },
    { type: 'update_asset', label: t('actions.nodes.updateAsset'), icon: Database },
    { type: 'create_asset', label: t('actions.nodes.createAsset'), icon: PlusSquare }
  ];

  // Trigger type options
  const triggerTypes = [
    { value: 'status_transition', label: t('actions.trigger.statusTransition') },
    { value: 'item_created', label: t('actions.trigger.itemCreated') },
    { value: 'item_updated', label: t('actions.trigger.itemUpdated') },
    { value: 'item_linked', label: t('actions.trigger.itemLinked') },
    { value: 'manual', label: t('actions.trigger.manual') }
  ];

  async function handleSave(apiData) {
    // Inject actor override before forwarding to the caller. Backend only
    // enforces action.set_actor when the value actually changes vs the stored
    // action, so passing through unchanged is a no-op.
    apiData.actor_user_id = actorUserId;
    await onSave(apiData);
  }

  // Mapping from FieldSelector IDs to backend column names
  const fieldIdToBackendName = {
    title: 'title',
    description: 'description',
    status: 'status_id',
    priority: 'priority_id',
    assignee: 'assignee_id',
    reporter: 'creator_id',
    milestone: 'milestone_id',
    iteration: 'iteration_id',
    dueDate: 'due_date',
    startDate: 'start_date',
    storyPoints: 'story_points',
    parent: 'parent_id',
    project: 'project_id',
    itemType: 'item_type_id'
  };

  const backendNameToFieldId = Object.fromEntries(
    Object.entries(fieldIdToBackendName).map(([k, v]) => [v, k])
  );

  function getFieldSelectorValue(config) {
    const backendName = config?.field_name;
    if (!backendName) return null;
    if (backendName.startsWith('cf_')) {
      return { id: backendName, name: backendName.slice(3) };
    }
    const fieldId = backendNameToFieldId[backendName];
    return fieldId ? { id: fieldId, name: fieldId } : { id: backendName, name: backendName };
  }
</script>

<BaseActionFlowEditor
  {action}
  flowStore={actionFlowStore}
  initArgs={[statuses]}
  {nodeTypes}
  {nodePalette}
  {triggerTypes}
  sidebarTitle={t('actions.title')}
  addNodesLabel={t('actions.addNodes')}
  tipsLabel={t('actions.tips')}
  tips={[
    t('actions.tipDragToConnect'),
    t('actions.tipClickToEdit'),
    t('actions.tipConditionBranches'),
  ]}
  nodeConfigLabel={t('actions.nodeConfig')}
  newActionLabel={t('actions.newAction')}
  cancelLabel={t('common.cancel')}
  saveLabel={t('common.save')}
  switchToVerticalLabel={t('actions.switchToVertical')}
  switchToHorizontalLabel={t('actions.switchToHorizontal')}
  saveErrorMessage={t('actions.failedToSave')}
  cancelButtonProps={{
    keyboardHint: getShortcutDisplay('actions', 'cancel'),
    hotkeyConfig: { key: toHotkeyString('actions', 'cancel'), guard: () => !actionFlowStore.saving },
  }}
  saveButtonProps={{
    keyboardHint: getShortcutDisplay('actions', 'save'),
    hotkeyConfig: { key: toHotkeyString('actions', 'save'), guard: () => !actionFlowStore.saving },
  }}
  minimapClass="action-minimap"
  minimapNodeColor={minimapNodeColor}
  minimapNodeStrokeColor={minimapNodeStroke}
  minimapNodeStrokeWidth={2}
  minimapNodeBorderRadius={3}
  minimapMaskColor="var(--action-minimap-mask, rgba(15, 23, 42, 0.55))"
  onSave={handleSave}
  {onCancel}
>
  {#snippet sidebarTop()}
    <h3 class="text-sm font-medium sidebar-title mb-2">{t('actions.runAs')}</h3>
    {#if canSetActor}
      <UserPicker
        bind:value={actorUserId}
        placeholder={t('actions.runAsTriggerUser')}
        showUnassigned={true}
        unassignedLabel={t('actions.runAsTriggerUser')}
        onSelect={(user) => { actorUserId = user?.id ?? null; }}
      />
      <p class="mt-2 text-xs sidebar-hints">{t('actions.runAsHint')}</p>
    {:else if action?.actor_user_id && action?.actor_name}
      <div class="text-xs sidebar-subtitle">
        <div class="font-medium" style="color: var(--ds-text);">{action.actor_name}</div>
        <div class="mt-1">{t('actions.runAsReadonlyHint')}</div>
      </div>
    {:else}
      <p class="text-xs sidebar-hints">{t('actions.runAsTriggerUser')}</p>
    {/if}
  {/snippet}

  {#snippet triggerConfig(selectedNode, store)}
    <div>
      <label for="config-trigger-type" class="block text-xs font-medium mb-1">{t('actions.config.triggerType')}</label>
      <Select
        id="config-trigger-type"
        options={triggerTypes}
        value={selectedNode.data?.triggerType || action?.trigger_type || 'status_transition'}
        onchange={(v) => {
          store.updateNodeData(selectedNode.id, { triggerType: v });
          store.updateTriggerType(v);
        }}
        size="small"
      />
    </div>
    {#if (selectedNode.data?.triggerType || action?.trigger_type) === 'status_transition'}
      <div>
        <label for="config-from-status" class="block text-xs font-medium mb-1">{t('actions.config.fromStatus')}</label>
        <Select
          id="config-from-status"
          options={[{ value: '', label: t('actions.config.anyStatus') }, ...statuses.map(s => ({ value: s.id, label: s.name }))]}
          value={selectedNode.data?.config?.from_status_id || ''}
          onchange={(v) => store.updateNodeConfig(selectedNode.id, { from_status_id: v ? parseInt(v) : null })}
          size="small"
        />
      </div>
      <div>
        <label for="config-to-status" class="block text-xs font-medium mb-1">{t('actions.config.toStatus')}</label>
        <Select
          id="config-to-status"
          options={[{ value: '', label: t('actions.config.anyStatus') }, ...statuses.map(s => ({ value: s.id, label: s.name }))]}
          value={selectedNode.data?.config?.to_status_id || ''}
          onchange={(v) => store.updateNodeConfig(selectedNode.id, { to_status_id: v ? parseInt(v) : null })}
          size="small"
        />
      </div>
    {/if}
    {#if (selectedNode.data?.triggerType || action?.trigger_type) === 'item_updated'}
      <div>
        <label class="block text-xs font-medium mb-1">{t('actions.config.triggerField')}</label>
        <FieldSelector
          placeholder={t('actions.config.anyField')}
          selectedField={getFieldSelectorValue(selectedNode.data?.config)}
          onSelect={(field) => {
            const backendName = field.id.startsWith('cf_') ? field.id : (fieldIdToBackendName[field.id] || field.id);
            store.updateNodeConfig(selectedNode.id, { field_name: backendName });
          }}
          onClear={() => store.updateNodeConfig(selectedNode.id, { field_name: '' })}
        />
      </div>
    {/if}
    <div class="pt-4 border-t cascade-option">
      <Checkbox
        checked={selectedNode.data?.config?.respond_to_cascades || false}
        onchange={(checked) => store.updateNodeConfig(selectedNode.id, { respond_to_cascades: checked })}
        label={t('actions.trigger.respondToCascades')}
        hint={t('actions.trigger.respondToCascadesHint')}
        size="small"
      />
    </div>
  {/snippet}

  {#snippet nodeConfig(selectedNode, store, _handleDeleteNode)}
    {#if selectedNode.type === 'set_status'}
      <div>
        <label for="config-target-status" class="block text-xs font-medium mb-1">{t('actions.config.targetStatus')}</label>
        <Select
          id="config-target-status"
          options={[{ value: '', label: t('actions.config.selectStatus') }, ...statuses.map(s => ({ value: s.id, label: s.name }))]}
          value={selectedNode.data?.config?.status_id || ''}
          onchange={(v) => store.updateNodeConfig(selectedNode.id, { status_id: parseInt(v) })}
          size="small"
        />
      </div>
    {:else if selectedNode.type === 'set_field'}
      <div>
        <label for="config-set-field-name" class="block text-xs font-medium mb-1">{t('actions.config.fieldName')}</label>
        <FieldSelector
          selectedField={selectedNode.data?.config?.field_name ? { id: selectedNode.data.config.field_name, name: selectedNode.data.config.field_name } : null}
          onSelect={(field) => store.updateNodeConfig(selectedNode.id, { field_name: field.id })}
          onClear={() => store.updateNodeConfig(selectedNode.id, { field_name: '' })}
        />
      </div>
      <div>
        <div class="flex items-center gap-1 mb-1">
          <label for="config-set-field-value" class="block text-xs font-medium">{t('actions.config.value')}</label>
          <button
            onclick={() => showPlaceholderModal = true}
            class="text-[var(--ds-text-subtlest)] hover:text-[var(--ds-interactive)] transition-colors"
            title={t('actions.placeholders.showReference')}
          >
            <HelpCircle class="w-3.5 h-3.5" />
          </button>
        </div>
        <input
          id="config-set-field-value"
          type="text"
          class="w-full px-3 py-2 border rounded-md text-sm config-input"
          value={selectedNode.data?.config?.value || ''}
          oninput={(e) => store.updateNodeConfig(selectedNode.id, { value: e.target.value })}
          placeholder="{'{{'}item.creator_id{'}}'}"
        />
      </div>
    {:else if selectedNode.type === 'add_comment'}
      <div>
        <div class="flex items-center gap-1 mb-1">
          <label for="config-comment-content" class="block text-xs font-medium">{t('actions.config.commentContent')}</label>
          <button
            onclick={() => showPlaceholderModal = true}
            class="text-[var(--ds-text-subtlest)] hover:text-[var(--ds-interactive)] transition-colors"
            title={t('actions.placeholders.showReference')}
          >
            <HelpCircle class="w-3.5 h-3.5" />
          </button>
        </div>
        <textarea
          id="config-comment-content"
          class="w-full px-3 py-2 border rounded-md text-sm config-input"
          rows="4"
          value={selectedNode.data?.config?.content || ''}
          oninput={(e) => store.updateNodeConfig(selectedNode.id, { content: e.target.value })}
          placeholder={t('actions.config.commentPlaceholder')}
        ></textarea>
      </div>
      <Checkbox
        checked={selectedNode.data?.config?.is_private || false}
        onchange={(checked) => store.updateNodeConfig(selectedNode.id, { is_private: checked })}
        label={t('actions.config.privateComment')}
        size="small"
      />
    {:else if selectedNode.type === 'condition'}
      <div>
        <label for="config-condition-field" class="block text-xs font-medium mb-1">{t('actions.config.fieldToCheck')}</label>
        <FieldSelector
          selectedField={selectedNode.data?.config?.field_name ? { id: selectedNode.data.config.field_name, name: selectedNode.data.config.field_name } : null}
          onSelect={(field) => store.updateNodeConfig(selectedNode.id, { field_name: field.id })}
          onClear={() => store.updateNodeConfig(selectedNode.id, { field_name: '' })}
        />
      </div>
      <div>
        <label for="config-condition-operator" class="block text-xs font-medium mb-1">{t('actions.config.operator')}</label>
        <Select
          id="config-condition-operator"
          options={[
            { value: 'eq', label: t('actions.operators.equals') },
            { value: 'ne', label: t('actions.operators.notEquals') },
            { value: 'contains', label: t('actions.operators.contains') },
            { value: 'gt', label: t('actions.operators.greaterThan') },
            { value: 'lt', label: t('actions.operators.lessThan') },
            { value: 'is_empty', label: t('actions.operators.isEmpty') },
            { value: 'is_not_empty', label: t('actions.operators.isNotEmpty') }
          ]}
          value={selectedNode.data?.config?.operator || 'eq'}
          onchange={(v) => store.updateNodeConfig(selectedNode.id, { operator: v })}
          size="small"
        />
      </div>
      <div>
        <label for="config-condition-value" class="block text-xs font-medium mb-1">{t('actions.config.compareValue')}</label>
        <input
          id="config-condition-value"
          type="text"
          class="w-full px-3 py-2 border rounded-md text-sm config-input"
          value={selectedNode.data?.config?.value || ''}
          oninput={(e) => store.updateNodeConfig(selectedNode.id, { value: e.target.value })}
        />
      </div>
    {:else if selectedNode.type === 'notify_user'}
      <div>
        <label for="config-recipient-type" class="block text-xs font-medium mb-1">{t('actions.config.recipientType')}</label>
        <Select
          id="config-recipient-type"
          options={[
            { value: 'assignee', label: t('actions.recipients.assignee') },
            { value: 'creator', label: t('actions.recipients.creator') },
            { value: 'specific', label: t('actions.recipients.specific') }
          ]}
          value={selectedNode.data?.config?.recipient_type || 'assignee'}
          onchange={(v) => store.updateNodeConfig(selectedNode.id, { recipient_type: v })}
          size="small"
        />
      </div>
      <div>
        <div class="flex items-center gap-1 mb-1">
          <label for="config-notify-message" class="block text-xs font-medium">{t('actions.config.notifyMessage')}</label>
          <button
            onclick={() => showPlaceholderModal = true}
            class="text-[var(--ds-text-subtlest)] hover:text-[var(--ds-interactive)] transition-colors"
            title={t('actions.placeholders.showReference')}
          >
            <HelpCircle class="w-3.5 h-3.5" />
          </button>
        </div>
        <textarea
          id="config-notify-message"
          class="w-full px-3 py-2 border rounded-md text-sm config-input"
          rows="4"
          value={selectedNode.data?.config?.message || ''}
          oninput={(e) => store.updateNodeConfig(selectedNode.id, { message: e.target.value })}
          placeholder={t('actions.config.notifyPlaceholder')}
        ></textarea>
      </div>
      <Checkbox
        checked={selectedNode.data?.config?.include_link ?? true}
        onchange={(checked) => store.updateNodeConfig(selectedNode.id, { include_link: checked })}
        label={t('actions.config.includeLink')}
        size="small"
      />
    {:else if selectedNode.type === 'update_asset'}
      <UpdateAssetConfigPanel {selectedNode} bind:showPlaceholderModal />
    {:else if selectedNode.type === 'create_asset'}
      <CreateAssetConfigPanel {selectedNode} bind:showPlaceholderModal />
    {/if}
  {/snippet}
</BaseActionFlowEditor>

{#if showPlaceholderModal}
  <PlaceholderReferenceModal onclose={() => showPlaceholderModal = false} />
{/if}

<style>
  :global(.action-minimap) {
    background-color: var(--ds-surface-raised) !important;
    border: 1px solid var(--ds-border) !important;
    border-radius: 8px !important;
    box-shadow: var(--shadow-md) !important;
    overflow: hidden;
  }

  :global(.action-minimap .svelte-flow__minimap-mask) {
    fill: var(--action-minimap-mask, rgba(15, 23, 42, 0.55));
  }

  .cascade-option {
    border-color: var(--ds-border);
  }
</style>
