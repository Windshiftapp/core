import { fetchAPI } from './core.js';
import { createCrudClient } from './createCrudClient.js';

// Provider administration and OAuth.
export const scmProviders = {
  ...createCrudClient('/admin/scm-providers'),

  test: (id) =>
    fetchAPI(`/admin/scm-providers/${id}/test`, {
      method: 'POST',
    }),

  startOAuth: (slug) => fetchAPI(`/scm/oauth/${slug}/start`),

  getAllowedWorkspaces: (id) => fetchAPI(`/admin/scm-providers/${id}/allowed-workspaces`),

  updateAllowedWorkspaces: (id, workspaceIds) =>
    fetchAPI(`/admin/scm-providers/${id}/allowed-workspaces`, {
      method: 'PUT',
      body: JSON.stringify({ workspace_ids: workspaceIds }),
    }),

  addAllowedWorkspace: (id, workspaceId) =>
    fetchAPI(`/admin/scm-providers/${id}/allowed-workspaces`, {
      method: 'POST',
      body: JSON.stringify({ workspace_id: workspaceId }),
    }),

  removeAllowedWorkspace: (id, workspaceId) =>
    fetchAPI(`/admin/scm-providers/${id}/allowed-workspaces/${workspaceId}`, {
      method: 'DELETE',
    }),
};

// Workspace connections, repositories, and authentication.
export const workspaceSCM = {
  getAccessibleConnections: () => fetchAPI('/scm-connections'),

  getAvailableProviders: (workspaceId) => fetchAPI(`/workspaces/${workspaceId}/scm-providers`),

  getConnections: (workspaceId) => fetchAPI(`/workspaces/${workspaceId}/scm-connections`),

  // Optional expansions avoid loading repository and auth summaries by default.
  getConnectionsOverview: (
    workspaceId,
    { includeRepositories = false, includeAuthStatus = false } = {}
  ) => {
    const query = new URLSearchParams();
    if (includeRepositories) query.set('include_repositories', 'true');
    if (includeAuthStatus) query.set('include_auth_status', 'true');
    const suffix = query.size > 0 ? `?${query}` : '';
    return fetchAPI(`/workspaces/${workspaceId}/scm-connections${suffix}`);
  },

  createConnection: (workspaceId, data) =>
    fetchAPI(`/workspaces/${workspaceId}/scm-connections`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  getConnection: (workspaceId, connId) =>
    fetchAPI(`/workspaces/${workspaceId}/scm-connections/${connId}`),

  updateConnection: (workspaceId, connId, data) =>
    fetchAPI(`/workspaces/${workspaceId}/scm-connections/${connId}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  deleteConnection: (workspaceId, connId) =>
    fetchAPI(`/workspaces/${workspaceId}/scm-connections/${connId}`, {
      method: 'DELETE',
    }),

  getAvailableRepos: (workspaceId, connId, params = {}) => {
    const queryString = new URLSearchParams(params).toString();
    const url = `/workspaces/${workspaceId}/scm-connections/${connId}/repositories/available${queryString ? `?${queryString}` : ''}`;
    return fetchAPI(url);
  },

  getLinkedRepos: (workspaceId, connId) =>
    fetchAPI(`/workspaces/${workspaceId}/scm-connections/${connId}/repositories`),

  linkRepo: (workspaceId, connId, data) =>
    fetchAPI(`/workspaces/${workspaceId}/scm-connections/${connId}/repositories`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  unlinkRepo: (repoId) =>
    fetchAPI(`/workspace-repositories/${repoId}`, {
      method: 'DELETE',
    }),

  // Sends only the fields being changed.
  updateRepo: (repoId, data) =>
    fetchAPI(`/workspace-repositories/${repoId}`, {
      method: 'PATCH',
      body: JSON.stringify(data),
    }),

  syncRepo: (repoId) =>
    fetchAPI(`/workspace-repositories/${repoId}/sync`, {
      method: 'POST',
    }),

  getReleaseCandidates: (repoId) =>
    fetchAPI(`/workspace-repositories/${repoId}/release-candidates`),

  getWebhookConfig: (repoId) => fetchAPI(`/workspace-repositories/${repoId}/webhook`),

  rotateWebhookSecret: (repoId) =>
    fetchAPI(`/workspace-repositories/${repoId}/webhook/rotate-secret`, { method: 'POST' }),

  deleteWebhookConfig: (repoId) =>
    fetchAPI(`/workspace-repositories/${repoId}/webhook`, { method: 'DELETE' }),

  startOAuth: (workspaceId, connId) =>
    fetchAPI(`/workspaces/${workspaceId}/scm-connections/${connId}/auth/start`, {
      method: 'POST',
    }),

  getAuthStatus: (workspaceId, connId) =>
    fetchAPI(`/workspaces/${workspaceId}/scm-connections/${connId}/auth/status`),
};

// Pull requests, branches, and commits linked to work items.
export const itemSCMLinks = {
  get: (itemId, options = {}) => fetchAPI(`/items/${itemId}/scm-links`, options),

  create: (itemId, data) =>
    fetchAPI(`/items/${itemId}/scm-links`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  delete: (linkId) =>
    fetchAPI(`/item-scm-links/${linkId}`, {
      method: 'DELETE',
    }),

  refresh: (linkId) =>
    fetchAPI(`/item-scm-links/${linkId}/refresh`, {
      method: 'POST',
    }),

  getRepositories: (itemId) => fetchAPI(`/items/${itemId}/scm-repositories`),

  createBranch: (itemId, data) =>
    fetchAPI(`/items/${itemId}/scm-links/create-branch`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  createPRFromBranch: (linkId, data) =>
    fetchAPI(`/item-scm-links/${linkId}/create-pr`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  getConnectionStatus: (itemId, options = {}) =>
    fetchAPI(`/items/${itemId}/scm-connection-status`, options),
};

// GitHub issue-sync configuration and status.
export const issueSync = {
  getConfig: (workspaceId) => fetchAPI(`/workspaces/${workspaceId}/issue-sync`),

  createConfig: (workspaceId, data) =>
    fetchAPI(`/workspaces/${workspaceId}/issue-sync`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  updateConfig: (workspaceId, data) =>
    fetchAPI(`/workspaces/${workspaceId}/issue-sync`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  deleteConfig: (workspaceId) =>
    fetchAPI(`/workspaces/${workspaceId}/issue-sync`, {
      method: 'DELETE',
    }),

  triggerSync: (workspaceId) =>
    fetchAPI(`/workspaces/${workspaceId}/issue-sync/trigger`, {
      method: 'POST',
    }),

  getStatus: (workspaceId) => fetchAPI(`/workspaces/${workspaceId}/issue-sync/status`),

  getItems: (workspaceId) => fetchAPI(`/workspaces/${workspaceId}/issue-sync/items`),

  getGitHubLabels: (workspaceId, repositoryId) =>
    fetchAPI(`/workspaces/${workspaceId}/issue-sync/github-labels?repository_id=${repositoryId}`),

  getGitHubMilestones: (workspaceId, repositoryId) =>
    fetchAPI(
      `/workspaces/${workspaceId}/issue-sync/github-milestones?repository_id=${repositoryId}`
    ),
};

// Personal SCM OAuth connections.
export const userSCM = {
  getConnections: () => fetchAPI('/users/me/scm-connections'),

  getAvailableProviders: () => fetchAPI('/users/me/scm-connections/available'),

  getConnectionStatus: (providerId) => fetchAPI(`/users/me/scm-connections/${providerId}`),

  disconnect: (providerId) =>
    fetchAPI(`/users/me/scm-connections/${providerId}`, {
      method: 'DELETE',
    }),
};
