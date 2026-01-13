import { fetchAPI } from './core.js';

export const workspaces = {
  getAll: (filters = {}) => {
    const params = new URLSearchParams();
    Object.entries(filters).forEach(([key, value]) => {
      if (value) params.append(key, value);
    });
    const queryString = params.toString();
    return fetchAPI(`/workspaces${queryString ? '?' + queryString : ''}`);
  },
  get: (id) => fetchAPI(`/workspaces/${id}`),
  create: (data) => fetchAPI('/workspaces', {
    method: 'POST',
    body: JSON.stringify(data),
  }),
  update: (id, data) => fetchAPI(`/workspaces/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  }),
  delete: (id) => fetchAPI(`/workspaces/${id}`, {
    method: 'DELETE',
  }),
  getProjects: (id) => fetchAPI(`/workspaces/${id}/projects`),
  getOrCreatePersonal: () => fetchAPI('/workspaces/personal'),
  getStats: (id, params = {}) => {
    const search = new URLSearchParams();
    Object.entries(params).forEach(([key, value]) => {
      if (value !== undefined && value !== null && value !== '') {
        search.append(key, value);
      }
    });
    const query = search.toString();
    return fetchAPI(`/workspaces/${id}/stats${query ? `?${query}` : ''}`);
  },
  getHomepageLayout: (id) => fetchAPI(`/workspaces/${id}/homepage/layout`),
  updateHomepageLayout: (id, layout) => fetchAPI(`/workspaces/${id}/homepage/layout`, {
    method: 'PUT',
    body: JSON.stringify(layout),
  }),
  getStatuses: (id) => fetchAPI(`/workspaces/${id}/statuses`),
};

export const workspaceRoles = {
  getAll: () => fetchAPI('/workspace-roles'),
  get: (id) => fetchAPI(`/workspace-roles/${id}`),
  getWorkspaceAssignments: (workspaceId) => fetchAPI(`/workspaces/${workspaceId}/role-assignments`),
  getEveryoneRole: (workspaceId) => fetchAPI(`/workspaces/${workspaceId}/everyone-role`),
  setEveryoneRole: (workspaceId, data) => fetchAPI(`/workspaces/${workspaceId}/everyone-role`, {
    method: 'PUT',
    body: JSON.stringify(data),
  }),
  assignToUser: (data) => fetchAPI('/workspace-roles/assign', {
    method: 'POST',
    body: JSON.stringify(data),
  }),
  revokeFromUser: (userId, workspaceId, roleId) => fetchAPI(
    `/users/${userId}/workspaces/${workspaceId}/roles/${roleId}`,
    { method: 'DELETE' }
  ),
  getUserRoles: (userId, workspaceId) => fetchAPI(`/users/${userId}/workspaces/${workspaceId}/roles`),
};
