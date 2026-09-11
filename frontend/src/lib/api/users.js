import { fetchAllV2Pages, fetchAPI, fetchV2Data } from './core.js';

// Users
export const getUsers = () => fetchAllV2Pages('/users');
export const getAdminUsers = () => fetchAllV2Pages('/admin/users');
export const getAssignableUsers = (workspaceId) =>
  fetchV2Data(`/workspaces/${workspaceId}/assignable-users`);
export const getUser = (id) => fetchV2Data(`/users/${id}`);
export const getAgentOwner = (id) => fetchAPI(`/users/${id}/agent-owner`);
export const createUser = (data) =>
  fetchAPI('/users', {
    method: 'POST',
    body: JSON.stringify(data),
  });
export const inviteUser = (data) =>
  fetchAPI('/users/invite', {
    method: 'POST',
    body: JSON.stringify(data),
  });
export const updateUser = (id, data) =>
  fetchV2Data(`/admin/users/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/merge-patch+json' },
    body: JSON.stringify(
      Object.fromEntries(
        [
          'email',
          'username',
          'first_name',
          'last_name',
          'avatar_url',
          'timezone',
          'language',
          'is_active',
        ]
          .filter((field) => data[field] !== undefined)
          .map((field) => [field, data[field]])
      )
    ),
  });
export const updateUserAvatar = (id, avatar_url) =>
  fetchAPI(`/users/${id}/avatar`, {
    method: 'PUT',
    body: JSON.stringify({ avatar_url }),
  });
export const updateUserRegionalSettings = (id, data) =>
  fetchAPI(`/users/${id}/regional-settings`, {
    method: 'PUT',
    body: JSON.stringify(data),
  });
export const deleteUser = (id) =>
  fetchAPI(`/users/${id}`, {
    method: 'DELETE',
  });
export const resetUserPassword = (id, payload) =>
  fetchAPI(`/users/${id}/reset-password`, {
    method: 'POST',
    body: JSON.stringify(payload || { generate_random: true }),
  });
export const activateUser = (id) =>
  fetchAPI(`/users/${id}/activate`, {
    method: 'POST',
  });
export const deactivateUser = (id) =>
  fetchAPI(`/users/${id}/deactivate`, {
    method: 'POST',
  });

// User Credentials
export const getUserCredentials = (userId) => fetchAPI(`/users/${userId}/credentials`);
export const startFIDORegistration = (userId, credentialName) =>
  fetchAPI(`/users/${userId}/credentials/webauthn/register/start`, {
    method: 'POST',
    body: JSON.stringify({ credential_name: credentialName }),
  });
export const completeFIDORegistration = (userId, credentialData) =>
  fetchAPI(`/users/${userId}/credentials/webauthn/register/complete`, {
    method: 'POST',
    body: JSON.stringify(credentialData),
  });
export const createSSHKey = (userId, credentialName, publicKey) =>
  fetchAPI(`/users/${userId}/credentials/ssh`, {
    method: 'POST',
    body: JSON.stringify({
      credential_name: credentialName,
      public_key: publicKey,
    }),
  });
export const removeUserCredential = (userId, credentialId) =>
  fetchAPI(`/users/${userId}/credentials/${credentialId}`, {
    method: 'DELETE',
  });

// My Agents (user-managed agents owned by the current user)
export const getMyAgents = () => fetchAPI('/me/agents');
export const createMyAgent = (data) =>
  fetchAPI('/me/agents', {
    method: 'POST',
    body: JSON.stringify(data),
  });
export const updateMyAgent = (agentId, data) =>
  fetchAPI(`/me/agents/${agentId}`, {
    method: 'PATCH',
    body: JSON.stringify(data),
  });
export const deleteMyAgent = (agentId) =>
  fetchAPI(`/me/agents/${agentId}`, {
    method: 'DELETE',
  });

// API Tokens
export const getApiTokens = (userId) =>
  fetchAPI(userId ? `/api-tokens?user_id=${userId}` : '/api-tokens');
export const createApiToken = (data) =>
  fetchAPI('/api-tokens', {
    method: 'POST',
    body: JSON.stringify(data),
  });
export const getApiToken = (tokenId) => fetchAPI(`/api-tokens/${tokenId}`);
export const revokeApiToken = (tokenId) =>
  fetchAPI(`/api-tokens/${tokenId}`, {
    method: 'DELETE',
  });
export const validateApiToken = () => fetchAPI('/api-tokens/validate');
// The authoritative list of grantable scopes, with the label/description/
// grouping each picker renders. Sourced from auth.ScopeCatalog server-side so
// the pickers can't drift from what the server actually accepts.
export const getScopeCatalog = () => fetchAPI('/api-tokens/scope-catalog');

// CLI onboarding (used by `ws init` — consent page + status probe)
export const cliAuth = {
  capabilities: () => fetchAPI('/cli/capabilities'),
  approve: (data) =>
    fetchAPI('/cli/auth/approve', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  deny: (data) =>
    fetchAPI('/cli/auth/deny', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
};

// User Preferences API
export const userPreferences = {
  // Get current user's preferences
  get: () => fetchV2Data('/users/me/preferences'),

  // Update current user's preferences
  update: (data) =>
    fetchV2Data('/users/me/preferences', {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/merge-patch+json' },
      body: JSON.stringify(data),
    }),
};
