// Lazy entry point for the OpenUI renderer.
//
// The renderer, the component library, and zod are only loaded when a
// dashboard actually contains a custom widget, so dashboards that use only
// built-in widgets do not pay for them.

/** @type {Promise<import('svelte').Component<any, any, any>> | null} */
let rendererPromise = null;

export function loadCustomWidgetRenderer() {
  if (!rendererPromise) {
    rendererPromise = import('./CustomWidgetRenderer.svelte').then((module) => module.default);
  }
  return rendererPromise;
}
