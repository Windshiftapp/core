import { fetchAPI } from '../core.js';

export const testCases = {
  getAll: (workspaceId, params = {}) => {
    const queryParams = new URLSearchParams();
    if (params.all) {
      queryParams.append('all', 'true');
    } else if (params.folder_id !== undefined) {
      queryParams.append('folder_id', params.folder_id === null ? 'null' : params.folder_id);
    }
    const queryString = queryParams.toString();
    return fetchAPI(`/workspaces/${workspaceId}/test-cases${queryString ? '?' + queryString : ''}`);
  },
  get: (workspaceId, id) => fetchAPI(`/workspaces/${workspaceId}/test-cases/${id}`),
  create: (workspaceId, data) => fetchAPI(`/workspaces/${workspaceId}/test-cases`, {
    method: 'POST',
    body: JSON.stringify(data),
  }),
  update: (workspaceId, id, data) => fetchAPI(`/workspaces/${workspaceId}/test-cases/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  }),
  delete: (workspaceId, id) => fetchAPI(`/workspaces/${workspaceId}/test-cases/${id}`, {
    method: 'DELETE',
  }),
  move: (workspaceId, id, data) => fetchAPI(`/workspaces/${workspaceId}/test-cases/${id}/move`, {
    method: 'PUT',
    body: JSON.stringify(data),
  }),
  reorder: (workspaceId, data) => fetchAPI(`/workspaces/${workspaceId}/test-cases/reorder`, {
    method: 'PUT',
    body: JSON.stringify(data),
  }),
  connections: (workspaceId, id) => fetchAPI(`/workspaces/${workspaceId}/test-cases/${id}/connections`),
  // Test Steps
  steps: {
    getAll: (workspaceId, testCaseId) => fetchAPI(`/workspaces/${workspaceId}/test-cases/${testCaseId}/steps`),
    create: (workspaceId, testCaseId, data) => fetchAPI(`/workspaces/${workspaceId}/test-cases/${testCaseId}/steps`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
    update: (workspaceId, testCaseId, stepId, data) => fetchAPI(`/workspaces/${workspaceId}/test-cases/${testCaseId}/steps/${stepId}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
    delete: (workspaceId, testCaseId, stepId) => fetchAPI(`/workspaces/${workspaceId}/test-cases/${testCaseId}/steps/${stepId}`, {
      method: 'DELETE',
    }),
    reorder: (workspaceId, testCaseId, data) => fetchAPI(`/workspaces/${workspaceId}/test-cases/${testCaseId}/steps/reorder`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  },
  // Test Case Labels
  labels: {
    getAll: (workspaceId, testCaseId) => fetchAPI(`/workspaces/${workspaceId}/test-cases/${testCaseId}/labels`),
    add: (workspaceId, testCaseId, labelId) => fetchAPI(`/workspaces/${workspaceId}/test-cases/${testCaseId}/labels`, {
      method: 'POST',
      body: JSON.stringify({ label_id: labelId }),
    }),
    remove: (workspaceId, testCaseId, labelId) => fetchAPI(`/workspaces/${workspaceId}/test-cases/${testCaseId}/labels/${labelId}`, {
      method: 'DELETE',
    }),
  }
};
