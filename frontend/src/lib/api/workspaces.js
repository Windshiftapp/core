import { fetchAPI } from './core.js';
import { buildQueryString } from './utils.js';

export const workspaces = {
  getAll: (filters = {}) => {
    return fetchAPI(`/workspaces${buildQueryString(filters)}`);
  },
  get: (id) => fetchAPI(`/workspaces/${id}`),
  create: (data) =>
    fetchAPI('/workspaces', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  update: (id, data) =>
    fetchAPI(`/workspaces/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  delete: (id) =>
    fetchAPI(`/workspaces/${id}`, {
      method: 'DELETE',
    }),
  getProjects: (id) => fetchAPI(`/workspaces/${id}/projects`),
  getOrCreatePersonal: () => fetchAPI('/workspaces/personal'),
  getStats: (id, params = {}) => {
    return fetchAPI(`/workspaces/${id}/stats${buildQueryString(params)}`);
  },
  getHomepageLayout: (id) => fetchAPI(`/workspaces/${id}/homepage/layout`),
  updateHomepageLayout: (id, layout) =>
    fetchAPI(`/workspaces/${id}/homepage/layout`, {
      method: 'PUT',
      body: JSON.stringify(layout),
    }),
  getStatuses: (id) => fetchAPI(`/workspaces/${id}/statuses`),
};

export const workspaceRoles = {
  getAll: () => fetchAPI('/workspace-roles'),
  get: (id) => fetchAPI(`/workspace-roles/${id}`),
  getWorkspaceAssignments: (workspaceId) => fetchAPI(`/workspaces/${workspaceId}/role-assignments`),
  assignToUser: (data) =>
    fetchAPI('/workspace-roles/assign', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  revokeFromUser: (userId, workspaceId, roleId) =>
    fetchAPI(`/users/${userId}/workspaces/${workspaceId}/roles/${roleId}`, { method: 'DELETE' }),
  getUserRoles: (userId, workspaceId) =>
    fetchAPI(`/users/${userId}/workspaces/${workspaceId}/roles`),
};
