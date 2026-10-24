<script>
  import { t } from '../../stores/i18n.svelte.js';
  import SourceBoundary from './SourceBoundary.svelte';
  import { useGraphSource } from './graphSource.svelte.js';

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

  const RENDERED_NODES_DEFAULT = 200;
  const CELL = 10;

  let canvas = $state(null);
  let canvasUnavailable = $state(false);

  const items = $derived(graph.nodes.filter((node) => node.type === 'item'));
  const assets = $derived(graph.nodes.filter((node) => node.type === 'asset'));
  const renderedLimit = $derived(Math.max(10, Number(props.rendered_nodes) || RENDERED_NODES_DEFAULT));
  const renderedItems = $derived(items.slice(0, renderedLimit));
  const renderedAssets = $derived(assets.slice(0, renderedLimit));
  const rendered = $derived(renderedItems.length < items.length || renderedAssets.length < assets.length);

  const width = $derived(renderedItems.length * CELL);
  const height = $derived(renderedAssets.length * CELL);

  function draw() {
    if (!canvas) return;
    const context = canvas.getContext?.('2d');
    if (!context) {
      canvasUnavailable = true;
      return;
    }
    canvasUnavailable = false;

    const ratio = window.devicePixelRatio || 1;
    canvas.width = Math.max(1, width) * ratio;
    canvas.height = Math.max(1, height) * ratio;
    context.setTransform(ratio, 0, 0, ratio, 0, 0);
    context.clearRect(0, 0, width, height);

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
