import { fetchV2Data } from '../core.js';
import { createCrudClient } from '../createCrudClient.js';

export const testRuns = {
  ...createCrudClient('/test-runs', { parentPath: '/workspaces', v2: true }),
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
  // BDD example execution: one result row per Scenario Outline example row,
  // created when the run starts against the frozen spec snapshot.
  getExampleResults: (workspaceId, runId) =>
    fetchV2Data(`/workspaces/${workspaceId}/test-runs/${runId}/example-results`),
  updateExampleResult: (workspaceId, runId, testCaseId, exampleIndex, data) =>
    fetchV2Data(
      `/workspaces/${workspaceId}/test-runs/${runId}/test-cases/${testCaseId}/examples/${exampleIndex}`,
      {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/merge-patch+json' },
        body: JSON.stringify(data),
      }
    ),
  updateExampleStepResult: (workspaceId, runId, testCaseId, exampleIndex, stepNumber, data) =>
    fetchV2Data(
      `/workspaces/${workspaceId}/test-runs/${runId}/test-cases/${testCaseId}/examples/${exampleIndex}/steps/${stepNumber}`,
      {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/merge-patch+json' },
        body: JSON.stringify(data),
      }
    ),
  updateStepResult: (workspaceId, runId, stepId, data) =>
    fetchV2Data(`/workspaces/${workspaceId}/test-runs/${runId}/steps/${stepId}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/merge-patch+json' },
      body: JSON.stringify(data),
    }),
  getSummary: (workspaceId, runId) =>
    fetchV2Data(`/workspaces/${workspaceId}/test-runs/${runId}/summary`),
};
