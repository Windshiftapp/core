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
};
