import { derived, writable } from 'svelte/store';
import { api } from '../api.js';
import { authStore } from './auth.svelte.js';

// Export isSystemAdmin as a standalone derived store for backward compatibility
export const isSystemAdmin = derived(authStore, ($authStore) => {
  return $authStore.currentUser?.is_system_admin === true;
});

function createPermissionStore() {
  const permissions = writable([]);
  const userPermissions = writable(new Set());
  const userPermissionKeys = writable(new Set());
  const loading = writable(false);
  const error = writable(null);
  const hasAssetSets = writable(false);
  const hasActivePortals = writable(false);
  const logbookAvailable = writable(false);

  const canAccessAdmin = derived([authStore], ([$authStore]) => {
    return $authStore.currentUser?.is_system_admin === true;
  });

  const canAccessCustomers = derived(
    [authStore, userPermissionKeys, hasActivePortals],
    ([$authStore, $userPermissionKeys, $hasActivePortals]) => {
      const user = $authStore.currentUser;
      if (!user) return false;

      // Hide if no active portals
      if (!$hasActivePortals) return false;

      // System admins can always access
      if (user.is_system_admin) return true;

      // Check if user has customers.manage permission
      return $userPermissionKeys.has('customers.manage');
    }
  );

  const canAccessAssets = derived([authStore, hasAssetSets], ([$authStore, $hasAssetSets]) => {
    const user = $authStore.currentUser;
    if (!user) return false;
    return $hasAssetSets;
  });

  const canAccessPortalHub = derived(
    [authStore, hasActivePortals],
    ([$authStore, $hasActivePortals]) => {
      const user = $authStore.currentUser;
      if (!user) return false;
      return $hasActivePortals;
    }
  );

  const canAccessLogbook = derived(
    [authStore, logbookAvailable],
    ([$authStore, $logbookAvailable]) => {
      const user = $authStore.currentUser;
      if (!user) return false;
      return $logbookAvailable;
    }
  );

  const canManageAssets = derived([authStore, userPermissionKeys], ([$authStore, $keys]) => {
    if (!$authStore.currentUser) return false;
    if ($authStore.currentUser.is_system_admin) return true;
    return $keys.has('asset.manage');
  });

  // Create a combined derived store for easy subscription
  const combined = derived(
    [
      permissions,
      userPermissions,
      userPermissionKeys,
      loading,
      error,
      isSystemAdmin,
      canAccessAdmin,
      canAccessCustomers,
      canAccessAssets,
      canAccessPortalHub,
      canAccessLogbook,
      canManageAssets,
    ],
    ([
      $permissions,
      $userPermissions,
      $userPermissionKeys,
      $loading,
      $error,
      $isSystemAdmin,
      $canAccessAdmin,
      $canAccessCustomers,
      $canAccessAssets,
      $canAccessPortalHub,
      $canAccessLogbook,
      $canManageAssets,
    ]) => ({
      permissions: $permissions,
      userPermissions: $userPermissions,
      userPermissionKeys: $userPermissionKeys,
      loading: $loading,
      error: $error,
      isSystemAdmin: $isSystemAdmin,
      canAccessAdmin: $canAccessAdmin,
      canAccessCustomers: $canAccessCustomers,
      canAccessAssets: $canAccessAssets,
      canAccessPortalHub: $canAccessPortalHub,
      canAccessLogbook: $canAccessLogbook,
      canManageAssets: $canManageAssets,
    })
  );

  return {
    // Subscribe to combined state
    subscribe: combined.subscribe,

    // Convenience getters for backward compatibility with direct property access
    get isSystemAdmin() {
      let value;
      isSystemAdmin.subscribe((v) => (value = v))();
      return value;
    },

    get canAccessAdmin() {
      let value;
      canAccessAdmin.subscribe((v) => (value = v))();
      return value;
    },

    get canAccessCustomers() {
      let value;
      canAccessCustomers.subscribe((v) => (value = v))();
      return value;
    },

    get canAccessAssets() {
      let value;
      canAccessAssets.subscribe((v) => (value = v))();
      return value;
    },

    get canAccessPortalHub() {
      let value;
      canAccessPortalHub.subscribe((v) => (value = v))();
      return value;
    },

    get canAccessLogbook() {
      let value;
      canAccessLogbook.subscribe((v) => (value = v))();
      return value;
    },

    get canManageAssets() {
      let value;
      canManageAssets.subscribe((v) => (value = v))();
      return value;
    },

    // Set whether asset sets exist
    setHasAssetSets(value) {
      hasAssetSets.set(value);
    },

    // Set whether active portals exist
    setHasActivePortals(value) {
      hasActivePortals.set(value);
    },

    // Set whether logbook service is available
    setLogbookAvailable(value) {
      logbookAvailable.set(value);
    },

    // Load user permissions
    async loadUserPermissions(userId) {
      if (!userId) {
        userPermissions.set(new Set());
        userPermissionKeys.set(new Set());
        loading.set(false);
        error.set(null);
        return;
      }

      loading.set(true);
      error.set(null);

      try {
        const response = await api.permissions.getUserPermissions(userId);
        const globalPerms = response.global_permissions || [];
        const globalPermissionIds = new Set(globalPerms.map((p) => p.permission_id));
        const globalPermKeys = new Set(
          globalPerms
            .filter((p) => p.permission?.permission_key)
            .map((p) => p.permission.permission_key)
        );

        userPermissions.set(globalPermissionIds);
        userPermissionKeys.set(globalPermKeys);
        loading.set(false);
        error.set(null);
      } catch (err) {
        console.warn('Failed to load user permissions for user', userId, ':', err);
        // Don't treat permission loading failures as critical errors
        // Clear permissions and continue to avoid blocking the UI
        userPermissions.set(new Set());
        userPermissionKeys.set(new Set());
        loading.set(false);
        error.set(null); // Set to null to avoid error states blocking UI
      }
    },

    // Load all permissions (for admin only)
    async loadAllPermissions(user) {
      loading.set(true);

      // Only load all permissions if user is system admin
      if (!user?.is_system_admin) {
        permissions.set([]);
        loading.set(false);
        error.set(null);
        return;
      }

      try {
        const allPermissions = await api.permissions.getAll();
        permissions.set(allPermissions);
        loading.set(false);
        error.set(null);
      } catch (err) {
        console.warn('Failed to load all permissions:', err);
        permissions.set([]);
        loading.set(false);
        error.set(err.message);
      }
    },

    // Clear permissions
    clear() {
      permissions.set([]);
      userPermissions.set(new Set());
      userPermissionKeys.set(new Set());
      loading.set(false);
      error.set(null);
    },

    // Check if user has a specific permission by ID
    hasPermission(permissionId) {
      const user = authStore.currentUser;
      if (!user) return false;

      // System admins have all permissions
      if (user.is_system_admin) return true;

      let has = false;
      userPermissions.subscribe((perms) => (has = perms.has(permissionId)))();
      return has;
    },

    // Check if user has a specific permission by key
    hasPermissionKey(permissionKey) {
      const user = authStore.currentUser;
      if (!user) return false;

      // System admins have all permissions
      if (user.is_system_admin) return true;

      // Use userPermissionKeys set for the check
      let has = false;
      userPermissionKeys.subscribe((keys) => (has = keys.has(permissionKey)))();
      return has;
    },
  };
}

export const permissionStore = createPermissionStore();
