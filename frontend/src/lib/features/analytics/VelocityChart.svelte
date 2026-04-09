<script>
  import { t } from '../../stores/i18n.svelte.js';
  import { useResizeObserver } from 'runed';
  import Text from '../../components/Text.svelte';

  let { data = null } = $props();

  const padding = { top: 32, right: 24, bottom: 64, left: 56 };
  const barColor = '#3b82f6';
  const avgColor = '#f59e0b';

  let container = $state(null);
  let width = $state(600);

  useResizeObserver(() => container, (entries) => {
    const entry = entries[0];
    if (entry) width = entry.contentRect.width;
  });

  const iterations = $derived(data?.iterations || []);
  const averages = $derived(data?.averages || {});
  const chartWidth = $derived(Math.max(width - padding.left - padding.right, 100));
  const chartHeight = $derived(Math.min(Math.max(chartWidth * 0.45, 180), 300));
  const svgWidth = $derived(chartWidth + padding.left + padding.right);
  const svgHeight = $derived(chartHeight + padding.top + padding.bottom);

  const maxValue = $derived.by(() => {
    if (!iterations.length) return 1;
    return Math.max(...iterations.map(i => Math.max(i.completed_count, i.completed_points || 0)), 1);
  });

  const barWidth = $derived.by(() => {
    if (!iterations.length) return 20;
    const available = chartWidth / iterations.length;
    return Math.min(available * 0.6, 48);
  });

  const getX = $derived.by(() => (index) => {
    if (!iterations.length) return padding.left;
    const slot = chartWidth / iterations.length;
    return padding.left + slot * index + slot / 2;
  });

  const getY = $derived.by(() => (value) => {
    return padding.top + chartHeight - (value / maxValue) * chartHeight;
  });

  const gridLines = $derived.by(() => {
    if (maxValue <= 5) {
      // Use integer steps only to avoid duplicate labels
      const steps = maxValue + 1;
      return Array.from({ length: steps }, (_, i) => ({
        y: padding.top + (i / Math.max(steps - 1, 1)) * chartHeight,
        value: maxValue - i
      }));
    }
    const count = 5;
    const seen = new Set();
    return Array.from({ length: count }, (_, i) => ({
      y: padding.top + (i / (count - 1)) * chartHeight,
      value: Math.round(maxValue - (i / (count - 1)) * maxValue)
    })).filter(line => {
      if (seen.has(line.value)) return false;
      seen.add(line.value);
      return true;
    });
  });

  const maxXLabels = 8;
  const xLabelStep = $derived(iterations.length > maxXLabels ? Math.ceil(iterations.length / maxXLabels) : 1);

  const avgY = $derived(getY(averages.avg_count || 0));

  let tooltip = $state(null);

  function showTooltip(iter, index) {
    tooltip = {
      x: getX(index),
      y: getY(iter.completed_count),
      name: iter.name,
      count: iter.completed_count,
      points: iter.completed_points,
      total: iter.total_count
    };
  }
</script>

{#if data && iterations.length > 0}
  <div class="velocity-chart">
    <div class="flex items-center gap-4 text-xs mb-3" style="color: var(--ds-text-subtle);">
      <div class="flex items-center gap-1.5">
        <span class="inline-block w-3 h-3 rounded-sm" style="background: {barColor};"></span>
        {t('analytics.velocity.completed')}
      </div>
      <div class="flex items-center gap-1.5">
        <span class="inline-block w-3 h-0.5" style="background: {avgColor};"></span>
        {t('analytics.velocity.average')}
      </div>
    </div>

    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div bind:this={container} onmouseleave={() => tooltip = null}>
      <svg viewBox={`0 0 ${svgWidth} ${svgHeight}`} style="height:{svgHeight}px; width:100%;">
        <!-- Grid lines -->
        {#each gridLines as line}
          <line x1={padding.left} y1={line.y} x2={padding.left + chartWidth} y2={line.y}
            stroke="var(--ds-border)" stroke-width="1" />
          <text x={padding.left - 8} y={line.y + 4} text-anchor="end" font-size="11" fill="var(--ds-text-subtle)">
            {line.value}
          </text>
        {/each}

        <!-- Bars -->
        {#each iterations as iter, i}
          {@const barH = (iter.completed_count / maxValue) * chartHeight}
          <rect
            x={getX(i) - barWidth / 2}
            y={padding.top + chartHeight - barH}
            width={barWidth}
            height={barH}
            rx="3"
            fill={barColor}
            opacity={tooltip?.name === iter.name ? 1 : 0.85}
            class="cursor-pointer"
            role="img"
            onmouseenter={() => showTooltip(iter, i)}
            onmouseleave={() => tooltip = null}
          />
          <!-- X label -->
          {#if i % xLabelStep === 0}
            {@const lx = getX(i)}
            {@const ly = padding.top + chartHeight + 16}
            <text
              x={lx}
              y={ly}
              text-anchor={iterations.length > maxXLabels ? 'end' : 'middle'}
              font-size="10"
              fill="var(--ds-text-subtle)"
              class="select-none"
              transform={iterations.length > maxXLabels ? `rotate(-45, ${lx}, ${ly})` : undefined}
            >
              {iter.name.length > 12 ? iter.name.slice(0, 11) + '…' : iter.name}
            </text>
          {/if}
        {/each}

        <!-- Average line -->
        {#if averages.avg_count > 0}
          <line
            x1={padding.left}
            y1={avgY}
            x2={padding.left + chartWidth}
            y2={avgY}
            stroke={avgColor}
            stroke-width="1.5"
            stroke-dasharray="6,4"
          />
          <text
            x={padding.left + chartWidth + 4}
            y={avgY + 4}
            font-size="10"
            fill={avgColor}
          >
            {averages.avg_count.toFixed(1)}
          </text>
        {/if}

        <!-- Tooltip -->
        {#if tooltip}
          {@const tx = Math.min(tooltip.x, svgWidth - 120)}
          {@const ty = Math.max(tooltip.y - 60, 8)}
          <g>
            <rect x={tx - 55} y={ty} width="110" height="50" rx="6"
              fill="var(--ds-surface-overlay)" stroke="var(--ds-border)" stroke-width="1" />
            <text x={tx} y={ty + 16} text-anchor="middle" font-size="11" font-weight="600" fill="var(--ds-text)">
              {tooltip.name}
            </text>
            <text x={tx} y={ty + 30} text-anchor="middle" font-size="10" fill="var(--ds-text-subtle)">
              {tooltip.count} items · {tooltip.points ?? 0} pts
            </text>
            <text x={tx} y={ty + 43} text-anchor="middle" font-size="10" fill="var(--ds-text-subtle)">
              {tooltip.total} total
            </text>
          </g>
        {/if}
      </svg>
    </div>
  </div>
{:else}
  <div class="flex flex-col items-center justify-center py-10 gap-2" style="color: var(--ds-text-subtle);">
    {#if data?.data_quality?.reason}
      <Text variant="subtle" size="sm">{t('analytics.insufficientData.' + data.data_quality.reason)}</Text>
    {:else}
      <Text variant="subtle" size="sm">{t('analytics.noData')}</Text>
    {/if}
  </div>
{/if}
