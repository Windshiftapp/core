import { API_BASE, fetchAPI, fetchAPIV2, fetchV2Data } from './core.js';
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
  // Workspace-scoped assignment (WI-1359): workspace admins pick the set their
  // workspace uses; pass null to unassign. Server touches only the join rows.
  assignToWorkspace: (workspaceId, configurationSetId) =>
    fetchAPI(`/workspaces/${workspaceId}/configuration-set`, {
      method: 'PUT',
      body: JSON.stringify({ configuration_set_id: configurationSetId }),
    }),
  // Direct download URL for the export endpoint. Browsers carry the session
  // cookie automatically; bind this to an <a download> rather than calling
  // fetchAPI so the response streams to a file.
  exportUrl: (id) => `${API_BASE}/configuration-sets/${id}/export`,
  // Multipart upload of an exported configuration-set bundle. Bypasses
  // fetchAPI because that helper sets Content-Type: application/json — for
  // multipart we need the browser to write its own boundary.
  import: async (file) => {
    const fd = new FormData();
    fd.append('file', file);
    const response = await fetch(`${API_BASE}/configuration-sets/import`, {
      method: 'POST',
      body: fd,
      credentials: 'same-origin',
    });
    const text = await response.text();
    let body = null;
    try {
      body = text ? JSON.parse(text) : null;
    } catch {
      // Non-JSON error body: surface raw text below.
    }
    if (!response.ok) {
      const err = /** @type {any} */ (
        new Error((body && (body.error || body.message)) || text || 'Import failed')
      );
      // Carry structured details so the UI can render the unresolved-refs list.
      err.status = response.status;
      err.code = body?.code;
      err.details = body?.details || {};
      throw err;
    }
    return body;
  },
};

export const screens = {
  ...createCrudClient('/screens'),
  getAllWithFields: () => fetchAPI('/screens?include_fields=true'),
  getFields: (id) => fetchAPI(`/screens/${id}/fields`),
  updateFields: (id, data) =>
    fetchAPI(`/screens/${id}/fields`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
};

export const catalogs = {
  // One request for every global reference catalog. Custom fields are
  // optional: the server returns null plus custom_fields_error when the
  // caller may not read them or the reader failed.
  getGlobalBootstrap: (requestOptions = {}) =>
    fetchV2Data('/catalogs/global-bootstrap', requestOptions),
};

export const customFields = {
  ...createCrudClient('/custom-fields', { adminBasePath: '/admin/custom-fields', readV2: true }),
  getOverview: async (requestOptions = {}) => {
    const document = await fetchAPIV2('/custom-fields', requestOptions);
    return {
      customFields: document?.data ?? [],
      indexCounts: document?.meta?.index_counts ?? {},
    };
  },
  updateSettings: (data) =>
    fetchAPI('/admin/custom-fields/settings', {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
};

export const itemTypes = createCrudClient('/item-types', { v2: true });

export const itemTemplates = createCrudClient('/item-templates', {
  parentPath: '/workspaces',
  v2: true,
});

export const cannedResponses = {
  getAll: (workspaceId, includeArchived = false) =>
    fetchV2Data(
      `/workspaces/${workspaceId}/canned-responses${includeArchived ? '?include_archived=true' : ''}`
    ),
  get: (workspaceId, responseId) =>
    fetchV2Data(`/workspaces/${workspaceId}/canned-responses/${responseId}`),
  create: (workspaceId, data) =>
    fetchV2Data(`/workspaces/${workspaceId}/canned-responses`, {
      method: 'POST',
      body: JSON.stringify({
        name: data.name,
        body: data.body,
        is_private: Boolean(data.is_private),
      }),
    }),
  update: (workspaceId, responseId, data) =>
    fetchV2Data(`/workspaces/${workspaceId}/canned-responses/${responseId}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/merge-patch+json' },
      body: JSON.stringify(data),
    }),
  delete: (workspaceId, responseId) =>
    fetchV2Data(`/workspaces/${workspaceId}/canned-responses/${responseId}`, { method: 'DELETE' }),
  preview: (workspaceId, responseId, itemId = 0) =>
    fetchV2Data(`/workspaces/${workspaceId}/canned-responses/${responseId}/preview`, {
      method: 'POST',
      body: JSON.stringify({ item_id: itemId }),
    }),
};

export const priorities = createCrudClient('/priorities', { v2: true });

export const hierarchyLevels = createCrudClient('/hierarchy-levels');

export const linkTypes = {
  ...createCrudClient('/link-types', { v2: true }),
  // One caller passes a boolean (LinkTypeManager); keep that signature.
  getAll: (includeInactive = false, requestOptions = {}) =>
    fetchV2Data(`/link-types${includeInactive ? '?include_inactive=true' : ''}`, requestOptions),
};

export const links = {
  getForItem: (type, id, requestOptions = {}) =>
    fetchV2Data(`/${type}/${id}/links`, requestOptions),
  // Batch variant of getForItem('items', ...): returns links keyed by item id
  // ({ "<id>": { outgoing, incoming } }) for many items in one request. Used by
  // board/roadmap dependency badges so a board render is one request instead of
  // one per card. Callers chunk to the 200-item cap; the server accepts up to 500.
  getForItems: async (ids, options = {}) => {
    const { includeCustomFields = false, ...requestOptions } = options;
    const customFields = includeCustomFields ? '&include_custom_fields=true' : '';
    const document = await fetchAPIV2(
      `/links/batch?ids=${ids.join(',')}&page_size=100${customFields}`,
      requestOptions
    );
    return Object.fromEntries((document?.data ?? []).map((group) => [group.item_id, group]));
  },
  // Symmetric to getForItem for the page-detail "Work items" popover.
  // Routes through the same handler (GET /pages/{id}/links).
  getForPage: (pageId) => fetchV2Data(`/pages/${pageId}/links`),
  getFieldLinks: (itemId, fieldId) => fetchV2Data(`/items/${itemId}/fields/${fieldId}/links`),
  create: (data) =>
    fetchV2Data('/links', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  delete: (id) =>
    fetchV2Data(`/links/${id}`, {
      method: 'DELETE',
    }),
  search: (query, type = '', limit = 20, itemTypeIds = []) => {
    const params = new URLSearchParams();
    if (query) params.append('q', query);
    if (type) params.append('type', type);
    if (limit !== 20) params.append('limit', limit.toString());
    if (itemTypeIds.length > 0) params.append('item_type_ids', itemTypeIds.join(','));
    return fetchV2Data(`/links/search?${params.toString()}`);
  },
};
