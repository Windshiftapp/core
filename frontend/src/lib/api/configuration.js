import { fetchAPI } from './core.js';

export const configurationSets = {
  getAll: (filters = {}) => {
    const params = new URLSearchParams();
    Object.entries(filters).forEach(([key, value]) => {
      if (value) params.append(key, value);
    });
    const queryString = params.toString();
    return fetchAPI(`/configuration-sets${queryString ? `?${queryString}` : ''}`);
  },
  get: (id) => fetchAPI(`/configuration-sets/${id}`),
  create: (data) =>
    fetchAPI('/configuration-sets', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  update: (id, data) =>
    fetchAPI(`/configuration-sets/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  delete: (id) =>
    fetchAPI(`/configuration-sets/${id}`, {
      method: 'DELETE',
    }),
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
  // Update with skip migration check (used after migration is complete)
  updateWithSkipMigrationCheck: (id, data) =>
    fetchAPI(`/configuration-sets/${id}?skip_migration_check=true`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
};

export const screens = {
  getAll: (filters = {}) => {
    const params = new URLSearchParams();
    Object.entries(filters).forEach(([key, value]) => {
      if (value) params.append(key, value);
    });
    const queryString = params.toString();
    return fetchAPI(`/screens${queryString ? `?${queryString}` : ''}`);
  },
  get: (id) => fetchAPI(`/screens/${id}`),
  create: (data) =>
    fetchAPI('/screens', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  update: (id, data) =>
    fetchAPI(`/screens/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  delete: (id) =>
    fetchAPI(`/screens/${id}`, {
      method: 'DELETE',
    }),
  getFields: (id) => fetchAPI(`/screens/${id}/fields`),
  updateFields: (id, data) =>
    fetchAPI(`/screens/${id}/fields`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
};

export const customFields = {
  getAll: (filters = {}) => {
    const params = new URLSearchParams();
    Object.entries(filters).forEach(([key, value]) => {
      if (value) params.append(key, value);
    });
    const queryString = params.toString();
    return fetchAPI(`/custom-fields${queryString ? `?${queryString}` : ''}`);
  },
  get: (id) => fetchAPI(`/custom-fields/${id}`),
  create: (data) =>
    fetchAPI('/admin/custom-fields', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  update: (id, data) =>
    fetchAPI(`/admin/custom-fields/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  delete: (id) =>
    fetchAPI(`/admin/custom-fields/${id}`, {
      method: 'DELETE',
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

export const itemTypes = {
  getAll: (filters = {}) => {
    const params = new URLSearchParams();
    Object.entries(filters).forEach(([key, value]) => {
      if (value) params.append(key, value);
    });
    const queryString = params.toString();
    return fetchAPI(`/item-types${queryString ? `?${queryString}` : ''}`);
  },
  get: (id) => fetchAPI(`/item-types/${id}`),
  create: (data) =>
    fetchAPI('/item-types', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  update: (id, data) =>
    fetchAPI(`/item-types/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  delete: (id) =>
    fetchAPI(`/item-types/${id}`, {
      method: 'DELETE',
    }),
};

export const priorities = {
  getAll: (filters = {}) => {
    const params = new URLSearchParams();
    Object.entries(filters).forEach(([key, value]) => {
      if (value) params.append(key, value);
    });
    const queryString = params.toString();
    return fetchAPI(`/priorities${queryString ? `?${queryString}` : ''}`);
  },
  get: (id) => fetchAPI(`/priorities/${id}`),
  create: (data) =>
    fetchAPI('/priorities', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  update: (id, data) =>
    fetchAPI(`/priorities/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  delete: (id) =>
    fetchAPI(`/priorities/${id}`, {
      method: 'DELETE',
    }),
};

export const hierarchyLevels = {
  getAll: () => fetchAPI('/hierarchy-levels'),
  get: (id) => fetchAPI(`/hierarchy-levels/${id}`),
  create: (data) =>
    fetchAPI('/hierarchy-levels', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  update: (id, data) =>
    fetchAPI(`/hierarchy-levels/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  delete: (id) =>
    fetchAPI(`/hierarchy-levels/${id}`, {
      method: 'DELETE',
    }),
};

export const linkTypes = {
  getAll: (includeInactive = false) => {
    const params = includeInactive ? '?include_inactive=true' : '';
    return fetchAPI(`/link-types${params}`);
  },
  get: (id) => fetchAPI(`/link-types/${id}`),
  create: (data) =>
    fetchAPI('/admin/link-types', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  update: (id, data) =>
    fetchAPI(`/admin/link-types/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  delete: (id) =>
    fetchAPI(`/admin/link-types/${id}`, {
      method: 'DELETE',
    }),
};

export const links = {
  getForItem: (type, id) => fetchAPI(`/${type}/${id}/links`),
  create: (data) =>
    fetchAPI('/links', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  delete: (id) =>
    fetchAPI(`/links/${id}`, {
      method: 'DELETE',
    }),
  search: (query, type = '', limit = 20) => {
    const params = new URLSearchParams();
    if (query) params.append('q', query);
    if (type) params.append('type', type);
    if (limit !== 20) params.append('limit', limit.toString());
    return fetchAPI(`/links/search?${params.toString()}`);
  },
};
