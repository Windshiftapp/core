// Shared data access for the graph components.
//
// All three graph components read the same edge slot, so the spec is built in
// one place. As with the row components, a missing prefetch falls back to an
// on-demand request so a graph never renders an empty state while it waits.

import { getWidgetDataContext } from '../context.js';
import { linksSpec, sourceKey } from '../querySpec.js';

/**
 * The props are read through a getter so Svelte keeps tracking them; passing
 * the props object itself would only capture its initial value.
 *
 * @param {() => { item_query?: string, item_sort?: string, item_limit?: number,
 *                 asset_query?: string, asset_sort?: string, asset_limit?: number,
 *                 max_edges?: number }} getProps
 */
export function useGraphSource(getProps) {
  const context = getWidgetDataContext();

  const spec = $derived(
    linksSpec({
      itemQuery: getProps().item_query,
      itemSort: getProps().item_sort,
      itemLimit: getProps().item_limit,
      assetQuery: getProps().asset_query,
      assetSort: getProps().asset_sort,
      assetLimit: getProps().asset_limit,
      assetSetId: context?.assetSetId,
      maxEdges: getProps().max_edges,
    })
  );
  const key = $derived(sourceKey(spec));
  const source = $derived(context?.sources?.[key] ?? null);
  const graph = $derived(
    source?.graph ?? { nodes: [], edges: [], counts: { items: 0, assets: 0, edges: 0 } }
  );

  $effect(() => {
    if (source) return;
    context?.requestSource?.(spec, key);
  });

  return {
    get source() {
      return source;
    },
    get graph() {
      return graph;
    },
  };
}
