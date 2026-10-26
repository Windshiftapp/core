import { getSource, getWidgetDataContext } from '../context.js';
import { assetSpec, itemSpec, sourceKey } from '../querySpec.js';

/**
 * Resolve the data source for an OpenUI query component (charts, lists,
 * tables, metrics) from the widget data context, requesting the spec on
 * demand when the prefetch did not cover it.
 *
 * The returned object uses getters; read properties through it (for example
 * `chart.source`) so Svelte keeps tracking. Destructuring would freeze the
 * first value.
 *
 * @param {() => any} getProps
 * @param {{ defaultSort?: string }} [options]
 */
export function useChartSource(getProps, options = {}) {
  const { defaultSort } = options;
  const context = getWidgetDataContext();
  const entity = $derived(getProps().entity === 'asset' ? 'asset' : 'item');
  const spec = $derived(
    entity === 'asset'
      ? assetSpec({
          query: getProps().query,
          sort: getProps().sort ?? defaultSort,
          limit: getProps().limit,
          assetSetId: context?.assetSetId,
        })
      : itemSpec({
          query: getProps().query,
          sort: getProps().sort ?? defaultSort,
          limit: getProps().limit,
        })
  );
  const key = $derived(sourceKey(spec));
  const source = $derived(getSource(key));

  // Prefetching covers the stored slots; this covers a spec the prefetch did
  // not know about, so the component never hangs waiting for data.
  $effect(() => {
    if (source) return;
    context?.requestSource?.(spec, key);
  });

  return {
    get context() {
      return context;
    },
    get entity() {
      return entity;
    },
    get spec() {
      return spec;
    },
    get key() {
      return key;
    },
    get source() {
      return source;
    },
  };
}
