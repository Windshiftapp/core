<script>
  import { t } from '../../stores/i18n.svelte.js';
  import SourceBoundary from './SourceBoundary.svelte';
  import { useGraphSource } from './graphSource.svelte.js';

  /**
   * A readable list of the item-to-asset links. This is the default rendering
   * for the edge slot: unlike the canvas views it stays usable at any size and
   * works without a canvas.
   *
   * @type {{
   *   props: {
   *     item_query?: string, item_sort?: string, item_limit?: number,
   *     asset_query?: string, asset_sort?: string, asset_limit?: number,
   *     max_edges?: number
   *   }
   * }}
   */
  let { props } = $props();

  const { source, graph } = useGraphSource(() => props);

  const itemsByID = $derived(new Map(graph.nodes.filter((node) => node.type === 'item').map((node) => [node.id, node])));
  const assetsByID = $derived(new Map(graph.nodes.filter((node) => node.type === 'asset').map((node) => [node.id, node])));

  const groups = $derived.by(() => {
    const byItem = new Map();
    for (const edge of graph.edges) {
      const item = itemsByID.get(edge.source);
      const asset = assetsByID.get(edge.target);
      if (!item || !asset) continue;
      if (!byItem.has(item.id)) byItem.set(item.id, { item, assets: [], assetIDs: new Set() });
      const group = byItem.get(item.id);
      // The same asset can be linked by both a link row and a reference field;
      // it belongs in the list once.
      if (group.assetIDs.has(asset.id)) continue;
      group.assetIDs.add(asset.id);
      group.assets.push(asset);
    }
    return [...byItem.values()];
  });
</script>

<SourceBoundary {source} loadingRows={4}>
  {#snippet ready()}
    {#if groups.length === 0}
      <p class="text-sm" style="color: var(--ds-text-subtle);" data-testid="openui-bipartite-empty">
        {t('dashboard.customWidgets.noLinks')}
      </p>
    {:else}
      <ul class="space-y-3" data-testid="openui-bipartite-list">
        {#each groups as group (group.item.id)}
          <li>
            <div class="text-sm font-medium" style="color: var(--ds-text);">
              {group.item.key ? `${group.item.key} · ` : ''}{group.item.title}
            </div>
            <ul class="mt-1 space-y-0.5 border-l pl-3" style="border-color: var(--ds-border);">
              {#each group.assets as asset (asset.id)}
                <li class="text-xs" style="color: var(--ds-text-subtle);">
                  {asset.key ? `${asset.key} · ` : ''}{asset.title}
                </li>
              {/each}
            </ul>
          </li>
        {/each}
      </ul>
    {/if}
  {/snippet}
</SourceBoundary>
