import { fetchAPI } from '../core.js';

export const testFolders = {
  getAll: (workspaceId) => fetchAPI(`/workspaces/${workspaceId}/test-folders`),
  get: (workspaceId, id) => fetchAPI(`/workspaces/${workspaceId}/test-folders/${id}`),
  create: (workspaceId, data) =>
    fetchAPI(`/workspaces/${workspaceId}/test-folders`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  update: (workspaceId, id, data) =>
    fetchAPI(`/workspaces/${workspaceId}/test-folders/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  delete: (workspaceId, id) =>
    fetchAPI(`/workspaces/${workspaceId}/test-folders/${id}`, {
      method: 'DELETE',
    }),
  reorder: (workspaceId, data) =>
    fetchAPI(`/workspaces/${workspaceId}/test-folders/reorder`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
};
