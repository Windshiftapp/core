import { fetchAPI } from '../core.js';

export const testRuns = {
  getAll: (workspaceId, params = {}) => {
    const queryParams = new URLSearchParams();
    if (params.assignee_id) queryParams.set('assignee_id', params.assignee_id);
    const queryString = queryParams.toString();
    return fetchAPI(`/workspaces/${workspaceId}/test-runs${queryString ? '?' + queryString : ''}`);
  },
  get: (workspaceId, id) => fetchAPI(`/workspaces/${workspaceId}/test-runs/${id}`),
  create: (workspaceId, data) => fetchAPI(`/workspaces/${workspaceId}/test-runs`, {
    method: 'POST',
    body: JSON.stringify(data),
  }),
  update: (workspaceId, id, data) => fetchAPI(`/workspaces/${workspaceId}/test-runs/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  }),
  delete: (workspaceId, id) => fetchAPI(`/workspaces/${workspaceId}/test-runs/${id}`, {
    method: 'DELETE',
  }),
  end: (workspaceId, id) => fetchAPI(`/workspaces/${workspaceId}/test-runs/${id}/end`, {
    method: 'POST',
  }),
  getResults: (workspaceId, runId) => fetchAPI(`/workspaces/${workspaceId}/test-runs/${runId}/results`),
  updateResult: (workspaceId, runId, resultId, data) => fetchAPI(`/workspaces/${workspaceId}/test-runs/${runId}/results/${resultId}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  }),
  getStepResults: (workspaceId, runId) => fetchAPI(`/workspaces/${workspaceId}/test-runs/${runId}/steps`),
  updateStepResult: (workspaceId, runId, stepId, data) => fetchAPI(`/workspaces/${workspaceId}/test-runs/${runId}/steps/${stepId}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  }),
  getSummary: (workspaceId, runId) => fetchAPI(`/workspaces/${workspaceId}/test-runs/${runId}/summary`),
};
