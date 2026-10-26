<script>
  import { t } from '../../stores/i18n.svelte.js';
  import SourceBoundary from './SourceBoundary.svelte';
  import { useGraphSource } from './graphSource.svelte.js';
  import { prepareCanvas, splitGraphNodes } from './graphCanvas.js';

  /**
   * Bipartite layout: items in one column, assets in another, links as lines.
   * A force layout was deliberately rejected — at these sizes it produces an
   * unreadable hairball and costs seconds of CPU.
   *
   * @type {{
   *   props: {
   *     item_query?: string, item_sort?: string, item_limit?: number,
   *     asset_query?: string, asset_sort?: string, asset_limit?: number,
   *     max_edges?: number, rendered_nodes?: number
   *   }
   * }}
   */
  let { props } = $props();

  const { source, graph } = useGraphSource(() => props);

  const COLUMN_WIDTH = 220;
  const ROW_HEIGHT = 18;

  let canvas = $state(null);
  let canvasUnavailable = $state(false);

  const { items, assets, renderedItems, renderedAssets, rendered } = $derived(
    splitGraphNodes(graph, props.rendered_nodes, { minRendered: 20 })
  );

  const height = $derived(Math.max(120, Math.max(renderedItems.length, renderedAssets.length) * ROW_HEIGHT + 24));

  function draw() {
    const width = canvas?.clientWidth || 400;
    const context = prepareCanvas(canvas, width, height);
    if (context === undefined) return;
    if (context === null) {
      canvasUnavailable = true;
      return;
    }
    canvasUnavailable = false;

    const itemIndex = new Map(renderedItems.map((node, index) => [node.id, index]));
    const assetIndex = new Map(renderedAssets.map((node, index) => [node.id, index]));
    const assetX = Math.min(COLUMN_WIDTH, width / 2);
    const itemX = width - assetX;

    const yFor = (index) => 20 + index * ROW_HEIGHT;

    context.strokeStyle = 'rgba(120, 120, 130, 0.35)';
    context.lineWidth = 1;
    for (const edge of graph.edges) {
      const from = itemIndex.get(edge.source);
      const to = assetIndex.get(edge.target);
      if (from === undefined || to === undefined) continue;
      context.beginPath();
      context.moveTo(itemX, yFor(from));
      context.lineTo(assetX, yFor(to));
      context.stroke();
    }

    context.font = '11px system-ui, sans-serif';
    context.fillStyle = 'rgba(130, 130, 140, 1)';
    context.textAlign = 'right';
    renderedItems.forEach((node, index) => {
      context.fillText(node.title.slice(0, 26), itemX - 8, yFor(index) + 3);
    });
    context.textAlign = 'left';
    renderedAssets.forEach((node, index) => {
      context.fillText(node.title.slice(0, 26), assetX + 8, yFor(index) + 3);
    });
  }

  $effect(() => {
    // Track the data and the size so a resize redraws.
    void graph;
    void height;
    draw();
  });
</script>

<SourceBoundary {source} loadingRows={4}>
  {#snippet ready()}
    {#if graph.edges.length === 0}
      <p class="text-sm" style="color: var(--ds-text-subtle);" data-testid="openui-network-empty">
        {t('dashboard.customWidgets.noLinks')}
      </p>
    {:else if canvasUnavailable}
      <p class="text-sm" style="color: var(--ds-text-subtle);" data-testid="openui-network-unavailable">
        {t('dashboard.customWidgets.canvasUnavailable')}
      </p>
    {:else}
      <canvas
        bind:this={canvas}
        style="width: 100%; height: {height}px;"
        data-testid="openui-network-canvas"
      ></canvas>
    {/if}
    {#if rendered}
      <p class="mt-2 text-xs" style="color: var(--ds-text-subtle);" data-testid="openui-rendered-nodes-badge">
        {t('dashboard.customWidgets.renderedNodes', {
          shown: Math.max(renderedItems.length, renderedAssets.length),
          total: Math.max(items.length, assets.length),
        })}
      </p>
    {/if}
  {/snippet}
</SourceBoundary>
