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
  clearAll: () =>
    fetchAPI('/notifications', {
      method: 'DELETE',
    }),
  dismiss: (id) =>
    fetchAPI(`/notifications/${id}`, {
      method: 'DELETE',
    }),
  markAsRead: (id) =>
    fetchAPI(`/notifications/${id}/read`, {
      method: 'PATCH',
    }),
  markAllAsRead: () =>
    fetchAPI('/notifications/read-all', {
      method: 'PATCH',
    }),
  // Mark every unseen notification as seen. Distinct from markAsRead: the
  // backend keeps `read = false` so the email batcher still fires; only the
  // tray-side "new since you last looked" cue should drop.
  markAllAsSeen: () =>
    fetchAPI('/notifications/seen-all', {
      method: 'PATCH',
    }),

  // Mark every unread notification pointing at the given item as read.
  // Viewing an item should clear its notifications regardless of how it was
  // opened (mobile PWA deep link, desktop tray, plain navigation) — not only
  // when the item is opened from the notification list.
  markItemAsRead: (itemId) =>
    fetchAPI('/notifications/mark-item-read', {
      method: 'POST',
      body: JSON.stringify({ item_id: itemId }),
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
