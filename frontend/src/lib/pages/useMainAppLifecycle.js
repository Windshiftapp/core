import { useEventListener } from 'runed';
import { onMount } from 'svelte';
import { ADMIN_UI_MUTATION_EVENT } from '../api/core.js';
import { desktopBridge } from '../desktop/bridge.svelte.js';
import {
  resetAuthenticatedShellState,
  runAuthenticatedShellBootstrap,
} from '../services/authenticatedShellBootstrap.js';
import { refreshAuthenticatedShellUI } from '../services/authenticatedShellUI.js';
import { activityStore } from '../stores/activityStore.svelte.js';
import { authStore } from '../stores/auth.svelte.js';
import { capabilitiesStore } from '../stores/capabilities.svelte.js';
import { homepageStore } from '../stores/homepageStore.svelte.js';
import { startNotificationPoller, stopNotificationPoller } from '../stores/notifications.js';
import { workspaceDataStore } from '../stores/workspaceDataStore.svelte.js';
import { workspacesStore } from '../stores/workspaces.svelte.js';
import { initDesktopFocusRefresh } from '../utils/desktopFocusRefresh.svelte.js';
import { buildAuthenticatedShellTasks } from './authenticatedShellTasks.js';

const DOUBLE_SPACE_THRESHOLD_MS = 300;
const ADMIN_UI_REFRESH_DEBOUNCE_MS = 75;

/**
 * Own the authenticated shell's process-level lifecycle and translate global
 * browser events into the small set of UI actions MainApp composes.
 */
export function useMainAppLifecycle({
  onEmailVerificationChange,
  onShowCommandPalette,
  onShowCreateModal,
}) {
  let lastSpaceTime = 0;
  let adminUIRefreshTimer = null;
  let adminUIRefreshPromise = null;
  let adminUIRefreshQueued = false;

  capabilitiesStore.beginHydration();

  async function runAdminUIRefresh() {
    if (adminUIRefreshPromise) {
      adminUIRefreshQueued = true;
      return adminUIRefreshPromise;
    }

    do {
      adminUIRefreshQueued = false;
      adminUIRefreshPromise = refreshAuthenticatedShellUI();
      try {
        await adminUIRefreshPromise;
      } finally {
        adminUIRefreshPromise = null;
      }
    } while (adminUIRefreshQueued);
  }

  function scheduleAdminUIRefresh() {
    if (adminUIRefreshTimer) clearTimeout(adminUIRefreshTimer);
    adminUIRefreshTimer = setTimeout(() => {
      adminUIRefreshTimer = null;
      void runAdminUIRefresh();
    }, ADMIN_UI_REFRESH_DEBOUNCE_MS);
  }

  onMount(() => {
    activityStore.init();
    initDesktopFocusRefresh();
    desktopBridge.init();
    startNotificationPoller();

    const user = authStore.currentUser;
    const userId = user?.id;
    const { criticalTasks, deferredTasks } = buildAuthenticatedShellTasks({
      user,
      userId,
      onEmailVerificationChange,
    });

    void runAuthenticatedShellBootstrap({
      userId,
      criticalTasks,
      deferredTasks,
      onMeasured: (metrics) => {
        window.dispatchEvent(
          new CustomEvent('windshift:auth-shell-bootstrap', { detail: metrics })
        );
      },
    });

    return () => {
      stopNotificationPoller();
      homepageStore.reset();
      resetAuthenticatedShellState();
    };
  });

  useEventListener(
    () => document,
    'keydown',
    (event) => {
      if (event.code !== 'Space') return;

      const target = /** @type {HTMLElement} */ (event.target);
      const isInInputField =
        target.tagName === 'INPUT' ||
        target.tagName === 'TEXTAREA' ||
        target.contentEditable === 'true' ||
        target.closest('[contenteditable="true"]');
      if (isInInputField) return;

      const now = Date.now();
      if (now - lastSpaceTime < DOUBLE_SPACE_THRESHOLD_MS) onShowCommandPalette();
      event.preventDefault();
      lastSpaceTime = now;
    }
  );

  onMount(() => {
    const handleShowCreateModal = (event) => onShowCreateModal(event.detail || {});
    const handleRefreshWorkspaces = (event) => {
      if (event.detail?.workspace) {
        workspacesStore.add(event.detail.workspace);
        return;
      }
      void workspacesStore.reload();
    };
    const handleRefreshWorkspaceData = () => void workspaceDataStore.refresh();
    const handleRefreshWorkItems = () => homepageStore.invalidateSnapshot();

    window.addEventListener('show-create-modal', handleShowCreateModal);
    window.addEventListener(ADMIN_UI_MUTATION_EVENT, scheduleAdminUIRefresh);
    window.addEventListener('refresh-workspaces', handleRefreshWorkspaces);
    window.addEventListener('refresh-workspace-data', handleRefreshWorkspaceData);
    window.addEventListener('refresh-work-items', handleRefreshWorkItems);

    return () => {
      window.removeEventListener('show-create-modal', handleShowCreateModal);
      window.removeEventListener(ADMIN_UI_MUTATION_EVENT, scheduleAdminUIRefresh);
      window.removeEventListener('refresh-workspaces', handleRefreshWorkspaces);
      window.removeEventListener('refresh-workspace-data', handleRefreshWorkspaceData);
      window.removeEventListener('refresh-work-items', handleRefreshWorkItems);
      if (adminUIRefreshTimer) clearTimeout(adminUIRefreshTimer);
    };
  });
}
