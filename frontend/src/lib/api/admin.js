import { fetchAPI, API_BASE } from './core.js';

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
