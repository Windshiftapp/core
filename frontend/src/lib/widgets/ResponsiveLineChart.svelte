<script>
  import { useResizeObserver } from 'runed';
  import { t } from '../stores/i18n.svelte.js';

  let {
    chartData = [],
    color = '#10b981',
    emptyMessage = t('widgets.chart.noDataAvailable'),
    gradientPrefix = 'chart',
    minHeight = 110,
    maxHeight = 220,
    valueFormat = null,
    valueSuffix = t('widgets.chart.items'),
    showYAxis = false,
    yAxisFormat = null,
    minValue = null,
    maxValue = null
  } = $props();

  const clamp = (value, min, max) => Math.min(Math.max(value, min), max);
  function formatDate(date) {
    const month = String(date.getMonth() + 1).padStart(2, '0');
    const day = String(date.getDate()).padStart(2, '0');
    return `${month}/${day}`;
  }
  const gradientId = `${gradientPrefix}-${Math.random().toString(36).slice(2, 9)}`;

  let container = $state(null);
  let width = $state(360);

  useResizeObserver(() => container, (entries) => {
    const entry = entries[0];
    if (entry) { width = entry.contentRect.width; }
  });

  const normalizeDate = value => {
    if (value instanceof Date) return value;
    const parsed = new Date(value);
    return Number.isNaN(parsed.getTime()) ? new Date() : parsed;
  };

  function getRatio(index, total) {
    if (total <= 1) return 0.5;
    return index / (total - 1);
  }

  function buildSmoothPath(points) {
    if (points.length < 2) return '';

    let path = `M ${points[0].x} ${points[0].y}`;
    for (let i = 1; i < points.length; i++) {
      const prev = points[i - 1];
      const curr = points[i];
      const controlX = (prev.x + curr.x) / 2;
      path += ` Q ${controlX} ${prev.y} ${curr.x} ${curr.y}`;
    }
    return path;
  }

  let effectivePadding = $derived({ top: 24, right: showYAxis ? 16 : 32, bottom: 24, left: showYAxis ? 48 : 32 });
  let chartWidth = $derived(Math.max(width - (effectivePadding.left + effectivePadding.right), 0));
  let normalizedMinHeight = $derived(Math.min(minHeight, maxHeight));
  let normalizedMaxHeight = $derived(Math.max(minHeight, maxHeight));
  let chartHeight = $derived(clamp(chartWidth * 0.35, normalizedMinHeight, normalizedMaxHeight));
  let svgWidth = $derived(chartWidth + effectivePadding.left + effectivePadding.right);
  let svgHeight = $derived(chartHeight + effectivePadding.top + effectivePadding.bottom);
  let dataMin = $derived(minValue !== null ? minValue : 0);
  let dataMax = $derived(maxValue !== null ? maxValue : Math.max(...chartData.map(d => d.count ?? 0), 1));
  let valueRange = $derived(dataMax - dataMin || 1);
  let points = $derived(chartData.map((d, index) => {
    const ratio = getRatio(index, chartData.length);
    const value = d.count ?? 0;
    const normalizedValue = (value - dataMin) / valueRange;
    return {
      x: effectivePadding.left + chartWidth * ratio,
      y: effectivePadding.top + (chartHeight - normalizedValue * chartHeight)
    };
  }));
  let smoothPath = $derived(points.length > 1 ? buildSmoothPath(points) : '');
  let areaPath = $derived(smoothPath
    ? `${smoothPath} L ${effectivePadding.left + chartWidth} ${effectivePadding.top + chartHeight} L ${effectivePadding.left} ${effectivePadding.top + chartHeight} Z`
    : '');
  let gridLines = $derived(Array.from({ length: 4 }, (_, i) => effectivePadding.top + (i / 3) * chartHeight));
  let yAxisValues = $derived(Array.from({ length: 4 }, (_, i) => dataMax - (i / 3) * valueRange));
  let labels = $derived(chartData.map(point => formatDate(normalizeDate(point.date))));

  let tooltip = $state(null);
  let hoveredPointIndex = $state(null);

  function showTooltip(point, index) {
    const baseWidth = svgWidth || width || 1;
    const baseHeight = svgHeight || (container?.clientHeight ?? 0) || 1;
    const scaleX = (container?.clientWidth || baseWidth) / baseWidth;
    const scaleY = (container?.clientHeight || baseHeight) / baseHeight;
    const tooltipLabel = chartData[index]?.label || labels[index];
    const tooltipCount = chartData[index]?.count ?? 0;
    hoveredPointIndex = index;
    tooltip = {
      label: tooltipLabel,
      count: tooltipCount,
      x: point.x * scaleX,
      y: point.y * scaleY
    };
  }

  function hideTooltip() {
    tooltip = null;
    hoveredPointIndex = null;
  }
