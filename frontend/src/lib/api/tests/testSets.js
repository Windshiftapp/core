import { fetchAPI } from '../core.js';

export const testSets = {
  getAll: (workspaceId) => fetchAPI(`/workspaces/${workspaceId}/test-sets`),
  get: (workspaceId, id) => fetchAPI(`/workspaces/${workspaceId}/test-sets/${id}`),
  create: (workspaceId, data) => fetchAPI(`/workspaces/${workspaceId}/test-sets`, {
    method: 'POST',
    body: JSON.stringify(data),
  }),
  update: (workspaceId, id, data) => fetchAPI(`/workspaces/${workspaceId}/test-sets/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  }),
  delete: (workspaceId, id) => fetchAPI(`/workspaces/${workspaceId}/test-sets/${id}`, {
    method: 'DELETE',
  }),
  getTestCases: (workspaceId, id) => fetchAPI(`/workspaces/${workspaceId}/test-sets/${id}/test-cases`),
  addTestCase: (workspaceId, setId, testCaseId) => fetchAPI(`/workspaces/${workspaceId}/test-sets/${setId}/test-cases`, {
    method: 'POST',
    body: JSON.stringify({ test_case_id: testCaseId }),
  }),
  removeTestCase: (workspaceId, setId, testCaseId) => fetchAPI(`/workspaces/${workspaceId}/test-sets/${setId}/test-cases/${testCaseId}`, {
    method: 'DELETE',
  }),
  getRuns: (workspaceId, setId) => fetchAPI(`/workspaces/${workspaceId}/test-sets/${setId}/runs`),
};
