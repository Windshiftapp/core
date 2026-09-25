import { fetchAPI } from './core.js';

// Incident state is attached to a work item, so the lifecycle is item-scoped.
export const itemIncidents = {
  get: (itemId) => fetchAPI(`/items/${itemId}/incident`),
  trigger: (itemId, data = {}) =>
    fetchAPI(`/items/${itemId}/incident`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  acknowledge: (itemId) =>
    fetchAPI(`/items/${itemId}/incident/acknowledge`, {
      method: 'POST',
    }),
  unacknowledge: (itemId) =>
    fetchAPI(`/items/${itemId}/incident/unacknowledge`, {
      method: 'POST',
    }),
  resolve: (itemId) =>
    fetchAPI(`/items/${itemId}/incident/resolve`, {
      method: 'POST',
    }),
};
