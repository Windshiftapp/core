import { API_BASE, fetchAPI } from './core.js';
import { createCrudClient } from './createCrudClient.js';

export const assetSets = {
  ...createCrudClient('/asset-sets'),
  // Delete is gated behind the admin route; everything else uses /asset-sets.
  delete: (id) =>
    fetchAPI(`/admin/asset-sets/${id}`, {
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
  ...createCrudClient('/types', { parentPath: '/asset-sets', itemPath: '/asset-types' }),
  // Type fields
  getFields: (id) => fetchAPI(`/asset-types/${id}/fields`),
  updateFields: (id, data) =>
    fetchAPI(`/asset-types/${id}/fields`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
};

export const assetCategories = {
  ...createCrudClient('/categories', {
    parentPath: '/asset-sets',
    itemPath: '/asset-categories',
  }),
  // Override getAll: callers pass `tree` as a positional boolean rather than a
  // filters object, so the factory's filters path doesn't fit.
  getAll: (setId, tree = false) =>
    fetchAPI(`/asset-sets/${setId}/categories${tree ? '?tree=true' : ''}`),
  move: (id, data) =>
    fetchAPI(`/asset-categories/${id}/move`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
};

export const assetStatuses = createCrudClient('/statuses', {
  parentPath: '/asset-sets',
  itemPath: '/asset-statuses',
});

export const assets = {
  ...createCrudClient('/assets', { parentPath: '/asset-sets', itemPath: '/assets' }),
  // Asset links
  getLinks: (id) => fetchAPI(`/assets/${id}/links`),
  createLink: (id, data) =>
    fetchAPI(`/assets/${id}/links`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  getRelationshipGraph: (id) => fetchAPI(`/assets/${id}/relationship-graph`),
};

export const itemLinkedAssets = {
  get: (itemId) => fetchAPI(`/items/${itemId}/linked-assets`),
};

export const assetImport = {
  upload: async (setId, formData) => {
    const response = await fetch(`${API_BASE}/asset-sets/${setId}/import/upload`, {
      method: 'POST',
      body: formData,
      credentials: 'same-origin',
    });
    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(errorText || 'Upload failed');
    }
    return response.json();
  },
  start: (setId, data) =>
    fetchAPI(`/asset-sets/${setId}/import/start`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  getJob: (setId, jobId) => fetchAPI(`/asset-sets/${setId}/import/jobs/${jobId}`),
  getJobs: (setId) => fetchAPI(`/asset-sets/${setId}/import/jobs`),
  suggestFields: (setId, data) =>
    fetchAPI(`/asset-sets/${setId}/import/suggest-fields`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  createType: (setId, data) =>
    fetchAPI(`/asset-sets/${setId}/import/create-type`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
};
