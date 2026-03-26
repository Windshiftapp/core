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
  import { FileText, PlusSquare, Users, HelpCircle, Zap, ArrowRight, ArrowDown } from 'lucide-svelte';
  import Button from '../../components/Button.svelte';
  import Select from '../../components/Select.svelte';
  import LogbookTriggerNode from './nodes/LogbookTriggerNode.svelte';
  import CreateItemNode from './nodes/CreateItemNode.svelte';
  import AssociateCustomerNode from './nodes/AssociateCustomerNode.svelte';
  import LogbookConditionNode from './nodes/LogbookConditionNode.svelte';
  import CreateAssetNode from '../actions/nodes/CreateAssetNode.svelte';
  import ActionEdge from '../actions/edges/ActionEdge.svelte';
  import CreateAssetConfigPanel from '../actions/CreateAssetConfigPanel.svelte';
  import CreateItemConfigPanel from './CreateItemConfigPanel.svelte';
  import { logbookActionFlowStore } from '../../stores/logbookActionFlowStore.svelte.js';
  import { errorToast } from '../../stores/toasts.svelte.js';

  let { action, onSave, onCancel } = $props();

  let nodes = $state([]);
  let edges = $state([]);
  let selectedNodeId = $state(null);
  let saving = $state(false);
  let isReconnecting = $state(false);
  let lastStoreNodesVersion = $state(0);

  $effect(() => {
    const storeNodes = logbookActionFlowStore.nodes;
    const currentVersion = storeNodes.length + JSON.stringify(storeNodes.map(n => n.data));
    const lastVersion = untrack(() => lastStoreNodesVersion);
    const localNodes = untrack(() => nodes);

    if (currentVersion !== lastVersion) {
      lastStoreNodesVersion = currentVersion;
      nodes = storeNodes.map(storeNode => {
        const localNode = localNodes.find(n => n.id === storeNode.id);
        if (localNode) {
          return { ...storeNode, position: localNode.position };
        }
        return storeNode;
      });
    }
  });

  $effect(() => { edges = logbookActionFlowStore.edges; });
  $effect(() => { selectedNodeId = logbookActionFlowStore.selectedNodeId; });
  $effect(() => { saving = logbookActionFlowStore.saving; });

  let selectedNode = $derived(
    selectedNodeId ? nodes.find(n => n.id === selectedNodeId) : null
  );

  const nodeTypes = {
    trigger: LogbookTriggerNode,
    create_item: CreateItemNode,
    create_asset: CreateAssetNode,
    associate_customer: AssociateCustomerNode,
    condition: LogbookConditionNode
  };

  const edgeTypes = { action: ActionEdge };

  const flowOptions = {
    connectionMode: ConnectionMode.Loose,
    attributionPosition: 'bottom-left',
    defaultViewport: { x: 0, y: 0, zoom: 0.7 },
    fitViewOptions: { maxZoom: 1, padding: 0.1 },
    minZoom: 0.2,
    maxZoom: 1.5,
    defaultEdgeOptions: { type: 'action' }
  };

  const nodePalette = [
    { type: 'create_item', label: 'Create Item', icon: FileText },
    { type: 'create_asset', label: 'Create Asset', icon: PlusSquare },
    { type: 'associate_customer', label: 'Associate Customer', icon: Users },
    { type: 'condition', label: 'Condition', icon: HelpCircle }
  ];

  const triggerTypes = [
    { value: 'document_classified', label: 'Document Classified' },
    { value: 'content_keyword', label: 'Content Keyword' },
    { value: 'mime_type', label: 'MIME Type' },
    { value: 'manual', label: 'Manual' }
  ];

  const conditionFields = [
    { value: 'content_type', label: 'Content Type' },
    { value: 'mime_type', label: 'MIME Type' },
    { value: 'title', label: 'Title' },
    { value: 'source_type', label: 'Source Type' },
    { value: 'author', label: 'Author' }
  ];

  const conditionOperators = [
    { value: 'eq', label: 'Equals' },
    { value: 'ne', label: 'Not Equals' },
    { value: 'contains', label: 'Contains' },
    { value: 'not_contains', label: 'Not Contains' },
    { value: 'starts_with', label: 'Starts With' },
    { value: 'ends_with', label: 'Ends With' },
    { value: 'matches', label: 'Matches (Regex)' }
  ];

  onMount(() => {
    logbookActionFlowStore.init(action);
  });

  function handleConnect(params) {
    const newEdge = logbookActionFlowStore.addEdge(params);
    logbookActionFlowStore.setEdges(addEdge(newEdge, logbookActionFlowStore.edges));
  }

  function handleNodesChange(event) {
    const changes = event.detail;
    changes.forEach(change => {
      if (change.type === 'position' && !change.dragging) {
        const node = nodes.find(n => n.id === change.id);
        if (node?.position) {
          logbookActionFlowStore.updateNodePosition(change.id, node.position);
        }
      }
    });
  }

  function handleEdgesChange(event) {
    const changes = event.detail;
    const edgesToRemove = changes.filter(c => c.type === 'remove').map(c => c.id);
    if (edgesToRemove.length > 0) {
      logbookActionFlowStore.removeEdges(edgesToRemove);
    }
  }

  function handleReconnectStart() { isReconnecting = true; }
  function handleReconnectEnd() { isReconnecting = false; }

  function handleReconnect(oldEdge, newConnection) {
    logbookActionFlowStore.updateEdge(oldEdge.id, {
      source: newConnection.source,
      target: newConnection.target,
      sourceHandle: newConnection.sourceHandle,
      targetHandle: newConnection.targetHandle
    });
  }

  function isValidConnection(connection) {
    if (isReconnecting) return true;
    if (connection.source === connection.target) return false;
    const targetNode = nodes.find(n => n.id === connection.target);
    if (targetNode?.type === 'trigger') return false;
    return true;
  }

  function handleNodeClick(event) {
    const node = event.detail?.node || event.node;
    if (node) logbookActionFlowStore.selectNode(node.id);
  }

  function handleAddNode(nodeType) {
    const newNode = logbookActionFlowStore.addNode(nodeType);
    logbookActionFlowStore.selectNode(newNode.id);
  }

  function handleClearSelection() {
    logbookActionFlowStore.clearSelection();
  }

  function handleTriggerTypeChange(value) {
    logbookActionFlowStore.updateNodeData(selectedNode.id, { triggerType: value });
    logbookActionFlowStore.updateTriggerType(value);
  }

  async function handleSave() {
    logbookActionFlowStore.setSaving(true);
    try {
      const apiData = logbookActionFlowStore.toApiFormat(action);
      await onSave?.(apiData);
    } catch (err) {
      errorToast('Failed to save action');
      console.error(err);
    } finally {
      logbookActionFlowStore.setSaving(false);
    }
  }

  function handleDeleteNode() {
    if (selectedNode && selectedNode.type !== 'trigger') {
      logbookActionFlowStore.removeNode(selectedNode.id);
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
        <span class="font-medium text-sm" style="color: var(--ds-text);">Actions</span>
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
          <li><code>{'{{doc.title}}'}</code>, <code>{'{{doc.content_type}}'}</code></li>
          <li><code>{'{{doc.mime_type}}'}</code>, <code>{'{{doc.source_type}}'}</code></li>
          <li><code>{'{{doc.author}}'}</code>, <code>{'{{doc.id}}'}</code></li>
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
      <Button variant="default" onclick={onCancel} disabled={saving}>
        Cancel
      </Button>
      <Button variant="primary" onclick={handleSave} disabled={saving} loading={saving}>
        {saving ? 'Saving...' : 'Save'}
      </Button>
    </div>

    <!-- Action info header -->
    <div class="absolute top-4 left-4 z-10 flex items-start gap-2">
      <div class="action-header px-3 py-2 rounded-lg border">
        <div class="text-sm font-medium">{action?.name || 'New Action'}</div>
        <div class="text-xs sidebar-subtitle">
          {triggerTypes.find(tt => tt.value === action?.trigger_type)?.label || action?.trigger_type}
        </div>
      </div>
      <button
        class="direction-toggle rounded-lg border p-2"
        onclick={() => logbookActionFlowStore.toggleDirection()}
        title={logbookActionFlowStore.direction === 'horizontal' ? 'Switch to vertical' : 'Switch to horizontal'}
      >
        {#if logbookActionFlowStore.direction === 'horizontal'}
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
            <label for="trigger-type" class="block text-xs font-medium mb-1">Trigger Type</label>
            <Select
              id="trigger-type"
              options={triggerTypes}
              value={logbookActionFlowStore.triggerType}
              onchange={handleTriggerTypeChange}
              size="small"
            />
          </div>

          {#if logbookActionFlowStore.triggerType === 'document_classified'}
            <div>
              <label class="block text-xs font-medium mb-1">Content Types</label>
              <div class="flex flex-col gap-1.5">
                {#each ['knowledge', 'record', 'correspondence'] as ct}
                  <label class="checkbox-label">
                    <input
                      type="checkbox"
                      checked={selectedNode.data?.config?.content_types?.includes(ct) || false}
                      onchange={(e) => {
                        const current = selectedNode.data?.config?.content_types || [];
                        const updated = e.target.checked
                          ? [...current, ct]
                          : current.filter(c => c !== ct);
                        logbookActionFlowStore.updateNodeConfig(selectedNode.id, { content_types: updated });
                      }}
                    />
                    {ct}
                  </label>
                {/each}
              </div>
            </div>
          {/if}

          {#if logbookActionFlowStore.triggerType === 'content_keyword'}
            <div>
              <label class="block text-xs font-medium mb-1">Keywords (one per line)</label>
              <textarea
                class="w-full px-3 py-2 border rounded-md text-sm config-input"
                rows="4"
                value={selectedNode.data?.config?.keywords?.join('\n') || ''}
                oninput={(e) => {
                  const keywords = e.target.value.split('\n').filter(k => k.trim());
                  logbookActionFlowStore.updateNodeConfig(selectedNode.id, { keywords });
                }}
              ></textarea>
            </div>
            <div>
              <label for="keyword-mode" class="block text-xs font-medium mb-1">Match Mode</label>
              <Select
                id="keyword-mode"
                options={[{ value: 'any', label: 'Match Any' }, { value: 'all', label: 'Match All' }]}
                value={selectedNode.data?.config?.keyword_mode || 'any'}
                onchange={(v) => logbookActionFlowStore.updateNodeConfig(selectedNode.id, { keyword_mode: v })}
                size="small"
              />
            </div>
          {/if}

          {#if logbookActionFlowStore.triggerType === 'mime_type'}
            <div>
              <label class="block text-xs font-medium mb-1">MIME Types (one per line)</label>
              <textarea
                class="w-full px-3 py-2 border rounded-md text-sm config-input"
                rows="3"
                placeholder="e.g. application/pdf&#10;image/*"
                value={selectedNode.data?.config?.mime_types?.join('\n') || ''}
                oninput={(e) => {
                  const mime_types = e.target.value.split('\n').filter(m => m.trim());
                  logbookActionFlowStore.updateNodeConfig(selectedNode.id, { mime_types });
                }}
              ></textarea>
            </div>
          {/if}

        {:else if selectedNode.type === 'create_item'}
          <CreateItemConfigPanel {selectedNode} />
          <Button variant="ghost" size="small" onclick={handleDeleteNode}>Delete Node</Button>

        {:else if selectedNode.type === 'create_asset'}
          <CreateAssetConfigPanel {selectedNode} />
          <Button variant="ghost" size="small" onclick={handleDeleteNode}>Delete Node</Button>

        {:else if selectedNode.type === 'associate_customer'}
          <div>
            <label class="block text-xs font-medium mb-1">Customer Organisation ID</label>
            <input
              type="number"
              class="w-full px-3 py-2 border rounded-md text-sm config-input"
              value={selectedNode.data?.config?.customer_organisation_id || ''}
              oninput={(e) => logbookActionFlowStore.updateNodeConfig(selectedNode.id, { customer_organisation_id: parseInt(e.target.value) || null })}
            />
          </div>
          <div>
            <label class="block text-xs font-medium mb-1">Portal Customer ID</label>
            <input
              type="number"
              class="w-full px-3 py-2 border rounded-md text-sm config-input"
              value={selectedNode.data?.config?.portal_customer_id || ''}
              oninput={(e) => logbookActionFlowStore.updateNodeConfig(selectedNode.id, { portal_customer_id: parseInt(e.target.value) || null })}
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
              onchange={(v) => logbookActionFlowStore.updateNodeConfig(selectedNode.id, { field_name: v })}
              size="small"
            />
          </div>
          <div>
            <label for="condition-operator" class="block text-xs font-medium mb-1">Operator</label>
            <Select
              id="condition-operator"
              options={conditionOperators}
              value={selectedNode.data?.config?.operator || 'eq'}
              onchange={(v) => logbookActionFlowStore.updateNodeConfig(selectedNode.id, { operator: v })}
              size="small"
            />
          </div>
          <div>
            <label class="block text-xs font-medium mb-1">Value</label>
            <input
              type="text"
              class="w-full px-3 py-2 border rounded-md text-sm config-input"
              value={selectedNode.data?.config?.value || ''}
              oninput={(e) => logbookActionFlowStore.updateNodeConfig(selectedNode.id, { value: e.target.value })}
            />
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
    transition: background-color 150ms ease, color 150ms ease;
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

  .checkbox-label {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 13px;
    color: var(--ds-text);
    cursor: pointer;
    text-transform: capitalize;
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
