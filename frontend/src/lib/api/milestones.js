import { fetchAPI } from './core.js';
import { createCrudClient } from './createCrudClient.js';

export const milestoneCategories = createCrudClient('/milestone-categories');

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
  ...createCrudClient('/milestones'),
  // Override update: the URL depends on data.is_global / data.workspace_id.
  update: (id, data) =>
    fetchAPI(milestoneUpdateUrl(id, data), {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  getTestStatistics: (id) => fetchAPI(`/milestones/${id}/test-statistics`),
  getProgress: (id) => fetchAPI(`/milestones/${id}/progress`),
  release: (id, data) =>
    fetchAPI(`/milestones/${id}/release`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
};

export const iterationTypes = createCrudClient('/iteration-types');

// Iteration update — same scope rules as milestones (see milestoneUpdateUrl).
function iterationUpdateUrl(id, data) {
  if (data?.is_global) return `/global/iterations/${id}`;
  if (data?.workspace_id == null) {
    throw new Error('iteration update requires workspace_id when is_global is false');
  }
  return `/workspaces/${data.workspace_id}/iterations/${id}`;
}

export const iterations = {
  ...createCrudClient('/iterations'),
  // Override update: the URL depends on data.is_global / data.workspace_id.
  update: (id, data) =>
    fetchAPI(iterationUpdateUrl(id, data), {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  getProgress: (id) => fetchAPI(`/iterations/${id}/progress`),
  getBurndown: (id) => fetchAPI(`/iterations/${id}/burndown`),
};
