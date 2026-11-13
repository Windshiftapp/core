import { collectionEditorOptions } from '../stores/collectionEditorOptions.svelte.js';
import { collectionFieldLinks } from '../stores/collectionFieldLinks.svelte.js';
import { beginPermissionProfileGeneration } from '../stores/permissionProfile.js';
import { referenceDisplayCache } from '../stores/referenceDisplayCache.svelte.js';
import { timeLogStore } from '../stores/timeLogStore.svelte.js';

export const AUTHENTICATED_SHELL_BOOTSTRAP_BUDGET_MS = 2500;

function now() {
  return globalThis.performance?.now?.() ?? Date.now();
}

function startTasks(tasks) {
  return tasks.map((task) => Promise.resolve().then(task));
}

/** Clear every cache whose contents depend on the authenticated identity. */
export function resetAuthenticatedShellState() {
  collectionEditorOptions.reset();
  collectionFieldLinks.reset();
  referenceDisplayCache.reset();
  timeLogStore.reset();
}

/**
 * Start all authenticated-shell work together while exposing a short critical
 * path for navigation and permissions. Optional feature probes continue in the
 * background and cannot delay the first useful route render.
 */
export async function runAuthenticatedShellBootstrap({
  userId,
  criticalTasks = [],
  deferredTasks = [],
  onMeasured = null,
}) {
  resetAuthenticatedShellState();
  beginPermissionProfileGeneration();
  const startedAt = now();

  // Invoke both groups before awaiting either so independent calls never form
  // a critical -> optional waterfall.
  const criticalPromises = startTasks(criticalTasks);
  const deferredPromise = Promise.allSettled(startTasks(deferredTasks));
  const criticalResults = await Promise.allSettled(criticalPromises);
  const criticalDurationMs = now() - startedAt;
  const metrics = {
    userId,
    criticalDurationMs,
    criticalRequestCount: criticalTasks.length,
    deferredRequestCount: deferredTasks.length,
    withinBudget: criticalDurationMs <= AUTHENTICATED_SHELL_BOOTSTRAP_BUDGET_MS,
  };

  globalThis.performance?.mark?.('windshift-auth-shell-critical-ready');
  onMeasured?.(metrics);

  return { criticalResults, deferredPromise, metrics };
}
