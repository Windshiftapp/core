import { fetchAPI } from './core.js';

/**
 * Custom dashboard widget API.
 *
 * The data endpoint is the only one that takes query slots; the program itself
 * is opaque to the client API layer.
 */
export const customWidgets = {
  list: () => fetchAPI('/custom-widgets'),

  get: (id) => fetchAPI(`/custom-widgets/${encodeURIComponent(id)}`),

  create: (widget) => fetchAPI('/custom-widgets', { method: 'POST', body: JSON.stringify(widget) }),

  update: (id, widget) =>
    fetchAPI(`/custom-widgets/${encodeURIComponent(id)}`, {
      method: 'PUT',
      body: JSON.stringify(widget),
    }),

  remove: (id) => fetchAPI(`/custom-widgets/${encodeURIComponent(id)}`, { method: 'DELETE' }),

  validate: (widget) =>
    fetchAPI('/custom-widgets/validate', { method: 'POST', body: JSON.stringify(widget) }),

  /**
   * Generate or refine a widget from a description. Rate limited server-side.
   * Pass current_program/current_queries to refine an existing widget.
   */
  generate: (payload) =>
    fetchAPI('/custom-widgets/generate', { method: 'POST', body: JSON.stringify(payload) }),

  /**
   * Resolve query slots. Pass either widget_id (a saved widget) or workspace_id
   * (unsaved preview). Slots carry no workspace or user IDs.
   */
  data: (payload) =>
    fetchAPI('/custom-widgets/data', { method: 'POST', body: JSON.stringify(payload) }),
};
