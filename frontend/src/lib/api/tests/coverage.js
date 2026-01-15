import { fetchAPI } from '../core.js';

// Helper to build URL with workspace_id param for 'default' collections
const buildUrl = (id, workspaceId, path) => {
  const base = `/collections/${id}/test-coverage/${path}`;
  return id === 'default' ? `${base}?workspace_id=${workspaceId}` : base;
};

// Test coverage API
export const coverage = {
  // Config endpoints
  getConfig: (id, workspaceId) => fetchAPI(buildUrl(id, workspaceId, 'config')),

  createConfig: (id, config, workspaceId) =>
    fetchAPI(buildUrl(id, workspaceId, 'config'), {
      method: 'POST',
      body: JSON.stringify(config),
    }),

  updateConfig: (collectionId, configId, config) =>
    fetchAPI(`/collections/${collectionId}/test-coverage/config/${configId}`, {
      method: 'PUT',
      body: JSON.stringify(config),
    }),

  deleteConfig: (collectionId, configId) =>
    fetchAPI(`/collections/${collectionId}/test-coverage/config/${configId}`, {
      method: 'DELETE',
    }),

  // Coverage data
  getSummary: (id, workspaceId) => fetchAPI(buildUrl(id, workspaceId, 'summary')),

  getRequirements: (id, workspaceId, options = {}) => {
    const params = new URLSearchParams();
    if (id === 'default') params.append('workspace_id', workspaceId);
    if (options.page) params.append('page', options.page);
    if (options.limit) params.append('limit', options.limit);
    if (options.covered !== undefined) params.append('covered', options.covered);
    if (options.itemTypeId) params.append('item_type_id', options.itemTypeId);
    if (options.search) params.append('search', options.search);
    const queryString = params.toString();
    return fetchAPI(`/collections/${id}/test-coverage/requirements${queryString ? '?' + queryString : ''}`);
  },
};
