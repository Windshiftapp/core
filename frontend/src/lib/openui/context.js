// Widget data context.
//
// The OpenUI renderer owns its own context (library, form state) and does not
// allow extra fields, so widget data travels in a separate Svelte context set
// by the wrapper around <Renderer>. Components read it with
// getWidgetDataContext().

import { getContext, setContext } from 'svelte';

const WIDGET_DATA_KEY = Symbol('windshift-openui-widget-data');

/**
 * @typedef {object} SourceResult
 * @property {'loading'|'ready'|'error'} status
 * @property {Array<object>} [rows]
 * @property {number} [total]
 * @property {boolean} [truncated]
 * @property {{ code?: string, message?: string }} [error]
 * @property {object} [graph] bipartite nodes/edges for an edge slot
 */

/**
 * @typedef {object} WidgetDataContext
 * @property {Record<string, SourceResult>} sources keyed by sourceKey(spec)
 * @property {number|null} [assetSetId] set chosen in the builder for asset queries
 * @property {(spec: object, key: string) => void} [requestSource] fetch a spec the prefetch missed
 * @property {(itemId: number) => void} [openItem]
 * @property {number|null} [workspaceId]
 * @property {string} [locale]
 */

/** @param {WidgetDataContext} value */
export function setWidgetDataContext(value) {
  setContext(WIDGET_DATA_KEY, value);
}

/** @returns {WidgetDataContext|null} */
export function getWidgetDataContext() {
  try {
    return getContext(WIDGET_DATA_KEY) ?? null;
  } catch {
    return null;
  }
}

/** @returns {SourceResult|null} */
export function getSource(key) {
  const context = getWidgetDataContext();
  if (!context?.sources) return null;
  return context.sources[key] ?? null;
}
