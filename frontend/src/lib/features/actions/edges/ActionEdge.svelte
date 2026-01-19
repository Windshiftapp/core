<script>
  import { BaseEdge, EdgeReconnectAnchor, getBezierPath } from '@xyflow/svelte';
  import { t } from '../../../stores/i18n.svelte.js';

  export let id;
  export let sourceX;
  export let sourceY;
  export let targetX;
  export let targetY;
  export let sourcePosition;
  export let targetPosition;
  export let data = {};
  export let markerEnd;
  export let style;
  export let sourceHandleId;
  export let selected = false;

  let reconnecting = false;

  $: [edgePath, labelX, labelY] = getBezierPath({
    sourceX,
    sourceY,
    sourcePosition,
    targetX,
    targetY,
    targetPosition
  });

  $: edgeColor = sourceHandleId === 'true'
    ? 'var(--ds-success)'
    : sourceHandleId === 'false'
      ? 'var(--ds-danger)'
      : 'var(--ds-border-bold)';

  $: showLabel = sourceHandleId === 'true' || sourceHandleId === 'false';
  $: labelText = sourceHandleId === 'true'
    ? t('actions.condition.true')
    : sourceHandleId === 'false'
      ? t('actions.condition.false')
      : '';
</script>

<!-- Hide base edge during reconnection -->
{#if !reconnecting}
  <BaseEdge
    {id}
    path={edgePath}
    {markerEnd}
    style="{style}; stroke: {edgeColor}; stroke-width: 2px;"
  />
{/if}

<!-- Show reconnection anchors when edge is selected -->
{#if selected}
  <EdgeReconnectAnchor
    bind:reconnecting
    type="source"
    position={{ x: sourceX, y: sourceY }}
    style={!reconnecting ? `background: ${edgeColor}; border: 2px solid var(--ds-surface-raised); border-radius: 100%; width: 10px; height: 10px;` : ''}
  />
  <EdgeReconnectAnchor
    bind:reconnecting
    type="target"
    position={{ x: targetX, y: targetY }}
    style={!reconnecting ? `background: ${edgeColor}; border: 2px solid var(--ds-surface-raised); border-radius: 100%; width: 10px; height: 10px;` : ''}
  />
{/if}

{#if showLabel}
  <foreignObject
    x={labelX - 20}
    y={labelY - 10}
    width="40"
    height="20"
    style="overflow: visible; pointer-events: none;"
  >
    <div
      class="edge-label"
      style:background-color={edgeColor}
    >
      {labelText}
    </div>
  </foreignObject>
{/if}

<style>
  .edge-label {
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 10px;
    font-weight: 600;
    color: white;
    padding: 2px 8px;
    border-radius: 10px;
    white-space: nowrap;
  }
</style>
