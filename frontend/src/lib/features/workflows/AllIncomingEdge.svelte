<script>
  import { BaseEdge } from '@xyflow/svelte';
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
  // on its left side as a squared bracket, so it reads as one special
  // incoming transition instead of one arrow per source status.
  const RISE = 26; // stand-off above the top handle
  const STANDOFF = 24; // horizontal distance from the left handle
  const CORNER = 10; // corner rounding — square with softened joints

  // The loop always leaves the top handle and enters the left handle, which
  // is how createAllIncomingEdge wires it; the bracket below is that shape.
  let loop = $derived.by(() => {
    const topY = sourceY - RISE;
    const leftX = targetX - STANDOFF;
    const r = Math.max(
      2,
      Math.min(CORNER, RISE, STANDOFF, Math.abs(sourceX - leftX) / 2, Math.abs(targetY - topY) / 2)
    );

    // Up from the top handle, left along the top, down past the node's left
    // edge, then right into the left handle. Corners are quadratic curves so
    // the bracket reads square but not sharp.
    const path = [
      `M ${sourceX} ${sourceY}`,
      `L ${sourceX} ${topY + r}`,
      `Q ${sourceX} ${topY} ${sourceX - r} ${topY}`,
      `L ${leftX + r} ${topY}`,
      `Q ${leftX} ${topY} ${leftX} ${topY + r}`,
      `L ${leftX} ${targetY - r}`,
      `Q ${leftX} ${targetY} ${leftX + r} ${targetY}`,
      `L ${targetX} ${targetY}`,
    ].join(' ');

    return { path, labelX: (sourceX + leftX) / 2, labelY: topY };
  });
  let edgePath = $derived(loop.path);
  let labelX = $derived(loop.labelX);
  let labelY = $derived(loop.labelY);
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
