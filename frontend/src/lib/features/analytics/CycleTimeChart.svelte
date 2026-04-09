<script>
  import { t } from '../../stores/i18n.svelte.js';
  import { useResizeObserver } from 'runed';
  import Text from '../../components/Text.svelte';

  let { data = null } = $props();

  const padding = { top: 16, right: 24, bottom: 16, left: 120 };
  const barColors = ['#3b82f6', '#8b5cf6', '#06b6d4'];

  let container = $state(null);
  let width = $state(600);

  useResizeObserver(() => container, (entries) => {
    const entry = entries[0];
    if (entry) width = entry.contentRect.width;
  });

  const stages = $derived(data?.stages || []);
  const totalCycleTime = $derived(data?.total_cycle_time || {});
  const totalAnalyzed = $derived(data?.total_items_analyzed || 0);

  const chartWidth = $derived(Math.max(width - padding.left - padding.right, 100));
  const barHeight = 24;
  const barGap = 8;
  const chartHeight = $derived(stages.length * (barHeight + barGap));
  const svgWidth = $derived(chartWidth + padding.left + padding.right);
  const svgHeight = $derived(chartHeight + padding.top + padding.bottom);

  const maxHours = $derived.by(() => {
    if (!stages.length) return 1;
    return Math.max(...stages.map(s => s.p85_hours || s.avg_hours || 0), 1);
  });

  function formatHours(h) {
    if (h < 1) return `${Math.round(h * 60)}m`;
    if (h < 24) return `${h.toFixed(1)}h`;
    return `${(h / 24).toFixed(1)}d`;
  }

  let tooltip = $state(null);
</script>

{#if data && stages.length > 0}
  <div class="cycle-time-chart">
    <div class="mb-3">
      <Text variant="subtle" size="xs">{totalAnalyzed} {t('analytics.cycleTime.itemsAnalyzed')}</Text>
    </div>

    <!-- Summary card -->
    <div class="flex gap-4 mb-4 p-3 rounded-lg" style="background: var(--ds-surface-sunken);">
      <div class="text-center">
        <Text variant="subtle" size="xs">{t('analytics.cycleTime.avgTotal')}</Text>
        <div class="text-lg font-semibold" style="color: var(--ds-text);">{formatHours(totalCycleTime.avg_hours || 0)}</div>
      </div>
      <div class="text-center">
        <Text variant="subtle" size="xs">{t('analytics.cycleTime.median')}</Text>
        <div class="text-lg font-semibold" style="color: var(--ds-text);">{formatHours(totalCycleTime.median_hours || 0)}</div>
      </div>
      <div class="text-center">
        <Text variant="subtle" size="xs">{t('analytics.cycleTime.p85')}</Text>
        <div class="text-lg font-semibold" style="color: var(--ds-text);">{formatHours(totalCycleTime.p85_hours || 0)}</div>
      </div>
    </div>

    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div bind:this={container} onmouseleave={() => tooltip = null}>
      <svg viewBox={`0 0 ${svgWidth} ${svgHeight}`} style="height:{svgHeight}px; width:100%;">
        {#each stages as stage, i}
          {@const y = padding.top + i * (barHeight + barGap)}
          {@const avgW = (stage.avg_hours / maxHours) * chartWidth}
          {@const medW = (stage.median_hours / maxHours) * chartWidth}
          {@const p85W = (stage.p85_hours / maxHours) * chartWidth}

          <!-- Stage name -->
          <text x={padding.left - 8} y={y + barHeight / 2 + 4} text-anchor="end" font-size="11" fill="var(--ds-text-subtle)">
            {stage.name.length > 16 ? stage.name.slice(0, 15) + '…' : stage.name}
          </text>

          <!-- P85 bar (background, widest) -->
          <rect x={padding.left} y={y} width={Math.max(p85W, 2)} height={barHeight}
            rx="4" fill={barColors[2]} opacity="0.3"
            role="img"
            onmouseenter={() => tooltip = { stage: stage.name, avg: stage.avg_hours, median: stage.median_hours, p85: stage.p85_hours, x: padding.left + p85W, y: y }}
          />

          <!-- Average bar -->
          <rect x={padding.left} y={y + 4} width={Math.max(avgW, 2)} height={barHeight - 8}
            rx="3" fill={barColors[0]} opacity="0.85" />

          <!-- Value label -->
          <text x={padding.left + Math.max(p85W, 2) + 6} y={y + barHeight / 2 + 4} font-size="10" fill="var(--ds-text-subtle)">
            {formatHours(stage.avg_hours)}
          </text>
        {/each}

        <!-- Tooltip -->
        {#if tooltip}
          {@const tx = Math.min(tooltip.x + 10, svgWidth - 130)}
          {@const ty = Math.max(tooltip.y - 10, 0)}
          <rect x={tx} y={ty} width="120" height="55" rx="6"
            fill="var(--ds-surface-overlay)" stroke="var(--ds-border)" stroke-width="1" />
          <text x={tx + 60} y={ty + 15} text-anchor="middle" font-size="11" font-weight="600" fill="var(--ds-text)">
            {tooltip.stage}
          </text>
          <text x={tx + 60} y={ty + 30} text-anchor="middle" font-size="10" fill="var(--ds-text-subtle)">
            avg {formatHours(tooltip.avg)} · med {formatHours(tooltip.median)}
          </text>
          <text x={tx + 60} y={ty + 45} text-anchor="middle" font-size="10" fill="var(--ds-text-subtle)">
            p85 {formatHours(tooltip.p85)}
          </text>
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
