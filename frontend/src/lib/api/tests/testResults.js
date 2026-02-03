import { fetchAPI } from '../core.js';

// Test result item linking (new endpoints)
export const testResults = {
  linkItem: (workspaceId, resultId, itemId) =>
    fetchAPI(`/workspaces/${workspaceId}/test-results/${resultId}/items`, {
      method: 'POST',
      body: JSON.stringify({ item_id: itemId }),
    }),
  unlinkItem: (workspaceId, resultId, itemId) =>
    fetchAPI(`/workspaces/${workspaceId}/test-results/${resultId}/items/${itemId}`, {
      method: 'DELETE',
    }),
  getLinkedItems: (workspaceId, resultId) =>
    fetchAPI(`/workspaces/${workspaceId}/test-results/${resultId}/items`),
};
