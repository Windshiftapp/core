import { fetchAPI } from '../core.js';

// Test Plans (preferred terminology, same as testSets)
export const testPlans = {
  getAll: (workspaceId) => fetchAPI(`/workspaces/${workspaceId}/test-plans`),
  get: (workspaceId, id) => fetchAPI(`/workspaces/${workspaceId}/test-plans/${id}`),
  create: (workspaceId, data) => fetchAPI(`/workspaces/${workspaceId}/test-plans`, {
    method: 'POST',
    body: JSON.stringify(data),
  }),
  update: (workspaceId, id, data) => fetchAPI(`/workspaces/${workspaceId}/test-plans/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  }),
  delete: (workspaceId, id) => fetchAPI(`/workspaces/${workspaceId}/test-plans/${id}`, {
    method: 'DELETE',
  }),
  getTestCases: (workspaceId, id) => fetchAPI(`/workspaces/${workspaceId}/test-plans/${id}/test-cases`),
  addTestCase: (workspaceId, planId, testCaseId) => fetchAPI(`/workspaces/${workspaceId}/test-plans/${planId}/test-cases`, {
    method: 'POST',
    body: JSON.stringify({ test_case_id: testCaseId }),
  }),
  removeTestCase: (workspaceId, planId, testCaseId) => fetchAPI(`/workspaces/${workspaceId}/test-plans/${planId}/test-cases/${testCaseId}`, {
    method: 'DELETE',
  }),
  getRuns: (workspaceId, planId) => fetchAPI(`/workspaces/${workspaceId}/test-plans/${planId}/runs`),
};
