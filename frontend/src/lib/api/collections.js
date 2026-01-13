import { fetchAPI } from './core.js';

export const collectionCategories = {
  getAll: () => fetchAPI('/collection-categories'),
  get: (id) => fetchAPI(`/collection-categories/${id}`),
  create: (data) => fetchAPI('/collection-categories', {
    method: 'POST',
    body: JSON.stringify(data),
  }),
  update: (id, data) => fetchAPI(`/collection-categories/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  }),
  delete: (id) => fetchAPI(`/collection-categories/${id}`, {
    method: 'DELETE',
  }),
};

export const collections = {
  getAll: () => fetchAPI('/collections'),
  get: (id) => fetchAPI(`/collections/${id}`),
  create: (data) => fetchAPI('/collections', {
    method: 'POST',
    body: JSON.stringify(data),
  }),
  update: (id, data) => fetchAPI(`/collections/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  }),
  delete: (id) => fetchAPI(`/collections/${id}`, {
    method: 'DELETE',
  }),
  // Board configuration methods
  getBoardConfiguration: (collectionId, workspaceId = null) => {
    const id = collectionId || 'default';
    const url = workspaceId && !collectionId
      ? `/collections/${id}/board-configuration?workspace_id=${workspaceId}`
      : `/collections/${id}/board-configuration`;
    return fetchAPI(url);
  },
  createBoardConfiguration: (collectionId, workspaceId, data) => {
    const id = collectionId || 'default';
    const url = workspaceId && !collectionId
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
