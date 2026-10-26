import { loadAuthenticatedShellUI } from '../services/authenticatedShellUI.js';
import { permissionStore } from '../stores/permissions.svelte.js';
import { ssoStore } from '../stores/sso.svelte.js';
import { workspacePermissions } from '../stores/workspacePermissions.svelte.js';
import { workspacesStore } from '../stores/workspaces.svelte.js';

/**
 * Split authenticated-shell work into the navigation critical path and the
 * background probes that cannot delay the first useful route render.
 *
 * The workspace directory is a deferred probe: the board and map warm it on
 * their own cached/single-flight request so its response never gates their
 * first paint. Personal-workspace loading stays on the board's own critical
 * path because personal item classification, opening and drag behavior depend
 * on it.
 */
export function buildAuthenticatedShellTasks({ user, userId, onEmailVerificationChange }) {
  const criticalTasks = [() => permissionStore.loadAllPermissions(user)];
  if (userId) {
    criticalTasks.push(
      () => permissionStore.loadUserPermissions(userId),
      () => workspacePermissions.loadPermissions(userId)
    );
  }

  const deferredTasks = [
    () => workspacesStore.load(),
    () => workspacesStore.loadPersonalWorkspace(),
    () => loadAuthenticatedShellUI(userId),
    async () => {
      try {
        const status = await ssoStore.getVerificationStatus();
        onEmailVerificationChange(Boolean(status.configured && !status.email_verified));
      } catch (error) {
        console.warn('Failed to check email verification status:', error);
      }
    },
  ];

  return { criticalTasks, deferredTasks };
}
