<script>
  import { BaseEdge, getBezierPath, Position } from '@xyflow/svelte';
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

  // The all-statuses arrow loops out of the top of the status and re-enters
  // on its left side, so it reads as one special incoming transition instead
  // of one arrow per source status.
  const LOOP_GAP = 22;

  let pathResult = $derived(getBezierPath({
    sourceX,
    sourceY: sourceY - LOOP_GAP,
    sourcePosition: sourcePosition || Position.Top,
    targetX: targetX - LOOP_GAP,
    targetY,
    targetPosition: targetPosition || Position.Left,
    curvature: 0.8,
  }));
  let edgePath = $derived(pathResult[0]);
  let labelX = $derived(pathResult[1]);
  let labelY = $derived(pathResult[2]);
</script>

<BaseEdge
  id={id}
  path={edgePath}
  markerEnd="url(#workflow-all-arrowhead)"
  class="all-incoming-path"
  style={`stroke: var(--workflow-accent, #3b82f6); stroke-width: ${selected ? 2 : 1.5}; stroke-dasharray: 5 3; fill: none;`}
/>

<foreignObject x={labelX - 24} y={labelY - 10} width="48" height="20" style="overflow: visible;">
  <div
    class="all-incoming-label"
    class:all-incoming-label-selected={selected}
    data-testid={`workflow-${id}`}
    title={t('workflows.fromAllStatuses')}
  >
    {t('workflows.allStatuses')}
  </div>
</foreignObject>

<style>
  .all-incoming-label {
    width: 48px;
    text-align: center;
    font-size: 9px;
    line-height: 1;
    padding: 3px 0;
    border-radius: 999px;
    border: 1px solid var(--workflow-accent, #3b82f6);
    background: var(--workflow-panel, #fff);
    color: var(--workflow-accent, #3b82f6);
    cursor: default;
    user-select: none;
  }

  .all-incoming-label-selected {
    background: var(--workflow-accent, #3b82f6);
    color: #fff;
  }
</style>
