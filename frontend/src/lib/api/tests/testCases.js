import { fetchAllV2Pages, fetchAPIV2, fetchV2Data } from '../core.js';
import { createCrudClient } from '../createCrudClient.js';

export const testCases = {
  ...createCrudClient('/test-cases', { parentPath: '/workspaces', v2: true }),
  // Custom getAll: callers pass `folder_id: null` (literal string "null" expected
  // by backend) or `all: true`; the generic buildQueryString cannot replicate
  // that, so the override stays bespoke.
  getAll: (workspaceId, params = {}) => {
    const queryParams = new URLSearchParams();
    if (params.all) {
      queryParams.append('all', 'true');
    } else if (params.folder_id !== undefined) {
      queryParams.append('folder_id', params.folder_id === null ? 'null' : params.folder_id);
    }
    if (params.limit) queryParams.append('page_size', String(params.limit));
    if (params.offset && params.limit) {
      queryParams.append('page', String(Math.floor(params.offset / params.limit) + 1));
    }
    if (params.q) queryParams.append('q', params.q);
    if (params.label_id) queryParams.append('label_id', params.label_id);
    const queryString = queryParams.toString();
    const endpoint = `/workspaces/${workspaceId}/test-cases${queryString ? `?${queryString}` : ''}`;
    return params.all ? fetchAllV2Pages(endpoint) : fetchV2Data(endpoint);
  },
  count: async (workspaceId) => {
    const document = await fetchAPIV2(`/workspaces/${workspaceId}/test-cases?page=1&page_size=1`);
    return { count: document?.pagination?.total_items ?? 0 };
  },
  move: (workspaceId, id, data) =>
    fetchV2Data(`/workspaces/${workspaceId}/test-cases/${id}/move`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  reorder: (workspaceId, data) =>
    fetchV2Data(`/workspaces/${workspaceId}/test-cases/reorder`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  connections: (workspaceId, id) =>
    fetchV2Data(`/workspaces/${workspaceId}/test-cases/${id}/connections`),
  // Test Steps
  steps: {
    getAll: (workspaceId, testCaseId) =>
      fetchV2Data(`/workspaces/${workspaceId}/test-cases/${testCaseId}/steps`),
    create: (workspaceId, testCaseId, data) =>
      fetchV2Data(`/workspaces/${workspaceId}/test-cases/${testCaseId}/steps`, {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    update: (workspaceId, testCaseId, stepId, data) =>
      fetchV2Data(`/workspaces/${workspaceId}/test-cases/${testCaseId}/steps/${stepId}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/merge-patch+json' },
        body: JSON.stringify(data),
      }),
    delete: (workspaceId, testCaseId, stepId) =>
      fetchV2Data(`/workspaces/${workspaceId}/test-cases/${testCaseId}/steps/${stepId}`, {
        method: 'DELETE',
      }),
    reorder: (workspaceId, testCaseId, data) =>
      fetchV2Data(`/workspaces/${workspaceId}/test-cases/${testCaseId}/steps/reorder`, {
        method: 'POST',
        body: JSON.stringify(data),
      }),
  },
  // Test Case Labels
  labels: {
    getAll: (workspaceId, testCaseId) =>
      fetchV2Data(`/workspaces/${workspaceId}/test-cases/${testCaseId}/labels`),
    add: (workspaceId, testCaseId, labelId) =>
      fetchV2Data(`/workspaces/${workspaceId}/test-cases/${testCaseId}/labels`, {
        method: 'POST',
        body: JSON.stringify({ label_id: labelId }),
      }),
    remove: (workspaceId, testCaseId, labelId) =>
      fetchV2Data(`/workspaces/${workspaceId}/test-cases/${testCaseId}/labels/${labelId}`, {
        method: 'DELETE',
      }),
  },
};
