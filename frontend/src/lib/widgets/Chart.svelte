<script>
  import { useResizeObserver } from 'runed';
  import { t } from '../stores/i18n.svelte.js';

  // Categorical palette for pie slices, where every slice is its own series.
  const PIE_COLORS = [
    '#6366f1', '#22c55e', '#f59e0b', '#ec4899', '#06b6d4',
    '#8b5cf6', '#ef4444', '#14b8a6', '#f97316', '#84cc16'
  ];
  const PIE_OTHER_COLOR = '#94a3b8';

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
    rotateXLabels = /** @type {boolean | 'auto'} */ ('auto'),
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
    tooltipContent = null,
    // Pie charts take their slices from `series`, one per category, unless the
    // caller passes an explicit `slices` array of {label, value, color?}.
    slices: slicesProp = null,
    maxSlices = 10,
    donut = false
  } = $props();

  const clamp = (v, lo, hi) => Math.min(Math.max(v, lo), hi);
  const chartId = Math.random().toString(36).slice(2, 9);

  // Type-specific default padding
  const defaultPad = $derived.by(() => {
    switch (type) {
      case 'bar': return { top: 32, right: 24, bottom: 64, left: 56 };
      case 'stacked-area': return { top: 32, right: 24, bottom: 48, left: 56 };
      case 'horizontal-bar': return { top: 16, right: 24, bottom: 16, left: 120 };
      case 'pie': return { top: 8, right: 8, bottom: 8, left: 8 };
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
    if (type === 'pie') return clamp(cw * heightRatio, Math.min(minHeight, maxHeight), Math.max(minHeight, maxHeight));
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

  // Grid lines with smart integer-step dedup when max <= 5. A pie has no value
  // axis, so it draws none.
  const gridLines = $derived.by(() => {
    if (type === 'horizontal-bar' || type === 'pie') return [];
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

  // X-axis labels with max count + always-include-last. A pie labels its
  // slices instead.
  const xLabels = $derived.by(() => {
    if (type === 'horizontal-bar' || type === 'pie') return [];
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
    rotateXLabels === true || (/** @type {string} */ (rotateXLabels) === 'auto' && categories.length > maxXLabels)
  );

  // Path builders
  function buildSmoothPath(pts) {
    if (pts.length < 2) return '';

    const slopes = [];
    for (let i = 0; i < pts.length - 1; i++) {
      slopes.push((pts[i + 1].y - pts[i].y) / (pts[i + 1].x - pts[i].x));
    }

    // Average neighboring slopes, then constrain the tangents so the curve
    // remains inside each pair of values instead of introducing false peaks.
    const tangents = [slopes[0]];
    for (let i = 1; i < pts.length - 1; i++) {
      tangents.push((slopes[i - 1] + slopes[i]) / 2);
    }
    tangents.push(slopes[slopes.length - 1]);

    for (let i = 0; i < slopes.length; i++) {
      if (slopes[i] === 0) {
        tangents[i] = 0;
        tangents[i + 1] = 0;
        continue;
      }

      const startRatio = tangents[i] / slopes[i];
      const endRatio = tangents[i + 1] / slopes[i];
      if (startRatio < 0) tangents[i] = 0;
      if (endRatio < 0) tangents[i + 1] = 0;

      const constrainedStart = tangents[i] / slopes[i];
      const constrainedEnd = tangents[i + 1] / slopes[i];
      const magnitude = constrainedStart ** 2 + constrainedEnd ** 2;
      if (magnitude > 9) {
        const scale = 3 / Math.sqrt(magnitude);
        tangents[i] = scale * constrainedStart * slopes[i];
        tangents[i + 1] = scale * constrainedEnd * slopes[i];
      }
    }

    let d = `M ${pts[0].x} ${pts[0].y}`;
    for (let i = 1; i < pts.length; i++) {
      const previous = pts[i - 1];
      const current = pts[i];
      const width = current.x - previous.x;
      d += ` C ${previous.x + width / 3} ${previous.y + (tangents[i - 1] * width) / 3}`;
      d += ` ${current.x - width / 3} ${current.y - (tangents[i] * width) / 3}`;
      d += ` ${current.x} ${current.y}`;
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

  // Legend visibility. `auto` shows it only when there is more than one series;
  // a pie always lists its slices unless the caller turns the legend off.
  const legendVisible = $derived.by(() => {
    if (showLegend === 'never') return false;
    if (showLegend === 'always') return true;
    if (type === 'pie') return true;
    return series.length > 1;
  });

  // Pie charts fold the categorical series into one value axis: one slice per
  // category. A long tail collapses into an "Other" slice so the drawing stays
  // readable, and the legend lists every slice with its share.
  const pieSlices = $derived.by(() => {
    if (type !== 'pie') return [];
    const raw = slicesProp
      ? slicesProp.map((slice, index) => ({
          label: String(slice.label ?? ''),
          value: Math.max(Number(slice.value) || 0, 0),
          color: slice.color ?? PIE_COLORS[index % PIE_COLORS.length]
        }))
      : series.map((s, index) => ({
          label: String(categories[index] ?? s.label ?? ''),
          value: Math.max(Number(s.values[0]) || 0, 0),
          color: s.color ?? PIE_COLORS[index % PIE_COLORS.length]
        }));

    const sorted = raw.filter((slice) => slice.value > 0).sort((a, b) => b.value - a.value);
    if (sorted.length <= maxSlices) return sorted;

    const kept = sorted.slice(0, maxSlices - 1);
    const rest = sorted.slice(maxSlices - 1);
    const restValue = rest.reduce((sum, slice) => sum + slice.value, 0);
    const otherLabel = t('widgets.chart.otherSlices', { count: rest.length });
    kept.push({ label: otherLabel, value: restValue, color: PIE_OTHER_COLOR, isOther: true });
    return kept;
  });
  const pieTotal = $derived(pieSlices.reduce((sum, slice) => sum + slice.value, 0));
  const pieRadius = $derived(Math.max(Math.min(cw, ch) / 2, 20));
  const pieCenter = $derived({
    x: pad.left + cw / 2,
    y: pad.top + ch / 2
  });

  function polar(angle, radius) {
    return {
      x: pieCenter.x + radius * Math.sin(angle),
      y: pieCenter.y - radius * Math.cos(angle)
    };
  }
  function slicePath(startAngle, endAngle, radius) {
    // A single slice covering the whole circle has no meaningful arc, so draw it
    // as two half-circle wedges instead of one degenerate path.
    if (endAngle - startAngle >= Math.PI * 2 - 1e-6) {
      const a = polar(0, radius);
      const b = polar(Math.PI, radius);
      return `M ${a.x} ${a.y} A ${radius} ${radius} 0 1 1 ${b.x} ${b.y} A ${radius} ${radius} 0 1 1 ${a.x} ${a.y} Z`;
    }
    const start = polar(startAngle, radius);
    const end = polar(endAngle, radius);
    const largeArc = endAngle - startAngle > Math.PI ? 1 : 0;
    return `M ${pieCenter.x} ${pieCenter.y} L ${start.x} ${start.y} A ${radius} ${radius} 0 ${largeArc} 1 ${end.x} ${end.y} Z`;
  }
  const pieData = $derived.by(() => {
    if (type !== 'pie' || pieTotal <= 0) return [];
    const innerRadius = donut ? pieRadius * 0.55 : 0;
    let angle = 0;
    return pieSlices.map((slice, index) => {
      const sweep = (slice.value / pieTotal) * Math.PI * 2;
      const start = angle;
      const end = angle + sweep;
      angle = end;
      const mid = start + sweep / 2;
      return {
        ...slice,
        index,
        percent: (slice.value / pieTotal) * 100,
        startAngle: start,
        endAngle: end,
        path: slicePath(start, end, pieRadius),
        innerRadius,
        labelPoint: polar(mid, (pieRadius + innerRadius) / 2)
      };
    });
  });
  const pieCenterLabel = $derived(valueFormat ? valueFormat(pieTotal) : pieTotal);

  // Labels inside slices need room for the text, so short sweeps keep their
  // label in the legend only instead of printing a clipped fragment.
  const sliceShowsLabel = (slice) => slice.percent >= 8;

  // Tooltip for a pie slice.
  function showSliceTip(slice, x, y) {
    hoveredIndex = slice.index;
    hoveredSeries = 'pie';
    tooltip = {
      index: slice.index,
      category: slice.label,
      seriesValues: [{ key: 'pie', label: slice.label, color: slice.color, value: slice.value }],
      percent: slice.percent,
      x,
      y
    };
  }

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

{#if type === 'pie' ? pieSlices.length > 0 : series.length > 0 && categories.length > 0}
  <div class="chart" class:chart--hbar={type === 'horizontal-bar'} class:chart--pie={type === 'pie'}>
    {#if legendVisible && type !== 'pie'}
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

    <!-- Pie keeps its legend beside the chart when the tile is wide enough,
         and stacks it underneath in a narrow tile. -->
    <div class="chart-body" class:chart-body--pie={type === 'pie'}>
      <div class="chart-body-inner" class:chart-body-inner--pie={type === 'pie'}>
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
                    data-testid="chart-series-{ld.key}"
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
                      data-testid="chart-point-{ld.key}-{idx}"
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

            <!-- ── PIE ── -->
            {:else if type === 'pie'}
              {#if pieData.length === 0}
                <text x={pieCenter.x} y={pieCenter.y} text-anchor="middle" font-size="12" fill="var(--ds-text-subtle)">
                  {emptyMessage}
                </text>
              {:else}
                {#each pieData as slice (slice.label)}
                  <!-- svelte-ignore a11y_no_static_element_interactions -->
                  <path
                    d={slice.path}
                    fill={slice.color}
                    stroke="var(--ds-surface-raised, white)"
                    stroke-width={pieData.length > 1 ? 1 : 0}
                    opacity={hoveredIndex === null || hoveredIndex === slice.index ? 1 : 0.55}
                    class="chart-point"
                    tabindex="-1"
                    role="img"
                    aria-label="{slice.label}: {fmtVal(slice.value)}"
                    data-testid="chart-slice-{slice.index}"
                    onmouseenter={() => showSliceTip(slice, slice.labelPoint.x, slice.labelPoint.y)}
                    onfocus={() => showSliceTip(slice, slice.labelPoint.x, slice.labelPoint.y)}
                    onmouseleave={hideTip}
                    onblur={hideTip}
                  />
                {/each}
                {#each pieData as slice (slice.label)}
                  {#if sliceShowsLabel(slice)}
                    <text
                      x={slice.labelPoint.x}
                      y={slice.labelPoint.y + 4}
                      text-anchor="middle"
                      font-size="11"
                      fill="#ffffff"
                      pointer-events="none"
                      data-testid="chart-slice-label-{slice.index}"
                    >
                      {Math.round(slice.percent)}%
                    </text>
                  {/if}
                {/each}
                {#if donut}
                  <text
                    x={pieCenter.x}
                    y={pieCenter.y + 5}
                    text-anchor="middle"
                    font-size="16"
                    font-weight="600"
                    fill="var(--ds-text)"
                    pointer-events="none"
                  >
                    {pieCenterLabel}
                  </text>
                {/if}
              {/if}

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
            {#if type !== 'pie'}
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
            {/if}

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
                    {#if series.length > 1 || type === 'pie'}
                      <span class="tooltip-dot" style="background:{sv.color};"></span>
                    {/if}
                    <span class="tooltip-val">{fmtVal(sv.value)}</span>
                    {#if type === 'pie' && tooltip.percent !== undefined}
                      <span class="tooltip-percent">{Math.round(tooltip.percent)}%</span>
                    {/if}
                  </div>
                {/each}
              {/if}
            </div>
          {/if}
        </div>

      <!-- Pie legend: every slice is listed with its value and share, so a
           slice too small to label stays readable. -->
      {#if type === 'pie' && pieData.length > 0 && legendVisible}
        <ul class="pie-legend" data-testid="chart-pie-legend">
          {#each pieData as slice (slice.label)}
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <li
              class="pie-legend-item"
              class:pie-legend-item--muted={hoveredIndex !== null && hoveredIndex !== slice.index}
              data-testid="chart-slice-legend-{slice.index}"
              onmouseenter={() => showSliceTip(slice, slice.labelPoint.x, slice.labelPoint.y)}
              onmouseleave={hideTip}
            >
              <span class="pie-legend-dot" style="background:{slice.color};"></span>
              <span class="pie-legend-label" title={slice.label}>{slice.label}</span>
              <span class="pie-legend-value">{fmtVal(slice.value)}</span>
              <span class="pie-legend-percent">{Math.round(slice.percent)}%</span>
            </li>
          {/each}
        </ul>
      {/if}
      </div>
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

  .chart--pie {
    display: flex;
    flex-direction: column;
  }

  /* Wide tiles put the legend beside the pie so the circle and the labels
     both get the horizontal room; narrow tiles stack them. The query has to
     live on a wrapper: a container cannot match its own size. */
  .chart-body {
    display: flex;
    flex-direction: column;
    align-items: center;
  }

  .chart-body--pie {
    container-type: inline-size;
  }

  .chart-body-inner {
    display: flex;
    flex-direction: column;
    align-items: center;
    width: 100%;
  }

  .chart-body-inner--pie {
    gap: 1rem;
  }

  .chart-body-inner--pie .chart-wrapper {
    max-width: 320px;
    flex: 0 0 auto;
    width: 100%;
  }

  @container (min-width: 420px) {
    .chart-body-inner--pie {
      flex-direction: row;
      align-items: center;
    }

    .chart-body-inner--pie .chart-wrapper {
      width: 55%;
    }

    .chart-body-inner--pie .pie-legend {
      flex: 1;
      min-width: 0;
    }
  }

  .pie-legend {
    list-style: none;
    margin: 0.5rem 0 0;
    padding: 0;
    width: 100%;
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .pie-legend-item {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.75rem;
    color: var(--ds-text-subtle);
    transition: opacity 0.1s ease;
  }

  .pie-legend-item--muted {
    opacity: 0.5;
  }

  .pie-legend-dot {
    width: 0.625rem;
    height: 0.625rem;
    border-radius: 0.125rem;
    flex-shrink: 0;
  }

  .pie-legend-label {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .pie-legend-value {
    font-variant-numeric: tabular-nums;
    color: var(--ds-text);
  }

  .pie-legend-percent {
    font-variant-numeric: tabular-nums;
    min-width: 2.25rem;
    text-align: right;
  }

  .tooltip-percent {
    color: var(--ds-text-subtle);
  }
</style>
