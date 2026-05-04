import { fetchAPI } from './core.js';
import { createCrudClient } from './createCrudClient.js';

export const configurationSets = {
  ...createCrudClient('/configuration-sets'),
  analyzeMigration: (id, workspaceId = null) => {
    const url = workspaceId
      ? `/configuration-sets/${id}/analyze-migration?workspace_id=${workspaceId}`
      : `/configuration-sets/${id}/analyze-migration`;
    return fetchAPI(url);
  },
  executeMigration: (data) =>
    fetchAPI('/configuration-sets/execute-migration', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  // Comprehensive migration (all dimensions: item types, fields, statuses, priorities)
  analyzeComprehensiveMigration: (targetConfigSetId, workspaceId) => {
    return fetchAPI(
      `/configuration-sets/${targetConfigSetId}/analyze-comprehensive-migration?workspace_id=${workspaceId}`
    );
  },
  executeComprehensiveMigration: (data) =>
    fetchAPI('/configuration-sets/execute-comprehensive-migration', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
};

export const screens = {
  ...createCrudClient('/screens'),
  getFields: (id) => fetchAPI(`/screens/${id}/fields`),
  updateFields: (id, data) =>
    fetchAPI(`/screens/${id}/fields`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
};

export const customFields = {
  ...createCrudClient('/custom-fields', { adminBasePath: '/admin/custom-fields' }),
  updateSettings: (data) =>
    fetchAPI('/admin/custom-fields/settings', {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
};

export const projectFieldRequirements = {
  getByProject: (id) => fetchAPI(`/projects/${id}/field-requirements`),
  setRequirement: (projectId, data) =>
    fetchAPI(`/projects/${projectId}/field-requirements`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  removeRequirement: (projectId, fieldId) =>
    fetchAPI(`/projects/${projectId}/field-requirements/${fieldId}`, {
      method: 'DELETE',
    }),
  getAvailableFields: (id) => fetchAPI(`/projects/${id}/available-fields`),
};

export const itemTypes = createCrudClient('/item-types');

export const priorities = createCrudClient('/priorities');

export const hierarchyLevels = createCrudClient('/hierarchy-levels');

export const linkTypes = {
  ...createCrudClient('/link-types', { adminBasePath: '/admin/link-types' }),
  // One caller passes a boolean (LinkTypeManager); keep that signature.
  getAll: (includeInactive = false) =>
    fetchAPI(`/link-types${includeInactive ? '?include_inactive=true' : ''}`),
};

export const links = {
  getForItem: (type, id) => fetchAPI(`/${type}/${id}/links`),
  getFieldLinks: (itemId, fieldId) => fetchAPI(`/items/${itemId}/field-links/${fieldId}`),
  create: (data) =>
    fetchAPI('/links', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  delete: (id) =>
    fetchAPI(`/links/${id}`, {
      method: 'DELETE',
    }),
  search: (query, type = '', limit = 20, itemTypeIds = []) => {
    const params = new URLSearchParams();
    if (query) params.append('q', query);
    if (type) params.append('type', type);
    if (limit !== 20) params.append('limit', limit.toString());
    if (itemTypeIds.length > 0) params.append('item_type_ids', itemTypeIds.join(','));
    return fetchAPI(`/links/search?${params.toString()}`);
  },
};
