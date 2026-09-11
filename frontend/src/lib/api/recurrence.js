import { fetchAPIV2, fetchV2Data } from './core.js';
import { buildQueryString } from './utils.js';

export const recurrence = {
  // Item-scoped endpoints
  get: (itemId) => fetchV2Data(`/items/${itemId}/recurrence`),
  create: (itemId, { is_active: _isActive, ...data }) =>
    fetchV2Data(`/items/${itemId}/recurrence`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  update: (itemId, data) =>
    fetchV2Data(`/items/${itemId}/recurrence`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/merge-patch+json' },
      body: JSON.stringify(data),
    }),
  delete: (itemId) =>
    fetchV2Data(`/items/${itemId}/recurrence`, {
      method: 'DELETE',
    }),
  getInstances: (itemId, params = {}) => {
    return fetchAPIV2(`/items/${itemId}/recurrence/instances${buildQueryString(params)}`);
  },
  forceGenerate: (itemId) =>
    fetchV2Data(`/items/${itemId}/recurrence/generate`, {
      method: 'POST',
    }),

  // Standalone preview
  preview: (data) =>
    fetchV2Data('/recurrence-rules/preview', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  // Workspace-scoped (admin)
  listByWorkspace: (workspaceId) => fetchV2Data(`/workspaces/${workspaceId}/recurrence-rules`),
};
