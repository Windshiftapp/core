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
  import { Zap, ArrowRight, ArrowDown } from 'lucide-svelte';
  import Button from '../../../components/Button.svelte';
  import ActionEdge from '../edges/ActionEdge.svelte';
  import { errorToast } from '../../../stores/toasts.svelte.js';

  let {
    action,
    flowStore,
    nodeTypes,
    nodePalette = [],
    triggerTypes = [],
    sidebarTitle = 'Actions',
    addNodesLabel = 'Add Nodes',
    tipsLabel = 'Tips',
    tips = [
      'Drag handles to connect nodes',
      'Click a node to configure it',
      'Use conditions to branch the flow',
    ],
    nodeConfigLabel = 'Configuration',
    newActionLabel = 'New Action',
    cancelLabel = 'Cancel',
    saveLabel = 'Save',
    savingLabel = 'Saving...',
    switchToVerticalLabel = 'Switch to vertical',
    switchToHorizontalLabel = 'Switch to horizontal',
    saveErrorMessage = 'Failed to save action',
    cancelButtonProps = {},
    saveButtonProps = {},
    minimapClass = '',
    minimapNodeColor = 'var(--action-minimap-node, #e2e8f0)',
    minimapNodeStrokeColor = undefined,
    minimapNodeStrokeWidth = undefined,
    minimapNodeBorderRadius = undefined,
    minimapMaskColor = undefined,
    initArgs = [],
    onSave,
    onCancel,
    triggerConfig,
    nodeConfig,
    sidebarExtra,
    sidebarTop,
  } = $props();

  let nodes = $state([]);
  let edges = $state([]);
  let selectedNodeId = $state(null);
  let saving = $state(false);
  let isReconnecting = $state(false);
  let lastStoreNodesVersion = $state(0);

  // Viewport tracking so handleAddNode can drop new nodes inside the visible
  // canvas. We observe via onmove instead of binding to defaultViewport so
  // SvelteFlow stays uncontrolled — defaultViewport remains authoritative for
  // initial render.
  let flowContainer = $state(null);
  let flowViewport = $state({ x: 0, y: 0, zoom: 0.7 });

  function handleMove(_event, viewport) {
    flowViewport = viewport;
  }

  $effect(() => {
    const storeNodes = flowStore.nodes;
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

  $effect(() => { edges = flowStore.edges; });
  $effect(() => { selectedNodeId = flowStore.selectedNodeId; });
  $effect(() => { saving = flowStore.saving; });

  let selectedNode = $derived(
    selectedNodeId ? nodes.find(n => n.id === selectedNodeId) : null
  );

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

  onMount(() => {
    flowStore.init(action, ...initArgs);
  });

  function handleConnect(params) {
    const newEdge = flowStore.addEdge(params);
    flowStore.setEdges(addEdge(newEdge, flowStore.edges));
  }

  function handleNodesChange(event) {
    const changes = event.detail;
    changes.forEach(change => {
      if (change.type === 'position' && !change.dragging) {
        const node = nodes.find(n => n.id === change.id);
        if (node?.position) {
          flowStore.updateNodePosition(change.id, node.position);
        }
      }
    });
  }

  function handleEdgesChange(event) {
    const changes = event.detail;
    const edgesToRemove = changes.filter(c => c.type === 'remove').map(c => c.id);
    if (edgesToRemove.length > 0) {
      flowStore.removeEdges(edgesToRemove);
    }
  }

  function handleReconnectStart() { isReconnecting = true; }
  function handleReconnectEnd() { isReconnecting = false; }

  function handleReconnect(oldEdge, newConnection) {
    flowStore.updateEdge(oldEdge.id, {
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
    if (node) flowStore.selectNode(node.id);
  }

  // Approximate default node footprint used to center the drop: nodes are
  // around 180x80 px at zoom=1. Offsetting by half keeps the new node roughly
  // centered on the viewport rather than anchored at its top-left.
  const NODE_CENTER_OFFSET = { x: 90, y: 40 };

  function viewportCenterInFlowCoords() {
    if (!flowContainer) return null;
    const rect = flowContainer.getBoundingClientRect();
    if (!rect.width || !rect.height) return null;
    const zoom = flowViewport.zoom || 1;
    return {
      x: (rect.width / 2 - flowViewport.x) / zoom - NODE_CENTER_OFFSET.x,
      y: (rect.height / 2 - flowViewport.y) / zoom - NODE_CENTER_OFFSET.y,
    };
  }

  function handleAddNode(nodeType) {
    // Small jitter so repeated clicks don't stack perfectly on one spot.
    const center = viewportCenterInFlowCoords();
    const position = center
      ? {
          x: center.x + (Math.random() - 0.5) * 60,
          y: center.y + (Math.random() - 0.5) * 60,
        }
      : null;
    const newNode = flowStore.addNode(nodeType, position);
    flowStore.selectNode(newNode.id);
  }

  function handleClearSelection() {
    flowStore.clearSelection();
  }

  async function doSave() {
    flowStore.setSaving(true);
    try {
      // Sync any in-flight drag positions before serialising.
      nodes.forEach(node => {
        flowStore.updateNodePosition(node.id, node.position);
      });
      const apiData = flowStore.toApiFormat(action);
      await onSave?.(apiData);
    } catch (err) {
      errorToast(err?.message || String(err), saveErrorMessage);
      console.error(err);
    } finally {
      flowStore.setSaving(false);
    }
  }

  function handleDeleteNode() {
    if (selectedNode && selectedNode.type !== 'trigger') {
      flowStore.removeNode(selectedNode.id);
    }
  }
</script>

<div class="flex h-full action-flow-editor">
  <!-- Node Palette -->
  <div class="w-64 sidebar border-r flex flex-col py-4 overflow-y-auto flex-shrink-0">
    <div class="px-4 mb-4 pb-4 border-b" style="border-color: var(--ds-border);">
      <div class="flex items-center gap-3">
        <div class="flex items-center justify-center w-10 h-10 flex-shrink-0">
          <div class="w-8 h-8 rounded-md flex items-center justify-center bg-amber-500">
            <Zap size={18} color="white" />
          </div>
        </div>
        <span class="font-medium text-sm" style="color: var(--ds-text);">{sidebarTitle}</span>
      </div>
    </div>

    {#if sidebarTop}
      <div class="px-4 mb-4 pb-4 border-b" style="border-color: var(--ds-border);">
        {@render sidebarTop()}
      </div>
    {/if}

    <div class="px-4">
      <h3 class="text-sm font-medium sidebar-title mb-3">{addNodesLabel}</h3>
      <div class="space-y-2">
        {#each nodePalette as item}
          <button
            class="w-full px-3 py-2 text-left rounded-lg text-sm font-medium flex items-center gap-2 node-palette-item cursor-pointer"
            onclick={() => handleAddNode(item.type)}
            data-testid={`action-palette-${item.type}`}
          >
            <svelte:component this={item.icon} size={16} class="flex-shrink-0" />
            <span>{item.label}</span>
          </button>
        {/each}
      </div>

      <div class="mt-6 pt-4 border-t">
        <h4 class="text-xs font-medium sidebar-subtitle mb-2">{tipsLabel}</h4>
        <ul class="text-xs space-y-1 sidebar-hints">
          {#each tips as tip}
            <li>{@html tip}</li>
          {/each}
        </ul>
        {#if sidebarExtra}
          {@render sidebarExtra()}
        {/if}
      </div>
    </div>
  </div>

  <!-- Svelte Flow Canvas -->
  <div class="flex-1 relative" bind:this={flowContainer}>
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
      onmove={handleMove}
      {isValidConnection}
      {...flowOptions}
      fitView
      class="action-flow"
    >
      <Controls />
      <MiniMap
        class={minimapClass}
        nodeColor={minimapNodeColor}
        nodeStrokeColor={minimapNodeStrokeColor}
        nodeStrokeWidth={minimapNodeStrokeWidth}
        nodeBorderRadius={minimapNodeBorderRadius}
        maskColor={minimapMaskColor}
      />
      <Background variant="dots" gap={20} size={1} />
    </SvelteFlow>

    <!-- Save/Cancel buttons overlay -->
    <div class="absolute top-4 right-4 flex gap-2 z-10">
      <Button variant="default" onclick={onCancel} disabled={saving} dataTestid="action-editor-cancel" {...cancelButtonProps}>
        {cancelLabel}
      </Button>
      <Button variant="primary" onclick={doSave} disabled={saving} loading={saving} dataTestid="action-editor-save" {...saveButtonProps}>
        {saving ? savingLabel : saveLabel}
      </Button>
    </div>

    <!-- Action info header -->
    <div class="absolute top-4 left-4 z-10 flex items-start gap-2">
      <div class="action-header px-3 py-2 rounded-lg border">
        <div class="text-sm font-medium">{action?.name || newActionLabel}</div>
        <div class="text-xs sidebar-subtitle">
          {triggerTypes.find(tt => tt.value === action?.trigger_type)?.label || action?.trigger_type}
        </div>
      </div>
      <button
        class="direction-toggle rounded-lg border p-2"
        onclick={() => flowStore.toggleDirection()}
        title={flowStore.direction === 'horizontal' ? switchToVerticalLabel : switchToHorizontalLabel}
      >
        {#if flowStore.direction === 'horizontal'}
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
        <h3 class="text-sm font-medium sidebar-title">{nodeConfigLabel}</h3>
        <button
          class="text-sm text-gray-500 hover:text-gray-700"
          onclick={handleClearSelection}
        >
          &times;
        </button>
      </div>

      <div class="space-y-4">
        {#if selectedNode.type === 'trigger' && triggerConfig}
          {@render triggerConfig(selectedNode, flowStore)}
        {:else if selectedNode.type !== 'trigger' && nodeConfig}
          {@render nodeConfig(selectedNode, flowStore, handleDeleteNode)}
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

  .sidebar-hints :global(code) {
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
    background-color: var(--ds-background-neutral-hovered);
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
    background-color: var(--ds-background-neutral-hovered);
    color: var(--ds-text);
  }

  :global(.action-flow-editor .config-input) {
    background-color: var(--ds-surface);
    border-color: var(--ds-border);
    color: var(--ds-text);
  }

  :global(.action-flow-editor .config-input:focus) {
    border-color: var(--ds-interactive);
    outline: none;
  }

  :global(.action-flow-editor .checkbox-label) {
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
    background-color: var(--ds-background-neutral-hovered);
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
