import { fetchAPI } from './core.js';
import { createCrudClient } from './createCrudClient.js';

// Integration providers (admin management)
export const integrationProviders = createCrudClient('/admin/integration-providers');

// User integration connections
export const userIntegrations = {
  getConnections: (options) => fetchAPI('/users/me/integration-connections', options),
  getAvailableProviders: (options) =>
    fetchAPI('/users/me/integration-connections/available', options),
  disconnect: (providerId) =>
    fetchAPI(`/users/me/integration-connections/${providerId}`, {
      method: 'DELETE',
    }),
  startOAuth: (slug) => fetchAPI(`/integrations/oauth/${slug}/start`),
};

// Todoist personal-task sync
export const todoistSync = {
  get: () => fetchAPI('/users/me/todoist-sync'),
  update: (data) =>
    fetchAPI('/users/me/todoist-sync', {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  getProjects: () => fetchAPI('/users/me/todoist-sync/projects'),
  run: () => fetchAPI('/users/me/todoist-sync/run', { method: 'POST' }),
};

// Item integration links
export const itemIntegrationLinks = {
  get: (itemId, options) => fetchAPI(`/items/${itemId}/integration-links`, options),
  create: (itemId, data) =>
    fetchAPI(`/items/${itemId}/integration-links`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  delete: (linkId) =>
    fetchAPI(`/item-integration-links/${linkId}`, {
      method: 'DELETE',
    }),
  refresh: (linkId) =>
    fetchAPI(`/item-integration-links/${linkId}/refresh`, {
      method: 'POST',
    }),
  search: (itemId, query, providerId) =>
    fetchAPI(
      `/items/${itemId}/integration-search?q=${encodeURIComponent(query)}&provider_id=${providerId}`
    ),
};

// Zammad system connections and item ticket links
export const zammadConnections = {
  getAll: () => fetchAPI('/admin/zammad-connections'),
  get: (id) => fetchAPI(`/admin/zammad-connections/${id}`),
  create: (data) =>
    fetchAPI('/admin/zammad-connections', { method: 'POST', body: JSON.stringify(data) }),
  update: (id, data) =>
    fetchAPI(`/admin/zammad-connections/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  delete: (id) => fetchAPI(`/admin/zammad-connections/${id}`, { method: 'DELETE' }),
  test: (id) => fetchAPI(`/admin/zammad-connections/${id}/test`, { method: 'POST' }),
  refreshAllTickets: () => fetchAPI('/admin/zammad-ticket-links/refresh', { method: 'POST' }),
  startOAuth: (id) =>
    fetchAPI(`/admin/integration-providers/${id}/oauth/start`, { method: 'POST' }),
  forWorkspace: (workspaceId) => fetchAPI(`/workspaces/${workspaceId}/zammad-connections`),
  metadata: (workspaceId, id) =>
    fetchAPI(`/workspaces/${workspaceId}/zammad-connections/${id}/metadata`),
  owners: (workspaceId, id, groupId) =>
    fetchAPI(
      `/workspaces/${workspaceId}/zammad-connections/${id}/owners?group_id=${encodeURIComponent(groupId)}`
    ),
};

export const zammadTickets = {
  resolve: (correlationKey) =>
    fetchAPI(`/zammad-ticket-links/resolve/${encodeURIComponent(correlationKey)}`),
  forItem: (itemId) => fetchAPI(`/items/${itemId}/zammad-links`),
  create: (itemId, data) =>
    fetchAPI(`/items/${itemId}/zammad-tickets`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  link: (itemId, data) =>
    fetchAPI(`/items/${itemId}/zammad-ticket-links`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  update: (linkId, data) =>
    fetchAPI(`/zammad-ticket-links/${linkId}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  delete: (linkId) => fetchAPI(`/zammad-ticket-links/${linkId}`, { method: 'DELETE' }),
  refresh: (linkId) => fetchAPI(`/zammad-ticket-links/${linkId}/refresh`, { method: 'POST' }),
};
