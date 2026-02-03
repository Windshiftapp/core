import { fetchAPI } from './core.js';

export const assetSets = {
  getAll: () => fetchAPI('/asset-sets'),
  get: (id) => fetchAPI(`/asset-sets/${id}`),
  create: (data) =>
    fetchAPI('/asset-sets', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  update: (id, data) =>
    fetchAPI(`/asset-sets/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  delete: (id) =>
    fetchAPI(`/asset-sets/${id}`, {
      method: 'DELETE',
    }),
  // Set role assignments
  getRoles: (id) => fetchAPI(`/asset-sets/${id}/roles`),
  assignRole: (id, data) =>
    fetchAPI(`/asset-sets/${id}/roles`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  revokeRole: (setId, assignmentId, type) =>
    fetchAPI(`/asset-sets/${setId}/roles/${assignmentId}?type=${type}`, {
      method: 'DELETE',
    }),
  // Everyone default role
  getEveryoneRole: (id) => fetchAPI(`/asset-sets/${id}/everyone-role`),
  setEveryoneRole: (id, data) =>
    fetchAPI(`/asset-sets/${id}/everyone-role`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
};

export const assetRoles = {
  getAll: () => fetchAPI('/asset-roles'),
  get: (id) => fetchAPI(`/asset-roles/${id}`),
};

export const assetTypes = {
  getAll: (setId) => fetchAPI(`/asset-sets/${setId}/types`),
  get: (id) => fetchAPI(`/asset-types/${id}`),
  create: (setId, data) =>
    fetchAPI(`/asset-sets/${setId}/types`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  update: (id, data) =>
    fetchAPI(`/asset-types/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  delete: (id) =>
    fetchAPI(`/asset-types/${id}`, {
      method: 'DELETE',
    }),
  // Type fields
  getFields: (id) => fetchAPI(`/asset-types/${id}/fields`),
  updateFields: (id, data) =>
    fetchAPI(`/asset-types/${id}/fields`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
};

export const assetCategories = {
  getAll: (setId, tree = false) =>
    fetchAPI(`/asset-sets/${setId}/categories${tree ? '?tree=true' : ''}`),
  get: (id) => fetchAPI(`/asset-categories/${id}`),
  create: (setId, data) =>
    fetchAPI(`/asset-sets/${setId}/categories`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  update: (id, data) =>
    fetchAPI(`/asset-categories/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  delete: (id) =>
    fetchAPI(`/asset-categories/${id}`, {
      method: 'DELETE',
    }),
  move: (id, data) =>
    fetchAPI(`/asset-categories/${id}/move`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
};

export const assetStatuses = {
  getAll: (setId) => fetchAPI(`/asset-sets/${setId}/statuses`),
  get: (id) => fetchAPI(`/asset-statuses/${id}`),
  create: (setId, data) =>
    fetchAPI(`/asset-sets/${setId}/statuses`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  update: (id, data) =>
    fetchAPI(`/asset-statuses/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  delete: (id) =>
    fetchAPI(`/asset-statuses/${id}`, {
      method: 'DELETE',
    }),
};

export const assets = {
  getAll: (setId, filters = {}) => {
    const params = new URLSearchParams();
    Object.entries(filters).forEach(([key, value]) => {
      if (value !== undefined && value !== null) params.append(key, value);
    });
    const queryString = params.toString();
    return fetchAPI(`/asset-sets/${setId}/assets${queryString ? `?${queryString}` : ''}`);
  },
  get: (id) => fetchAPI(`/assets/${id}`),
  create: (setId, data) =>
    fetchAPI(`/asset-sets/${setId}/assets`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  update: (id, data) =>
    fetchAPI(`/assets/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  delete: (id) =>
    fetchAPI(`/assets/${id}`, {
      method: 'DELETE',
    }),
  // Asset links
  getLinks: (id) => fetchAPI(`/assets/${id}/links`),
  createLink: (id, data) =>
    fetchAPI(`/assets/${id}/links`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
};

export const itemLinkedAssets = {
  get: (itemId) => fetchAPI(`/items/${itemId}/linked-assets`),
};
