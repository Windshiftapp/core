import { fetchAPI } from './core.js';
import { createCrudClient } from './createCrudClient.js';
import { buildQueryString } from './utils.js';

export const channels = {
  ...createCrudClient('/channels'),
  toggle: (id) =>
    fetchAPI(`/channels/${id}/toggle`, {
      method: 'PUT',
    }),
  testWithEmail: (id, testEmail) =>
    fetchAPI(`/channels/${id}/test`, {
      method: 'POST',
      body: JSON.stringify({ test_email: testEmail }),
    }),
  updateConfig: (id, config) =>
    fetchAPI(`/channels/${id}/config`, {
      method: 'PUT',
      body: JSON.stringify({ config }),
    }),
  getDeleteImpact: (id) => fetchAPI(`/channels/${id}/delete-impact`),
  testConfig: (id, config) =>
    fetchAPI(`/channels/${id}/test-config`, {
      method: 'POST',
      body: JSON.stringify({ config }),
    }),
  // Channel Managers
  getManagers: (id) => fetchAPI(`/channels/${id}/managers`),
  addManagers: (id, managerType, managerIds) =>
    fetchAPI(`/channels/${id}/managers`, {
      method: 'POST',
      body: JSON.stringify({
        manager_type: managerType,
        manager_ids: managerIds,
      }),
    }),
  removeManager: (id, managerId) =>
    fetchAPI(`/channels/${id}/managers/${managerId}`, {
      method: 'DELETE',
    }),
  // Email processing log
  getEmailLog: (id, page = 1, pageSize = 50, search = '') => {
    let url = `/channels/${id}/email-log?page=${page}&page_size=${pageSize}`;
    if (search) url += `&search=${encodeURIComponent(search)}`;
    return fetchAPI(url);
  },
  // Rewind the IMAP watermark to the earliest flood-declined message so the
  // next poll retries rate-limited mail that was left in the mailbox.
  requeueRateLimitedEmail: (id) =>
    fetchAPI(`/channels/${id}/email/requeue-rate-limited`, {
      method: 'POST',
    }),
  // Outbound customer-reply queue: envelope metadata and delivery state.
  getEmailReplies: (id, status = 'pending', page = 1, pageSize = 50) =>
    fetchAPI(
      `/channels/${id}/email/replies?status=${encodeURIComponent(status)}&page=${page}&page_size=${pageSize}`
    ),
  retryEmailReply: (id, commentId) =>
    fetchAPI(`/channels/${id}/email/replies/${commentId}/retry`, {
      method: 'POST',
    }),
  discardEmailReply: (id, commentId) =>
    fetchAPI(`/channels/${id}/email/replies/${commentId}/discard`, {
      method: 'POST',
    }),
  // Email OAuth (inline per-channel OAuth credentials)
  startEmailOAuth: (channelId, restoreChannelEnabled = false) =>
    fetchAPI(`/channels/${channelId}/inline-oauth/start`, {
      method: 'POST',
      body: JSON.stringify({ restore_channel_enabled: restoreChannelEnabled }),
    }),
};

export const channelCategories = createCrudClient('/channel-categories');

// Create a channel-scoped CRUD client for sub-resources like request-types
// and asset-reports that live under /channels/:channelId/.
function createChannelScopedCrud(resourceKey) {
  return {
    getForChannel: (channelId) => fetchAPI(`/channels/${channelId}/${resourceKey}`),
    getForPortal: (slug) => fetchAPI(`/portal/${slug}/${resourceKey}`),
    get: (id) => fetchAPI(`/${resourceKey}/${id}`),
    create: (channelId, data) =>
      fetchAPI(`/channels/${channelId}/${resourceKey}`, {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    update: (channelId, id, data) =>
      fetchAPI(`/channels/${channelId}/${resourceKey}/${id}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    delete: (channelId, id) =>
      fetchAPI(`/channels/${channelId}/${resourceKey}/${id}`, {
        method: 'DELETE',
      }),
    getFields: (id, requestOptions = {}) =>
      fetchAPI(`/${resourceKey}/${id}/fields`, requestOptions),
    updateFields: (channelId, id, fields) =>
      fetchAPI(`/channels/${channelId}/${resourceKey}/${id}/fields`, {
        method: 'PUT',
        body: JSON.stringify(fields),
      }),
    getAvailableFields: (id) => fetchAPI(`/${resourceKey}/${id}/available-fields`),
    updateVisibility: (channelId, id, { groupIds, orgIds }) =>
      fetchAPI(`/channels/${channelId}/${resourceKey}/${id}/visibility`, {
        method: 'PUT',
        body: JSON.stringify({ group_ids: groupIds, org_ids: orgIds }),
      }),
  };
}

// Request Types (channel-scoped)
export const requestTypes = {
  ...createChannelScopedCrud('request-types'),
  getAllForChannel: (channelId) => fetchAPI(`/channels/${channelId}/request-types`),
  updateConfig: (id, config) =>
    fetchAPI(`/request-types/${id}/config`, {
      method: 'PUT',
      body: JSON.stringify(config),
    }),
};

// Asset Reports (channel-scoped)
export const assetReports = {
  ...createChannelScopedCrud('asset-reports'),
  getPortalFields: (slug, id) => fetchAPI(`/portal/${slug}/asset-reports/${id}/fields`),
  execute: (slug, id, params = {}) => {
    const mapped = {};
    if (params.page) mapped.page = params.page;
    if (params.pageSize) mapped.per_page = params.pageSize;
    return fetchAPI(`/portal/${slug}/asset-reports/${id}/execute${buildQueryString(mapped)}`);
  },
  submit: (slug, id, { params = {}, page = 1, perPage = 25 } = {}) =>
    fetchAPI(`/portal/${slug}/asset-reports/${id}/execute?page=${page}&per_page=${perPage}`, {
      method: 'POST',
      body: JSON.stringify({ params }),
    }),
};
