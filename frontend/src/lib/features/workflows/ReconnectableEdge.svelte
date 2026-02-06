<script>
  import {
    BaseEdge,
    EdgeReconnectAnchor,
    getSmoothStepPath,
    Position
  } from '@xyflow/svelte';
  import { t } from '../../stores/i18n.svelte.js';

  let {
    id,
    sourceX,
    sourceY,
    targetX,
    targetY,
    sourcePosition,
    targetPosition,
    selected = false,
    data = {},
    ...rest
  } = $props();

  // Get offset from edge data (for multiple edges to same handle)
  let offset = $derived(data?.offset || 0);

  // Apply offset perpendicular to the edge direction
  let offsetSourceX = $derived(sourcePosition === 'top' || sourcePosition === 'bottom'
    ? sourceX + offset
    : sourceX);
  let offsetSourceY = $derived(sourcePosition === 'left' || sourcePosition === 'right'
    ? sourceY + offset
    : sourceY);
  let offsetTargetX = $derived(targetPosition === 'top' || targetPosition === 'bottom'
    ? targetX + offset
    : targetX);
  let offsetTargetY = $derived(targetPosition === 'left' || targetPosition === 'right'
    ? targetY + offset
    : targetY);

  // Calculate the smooth step path for rectangular edges with rounded corners
  let pathResult = $derived(getSmoothStepPath({
    sourceX: offsetSourceX,
    sourceY: offsetSourceY,
    sourcePosition: sourcePosition || Position.Right,
    targetX: offsetTargetX,
    targetY: offsetTargetY,
    targetPosition: targetPosition || Position.Left,
    borderRadius: 8
  }));
  let edgePath = $derived(pathResult[0]);
  let labelX = $derived(pathResult[1]);
  let labelY = $derived(pathResult[2]);

  let reconnecting = $state(false);
</script>

<!-- Render the base edge path if not currently reconnecting -->
{#if !reconnecting}
  <BaseEdge
    path={edgePath}
    markerEnd="url(#workflow-arrowhead)"
    style={`stroke: var(--workflow-edge-stroke, #d1d5db); stroke-width: 1;`}
  />
{/if}

<!-- Show reconnection anchors when edge is selected -->
<!-- Source (start) = green, Target (end/arrow) = blue -->
{#if selected}
  <EdgeReconnectAnchor
    bind:reconnecting
    type="source"
    position={{ x: sourceX, y: sourceY }}
    style={!reconnecting ? 'background: rgba(34, 197, 94, 0.9); border: 2px solid var(--workflow-panel); border-radius: 100%; width: 12px; height: 12px;' : ''}
  />
  <EdgeReconnectAnchor
    bind:reconnecting
    type="target"
    position={{ x: targetX, y: targetY }}
    style={!reconnecting ? 'background: rgba(59, 130, 246, 0.9); border: 2px solid var(--workflow-panel); border-radius: 100%; width: 12px; height: 12px;' : ''}
  />
  <foreignObject x={labelX - 14} y={labelY - 14} width="28" height="28" style="overflow: visible;">
    <div class="edge-toolbar">
      <button
        class="edge-swap-btn"
        title={t('workflows.swapDirection')}
        onclick={(evt) => {
          evt.stopPropagation();
          const customId = id || data?.transitionId || data?.id;
          if (customId) {
            window.dispatchEvent(new CustomEvent('workflow-edge-swap', { detail: { id: customId } }));
          }
        }}
      >
        ⇅
      </button>
    </div>
  </foreignObject>
{/if}

<style>
  .edge-toolbar {
    width: 28px;
    height: 28px;
    display: flex;
    align-items: center;
    justify-content: center;
    pointer-events: all;
  }

  .edge-swap-btn {
    width: 24px;
    height: 24px;
    border-radius: 6px;
    border: 1px solid var(--workflow-border);
    background: var(--workflow-panel);
    color: var(--workflow-text);
    font-size: 12px;
    line-height: 1;
    cursor: pointer;
    padding: 0;
    transition: background-color 0.15s ease, color 0.15s ease, border-color 0.15s ease;
  }

  .edge-swap-btn:hover {
    background: var(--workflow-panel-hover);
    color: var(--workflow-accent);
    border-color: var(--workflow-accent);
  }
</style>
