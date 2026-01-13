import { fetchAPI } from './core.js';

// Users
export const getUsers = () => fetchAPI('/users');
export const getUser = (id) => fetchAPI(`/users/${id}`);
export const createUser = (data) => fetchAPI('/users', {
  method: 'POST',
  body: JSON.stringify(data),
});
export const updateUser = (id, data) => fetchAPI(`/users/${id}`, {
  method: 'PUT',
  body: JSON.stringify(data),
});
export const updateUserAvatar = (id, avatar_url) => fetchAPI(`/users/${id}/avatar`, {
  method: 'PUT',
  body: JSON.stringify({ avatar_url }),
});
export const updateUserRegionalSettings = (id, data) => fetchAPI(`/users/${id}/regional-settings`, {
  method: 'PUT',
  body: JSON.stringify(data),
});
export const deleteUser = (id) => fetchAPI(`/users/${id}`, {
  method: 'DELETE',
});
export const resetUserPassword = (id, payload) => fetchAPI(`/users/${id}/reset-password`, {
  method: 'POST',
  body: JSON.stringify(payload || { generate_random: true }),
});
export const activateUser = (id) => fetchAPI(`/users/${id}/activate`, {
  method: 'POST',
});
export const deactivateUser = (id) => fetchAPI(`/users/${id}/deactivate`, {
  method: 'POST',
});

// User Credentials
export const getUserCredentials = (userId) => fetchAPI(`/users/${userId}/credentials`);
export const startFIDORegistration = (userId, credentialName) => fetchAPI(`/users/${userId}/credentials/fido/register`, {
  method: 'POST',
  body: JSON.stringify({ credential_name: credentialName }),
});
export const completeFIDORegistration = (userId, credentialData) => fetchAPI(`/users/${userId}/credentials/fido/complete`, {
  method: 'POST',
  body: JSON.stringify(credentialData),
});
export const createSSHKey = (userId, credentialName, publicKey) => fetchAPI(`/users/${userId}/credentials/ssh`, {
  method: 'POST',
  body: JSON.stringify({
    credential_name: credentialName,
    public_key: publicKey
  }),
});
export const removeUserCredential = (userId, credentialId) => fetchAPI(`/users/${userId}/credentials/${credentialId}`, {
  method: 'DELETE',
});

// App Tokens
export const getUserAppTokens = (userId) => fetchAPI(`/users/${userId}/tokens`);
export const createAppToken = (userId, data) => fetchAPI(`/users/${userId}/tokens`, {
  method: 'POST',
  body: JSON.stringify(data),
});
export const updateAppToken = (userId, tokenId, data) => fetchAPI(`/users/${userId}/tokens/${tokenId}`, {
  method: 'PUT',
  body: JSON.stringify(data),
});
export const revokeAppToken = (userId, tokenId) => fetchAPI(`/users/${userId}/tokens/${tokenId}`, {
  method: 'DELETE',
});

// API Tokens
export const getApiTokens = () => fetchAPI('/api-tokens');
export const createApiToken = (data) => fetchAPI('/api-tokens', {
  method: 'POST',
  body: JSON.stringify(data),
});
export const getApiToken = (tokenId) => fetchAPI(`/api-tokens/${tokenId}`);
export const revokeApiToken = (tokenId) => fetchAPI(`/api-tokens/${tokenId}`, {
  method: 'DELETE',
});
export const validateApiToken = () => fetchAPI('/api-tokens/validate');

// User Preferences API
export const userPreferences = {
  // Get current user's preferences
  get: () => fetchAPI('/user/preferences'),

  // Update current user's preferences
  update: (data) => fetchAPI('/user/preferences', {
    method: 'PUT',
    body: JSON.stringify(data),
  }),
};
