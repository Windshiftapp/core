import { fetchAPI } from '../core.js';

export const testLabels = {
  getAll: (workspaceId) => fetchAPI(`/workspaces/${workspaceId}/test-labels`),
  create: (workspaceId, data) =>
    fetchAPI(`/workspaces/${workspaceId}/test-labels`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  update: (workspaceId, id, data) =>
    fetchAPI(`/workspaces/${workspaceId}/test-labels/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  delete: (workspaceId, id) =>
    fetchAPI(`/workspaces/${workspaceId}/test-labels/${id}`, {
      method: 'DELETE',
    }),
};
