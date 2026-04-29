import { fetchAPI } from './core.js';
import { createCrudClient } from './createCrudClient.js';
import { buildQueryString } from './utils.js';

export const notifications = {
  getAll: (params = {}) => {
    return fetchAPI(`/notifications${buildQueryString(params)}`);
  },
  create: (data) =>
    fetchAPI('/notifications', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  markAsRead: (id) =>
    fetchAPI(`/notifications/${id}/read`, {
      method: 'PATCH',
    }),
};

// Notification Settings API
export const notificationSettings = {
  ...createCrudClient('/notification-settings'),
  getAvailableEvents: () => fetchAPI('/notification-settings/available-events'),
};

// Configuration Set Notification assignments
export const configurationSetNotifications = {
  // Get all notification settings for a configuration set
  getForConfigurationSet: (configSetId) =>
    fetchAPI(`/configuration-sets/${configSetId}/notification-settings`),

  // Assign notification setting to configuration set
  assign: (configSetId, data) =>
    fetchAPI(`/configuration-sets/${configSetId}/notification-settings`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  // Remove notification setting from configuration set
  unassign: (configSetId, assignmentId) =>
    fetchAPI(`/configuration-sets/${configSetId}/notification-settings/${assignmentId}`, {
      method: 'DELETE',
    }),

  // Get available notification settings for a configuration set (not yet assigned)
  getAvailable: (configSetId) =>
    fetchAPI(`/configuration-sets/${configSetId}/available-notification-settings`),
};