</script>

{#if chartData && chartData.length > 0}
  <div class="responsive-chart">
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div class="chart-wrapper" bind:this={container} onmouseleave={hideTooltip}>
      <svg
        class="chart-svg"
        viewBox={`0 0 ${svgWidth} ${svgHeight}`}
        style={`height:${svgHeight}px;`}
      >
        <defs>
          <linearGradient id={gradientId} x1="0%" y1="0%" x2="0%" y2="100%">
            <stop offset="0%" style={`stop-color:${color};stop-opacity:0.35`} />
            <stop offset="100%" style={`stop-color:${color};stop-opacity:0.05`} />
          </linearGradient>
        </defs>

        {#each gridLines as y, i}
          <line
            x1={effectivePadding.left}
            y1={y}
            x2={effectivePadding.left + chartWidth}
            y2={y}
            stroke="#e5e7eb"
            stroke-width="1"
            stroke-dasharray="3,3"
          />
          {#if showYAxis}
            <text
              x={effectivePadding.left - 8}
              y={y + 4}
              text-anchor="end"
              font-size="11"
              fill="#6b7280"
            >
              {yAxisFormat ? yAxisFormat(yAxisValues[i]) : Math.round(yAxisValues[i])}
            </text>
          {/if}
        {/each}

        {#if areaPath}
          <path d={areaPath} fill={`url(#${gradientId})`} opacity="0.8" />
        {/if}

        {#if smoothPath}
          <path d={smoothPath} fill="none" stroke={color} stroke-width="2.5" stroke-linecap="round" />
        {/if}

        {#each points as point, index}
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <circle
            cx={point.x}
            cy={point.y}
            r={hoveredPointIndex === index ? 6 : 4}
            fill={color}
            stroke="white"
            stroke-width="2"
            class="chart-point"
            class:chart-point--active={hoveredPointIndex === index}
            tabindex="-1"
            aria-label={`${labels[index]}: ${chartData[index]?.count ?? 0} items`}
            onmouseenter={() => showTooltip(point, index)}
            onfocus={() => showTooltip(point, index)}
            onmouseleave={hideTooltip}
            onblur={hideTooltip}
          />
        {/each}
      </svg>

      {#if tooltip}
        <div
          class="chart-tooltip"
          style={`left:${tooltip.x}px;top:${tooltip.y}px;`}
        >
          <div class="chart-tooltip-date">{tooltip.label}</div>
          <div class="chart-tooltip-value">
            {#if valueFormat}
              {valueFormat(tooltip.count)}
            {:else}
              {tooltip.count} {tooltip.count === 1 ? valueSuffix.replace(/s$/, '') : valueSuffix}
            {/if}
          </div>
        </div>
      {/if}
    </div>

    <div class="chart-labels" style={`padding:0 ${effectivePadding.left}px;`}>
      {#each labels as label}
        <span>{label}</span>
      {/each}
    </div>
  </div>
{:else}
  <div class="chart-empty">
    <p>{emptyMessage}</p>
  </div>
{/if}

<style>
  .responsive-chart {
    width: 100%;
  }

  .chart-wrapper {
    width: 100%;
    position: relative;
  }

  .chart-svg {
    width: 100%;
    display: block;
  }

  .chart-labels {
    display: flex;
    justify-content: space-between;
    font-size: 0.75rem;
    color: #6b7280;
    margin-top: 0.5rem;
  }

  .chart-labels span {
    white-space: nowrap;
  }

  .chart-empty {
    height: 160px;
    display: flex;
    align-items: center;
    justify-content: center;
    color: #9ca3af;
    font-size: 0.875rem;
  }

  .chart-tooltip {
    position: absolute;
    transform: translate(-50%, calc(-100% - 8px));
    background: white;
    border: 1px solid #e5e7eb;
    border-radius: 0.5rem;
    padding: 0.35rem 0.65rem;
    box-shadow: 0 10px 15px -3px rgba(107, 114, 128, 0.2);
    pointer-events: none;
    font-size: 0.75rem;
    color: #111827;
    min-width: 90px;
    text-align: center;
    z-index: 1;
  }

  .chart-tooltip-date {
    font-weight: 600;
  }

  .chart-tooltip-value {
    color: #6b7280;
  }

  .chart-point {
    transition: r 0.1s ease, stroke-width 0.1s ease;
  }

  .chart-point--active {
    stroke-width: 2.5;
  }
</style>
