import { fetchAPI } from './core.js';
import { createCrudClient } from './createCrudClient.js';

// Integration providers (admin management)
export const integrationProviders = createCrudClient('/admin/integration-providers');

// User integration connections
export const userIntegrations = {
  getConnections: () => fetchAPI('/users/me/integration-connections'),
  getAvailableProviders: () => fetchAPI('/users/me/integration-connections/available'),
  disconnect: (providerId) =>
    fetchAPI(`/users/me/integration-connections/${providerId}`, {
      method: 'DELETE',
    }),
  startOAuth: (slug) => fetchAPI(`/integrations/oauth/${slug}/start`),
};

// Item integration links
export const itemIntegrationLinks = {
  get: (itemId) => fetchAPI(`/items/${itemId}/integration-links`),
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
