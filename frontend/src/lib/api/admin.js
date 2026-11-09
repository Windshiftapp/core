import { fetchAPI } from './core.js';
import { createCrudClient } from './createCrudClient.js';

export const setup = {
  getStatus: (options) => fetchAPI('/setup/status', options),
  complete: (data) =>
    fetchAPI('/setup/complete', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  getModuleSettings: () => fetchAPI('/setup/modules'),
  updateModuleSettings: (data) =>
    fetchAPI('/setup/modules', {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
};

export const shellBootstrap = {
  get: (options) => fetchAPI('/shell-bootstrap', options),
};

// Instance-wide general configuration (admin)
export const generalSettings = {
  get: () => fetchAPI('/admin/general-settings'),
  update: (data) =>
    fetchAPI('/admin/general-settings', {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
};

export const system = {
  shutdown: () =>
    fetchAPI('/shutdown', {
      method: 'POST',
    }),
};

export const themes = {
  ...createCrudClient('/themes'),
  getActive: (options) => fetchAPI('/themes/active', options),
  activate: (id) =>
    fetchAPI(`/themes/${id}/activate`, {
      method: 'POST',
    }),
};

// Security Settings (admin only)
export const securitySettings = {
  // Get current security settings
  get: () => fetchAPI('/admin/security-settings'),

  // Update security settings
  update: (data) =>
    fetchAPI('/admin/security-settings', {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
};

// Named exports for backward compatibility
export const getSecuritySettings = securitySettings.get;
export const updateSecuritySettings = securitySettings.update;

// Authentication Policy (admin only)
export const authPolicy = {
  // Get current auth policy configuration
  get: () => fetchAPI('/admin/auth-policy'),

  // Update auth policy
  update: (data) =>
    fetchAPI('/admin/auth-policy', {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  // Get statistics for policy planning
  getStats: () => fetchAPI('/admin/auth-policy/stats'),

  // Get a bounded page of users affected by current policy.
  // Supports `limit` and `cursor` (the previous page's next_cursor); the
  // response carries `users`, `total_count`, `has_more`, and `next_cursor`.
  getAffected: ({ limit = 50, cursor = '' } = {}) => {
    const params = new URLSearchParams({ limit: String(limit) });
    if (cursor) params.set('cursor', cursor);
    return fetchAPI(`/admin/auth-policy/affected?${params}`);
  },

  // Get public policy status (no auth required - for login page)
  getPublicStatus: () => fetchAPI('/auth/policy-status'),
};

// OAuth clients (admin only). Manages registered third-party apps that can
// drive the generic OAuth 2.0 server (`/api/oauth/authorize` + `/api/oauth/token`).
// `create` and `rotateSecret` return the plaintext `client_secret` exactly
// once — the server stores only its bcrypt hash.
export const oauthClients = {
  ...createCrudClient('/admin/oauth-clients'),
  rotateSecret: (id) =>
    fetchAPI(`/admin/oauth-clients/${id}/rotate-secret`, {
      method: 'POST',
    }),
};

// Agent template catalog (admin only). System-admin overrides for the Agent
// Studio creation catalog (WI-922). `defaults` returns the embedded default
// catalog so admins can seed an override that overwrites a built-in template.
export const agentTemplates = {
  ...createCrudClient('/admin/agent-templates'),
  defaults: () => fetchAPI('/admin/agent-templates/defaults'),
};
