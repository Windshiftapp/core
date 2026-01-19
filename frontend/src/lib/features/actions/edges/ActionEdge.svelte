<script>
  import { BaseEdge, getBezierPath } from '@xyflow/svelte';
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

<BaseEdge
  {id}
  path={edgePath}
  {markerEnd}
  style="{style}; stroke: {edgeColor}; stroke-width: 2px;"
/>

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
