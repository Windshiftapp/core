import { fetchAllV2Pages, fetchAPI, fetchV2Data } from './core.js';
import { createCrudClient } from './createCrudClient.js';
import { buildQueryString } from './utils.js';
import { normalizeStatuses } from './workflows.js';

const workspaceCRUD = createCrudClient('/workspaces', { v2: true, allV2: true });

export const workspaces = {
  ...workspaceCRUD,
  getAll: (filters = {}, requestOptions = {}) =>
    fetchAllV2Pages(`/workspaces${buildQueryString(filters)}`, requestOptions),
  get: (id, requestOptions = {}) => fetchV2Data(`/workspaces/${id}`, requestOptions),
  getBootstrap: (id) => fetchAPI(`/workspaces/${id}/bootstrap`),
  getProjects: (id) => fetchV2Data(`/workspaces/${id}/time-projects`),
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
  getStatuses: async (id, itemTypeId = null) =>
    normalizeStatuses(
      await fetchV2Data(
        `/workspaces/${id}/statuses${itemTypeId ? `?item_type_id=${itemTypeId}` : ''}`
      )
    ),
  getItemTypes: (id) => fetchV2Data(`/workspaces/${id}/item-types`),
  getWorkflows: (id) => fetchV2Data(`/workspaces/${id}/workflows`),
  getPriorities: (id) => fetchV2Data(`/workspaces/${id}/priorities`),
  getTemplates: () => fetchV2Data('/workspace-templates'),
  // Allowed status transitions for every (item_type_id, status_id) pair in the
  // workspace, keyed "<itemTypeId>:<statusId>". One request replaces the
  // board's per-pair /items/{id}/available-status-transitions preload.
  getTransitionMatrix: async (id) => {
    const entries = await fetchV2Data(`/workspaces/${id}/transition-matrix`);
    return Object.fromEntries(
      (entries || []).map(({ item_type_id: itemTypeId, status_id: statusId, transitions }) => [
        `${itemTypeId}:${statusId}`,
        transitions,
      ])
    );
  },
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
  getWorkspaceGroupAssignments: (workspaceId) =>
    fetchAPI(`/workspaces/${workspaceId}/group-role-assignments`),
  assignToGroup: (data) =>
    fetchAPI('/workspace-roles/assign-group', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  revokeFromGroup: (groupId, workspaceId, roleId) =>
    fetchAPI(`/groups/${groupId}/workspaces/${workspaceId}/roles/${roleId}`, { method: 'DELETE' }),
};
