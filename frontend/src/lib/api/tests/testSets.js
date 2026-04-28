import { fetchAPI } from '../core.js';
import { createCrudClient } from '../createCrudClient.js';

export const testSets = {
  ...createCrudClient('/test-sets', { parentPath: '/workspaces' }),
  getTestCases: (workspaceId, id) =>
    fetchAPI(`/workspaces/${workspaceId}/test-sets/${id}/test-cases`),
  addTestCase: (workspaceId, setId, testCaseId) =>
    fetchAPI(`/workspaces/${workspaceId}/test-sets/${setId}/test-cases`, {
      method: 'POST',
      body: JSON.stringify({ test_case_id: testCaseId }),
    }),
  removeTestCase: (workspaceId, setId, testCaseId) =>
    fetchAPI(`/workspaces/${workspaceId}/test-sets/${setId}/test-cases/${testCaseId}`, {
      method: 'DELETE',
    }),
  getRuns: (workspaceId, setId) => fetchAPI(`/workspaces/${workspaceId}/test-sets/${setId}/runs`),
};
