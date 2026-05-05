import { fetchAPI } from './core.js';

export const actions = {
  getAll: (workspaceId) => fetchAPI(`/workspaces/${workspaceId}/actions`),
  get: (workspaceId, id) => fetchAPI(`/workspaces/${workspaceId}/actions/${id}`),
  create: (workspaceId, data) =>
    fetchAPI(`/workspaces/${workspaceId}/actions`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  update: (workspaceId, id, data) =>
    fetchAPI(`/workspaces/${workspaceId}/actions/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  delete: (workspaceId, id) =>
    fetchAPI(`/workspaces/${workspaceId}/actions/${id}`, {
      method: 'DELETE',
    }),
  execute: (workspaceId, actionId, itemId) =>
    fetchAPI(`/workspaces/${workspaceId}/actions/${actionId}/execute`, {
      method: 'POST',
      body: JSON.stringify({ item_id: itemId }),
    }),
  getLogs: (workspaceId, actionId) =>
    fetchAPI(`/workspaces/${workspaceId}/actions/${actionId}/logs`),
};

// Action templates: read-only registry shipped with the binary, plus
// instantiation into a workspace via snapshot copy.
export const actionTemplates = {
  list: () => fetchAPI('/action-templates'),
  apply: (workspaceId, templateKey) =>
    fetchAPI(`/workspaces/${workspaceId}/action-templates/${templateKey}/apply`, {
      method: 'POST',
    }),
};
