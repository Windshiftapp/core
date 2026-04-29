import { fetchAPI } from './core.js';
import { createCrudClient } from './createCrudClient.js';

export const teams = {
  ...createCrudClient('/teams'),

  getResolvedMembers: (teamId) => fetchAPI(`/teams/${teamId}/resolved-members`),

  addMembers: (teamId, userIds, role = 'member') =>
    fetchAPI(`/teams/${teamId}/members`, {
      method: 'POST',
      body: JSON.stringify({ user_ids: userIds, role }),
    }),

  removeMembers: (teamId, userIds) =>
    fetchAPI(`/teams/${teamId}/members`, {
      method: 'DELETE',
      body: JSON.stringify({ user_ids: userIds }),
    }),

  updateMemberRole: (teamId, userId, role) =>
    fetchAPI(`/teams/${teamId}/members/${userId}/role`, {
      method: 'PUT',
      body: JSON.stringify({ role }),
    }),

  addGroups: (teamId, groupIds) =>
    fetchAPI(`/teams/${teamId}/groups`, {
      method: 'POST',
      body: JSON.stringify({ group_ids: groupIds }),
    }),

  removeGroups: (teamId, groupIds) =>
    fetchAPI(`/teams/${teamId}/groups`, {
      method: 'DELETE',
      body: JSON.stringify({ group_ids: groupIds }),
    }),

  getTeamsForUser: (userId) => fetchAPI(`/users/${userId}/teams`),
};
