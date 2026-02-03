import { fetchAPI } from './core.js';

export const permissions = {
  // Get all available permissions
  getAll: () => fetchAPI('/permissions'),

  // Get user's permissions
  getUserPermissions: (userId) => fetchAPI(`/users/${userId}/permissions`),

  // Grant global permission to user
  grantGlobal: (data) =>
    fetchAPI('/permissions/global/grant', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  // Revoke global permission from user
  revokeGlobal: (userId, permissionId) =>
    fetchAPI(`/users/${userId}/permissions/global/${permissionId}`, {
      method: 'DELETE',
    }),

  // Grant global permission to group
  grantGlobalToGroup: (data) =>
    fetchAPI('/permissions/global/grant-group', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  // Revoke global permission from group
  revokeGlobalFromGroup: (groupId, permissionId) =>
    fetchAPI(`/groups/${groupId}/permissions/global/${permissionId}`, {
      method: 'DELETE',
    }),

  // Get all group permissions
  getAllGroupPermissions: () => fetchAPI('/groups/permissions'),

  // Grant workspace permission
  grantWorkspace: (data) =>
    fetchAPI('/permissions/workspace/grant', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  // Revoke workspace permission
  revokeWorkspace: (userId, workspaceId, permissionId) =>
    fetchAPI(`/users/${userId}/workspaces/${workspaceId}/permissions/${permissionId}`, {
      method: 'DELETE',
    }),

  // Search users without a specific permission (server-side search)
  searchUsersWithoutPermission: (permissionId, query = '', limit = 50) =>
    fetchAPI(
      `/permissions/${permissionId}/available-users?search=${encodeURIComponent(query)}&limit=${limit}`
    ),
};

// Group Management
export const groups = {
  getAll: () => fetchAPI('/groups'),
  get: (id) => fetchAPI(`/groups/${id}`),
  create: (data) =>
    fetchAPI('/groups', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  update: (id, data) =>
    fetchAPI(`/groups/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  delete: (id) =>
    fetchAPI(`/groups/${id}`, {
      method: 'DELETE',
    }),
  addMembers: (groupId, userIds) =>
    fetchAPI(`/groups/${groupId}/members`, {
      method: 'POST',
      body: JSON.stringify({ user_ids: userIds }),
    }),
  removeMembers: (groupId, userIds) =>
    fetchAPI(`/groups/${groupId}/members`, {
      method: 'DELETE',
      body: JSON.stringify({ user_ids: userIds }),
    }),
  getUserMemberships: (userId) => fetchAPI(`/users/${userId}/groups`),
};
