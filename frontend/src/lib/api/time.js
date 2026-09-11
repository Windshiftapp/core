import { fetchAllV2Pages, fetchV2Data } from './core.js';
import { createCrudClient } from './createCrudClient.js';
import { buildQueryString } from './utils.js';

function v2WorklogFilters(filters = {}) {
  const { date_from, date_to, ...rest } = filters;
  return {
    ...rest,
    ...(date_from ? { from: date_from } : {}),
    ...(date_to ? { to: date_to } : {}),
  };
}

export const time = {
  projectCategories: {
    ...createCrudClient('/time/project-categories', { v2: true }),
    reorder: (data) =>
      fetchV2Data('/time/project-categories/order', {
        method: 'PUT',
        body: JSON.stringify({ items: data }),
      }),
  },

  projects: {
    ...createCrudClient('/time/projects', { v2: true }),
    getByWorkspace: (workspaceId, requestOptions = {}) =>
      fetchV2Data(`/workspaces/${workspaceId}/time-projects`, requestOptions),
    getWorklogs: (id, filters = {}) => {
      return fetchAllV2Pages(
        `/time/projects/${id}/worklogs${buildQueryString(v2WorklogFilters(filters))}`
      );
    },

    // Project Managers
    getManagers: (id) => fetchV2Data(`/time/projects/${id}/managers`),
    addManager: (id, managerType, managerId) =>
      fetchV2Data(`/time/projects/${id}/managers`, {
        method: 'POST',
        body: JSON.stringify({ principal_type: managerType, principal_id: managerId }),
      }),
    removeManager: (id, managerId) =>
      fetchV2Data(`/time/projects/${id}/managers/${managerId}`, {
        method: 'DELETE',
      }),

    // Project Members
    getMembers: (id) => fetchV2Data(`/time/projects/${id}/members`),
    addMember: (id, memberType, memberId) =>
      fetchV2Data(`/time/projects/${id}/members`, {
        method: 'POST',
        body: JSON.stringify({ principal_type: memberType, principal_id: memberId }),
      }),
    removeMember: (id, memberId) =>
      fetchV2Data(`/time/projects/${id}/members/${memberId}`, {
        method: 'DELETE',
      }),
  },

  worklogs: {
    getAll: (filters = {}) =>
      fetchAllV2Pages(`/time/worklogs${buildQueryString(v2WorklogFilters(filters))}`),
    get: (id, requestOptions = {}) => fetchV2Data(`/time/worklogs/${id}`, requestOptions),
    create: (data) => fetchV2Data('/time/worklogs', { method: 'POST', body: JSON.stringify(data) }),
    update: (id, data) =>
      fetchV2Data(`/time/worklogs/${id}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/merge-patch+json' },
        body: JSON.stringify(data),
      }),
    delete: (id) => fetchV2Data(`/time/worklogs/${id}`, { method: 'DELETE' }),
    getByItem: (itemId, requestOptions = {}) =>
      fetchAllV2Pages(`/items/${itemId}/worklogs`, requestOptions),
  },
};

export const timer = {
  start: (data) =>
    fetchV2Data('/time/timers', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  getActive: () => fetchV2Data('/time/timers/active'),
  stop: () =>
    fetchV2Data('/time/timers/active/stop', {
      method: 'POST',
    }),
};
