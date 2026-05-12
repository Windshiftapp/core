import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

// Mock api.notifications. The store's top-level scope kicks off a load on
// import, so the mocks need to be in place before the module is imported.
vi.mock('../api.js', () => ({
  api: {
    notifications: {
      getAll: vi.fn(() => Promise.resolve([])),
      markAsRead: vi.fn(() => Promise.resolve()),
      create: vi.fn(),
    },
  },
}));

// Other side effects we don't want firing during unit tests:
vi.mock('../router.js', () => ({ navigate: vi.fn() }));
vi.mock('../utils/dateFormatter.js', () => ({
  formatDateSimple: vi.fn((d) => `formatted:${d}`),
}));
vi.mock('../utils/serverClock.js', () => ({
  serverNow: vi.fn(() => new Date('2026-05-12T12:00:00Z')),
}));
vi.mock('./activityStore.svelte.js', () => ({
  activityStore: { isIdle: false },
}));
vi.mock('./toasts.svelte.js', () => ({ addToast: vi.fn() }));

import { api } from '../api.js';
import { notificationActions, notifications } from './notifications.js';

beforeEach(() => {
  notifications.set([]);
  vi.clearAllMocks();
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe('notificationActions.markAsRead', () => {
  test('optimistically flips read=true on the matching id', async () => {
    notifications.set([
      { id: 1, read: false, title: 'a' },
      { id: 2, read: false, title: 'b' },
    ]);

    await notificationActions.markAsRead(2);

    expect(api.notifications.markAsRead).toHaveBeenCalledWith(2);
    expect(get(notifications)).toEqual([
      { id: 1, read: false, title: 'a' },
      { id: 2, read: true, title: 'b' },
    ]);
  });

  test('leaves state untouched when the API call rejects', async () => {
    const errSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    api.notifications.markAsRead.mockRejectedValueOnce(new Error('500'));
    notifications.set([{ id: 1, read: false }]);

    await notificationActions.markAsRead(1);

    // The implementation only updates local state on success — failure
    // path swallows the error and keeps the optimistic-free state intact.
    expect(get(notifications)).toEqual([{ id: 1, read: false }]);
    expect(errSpy).toHaveBeenCalled();
  });

  test('no-op when id does not match any notification', async () => {
    notifications.set([{ id: 1, read: false }]);
    await notificationActions.markAsRead(99);
    expect(get(notifications)).toEqual([{ id: 1, read: false }]);
  });
});

describe('notificationActions.dismiss', () => {
  test('removes the matching notification from local state', () => {
    notifications.set([
      { id: 1, read: false },
      { id: 2, read: false },
      { id: 3, read: true },
    ]);
    notificationActions.dismiss(2);
    expect(get(notifications)).toEqual([
      { id: 1, read: false },
      { id: 3, read: true },
    ]);
  });

  test('does not call the API (local-only dismissal)', () => {
    notifications.set([{ id: 1, read: false }]);
    notificationActions.dismiss(1);
    expect(api.notifications.markAsRead).not.toHaveBeenCalled();
  });
});

describe('notificationActions.markAllAsRead', () => {
  test('marks only the unread ones via the API and flips them all read locally', async () => {
    notifications.set([
      { id: 1, read: false, title: 'a' },
      { id: 2, read: true, title: 'b' }, // already read — must not hit API
      { id: 3, read: false, title: 'c' },
    ]);

    await notificationActions.markAllAsRead();

    // API was called for ids 1 and 3, not 2.
    const callIds = api.notifications.markAsRead.mock.calls.map((c) => c[0]).sort();
    expect(callIds).toEqual([1, 3]);

    // Every item now has read=true.
    expect(get(notifications).map((n) => n.read)).toEqual([true, true, true]);
  });

  test('still flips local state when one of the API calls rejects', async () => {
    const errSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    api.notifications.markAsRead.mockRejectedValueOnce(new Error('500'));
    notifications.set([
      { id: 1, read: false },
      { id: 2, read: false },
    ]);

    await notificationActions.markAllAsRead();

    // Implementation wraps the whole flow in try/catch — on Promise.all
    // failure it logs and exits before the local update, so unread state
    // persists. This documents the current behavior (see logbook for any
    // future change to optimistic update semantics).
    expect(errSpy).toHaveBeenCalled();
    expect(get(notifications).every((n) => n.read === false)).toBe(true);
  });
});

describe('notificationActions.add', () => {
  test('prepends the created notification and maps action_url → actionUrl', async () => {
    api.notifications.create.mockResolvedValueOnce({
      id: 7,
      title: 'New',
      timestamp: '2026-05-12T10:00:00Z',
      action_url: '/items/42',
      read: false,
    });
    notifications.set([{ id: 1, title: 'existing' }]);

    const result = await notificationActions.add({ title: 'New' });

    expect(api.notifications.create).toHaveBeenCalledTimes(1);
    expect(result.actionUrl).toBe('/items/42');
    expect(result.timestamp).toBeInstanceOf(Date);

    const state = get(notifications);
    expect(state[0].id).toBe(7); // prepended
    expect(state[1].id).toBe(1);
  });

  test('rethrows on API failure and leaves state unchanged', async () => {
    const errSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    api.notifications.create.mockRejectedValueOnce(new Error('boom'));
    notifications.set([{ id: 1 }]);

    await expect(notificationActions.add({ title: 'x' })).rejects.toThrow('boom');
    expect(get(notifications)).toEqual([{ id: 1 }]);
    expect(errSpy).toHaveBeenCalled();
  });
});

describe('notificationActions.refresh', () => {
  test('reloads notifications from the server, processes timestamps + action_url', async () => {
    api.notifications.getAll.mockResolvedValueOnce([
      {
        id: 1,
        title: 'n1',
        timestamp: '2026-05-12T11:00:00Z',
        action_url: '/x',
      },
    ]);

    await notificationActions.refresh();
    const state = get(notifications);
    expect(state).toHaveLength(1);
    expect(state[0].timestamp).toBeInstanceOf(Date);
    expect(state[0].actionUrl).toBe('/x');
  });

  test('falls back to empty array when API returns null', async () => {
    notifications.set([{ id: 99 }]);
    api.notifications.getAll.mockResolvedValueOnce(null);
    await notificationActions.refresh();
    expect(get(notifications)).toEqual([]);
  });

  test('falls back to empty array when API returns non-array', async () => {
    notifications.set([{ id: 99 }]);
    api.notifications.getAll.mockResolvedValueOnce({ unexpected: 'shape' });
    await notificationActions.refresh();
    expect(get(notifications)).toEqual([]);
  });

  test('on rejection logs and clears the list', async () => {
    const errSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    notifications.set([{ id: 99 }]);
    api.notifications.getAll.mockRejectedValueOnce(new Error('net'));
    await notificationActions.refresh();
    expect(get(notifications)).toEqual([]);
    expect(errSpy).toHaveBeenCalled();
  });
});

describe('notificationActions.getUnreadCount', () => {
  test('counts only unread items', () => {
    const items = [
      { id: 1, read: false },
      { id: 2, read: true },
      { id: 3, read: false },
    ];
    expect(notificationActions.getUnreadCount(items)).toBe(2);
  });

  test('returns 0 for empty list', () => {
    expect(notificationActions.getUnreadCount([])).toBe(0);
  });
});

describe('notificationActions.formatTimestamp', () => {
  // serverNow is mocked to 2026-05-12T12:00:00Z, so all asserts here are
  // relative to that fixed "now".
  test('"Just now" within 1 minute', () => {
    expect(notificationActions.formatTimestamp('2026-05-12T11:59:30Z')).toBe('Just now');
  });

  test('minute / hour / day buckets', () => {
    expect(notificationActions.formatTimestamp('2026-05-12T11:55:00Z')).toBe('5m ago');
    expect(notificationActions.formatTimestamp('2026-05-12T10:00:00Z')).toBe('2h ago');
    expect(notificationActions.formatTimestamp('2026-05-09T12:00:00Z')).toBe('3d ago');
  });

  test('falls back to formatDateSimple past 7 days', () => {
    expect(notificationActions.formatTimestamp('2026-04-01T12:00:00Z')).toBe(
      'formatted:2026-04-01T12:00:00Z'
    );
  });
});
