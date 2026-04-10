<script>
  import { onMount, untrack } from 'svelte';
  import {
    SvelteFlow,
    Controls,
    MiniMap,
    Background,
    ConnectionMode,
    addEdge,
  } from '@xyflow/svelte';
  import '@xyflow/svelte/dist/style.css';
  import { FileText, Pencil, Bell, HelpCircle, Zap, ArrowRight, ArrowDown } from 'lucide-svelte';
  import Button from '../../components/Button.svelte';
  import Select from '../../components/Select.svelte';
  import TriggerNode from '../actions/nodes/TriggerNode.svelte';
  import SetFieldNode from '../actions/nodes/SetFieldNode.svelte';
  import SetStatusNode from '../actions/nodes/SetStatusNode.svelte';
  import ConditionNode from '../actions/nodes/ConditionNode.svelte';
  import NotifyUserNode from '../actions/nodes/NotifyUserNode.svelte';
  import ActionEdge from '../actions/edges/ActionEdge.svelte';
  import { assetActionFlowStore } from '../../stores/assetActionFlowStore.svelte.js';
  import { errorToast } from '../../stores/toasts.svelte.js';
  import FieldSelector from '../../pickers/FieldSelector.svelte';
  import { api } from '../../api.js';
  import { t } from '../../stores/i18n.svelte.js';

  // Reuse CreateItemNode from logbook-actions (same visual)
  import CreateItemNode from '../logbook-actions/nodes/CreateItemNode.svelte';
  import CreateItemConfigPanel from '../logbook-actions/CreateItemConfigPanel.svelte';

  let { action, onSave, onCancel } = $props();

  let nodes = $state([]);
  let edges = $state([]);
  let selectedNodeId = $state(null);
  let saving = $state(false);
  let isReconnecting = $state(false);
  let lastStoreNodesVersion = $state(0);

  $effect(() => {
    const storeNodes = assetActionFlowStore.nodes;
    const currentVersion = storeNodes.length + JSON.stringify(storeNodes.map((n) => n.data));
    const lastVersion = untrack(() => lastStoreNodesVersion);
    const localNodes = untrack(() => nodes);

    if (currentVersion !== lastVersion) {
      lastStoreNodesVersion = currentVersion;
      nodes = storeNodes.map((storeNode) => {
        const localNode = localNodes.find((n) => n.id === storeNode.id);
        if (localNode) {
          return { ...storeNode, position: localNode.position };
        }
        return storeNode;
      });
    }
  });

  $effect(() => {
    edges = assetActionFlowStore.edges;
  });
  $effect(() => {
    selectedNodeId = assetActionFlowStore.selectedNodeId;
  });
  $effect(() => {
    saving = assetActionFlowStore.saving;
  });

  let selectedNode = $derived(selectedNodeId ? nodes.find((n) => n.id === selectedNodeId) : null);

  // Asset-specific field groups for the FieldSelector
  const assetFieldGroups = [
    {
      category: t('pickers.fieldCategories.basic'),
      fields: [
        { id: 'title', name: 'Title', type: 'text' },
        { id: 'asset_tag', name: 'Asset Tag', type: 'identifier' },
        { id: 'description', name: 'Description', type: 'text' },
      ],
    },
  ];

  // Load asset custom fields for the FieldSelector
  let assetCustomFields = $state([]);
  $effect(() => {
    api.customFields.getAll().then((result) => {
      assetCustomFields = (result?.data || []).map((field) => ({
        id: String(field.id),
        name: field.name,
        type: field.field_type,
        description: field.description || '',
        isCustom: true,
      }));
    }).catch(() => {
      assetCustomFields = [];
    });
  });

  // Resolve selectedField from the current node config
  let setFieldSelected = $derived.by(() => {
    if (!selectedNode || selectedNode.type !== 'set_field') return null;
    const config = selectedNode.data?.config;
    if (!config?.field_name) return null;
    // Check built-in fields
    const builtIn = assetFieldGroups[0].fields.find((f) => f.id === config.field_name);
    if (builtIn) return builtIn;
    // Check custom fields
    const custom = assetCustomFields.find((f) => f.id === config.field_name);
    if (custom) return custom;
    // Fallback: reconstruct from config
    return { id: config.field_name, name: config.field_display_name || config.field_name, type: 'text' };
  });

  const nodeTypes = {
    trigger: TriggerNode,
    create_item: CreateItemNode,
    set_field: SetFieldNode,
    set_status: SetStatusNode,
    condition: ConditionNode,
    notify_user: NotifyUserNode,
  };

  const edgeTypes = { action: ActionEdge };

  const flowOptions = {
    connectionMode: ConnectionMode.Loose,
    attributionPosition: 'bottom-left',
    defaultViewport: { x: 0, y: 0, zoom: 0.7 },
    fitViewOptions: { maxZoom: 1, padding: 0.1 },
    minZoom: 0.2,
    maxZoom: 1.5,
    defaultEdgeOptions: { type: 'action' },
  };

  const nodePalette = [
    { type: 'create_item', label: 'Create Work Item', icon: FileText },
    { type: 'set_field', label: 'Set Field', icon: Pencil },
    { type: 'set_status', label: 'Set Status', icon: Zap },
    { type: 'condition', label: 'Condition', icon: HelpCircle },
    { type: 'notify_user', label: 'Notify User', icon: Bell },
  ];

  const triggerTypes = [
    { value: 'asset_created', label: 'Asset Created' },
    { value: 'asset_updated', label: 'Asset Updated' },
    { value: 'asset_status_changed', label: 'Status Changed' },
    { value: 'manual', label: 'Manual' },
  ];

  const conditionFields = [
    { value: 'title', label: 'Title' },
    { value: 'asset_tag', label: 'Asset Tag' },
    { value: 'type_name', label: 'Type Name' },
    { value: 'status_name', label: 'Status Name' },
  ];

  const conditionOperators = [
    { value: 'eq', label: 'Equals' },
    { value: 'ne', label: 'Not Equals' },
    { value: 'contains', label: 'Contains' },
    { value: 'not_contains', label: 'Not Contains' },
    { value: 'starts_with', label: 'Starts With' },
    { value: 'ends_with', label: 'Ends With' },
  ];

  onMount(() => {
    assetActionFlowStore.init(action);
  });

  function handleConnect(params) {
    const newEdge = assetActionFlowStore.addEdge(params);
    assetActionFlowStore.setEdges(addEdge(newEdge, assetActionFlowStore.edges));
  }

  function handleNodesChange(event) {
    const changes = event.detail;
    changes.forEach((change) => {
      if (change.type === 'position' && !change.dragging) {
        const node = nodes.find((n) => n.id === change.id);
        if (node?.position) {
          assetActionFlowStore.updateNodePosition(change.id, node.position);
        }
      }
    });
  }

  function handleEdgesChange(event) {
    const changes = event.detail;
    const edgesToRemove = changes.filter((c) => c.type === 'remove').map((c) => c.id);
    if (edgesToRemove.length > 0) {
      assetActionFlowStore.removeEdges(edgesToRemove);
    }
  }

  function handleReconnectStart() {
    isReconnecting = true;
  }
  function handleReconnectEnd() {
    isReconnecting = false;
  }

  function handleReconnect(oldEdge, newConnection) {
    assetActionFlowStore.updateEdge(oldEdge.id, {
      source: newConnection.source,
      target: newConnection.target,
      sourceHandle: newConnection.sourceHandle,
      targetHandle: newConnection.targetHandle,
    });
  }

  function isValidConnection(connection) {
    if (isReconnecting) return true;
    if (connection.source === connection.target) return false;
    const targetNode = nodes.find((n) => n.id === connection.target);
    if (targetNode?.type === 'trigger') return false;
    return true;
  }

  function handleNodeClick(event) {
    const node = event.detail?.node || event.node;
    if (node) assetActionFlowStore.selectNode(node.id);
  }

  function handleAddNode(nodeType) {
    const newNode = assetActionFlowStore.addNode(nodeType);
    assetActionFlowStore.selectNode(newNode.id);
  }

  function handleClearSelection() {
    assetActionFlowStore.clearSelection();
  }

  function handleTriggerTypeChange(value) {
    assetActionFlowStore.updateNodeData(selectedNode.id, { triggerType: value });
    assetActionFlowStore.updateTriggerType(value);
  }

  async function handleSave() {
    assetActionFlowStore.setSaving(true);
    try {
      const apiData = assetActionFlowStore.toApiFormat(action);
      await onSave?.(apiData);
    } catch (err) {
      errorToast('Failed to save action');
      console.error(err);
    } finally {
      assetActionFlowStore.setSaving(false);
    }
  }

  function handleDeleteNode() {
    if (selectedNode && selectedNode.type !== 'trigger') {
      assetActionFlowStore.removeNode(selectedNode.id);
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
        <span class="font-medium text-sm" style="color: var(--ds-text);">Asset Actions</span>
      </div>
    </div>

    <div class="px-4">
      <h3 class="text-sm font-medium sidebar-title mb-3">Add Nodes</h3>
      <div class="space-y-2">
        {#each nodePalette as item}
          <button
            class="w-full px-3 py-2 text-left rounded-lg text-sm font-medium flex items-center gap-2 node-palette-item cursor-pointer"
            onclick={() => handleAddNode(item.type)}
          >
            <svelte:component this={item.icon} size={16} class="flex-shrink-0" />
            <span>{item.label}</span>
          </button>
        {/each}
      </div>

      <div class="mt-6 pt-4 border-t">
        <h4 class="text-xs font-medium sidebar-subtitle mb-2">Tips</h4>
        <ul class="text-xs space-y-1 sidebar-hints">
          <li>Drag handles to connect nodes</li>
          <li>Click a node to configure it</li>
          <li>Use conditions to branch the flow</li>
        </ul>
        <h4 class="text-xs font-medium sidebar-subtitle mb-2 mt-4">Variables</h4>
        <ul class="text-xs space-y-1 sidebar-hints">
          <li><code>{'{{asset.title}}'}</code>, <code>{'{{asset.tag}}'}</code></li>
          <li><code>{'{{asset.type_name}}'}</code>, <code>{'{{asset.status_name}}'}</code></li>
          <li><code>{'{{asset.id}}'}</code>, <code>{'{{actor.id}}'}</code></li>
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
      onnodeclick={handleNodeClick}
      onnodeschange={handleNodesChange}
      onedgeschange={handleEdgesChange}
      onreconnectstart={handleReconnectStart}
      onreconnectend={handleReconnectEnd}
      onreconnect={handleReconnect}
      {isValidConnection}
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
      <Button variant="default" onclick={onCancel} disabled={saving}>Cancel</Button>
      <Button variant="primary" onclick={handleSave} disabled={saving} loading={saving}>
        {saving ? 'Saving...' : 'Save'}
      </Button>
    </div>

    <!-- Action info header -->
    <div class="absolute top-4 left-4 z-10 flex items-start gap-2">
      <div class="action-header px-3 py-2 rounded-lg border">
        <div class="text-sm font-medium">{action?.name || 'New Action'}</div>
        <div class="text-xs sidebar-subtitle">
          {triggerTypes.find((tt) => tt.value === action?.trigger_type)?.label ||
            action?.trigger_type}
        </div>
      </div>
      <button
        class="direction-toggle rounded-lg border p-2"
        onclick={() => assetActionFlowStore.toggleDirection()}
        title={assetActionFlowStore.direction === 'horizontal'
          ? 'Switch to vertical'
          : 'Switch to horizontal'}
      >
        {#if assetActionFlowStore.direction === 'horizontal'}
          <ArrowRight size={16} />
        {:else}
          <ArrowDown size={16} />
        {/if}
      </button>
    </div>
  </div>

  <!-- Config Panel (shown when node is selected) -->
  {#if selectedNode}
    <div class="w-80 sidebar border-l p-4 overflow-y-auto flex-shrink-0">
      <div class="flex items-center justify-between mb-4">
        <h3 class="text-sm font-medium sidebar-title">Configuration</h3>
        <button class="text-sm text-gray-500 hover:text-gray-700" onclick={handleClearSelection}>
          &times;
        </button>
      </div>

      <div class="space-y-4">
        {#if selectedNode.type === 'trigger'}
          <div>
            <label for="trigger-type" class="block text-xs font-medium mb-1">Trigger Type</label>
            <Select
              id="trigger-type"
              options={triggerTypes}
              value={assetActionFlowStore.triggerType}
              onchange={handleTriggerTypeChange}
              size="small"
            />
          </div>

          {#if assetActionFlowStore.triggerType === 'asset_status_changed'}
            <div>
              <label class="block text-xs font-medium mb-1">From Status ID (optional)</label>
              <input
                type="number"
                class="w-full px-3 py-2 border rounded-md text-sm config-input"
                value={selectedNode.data?.config?.from_status_id || ''}
                oninput={(e) =>
                  assetActionFlowStore.updateNodeConfig(selectedNode.id, {
                    from_status_id: parseInt(e.target.value) || null,
                  })}
              />
            </div>
            <div>
              <label class="block text-xs font-medium mb-1">To Status ID (optional)</label>
              <input
                type="number"
                class="w-full px-3 py-2 border rounded-md text-sm config-input"
                value={selectedNode.data?.config?.to_status_id || ''}
                oninput={(e) =>
                  assetActionFlowStore.updateNodeConfig(selectedNode.id, {
                    to_status_id: parseInt(e.target.value) || null,
                  })}
              />
            </div>
          {/if}

          {#if assetActionFlowStore.triggerType !== 'manual'}
            <div>
              <label class="block text-xs font-medium mb-1">Asset Type ID (optional filter)</label>
              <input
                type="number"
                class="w-full px-3 py-2 border rounded-md text-sm config-input"
                value={selectedNode.data?.config?.asset_type_id || ''}
                oninput={(e) =>
                  assetActionFlowStore.updateNodeConfig(selectedNode.id, {
                    asset_type_id: parseInt(e.target.value) || null,
                  })}
              />
            </div>
          {/if}
        {:else if selectedNode.type === 'create_item'}
          <CreateItemConfigPanel {selectedNode} />
          <Button variant="ghost" size="small" onclick={handleDeleteNode}>Delete Node</Button>
        {:else if selectedNode.type === 'set_field'}
          <div>
            <label class="block text-xs font-medium mb-1">Field</label>
            <FieldSelector
              fieldGroups={assetFieldGroups}
              customFieldItems={assetCustomFields}
              selectedField={setFieldSelected}
              onSelect={(field) => {
                assetActionFlowStore.updateNodeConfig(selectedNode.id, {
                  field_name: field.id,
                  field_display_name: field.name,
                });
              }}
              onClear={() => {
                assetActionFlowStore.updateNodeConfig(selectedNode.id, {
                  field_name: '',
                  field_display_name: '',
                });
              }}
            />
          </div>
          <div>
            <label class="block text-xs font-medium mb-1">Value</label>
            <input
              type="text"
              class="w-full px-3 py-2 border rounded-md text-sm config-input"
              value={selectedNode.data?.config?.value || ''}
              oninput={(e) =>
                assetActionFlowStore.updateNodeConfig(selectedNode.id, { value: e.target.value })}
            />
          </div>
          <Button variant="ghost" size="small" onclick={handleDeleteNode}>Delete Node</Button>
        {:else if selectedNode.type === 'set_status'}
          <div>
            <label class="block text-xs font-medium mb-1">Status ID</label>
            <input
              type="number"
              class="w-full px-3 py-2 border rounded-md text-sm config-input"
              value={selectedNode.data?.config?.status_id || ''}
              oninput={(e) =>
                assetActionFlowStore.updateNodeConfig(selectedNode.id, {
                  status_id: parseInt(e.target.value) || 0,
                })}
            />
          </div>
          <Button variant="ghost" size="small" onclick={handleDeleteNode}>Delete Node</Button>
        {:else if selectedNode.type === 'condition'}
          <div>
            <label for="condition-field" class="block text-xs font-medium mb-1">Field</label>
            <Select
              id="condition-field"
              options={conditionFields}
              value={selectedNode.data?.config?.field_name || ''}
              onchange={(v) =>
                assetActionFlowStore.updateNodeConfig(selectedNode.id, { field_name: v })}
              size="small"
            />
          </div>
          <div>
            <label for="condition-operator" class="block text-xs font-medium mb-1">Operator</label>
            <Select
              id="condition-operator"
              options={conditionOperators}
              value={selectedNode.data?.config?.operator || 'eq'}
              onchange={(v) =>
                assetActionFlowStore.updateNodeConfig(selectedNode.id, { operator: v })}
              size="small"
            />
          </div>
          <div>
            <label class="block text-xs font-medium mb-1">Value</label>
            <input
              type="text"
              class="w-full px-3 py-2 border rounded-md text-sm config-input"
              value={selectedNode.data?.config?.value || ''}
              oninput={(e) =>
                assetActionFlowStore.updateNodeConfig(selectedNode.id, { value: e.target.value })}
            />
          </div>
          <Button variant="ghost" size="small" onclick={handleDeleteNode}>Delete Node</Button>
        {:else if selectedNode.type === 'notify_user'}
          <div>
            <label class="block text-xs font-medium mb-1">User ID</label>
            <input
              type="number"
              class="w-full px-3 py-2 border rounded-md text-sm config-input"
              value={selectedNode.data?.config?.user_id || ''}
              oninput={(e) =>
                assetActionFlowStore.updateNodeConfig(selectedNode.id, {
                  user_id: parseInt(e.target.value) || 0,
                })}
            />
          </div>
          <div>
            <label class="block text-xs font-medium mb-1">Message</label>
            <textarea
              class="w-full px-3 py-2 border rounded-md text-sm config-input"
              rows="3"
              value={selectedNode.data?.config?.message || ''}
              oninput={(e) =>
                assetActionFlowStore.updateNodeConfig(selectedNode.id, { message: e.target.value })}
            ></textarea>
          </div>
          <Button variant="ghost" size="small" onclick={handleDeleteNode}>Delete Node</Button>
        {/if}
      </div>
    </div>
  {/if}
</div>

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

  .sidebar-hints code {
    font-size: 10px;
    background: var(--ds-surface-sunken);
    padding: 1px 4px;
    border-radius: 3px;
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

  .direction-toggle {
    background-color: var(--ds-surface-raised);
    border-color: var(--ds-border);
    color: var(--ds-text-subtle);
    cursor: pointer;
    transition:
      background-color 150ms ease,
      color 150ms ease;
  }

  .direction-toggle:hover {
    background-color: var(--ds-surface-hovered);
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
