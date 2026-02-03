<script>
  import { onMount } from 'svelte';
  import { t } from '../../stores/i18n.svelte.js';

  let { burndownData = null } = $props();

  const padding = { top: 32, right: 32, bottom: 40, left: 48 };
  const clamp = (value, min, max) => Math.min(Math.max(value, min), max);

  function formatDateShort(dateStr) {
    const date = new Date(dateStr);
    const month = String(date.getMonth() + 1).padStart(2, '0');
    const day = String(date.getDate()).padStart(2, '0');
    return `${month}/${day}`;
  }

  const idealColor = '#9ca3af';
  const actualColor = '#3b82f6';
  const gradientId = `burndown-${Math.random().toString(36).slice(2, 9)}`;

  let container = $state(null);
  let width = $state(600);

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

  const dataPoints = $derived(burndownData?.data_points || []);
  const totalItems = $derived(burndownData?.total_items || 0);
  const chartWidth = $derived(Math.max(width - (padding.left + padding.right), 100));
  const chartHeight = $derived(clamp(chartWidth * 0.4, 180, 280));
  const svgWidth = $derived(chartWidth + padding.left + padding.right);
  const svgHeight = $derived(chartHeight + padding.top + padding.bottom);
  const maxValue = $derived(Math.max(totalItems, 1));

  const getX = $derived.by(() => (index, total) => {
    if (total <= 1) return padding.left + chartWidth / 2;
    return padding.left + (index / (total - 1)) * chartWidth;
  });

  const getY = $derived.by(() => (value) => {
    return padding.top + chartHeight - (value / maxValue) * chartHeight;
  });

  const idealPoints = $derived.by(() => {
    return dataPoints.map((d, i) => ({
      x: getX(i, dataPoints.length),
      y: getY(d.ideal),
      value: d.ideal,
      date: d.date
    }));
  });

  const actualPoints = $derived.by(() => {
    return dataPoints.map((d, i) => ({
      x: getX(i, dataPoints.length),
      y: getY(d.remaining),
      value: d.remaining,
      date: d.date
    }));
  });

  function buildPath(points) {
    if (points.length < 2) return '';
    let path = `M ${points[0].x} ${points[0].y}`;
    for (let i = 1; i < points.length; i++) {
      path += ` L ${points[i].x} ${points[i].y}`;
    }
    return path;
  }

  const idealPath = $derived(buildPath(idealPoints));
  const actualPath = $derived(buildPath(actualPoints));
  const areaPath = $derived(
    actualPath
      ? `${actualPath} L ${padding.left + chartWidth} ${padding.top + chartHeight} L ${padding.left} ${padding.top + chartHeight} Z`
      : ''
  );

  // Y-axis grid lines
  const gridLines = $derived.by(() => {
    return Array.from({ length: 5 }, (_, i) => ({
      y: padding.top + (i / 4) * chartHeight,
      value: Math.round(maxValue - (i / 4) * maxValue)
    }));
  });

  // X-axis labels (show max 7 labels)
  const xLabels = $derived.by(() => {
    if (dataPoints.length <= 7) {
      return dataPoints.map((d, i) => ({
        x: getX(i, dataPoints.length),
        label: formatDateShort(d.date)
      }));
    }
    const step = Math.ceil(dataPoints.length / 6);
    const labels = [];
    for (let i = 0; i < dataPoints.length; i += step) {
      labels.push({
        x: getX(i, dataPoints.length),
        label: formatDateShort(dataPoints[i].date)
      });
    }
    // Always include last point
    if (labels.length > 0 && labels[labels.length - 1].label !== formatDateShort(dataPoints[dataPoints.length - 1].date)) {
      labels.push({
        x: getX(dataPoints.length - 1, dataPoints.length),
        label: formatDateShort(dataPoints[dataPoints.length - 1].date)
      });
    }
    return labels;
  });

  let tooltip = $state(null);
  let hoveredIndex = $state(null);

  function showTooltip(point, index, type) {
    const dataPoint = dataPoints[index];
    hoveredIndex = index;
    tooltip = {
      x: point.x,
      y: point.y,
      date: formatDateShort(dataPoint.date),
      remaining: dataPoint.remaining,
      completed: dataPoint.completed,
      ideal: dataPoint.ideal,
      type
    };
  }

  function hideTooltip() {
    tooltip = null;
    hoveredIndex = null;
  }
</script>

