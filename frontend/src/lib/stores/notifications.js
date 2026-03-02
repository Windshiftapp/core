import { writable } from 'svelte/store';
import { api } from '../api.js';
import { serverNow } from '../utils/serverClock.js';

// Notification store
export const notifications = writable([]);

// Load notifications from API
let loadPromise = null;
export function loadNotifications() {
  if (loadPromise) return loadPromise;

  loadPromise = api.notifications
    .getAll()
    .then((data) => {
      // Handle null response (no notifications)
      if (!data || !Array.isArray(data)) {
        notifications.set([]);
        return [];
      }

      // Convert timestamp strings to Date objects
      const processedNotifications = data.map((notification) => ({
        ...notification,
        timestamp: new Date(notification.timestamp),
        actionUrl: notification.action_url, // Convert snake_case to camelCase
      }));
      notifications.set(processedNotifications);
      return processedNotifications;
    })
    .catch((error) => {
      console.error('Failed to load notifications:', error);
      // Fall back to empty array on error
      notifications.set([]);
      return [];
    })
    .finally(() => {
      loadPromise = null; // Reset promise
    });

  return loadPromise;
}

// Initialize notifications
loadNotifications();

// Helper functions
export const notificationActions = {
  // Mark notification as read
  markAsRead: async (id) => {
    try {
      await api.notifications.markAsRead(id);
      notifications.update((items) =>
        items.map((item) => (item.id === id ? { ...item, read: true } : item))
      );
    } catch (error) {
      console.error('Failed to mark notification as read:', error);
    }
  },

  // Dismiss notification (remove from list - local only for now)
  dismiss: (id) => {
    notifications.update((items) => items.filter((item) => item.id !== id));
  },

  // Mark all as read
  markAllAsRead: async () => {
    try {
      // Get current notifications to mark them all as read
      let currentNotifications = [];
      notifications.subscribe((items) => {
        currentNotifications = items;
      })();

      // Mark each unread notification as read
      const unreadNotifications = currentNotifications.filter((item) => !item.read);
      await Promise.all(unreadNotifications.map((item) => api.notifications.markAsRead(item.id)));

      // Update local state
      notifications.update((items) => items.map((item) => ({ ...item, read: true })));
    } catch (error) {
      console.error('Failed to mark all notifications as read:', error);
    }
  },

  // Add new notification
  add: async (notification) => {
    try {
      const newNotification = {
        timestamp: new Date(),
        read: false,
        ...notification,
      };

      const createdNotification = await api.notifications.create(newNotification);
      // Convert response to match our format
      const processedNotification = {
        ...createdNotification,
        timestamp: new Date(createdNotification.timestamp),
        actionUrl: createdNotification.action_url,
      };

      notifications.update((items) => [processedNotification, ...items]);
      return processedNotification;
    } catch (error) {
      console.error('Failed to create notification:', error);
      throw error;
    }
  },

  // Refresh notifications from server
  refresh: () => {
    return loadNotifications();
  },

  // Get unread count
  getUnreadCount: (items) => {
    return items.filter((item) => !item.read).length;
  },

  // Format timestamp for display
  formatTimestamp: (timestamp) => {
    const now = serverNow();
    const diff = now - timestamp;
    const minutes = Math.floor(diff / (1000 * 60));
    const hours = Math.floor(diff / (1000 * 60 * 60));
    const days = Math.floor(diff / (1000 * 60 * 60 * 24));

    if (minutes < 1) return 'Just now';
    if (minutes < 60) return `${minutes}m ago`;
    if (hours < 24) return `${hours}h ago`;
    if (days < 7) return `${days}d ago`;
    return timestamp.toLocaleDateString();
  },
};

// --- Desktop notification bridge (Tauri only) ---
if (typeof window !== 'undefined' && window.__TAURI__?.core) {
  const _seenIds = new Set();

  function _seedAndStartPoll() {
    import('svelte/store').then(({ get }) => {
      // Seed with already-loaded notifications
      for (const n of get(notifications)) _seenIds.add(n.id);

      // Poll every 30 seconds
      setInterval(async () => {
        try {
          await notificationActions.refresh();
          for (const n of get(notifications)) {
            if (!n.read && !_seenIds.has(n.id)) {
              _sendDesktopNotification(n.title, n.message || '');
            }
            _seenIds.add(n.id);
          }
        } catch (e) {
          console.warn('[desktop-notifications] poll failed:', e);
        }
      }, 30_000);
    });
  }

  async function _sendDesktopNotification(title, body) {
    try {
      const invoke = window.__TAURI__.core.invoke;
      let granted = await invoke('plugin:notification|is_permission_granted');
      if (!granted) {
        const perm = await invoke('plugin:notification|request_permission');
        granted = perm === 'granted';
      }
      if (granted) {
        await invoke('plugin:notification|notify', { title, body });
      }
    } catch (e) {
      console.warn('[desktop-notifications] send failed:', e);
    }
  }

  // Start after initial load completes
  loadNotifications().then(() => _seedAndStartPoll());
}
