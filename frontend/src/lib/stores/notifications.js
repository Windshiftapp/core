import { get, writable } from 'svelte/store';
import { api } from '../api.js';
import { navigate } from '../router.js';
import { formatDateSimple } from '../utils/dateFormatter.js';
import { serverNow } from '../utils/serverClock.js';
import { activityStore } from './activityStore.svelte.js';
import { addToast } from './toasts.svelte.js';

// Notification store
export const notifications = writable([]);

// Notification types that are important enough to interrupt the user with a toast.
// Plain comments are intentionally excluded — the item view itself live-updates
// comments, and toasting every comment would be noisy.
const TOASTABLE_TYPES = new Set(['mention', 'assignment']);

const ACTIVE_POLL_MS = 30_000;
const IDLE_POLL_MS = 5 * 60_000;

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
    const now = +serverNow();
    const diff = now - +new Date(timestamp);
    const minutes = Math.floor(diff / (1000 * 60));
    const hours = Math.floor(diff / (1000 * 60 * 60));
    const days = Math.floor(diff / (1000 * 60 * 60 * 24));

    if (minutes < 1) return 'Just now';
    if (minutes < 60) return `${minutes}m ago`;
    if (hours < 24) return `${hours}h ago`;
    if (days < 7) return `${days}d ago`;
    return formatDateSimple(timestamp);
  },
};

// --- New-notification pub/sub ---
// Anyone can subscribe to be notified when a new unread notification arrives
// (e.g. the open item view uses this to pull in new comments instantly).
const _busSubscribers = new Set();
const _seenIds = new Set();
let _seeded = false;

/**
 * Subscribe to freshly-arrived unread notifications.
 * @param {(n: any) => void} fn
 * @returns {() => void} unsubscribe
 */
export function subscribeToNewNotifications(fn) {
  _busSubscribers.add(fn);
  return () => _busSubscribers.delete(fn);
}

function _emitNew(notification) {
  for (const fn of _busSubscribers) {
    try {
      fn(notification);
    } catch (err) {
      console.warn('newNotification subscriber threw:', err);
    }
  }
}

function _toastFor(n) {
  addToast({
    title: n.title || '',
    message: n.message || '',
    variant: 'info',
    duration: 6000,
    clickable: Boolean(n.actionUrl),
    onClick: n.actionUrl ? () => navigate(n.actionUrl) : null,
  });
}

function _dispatchNew(items) {
  for (const n of items) {
    if (n.read || _seenIds.has(n.id)) continue;
    _seenIds.add(n.id);
    _emitNew(n);
    if (TOASTABLE_TYPES.has(n.type)) _toastFor(n);
  }
}

// --- Global poller ---
let _pollerStarted = false;
let _pollTimer = null;

function _scheduleNextPoll() {
  clearTimeout(_pollTimer);
  const delay = activityStore.isIdle ? IDLE_POLL_MS : ACTIVE_POLL_MS;
  _pollTimer = setTimeout(_tick, delay);
}

async function _tick() {
  try {
    await loadNotifications();
    _dispatchNew(get(notifications));
  } catch (err) {
    console.warn('notification poller: tick failed', err);
  } finally {
    _scheduleNextPoll();
  }
}

/**
 * Start the shared notification poller. Safe to call multiple times; only
 * the first call takes effect. Seeds lastSeen from the initial load so the
 * first tick doesn't toast the entire inbox.
 */
export function startNotificationPoller() {
  if (_pollerStarted) return;
  _pollerStarted = true;

  loadNotifications().then(() => {
    if (!_seeded) {
      for (const n of get(notifications)) _seenIds.add(n.id);
      _seeded = true;
    }
    _scheduleNextPoll();
  });
}

// --- Desktop notification bridge (Tauri only) ---
// Rides on the shared poller — no separate interval.
if (typeof window !== 'undefined' && /** @type {any} */ (window).__TAURI__?.core) {
  async function _sendDesktopNotification(title, body) {
    try {
      const invoke = /** @type {any} */ (window).__TAURI__.core.invoke;
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

  subscribeToNewNotifications((n) => {
    _sendDesktopNotification(n.title, n.message || '');
  });
}
