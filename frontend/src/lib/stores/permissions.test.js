import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

// Mock authStore with the same dual shape as the real one: it is both a
// readable store (so `derived([authStore], ...)` works) AND exposes a
// `currentUser` getter (so `permissionStore.hasPermission` can synchronously
// read it). Tests drive state via .set on this mock.
vi.mock('./auth.svelte.js', async () => {
  const { writable, get } = await import('svelte/store');
  const inner = writable({ currentUser: null });
  return {
    authStore: {
      subscribe: inner.subscribe,
      set: inner.set,
      update: inner.update,
      get currentUser() {
        return get(inner).currentUser;
      },
    },
  };
});

// Stub the api module — permissions store calls api.permissions.* during
// loadUserPermissions / loadAllPermissions; tests that exercise those paths
// override these mocks.
vi.mock('../api.js', () => ({
  api: {
    permissions: {
      getUserPermissions: vi.fn(),
      getAll: vi.fn(),
    },
  },
}));

import { api } from '../api.js';
import { authStore } from './auth.svelte.js';
import { isSystemAdmin, permissionStore } from './permissions.svelte.js';

beforeEach(() => {
  authStore.set({ currentUser: null });
  permissionStore.clear();
  permissionStore.setHasAssetSets(false);
  permissionStore.setHasActivePortals(false);
  permissionStore.setLogbookAvailable(false);
  vi.clearAllMocks();
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe('isSystemAdmin (standalone)', () => {
  test('false when no user', () => {
    authStore.set({ currentUser: null });
    expect(get(isSystemAdmin)).toBe(false);
  });

  test('false when user is not system admin', () => {
    authStore.set({ currentUser: { id: 1, is_system_admin: false } });
    expect(get(isSystemAdmin)).toBe(false);
  });

  test('true when user.is_system_admin === true', () => {
    authStore.set({ currentUser: { id: 1, is_system_admin: true } });
    expect(get(isSystemAdmin)).toBe(true);
  });

  test('explicitly false when is_system_admin is truthy-but-not-true', () => {
    // Defensive: the derived store uses strict === true, so a value of 1
    // or 'yes' must not grant access.
    authStore.set({ currentUser: { id: 1, is_system_admin: 1 } });
    expect(get(isSystemAdmin)).toBe(false);
  });
});

describe('canAccessAdmin', () => {
  test('requires system admin', () => {
    authStore.set({ currentUser: { id: 1, is_system_admin: false } });
    expect(get(permissionStore).canAccessAdmin).toBe(false);
    authStore.set({ currentUser: { id: 1, is_system_admin: true } });
    expect(get(permissionStore).canAccessAdmin).toBe(true);
  });

  test('false when not authenticated', () => {
    authStore.set({ currentUser: null });
    expect(get(permissionStore).canAccessAdmin).toBe(false);
  });
});

describe('canAccessCustomers', () => {
  test('false when no user', () => {
    permissionStore.setHasActivePortals(true);
    expect(get(permissionStore).canAccessCustomers).toBe(false);
  });

  test('false when no active portals (even for system admin)', () => {
    authStore.set({ currentUser: { id: 1, is_system_admin: true } });
    permissionStore.setHasActivePortals(false);
    expect(get(permissionStore).canAccessCustomers).toBe(false);
  });

  test('true for system admin when portals are active', () => {
    authStore.set({ currentUser: { id: 1, is_system_admin: true } });
    permissionStore.setHasActivePortals(true);
    expect(get(permissionStore).canAccessCustomers).toBe(true);
  });

  test('false for non-admin without customers.manage even with portals active', async () => {
    authStore.set({ currentUser: { id: 1, is_system_admin: false } });
    permissionStore.setHasActivePortals(true);
    // No permission keys loaded.
    expect(get(permissionStore).canAccessCustomers).toBe(false);
  });

  test('true for non-admin with customers.manage when portals are active', async () => {
    authStore.set({ currentUser: { id: 1, is_system_admin: false } });
    permissionStore.setHasActivePortals(true);
    api.permissions.getUserPermissions.mockResolvedValueOnce({
      global_permissions: [
        {
          permission_id: 7,
          permission: { permission_key: 'customers.manage' },
        },
      ],
    });
    await permissionStore.loadUserPermissions(1);
    expect(get(permissionStore).canAccessCustomers).toBe(true);
  });
});

describe('canAccessAssets', () => {
  test('false when user is missing', () => {
    permissionStore.setHasAssetSets(true);
    expect(get(permissionStore).canAccessAssets).toBe(false);
  });

  test('false when no asset sets exist', () => {
    authStore.set({ currentUser: { id: 1 } });
    permissionStore.setHasAssetSets(false);
    expect(get(permissionStore).canAccessAssets).toBe(false);
  });

  test('true when user and asset sets are present', () => {
    authStore.set({ currentUser: { id: 1 } });
    permissionStore.setHasAssetSets(true);
    expect(get(permissionStore).canAccessAssets).toBe(true);
  });
});

describe('canAccessPortalHub', () => {
  test('mirrors active-portals × authenticated', () => {
    authStore.set({ currentUser: null });
    permissionStore.setHasActivePortals(true);
    expect(get(permissionStore).canAccessPortalHub).toBe(false);

    authStore.set({ currentUser: { id: 1 } });
    permissionStore.setHasActivePortals(false);
    expect(get(permissionStore).canAccessPortalHub).toBe(false);

    permissionStore.setHasActivePortals(true);
    expect(get(permissionStore).canAccessPortalHub).toBe(true);
  });
});

describe('canAccessLogbook', () => {
  test('requires logbook availability + authenticated user', () => {
    authStore.set({ currentUser: { id: 1 } });
    expect(get(permissionStore).canAccessLogbook).toBe(false);

    permissionStore.setLogbookAvailable(true);
    expect(get(permissionStore).canAccessLogbook).toBe(true);

    authStore.set({ currentUser: null });
    expect(get(permissionStore).canAccessLogbook).toBe(false);
  });
});

describe('canManageAssets', () => {
  test('false without user', () => {
    expect(get(permissionStore).canManageAssets).toBe(false);
  });

  test('system admin bypasses key check', () => {
    authStore.set({ currentUser: { id: 1, is_system_admin: true } });
    expect(get(permissionStore).canManageAssets).toBe(true);
  });

  test('non-admin requires asset.manage key', async () => {
    authStore.set({ currentUser: { id: 1, is_system_admin: false } });
    expect(get(permissionStore).canManageAssets).toBe(false);

    api.permissions.getUserPermissions.mockResolvedValueOnce({
      global_permissions: [{ permission_id: 9, permission: { permission_key: 'asset.manage' } }],
    });
    await permissionStore.loadUserPermissions(1);
    expect(get(permissionStore).canManageAssets).toBe(true);
  });
});

describe('hasPermission / hasPermissionKey', () => {
  test('hasPermission returns false without a user', () => {
    expect(permissionStore.hasPermission(3)).toBe(false);
  });

  test('hasPermission grants everything to system admin', () => {
    authStore.set({ currentUser: { id: 1, is_system_admin: true } });
    expect(permissionStore.hasPermission(99)).toBe(true);
    expect(permissionStore.hasPermissionKey('any.permission')).toBe(true);
  });

  test('hasPermission checks the loaded set for non-admins', async () => {
    authStore.set({ currentUser: { id: 1, is_system_admin: false } });
    api.permissions.getUserPermissions.mockResolvedValueOnce({
      global_permissions: [{ permission_id: 42, permission: { permission_key: 'workspace.edit' } }],
    });
    await permissionStore.loadUserPermissions(1);

    expect(permissionStore.hasPermission(42)).toBe(true);
    expect(permissionStore.hasPermission(43)).toBe(false);
    expect(permissionStore.hasPermissionKey('workspace.edit')).toBe(true);
    expect(permissionStore.hasPermissionKey('workspace.delete')).toBe(false);
  });
});

describe('loadUserPermissions', () => {
  test('clears state when called with no userId', async () => {
    // Pre-seed something so we can confirm it's cleared.
    authStore.set({ currentUser: { id: 1, is_system_admin: false } });
    api.permissions.getUserPermissions.mockResolvedValueOnce({
      global_permissions: [{ permission_id: 1, permission: { permission_key: 'foo.bar' } }],
    });
    await permissionStore.loadUserPermissions(1);
    expect(permissionStore.hasPermissionKey('foo.bar')).toBe(true);

    await permissionStore.loadUserPermissions(null);
    expect(permissionStore.hasPermissionKey('foo.bar')).toBe(false);
    expect(get(permissionStore).loading).toBe(false);
  });

  test('swallows API errors but leaves the store in a usable state', async () => {
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});
    authStore.set({ currentUser: { id: 1, is_system_admin: false } });
    api.permissions.getUserPermissions.mockRejectedValueOnce(new Error('500'));

    await permissionStore.loadUserPermissions(1);

    // Error is non-fatal: state must be cleared, loading off, error null
    // (so the UI isn't blocked by an error banner over permission loading).
    expect(get(permissionStore).error).toBeNull();
    expect(get(permissionStore).loading).toBe(false);
    expect(get(permissionStore).userPermissionKeys.size).toBe(0);
    expect(warnSpy).toHaveBeenCalled();
  });

  test('skips permission entries without a permission_key', async () => {
    authStore.set({ currentUser: { id: 1, is_system_admin: false } });
    api.permissions.getUserPermissions.mockResolvedValueOnce({
      global_permissions: [
        { permission_id: 1, permission: { permission_key: 'a.b' } },
        { permission_id: 2, permission: null },
        { permission_id: 3 }, // no permission object at all
      ],
    });
    await permissionStore.loadUserPermissions(1);

    expect(get(permissionStore).userPermissionKeys).toEqual(new Set(['a.b']));
    expect(get(permissionStore).userPermissions).toEqual(new Set([1, 2, 3]));
  });
});

describe('loadAllPermissions', () => {
  test('no-op for non-admin user', async () => {
    await permissionStore.loadAllPermissions({ id: 1, is_system_admin: false });
    expect(api.permissions.getAll).not.toHaveBeenCalled();
    expect(get(permissionStore).permissions).toEqual([]);
  });

  test('loads when user is system admin', async () => {
    const all = [{ id: 1, permission_key: 'a' }];
    api.permissions.getAll.mockResolvedValueOnce(all);
    await permissionStore.loadAllPermissions({ id: 1, is_system_admin: true });
    expect(get(permissionStore).permissions).toEqual(all);
  });

  test('records error on failure', async () => {
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});
    api.permissions.getAll.mockRejectedValueOnce(new Error('boom'));
    await permissionStore.loadAllPermissions({ id: 1, is_system_admin: true });
    expect(get(permissionStore).permissions).toEqual([]);
    expect(get(permissionStore).error).toBe('boom');
    expect(warnSpy).toHaveBeenCalled();
  });
});
