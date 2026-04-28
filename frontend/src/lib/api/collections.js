import { fetchAPI } from './core.js';
import { createCrudClient } from './createCrudClient.js';

export const collectionCategories = createCrudClient('/collection-categories');

export const collections = {
  ...createCrudClient('/collections'),
  updatePublicSharing: (id, data) =>
    fetchAPI(`/collections/${id}/public`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  // Board configuration methods
  getBoardConfiguration: (collectionId, workspaceId = null) => {
    const id = collectionId || 'default';
    const url =
      workspaceId && !collectionId
        ? `/collections/${id}/board-configuration?workspace_id=${workspaceId}`
        : `/collections/${id}/board-configuration`;
    return fetchAPI(url);
  },
  createBoardConfiguration: (collectionId, workspaceId, data) => {
    const id = collectionId || 'default';
    const url =
      workspaceId && !collectionId
        ? `/collections/${id}/board-configuration?workspace_id=${workspaceId}`
        : `/collections/${id}/board-configuration`;
    return fetchAPI(url, {
      method: 'POST',
      body: JSON.stringify(data),
    });
  },
  updateBoardConfiguration: (collectionId, configId, data) => {
    const id = collectionId || 'default';
    return fetchAPI(`/collections/${id}/board-configuration/${configId}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    });
  },
  deleteBoardConfiguration: (collectionId, configId) => {
    const id = collectionId || 'default';
    return fetchAPI(`/collections/${id}/board-configuration/${configId}`, {
      method: 'DELETE',
    });
  },
};
