// Wires the side effects that the dependency-free API and mobile modules must
// not import themselves: stores, translations, toasts and the router. App.svelte
// installs these at startup so an edit to a store, API domain or the router
// stays a hot update instead of reloading the page.

import { configureAPIEventHandlers } from '../api/core.js';
import { configureServiceWorkerNotifications } from '../mobile/serviceWorkerClient.js';
import { navigate } from '../router.js';
import { authStore } from '../stores/auth.svelte.js';
import { t } from '../stores/i18n.svelte.js';
import { addToast, surfaceError } from '../stores/toasts.svelte.js';
import { initCrossTabSync } from '../utils/crossTabSync.js';

// `installAppShellHandlers` is idempotent; track installed targets so a
// re-mount (or a test reusing a fake window) cannot stack duplicate handlers.
const globalErrorHandlerTargets = new WeakSet();

/**
 * Surface failures that no component caught. Without this they only exist in
 * the console, which is how silent errors reach the user as "nothing happened".
 * Exported with an injectable target so the wiring can be tested directly.
 * @param {Window | EventTarget | null} [target]
 */
export function installGlobalErrorHandlers(target = typeof window !== 'undefined' ? window : null) {
  if (!target || globalErrorHandlerTargets.has(target)) return;
  globalErrorHandlerTargets.add(target);

  // A promise rejection nobody handled. Aborting is navigation/cancellation
  // control flow, not a failure the user should see.
  target.addEventListener('unhandledrejection', (event) => {
    const reason = /** @type {PromiseRejectionEvent} */ (event).reason;
    if (reason?.name === 'AbortError') return;
    surfaceError(reason, { context: 'unhandledrejection' });
  });

  // Synchronous errors outside Svelte's render tree. Resource load failures
  // (a missing image or a 404 chunk) carry no `error` and are not surfaced.
  target.addEventListener('error', (event) => {
    const { error } = /** @type {ErrorEvent} */ (event);
    if (!error) return;
    surfaceError(error, { context: 'window.onerror' });
  });
}

/** Install the injected handlers. Safe to call more than once. */
export function installAppShellHandlers() {
  installGlobalErrorHandlers();

  configureAPIEventHandlers({
    onAuthExpired: () => authStore.clearAuth(),
    onClockDrift: notifyAdminOfClockDrift,
  });

  configureServiceWorkerNotifications({
    notifyUpdateAvailable: ({ onApply }) =>
      addToast({
        title: 'Update available',
        message: 'A new version of Windshift is ready.',
        variant: 'info',
        actionLabel: 'Reload',
        duration: 0,
        clickable: true,
        onClick: onApply,
      }),
  });

  // Refresh open tabs after another tab mutates a work item. Polling still
  // covers server-side changes.
  initCrossTabSync({ onInvalidate: invalidateCollectionViews });
}

export async function invalidateCollectionViews() {
  const { scheduleCollectionReconciliation } = await import('../stores/collectionContext.js');
  scheduleCollectionReconciliation();
}

/** @param {{ offsetMs: number }} event */
export function notifyAdminOfClockDrift({ offsetMs }) {
  let user;
  authStore.subscribe((state) => (user = state.currentUser))();
  if (!(/** @type {any} */ (user)?.is_system_admin)) return;

  const offsetSec = Math.round(offsetMs / 1000);
  const absMin = Math.floor(Math.abs(offsetSec) / 60);
  const absSec = Math.abs(offsetSec) % 60;
  const direction = offsetSec > 0 ? 'ahead' : 'behind';
  const amount = absMin > 0 ? `${absMin}m ${absSec}s ${direction}` : `${absSec}s ${direction}`;

  addToast({
    message: `Server clock appears to be ${amount}. Click for details.`,
    title: t('toast.warning'),
    variant: 'warning',
    clickable: true,
    onClick: () => navigate('/admin/diagnostics?subtab=clock'),
  });
}
