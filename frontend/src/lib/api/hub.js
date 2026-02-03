// Hub API module
import { get, put } from './core.js';

/**
 * Get hub configuration and all enabled portals
 */
async function getHub() {
  return get('/hub');
}

/**
 * Update hub configuration
 * @param {Object} config - Hub configuration object
 */
async function updateConfig(config) {
  return put('/hub/config', config);
}

/**
 * Get hub inbox (requests from all portals)
 * @param {Object} params - Query parameters
 * @param {number} params.page - Page number (default 1)
 * @param {number} params.per_page - Items per page (default 20)
 * @param {string} params.portal_id - Filter by portal ID
 * @param {string} params.status - Filter by status
 */
async function getInbox(params = {}) {
  const query = new URLSearchParams();
  if (params.page) query.set('page', params.page);
  if (params.per_page) query.set('per_page', params.per_page);
  if (params.portal_id) query.set('portal_id', params.portal_id);
  if (params.status) query.set('status', params.status);

  const queryString = query.toString();
  return get(`/hub/inbox${queryString ? `?${queryString}` : ''}`);
}

/**
 * Get a specific inbox item detail
 * @param {number} itemId - Item ID
 */
async function getInboxItem(itemId) {
  return get(`/hub/inbox/${itemId}`);
}

export const hub = {
  get: getHub,
  updateConfig,
  getInbox,
  getInboxItem,
};
