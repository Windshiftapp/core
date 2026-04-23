import { fetchAPI } from './core.js';
import { buildQueryString } from './utils.js';

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

// Update routes are scope-specific: workspace milestones live at
// /workspaces/{ws}/milestones/{id} (gated by workspace edit permission),
// global milestones at /global/milestones/{id} (gated by milestone.create).
// The helper picks the right URL from data.is_global / data.workspace_id so
// callers don't have to know about the route shape.
function milestoneUpdateUrl(id, data) {
  if (data?.is_global) return `/global/milestones/${id}`;
  if (data?.workspace_id == null) {
    throw new Error('milestone update requires workspace_id when is_global is false');
  }
  return `/workspaces/${data.workspace_id}/milestones/${id}`;
}

export const milestones = {
  getAll: (filters = {}) => {
    return fetchAPI(`/milestones${buildQueryString(filters)}`);
  },
  get: (id) => fetchAPI(`/milestones/${id}`),
  create: (data) =>
    fetchAPI('/milestones', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  update: (id, data) =>
    fetchAPI(milestoneUpdateUrl(id, data), {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  delete: (id) =>
    fetchAPI(`/milestones/${id}`, {
      method: 'DELETE',
    }),
  getTestStatistics: (id) => fetchAPI(`/milestones/${id}/test-statistics`),
  getProgress: (id) => fetchAPI(`/milestones/${id}/progress`),
  release: (id, data) =>
    fetchAPI(`/milestones/${id}/release`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
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

// Iteration update — same scope rules as milestones (see milestoneUpdateUrl).
function iterationUpdateUrl(id, data) {
  if (data?.is_global) return `/global/iterations/${id}`;
  if (data?.workspace_id == null) {
    throw new Error('iteration update requires workspace_id when is_global is false');
  }
  return `/workspaces/${data.workspace_id}/iterations/${id}`;
}

export const iterations = {
  getAll: (filters = {}) => {
    return fetchAPI(`/iterations${buildQueryString(filters)}`);
  },
  get: (id) => fetchAPI(`/iterations/${id}`),
  create: (data) =>
    fetchAPI('/iterations', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  update: (id, data) =>
    fetchAPI(iterationUpdateUrl(id, data), {
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
