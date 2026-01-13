import { fetchAPI } from './core.js';

export const notifications = {
  getAll: (params = {}) => {
    const queryParams = new URLSearchParams();
    if (params.limit) queryParams.append('limit', params.limit);
    if (params.offset) queryParams.append('offset', params.offset);
    const queryString = queryParams.toString();
    return fetchAPI(`/notifications${queryString ? '?' + queryString : ''}`);
  },
  create: (data) => fetchAPI('/notifications', {
    method: 'POST',
    body: JSON.stringify(data),
  }),
  markAsRead: (id) => fetchAPI(`/notifications/${id}/read`, {
    method: 'PATCH',
  }),
};

// Notification Settings API
export const notificationSettings = {
  // Get all notification settings
  getAll: () => fetchAPI('/notification-settings'),

  // Get a specific notification setting
  get: (id) => fetchAPI(`/notification-settings/${id}`),

  // Create a new notification setting
  create: (data) => fetchAPI('/notification-settings', {
    method: 'POST',
    body: JSON.stringify(data),
  }),

  // Update a notification setting
  update: (id, data) => fetchAPI(`/notification-settings/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  }),

  // Delete a notification setting
  delete: (id) => fetchAPI(`/notification-settings/${id}`, {
    method: 'DELETE',
  }),

  // Get available notification events
  getAvailableEvents: () => fetchAPI('/notification-settings/available-events'),
};

// Configuration Set Notification assignments
export const configurationSetNotifications = {
  // Get all notification settings for a configuration set
  getForConfigurationSet: (configSetId) => fetchAPI(`/configuration-sets/${configSetId}/notification-settings`),

  // Assign notification setting to configuration set
  assign: (configSetId, data) => fetchAPI(`/configuration-sets/${configSetId}/notification-settings`, {
    method: 'POST',
    body: JSON.stringify(data),
  }),

  // Remove notification setting from configuration set
  unassign: (configSetId, assignmentId) => fetchAPI(`/configuration-sets/${configSetId}/notification-settings/${assignmentId}`, {
    method: 'DELETE',
  }),

  // Get available notification settings for a configuration set (not yet assigned)
  getAvailable: (configSetId) => fetchAPI(`/configuration-sets/${configSetId}/available-notification-settings`),
};

// Named exports for backward compatibility
export const getNotificationSettings = notificationSettings.getAll;
export const getNotificationSetting = notificationSettings.get;
export const createNotificationSetting = notificationSettings.create;
export const updateNotificationSetting = notificationSettings.update;
export const deleteNotificationSetting = notificationSettings.delete;
export const getAvailableNotificationEvents = notificationSettings.getAvailableEvents;
