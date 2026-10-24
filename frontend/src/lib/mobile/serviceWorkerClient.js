let registrationPromise = null;
let updateReloadRequested = false;
const UPDATE_APPLIED_KEY = 'pwa-just-updated';
const UPDATE_SUPPRESSION_MS = 30_000;

// Injected by the app shell so this module never imports the toast store (and
// through it i18n) while main.js still owns the registration.
let notifyUpdateAvailable = null;
let pendingOffer = null;

/**
 * @param {{ notifyUpdateAvailable?: null | ((update: { onApply: () => void }) => void) }} [handlers]
 */
export function configureServiceWorkerNotifications(handlers = {}) {
  if (!('notifyUpdateAvailable' in handlers)) return;
  notifyUpdateAvailable =
    typeof handlers.notifyUpdateAvailable === 'function' ? handlers.notifyUpdateAvailable : null;
  if (notifyUpdateAvailable && pendingOffer) {
    const offer = pendingOffer;
    pendingOffer = null;
    offer();
  }
}

function recentlyAppliedUpdate() {
  try {
    const appliedAt = Number(localStorage.getItem(UPDATE_APPLIED_KEY));
    return Number.isFinite(appliedAt) && Date.now() - appliedAt < UPDATE_SUPPRESSION_MS;
  } catch {
    return false;
  }
}

function rememberAppliedUpdate() {
  try {
    localStorage.setItem(UPDATE_APPLIED_KEY, String(Date.now()));
  } catch {
    // Storage can be unavailable in private browsing contexts.
  }
}

function applyWaitingUpdate(registration) {
  if (updateReloadRequested) return;
  updateReloadRequested = true;
  rememberAppliedUpdate();

  const reload = () => window.location.reload();
  if (navigator.serviceWorker?.addEventListener) {
    navigator.serviceWorker.addEventListener('controllerchange', reload, { once: true });
  }

  if (registration.waiting) {
    registration.waiting.postMessage({ type: 'SKIP_WAITING' });
  } else {
    reload();
  }
}

function monitorRegistration(registration) {
  let updateOffered = false;

  const offerUpdate = () => {
    if (
      updateOffered ||
      !navigator.serviceWorker.controller ||
      !registration.waiting ||
      recentlyAppliedUpdate()
    ) {
      return;
    }
    if (!notifyUpdateAvailable) {
      // Hold the offer until the shell registers a notifier. A waiting worker
      // keeps waiting, so deferring the toast loses nothing.
      pendingOffer = offerUpdate;
      return;
    }
    updateOffered = true;
    notifyUpdateAvailable({ onApply: () => applyWaitingUpdate(registration) });
  };

  const watchInstalling = () => {
    const installing = registration.installing;
    if (!installing) return;
    if (installing.state === 'installed') {
      offerUpdate();
      return;
    }
    installing.addEventListener?.('statechange', () => {
      if (installing.state === 'installed') offerUpdate();
    });
  };

  registration.addEventListener?.('updatefound', watchInstalling);
  watchInstalling();
  if (registration.waiting) queueMicrotask(offerUpdate);
}

/**
 * Drop service workers and their caches on a dev origin. A worker installed by
 * an earlier production build on the same origin serves stale modules and
 * defeats HMR, so the app shell clears it before the first import.
 * @returns {Promise<number>} number of unregistered workers
 */
export async function clearDevelopmentServiceWorkerState() {
  if (typeof navigator === 'undefined' || !navigator.serviceWorker?.getRegistrations) {
    return 0;
  }

  /** @type {readonly ServiceWorkerRegistration[]} */
  let registrations = [];
  try {
    registrations = await navigator.serviceWorker.getRegistrations();
  } catch (error) {
    console.warn('[mobile] could not list service workers:', error);
    return 0;
  }

  await Promise.all(
    registrations.map((registration) =>
      Promise.resolve(registration.unregister()).catch(() => false)
    )
  );

  if (typeof caches !== 'undefined') {
    try {
      const keys = await caches.keys();
      await Promise.all(keys.map((key) => caches.delete(key)));
    } catch (error) {
      console.warn('[mobile] could not clear dev caches:', error);
    }
  }

  return registrations.length;
}

/**
 * Register the mobile service worker. Idempotent — repeated calls return the
 * same in-flight/settled registration. Paths resolve against document.baseURI
 * so root and context-path deployments use the correct script and scope.
 *
 * @returns {Promise<ServiceWorkerRegistration|null>}
 */
export function registerMobileServiceWorker() {
  if (typeof navigator === 'undefined' || !('serviceWorker' in navigator)) {
    return Promise.resolve(null);
  }
  if (registrationPromise) return registrationPromise;

  const swUrl = new URL('service-worker.js', document.baseURI).pathname;
  const scope = new URL('./', document.baseURI).pathname;

  registrationPromise = navigator.serviceWorker
    .register(swUrl, { scope })
    .then((registration) => {
      monitorRegistration(registration);
      return registration;
    })
    .catch((err) => {
      console.warn('[mobile] service worker registration failed:', err);
      registrationPromise = null;
      return null;
    });

  return registrationPromise;
}

/** Test helper: allow isolated registration tests without reloading the module. */
export function resetServiceWorkerRegistrationForTests() {
  registrationPromise = null;
  updateReloadRequested = false;
  notifyUpdateAvailable = null;
  pendingOffer = null;
}
