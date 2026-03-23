<script>
  import {
    SvelteFlow,
    Controls,
    MiniMap,
    Background,
  } from '@xyflow/svelte';
  import '@xyflow/svelte/dist/style.css';
  import dagre from '@dagrejs/dagre';
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import RelationshipNode from './RelationshipNode.svelte';
  import ItemDetail from '../items/ItemDetail.svelte';
  import { api } from '../../api.js';
  import { navigate } from '../../router.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { AlertTriangle } from 'lucide-svelte';

  let { isOpen = $bindable(false), assetId } = $props();

  let nodes = $state([]);
  let edges = $state([]);
  let loading = $state(false);
  let error = $state(null);
  let truncated = $state(false);
  let selectedItemId = $state(null);
  let selectedItemWorkspaceId = $state(null);
  let showItemModal = $state(false);

  const nodeTypes = { relationship: RelationshipNode };

  function layoutGraph(graphNodes, graphEdges) {
    const g = new dagre.graphlib.Graph();
    g.setDefaultEdgeLabel(() => ({}));
    g.setGraph({ rankdir: 'LR', nodesep: 40, ranksep: 120 });

    for (const node of graphNodes) {
      g.setNode(node.id, { width: 180, height: 52 });
    }
    for (const edge of graphEdges) {
      g.setEdge(edge.source, edge.target);
    }

    dagre.layout(g);

    return graphNodes.map(node => {
      const pos = g.node(node.id);
      return { ...node, position: { x: pos.x - 90, y: pos.y - 26 } };
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

      nodes = layoutGraph(flowNodes, flowEdges);
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
          <AlertTriangle size={14} />
          Graph limited to 100 nodes. Some relationships may not be shown.
        </div>
      {/if}
      <div class="flow-wrapper" style="height: {truncated ? 'calc(100% - 36px)' : '100%'};">
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
          <MiniMap position="bottom-right" pannable zoomable />
          <Background gap={16} />
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
</style>
