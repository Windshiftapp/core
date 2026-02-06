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
  import StatusNode from './StatusNode.svelte';
  import ReconnectableEdge from './ReconnectableEdge.svelte';
  import {
    statusesToNodes,
    transitionsToEdges,
    nodesToStatuses,
    edgesToTransitions,
    createEdge,
    addPreservationTransitions,
    positionPersistence,
    DEFAULT_WORKFLOW_POSITIONS
  } from '../../utils/dataTransformers.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { useEventListener } from 'runed';

  let { workflow, statuses = [], onSave, onCancel } = $props();

  // Local state
  let nodes = $state([]);
  let edges = $state([]);
  let workflowStatuses = $state([]);
  let workingTransitions = $state([]);
  let savingTransitions = $state(false);
  let initialStatusId = $state(null);

  // Node and edge types configuration
  const nodeTypes = {
    status: StatusNode
  };

  const edgeTypes = {
    reconnectable: ReconnectableEdge
  };

  // Flow options
  const flowOptions = {
    connectionMode: ConnectionMode.Loose,
    attributionPosition: 'bottom-left',
    defaultViewport: { x: 0, y: 0, zoom: 0.7 },
    minZoom: 0.2,
    maxZoom: 1.5,
    defaultEdgeOptions: {
      type: 'reconnectable'
    }
  };

  useEventListener(() => window, 'workflow-edge-swap', (event) => handleEdgeSwap(event));
  useEventListener(() => window, 'workflow-set-initial', (event) => handleSetInitial(event.detail?.statusId));
  useEventListener(() => window, 'workflow-status-remove', (event) => onStatusRemove(event));

  $effect(() => {
    const w = workflow;
    const s = statuses;
    if (w && s.length > 0) {
      untrack(() => loadWorkflowData());
    }
  });

  function loadWorkflowData() {
    // Build list of statuses currently in workflow
    const statusIds = new Set();
    workflow.transitions?.forEach(t => {
      if (t.from_status_id) statusIds.add(t.from_status_id);
      statusIds.add(t.to_status_id);
    });

    // Load saved positions
    const savedPositions = positionPersistence.load(workflow.id);

    workflowStatuses = Array.from(statusIds).map(id => {
      const status = statuses.find(s => s.id === id);
      const savedPos = savedPositions[id];
      // Use saved position, or default position for standard workflow, or random
      const defaultPos = DEFAULT_WORKFLOW_POSITIONS[status?.name];
      return {
        ...status,
        x: savedPos?.x ?? defaultPos?.x ?? Math.random() * 600 + 100,
        y: savedPos?.y ?? defaultPos?.y ?? Math.random() * 400 + 100
      };
    }).filter(Boolean);

    // Detect initial status from transition where from_status_id is NULL
    const initialTransition = (workflow.transitions || []).find(t => t.from_status_id === null);
    initialStatusId = initialTransition?.to_status_id || null;

    // Load existing transitions (exclude self-preservation transitions)
    workingTransitions = workflow.transitions?.filter(t => 
      t.from_status_id !== null && t.from_status_id !== t.to_status_id
    ) || [];

    // Convert to Svelte Flow format
    nodes = statusesToNodes(workflowStatuses, initialStatusId);
    edges = calculateEdgeOffsets(transitionsToEdges(workingTransitions));

    // If no initial status set, default to first node (if any)
    if (!initialStatusId && nodes.length > 0) {
      initialStatusId = nodes[0].data.statusId;
      nodes = markInitialNode(nodes, initialStatusId);
    }
  }

  function onConnect(params) {
    // Auto-correct direction based on handle types (source vs target)
    let { source, target, sourceHandle, targetHandle } = params;
    const fromIsTarget = sourceHandle?.startsWith('target') || sourceHandle === 'target';
    const toIsSource = targetHandle?.startsWith('target') === false && targetHandle !== undefined && targetHandle !== null;

    // If drag started on a target handle and ended on a source handle, swap to keep flow direction intuitive
    if (fromIsTarget && toIsSource) {
      [source, target] = [target, source];
      [sourceHandle, targetHandle] = [targetHandle, sourceHandle];
    }

    // Normalize handles to match actual handle IDs on nodes
    const normalizeSourceHandle = (handle) => (handle ? handle.replace(/^target-/, '') : handle);
    const normalizeTargetHandle = (handle) => {
      if (!handle) return handle;
      return handle.startsWith('target-') ? handle : `target-${handle}`;
    };

    const finalSourceHandle = normalizeSourceHandle(sourceHandle);
    const finalTargetHandle = normalizeTargetHandle(targetHandle);

    const fromStatusId = parseInt(source.replace('status-', ''));
    const toStatusId = parseInt(target.replace('status-', ''));

    // Check if transition already exists
    const existingEdge = edges.find(edge =>
      edge.source === source && edge.target === target
    );

    if (!existingEdge) {
      const newEdge = {
        ...createEdge(fromStatusId, toStatusId, workflow.id),
        sourceHandle: finalSourceHandle,
        targetHandle: finalTargetHandle
      };
      edges = calculateEdgeOffsets(addEdge(newEdge, edges));
    }
  }

  function onNodesChange(event) {
    // With bind:nodes, the changes should be handled automatically
    // This is just for debugging
  }

  function onEdgesChange(event) {
    const changes = event.detail;
    edges = edges.filter(edge => {
      const deleteChange = changes.find(c => c.id === edge.id && c.type === 'remove');
      return !deleteChange;
    });
    // Recalculate offsets after edge changes
    edges = calculateEdgeOffsets(edges);
  }

  function handleEdgeSwap(event) {
    const edgeId = event.detail?.id;
    if (!edgeId) return;

    edges = calculateEdgeOffsets(
      edges.map((edge) => {
        const edgeMatches =
          edge.id === edgeId ||
          edge.data?.transitionId === edgeId ||
          edge.data?.id === edgeId;

        if (!edgeMatches) {
          return edge;
        }

        const fromId = edge.data?.from_status_id ?? parseInt(edge.source.replace('status-', ''));
        const toId = edge.data?.to_status_id ?? parseInt(edge.target.replace('status-', ''));

        return {
          ...edge,
          source: edge.target,
          target: edge.source,
          sourceHandle: edge.targetHandle ? edge.targetHandle.replace(/^target-/, '') : edge.targetHandle,
          targetHandle: edge.sourceHandle ? (edge.sourceHandle.startsWith('target-') ? edge.sourceHandle : `target-${edge.sourceHandle}`) : edge.sourceHandle,
          data: {
            ...edge.data,
            from_status_id: toId,
            to_status_id: fromId
          }
        };
      })
    );
  }

  /**
   * Calculate offsets for edges that share the same source or target handle
   * so they don't overlap visually
   */
  function calculateEdgeOffsets(edgeList) {
    const OFFSET_STEP = 12; // pixels between parallel edges

    // Group edges by their connection points
    const sourceGroups = {}; // key: "nodeId-handle" -> array of edge indices
    const targetGroups = {};

    edgeList.forEach((edge, index) => {
      const sourceKey = `${edge.source}-${edge.sourceHandle}`;
      const targetKey = `${edge.target}-${edge.targetHandle}`;

      if (!sourceGroups[sourceKey]) sourceGroups[sourceKey] = [];
      if (!targetGroups[targetKey]) targetGroups[targetKey] = [];

      sourceGroups[sourceKey].push(index);
      targetGroups[targetKey].push(index);
    });

    // Calculate offsets for each edge
    return edgeList.map((edge, index) => {
      const sourceKey = `${edge.source}-${edge.sourceHandle}`;
      const targetKey = `${edge.target}-${edge.targetHandle}`;

      const sourceGroup = sourceGroups[sourceKey];
      const targetGroup = targetGroups[targetKey];

      // Use the larger group to determine offset
      const group = sourceGroup.length >= targetGroup.length ? sourceGroup : targetGroup;

      if (group.length <= 1) {
        // No offset needed for single edges
        return { ...edge, data: { ...edge.data, offset: 0 } };
      }

      // Calculate offset: center the group around 0
      const groupIndex = group.indexOf(index);
      const totalWidth = (group.length - 1) * OFFSET_STEP;
      const offset = (groupIndex * OFFSET_STEP) - (totalWidth / 2);

      return { ...edge, data: { ...edge.data, offset } };
    });
  }


  function addStatusToWorkflow(status) {
    // Check if status is already in workflow
    const existingNode = nodes.find(node => node.data.statusId === status.id);
    if (existingNode) return;

    const newNode = {
      id: `status-${status.id}`,
      type: 'status',
      position: { 
        x: Math.random() * 600 + 100, 
        y: Math.random() * 400 + 100 
      },
      data: {
        statusId: status.id,
        name: status.name,
        category_color: status.category_color,
        category_name: status.category_name,
        description: status.description
      }
    };

    nodes = [...nodes, newNode];
    
    // Set initial automatically if none exists
    if (!initialStatusId) {
      initialStatusId = status.id;
      nodes = markInitialNode(nodes, initialStatusId);
    }
    
    // Save positions
    positionPersistence.save(workflow.id, nodes);
  }

  function removeStatusFromWorkflow(statusId) {
    // Remove node
    nodes = nodes.filter(node => node.data.statusId !== statusId);
    
    // Remove all edges involving this status
    edges = edges.filter(edge => 
      edge.data.from_status_id !== statusId && 
      edge.data.to_status_id !== statusId
    );

    // If we removed the initial status, choose a new one
    if (initialStatusId === statusId) {
      initialStatusId = nodes[0]?.data.statusId || null;
      nodes = markInitialNode(nodes, initialStatusId);
    }
  }

  function onStatusRemove(event) {
    removeStatusFromWorkflow(event.detail.statusId);
  }

  async function saveWorkflowDesign() {
    if (!workflow) return;

    try {
      savingTransitions = true;

      // Convert Svelte Flow data back to API format
      const currentStatuses = nodesToStatuses(nodes);
      const currentTransitions = edgesToTransitions(edges, workflow.id);

      // Ensure there is a single initial transition (from_status_id NULL)
      const transitionsWithInitial = addInitialTransition(
        currentTransitions,
        initialStatusId || nodes[0]?.data.statusId || null,
        workflow.id
      );

      // Add preservation transitions for disconnected statuses
      const allTransitions = addPreservationTransitions(
        currentStatuses, 
        transitionsWithInitial, 
        workflow.id
      );

      await onSave(allTransitions);
    } catch (error) {
      console.error('Failed to save workflow design:', error);
      alert(t('workflows.failedToSaveDesign') + ': ' + (error.message || error));
    } finally {
      savingTransitions = false;
    }
  }

  // Filter statuses not already in workflow
  let availableStatuses = $derived(statuses.filter(status =>
    !nodes.some(node => node.data.statusId === status.id)
  ));

  // Watch for node count changes and save positions to localStorage
  $effect(() => {
    const nodeCount = nodes.length;
    const wfId = workflow?.id;
    if (nodeCount > 0 && wfId) {
      const timeout = setTimeout(() => {
        untrack(() => positionPersistence.save(wfId, nodes));
      }, 500);
      return () => clearTimeout(timeout);
    }
  });

  function onNodeDragStop() {
    if (workflow?.id) {
      positionPersistence.save(workflow.id, nodes);
    }
  }

  function markInitialNode(nodeList, statusId) {
    return nodeList.map((node) => ({
      ...node,
      data: {
        ...node.data,
        initial: node.data.statusId === statusId
      }
    }));
  }

  function handleSetInitial(statusId) {
    if (!statusId) return;
    initialStatusId = statusId;
    nodes = markInitialNode(nodes, statusId);
  }

  function addInitialTransition(transitions, statusId, workflowId) {
    if (!statusId) return transitions;
    // Remove existing null-from transitions
    const filtered = transitions.filter((t) => t.from_status_id !== null);
    return [
      {
        workflow_id: workflowId,
        from_status_id: null,
        to_status_id: statusId,
        display_order: -1
      },
      ...filtered
    ];
  }
