import { fetchAllV2Pages, fetchAPI, fetchV2Data } from './core.js';

export const permissions = {
  // Get all available permissions
  getAll: () => fetchAPI('/permissions'),

  // Get user's permissions
  getUserPermissions: (userId) => fetchAPI(`/users/${userId}/permissions`),

  // Get effective global permission assignments for all users
  getAllUserGlobalPermissions: () => fetchAPI('/users/permissions/global'),

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
  getAdminAll: () => fetchAllV2Pages('/admin/groups'),
  get: (groupId) => fetchV2Data(`/admin/groups/${groupId}`),
  create: (data) => fetchV2Data('/admin/groups', { method: 'POST', body: JSON.stringify(data) }),
  update: (groupId, data) =>
    fetchV2Data(`/admin/groups/${groupId}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/merge-patch+json' },
      body: JSON.stringify(data),
    }),
  delete: (groupId) => fetchV2Data(`/admin/groups/${groupId}`, { method: 'DELETE' }),
  addMembers: async (groupId, userIds) => {
    const group = await fetchV2Data(`/admin/groups/${groupId}`);
    return fetchV2Data(`/admin/groups/${groupId}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/merge-patch+json' },
      body: JSON.stringify({ member_ids: [...new Set([...group.member_ids, ...userIds])] }),
    });
  },
  removeMembers: async (groupId, userIds) => {
    const group = await fetchV2Data(`/admin/groups/${groupId}`);
    return fetchV2Data(`/admin/groups/${groupId}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/merge-patch+json' },
      body: JSON.stringify({ member_ids: group.member_ids.filter((id) => !userIds.includes(id)) }),
    });
  },
  getUserMemberships: (userId) => fetchAPI(`/users/${userId}/groups`),
};
