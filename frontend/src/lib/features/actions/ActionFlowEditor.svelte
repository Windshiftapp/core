<script>
  import { onMount, untrack } from 'svelte';
  import {
    SvelteFlow,
    Controls,
    MiniMap,
    Background,
    ConnectionMode,
    addEdge
  } from '@xyflow/svelte';
  import '@xyflow/svelte/dist/style.css';
  import { Pencil, RefreshCw, MessageSquare, Bell, HelpCircle, Zap, Database, PlusSquare } from 'lucide-svelte';
  import { toHotkeyString, getShortcutDisplay } from '../../utils/keyboardShortcuts.js';
  import Button from '../../components/Button.svelte';
  import FieldSelector from '../../pickers/FieldSelector.svelte';
  import TriggerNode from './nodes/TriggerNode.svelte';
  import SetFieldNode from './nodes/SetFieldNode.svelte';
  import SetStatusNode from './nodes/SetStatusNode.svelte';
  import AddCommentNode from './nodes/AddCommentNode.svelte';
  import NotifyUserNode from './nodes/NotifyUserNode.svelte';
  import ConditionNode from './nodes/ConditionNode.svelte';
  import UpdateAssetNode from './nodes/UpdateAssetNode.svelte';
  import CreateAssetNode from './nodes/CreateAssetNode.svelte';
  import ActionEdge from './edges/ActionEdge.svelte';
  import UpdateAssetConfigPanel from './UpdateAssetConfigPanel.svelte';
  import CreateAssetConfigPanel from './CreateAssetConfigPanel.svelte';
  import PlaceholderReferenceModal from './PlaceholderReferenceModal.svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import Checkbox from '../../components/Checkbox.svelte';
  import { errorToast } from '../../stores/toasts.svelte.js';
  import { actionFlowStore } from '../../stores/actionFlowStore.svelte.js';

  // Props using Svelte 5 $props()
  let {
    action,
    statuses = [],
    onSave,
    onCancel
  } = $props();

  // Local state for SvelteFlow binding (SvelteFlow requires mutable arrays)
  let nodes = $state([]);
  let edges = $state([]);
  let selectedNodeId = $state(null);
  let saving = $state(false);
  let isReconnecting = $state(false);
  let showPlaceholderModal = $state(false);

  // Track store version to detect config changes
  let lastStoreNodesVersion = $state(0);

  // Sync nodes from store, but preserve local positions (which SvelteFlow manages via drag)
  $effect(() => {
    const storeNodes = actionFlowStore.nodes;
    const currentVersion = storeNodes.length + JSON.stringify(storeNodes.map(n => n.data));

    // Read without creating dependencies to avoid infinite loops
    const lastVersion = untrack(() => lastStoreNodesVersion);
    const localNodes = untrack(() => nodes);

    if (currentVersion !== lastVersion) {
      lastStoreNodesVersion = currentVersion;

      // Merge store nodes with local nodes, preserving local positions
      nodes = storeNodes.map(storeNode => {
        const localNode = localNodes.find(n => n.id === storeNode.id);
        if (localNode) {
          // Keep local position (managed by SvelteFlow drag), update data from store
          return {
            ...storeNode,
            position: localNode.position
          };
        }
        // New node - use store position
        return storeNode;
      });
    }
  });

  $effect(() => {
    edges = actionFlowStore.edges;
  });

  $effect(() => {
    selectedNodeId = actionFlowStore.selectedNodeId;
  });

  $effect(() => {
    saving = actionFlowStore.saving;
  });

  // Computed: get selected node from local nodes array
  let selectedNode = $derived(
    selectedNodeId ? nodes.find(n => n.id === selectedNodeId) : null
  );

  // Node and edge types configuration
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

  const edgeTypes = {
    action: ActionEdge
  };

  // Flow options
  const flowOptions = {
    connectionMode: ConnectionMode.Loose,
    attributionPosition: 'bottom-left',
    defaultViewport: { x: 0, y: 0, zoom: 0.7 },
    fitViewOptions: { maxZoom: 1, padding: 0.1 },
    minZoom: 0.2,
    maxZoom: 1.5,
    defaultEdgeOptions: {
      type: 'action'
    }
  };

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

  onMount(() => {
    actionFlowStore.init(action, statuses);
  });

  function handleConnect(params) {
    const newEdge = actionFlowStore.addEdge(params);
    actionFlowStore.setEdges(addEdge(newEdge, actionFlowStore.edges));
  }

  function handleNodesChange(event) {
    const changes = event.detail;
    changes.forEach(change => {
      if (change.type === 'position' && !change.dragging) {
        // Sync final position to store when drag ends
        const node = nodes.find(n => n.id === change.id);
        if (node?.position) {
          actionFlowStore.updateNodePosition(change.id, node.position);
        }
      }
    });
  }

  function handleEdgesChange(event) {
    const changes = event.detail;
    const edgesToRemove = changes
      .filter(c => c.type === 'remove')
      .map(c => c.id);

    if (edgesToRemove.length > 0) {
      actionFlowStore.removeEdges(edgesToRemove);
    }
  }

  function handleReconnectStart() {
    isReconnecting = true;
  }

  function handleReconnectEnd() {
    isReconnecting = false;
  }

  function handleReconnect(oldEdge, newConnection) {
    // Update the edge with new connection info
    actionFlowStore.updateEdge(oldEdge.id, {
      source: newConnection.source,
      target: newConnection.target,
      sourceHandle: newConnection.sourceHandle,
      targetHandle: newConnection.targetHandle
    });
  }

  function isValidConnection(connection) {
    // Allow reconnections
    if (isReconnecting) return true;

    // Allow new connections from source to target
    // Prevent self-connections
    if (connection.source === connection.target) return false;

    // Prevent connecting to trigger node (it has no input)
    const targetNode = nodes.find(n => n.id === connection.target);
    if (targetNode?.type === 'trigger') return false;

    return true;
  }

  function handleNodeClick(event) {
    const node = event.detail?.node || event.node;
    if (node) {
      actionFlowStore.selectNode(node.id);
    }
  }

  function handleAddNode(nodeType) {
    const newNode = actionFlowStore.addNode(nodeType);
    actionFlowStore.selectNode(newNode.id);
  }

  function handleClearSelection() {
    actionFlowStore.clearSelection();
  }

  function handleTriggerTypeChange(e) {
    const value = e.target.value;
    actionFlowStore.updateNodeData(selectedNode.id, { triggerType: value });
    actionFlowStore.updateTriggerType(value);
  }

  function handleFromStatusChange(e) {
    actionFlowStore.updateNodeConfig(selectedNode.id, {
      from_status_id: e.target.value ? parseInt(e.target.value) : null
    });
  }

  function handleToStatusChange(e) {
    actionFlowStore.updateNodeConfig(selectedNode.id, {
      to_status_id: e.target.value ? parseInt(e.target.value) : null
    });
  }

  function handleRespondToCascadesChange(e) {
    actionFlowStore.updateNodeConfig(selectedNode.id, {
      respond_to_cascades: e.target.checked
    });
  }

  function handleTargetStatusChange(e) {
    actionFlowStore.updateNodeConfig(selectedNode.id, {
      status_id: parseInt(e.target.value)
    });
  }

  function handleFieldNameChange(e) {
    actionFlowStore.updateNodeConfig(selectedNode.id, {
      field_name: e.target.value
    });
  }

  function handleFieldValueChange(e) {
    actionFlowStore.updateNodeConfig(selectedNode.id, {
      value: e.target.value
    });
  }

  function handleCommentContentChange(e) {
    actionFlowStore.updateNodeConfig(selectedNode.id, {
      content: e.target.value
    });
  }

  function handlePrivateChange(e) {
    actionFlowStore.updateNodeConfig(selectedNode.id, {
      is_private: e.target.checked
    });
  }

  function handleConditionFieldSelect(e) {
    actionFlowStore.updateNodeConfig(selectedNode.id, {
      field_name: e.detail.id
    });
  }

  function handleConditionFieldClear() {
    actionFlowStore.updateNodeConfig(selectedNode.id, {
      field_name: ''
    });
  }

  function handleOperatorChange(e) {
    actionFlowStore.updateNodeConfig(selectedNode.id, {
      operator: e.target.value
    });
  }

  function handleConditionValueChange(e) {
    actionFlowStore.updateNodeConfig(selectedNode.id, {
      value: e.target.value
    });
  }

  function handleRecipientTypeChange(e) {
    actionFlowStore.updateNodeConfig(selectedNode.id, {
      recipient_type: e.target.value
    });
  }

  function handleNotifyMessageChange(e) {
    actionFlowStore.updateNodeConfig(selectedNode.id, {
      message: e.target.value
    });
  }

  function handleIncludeLinkChange(e) {
    actionFlowStore.updateNodeConfig(selectedNode.id, {
      include_link: e.target.checked
    });
  }

  async function handleSave() {
    if (!action) return;

    try {
      actionFlowStore.setSaving(true);

      // Sync current positions from local nodes to store before saving
      nodes.forEach(node => {
        actionFlowStore.updateNodePosition(node.id, node.position);
      });

      const actionData = actionFlowStore.toApiFormat(action);
      await onSave(actionData);
    } catch (error) {
      console.error('Failed to save action:', error);
      errorToast(error.message || String(error), t('actions.failedToSave'));
    } finally {
      actionFlowStore.setSaving(false);
    }
  }
