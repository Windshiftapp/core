// Shared helpers for the bipartite graph widgets (Matrix, NetworkGraph).
// Pure functions only, so the components keep their own reactive state.

const RENDERED_NODES_DEFAULT = 200;

/**
 * Split graph nodes into item/asset lists and apply the render limit.
 *
 * @param {{ nodes?: Array<{ id: number|string, type: string, title?: string }> }} graph
 * @param {number|undefined} renderedNodes prop override for the per-side limit
 * @param {{ minRendered?: number }} [options]
 */
export function splitGraphNodes(graph, renderedNodes, options = {}) {
  const { minRendered = 10 } = options;
  const nodes = graph?.nodes ?? [];
  const items = nodes.filter((node) => node.type === 'item');
  const assets = nodes.filter((node) => node.type === 'asset');
  const renderedLimit = Math.max(minRendered, Number(renderedNodes) || RENDERED_NODES_DEFAULT);
  const renderedItems = items.slice(0, renderedLimit);
  const renderedAssets = assets.slice(0, renderedLimit);
  const rendered = renderedItems.length < items.length || renderedAssets.length < assets.length;
  return { items, assets, renderedItems, renderedAssets, rendered };
}

/**
 * Size a canvas for the device pixel ratio, clear it, and return a context
 * whose coordinates are in CSS pixels.
 *
 * @param {HTMLCanvasElement | null} canvasEl
 * @param {number} width
 * @param {number} height
 * @returns {CanvasRenderingContext2D | null | undefined} undefined when the
 *   canvas is not mounted yet, null when a 2d context is unavailable, else the
 *   ready context.
 */
export function prepareCanvas(canvasEl, width, height) {
  if (!canvasEl) return undefined;
  const context = canvasEl.getContext?.('2d');
  if (!context) return null;

  const ratio = window.devicePixelRatio || 1;
  canvasEl.width = Math.max(1, width) * ratio;
  canvasEl.height = Math.max(1, height) * ratio;
  context.setTransform(ratio, 0, 0, ratio, 0, 0);
  context.clearRect(0, 0, width, height);
  return context;
}
