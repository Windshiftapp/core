<script>
  import { t } from '../../stores/i18n.svelte.js';
  import { useResizeObserver } from 'runed';
  import Text from '../../components/Text.svelte';

  let { data = null } = $props();

  const padding = { top: 32, right: 24, bottom: 48, left: 56 };

  let container = $state(null);
  let width = $state(600);

  useResizeObserver(() => container, (entries) => {
    const entry = entries[0];
    if (entry) width = entry.contentRect.width;
  });

  const categories = $derived(data?.categories || []);
  const dataPoints = $derived(data?.data_points || []);
  const chartWidth = $derived(Math.max(width - padding.left - padding.right, 100));
  const chartHeight = $derived(Math.min(Math.max(chartWidth * 0.45, 180), 300));
  const svgWidth = $derived(chartWidth + padding.left + padding.right);
  const svgHeight = $derived(chartHeight + padding.top + padding.bottom);

  const maxTotal = $derived.by(() => {
    if (!dataPoints.length) return 1;
    return Math.max(...dataPoints.map(dp => {
      const counts = dp.counts || {};
      return Object.values(counts).reduce((sum, c) => sum + c, 0);
    }), 1);
  });

  const getX = $derived.by(() => (index) => {
    if (dataPoints.length <= 1) return padding.left + chartWidth / 2;
    return padding.left + (index / (dataPoints.length - 1)) * chartWidth;
  });

  const getY = $derived.by(() => (value) => {
    return padding.top + chartHeight - (value / maxTotal) * chartHeight;
  });

  // Default fallback colors if API doesn't provide them
  const fallbackColors = ['#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6', '#ec4899', '#06b6d4', '#84cc16'];

  function getCategoryColor(cat, index) {
    return cat.color || fallbackColors[index % fallbackColors.length];
  }

  // Build stacked area paths - categories stacked bottom to top
  const stackedAreas = $derived.by(() => {
    if (!dataPoints.length || !categories.length) return [];

    const catNames = categories.map(c => c.name);
    const areas = [];

    for (let ci = 0; ci < catNames.length; ci++) {
      const catName = catNames[ci];
      const topPoints = [];
      const bottomPoints = [];

      for (let di = 0; di < dataPoints.length; di++) {
        const x = getX(di);
        const counts = dataPoints[di].counts || {};

        // Sum all categories below this one (for bottom of band)
        let bottom = 0;
        for (let bi = 0; bi < ci; bi++) {
          bottom += counts[catNames[bi]] || 0;
        }
        // Top includes this category
        const top = bottom + (counts[catName] || 0);

        topPoints.push({ x, y: getY(top) });
        bottomPoints.push({ x, y: getY(bottom) });
      }

      // Build path: top line forward, bottom line backward
      let path = `M ${topPoints[0].x} ${topPoints[0].y}`;
      for (let i = 1; i < topPoints.length; i++) {
        path += ` L ${topPoints[i].x} ${topPoints[i].y}`;
      }
      for (let i = bottomPoints.length - 1; i >= 0; i--) {
        path += ` L ${bottomPoints[i].x} ${bottomPoints[i].y}`;
      }
      path += ' Z';

      areas.push({ path, color: getCategoryColor(categories[ci], ci), name: catName });
    }

    return areas;
  });

  const gridLines = $derived.by(() => {
    const count = 5;
    return Array.from({ length: count }, (_, i) => ({
      y: padding.top + (i / (count - 1)) * chartHeight,
      value: Math.round(maxTotal - (i / (count - 1)) * maxTotal)
    }));
  });

  const xLabels = $derived.by(() => {
    if (dataPoints.length <= 7) {
      return dataPoints.map((d, i) => ({ x: getX(i), label: formatDate(d.date) }));
    }
    const step = Math.ceil(dataPoints.length / 6);
    const labels = [];
    for (let i = 0; i < dataPoints.length; i += step) {
      labels.push({ x: getX(i), label: formatDate(dataPoints[i].date) });
    }
    const last = dataPoints[dataPoints.length - 1];
    if (labels.length && labels[labels.length - 1].label !== formatDate(last.date)) {
      labels.push({ x: getX(dataPoints.length - 1), label: formatDate(last.date) });
    }
    return labels;
  });

  function formatDate(dateStr) {
    const d = new Date(dateStr);
    return `${String(d.getMonth() + 1).padStart(2, '0')}/${String(d.getDate()).padStart(2, '0')}`;
  }
</script>

{#if data && dataPoints.length > 0}
  <div class="cfd-chart">
    <div class="flex items-center gap-3 flex-wrap mb-3">
      {#each categories as cat, i}
        <div class="flex items-center gap-1.5 text-xs" style="color: var(--ds-text-subtle);">
          <span class="inline-block w-3 h-3 rounded-sm" style="background: {getCategoryColor(cat, i)};"></span>
          {cat.name}
        </div>
      {/each}
    </div>

    <div bind:this={container}>
      <svg viewBox={`0 0 ${svgWidth} ${svgHeight}`} style="height:{svgHeight}px; width:100%;">
        <!-- Grid -->
        {#each gridLines as line}
          <line x1={padding.left} y1={line.y} x2={padding.left + chartWidth} y2={line.y}
            stroke="var(--ds-border)" stroke-width="1" />
          <text x={padding.left - 8} y={line.y + 4} text-anchor="end" font-size="11" fill="var(--ds-text-subtle)">
            {line.value}
          </text>
        {/each}

        <!-- Stacked areas -->
        {#each stackedAreas as area}
          <path d={area.path} fill={area.color} opacity="0.6" />
        {/each}

        <!-- X labels -->
        {#each xLabels as label}
          <text x={label.x} y={padding.top + chartHeight + 16} text-anchor="middle" font-size="10" fill="var(--ds-text-subtle)">
            {label.label}
          </text>
        {/each}
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
