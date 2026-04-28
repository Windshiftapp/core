import { API_BASE, fetchAPI } from './core.js';
import { createCrudClient } from './createCrudClient.js';

export const projects = {
  ...createCrudClient('/projects'),
  getByWorkspace: (workspaceId) => fetchAPI(`/projects?workspace_id=${workspaceId}`),
  getIssues: (id) => fetchAPI(`/projects/${id}/issues`),
  getMilestones: (id) => fetchAPI(`/projects/${id}/milestones`),
};

export const issues = {
  ...createCrudClient('/issues'),
  getComments: (id) => fetchAPI(`/issues/${id}/comments`),
  createComment: (id, data) =>
    fetchAPI(`/issues/${id}/comments`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
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
export const getDiagrams = (itemId) => fetchAPI(`/items/${itemId}/diagrams`);
export const getDiagram = (diagramId) => fetchAPI(`/diagrams/${diagramId}`);
export const createDiagram = (itemId, name, diagramData) =>
  fetchAPI(`/items/${itemId}/diagrams`, {
    method: 'POST',
    body: JSON.stringify({ name, diagram_data: diagramData }),
  });
export const updateDiagram = (diagramId, name, diagramData) =>
  fetchAPI(`/diagrams/${diagramId}`, {
    method: 'PUT',
    body: JSON.stringify({ name, diagram_data: diagramData }),
  });
export const deleteDiagram = (diagramId) =>
  fetchAPI(`/diagrams/${diagramId}`, {
    method: 'DELETE',
  });

// Comment API functions
export const getComments = (itemId) => fetchAPI(`/items/${itemId}/comments`);
export const createComment = (itemId, data) =>
  fetchAPI(`/items/${itemId}/comments`, {
    method: 'POST',
    body: JSON.stringify(data),
  });
export const updateComment = (commentId, data) =>
  fetchAPI(`/comments/${commentId}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  });
export const deleteComment = (commentId) =>
  fetchAPI(`/comments/${commentId}`, {
    method: 'DELETE',
  });

// Attachments
export const attachments = {
  // Get attachments for an item with pagination support
  getByItem: (itemId, params = {}) => {
    const searchParams = new URLSearchParams();
    if (params.page) searchParams.append('page', params.page);
    if (params.limit) searchParams.append('limit', params.limit);
    const queryString = searchParams.toString();
    return fetchAPI(`/items/${itemId}/attachments${queryString ? `?${queryString}` : ''}`);
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

  // Download attachment (returns URL for download)
  getDownloadUrl: (attachmentId) => `${API_BASE}/attachments/${attachmentId}/download`,

  // Get thumbnail URL for image attachments
  getThumbnailUrl: (attachmentId) => `${API_BASE}/attachments/${attachmentId}/thumbnail`,

  // Delete attachment
  delete: (attachmentId) =>
    fetchAPI(`/attachments/${attachmentId}`, {
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
  // Get available Jira projects
  getProjects: (connectionId, openIssuesOnly = false) =>
    fetchAPI(
      `/admin/jira-import/projects?connection_id=${connectionId}&open_issues_only=${openIssuesOnly}`
    ),
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
};
