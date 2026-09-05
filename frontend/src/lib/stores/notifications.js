import { get, writable } from 'svelte/store';
import { api } from '../api.js';
import { navigate } from '../router.js';
import { itemIdFromActionUrl } from '../utils/actionUrl.js';
import {
  canRunBackgroundSync,
  isExpectedBackgroundSyncError,
  onBackgroundSyncAvailable,
} from '../utils/backgroundSync.js';
import { formatDateSimple } from '../utils/dateFormatter.js';
import { isTauri } from '../utils/isTauri.js';
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
const NOTIFICATION_PAGE_SIZE = 100;

// Load notifications from API
let loadPromise = null;
let pollerGeneration = 0;

async function loadNotificationPages(generation) {
  const allNotifications = [];
  let offset = 0;

  while (true) {
    const page = await api.notifications.getAll({ limit: NOTIFICATION_PAGE_SIZE, offset });
    if (generation !== pollerGeneration) return [];
    if (!page || !Array.isArray(page)) {
      if (offset === 0) return [];
      throw new Error(`Invalid notification page at offset ${offset}`);
    }

    allNotifications.push(...page);
    if (page.length < NOTIFICATION_PAGE_SIZE) return allNotifications;
    offset += page.length;
  }
}

function loadNotifications() {
  if (loadPromise) return loadPromise;

  const generation = pollerGeneration;

  // Keep component tests and optional consumers resilient when they provide a
  // partial API mock. Importing this module never starts authenticated work;
  // startNotificationPoller owns the first load after auth is established.
  if (typeof api?.notifications?.getAll !== 'function') {
    notifications.set([]);
    return Promise.resolve([]);
  }

  const request = loadNotificationPages(generation)
    .then((data) => {
      if (generation !== pollerGeneration) return [];
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
      if (generation !== pollerGeneration) return [];
      if (!isExpectedBackgroundSyncError(error)) {
        console.error('Failed to load notifications:', error);
      }
      // Preserve the last successfully loaded list during transient failures.
      return get(notifications);
    })
    .finally(() => {
      if (loadPromise === request) loadPromise = null;
    });

  loadPromise = request;
  return loadPromise;
}

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

  // Delete every notification from the server-backed inbox.
  clearAll: async () => {
    try {
      await api.notifications.clearAll();
      // An older inbox request may have started before the delete. Let it
      // settle first so its response cannot restore deleted notifications.
      if (loadPromise) await loadPromise;
      notifications.set([]);
    } catch (error) {
      console.error('Failed to clear notifications:', error);
      throw error;
    }
  },

  // Mark all as read
  markAllAsRead: async () => {
    try {
      await api.notifications.markAllAsRead();
      notifications.update((items) =>
        items.map((item) => (item.read ? item : { ...item, read: true }))
      );
    } catch (error) {
      console.error('Failed to mark all notifications as read:', error);
    }
  },

  // Mark all as "seen" — i.e. the user has looked at the tray. Distinct from
  // read: this drops the new-since-last-glance cue but does NOT suppress the
  // email batch (the scheduler keys off read = false). Used by the tray's
  // auto-timer so passive viewing doesn't silently snooze the email digest.
  markAllAsSeen: async () => {
    try {
      await api.notifications.markAllAsSeen();
      const now = new Date().toISOString();
      notifications.update((items) =>
        items.map((item) => (item.seen_at ? item : { ...item, seen_at: now }))
      );
    } catch (error) {
      console.error('Failed to mark all notifications as seen:', error);
    }
  },

  // Mark every notification pointing at the given item as read. Called when an
  // item is viewed (desktop ItemDetail / mobile MobileItemDetail) so the tray
  // and PWA badge clear regardless of how the item was opened — not only when
  // it's launched from the notification list. The API does the authoritative
  // match on action_url; here we mirror it locally for an instant tray update.
  markItemAsRead: async (itemId) => {
    if (itemId == null) return;

    const itemIdString = String(itemId);
    try {
      await api.notifications.markItemAsRead(itemId);
      // If the initial inbox request was already in flight, let it settle before
      // applying the local patch so an older response cannot restore unread.
      if (loadPromise) await loadPromise;
      notifications.update((items) =>
        items.map((item) =>
          !item.read && itemIdFromActionUrl(item.actionUrl) === itemIdString
            ? { ...item, read: true }
            : item
        )
      );
    } catch (error) {
      console.error('Failed to mark item notifications as read:', error);
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
let _stopReconnectListener = null;

function _scheduleNextPoll() {
  if (!_pollerStarted) return;
  clearTimeout(_pollTimer);
  const delay = activityStore.isIdle ? IDLE_POLL_MS : ACTIVE_POLL_MS;
  _pollTimer = setTimeout(_tick, delay);
}

async function _tick() {
  if (!canRunBackgroundSync()) {
    _scheduleNextPoll();
    return;
  }

  const generation = pollerGeneration;
  try {
    await loadNotifications();
    if (!_pollerStarted || generation !== pollerGeneration) return;
    _dispatchNew(get(notifications));
  } catch (err) {
    console.warn('notification poller: tick failed', err);
  } finally {
    if (_pollerStarted && generation === pollerGeneration) _scheduleNextPoll();
  }
}

function _loadInitialNotifications(generation) {
  loadNotifications().then(() => {
    if (!_pollerStarted || generation !== pollerGeneration) return;
    if (!_seeded) {
      for (const n of get(notifications)) _seenIds.add(n.id);
      _seeded = true;
    }
    _scheduleNextPoll();
  });
}

function _resumeNotificationPolling() {
  if (!_pollerStarted) return;
  clearTimeout(_pollTimer);
  _pollTimer = null;

  if (!canRunBackgroundSync()) {
    _scheduleNextPoll();
    return;
  }

  if (!_seeded) {
    _loadInitialNotifications(pollerGeneration);
    return;
  }
  void _tick();
}

/**
 * Start the shared notification poller. Safe to call multiple times; only
 * the first call takes effect. Seeds lastSeen from the initial load so the
 * first tick doesn't toast the entire inbox.
 */
export function startNotificationPoller() {
  if (_pollerStarted) return;
  _pollerStarted = true;
  const generation = ++pollerGeneration;

  _stopReconnectListener = onBackgroundSyncAvailable(() => {
    _resumeNotificationPolling();
  });

  if (!canRunBackgroundSync()) {
    _scheduleNextPoll();
    return;
  }
  _loadInitialNotifications(generation);
}

/** Stop account-scoped polling and discard every value from the old session. */
export function stopNotificationPoller() {
  _pollerStarted = false;
  pollerGeneration += 1;
  clearTimeout(_pollTimer);
  _pollTimer = null;
  _stopReconnectListener?.();
  _stopReconnectListener = null;
  loadPromise = null;
  _seeded = false;
  _seenIds.clear();
  notifications.set([]);
}

// --- Desktop notification bridge (Tauri only) ---
// Rides on the shared poller — no separate interval.
if (isTauri()) {
  async function _sendDesktopNotification(title, body) {
    try {
      const { invoke } = await import('@tauri-apps/api/core');
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
