import { fetchAllV2Pages, fetchAPIV2, fetchV2Data } from './core.js';
import { createCrudClient } from './createCrudClient.js';

export const collectionCategories = createCrudClient('/collection-categories', {
  v2: true,
  allV2: true,
});

function boardConfigurationPath(collectionId, workspaceId) {
  if (collectionId) return `/collections/${collectionId}/board-configuration`;
  return `/workspaces/${workspaceId}/board-configuration`;
}

export const collections = {
  list: (filters = {}, requestOptions = {}) => {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(filters)) {
      if (value != null && value !== '') params.set(key, String(value));
    }
    const query = params.toString();
    return fetchAPIV2(`/collections${query ? `?${query}` : ''}`, requestOptions);
  },
  getAll: (filters = {}, requestOptions = {}) => {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(filters)) {
      if (value != null && value !== '') params.set(key, String(value));
    }
    const query = params.toString();
    return fetchAllV2Pages(`/collections${query ? `?${query}` : ''}`, requestOptions);
  },
  get: (id, requestOptions = {}) => fetchV2Data(`/collections/${id}`, requestOptions),
  create: (data) => fetchV2Data('/collections', { method: 'POST', body: JSON.stringify(data) }),
  update: (id, data) =>
    fetchV2Data(`/collections/${id}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/merge-patch+json' },
      body: JSON.stringify(data),
    }),
  delete: (id) => fetchV2Data(`/collections/${id}`, { method: 'DELETE' }),
  updatePublicSharing: (id, data) =>
    fetchV2Data(`/collections/${id}/sharing`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/merge-patch+json' },
      body: JSON.stringify(data),
    }),
  // Board configuration methods
  getBoardConfiguration: (collectionId, workspaceId = null) => {
    return fetchV2Data(boardConfigurationPath(collectionId, workspaceId));
  },
  getBoardConfigurationBootstrap: (collectionId, workspaceId = null) => {
    const path = boardConfigurationPath(collectionId, workspaceId);
    const query =
      collectionId && workspaceId ? `?workspace_id=${encodeURIComponent(workspaceId)}` : '';
    return fetchV2Data(`${path}/bootstrap${query}`);
  },
  createBoardConfiguration: (collectionId, workspaceId, data) => {
    return fetchV2Data(boardConfigurationPath(collectionId, workspaceId), {
      method: 'PUT',
      body: JSON.stringify(data),
    });
  },
  updateBoardConfiguration: (collectionId, _configId, data, workspaceId = null) => {
    return fetchV2Data(boardConfigurationPath(collectionId, workspaceId), {
      method: 'PUT',
      body: JSON.stringify(data),
    });
  },
  deleteBoardConfiguration: (collectionId, _configId, workspaceId = null) => {
    return fetchV2Data(boardConfigurationPath(collectionId, workspaceId), {
      method: 'DELETE',
    });
  },
};
