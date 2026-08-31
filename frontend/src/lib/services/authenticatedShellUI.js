import { get } from 'svelte/store';
import { api } from '../api.js';
import { aiStore } from '../stores/aiStore.svelte.js';
import { attachmentStatus } from '../stores/attachmentStatus.svelte.js';
import { capabilitiesStore } from '../stores/capabilities.svelte.js';
import { logbookStore } from '../stores/logbook.svelte.js';
import { moduleSettings } from '../stores/moduleSettings.js';
import { permissionStore } from '../stores/permissions.svelte.js';
import { themeStore } from '../stores/theme.svelte.js';
import { workItemStalenessSettings } from '../stores/workItemStalenessSettings.svelte.js';
import { workspaceDataStore } from '../stores/workspaceDataStore.svelte.js';
import { currentWorkspace, workspacesStore } from '../stores/workspaces.svelte.js';

let refreshGeneration = 0;
let shellUILoadGeneration = 0;
let shellUILoadAudience = null;
let shellUILoadPromise = null;
let shellUISettledAudience = null;
let shellUISettledResult = false;

function shellUIAudience(userId) {
  return `user:${userId ?? 'authenticated'}`;
}

export function hydrateAuthenticatedShellUI(bootstrap) {
  if (!bootstrap) return false;

  moduleSettings.hydrate(bootstrap.module_settings);
  attachmentStatus.hydrate(bootstrap.attachment_status);
  aiStore.hydrate(bootstrap.ai);
  capabilitiesStore.hydrate(bootstrap.features);
  logbookStore.hydrateAvailability(bootstrap.features?.logbook_available);
  workItemStalenessSettings.hydrate(bootstrap.work_item_staleness);
  permissionStore.setLogbookAvailable(bootstrap.features?.logbook_available === true);
  permissionStore.setHasAssetSets(bootstrap.has_asset_sets === true);
  permissionStore.setHasActivePortals(bootstrap.has_active_portals === true);
  permissionStore.setManagesChannels(bootstrap.manages_channels === true);
  return true;
}

/**
 * Hydrate the navigation-facing shell state once per authenticated audience.
 * App.svelte waits for this promise before mounting the desktop shell so
 * capability-gated navigation entries cannot appear after the sidebar and
 * shift the action group below them.
 */
export function loadAuthenticatedShellUI(userId, { force = false } = {}) {
  const audience = shellUIAudience(userId);

  if (!force && shellUISettledAudience === audience) {
    return Promise.resolve(shellUISettledResult);
  }
  if (!force && shellUILoadAudience === audience && shellUILoadPromise) {
    return shellUILoadPromise;
  }

  const generation = ++shellUILoadGeneration;
  shellUILoadAudience = audience;

  const request = api.shellBootstrap
    .get()
    .then((bootstrap) => {
      if (generation !== shellUILoadGeneration) return false;
      const hydrated = hydrateAuthenticatedShellUI(bootstrap);
      shellUISettledAudience = audience;
      shellUISettledResult = hydrated;
      return hydrated;
    })
    .catch((error) => {
      if (generation !== shellUILoadGeneration) return false;
      shellUISettledAudience = audience;
      shellUISettledResult = false;
      capabilitiesStore.failHydration();
      console.warn('Failed to load shell capabilities:', error);
      return false;
    })
    .finally(() => {
      if (generation !== shellUILoadGeneration) return;
      shellUILoadAudience = null;
      shellUILoadPromise = null;
    });

  shellUILoadPromise = request;
  return request;
}

/** Clear request identity when the authenticated audience changes. */
export function resetAuthenticatedShellUILoad() {
  shellUILoadGeneration += 1;
  shellUILoadAudience = null;
  shellUILoadPromise = null;
  shellUISettledAudience = null;
  shellUISettledResult = false;
}

/**
 * Keep the shell workspace aligned with the routed workspace while the shared
 * workspace snapshot loads. The previous workspace is cleared synchronously
 * so navigation and command consumers cannot act on stale context.
 */
export async function hydrateCurrentWorkspaceFromSharedData(workspaceId) {
  const expectedId = Number.parseInt(String(workspaceId), 10);
  const activeWorkspace = get(currentWorkspace);

  if (activeWorkspace && Number.parseInt(String(activeWorkspace.id), 10) !== expectedId) {
    currentWorkspace.clear();
  }

  await workspaceDataStore.initialize(workspaceId);

  // A newer route already owns the store — let its own hydration finish.
  if (workspaceDataStore.workspaceId !== expectedId) return false;
  if (!workspaceDataStore.workspace) {
    currentWorkspace.clear();
    return false;
  }

  currentWorkspace.hydrate(workspaceDataStore.workspace);
  return true;
}

/**
 * Refresh every shared UI snapshot that an administration mutation can affect.
 * The latest request wins so rapid multi-request saves cannot restore stale UI.
 */
export async function refreshAuthenticatedShellUI() {
  const generation = ++refreshGeneration;
  const [shellResult, themeResult] = await Promise.allSettled([
    api.shellBootstrap.get(),
    api.themes.getActive(),
    workspacesStore.reload(),
    workspaceDataStore.refresh(),
  ]);

  if (generation !== refreshGeneration) return false;

  if (shellResult.status === 'fulfilled') {
    hydrateAuthenticatedShellUI(shellResult.value);
  } else {
    console.warn(
      'Failed to refresh the authenticated shell after an admin change:',
      shellResult.reason
    );
  }

  if (themeResult.status === 'fulfilled') {
    themeStore.setActiveTheme(themeResult.value);
  } else {
    console.warn('Failed to refresh the active theme after an admin change:', themeResult.reason);
  }

  return shellResult.status === 'fulfilled';
}
