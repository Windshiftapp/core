// Workspace Permission Store - manages workspace-scoped permissions
// Uses Svelte 5 runes for reactive state management
import { authStore } from './auth.svelte.js';
import { clearPermissionProfiles, loadPermissionProfile } from './permissionProfile.js';

function normalizeWorkspaceId(workspaceId) {
  if (workspaceId === null || workspaceId === undefined || workspaceId === '') return workspaceId;
  const numericId = Number(workspaceId);
  return Number.isFinite(numericId) ? numericId : workspaceId;
}

class WorkspacePermissionStore {
  // Map<workspaceId, Set<permissionKey>>
  permissions = $state(new Map());
  loading = $state(false);
  error = $state(null);

  // Check if user is system admin (always has all permissions)
  get isSystemAdmin() {
    return authStore.currentUser?.is_system_admin === true;
  }

  // Load permissions for current user
  async loadPermissions(userId) {
    if (!userId) {
      this.permissions = new Map();
      return;
    }

    this.loading = true;
    this.error = null;

    try {
      const response = await loadPermissionProfile(userId);

      // The profile groups permission keys by workspace ID (compact WI-1444
      // encoding); fold it in one pass into Map<workspaceId, Set<key>>.
      const wsPerms = new Map();
      for (const [wsId, keys] of Object.entries(response.workspace_permissions || {})) {
        wsPerms.set(normalizeWorkspaceId(wsId), new Set(keys));
      }
      this.permissions = wsPerms;
    } catch (err) {
      console.warn('Failed to load workspace permissions:', err);
      this.permissions = new Map();
    } finally {
      this.loading = false;
    }
  }

  // Check if user has permission in a workspace
  hasPermission(workspaceId, permissionKey) {
    if (this.isSystemAdmin) return true;
    return this.permissions.get(normalizeWorkspaceId(workspaceId))?.has(permissionKey) ?? false;
  }

  // Item permissions
  canView(workspaceId) {
    return this.hasPermission(workspaceId, 'item.view');
  }

  canCreate(workspaceId) {
    return this.hasPermission(workspaceId, 'item.create');
  }

  canEdit(workspaceId) {
    return this.hasPermission(workspaceId, 'item.edit');
  }

  canDelete(workspaceId) {
    return this.hasPermission(workspaceId, 'item.delete');
  }

  canComment(workspaceId) {
    return this.hasPermission(workspaceId, 'item.comment');
  }

  canEditOthersComments(workspaceId) {
    return this.hasPermission(workspaceId, 'comment.edit_others');
  }

  // Test permissions
  canViewTests(workspaceId) {
    return this.hasPermission(workspaceId, 'test.view');
  }

  canManageTests(workspaceId) {
    return this.hasPermission(workspaceId, 'test.manage');
  }

  canExecuteTests(workspaceId) {
    return this.hasPermission(workspaceId, 'test.execute');
  }

  // Workspace admin permission
  canAdminWorkspace(workspaceId) {
    return this.hasPermission(workspaceId, 'workspace.admin');
  }

  // Page permissions. Create mirrors the server's canCreate fallback chain;
  // archive additionally requires per-page admin on the target page.
  canViewPages(workspaceId) {
    return this.hasPermission(workspaceId, 'page.view');
  }

  canCreatePages(workspaceId) {
    return (
      this.hasPermission(workspaceId, 'page.create') ||
      this.hasPermission(workspaceId, 'page.admin') ||
      this.hasPermission(workspaceId, 'workspace.admin')
    );
  }

  canDeletePages(workspaceId) {
    return this.hasPermission(workspaceId, 'page.delete');
  }

  // Action management permission
  canManageActions(workspaceId) {
    return this.hasPermission(workspaceId, 'action.manage');
  }

  // Get all permission keys for a workspace (useful for debugging)
  getWorkspacePermissions(workspaceId) {
    if (this.isSystemAdmin) return new Set(['*']); // System admin has all
    return this.permissions.get(normalizeWorkspaceId(workspaceId)) || new Set();
  }

  // Check if user has any permissions in a workspace
  hasAnyPermission(workspaceId) {
    if (this.isSystemAdmin) return true;
    const perms = this.permissions.get(normalizeWorkspaceId(workspaceId));
    return perms && perms.size > 0;
  }

  // Refresh from the server, discarding the cached permission profile. Needed
  // after the user gains a new grant (e.g. creating a workspace makes them its
  // administrator) — the profile cache would otherwise serve stale data until
  // the next full page load.
  async reload() {
    clearPermissionProfiles();
    await this.loadPermissions(authStore.currentUser?.id);
  }

  // Clear all permissions
  clear() {
    clearPermissionProfiles();
    this.permissions = new Map();
    this.loading = false;
    this.error = null;
  }
}

export const workspacePermissions = new WorkspacePermissionStore();
