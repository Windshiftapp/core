import { fetchAPIV2, fetchV2Data } from '../core.js';

// Helper to build URL with workspace_id param for 'default' collections
const buildUrl = (id, workspaceId, path) =>
  id === 'default'
    ? `/workspaces/${workspaceId}/test-coverage/${path}`
    : `/collections/${id}/test-coverage/${path}`;

// Test coverage API
export const coverage = {
  // Config endpoints
  getConfig: (id, workspaceId) => fetchV2Data(buildUrl(id, workspaceId, 'config')),

  createConfig: (id, config, workspaceId) =>
    fetchV2Data(buildUrl(id, workspaceId, 'config'), {
      method: 'POST',
      body: JSON.stringify(config),
    }),

  updateConfig: (collectionId, configId, config, workspaceId) =>
    fetchV2Data(`${buildUrl(collectionId, workspaceId, 'config')}/${configId}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/merge-patch+json' },
      body: JSON.stringify(config),
    }),

  deleteConfig: (collectionId, configId, workspaceId) =>
    fetchV2Data(`${buildUrl(collectionId, workspaceId, 'config')}/${configId}`, {
      method: 'DELETE',
    }),

  // Coverage data
  getSummary: (id, workspaceId) => fetchV2Data(buildUrl(id, workspaceId, 'summary')),

  getRequirements: (id, workspaceId, options = {}) => {
    const params = new URLSearchParams();
    if (options.page) params.append('page', options.page);
    if (options.limit) params.append('page_size', options.limit);
    if (options.covered !== undefined) params.append('covered', options.covered);
    if (options.itemTypeId) params.append('item_type_id', options.itemTypeId);
    if (options.search) params.append('search', options.search);
    const queryString = params.toString();
    return fetchAPIV2(
      `${buildUrl(id, workspaceId, 'requirements')}${queryString ? `?${queryString}` : ''}`
    );
  },
};
