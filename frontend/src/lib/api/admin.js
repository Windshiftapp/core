import { fetchAPI } from './core.js';

export const setup = {
  getStatus: () => fetchAPI('/setup/status'),
  complete: (data) => fetchAPI('/setup/complete', {
    method: 'POST',
    body: JSON.stringify(data),
  }),
  getModuleSettings: () => fetchAPI('/setup/modules'),
  updateModuleSettings: (data) => fetchAPI('/setup/modules', {
    method: 'PUT',
    body: JSON.stringify(data),
  }),
};

export const system = {
  shutdown: () => fetchAPI('/shutdown', {
    method: 'POST',
  }),
};

export const themes = {
  // Get all themes
  getAll: () => fetchAPI('/themes'),

  // Get active theme
  getActive: () => fetchAPI('/themes/active'),

  // Get a specific theme by ID
  get: (id) => fetchAPI(`/themes/${id}`),

  // Create a new theme
  create: (data) => fetchAPI('/themes', {
    method: 'POST',
    body: JSON.stringify(data),
  }),

  // Update an existing theme
  update: (id, data) => fetchAPI(`/themes/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  }),

  // Delete a theme
  delete: (id) => fetchAPI(`/themes/${id}`, {
    method: 'DELETE',
  }),

  // Activate a theme
  activate: (id) => fetchAPI(`/themes/${id}/activate`, {
    method: 'POST',
  }),
};

// Security Settings (admin only)
export const securitySettings = {
  // Get current security settings
  get: () => fetchAPI('/admin/security-settings'),

  // Update security settings
  update: (data) => fetchAPI('/admin/security-settings', {
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
  update: (data) => fetchAPI('/admin/auth-policy', {
    method: 'PUT',
    body: JSON.stringify(data),
  }),

  // Get statistics for policy planning
  getStats: () => fetchAPI('/admin/auth-policy/stats'),

  // Get list of users affected by current policy
  getAffected: () => fetchAPI('/admin/auth-policy/affected'),

  // Get public policy status (no auth required - for login page)
  getPublicStatus: () => fetchAPI('/auth/policy-status'),
};

// Named exports for auth policy
export const getAuthPolicy = authPolicy.get;
export const updateAuthPolicy = authPolicy.update;
export const getAuthPolicyStats = authPolicy.getStats;
export const getAuthPolicyAffected = authPolicy.getAffected;
export const getAuthPolicyPublicStatus = authPolicy.getPublicStatus;
