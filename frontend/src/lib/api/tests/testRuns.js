import { fetchAPI } from '../core.js';
import { createCrudClient } from '../createCrudClient.js';

export const testRuns = {
  ...createCrudClient('/test-runs', { parentPath: '/workspaces' }),
  end: (workspaceId, id) =>
    fetchAPI(`/workspaces/${workspaceId}/test-runs/${id}/end`, {
      method: 'POST',
    }),
  getResults: (workspaceId, runId) =>
    fetchAPI(`/workspaces/${workspaceId}/test-runs/${runId}/results`),
  updateResult: (workspaceId, runId, resultId, data) =>
    fetchAPI(`/workspaces/${workspaceId}/test-runs/${runId}/results/${resultId}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  getStepResults: (workspaceId, runId) =>
    fetchAPI(`/workspaces/${workspaceId}/test-runs/${runId}/steps`),
  updateStepResult: (workspaceId, runId, stepId, data) =>
    fetchAPI(`/workspaces/${workspaceId}/test-runs/${runId}/steps/${stepId}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  getSummary: (workspaceId, runId) =>
    fetchAPI(`/workspaces/${workspaceId}/test-runs/${runId}/summary`),
};
