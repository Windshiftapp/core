import { fetchAPI } from './core.js';
import { createCrudClient } from './createCrudClient.js';
import { buildQueryString } from './utils.js';

export const workspaces = {
  ...createCrudClient('/workspaces'),
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

// `create` and `delete` here go to the new admin endpoints
// (POST /workspace-roles + DELETE /workspace-roles/{id}) which create
// label-only custom roles and refuse to delete is_system rows.
export const workspaceRoles = {
  ...createCrudClient('/workspace-roles'),
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
