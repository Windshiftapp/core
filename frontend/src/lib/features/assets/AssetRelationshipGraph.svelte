<script>
  import {
    SvelteFlow,
    Controls,
    MiniMap,
    Background,
  } from '@xyflow/svelte';
  import '@xyflow/svelte/dist/style.css';
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import RelationshipNode from './RelationshipNode.svelte';
  import ItemDetail from '../items/ItemDetail.svelte';
  import { api } from '../../api.js';
  import { navigate } from '../../router.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { IconAlertTriangle } from '@tabler/icons-svelte-runes';

  let { isOpen = $bindable(false), assetId } = $props();

  let nodes = $state.raw([]);
  let edges = $state([]);
  let loading = $state(false);
  let error = $state(null);
  let truncated = $state(false);
  let selectedItemId = $state(null);
  let selectedItemWorkspaceId = $state(null);
  let showItemModal = $state(false);

  const nodeTypes = { relationship: RelationshipNode };

  function layoutGraph(graphNodes) {
    const nodeWidth = 180;
    const nodeHeight = 52;
    const nodeSep = 40;
    const rankSep = 120;

    // Group nodes by hop level
    const columns = new Map();
    for (const node of graphNodes) {
      const hop = node.data.hop ?? 0;
      if (!columns.has(hop)) columns.set(hop, []);
      columns.get(hop).push(node);
    }

    // Find tallest column for vertical centering
    let maxColumnHeight = 0;
    for (const col of columns.values()) {
      const h = col.length * (nodeHeight + nodeSep) - nodeSep;
      if (h > maxColumnHeight) maxColumnHeight = h;
    }

    return graphNodes.map(node => {
      const hop = node.data.hop ?? 0;
      const col = columns.get(hop);
      const colHeight = col.length * (nodeHeight + nodeSep) - nodeSep;
      const yOffset = (maxColumnHeight - colHeight) / 2;
      const index = col.indexOf(node);
      return {
        ...node,
        position: {
          x: hop * (nodeWidth + rankSep),
          y: yOffset + index * (nodeHeight + nodeSep),
        },
      };
    });
  }

  async function loadGraph() {
    if (!assetId) return;
    loading = true;
    error = null;
    try {
      const data = await api.assets.getRelationshipGraph(assetId);
      truncated = data.truncated;

      const flowEdges = (data.edges || []).map(e => ({
        id: e.id,
        source: e.source,
        target: e.target,
        label: e.label,
        style: e.edge_type === 'field_reference'
          ? 'stroke-dasharray: 5 5; stroke: #a855f7;'
          : e.color ? `stroke: ${e.color};` : '',
        animated: e.edge_type === 'field_reference',
        type: 'default',
      }));

      const flowNodes = (data.nodes || []).map(n => ({
        id: n.id,
        type: 'relationship',
        data: {
          title: n.title,
          type: n.type,
          entity_id: n.entity_id,
          is_origin: n.is_origin,
          hop: n.hop,
          metadata: n.metadata || {},
        },
        position: { x: 0, y: 0 },
      }));

      nodes = layoutGraph(flowNodes);
      edges = flowEdges;
    } catch (e) {
      error = e.message || 'Failed to load graph';
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    if (isOpen && assetId) {
      loadGraph();
    }
  });

  async function handleNodeClick(event) {
    const node = event.detail?.node || event.node;
    if (!node) return;
    const { type, entity_id, is_origin, metadata } = node.data;

    if (type === 'item') {
      let wsId = metadata.workspace_id;
      if (!wsId) {
        try {
          const item = await api.items.get(entity_id);
          wsId = item.workspace_id;
        } catch { return; }
      }
      selectedItemId = entity_id;
      selectedItemWorkspaceId = wsId;
      showItemModal = true;
    } else if (type === 'asset' && !is_origin) {
      isOpen = false;
      navigate('/assets/' + entity_id);
    } else if (type === 'test_case') {
      isOpen = false;
      navigate('/workspaces/' + metadata.workspace_id + '/tests/cases/' + entity_id);
    }
  }
</script>

