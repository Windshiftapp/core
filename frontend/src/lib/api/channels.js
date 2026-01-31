import { fetchAPI } from './core.js';

export const channels = {
  getAll: () => fetchAPI('/channels'),
  get: (id) => fetchAPI(`/channels/${id}`),
  create: (data) => fetchAPI('/channels', {
    method: 'POST',
    body: JSON.stringify(data),
  }),
  update: (id, data) => fetchAPI(`/channels/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  }),
  delete: (id) => fetchAPI(`/channels/${id}`, {
    method: 'DELETE',
  }),
  test: (id) => fetchAPI(`/channels/${id}/test`, {
    method: 'POST',
  }),
  testWithEmail: (id, testEmail) => fetchAPI(`/channels/${id}/test`, {
    method: 'POST',
    body: JSON.stringify({ test_email: testEmail }),
  }),
  updateConfig: (id, config) => fetchAPI(`/channels/${id}/config`, {
    method: 'PUT',
    body: JSON.stringify({ config }),
  }),
  // Channel Managers
  getManagers: (id) => fetchAPI(`/channels/${id}/managers`),
  addManagers: (id, managerType, managerIds) => fetchAPI(`/channels/${id}/managers`, {
    method: 'POST',
    body: JSON.stringify({
      manager_type: managerType,
      manager_ids: managerIds
    }),
  }),
  removeManager: (id, managerId) => fetchAPI(`/channels/${id}/managers/${managerId}`, {
    method: 'DELETE',
  }),
  // Email OAuth (inline per-channel OAuth credentials)
  startEmailOAuth: (channelId) => fetchAPI(`/channels/${channelId}/inline-oauth/start`, {
    method: 'POST',
  }),
};

export const channelCategories = {
  getAll: () => fetchAPI('/channel-categories'),
  get: (id) => fetchAPI(`/channel-categories/${id}`),
  create: (data) => fetchAPI('/channel-categories', {
    method: 'POST',
    body: JSON.stringify(data),
  }),
  update: (id, data) => fetchAPI(`/channel-categories/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  }),
  delete: (id) => fetchAPI(`/channel-categories/${id}`, {
    method: 'DELETE',
  }),
};

// Request Types (channel-scoped)
export const requestTypes = {
  getForChannel: (channelId) => fetchAPI(`/channels/${channelId}/request-types`),
  getForPortal: (slug) => fetchAPI(`/portal/${slug}/request-types`),
  get: (id) => fetchAPI(`/request-types/${id}`),
  create: (channelId, data) => fetchAPI(`/channels/${channelId}/request-types`, {
    method: 'POST',
    body: JSON.stringify(data),
  }),
  update: (id, data) => fetchAPI(`/request-types/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  }),
  delete: (id) => fetchAPI(`/request-types/${id}`, {
    method: 'DELETE',
  }),
  getFields: (id) => fetchAPI(`/request-types/${id}/fields`),
  updateFields: (id, fields) => fetchAPI(`/request-types/${id}/fields`, {
    method: 'PUT',
    body: JSON.stringify(fields),
  }),
  getAvailableFields: (id) => fetchAPI(`/request-types/${id}/available-fields`),
  updateVisibility: (id, { groupIds, orgIds }) => fetchAPI(`/request-types/${id}/visibility`, {
    method: 'PUT',
    body: JSON.stringify({ group_ids: groupIds, org_ids: orgIds }),
  }),
};

// Asset Reports (channel-scoped)
export const assetReports = {
  getForChannel: (channelId) => fetchAPI(`/channels/${channelId}/asset-reports`),
  getForPortal: (slug) => fetchAPI(`/portal/${slug}/asset-reports`),
  get: (id) => fetchAPI(`/asset-reports/${id}`),
  create: (channelId, data) => fetchAPI(`/channels/${channelId}/asset-reports`, {
    method: 'POST',
    body: JSON.stringify(data),
  }),
  update: (id, data) => fetchAPI(`/asset-reports/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  }),
  delete: (id) => fetchAPI(`/asset-reports/${id}`, {
    method: 'DELETE',
  }),
  updateVisibility: (id, { groupIds, orgIds }) => fetchAPI(`/asset-reports/${id}/visibility`, {
    method: 'PUT',
    body: JSON.stringify({ group_ids: groupIds, org_ids: orgIds }),
  }),
  execute: (slug, id, params = {}) => {
    const queryParams = new URLSearchParams();
    if (params.page) queryParams.set('page', params.page);
    if (params.pageSize) queryParams.set('page_size', params.pageSize);
    const query = queryParams.toString();
    return fetchAPI(`/portal/${slug}/asset-reports/${id}/execute${query ? '?' + query : ''}`);
  },
};
