import { fetchAPI } from './core.js';

export const time = {
  projectCategories: {
    getAll: () => fetchAPI('/time/project-categories'),
    get: (id) => fetchAPI(`/time/project-categories/${id}`),
    create: (data) =>
      fetchAPI('/time/project-categories', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    update: (id, data) =>
      fetchAPI(`/time/project-categories/${id}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    delete: (id) =>
      fetchAPI(`/time/project-categories/${id}`, {
        method: 'DELETE',
      }),
    reorder: (data) =>
      fetchAPI('/time/project-categories/reorder', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
  },

  projects: {
    getAll: () => fetchAPI('/time/projects'),
    getByWorkspace: (workspaceId) => fetchAPI(`/workspaces/${workspaceId}/projects`),
    get: (id) => fetchAPI(`/time/projects/${id}`),
    create: (data) =>
      fetchAPI('/time/projects', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    update: (id, data) =>
      fetchAPI(`/time/projects/${id}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    delete: (id) =>
      fetchAPI(`/time/projects/${id}`, {
        method: 'DELETE',
      }),
    getWorklogs: (id) => fetchAPI(`/time/projects/${id}/worklogs`),

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
    getAll: (filters = {}) => {
      const params = new URLSearchParams();
      Object.entries(filters).forEach(([key, value]) => {
        if (value) params.append(key, value);
      });
      const queryString = params.toString();
      return fetchAPI(`/time/worklogs${queryString ? `?${queryString}` : ''}`);
    },
    get: (id) => fetchAPI(`/time/worklogs/${id}`),
    getByItem: (itemId) => fetchAPI(`/items/${itemId}/worklogs`),
    create: (data) =>
      fetchAPI('/time/worklogs', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    update: (id, data) =>
      fetchAPI(`/time/worklogs/${id}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    delete: (id) =>
      fetchAPI(`/time/worklogs/${id}`, {
        method: 'DELETE',
      }),
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
