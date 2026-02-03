import { fetchAPI } from './core.js';

// SCM (Source Control Management) providers - GitHub, GitLab, Gitea, Bitbucket
export const scmProviders = {
  // List all providers
  getAll: () => fetchAPI('/scm-providers'),

  // Get a specific provider
  get: (id) => fetchAPI(`/scm-providers/${id}`),

  // Create a new provider
  create: (data) =>
    fetchAPI('/scm-providers', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  // Update a provider
  update: (id, data) =>
    fetchAPI(`/scm-providers/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  // Delete a provider
  delete: (id) =>
    fetchAPI(`/scm-providers/${id}`, {
      method: 'DELETE',
    }),

  // Test provider connection
  test: (id) =>
    fetchAPI(`/scm-providers/${id}/test`, {
      method: 'POST',
    }),

  // Start OAuth flow (returns auth URL)
  startOAuth: (slug) => fetchAPI(`/scm/oauth/${slug}/start`),

  // Get allowed workspaces for a provider
  getAllowedWorkspaces: (id) => fetchAPI(`/scm-providers/${id}/allowed-workspaces`),

  // Update allowed workspaces (replace entire list)
  updateAllowedWorkspaces: (id, workspaceIds) =>
    fetchAPI(`/scm-providers/${id}/allowed-workspaces`, {
      method: 'PUT',
      body: JSON.stringify({ workspace_ids: workspaceIds }),
    }),

  // Add a workspace to the allowlist
  addAllowedWorkspace: (id, workspaceId) =>
    fetchAPI(`/scm-providers/${id}/allowed-workspaces`, {
      method: 'POST',
      body: JSON.stringify({ workspace_id: workspaceId }),
    }),

  // Remove a workspace from the allowlist
  removeAllowedWorkspace: (id, workspaceId) =>
    fetchAPI(`/scm-providers/${id}/allowed-workspaces/${workspaceId}`, {
      method: 'DELETE',
    }),
};

// Workspace SCM connections and repositories
export const workspaceSCM = {
  // Get available SCM providers for a workspace (enabled providers with connection status)
  getAvailableProviders: (workspaceId) => fetchAPI(`/workspaces/${workspaceId}/scm-providers`),

  // Get all SCM connections for a workspace
  getConnections: (workspaceId) => fetchAPI(`/workspaces/${workspaceId}/scm-connections`),

  // Create a new SCM connection for a workspace
  createConnection: (workspaceId, data) =>
    fetchAPI(`/workspaces/${workspaceId}/scm-connections`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  // Get a specific SCM connection
  getConnection: (workspaceId, connId) =>
    fetchAPI(`/workspaces/${workspaceId}/scm-connections/${connId}`),

  // Update an SCM connection
  updateConnection: (workspaceId, connId, data) =>
    fetchAPI(`/workspaces/${workspaceId}/scm-connections/${connId}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  // Delete an SCM connection
  deleteConnection: (workspaceId, connId) =>
    fetchAPI(`/workspaces/${workspaceId}/scm-connections/${connId}`, {
      method: 'DELETE',
    }),

  // Get available repositories from the provider (not yet linked)
  getAvailableRepos: (workspaceId, connId, params = {}) => {
    const queryString = new URLSearchParams(params).toString();
    const url = `/workspaces/${workspaceId}/scm-connections/${connId}/repositories/available${queryString ? `?${queryString}` : ''}`;
    return fetchAPI(url);
  },

  // Get linked repositories for a connection
  getLinkedRepos: (workspaceId, connId) =>
    fetchAPI(`/workspaces/${workspaceId}/scm-connections/${connId}/repositories`),

  // Link a repository to a workspace connection
  linkRepo: (workspaceId, connId, data) =>
    fetchAPI(`/workspaces/${workspaceId}/scm-connections/${connId}/repositories`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  // Unlink a repository
  unlinkRepo: (repoId) =>
    fetchAPI(`/workspace-repositories/${repoId}`, {
      method: 'DELETE',
    }),

  // Trigger manual sync for a repository
  syncRepo: (repoId) =>
    fetchAPI(`/workspace-repositories/${repoId}/sync`, {
      method: 'POST',
    }),
};

// Item SCM Links - PRs, branches, commits linked to items
export const itemSCMLinks = {
  // Get all SCM links for an item
  get: (itemId) => fetchAPI(`/items/${itemId}/scm-links`),

  // Create a new SCM link for an item
  create: (itemId, data) =>
    fetchAPI(`/items/${itemId}/scm-links`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  // Delete an SCM link
  delete: (linkId) =>
    fetchAPI(`/item-scm-links/${linkId}`, {
      method: 'DELETE',
    }),

  // Refresh an SCM link's details from the provider
  refresh: (linkId) =>
    fetchAPI(`/item-scm-links/${linkId}/refresh`, {
      method: 'POST',
    }),

  // Get available repositories for an item (based on item's workspace)
  getRepositories: (itemId) => fetchAPI(`/items/${itemId}/scm-repositories`),

  // Create a branch (and optionally a draft PR) for an item
  createBranch: (itemId, data) =>
    fetchAPI(`/items/${itemId}/scm-links/create-branch`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  // Create a pull request from an existing branch link
  createPRFromBranch: (linkId, data) =>
    fetchAPI(`/item-scm-links/${linkId}/create-pr`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  // Get SCM connection status for an item (whether user has connected their account)
  getConnectionStatus: (itemId) => fetchAPI(`/items/${itemId}/scm-connection-status`),
};

// User SCM connections - personal OAuth token management
export const userSCM = {
  // Get all user's connected SCM accounts
  getConnections: () => fetchAPI('/users/me/scm-connections'),

  // Get available OAuth providers with connection status
  getAvailableProviders: () => fetchAPI('/users/me/scm-connections/available'),

  // Get connection status for a specific provider
  getConnectionStatus: (providerId) => fetchAPI(`/users/me/scm-connections/${providerId}`),

  // Disconnect from a provider (revoke OAuth token)
  disconnect: (providerId) =>
    fetchAPI(`/users/me/scm-connections/${providerId}`, {
      method: 'DELETE',
    }),
};
