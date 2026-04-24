import { fetchAPI } from './core.js';
import { buildQueryString } from './utils.js';

export const items = {
  getAll: (filters = {}) => {
    return fetchAPI(`/items${buildQueryString(filters)}`);
  },
  get: (id) => fetchAPI(`/items/${id}`),
  create: (data) =>
    fetchAPI('/items', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  update: (id, data) =>
    fetchAPI(`/items/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  // Perform a workflow status transition. Use this instead of passing
  // status_id to update() — the update endpoint rejects status_id so that
  // validator-mode and condition-mode workflow rules are always enforced.
  // Returns the updated item (unwrapped from the {item, old_status_id, ...} envelope).
  transition: async (id, toStatusId) => {
    const response = await fetchAPI(`/items/${id}/transition`, {
      method: 'POST',
      body: JSON.stringify({ to_status_id: toStatusId }),
    });
    return response.item;
  },
  delete: (id) =>
    fetchAPI(`/items/${id}`, {
      method: 'DELETE',
    }),
  getDeleteInfo: (id) => fetchAPI(`/items/${id}/delete-info`),
  deleteCascade: (id) =>
    fetchAPI(`/items/${id}/cascade`, {
      method: 'DELETE',
    }),
  reparentChildren: (id, newParentId) =>
    fetchAPI(`/items/${id}/reparent-children`, {
      method: 'POST',
      body: JSON.stringify({ newParentId }),
    }),
  copy: (id) =>
    fetchAPI(`/items/${id}/copy`, {
      method: 'POST',
    }),
  updateFracIndex: (id, data) =>
    fetchAPI(`/items/${id}/frac-index`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  getBacklog: (
    workspaceId,
    ql = null,
    collectionId = null,
    /** @type {any} */ { page, limit } = {}
  ) => {
    const params = new URLSearchParams();
    if (collectionId) {
      params.append('collection_id', collectionId);
    } else if (workspaceId) {
      params.append('workspace_id', workspaceId);
    }
    if (ql) params.append('ql', ql);
    if (page) params.append('page', page);
    if (limit) params.append('limit', limit);
    return fetchAPI(`/items/backlog?${params}`);
  },
  getChildren: (itemId) => fetchAPI(`/items/${itemId}/children`),
  getAncestors: (itemId) => fetchAPI(`/items/${itemId}/ancestors`),
  getDescendants: (itemId, maxDepth = null) => {
    const params = maxDepth ? `?max_depth=${maxDepth}` : '';
    return fetchAPI(`/items/${itemId}/descendants${params}`);
  },
  // Get available status transitions for a specific item based on workflow configuration
  getAvailableStatusTransitions: (itemId) =>
    fetchAPI(`/items/${itemId}/available-status-transitions`),
  // Get history of changes for an item
  getHistory: (itemId) => fetchAPI(`/items/${itemId}/history`),

  // Get items created in the last N days
  getRecentlyCreated: (workspaceId, days = 7) => {
    const sevenDaysAgo = new Date();
    sevenDaysAgo.setDate(sevenDaysAgo.getDate() - days);
    const createdSince = sevenDaysAgo.toISOString();
    const params = new URLSearchParams({
      workspace_id: workspaceId,
      created_since: createdSince,
    });
    return fetchAPI(`/items?${params}`);
  },

  // Watch/unwatch items
  addWatch: (id) =>
    fetchAPI(`/items/${id}/watch`, {
      method: 'POST',
    }),
  removeWatch: (id) =>
    fetchAPI(`/items/${id}/watch`, {
      method: 'DELETE',
    }),
  getWatchStatus: (id) => fetchAPI(`/items/${id}/watch`),

  // Personal tasks relationship
  getPersonalTasks: (itemId) => fetchAPI(`/items/${itemId}/personal-tasks`),
  unlinkPersonalTask: (itemId) =>
    fetchAPI(`/items/${itemId}/related-work-item`, {
      method: 'DELETE',
    }),
};
