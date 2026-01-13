import { writable, derived } from 'svelte/store';
import { api } from '../api.js';
import { authStore } from './auth.svelte.js';

// Export isSystemAdmin as a standalone derived store for backward compatibility
export const isSystemAdmin = derived(authStore, ($authStore) => {
  return $authStore.currentUser?.is_system_admin === true;
});

function createPermissionStore() {
  const permissions = writable([]);
  const userPermissions = writable(new Set());
  const loading = writable(false);
  const error = writable(null);

  const canAccessAdmin = derived(
    [authStore, permissions, userPermissions],
    ([$authStore, $permissions, $userPermissions]) => {
      const user = $authStore.currentUser;
      if (!user) return false;

      // System admins can always access admin
      if (user.is_system_admin) return true;

      // Check if user has any admin-related permissions
      const adminPermissions = $permissions.filter(p =>
        p.permission_key === 'system.admin' ||
        p.permission_key.startsWith('admin.')
      );

      return adminPermissions.some(perm =>
        $userPermissions.has(perm.id)
      );
    }
  );

  const canAccessCustomers = derived(
    [authStore, permissions, userPermissions],
    ([$authStore, $permissions, $userPermissions]) => {
      const user = $authStore.currentUser;
      if (!user) return false;

      // System admins can always access
      if (user.is_system_admin) return true;

      // Check if user has customers.manage permission
      const hasPermission = $permissions.some(p =>
        p.permission_key === 'customers.manage' && $userPermissions.has(p.id)
      );

      return hasPermission;
    }
  );

  // Create a combined derived store for easy subscription
  const combined = derived(
    [permissions, userPermissions, loading, error, isSystemAdmin, canAccessAdmin, canAccessCustomers],
    ([$permissions, $userPermissions, $loading, $error, $isSystemAdmin, $canAccessAdmin, $canAccessCustomers]) => ({
      permissions: $permissions,
      userPermissions: $userPermissions,
      loading: $loading,
      error: $error,
      isSystemAdmin: $isSystemAdmin,
      canAccessAdmin: $canAccessAdmin,
      canAccessCustomers: $canAccessCustomers
    })
  );

  return {
    // Subscribe to combined state
    subscribe: combined.subscribe,

    // Convenience getters for backward compatibility with direct property access
    get isSystemAdmin() {
      let value;
      isSystemAdmin.subscribe(v => value = v)();
      return value;
    },

    get canAccessAdmin() {
      let value;
      canAccessAdmin.subscribe(v => value = v)();
      return value;
    },

    get canAccessCustomers() {
      let value;
      canAccessCustomers.subscribe(v => value = v)();
      return value;
    },

    // Load user permissions
    async loadUserPermissions(userId) {
      if (!userId) {
        userPermissions.set(new Set());
        loading.set(false);
        error.set(null);
        return;
      }

      loading.set(true);
      error.set(null);

      try {
        const response = await api.permissions.getUserPermissions(userId);
        const globalPermissionIds = new Set(
          (response.global_permissions || []).map(p => p.permission_id)
        );

        userPermissions.set(globalPermissionIds);
        loading.set(false);
        error.set(null);
      } catch (err) {
        console.warn('Failed to load user permissions for user', userId, ':', err);
        // Don't treat permission loading failures as critical errors
        // Clear permissions and continue to avoid blocking the UI
        userPermissions.set(new Set());
        loading.set(false);
        error.set(null); // Set to null to avoid error states blocking UI
      }
    },

    // Load all permissions (for admin only)
    async loadAllPermissions(user) {
      loading.set(true);

      // Only load all permissions if user is admin
      if (!user || user.role !== 'admin') {
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
      userPermissions.subscribe(perms => has = perms.has(permissionId))();
      return has;
    },

    // Check if user has a specific permission by key
    hasPermissionKey(permissionKey) {
      const user = authStore.currentUser;
      if (!user) return false;

      // System admins have all permissions
      if (user.is_system_admin) return true;

      // Find permission by key and check if user has it
      let permission;
      permissions.subscribe(perms => {
        permission = perms.find(p => p.permission_key === permissionKey);
      })();

      if (!permission) return false;

      let has = false;
      userPermissions.subscribe(perms => has = perms.has(permission.id))();
      return has;
    }
  };
}

export const permissionStore = createPermissionStore();
