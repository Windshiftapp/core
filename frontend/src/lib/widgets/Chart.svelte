<script>
  import { useResizeObserver } from 'runed';
  import { t } from '../stores/i18n.svelte.js';

  let {
    type = 'line',
    series = [],
    categories = [],
    minHeight = 180,
    maxHeight = 280,
    heightRatio = 0.4,
    padding: paddingProp = null,
    showYAxis = true,
    yAxisFormat = null,
    gridLineCount = 5,
    gridDashed = false,
    maxXLabels = 7,
    rotateXLabels = 'auto',
    minValue = 0,
    maxValue = null,
    referenceLines = [],
    showLegend = 'auto',
    valueFormat = null,
    emptyMessage = t('widgets.chart.noDataAvailable'),
    barMaxWidth = 48,
    barRadius = 3,
    barHeight: barHeightProp = 24,
    barGap = 8,
    tooltipContent
  } = $props();

  const clamp = (v, lo, hi) => Math.min(Math.max(v, lo), hi);
  const chartId = Math.random().toString(36).slice(2, 9);

  // Type-specific default padding
  const defaultPad = $derived.by(() => {
    switch (type) {
      case 'bar': return { top: 32, right: 24, bottom: 64, left: 56 };
      case 'stacked-area': return { top: 32, right: 24, bottom: 48, left: 56 };
      case 'horizontal-bar': return { top: 16, right: 24, bottom: 16, left: 120 };
      default: return { top: 32, right: showYAxis ? 16 : 32, bottom: 40, left: showYAxis ? 48 : 32 };
    }
  });
  const pad = $derived(paddingProp ? { ...defaultPad, ...paddingProp } : defaultPad);

  // Container + resize observer
  let container = $state(null);
  let containerWidth = $state(600);
  useResizeObserver(() => container, (entries) => {
    const entry = entries[0];
    if (entry) containerWidth = entry.contentRect.width;
  });

  // Chart dimensions
  const cw = $derived(Math.max(containerWidth - pad.left - pad.right, 100));
  const ch = $derived.by(() => {
    if (type === 'horizontal-bar') return categories.length * (barHeightProp + barGap);
    return clamp(cw * heightRatio, Math.min(minHeight, maxHeight), Math.max(minHeight, maxHeight));
  });
  const sw = $derived(cw + pad.left + pad.right);
  const sh = $derived(ch + pad.top + pad.bottom);

  // Value range
  const dMin = $derived(minValue ?? 0);
  const dMax = $derived.by(() => {
    if (maxValue !== null) return maxValue;
    if (type === 'stacked-area') {
      let mx = 1;
      for (let i = 0; i < categories.length; i++) {
        let sum = 0;
        for (const s of series) sum += s.values[i] || 0;
        mx = Math.max(mx, sum);
      }
      return mx;
    }
    let mx = 1;
    for (const s of series) for (const v of s.values) mx = Math.max(mx, v || 0);
    return mx;
  });
  const vRange = $derived(dMax - dMin || 1);

  // Coordinate helpers
  const getX = $derived.by(() => (index) => {
    const n = categories.length;
    if (type === 'bar') {
      const slot = cw / n;
      return pad.left + slot * index + slot / 2;
    }
    if (n <= 1) return pad.left + cw / 2;
    return pad.left + (index / (n - 1)) * cw;
  });
  const getY = $derived.by(() => (value) => {
    return pad.top + ch - ((value - dMin) / vRange) * ch;
  });

  // Grid lines with smart integer-step dedup when max <= 5
  const gridLines = $derived.by(() => {
    if (type === 'horizontal-bar') return [];
    if (dMax <= 5 && dMin === 0) {
      const steps = dMax + 1;
      return Array.from({ length: steps }, (_, i) => ({
        y: pad.top + (i / Math.max(steps - 1, 1)) * ch,
        value: dMax - i
      }));
    }
    const count = gridLineCount;
    const seen = new Set();
    return Array.from({ length: count }, (_, i) => ({
      y: pad.top + (i / (count - 1)) * ch,
      value: Math.round(dMax - (i / (count - 1)) * vRange)
    })).filter(l => {
      if (seen.has(l.value)) return false;
      seen.add(l.value);
      return true;
    });
  });

  // X-axis labels with max count + always-include-last
  const xLabels = $derived.by(() => {
    if (type === 'horizontal-bar') return [];
    const n = categories.length;
    if (n <= maxXLabels) return categories.map((l, i) => ({ x: getX(i), label: l }));
    const step = Math.ceil(n / (maxXLabels - 1));
    const labels = [];
    for (let i = 0; i < n; i += step) labels.push({ x: getX(i), label: categories[i] });
    const last = categories[n - 1];
    if (labels.length && labels[labels.length - 1].label !== last) {
      labels.push({ x: getX(n - 1), label: last });
    }
    return labels;
  });
  const shouldRotate = $derived(
    rotateXLabels === true || (rotateXLabels === 'auto' && categories.length > maxXLabels)
  );

  // Path builders
  function buildSmoothPath(pts) {
    if (pts.length < 2) return '';
    let d = `M ${pts[0].x} ${pts[0].y}`;
    for (let i = 1; i < pts.length; i++) {
      const cx = (pts[i - 1].x + pts[i].x) / 2;
      d += ` Q ${cx} ${pts[i - 1].y} ${pts[i].x} ${pts[i].y}`;
    }
    return d;
  }
  function buildLinearPath(pts) {
    if (pts.length < 2) return '';
    let d = `M ${pts[0].x} ${pts[0].y}`;
    for (let i = 1; i < pts.length; i++) d += ` L ${pts[i].x} ${pts[i].y}`;
    return d;
  }

  // Line chart derived data
  const lineData = $derived.by(() => {
    if (type !== 'line') return [];
    return series.map(s => {
      const pts = s.values.map((v, i) => ({ x: getX(i), y: getY(v), value: v }));
      const path = (s.smooth !== false) ? buildSmoothPath(pts) : buildLinearPath(pts);
      const area = (s.showArea !== false && path)
        ? `${path} L ${pad.left + cw} ${pad.top + ch} L ${pad.left} ${pad.top + ch} Z`
        : '';
      return { ...s, pts, path, area };
    });
  });

  // Stacked area derived data
  const stackedAreas = $derived.by(() => {
    if (type !== 'stacked-area' || !categories.length || !series.length) return [];
    return series.map((s, si) => {
      const top = [], bot = [];
      for (let ci = 0; ci < categories.length; ci++) {
        const x = getX(ci);
        let b = 0;
        for (let bi = 0; bi < si; bi++) b += series[bi].values[ci] || 0;
        top.push({ x, y: getY(b + (s.values[ci] || 0)) });
        bot.push({ x, y: getY(b) });
      }
      let d = `M ${top[0].x} ${top[0].y}`;
      for (let i = 1; i < top.length; i++) d += ` L ${top[i].x} ${top[i].y}`;
      for (let i = bot.length - 1; i >= 0; i--) d += ` L ${bot[i].x} ${bot[i].y}`;
      return { path: d + ' Z', color: s.color, label: s.label };
    });
  });

  // Bar chart bar width
  const bw = $derived.by(() => {
    if (type !== 'bar' || !categories.length) return 20;
    return Math.min((cw / categories.length) * 0.6, barMaxWidth);
  });

  // Legend visibility
  const legendVisible = $derived(showLegend === 'auto' ? series.length > 1 : showLegend);

  // Tooltip state
  let tooltip = $state(null);
  let hoveredIndex = $state(null);
  let hoveredSeries = $state(null);

  function showTip(index, x, y, seriesKey = null) {
    hoveredIndex = index;
    hoveredSeries = seriesKey;
    tooltip = {
      index,
      category: categories[index],
      seriesValues: series.map(s => ({
        key: s.key, label: s.label, color: s.color, value: s.values[index]
      })),
      x, y
    };
  }
  function hideTip() {
    tooltip = null;
    hoveredIndex = null;
    hoveredSeries = null;
  }
  function fmtVal(v) {
    return valueFormat ? valueFormat(v) : v;
  }
