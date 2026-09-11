import { fetchV2Data } from '../core.js';

// Test result item linking (new endpoints)
export const testResults = {
  linkItem: (workspaceId, resultId, itemId) =>
    fetchV2Data(`/workspaces/${workspaceId}/test-results/${resultId}/items`, {
      method: 'POST',
      body: JSON.stringify({ item_id: itemId }),
    }),
  unlinkItem: (workspaceId, resultId, itemId) =>
    fetchV2Data(`/workspaces/${workspaceId}/test-results/${resultId}/items/${itemId}`, {
      method: 'DELETE',
    }),
  getLinkedItems: (workspaceId, resultId) =>
    fetchV2Data(`/workspaces/${workspaceId}/test-results/${resultId}/items`),
};
