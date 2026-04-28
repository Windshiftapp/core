import { fetchAPI } from '../core.js';
import { createCrudClient } from '../createCrudClient.js';

// Test Plans (preferred terminology, same as testSets)
export const testPlans = {
  ...createCrudClient('/test-plans', { parentPath: '/workspaces' }),
  getTestCases: (workspaceId, id) =>
    fetchAPI(`/workspaces/${workspaceId}/test-plans/${id}/test-cases`),
  addTestCase: (workspaceId, planId, testCaseId) =>
    fetchAPI(`/workspaces/${workspaceId}/test-plans/${planId}/test-cases`, {
      method: 'POST',
      body: JSON.stringify({ test_case_id: testCaseId }),
    }),
  removeTestCase: (workspaceId, planId, testCaseId) =>
    fetchAPI(`/workspaces/${workspaceId}/test-plans/${planId}/test-cases/${testCaseId}`, {
      method: 'DELETE',
    }),
  getRuns: (workspaceId, planId) =>
    fetchAPI(`/workspaces/${workspaceId}/test-plans/${planId}/runs`),
};
