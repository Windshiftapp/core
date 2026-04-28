import { fetchAPI } from './core.js';
import { createCrudClient } from './createCrudClient.js';
import { buildQueryString } from './utils.js';

export const time = {
  projectCategories: {
    ...createCrudClient('/time/project-categories'),
    reorder: (data) =>
      fetchAPI('/time/project-categories/reorder', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
  },

  projects: {
    ...createCrudClient('/time/projects'),
    getByWorkspace: (workspaceId) => fetchAPI(`/workspaces/${workspaceId}/projects`),
    getWorklogs: (id, filters = {}) => {
      return fetchAPI(`/time/projects/${id}/worklogs${buildQueryString(filters)}`);
    },

    // Project Managers
    getManagers: (id) => fetchAPI(`/time/projects/${id}/managers`),
    addManager: (id, managerType, managerId) =>
      fetchAPI(`/time/projects/${id}/managers`, {
        method: 'POST',
        body: JSON.stringify({ manager_type: managerType, manager_id: managerId }),
      }),
    removeManager: (id, managerId) =>
      fetchAPI(`/time/projects/${id}/managers/${managerId}`, {
        method: 'DELETE',
      }),

    // Project Members
    getMembers: (id) => fetchAPI(`/time/projects/${id}/members`),
    addMember: (id, memberType, memberId) =>
      fetchAPI(`/time/projects/${id}/members`, {
        method: 'POST',
        body: JSON.stringify({ member_type: memberType, member_id: memberId }),
      }),
    removeMember: (id, memberId) =>
      fetchAPI(`/time/projects/${id}/members/${memberId}`, {
        method: 'DELETE',
      }),
  },

  worklogs: {
    ...createCrudClient('/time/worklogs'),
    getByItem: (itemId) => fetchAPI(`/items/${itemId}/worklogs`),
  },
};

export const timer = {
  start: (data) =>
    fetchAPI('/timer/start', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  getActive: () => fetchAPI('/timer/active'),
  stop: (id) =>
    fetchAPI(`/timer/${id}/stop`, {
      method: 'DELETE',
    }),
};
