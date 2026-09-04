import { API_BASE, fetchAPI, fetchAPIV2, fetchV2Data } from './core.js';
import { createCrudClient } from './createCrudClient.js';

export const projects = {
  ...createCrudClient('/projects'),
  getByWorkspace: (workspaceId) => fetchAPI(`/projects?workspace_id=${workspaceId}`),
  getMilestones: (id) => fetchAPI(`/projects/${id}/milestones`),
};

export const search = {
  items: (params = {}) => {
    const searchParams = new URLSearchParams();

    // Text search
    if (params.query) searchParams.append('q', params.query);

    // Multiple workspace IDs
    if (params.workspaceIds && params.workspaceIds.length > 0) {
      params.workspaceIds.forEach((id) => searchParams.append('workspace_id', id));
    }

    // Multiple statuses
    if (params.statuses && params.statuses.length > 0) {
      params.statuses.forEach((status) => searchParams.append('status', status));
    }

    // Multiple priorities
    if (params.priorities && params.priorities.length > 0) {
      params.priorities.forEach((priority) => searchParams.append('priority', priority));
    }

    // Limit
    if (params.limit) searchParams.append('limit', params.limit);

    return fetchAPI(`/items/search?${searchParams.toString()}`);
  },
};

export const homepage = {
  get: () => fetchAPI('/homepage'),
  getLayout: () => fetchAPI('/user/dashboard-layout'),
  updateLayout: (layout) =>
    fetchAPI('/user/dashboard-layout', {
      method: 'PUT',
      body: JSON.stringify(layout),
    }),
};

// Diagram API functions
export const getDiagrams = (itemId, requestOptions = {}) =>
  fetchV2Data(`/items/${itemId}/diagrams`, requestOptions);
export const getDiagram = (diagramId) => fetchV2Data(`/item-diagrams/${diagramId}`);
export const createDiagram = (itemId, name, diagramData) =>
  fetchV2Data(`/items/${itemId}/diagrams`, {
    method: 'POST',
    body: JSON.stringify({ name, diagram_data: diagramData }),
  });
export const updateDiagram = (diagramId, name, diagramData) =>
  fetchV2Data(`/item-diagrams/${diagramId}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/merge-patch+json' },
    body: JSON.stringify({ name, diagram_data: diagramData }),
  });
export const deleteDiagram = (diagramId) =>
  fetchV2Data(`/item-diagrams/${diagramId}`, {
    method: 'DELETE',
  });

// Comment API functions
export const getComments = (itemId, params = {}) => {
  const searchParams = new URLSearchParams();
  if (params.limit) searchParams.set('page_size', params.limit);
  if (params.cursor) searchParams.set('cursor', params.cursor);
  const query = searchParams.toString();
  return fetchV2Data(`/items/${itemId}/comments${query ? `?${query}` : ''}`);
};
export const createComment = (itemId, data) =>
  fetchV2Data(`/items/${itemId}/comments`, {
    method: 'POST',
    body: JSON.stringify({ content: data.content, is_private: Boolean(data.is_private) }),
  });
export const updateComment = (commentId, data) =>
  fetchV2Data(`/comments/${commentId}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/merge-patch+json' },
    body: JSON.stringify(data),
  });
export const deleteComment = (commentId) =>
  fetchV2Data(`/comments/${commentId}`, {
    method: 'DELETE',
  });

// Attachments
export const attachments = {
  getByItem: (itemId, params = {}) => {
    const { page, limit, ...requestOptions } = params;
    const searchParams = new URLSearchParams();
    if (page) searchParams.append('page', page);
    if (limit) searchParams.append('page_size', limit);
    const queryString = searchParams.toString();
    return fetchAPIV2(
      `/items/${itemId}/attachments${queryString ? `?${queryString}` : ''}`,
      requestOptions
    );
  },

  uploadToItem: (itemId, file) => {
    const formData = new FormData();
    formData.append('file', file);
    return fetchV2Data(`/items/${itemId}/attachments`, { method: 'POST', body: formData });
  },

  // Upload attachment (uses FormData, no JSON)
  upload: async (formData) => {
    const response = await fetch(`${API_BASE}/attachments/upload`, {
      method: 'POST',
      body: formData, // Don't stringify FormData
      credentials: 'same-origin',
      // Don't set Content-Type for FormData - browser sets it with boundary
    });

    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(errorText || 'Upload failed');
    }

    return response.json();
  },

  getDownloadUrl: (attachmentId) => `/api/v2/attachments/${attachmentId}/content`,

  // Get thumbnail URL for image attachments
  getThumbnailUrl: (attachmentId) => `/api/v2/attachments/${attachmentId}/thumbnail`,

  // Delete attachment
  delete: (attachmentId) =>
    fetchV2Data(`/attachments/${attachmentId}`, {
      method: 'DELETE',
    }),
};