</script>

{#if series.length > 0 && categories.length > 0}
  <div class="chart" class:chart--hbar={type === 'horizontal-bar'}>
    {#if legendVisible}
      <div class="chart-legend">
        {#each series as s}
          <div class="legend-item">
            {#if s.dashed}
              <span class="legend-line legend-line--dashed" style="--lc:{s.color};"></span>
            {:else}
              <span class="legend-dot" style="background:{s.color};"></span>
            {/if}
            <span class="legend-label">{s.label}</span>
          </div>
        {/each}
      </div>
    {/if}

    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div class="chart-wrapper" bind:this={container} onmouseleave={hideTip}>
      <svg viewBox="0 0 {sw} {sh}" style="height:{sh}px;width:100%;">
        <!-- Gradient defs for line areas -->
        {#if type === 'line'}
          <defs>
            {#each series as s}
              {#if s.showArea !== false}
                <linearGradient id="{chartId}-{s.key}" x1="0%" y1="0%" x2="0%" y2="100%">
                  <stop offset="0%" style="stop-color:{s.color};stop-opacity:{s.opacity ?? 0.35}" />
                  <stop offset="100%" style="stop-color:{s.color};stop-opacity:0.05" />
                </linearGradient>
              {/if}
            {/each}
          </defs>
        {/if}

        <!-- Grid lines + Y-axis labels -->
        {#each gridLines as gl}
          <line
            x1={pad.left} y1={gl.y} x2={pad.left + cw} y2={gl.y}
            stroke="var(--ds-border)" stroke-width="1"
            stroke-dasharray={gridDashed ? '3,3' : 'none'}
          />
          {#if showYAxis}
            <text x={pad.left - 8} y={gl.y + 4} text-anchor="end" font-size="11" fill="var(--ds-text-subtle)">
              {yAxisFormat ? yAxisFormat(gl.value) : gl.value}
            </text>
          {/if}
        {/each}

        <!-- ── LINE ── -->
        {#if type === 'line'}
          {#each lineData as ld}
            {#if ld.area}
              <path d={ld.area} fill="url(#{chartId}-{ld.key})" opacity="0.8" />
            {/if}
          {/each}
          {#each lineData as ld}
            {#if ld.path}
              <path
                d={ld.path} fill="none" stroke={ld.color}
                stroke-width={ld.strokeWidth ?? 2.5}
                stroke-linecap="round" stroke-linejoin="round"
                stroke-dasharray={ld.dashed ? '6,4' : 'none'}
              />
            {/if}
          {/each}
          {#each lineData as ld}
            {#if ld.showPoints !== false}
              {#each ld.pts as pt, idx}
                <!-- svelte-ignore a11y_no_static_element_interactions -->
                <circle
                  cx={pt.x} cy={pt.y}
                  r={hoveredIndex === idx ? (ld.pointRadius ?? 4) + 2 : (ld.pointRadius ?? 4)}
                  fill={ld.color} stroke="white" stroke-width="2"
                  class="chart-point"
                  tabindex="-1"
                  aria-label="{categories[idx]}: {pt.value}"
                  onmouseenter={() => showTip(idx, pt.x, pt.y, ld.key)}
                  onfocus={() => showTip(idx, pt.x, pt.y, ld.key)}
                  onmouseleave={hideTip}
                  onblur={hideTip}
                />
              {/each}
            {/if}
          {/each}

        <!-- ── BAR ── -->
        {:else if type === 'bar'}
          {#each categories as _, i}
            {@const s = series[0]}
            {@const v = s.values[i] || 0}
            {@const h = ((v - dMin) / vRange) * ch}
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <rect
              x={getX(i) - bw / 2} y={pad.top + ch - h}
              width={bw} height={h}
              rx={barRadius} fill={s.color}
              opacity={hoveredIndex === i ? 1 : 0.85}
              class="chart-point"
              tabindex="-1"
              onmouseenter={() => showTip(i, getX(i), getY(v))}
              onmouseleave={hideTip}
            />
          {/each}

        <!-- ── STACKED AREA ── -->
        {:else if type === 'stacked-area'}
          {#each stackedAreas as a}
            <path d={a.path} fill={a.color} opacity="0.6" />
          {/each}

        <!-- ── HORIZONTAL BAR ── -->
        {:else if type === 'horizontal-bar'}
          {#each categories as cat, i}
            {@const y = pad.top + i * (barHeightProp + barGap)}
            <!-- Category label on Y-axis -->
            <text x={pad.left - 8} y={y + barHeightProp / 2 + 4} text-anchor="end" font-size="11" fill="var(--ds-text-subtle)">
              {cat.length > 16 ? cat.slice(0, 15) + '\u2026' : cat}
            </text>
            <!-- Overlapping bars: first series is background (wider, lower opacity) -->
            {#each series as s, si}
              {@const v = s.values[i] || 0}
              {@const w = (v / dMax) * cw}
              <!-- svelte-ignore a11y_no_static_element_interactions -->
              <rect
                x={pad.left} y={si === 0 ? y : y + 4}
                width={Math.max(w, 2)}
                height={si === 0 ? barHeightProp : barHeightProp - 8}
                rx={barRadius + 1} fill={s.color}
                opacity={s.opacity ?? (si === 0 ? 0.3 : 0.85)}
                tabindex="-1"
                onmouseenter={() => showTip(i, pad.left + w, y)}
                onmouseleave={hideTip}
              />
            {/each}
            <!-- Value label after widest bar -->
            {@const maxW = Math.max(...series.map(s => ((s.values[i] || 0) / dMax) * cw))}
            <text x={pad.left + Math.max(maxW, 2) + 6} y={y + barHeightProp / 2 + 4} font-size="10" fill="var(--ds-text-subtle)">
              {fmtVal(series[series.length - 1].values[i])}
            </text>
          {/each}
        {/if}

        <!-- Reference lines -->
        {#each referenceLines as rl}
          {@const ry = getY(rl.value)}
          <line
            x1={pad.left} y1={ry} x2={pad.left + cw} y2={ry}
            stroke={rl.color} stroke-width="1.5"
            stroke-dasharray={rl.dashed ? '6,4' : 'none'}
          />
          {#if rl.label}
            <text x={pad.left + cw + 4} y={ry + 4} font-size="10" fill={rl.color}>{rl.label}</text>
          {/if}
        {/each}

        <!-- X-axis labels -->
        {#each xLabels as xl}
          {@const ly = pad.top + ch + (shouldRotate ? 16 : 20)}
          <text
            x={xl.x} y={ly}
            text-anchor={shouldRotate ? 'end' : 'middle'}
            font-size={type === 'bar' ? '10' : '11'}
            fill="var(--ds-text-subtle)"
            transform={shouldRotate ? `rotate(-45, ${xl.x}, ${ly})` : undefined}
          >
            {xl.label.length > 12 ? xl.label.slice(0, 11) + '\u2026' : xl.label}
          </text>
        {/each}
      </svg>

      <!-- HTML tooltip -->
      {#if tooltip}
        <div class="chart-tooltip" style="left:{tooltip.x}px;top:{tooltip.y}px;">
          {#if tooltipContent}
            {@render tooltipContent(tooltip)}
          {:else}
            <div class="tooltip-cat">{tooltip.category}</div>
            {#each tooltip.seriesValues as sv}
              <div class="tooltip-row">
                {#if series.length > 1}
                  <span class="tooltip-dot" style="background:{sv.color};"></span>
                {/if}
                <span class="tooltip-val">{fmtVal(sv.value)}</span>
              </div>
            {/each}
          {/if}
        </div>
      {/if}
    </div>
  </div>
{:else}
  <div class="chart-empty">
    <p>{emptyMessage}</p>
  </div>
{/if}

<style>
  .chart {
    width: 100%;
  }

  .chart-wrapper {
    width: 100%;
    position: relative;
  }

  .chart-wrapper svg {
    display: block;
  }

  .chart-legend {
    display: flex;
    gap: 1rem;
    margin-bottom: 0.75rem;
    flex-wrap: wrap;
  }

  .legend-item {
    display: flex;
    align-items: center;
    gap: 0.375rem;
    font-size: 0.75rem;
    color: var(--ds-text-subtle);
  }

  .legend-dot {
    width: 0.75rem;
    height: 0.75rem;
    border-radius: 0.125rem;
    flex-shrink: 0;
  }

  .legend-line {
    width: 1.25rem;
    height: 2px;
    flex-shrink: 0;
  }

  .legend-line--dashed {
    background: repeating-linear-gradient(
      to right, var(--lc), var(--lc) 6px, transparent 6px, transparent 10px
    );
  }

  .legend-label {
    white-space: nowrap;
  }

  .chart-point {
    cursor: pointer;
    transition: r 0.1s ease, opacity 0.1s ease;
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
    min-width: 100px;
    z-index: 10;
    text-align: center;
  }

  .chart--hbar .chart-tooltip {
    transform: translate(12px, -50%);
    text-align: left;
  }

  .tooltip-cat {
    font-weight: 600;
    margin-bottom: 0.25rem;
  }

  .tooltip-row {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.25rem;
    margin-top: 0.125rem;
  }

  .chart--hbar .tooltip-row {
    justify-content: flex-start;
  }

  .tooltip-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    flex-shrink: 0;
  }

  .tooltip-val {
    font-weight: 500;
  }

  .chart-empty {
    height: 160px;
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--ds-text-subtlest);
    font-size: 0.875rem;
    border: 1px dashed var(--ds-border);
    border-radius: 0.5rem;
  }
</style>