</script>

<div class="flex h-full">
  <!-- Status Palette -->
  <div class="w-80 workflow-sidebar border-r p-6 overflow-y-auto flex-shrink-0">
    <h3 class="text-lg font-medium sidebar-title mb-2">{t('workflows.availableStatuses')}</h3>
    <div class="mb-4 p-3 sidebar-hint border rounded-md">
      <div class="text-xs font-medium hint-heading">{t('workflows.transitionHintTitle')}</div>
      <div class="text-xs hint-body">
        {t('workflows.transitionHint1')}<br/>
        {t('workflows.transitionHint2')}<br/>
        {t('workflows.transitionHint3')}
      </div>
    </div>
    <div class="space-y-3">
      {#each availableStatuses as status}
        <div
          class="p-3 rounded border status-card cursor-pointer transition-colors"
          onclick={() => addStatusToWorkflow(status)}
        >
          <div class="flex items-center gap-3">
            <div 
              class="w-4 h-4 rounded border-2 status-dot"
              style="background-color: {status.category_color};"
            ></div>
            <div class="flex-1">
              <div class="font-medium text-sm status-title">{status.name}</div>
              <div class="text-xs status-subtitle">{status.category_name}</div>
            </div>
          </div>
        </div>
      {/each}
      
      {#if availableStatuses.length === 0}
        <div class="text-center py-8 status-empty">
          <p class="text-sm">{t('workflows.allStatusesAdded')}</p>
        </div>
      {/if}
    </div>

    <!-- Status Count Info -->
    <div class="mt-6 pt-4 border-t sidebar-footer">
      <div class="text-sm sidebar-counts">
        <div>{t('workflows.statusesInWorkflow', { count: nodes.length })}</div>
        <div>{t('workflows.transitionsDefined', { count: edges.length })}</div>
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
      {onConnect}
      onnodeschange={onNodesChange}
      onedgeschange={onEdgesChange}
      onnodedragstop={onNodeDragStop}
      {...flowOptions}
      fitView
      class="workflow-flow"
    >
      <Controls />
      <MiniMap nodeColor="var(--workflow-minimap-node, #e2e8f0)" maskColor="var(--workflow-minimap-mask, rgba(0, 0, 0, 0.2))" />
      <Background variant="dots" gap={20} size={1} />
      
      <!-- SVG marker definitions for arrowheads -->
      <svg style="position: absolute; top: 0; left: 0; width: 0; height: 0;">
        <defs>
          <marker
            id="workflow-arrowhead"
            markerWidth="4"
            markerHeight="4"
            refX="3.5"
            refY="2"
            orient="auto"
            markerUnits="strokeWidth"
          >
            <polygon
              points="0,0 4,2 0,4 1,2"
              fill="var(--workflow-edge-stroke, #d1d5db)"
              stroke="var(--workflow-edge-stroke, #d1d5db)"
              stroke-width="1"
            />
          </marker>
        </defs>
      </svg>
    </SvelteFlow>
    
    <!-- Instructions overlay when empty -->
    {#if nodes.length === 0}
      <div class="absolute inset-0 flex items-center justify-center pointer-events-none">
        <div class="text-center workflow-hint">
          <p class="text-lg mb-2">{t('workflows.startDesigning')}</p>
          <p class="text-sm">{t('workflows.clickStatusesToAdd')}</p>
          <p class="text-sm">{t('workflows.connectByDragging')}</p>
        </div>
      </div>
    {/if}

    <!-- Save/Cancel buttons overlay -->
    <div class="absolute top-4 right-4 flex gap-2 z-10">
      <button
        class="px-4 py-2 text-sm font-medium border rounded-md workflow-button-secondary focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500"
        onclick={onCancel}
        disabled={savingTransitions}
      >
        {t('common.cancel')}
      </button>
      <button
        class="px-4 py-2 text-sm font-medium text-white bg-blue-600 border border-transparent rounded-md hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500 disabled:opacity-50"
        onclick={saveWorkflowDesign}
        disabled={savingTransitions || nodes.length === 0}
      >
        {savingTransitions ? t('common.saving') : t('workflows.saveWorkflow')}
      </button>
    </div>
  </div>
</div>

<style>
  :global(.workflow-theme) {
    --workflow-surface: var(--ds-surface);
    --workflow-panel: var(--ds-surface-raised);
    --workflow-panel-hover: var(--ds-surface-hovered);
    --workflow-text: var(--ds-text);
    --workflow-text-subtle: var(--ds-text-subtle);
    --workflow-border: var(--ds-border);
    --workflow-edge-stroke: var(--ds-border-bold);
    --workflow-accent: var(--ds-interactive);
    --workflow-accent-strong: var(--ds-interactive-hovered);
    --workflow-grid: var(--ds-background-neutral);
    --workflow-minimap-bg: var(--ds-surface-raised);
    --workflow-minimap-mask: var(--ds-surface-overlay);
    --workflow-minimap-node: var(--ds-border);
    --workflow-hint-bg: var(--ds-accent-blue-subtle);
    --workflow-hint-border: var(--ds-accent-blue-subtler);
  }

  .workflow-sidebar {
    background-color: var(--workflow-panel);
    border-color: var(--workflow-border);
    color: var(--workflow-text);
  }

  .sidebar-title {
    color: var(--workflow-text);
  }

  .sidebar-hint {
    background-color: var(--workflow-hint-bg);
    border-color: var(--workflow-hint-border);
  }

  .hint-heading {
    color: var(--workflow-accent);
  }

  .hint-body {
    color: var(--workflow-text-subtle);
  }

  .status-card {
    background-color: var(--workflow-panel);
    border-color: var(--workflow-border);
  }

  .status-card:hover {
    border-color: var(--workflow-accent);
    box-shadow: var(--shadow-md);
  }

  .status-title {
    color: var(--workflow-text);
  }

  .status-subtitle,
  .status-empty {
    color: var(--workflow-text-subtle);
  }

  .status-dot {
    border-color: var(--workflow-border);
  }

  .sidebar-footer {
    border-color: var(--workflow-border);
  }

  .sidebar-counts {
    color: var(--workflow-text-subtle);
  }

  .workflow-hint {
    color: var(--workflow-text-subtle);
  }

  .workflow-button-secondary {
    color: var(--workflow-text);
    background-color: var(--workflow-panel);
    border-color: var(--workflow-border);
  }

  .workflow-button-secondary:hover:enabled {
    background-color: var(--workflow-panel-hover);
  }

  :global(.workflow-flow) {
    background-color: var(--workflow-surface);
    --xy-background-color: var(--workflow-surface);
    --xy-grid-color: var(--workflow-grid);
    --xy-minimap-background: var(--workflow-minimap-bg);
    --xy-minimap-mask: var(--workflow-minimap-mask);
  }

  :global(.workflow-flow .svelte-flow__background) {
    background-color: var(--workflow-surface);
  }

  :global(.workflow-flow .svelte-flow__edge-path) {
    stroke: var(--workflow-edge-stroke);
    stroke-width: 1;
  }

  :global(.workflow-flow .svelte-flow__edge.selected .svelte-flow__edge-path) {
    stroke: var(--workflow-accent);
    stroke-width: 1.5;
  }

  :global(.workflow-flow .svelte-flow__controls button) {
    background-color: var(--workflow-panel);
    color: var(--workflow-text);
    border: 1px solid var(--workflow-border);
  }

  :global(.workflow-flow .svelte-flow__controls button:hover) {
    background-color: var(--workflow-panel-hover);
  }

  :global(.workflow-flow .svelte-flow__minimap) {
    background-color: var(--workflow-minimap-bg);
    border: 1px solid var(--workflow-border);
  }

  :global(.workflow-flow .svelte-flow__attribution) {
    background-color: var(--workflow-panel);
    color: var(--workflow-text-subtle);
    border: 1px solid var(--workflow-border);
    border-radius: 6px;
    padding: 2px 6px;
  }

  :global(.workflow-flow .svelte-flow__attribution a) {
    color: var(--workflow-accent);
  }
</style>
