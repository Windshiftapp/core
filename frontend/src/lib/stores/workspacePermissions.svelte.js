// Workspace Permission Store - manages workspace-scoped permissions
// Uses Svelte 5 runes for reactive state management
import { api } from '../api.js';
import { authStore } from './auth.svelte.js';

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
      const response = await api.permissions.getUserPermissions(userId);

      // Parse workspace permissions into Map<workspaceId, Set<permissionKey>>
      const wsPerms = new Map();
      for (const wp of response.workspace_permissions || []) {
        const wsId = wp.workspace_id;
        if (!wsPerms.has(wsId)) {
          wsPerms.set(wsId, new Set());
        }
        if (wp.permission?.permission_key) {
          wsPerms.get(wsId).add(wp.permission.permission_key);
        }
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
    return this.permissions.get(workspaceId)?.has(permissionKey) ?? false;
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

  // Get all permission keys for a workspace (useful for debugging)
  getWorkspacePermissions(workspaceId) {
    if (this.isSystemAdmin) return new Set(['*']); // System admin has all
    return this.permissions.get(workspaceId) || new Set();
  }

  // Check if user has any permissions in a workspace
  hasAnyPermission(workspaceId) {
    if (this.isSystemAdmin) return true;
    const perms = this.permissions.get(workspaceId);
    return perms && perms.size > 0;
  }

  // Clear all permissions
  clear() {
    this.permissions = new Map();
    this.loading = false;
    this.error = null;
  }
}

export const workspacePermissions = new WorkspacePermissionStore();
