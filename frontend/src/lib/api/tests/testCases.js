import { fetchAPI } from '../core.js';
import { createCrudClient } from '../createCrudClient.js';

export const testCases = {
  ...createCrudClient('/test-cases', { parentPath: '/workspaces' }),
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
    const queryString = queryParams.toString();
    return fetchAPI(`/workspaces/${workspaceId}/test-cases${queryString ? `?${queryString}` : ''}`);
  },
  move: (workspaceId, id, data) =>
    fetchAPI(`/workspaces/${workspaceId}/test-cases/${id}/move`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  reorder: (workspaceId, data) =>
    fetchAPI(`/workspaces/${workspaceId}/test-cases/reorder`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  connections: (workspaceId, id) =>
    fetchAPI(`/workspaces/${workspaceId}/test-cases/${id}/connections`),
  // Test Steps
  steps: {
    getAll: (workspaceId, testCaseId) =>
      fetchAPI(`/workspaces/${workspaceId}/test-cases/${testCaseId}/steps`),
    create: (workspaceId, testCaseId, data) =>
      fetchAPI(`/workspaces/${workspaceId}/test-cases/${testCaseId}/steps`, {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    update: (workspaceId, testCaseId, stepId, data) =>
      fetchAPI(`/workspaces/${workspaceId}/test-cases/${testCaseId}/steps/${stepId}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    delete: (workspaceId, testCaseId, stepId) =>
      fetchAPI(`/workspaces/${workspaceId}/test-cases/${testCaseId}/steps/${stepId}`, {
        method: 'DELETE',
      }),
    reorder: (workspaceId, testCaseId, data) =>
      fetchAPI(`/workspaces/${workspaceId}/test-cases/${testCaseId}/steps/reorder`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
  },
  // Test Case Labels
  labels: {
    getAll: (workspaceId, testCaseId) =>
      fetchAPI(`/workspaces/${workspaceId}/test-cases/${testCaseId}/labels`),
    add: (workspaceId, testCaseId, labelId) =>
      fetchAPI(`/workspaces/${workspaceId}/test-cases/${testCaseId}/labels`, {
        method: 'POST',
        body: JSON.stringify({ label_id: labelId }),
      }),
    remove: (workspaceId, testCaseId, labelId) =>
      fetchAPI(`/workspaces/${workspaceId}/test-cases/${testCaseId}/labels/${labelId}`, {
        method: 'DELETE',
      }),
  },
};
