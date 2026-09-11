import { fetchV2Data } from '../core.js';
import { createCrudClient } from '../createCrudClient.js';

export const testRuns = {
  ...createCrudClient('/test-runs', { parentPath: '/workspaces', v2: true, allV2: true }),
  getDetail: (workspaceId, runId) =>
    fetchV2Data(`/workspaces/${workspaceId}/test-runs/${runId}/detail`),
  end: (workspaceId, id) =>
    fetchV2Data(`/workspaces/${workspaceId}/test-runs/${id}/end`, {
      method: 'POST',
    }),
  getResults: (workspaceId, runId) =>
    fetchV2Data(`/workspaces/${workspaceId}/test-runs/${runId}/results`),
  updateResult: (workspaceId, runId, resultId, data) =>
    fetchV2Data(`/workspaces/${workspaceId}/test-runs/${runId}/results/${resultId}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/merge-patch+json' },
      body: JSON.stringify(data),
    }),
  getStepResults: (workspaceId, runId) =>
    fetchV2Data(`/workspaces/${workspaceId}/test-runs/${runId}/steps`),
  updateStepResult: (workspaceId, runId, stepId, data) =>
    fetchV2Data(`/workspaces/${workspaceId}/test-runs/${runId}/steps/${stepId}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/merge-patch+json' },
      body: JSON.stringify(data),
    }),
  getSummary: (workspaceId, runId) =>
    fetchV2Data(`/workspaces/${workspaceId}/test-runs/${runId}/summary`),
};
