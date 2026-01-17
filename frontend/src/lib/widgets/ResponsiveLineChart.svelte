<script>
  import { onMount } from 'svelte';
  import { timeFormat } from 'd3-time-format';
  import { t } from '../stores/i18n.svelte.js';

  export let chartData = [];
  export let color = '#10b981';
  export let emptyMessage = t('widgets.chart.noDataAvailable');
  export let gradientPrefix = 'chart';
  export let minHeight = 110;
  export let maxHeight = 220;
  export let valueFormat = null; // Function to format tooltip value, e.g., (v) => `${v.toFixed(1)}%`
  export let valueSuffix = t('widgets.chart.items'); // Suffix for tooltip value (used if valueFormat is null)
  export let showYAxis = false; // Show Y-axis labels
  export let yAxisFormat = null; // Function to format Y-axis labels, e.g., (v) => `${v}%`
  export let minValue = null; // Optional minimum value for Y-axis (e.g., 0 for percentages)
  export let maxValue = null; // Optional maximum value for Y-axis (e.g., 100 for percentages)

  const padding = { top: 24, right: showYAxis ? 16 : 32, bottom: 24, left: showYAxis ? 48 : 32 };
  const clamp = (value, min, max) => Math.min(Math.max(value, min), max);
  const formatDate = timeFormat('%m/%d');
  const gradientId = `${gradientPrefix}-${Math.random().toString(36).slice(2, 9)}`;

  let container;
  let width = 360; // fallback before the ResizeObserver runs

  onMount(() => {
    if (!container) return;

    width = container.clientWidth || width;

    const observer = new ResizeObserver(entries => {
      const entry = entries[0];
      if (entry) {
        width = entry.contentRect.width;
      }
    });

    observer.observe(container);
    return () => observer.disconnect();
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

  $: effectivePadding = { top: 24, right: showYAxis ? 16 : 32, bottom: 24, left: showYAxis ? 48 : 32 };
  $: chartWidth = Math.max(width - (effectivePadding.left + effectivePadding.right), 0);
  $: normalizedMinHeight = Math.min(minHeight, maxHeight);
  $: normalizedMaxHeight = Math.max(minHeight, maxHeight);
  $: chartHeight = clamp(chartWidth * 0.35, normalizedMinHeight, normalizedMaxHeight);
  $: svgWidth = chartWidth + effectivePadding.left + effectivePadding.right;
  $: svgHeight = chartHeight + effectivePadding.top + effectivePadding.bottom;
  $: dataMin = minValue !== null ? minValue : 0;
  $: dataMax = maxValue !== null ? maxValue : Math.max(...chartData.map(d => d.count ?? 0), 1);
  $: valueRange = dataMax - dataMin || 1;
  $: points = chartData.map((d, index) => {
    const ratio = getRatio(index, chartData.length);
    const value = d.count ?? 0;
    const normalizedValue = (value - dataMin) / valueRange;
    return {
      x: effectivePadding.left + chartWidth * ratio,
      y: effectivePadding.top + (chartHeight - normalizedValue * chartHeight)
    };
  });
  $: smoothPath = points.length > 1 ? buildSmoothPath(points) : '';
  $: areaPath = smoothPath
    ? `${smoothPath} L ${effectivePadding.left + chartWidth} ${effectivePadding.top + chartHeight} L ${effectivePadding.left} ${effectivePadding.top + chartHeight} Z`
    : '';
  $: gridLines = Array.from({ length: 4 }, (_, i) => effectivePadding.top + (i / 3) * chartHeight);
  $: yAxisValues = Array.from({ length: 4 }, (_, i) => dataMax - (i / 3) * valueRange);
  $: labels = chartData.map(point => formatDate(normalizeDate(point.date)));

  let tooltip = null;
  let hoveredPointIndex = null;

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
    <div class="chart-wrapper" bind:this={container} on:mouseleave={hideTooltip}>
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
          <circle
            cx={point.x}
            cy={point.y}
            r={hoveredPointIndex === index ? 6 : 4}
            fill={color}
            stroke="white"
            stroke-width="2"
            class="chart-point"
            class:chart-point--active={hoveredPointIndex === index}
            tabindex="0"
            aria-label={`${labels[index]}: ${chartData[index]?.count ?? 0} items`}
            on:mouseenter={() => showTooltip(point, index)}
            on:focus={() => showTooltip(point, index)}
            on:mouseleave={hideTooltip}
            on:blur={hideTooltip}
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