// Attachment Settings (for admin)
export const attachmentSettings = {
  get: () => fetchAPI('/attachment-settings'),
  update: (id, data) =>
    fetchAPI(`/attachment-settings/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  getStatus: () => fetchAPI('/attachment-settings/status'),
};

// Reviews API (daily/weekly review feature)
export const reviews = {
  ...createCrudClient('/reviews'),
  getCompletedItems: (startDate, endDate) => {
    const params = new URLSearchParams();
    params.append('start_date', startDate);
    params.append('end_date', endDate);
    return fetchAPI(`/reviews/completed-items?${params.toString()}`);
  },
};

// Calendar Feed - ICS subscription management
export const calendarFeed = {
  // Get current user's feed token info
  getToken: () => fetchAPI('/calendar/feed/token'),

  // Create or regenerate feed token
  createToken: () =>
    fetchAPI('/calendar/feed/token', {
      method: 'POST',
    }),

  // Revoke feed token
  revokeToken: () =>
    fetchAPI('/calendar/feed/token', {
      method: 'DELETE',
    }),
};

// Named exports for backward compatibility
export const getCalendarFeedToken = calendarFeed.getToken;
export const createCalendarFeedToken = calendarFeed.createToken;
export const revokeCalendarFeedToken = calendarFeed.revokeToken;

// Personal Labels
//
// Calling getAll() with no argument returns the unified set: the caller's
// own labels (user_id = me) plus shared labels (user_id IS NULL). Pass an
// explicit userId only when you need the legacy filtered behavior.
export const personalLabels = {
  getAll: (userId = undefined) => {
    const params = new URLSearchParams();
    if (userId !== null && userId !== undefined && userId !== '') {
      params.append('user_id', userId);
    }
    const queryString = params.toString();
    return fetchAPI(`/personal-labels${queryString ? `?${queryString}` : ''}`);
  },
  get: (id) => fetchAPI(`/personal-labels/${id}`),
  create: (data) =>
    fetchAPI('/personal-labels', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  update: (id, data) =>
    fetchAPI(`/personal-labels/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  delete: (id) =>
    fetchAPI(`/personal-labels/${id}`, {
      method: 'DELETE',
    }),

  getForItem: (itemId) => fetchAPI(`/items/${itemId}/personal-labels`),
  setForItem: (itemId, labelIds) =>
    fetchAPI(`/items/${itemId}/personal-labels`, {
      method: 'PUT',
      body: JSON.stringify({ label_ids: labelIds }),
    }),
  addToItem: (itemId, labelId) =>
    fetchAPI(`/items/${itemId}/personal-labels`, {
      method: 'POST',
      body: JSON.stringify({ label_id: labelId }),
    }),
  removeFromItem: (itemId, labelId) =>
    fetchAPI(`/items/${itemId}/personal-labels/${labelId}`, {
      method: 'DELETE',
    }),
};

// Global item labels
export const labels = {
  getAll: (workspaceId) => fetchV2Data(`/workspaces/${workspaceId}/labels`),
  get: (workspaceId, id) => fetchV2Data(`/workspaces/${workspaceId}/labels/${id}`),
  create: ({ workspace_id: workspaceId, ...data }) =>
    fetchV2Data(`/workspaces/${workspaceId}/labels`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  update: (workspaceId, id, data) =>
    fetchV2Data(`/workspaces/${workspaceId}/labels/${id}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/merge-patch+json' },
      body: JSON.stringify(data),
    }),
  delete: (workspaceId, id) =>
    fetchAPIV2(`/workspaces/${workspaceId}/labels/${id}`, {
      method: 'DELETE',
    }),
  getForItem: (itemId) => fetchV2Data(`/items/${itemId}/labels`),
  setForItem: (itemId, labelIds) =>
    fetchV2Data(`/items/${itemId}/labels`, {
      method: 'PUT',
      body: JSON.stringify({ label_ids: labelIds }),
    }),
};

// Jira Cloud Import
export const jiraImport = {
  // List saved connections
  getConnections: () => fetchAPI('/admin/jira-import/connections'),
  // List import jobs
  getImportJobs: () => fetchAPI('/admin/jira-import/jobs'),
  // Test connection and store credentials
  testConnection: (data) =>
    fetchAPI('/admin/jira-import/connect', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  // Get available Jira projects. Counts are NOT fetched here by default —
  // use getProjectCounts() to batch counts for visible/selected projects so
  // the wizard advances immediately on large instances.
  getProjects: (connectionId) =>
    fetchAPI(`/admin/jira-import/projects?connection_id=${connectionId}`),
  // Batch fetch issue counts for a list of project keys.
  getProjectCounts: (connectionId, keys, openIssuesOnly = false) =>
    fetchAPI('/admin/jira-import/projects/counts', {
      method: 'POST',
      body: JSON.stringify({
        connection_id: connectionId,
        keys,
        open_issues_only: openIssuesOnly,
      }),
    }),
  // Analyze selected projects
  analyzeProjects: (connectionId, projectKeys, openIssuesOnly = false) =>
    fetchAPI('/admin/jira-import/analyze', {
      method: 'POST',
      body: JSON.stringify({
        connection_id: connectionId,
        project_keys: projectKeys,
        open_issues_only: openIssuesOnly,
      }),
    }),
  // Get asset schemas
  getAssetSchemas: (connectionId) =>
    fetchAPI(`/admin/jira-import/assets?connection_id=${connectionId}`),
  // Get object types for a schema
  getAssetTypes: (connectionId, schemaId) =>
    fetchAPI(`/admin/jira-import/assets/${schemaId}/types?connection_id=${connectionId}`),
  // Get import job status
  getJobStatus: (jobId) => fetchAPI(`/admin/jira-import/jobs/${jobId}`),
  // Delete connection
  deleteConnection: (connectionId) =>
    fetchAPI(`/admin/jira-import/connections/${connectionId}`, {
      method: 'DELETE',
    }),
  // Start import
  startImport: (data) =>
    fetchAPI('/admin/jira-import/start', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  // Delete all data mapped to an import job after explicit confirmation
  deleteImportedData: (jobId, confirmation) =>
    fetchAPI(`/admin/jira-import/jobs/${jobId}/data`, {
      method: 'DELETE',
      body: JSON.stringify(confirmation),
    }),
};