</script>

<div class="flex h-full action-flow-editor">
  <!-- Node Palette -->
  <div class="w-64 sidebar border-r flex flex-col py-4 overflow-y-auto flex-shrink-0">
    <!-- Actions Header -->
    <div class="px-4 mb-4 pb-4 border-b" style="border-color: var(--ds-border);">
      <div class="flex items-center gap-3">
        <div class="flex items-center justify-center w-10 h-10 flex-shrink-0">
          <div class="w-8 h-8 rounded-md flex items-center justify-center bg-amber-500">
            <Zap size={18} color="white" />
          </div>
        </div>
        <span class="font-medium text-sm" style="color: var(--ds-text);">{t('actions.title')}</span>
      </div>
    </div>

    <div class="px-4">
      <h3 class="text-sm font-medium sidebar-title mb-3">{t('actions.addNodes')}</h3>
      <div class="space-y-2">
        {#each nodePalette as item}
          <button
            class="w-full px-3 py-2 text-left rounded-lg text-sm font-medium flex items-center gap-2 node-palette-item cursor-pointer"
            onclick={() => handleAddNode(item.type)}
          >
            <svelte:component this={item.icon} class="w-4 h-4 flex-shrink-0" />
            <span>{item.label}</span>
          </button>
        {/each}
      </div>

      <div class="mt-6 pt-4 border-t">
        <h4 class="text-xs font-medium sidebar-subtitle mb-2">{t('actions.tips')}</h4>
        <ul class="text-xs space-y-1 sidebar-hints">
          <li>{t('actions.tipDragToConnect')}</li>
          <li>{t('actions.tipClickToEdit')}</li>
          <li>{t('actions.tipConditionBranches')}</li>
        </ul>
      </div>
    </div>
  </div>

  <!-- Svelte Flow Canvas -->
  <div class="flex-1 relative">
    <SvelteFlow
      bind:nodes
      bind:edges
      {nodeTypes}
      {edgeTypes}
      onconnect={handleConnect}
      onreconnectstart={handleReconnectStart}
      onreconnectend={handleReconnectEnd}
      onreconnect={handleReconnect}
      {isValidConnection}
      onnodeschange={handleNodesChange}
      onedgeschange={handleEdgesChange}
      onnodeclick={handleNodeClick}
      {...flowOptions}
      fitView
      class="action-flow"
    >
      <Controls />
      <MiniMap nodeColor="var(--action-minimap-node, #e2e8f0)" />
      <Background variant="dots" gap={20} size={1} />
    </SvelteFlow>

    <!-- Save/Cancel buttons overlay -->
    <div class="absolute top-4 right-4 flex gap-2 z-10">
      <Button
        variant="default"
        onclick={onCancel}
        disabled={saving}
        keyboardHint={getShortcutDisplay('actions', 'cancel')}
        hotkeyConfig={{ key: toHotkeyString('actions', 'cancel'), guard: () => !saving }}
      >
        {t('common.cancel')}
      </Button>
      <Button
        variant="primary"
        onclick={handleSave}
        disabled={saving}
        loading={saving}
        keyboardHint={getShortcutDisplay('actions', 'save')}
        hotkeyConfig={{ key: toHotkeyString('actions', 'save'), guard: () => !saving }}
      >
        {t('common.save')}
      </Button>
    </div>

    <!-- Action info header -->
    <div class="absolute top-4 left-4 z-10 action-header px-3 py-2 rounded-lg border">
      <div class="text-sm font-medium">{action?.name || t('actions.newAction')}</div>
      <div class="text-xs sidebar-subtitle">
        {triggerTypes.find(tt => tt.value === action?.trigger_type)?.label || action?.trigger_type}
      </div>
    </div>
  </div>

  <!-- Config Panel (shown when node is selected) -->
  {#if selectedNode}
    <div class="w-80 sidebar border-l p-4 overflow-y-auto flex-shrink-0">
      <div class="flex items-center justify-between mb-4">
        <h3 class="text-sm font-medium sidebar-title">{t('actions.nodeConfig')}</h3>
        <button
          class="text-sm text-gray-500 hover:text-gray-700"
          onclick={handleClearSelection}
        >
          &times;
        </button>
      </div>

      <div class="space-y-4">
        {#if selectedNode.type === 'trigger'}
          <div>
            <label class="block text-xs font-medium mb-1">{t('actions.config.triggerType')}</label>
            <select
              class="w-full px-3 py-2 border rounded-md text-sm config-input"
              value={selectedNode.data?.triggerType || action?.trigger_type || 'status_transition'}
              onchange={handleTriggerTypeChange}
            >
              {#each triggerTypes as type}
                <option value={type.value}>{type.label}</option>
              {/each}
            </select>
          </div>
          {#if (selectedNode.data?.triggerType || action?.trigger_type) === 'status_transition'}
            <div>
              <label class="block text-xs font-medium mb-1">{t('actions.config.fromStatus')}</label>
              <select
                class="w-full px-3 py-2 border rounded-md text-sm config-input"
                value={selectedNode.data?.config?.from_status_id || ''}
                onchange={handleFromStatusChange}
              >
                <option value="">{t('actions.config.anyStatus')}</option>
                {#each statuses as status}
                  <option value={status.id}>{status.name}</option>
                {/each}
              </select>
            </div>
            <div>
              <label class="block text-xs font-medium mb-1">{t('actions.config.toStatus')}</label>
              <select
                class="w-full px-3 py-2 border rounded-md text-sm config-input"
                value={selectedNode.data?.config?.to_status_id || ''}
                onchange={handleToStatusChange}
              >
                <option value="">{t('actions.config.anyStatus')}</option>
                {#each statuses as status}
                  <option value={status.id}>{status.name}</option>
                {/each}
              </select>
            </div>
          {/if}
          <div class="pt-4 border-t cascade-option">
            <Checkbox
              checked={selectedNode.data?.config?.respond_to_cascades || false}
              onchange={handleRespondToCascadesChange}
              label={t('actions.trigger.respondToCascades')}
              hint={t('actions.trigger.respondToCascadesHint')}
              size="small"
            />
          </div>
        {:else if selectedNode.type === 'set_status'}
          <div>
            <label class="block text-xs font-medium mb-1">{t('actions.config.targetStatus')}</label>
            <select
              class="w-full px-3 py-2 border rounded-md text-sm config-input"
              value={selectedNode.data?.config?.status_id || ''}
              onchange={handleTargetStatusChange}
            >
              <option value="">{t('actions.config.selectStatus')}</option>
              {#each statuses as status}
                <option value={status.id}>{status.name}</option>
              {/each}
            </select>
          </div>
        {:else if selectedNode.type === 'set_field'}
          <div>
            <label class="block text-xs font-medium mb-1">{t('actions.config.fieldName')}</label>
            <input
              type="text"
              class="w-full px-3 py-2 border rounded-md text-sm config-input"
              value={selectedNode.data?.config?.field_name || ''}
              oninput={handleFieldNameChange}
              placeholder="assignee_id, priority_id, etc."
            />
          </div>
          <div>
            <div class="flex items-center gap-1 mb-1">
              <label class="block text-xs font-medium">{t('actions.config.value')}</label>
              <button
                onclick={() => showPlaceholderModal = true}
                class="text-[var(--ds-text-subtlest)] hover:text-[var(--ds-interactive)] transition-colors"
                title={t('actions.placeholders.showReference')}
              >
                <HelpCircle class="w-3.5 h-3.5" />
              </button>
            </div>
            <input
              type="text"
              class="w-full px-3 py-2 border rounded-md text-sm config-input"
              value={selectedNode.data?.config?.value || ''}
              oninput={handleFieldValueChange}
              placeholder="{'{{'}item.creator_id{'}}'}"
            />
          </div>
        {:else if selectedNode.type === 'add_comment'}
          <div>
            <div class="flex items-center gap-1 mb-1">
              <label class="block text-xs font-medium">{t('actions.config.commentContent')}</label>
              <button
                onclick={() => showPlaceholderModal = true}
                class="text-[var(--ds-text-subtlest)] hover:text-[var(--ds-interactive)] transition-colors"
                title={t('actions.placeholders.showReference')}
              >
                <HelpCircle class="w-3.5 h-3.5" />
              </button>
            </div>
            <textarea
              class="w-full px-3 py-2 border rounded-md text-sm config-input"
              rows="4"
              value={selectedNode.data?.config?.content || ''}
              oninput={handleCommentContentChange}
              placeholder={t('actions.config.commentPlaceholder')}
            ></textarea>
          </div>
          <Checkbox
            checked={selectedNode.data?.config?.is_private || false}
            onchange={handlePrivateChange}
            label={t('actions.config.privateComment')}
            size="small"
          />
        {:else if selectedNode.type === 'condition'}
          <div>
            <label class="block text-xs font-medium mb-1">{t('actions.config.fieldToCheck')}</label>
            <FieldSelector
              selectedField={selectedNode.data?.config?.field_name ? { id: selectedNode.data.config.field_name, name: selectedNode.data.config.field_name } : null}
              onselect={handleConditionFieldSelect}
              onclear={handleConditionFieldClear}
            />
          </div>
          <div>
            <label class="block text-xs font-medium mb-1">{t('actions.config.operator')}</label>
            <select
              class="w-full px-3 py-2 border rounded-md text-sm config-input"
              value={selectedNode.data?.config?.operator || 'eq'}
              onchange={handleOperatorChange}
            >
              <option value="eq">{t('actions.operators.equals')}</option>
              <option value="ne">{t('actions.operators.notEquals')}</option>
              <option value="contains">{t('actions.operators.contains')}</option>
              <option value="gt">{t('actions.operators.greaterThan')}</option>
              <option value="lt">{t('actions.operators.lessThan')}</option>
              <option value="is_empty">{t('actions.operators.isEmpty')}</option>
              <option value="is_not_empty">{t('actions.operators.isNotEmpty')}</option>
            </select>
          </div>
          <div>
            <label class="block text-xs font-medium mb-1">{t('actions.config.compareValue')}</label>
            <input
              type="text"
              class="w-full px-3 py-2 border rounded-md text-sm config-input"
              value={selectedNode.data?.config?.value || ''}
              oninput={handleConditionValueChange}
            />
          </div>
        {:else if selectedNode.type === 'notify_user'}
          <div>
            <label class="block text-xs font-medium mb-1">{t('actions.config.recipientType')}</label>
            <select
              class="w-full px-3 py-2 border rounded-md text-sm config-input"
              value={selectedNode.data?.config?.recipient_type || 'assignee'}
              onchange={handleRecipientTypeChange}
            >
              <option value="assignee">{t('actions.recipients.assignee')}</option>
              <option value="creator">{t('actions.recipients.creator')}</option>
              <option value="specific">{t('actions.recipients.specific')}</option>
            </select>
          </div>
          <div>
            <div class="flex items-center gap-1 mb-1">
              <label class="block text-xs font-medium">{t('actions.config.notifyMessage')}</label>
              <button
                onclick={() => showPlaceholderModal = true}
                class="text-[var(--ds-text-subtlest)] hover:text-[var(--ds-interactive)] transition-colors"
                title={t('actions.placeholders.showReference')}
              >
                <HelpCircle class="w-3.5 h-3.5" />
              </button>
            </div>
            <textarea
              class="w-full px-3 py-2 border rounded-md text-sm config-input"
              rows="4"
              value={selectedNode.data?.config?.message || ''}
              oninput={handleNotifyMessageChange}
              placeholder={t('actions.config.notifyPlaceholder')}
            ></textarea>
          </div>
          <Checkbox
            checked={selectedNode.data?.config?.include_link ?? true}
            onchange={handleIncludeLinkChange}
            label={t('actions.config.includeLink')}
            size="small"
          />
        {:else if selectedNode.type === 'update_asset'}
          <UpdateAssetConfigPanel {selectedNode} bind:showPlaceholderModal />
        {:else if selectedNode.type === 'create_asset'}
          <CreateAssetConfigPanel {selectedNode} bind:showPlaceholderModal />
        {/if}
      </div>
    </div>
  {/if}
</div>

{#if showPlaceholderModal}
  <PlaceholderReferenceModal onclose={() => showPlaceholderModal = false} />
{/if}

<style>
  .action-flow-editor {
    background-color: var(--ds-surface);
  }

  .sidebar {
    background-color: var(--ds-surface-raised);
    border-color: var(--ds-border);
  }

  .sidebar-title {
    color: var(--ds-text);
  }

  .sidebar-subtitle {
    color: var(--ds-text-subtle);
  }

  .sidebar-hints {
    color: var(--ds-text-subtlest);
  }

  .node-palette-item {
    background-color: var(--ds-surface);
    color: var(--ds-text-subtle);
    transition:
      background-color 200ms ease,
      color 100ms ease,
      transform 100ms cubic-bezier(0.34, 1.56, 0.64, 1);
  }

  .node-palette-item:hover {
    background-color: var(--ds-surface-hovered);
    color: var(--ds-text);
    transform: translateX(4px);
  }

  .node-palette-item:active {
    transform: translateX(2px) scale(0.98);
  }

  .action-header {
    background-color: var(--ds-surface-raised);
    border-color: var(--ds-border);
    color: var(--ds-text);
  }

  .config-input {
    background-color: var(--ds-surface);
    border-color: var(--ds-border);
    color: var(--ds-text);
  }

  .config-input:focus {
    border-color: var(--ds-interactive);
    outline: none;
    ring: 2px var(--ds-interactive);
  }

  .cascade-option {
    border-color: var(--ds-border);
  }

  .cascade-hint {
    color: var(--ds-text-subtlest);
  }

  :global(.action-flow) {
    background-color: var(--ds-surface);
  }

  :global(.action-flow .svelte-flow__background) {
    background-color: var(--ds-surface);
  }

  :global(.action-flow .svelte-flow__controls button) {
    background-color: var(--ds-surface-raised);
    color: var(--ds-text);
    border: 1px solid var(--ds-border);
  }

  :global(.action-flow .svelte-flow__controls button:hover) {
    background-color: var(--ds-surface-hovered);
  }

  :global(.action-flow .svelte-flow__minimap) {
    background-color: var(--ds-surface-raised);
    border: 1px solid var(--ds-border);
  }

  :global(.action-flow .svelte-flow__attribution) {
    background-color: transparent;
  }

  :global(.action-flow .svelte-flow__attribution a) {
    color: var(--ds-text-subtlest);
  }
</style>
