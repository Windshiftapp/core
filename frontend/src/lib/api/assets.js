import { fetchAllV2Pages, fetchAPIV2, fetchV2Data } from './core.js';
import { createCrudClient } from './createCrudClient.js';

export const assetSets = {
  ...createCrudClient('/asset-sets', { v2: true, allV2: true }),
  // Set role assignments
  getRoles: (id) => fetchV2Data(`/asset-sets/${id}/roles`),
  assignRole: (id, data) =>
    fetchV2Data(`/asset-sets/${id}/roles`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  revokeRole: (setId, assignmentId, type) =>
    fetchV2Data(`/asset-sets/${setId}/roles/${assignmentId}?type=${type}`, {
      method: 'DELETE',
    }),
  // Everyone default role
  getEveryoneRole: (id) => fetchV2Data(`/asset-sets/${id}/everyone-role`),
  setEveryoneRole: (id, data) =>
    fetchV2Data(`/asset-sets/${id}/everyone-role`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
};

export const assetRoles = {
  getAll: () => fetchV2Data('/asset-roles'),
  get: (id) => fetchV2Data(`/asset-roles/${id}`),
};

export const assetTypes = {
  ...createCrudClient('/types', {
    parentPath: '/asset-sets',
    itemPath: '/asset-types',
    v2: true,
    allV2: true,
  }),
  // Type fields
  getFields: (id) => fetchV2Data(`/asset-types/${id}/fields`),
  updateFields: (id, data) =>
    fetchV2Data(`/asset-types/${id}/fields`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
};

export const assetCategories = {
  ...createCrudClient('/categories', {
    parentPath: '/asset-sets',
    itemPath: '/asset-categories',
    v2: true,
    allV2: true,
  }),
  // Override getAll: callers pass `tree` as a positional boolean rather than a
  // filters object, so the factory's filters path doesn't fit.
  getAll: (setId, tree = false) =>
    fetchAllV2Pages(`/asset-sets/${setId}/categories${tree ? '?tree=true' : ''}`),
  move: (id, data) =>
    fetchV2Data(`/asset-categories/${id}/move`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
};

export const assetStatuses = createCrudClient('/statuses', {
  parentPath: '/asset-sets',
  itemPath: '/asset-statuses',
  v2: true,
  allV2: true,
});

export const assets = {
  ...createCrudClient('/assets', { parentPath: '/asset-sets', itemPath: '/assets', v2: true }),
  getAll: (setId, filters = {}, options = {}) => {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(filters)) {
      if (value == null || value === '') continue;
      if (key === 'limit') params.set('page_size', String(value));
      else if (key !== 'offset') params.set(key, String(value));
    }
    if (filters.offset && filters.limit) {
      params.set('page', String(Math.floor(filters.offset / filters.limit) + 1));
    }
    return fetchAPIV2(`/asset-sets/${setId}/assets?${params}`, options);
  },
  getSummaries: (ids, options = {}) =>
    fetchV2Data('/assets/summaries', {
      ...options,
      method: 'POST',
      body: JSON.stringify({ ids: [...new Set(ids)] }),
    }),
  // Asset links
  getLinks: (id) => fetchV2Data(`/assets/${id}/links`),
  createLink: (id, data) =>
    fetchV2Data(`/assets/${id}/links`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  getRelationshipGraph: (id) => fetchV2Data(`/assets/${id}/relationship-graph`),
};

export const itemLinkedAssets = {
  get: (itemId) => fetchV2Data(`/items/${itemId}/linked-assets`),
};

export const assetImport = {
  upload: (setId, formData) =>
    fetchV2Data(`/asset-sets/${setId}/import/upload`, { method: 'POST', body: formData }),
  start: (setId, data) =>
    fetchV2Data(`/asset-sets/${setId}/import/start`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  getJob: (setId, jobId) => fetchV2Data(`/asset-sets/${setId}/import/jobs/${jobId}`),
  getJobs: (setId) => fetchV2Data(`/asset-sets/${setId}/import/jobs`),
  suggestFields: (setId, data) =>
    fetchV2Data(`/asset-sets/${setId}/import/suggest-fields`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  createType: (setId, data) =>
    fetchV2Data(`/asset-sets/${setId}/import/create-type`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
};
