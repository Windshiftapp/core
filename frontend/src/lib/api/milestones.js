import { fetchAPI } from './core.js';

export const milestoneCategories = {
  getAll: () => fetchAPI('/milestone-categories'),
  get: (id) => fetchAPI(`/milestone-categories/${id}`),
  create: (data) =>
    fetchAPI('/milestone-categories', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  update: (id, data) =>
    fetchAPI(`/milestone-categories/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  delete: (id) =>
    fetchAPI(`/milestone-categories/${id}`, {
      method: 'DELETE',
    }),
};

export const milestones = {
  getAll: (filters = {}) => {
    const params = new URLSearchParams();
    Object.entries(filters).forEach(([key, value]) => {
      if (value !== null && value !== undefined && value !== '') {
        params.append(key, value);
      }
    });
    const queryString = params.toString();
    return fetchAPI(`/milestones${queryString ? `?${queryString}` : ''}`);
  },
  get: (id) => fetchAPI(`/milestones/${id}`),
  create: (data) =>
    fetchAPI('/milestones', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  update: (id, data) =>
    fetchAPI(`/milestones/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  delete: (id) =>
    fetchAPI(`/milestones/${id}`, {
      method: 'DELETE',
    }),
  getTestStatistics: (id) => fetchAPI(`/milestones/${id}/test-statistics`),
  getProgress: (id) => fetchAPI(`/milestones/${id}/progress`),
};

export const iterationTypes = {
  getAll: () => fetchAPI('/iteration-types'),
  get: (id) => fetchAPI(`/iteration-types/${id}`),
  create: (data) =>
    fetchAPI('/iteration-types', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  update: (id, data) =>
    fetchAPI(`/iteration-types/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  delete: (id) =>
    fetchAPI(`/iteration-types/${id}`, {
      method: 'DELETE',
    }),
};

export const iterations = {
  getAll: (filters = {}) => {
    const params = new URLSearchParams();
    Object.entries(filters).forEach(([key, value]) => {
      if (value !== null && value !== undefined && value !== '') {
        params.append(key, value);
      }
    });
    const queryString = params.toString();
    return fetchAPI(`/iterations${queryString ? `?${queryString}` : ''}`);
  },
  get: (id) => fetchAPI(`/iterations/${id}`),
  create: (data) =>
    fetchAPI('/iterations', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  update: (id, data) =>
    fetchAPI(`/iterations/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  delete: (id) =>
    fetchAPI(`/iterations/${id}`, {
      method: 'DELETE',
    }),
  getProgress: (id) => fetchAPI(`/iterations/${id}/progress`),
  getBurndown: (id) => fetchAPI(`/iterations/${id}/burndown`),
};