<Modal bind:isOpen maxWidth="max-w-5xl" maxHeight="80vh" onclose={() => isOpen = false}>
  <ModalHeader title="Relationship Graph" onClose={() => isOpen = false} />
  <div class="graph-container">
    {#if loading}
      <div class="graph-state">{t('common.loading')}</div>
    {:else if error}
      <div class="graph-state" style="color: var(--ds-text-danger, #ef4444);">{error}</div>
    {:else if nodes.length === 0}
      <div class="graph-state" style="color: var(--ds-text-subtle);">No relationships found</div>
    {:else}
      {#if truncated}
        <div class="truncation-banner">
          <IconAlertTriangle size={14} />
          Graph limited to 100 nodes. Some relationships may not be shown.
        </div>
      {/if}
      <div class="flow-wrapper relationship-flow" style="height: {truncated ? 'calc(100% - 36px)' : '100%'};">
        <SvelteFlow
          {nodes}
          {edges}
          {nodeTypes}
          nodesConnectable={false}
          elementsSelectable={true}
          onnodeclick={handleNodeClick}
          nodesDraggable={true}
          fitView
          fitViewOptions={{ padding: 0.3 }}
          minZoom={0.2}
          maxZoom={2}
        >
          <Controls position="bottom-left" />
          <MiniMap
            position="bottom-right"
            pannable
            zoomable
            nodeColor="var(--ds-surface-raised)"
            nodeStrokeColor="var(--ds-border)"
            maskColor="rgba(9, 30, 66, 0.4)"
          />
          <Background gap={16} bgColor="var(--ds-surface)" patternColor="var(--ds-border-subtle)" />
        </SvelteFlow>
      </div>
    {/if}
  </div>
</Modal>

{#if showItemModal && selectedItemId}
  <ItemDetail
    isModal={true}
    itemId={selectedItemId}
    workspaceId={selectedItemWorkspaceId}
    onclose={() => { showItemModal = false; selectedItemId = null; }}
  />
{/if}

<style>
  .graph-container {
    height: 60vh;
    position: relative;
  }

  .graph-state {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100%;
    font-size: 14px;
    color: var(--ds-text-subtle, #6b7280);
  }

  .truncation-banner {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 8px 16px;
    font-size: 12px;
    color: var(--ds-text-warning, #d97706);
    background: var(--ds-surface-warning-subtle, #fffbeb);
    border-bottom: 1px solid var(--ds-border, #e5e7eb);
  }

  .flow-wrapper {
    width: 100%;
  }

  /* Theme Svelte Flow chrome with design-system tokens so the graph tracks
     light/dark mode instead of the library's default bright-white defaults. */
  :global(.relationship-flow) {
    background-color: var(--ds-surface);
  }

  :global(.relationship-flow .svelte-flow__background) {
    background-color: var(--ds-surface);
  }

  :global(.relationship-flow .svelte-flow__controls) {
    box-shadow: none;
    border: 1px solid var(--ds-border);
    border-radius: 6px;
    overflow: hidden;
  }

  :global(.relationship-flow .svelte-flow__controls-button) {
    background-color: var(--ds-surface-raised);
    color: var(--ds-text);
    border-bottom: 1px solid var(--ds-border);
    fill: currentColor;
  }

  :global(.relationship-flow .svelte-flow__controls-button:hover) {
    background-color: var(--ds-background-neutral-hovered);
  }

  :global(.relationship-flow .svelte-flow__controls-button:last-child) {
    border-bottom: none;
  }

  :global(.relationship-flow .svelte-flow__minimap) {
    background-color: var(--ds-surface-raised);
    border: 1px solid var(--ds-border);
    border-radius: 6px;
  }

  :global(.relationship-flow .svelte-flow__attribution) {
    background-color: transparent;
    color: var(--ds-text-subtlest);
  }

  :global(.relationship-flow .svelte-flow__attribution a) {
    color: var(--ds-text-subtle);
  }

  :global(.relationship-flow .svelte-flow__edge-text) {
    fill: var(--ds-text-subtle);
  }

  :global(.relationship-flow .svelte-flow__edge-textbg) {
    fill: var(--ds-surface);
  }
</style>