{#if burndownData && dataPoints.length > 0}
  <div class="burndown-chart">
    <div class="chart-header">
      <h3 class="chart-title">{t('iterations.burndownChart')}</h3>
      <div class="chart-legend">
        <div class="legend-item">
          <span class="legend-line legend-line--actual"></span>
          <span class="legend-label">{t('iterations.remaining')}</span>
        </div>
        <div class="legend-item">
          <span class="legend-line legend-line--ideal"></span>
          <span class="legend-label">{t('iterations.idealProgress')}</span>
        </div>
      </div>
    </div>

    <div class="chart-wrapper" bind:this={container} onmouseleave={hideTooltip}>
      <svg
        class="chart-svg"
        viewBox={`0 0 ${svgWidth} ${svgHeight}`}
        style={`height:${svgHeight}px;`}
      >
        <defs>
          <linearGradient id={gradientId} x1="0%" y1="0%" x2="0%" y2="100%">
            <stop offset="0%" style={`stop-color:${actualColor};stop-opacity:0.15`} />
            <stop offset="100%" style={`stop-color:${actualColor};stop-opacity:0.02`} />
          </linearGradient>
        </defs>

        <!-- Grid lines -->
        {#each gridLines as line}
          <line
            x1={padding.left}
            y1={line.y}
            x2={padding.left + chartWidth}
            y2={line.y}
            stroke="var(--ds-border)"
            stroke-width="1"
          />
          <text
            x={padding.left - 8}
            y={line.y + 4}
            text-anchor="end"
            font-size="11"
            fill="var(--ds-text-subtle)"
          >
            {line.value}
          </text>
        {/each}

        <!-- X-axis labels -->
        {#each xLabels as label}
          <text
            x={label.x}
            y={padding.top + chartHeight + 20}
            text-anchor="middle"
            font-size="11"
            fill="var(--ds-text-subtle)"
          >
            {label.label}
          </text>
        {/each}

        <!-- Area under actual line -->
        {#if areaPath}
          <path d={areaPath} fill={`url(#${gradientId})`} />
        {/if}

        <!-- Ideal line (dashed) -->
        {#if idealPath}
          <path
            d={idealPath}
            fill="none"
            stroke={idealColor}
            stroke-width="2"
            stroke-dasharray="6,4"
          />
        {/if}

        <!-- Actual remaining line -->
        {#if actualPath}
          <path
            d={actualPath}
            fill="none"
            stroke={actualColor}
            stroke-width="2.5"
            stroke-linecap="round"
            stroke-linejoin="round"
          />
        {/if}

        <!-- Ideal line points -->
        {#each idealPoints as point, index}
          <circle
            cx={point.x}
            cy={point.y}
            r={hoveredIndex === index ? 5 : 3}
            fill={idealColor}
            stroke="white"
            stroke-width="1.5"
            class="chart-point"
            tabindex="0"
            aria-label={`${point.date}: ${point.value} ideal`}
            onmouseenter={() => showTooltip(point, index, 'ideal')}
            onfocus={() => showTooltip(point, index, 'ideal')}
            onmouseleave={hideTooltip}
            onblur={hideTooltip}
          />
        {/each}

        <!-- Actual line points -->
        {#each actualPoints as point, index}
          <circle
            cx={point.x}
            cy={point.y}
            r={hoveredIndex === index ? 6 : 4}
            fill={actualColor}
            stroke="white"
            stroke-width="2"
            class="chart-point"
            tabindex="0"
            aria-label={`${point.date}: ${point.value} remaining`}
            onmouseenter={() => showTooltip(point, index, 'actual')}
            onfocus={() => showTooltip(point, index, 'actual')}
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
          <div class="tooltip-date">{tooltip.date}</div>
          <div class="tooltip-row">
            <span class="tooltip-label">{t('iterations.remaining')}:</span>
            <span class="tooltip-value tooltip-value--remaining">{tooltip.remaining}</span>
          </div>
          <div class="tooltip-row">
            <span class="tooltip-label">{t('iterations.completed')}:</span>
            <span class="tooltip-value tooltip-value--completed">{tooltip.completed}</span>
          </div>
          <div class="tooltip-row tooltip-row--ideal">
            <span class="tooltip-label">{t('iterations.ideal')}:</span>
            <span class="tooltip-value">{tooltip.ideal}</span>
          </div>
        </div>
      {/if}
    </div>
  </div>
{:else}
  <div class="chart-empty">
    <p>{t('iterations.noBurndownData')}</p>
  </div>
{/if}

<style>
  .burndown-chart {
    width: 100%;
  }

  .chart-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 1rem;
  }

  .chart-title {
    font-size: 0.875rem;
    font-weight: 600;
    color: var(--ds-text);
    margin: 0;
  }

  .chart-legend {
    display: flex;
    gap: 1rem;
  }

  .legend-item {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .legend-line {
    width: 20px;
    height: 2px;
  }

  .legend-line--actual {
    background-color: #3b82f6;
  }

  .legend-line--ideal {
    background: repeating-linear-gradient(
      to right,
      #9ca3af,
      #9ca3af 6px,
      transparent 6px,
      transparent 10px
    );
  }

  .legend-label {
    font-size: 0.75rem;
    color: var(--ds-text-subtle);
  }

  .chart-wrapper {
    width: 100%;
    position: relative;
  }

  .chart-svg {
    width: 100%;
    display: block;
  }

  .chart-point {
    cursor: pointer;
    transition: r 0.1s ease;
  }

  .chart-tooltip {
    position: absolute;
    transform: translate(-50%, calc(-100% - 12px));
    background: var(--ds-surface-raised);
    border: 1px solid var(--ds-border);
    border-radius: 0.5rem;
    padding: 0.5rem 0.75rem;
    box-shadow: 0 10px 15px -3px rgba(0, 0, 0, 0.1);
    pointer-events: none;
    font-size: 0.75rem;
    color: var(--ds-text);
    min-width: 120px;
    z-index: 10;
  }

  .tooltip-date {
    font-weight: 600;
    margin-bottom: 0.25rem;
    border-bottom: 1px solid var(--ds-border);
    padding-bottom: 0.25rem;
  }

  .tooltip-row {
    display: flex;
    justify-content: space-between;
    gap: 0.5rem;
    margin-top: 0.25rem;
  }

  .tooltip-row--ideal {
    color: var(--ds-text-subtle);
    font-size: 0.7rem;
  }

  .tooltip-label {
    color: var(--ds-text-subtle);
  }

  .tooltip-value {
    font-weight: 500;
  }

  .tooltip-value--remaining {
    color: #3b82f6;
  }

  .tooltip-value--completed {
    color: #22c55e;
  }

  .chart-empty {
    height: 200px;
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--ds-text-subtlest);
    font-size: 0.875rem;
    border: 1px dashed var(--ds-border);
    border-radius: 0.5rem;
  }
</style>
