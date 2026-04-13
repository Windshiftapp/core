import { fetchAPI } from './core.js';

export const recurrence = {
  // Item-scoped endpoints
  get: (itemId) => fetchAPI(`/items/${itemId}/recurrence`),
  create: (itemId, data) =>
    fetchAPI(`/items/${itemId}/recurrence`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  update: (itemId, data) =>
    fetchAPI(`/items/${itemId}/recurrence`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  delete: (itemId) =>
    fetchAPI(`/items/${itemId}/recurrence`, {
      method: 'DELETE',
    }),
  getInstances: (itemId, params = {}) => {
    const qs = new URLSearchParams();
    if (params.limit) qs.append('limit', params.limit);
    if (params.offset) qs.append('offset', params.offset);
    const query = qs.toString();
    return fetchAPI(`/items/${itemId}/recurrence/instances${query ? `?${query}` : ''}`);
  },
  forceGenerate: (itemId) =>
    fetchAPI(`/items/${itemId}/recurrence/generate`, {
      method: 'POST',
    }),

  // Standalone preview
  preview: (data) =>
    fetchAPI('/recurrence-rules/preview', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  // Workspace-scoped (admin)
  listByWorkspace: (workspaceId) =>
    fetchAPI(`/workspaces/${workspaceId}/recurrence-rules`),
};
