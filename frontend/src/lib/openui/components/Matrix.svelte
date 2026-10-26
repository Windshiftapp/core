<script>
  import { t } from '../../stores/i18n.svelte.js';
  import SourceBoundary from './SourceBoundary.svelte';
  import { useGraphSource } from './graphSource.svelte.js';
  import { prepareCanvas, splitGraphNodes } from './graphCanvas.js';

  /**
   * Item-by-asset matrix. Cells are links. The canvas is required because a
   * 1000 x 1000 DOM grid is not renderable, so the component reports when a
   * canvas is unavailable instead of degrading into something unusable.
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

  const CELL = 10;

  let canvas = $state(null);
  let canvasUnavailable = $state(false);

  const { items, assets, renderedItems, renderedAssets, rendered } = $derived(
    splitGraphNodes(graph, props.rendered_nodes)
  );

  const width = $derived(renderedItems.length * CELL);
  const height = $derived(renderedAssets.length * CELL);

  function draw() {
    const context = prepareCanvas(canvas, width, height);
    if (context === undefined) return;
    if (context === null) {
      canvasUnavailable = true;
      return;
    }
    canvasUnavailable = false;

    const itemIndex = new Map(renderedItems.map((node, index) => [node.id, index]));
    const assetIndex = new Map(renderedAssets.map((node, index) => [node.id, index]));

    context.fillStyle = 'rgba(99, 102, 241, 0.9)';
    for (const edge of graph.edges) {
      const column = itemIndex.get(edge.source);
      const row = assetIndex.get(edge.target);
      if (column === undefined || row === undefined) continue;
      context.fillRect(column * CELL + 1, row * CELL + 1, CELL - 2, CELL - 2);
    }
  }

  $effect(() => {
    void graph;
    void width;
    void height;
    draw();
  });
</script>

<SourceBoundary {source} loadingRows={4}>
  {#snippet ready()}
    {#if graph.edges.length === 0}
      <p class="text-sm" style="color: var(--ds-text-subtle);" data-testid="openui-matrix-empty">
        {t('dashboard.customWidgets.noLinks')}
      </p>
    {:else if canvasUnavailable}
      <p class="text-sm" style="color: var(--ds-text-subtle);" data-testid="openui-matrix-unavailable">
        {t('dashboard.customWidgets.canvasUnavailable')}
      </p>
    {:else}
      <div class="overflow-auto">
        <canvas
          bind:this={canvas}
          style="width: {width}px; height: {height}px;"
          data-testid="openui-matrix-canvas"
        ></canvas>
      </div>
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
