import { api } from '../api.js';

/**
 * @param {number} workspaceId
 * @param {number} testId
 */
export function loadMobileTestCaseSummary(workspaceId, testId) {
  return api.tests.testCases.get(workspaceId, testId);
}
