/**
 * Background polling is not useful while the browser has suspended the page
 * or knows it is offline. Skipping those requests also avoids a burst of
 * browser-level connection errors when a long-hidden tab wakes up.
 *
 * @param {{ document?: Document, navigator?: Navigator }} [environment]
 */
export function canRunBackgroundSync(environment = {}) {
  const documentRef = environment.document ?? globalThis.document;
  const navigatorRef = environment.navigator ?? globalThis.navigator;

  if (documentRef?.hidden || documentRef?.visibilityState === 'hidden') return false;
  if (navigatorRef?.onLine === false) return false;
  return true;
}

/**
 * Failures caused by connectivity changes or page teardown are expected
 * control flow for background refreshes. Other errors still deserve logging.
 *
 * @param {unknown} error
 */
export function isExpectedBackgroundSyncError(error) {
  const candidate = /** @type {{ name?: string, code?: string }} */ (error);
  return (
    candidate?.name === 'AbortError' ||
    candidate?.code === 'NETWORK_ERROR' ||
    candidate?.code === 'REQUEST_TIMEOUT'
  );
}

/**
 * Invoke a refresh callback as soon as a hidden/offline page is usable again.
 * The callback owns its own in-flight de-duplication.
 *
 * Only pages that were actually suspended (hidden or offline) refresh on
 * recovery: browsers deliver spurious `online`/`visibilitychange` events, and
 * re-fetching full workspace data on those would waste traffic for nothing.
 *
 * @param {() => void} callback
 * @param {{ document?: Document, navigator?: Navigator, window?: Window }} [environment]
 * @returns {() => void}
 */
export function onBackgroundSyncAvailable(callback, environment = {}) {
  const documentRef = environment.document ?? globalThis.document;
  const navigatorRef = environment.navigator ?? globalThis.navigator;
  const windowRef = environment.window ?? globalThis.window;

  let suspended = !canRunBackgroundSync({ document: documentRef, navigator: navigatorRef });
  const markSuspended = () => {
    if (!canRunBackgroundSync({ document: documentRef, navigator: navigatorRef })) {
      suspended = true;
    }
  };
  const refreshIfAvailable = () => {
    if (!suspended) return;
    if (canRunBackgroundSync({ document: documentRef, navigator: navigatorRef })) {
      suspended = false;
      callback();
    }
  };

  documentRef?.addEventListener('visibilitychange', markSuspended);
  documentRef?.addEventListener('visibilitychange', refreshIfAvailable);
  windowRef?.addEventListener('online', markSuspended);
  windowRef?.addEventListener('online', refreshIfAvailable);

  return () => {
    documentRef?.removeEventListener('visibilitychange', markSuspended);
    documentRef?.removeEventListener('visibilitychange', refreshIfAvailable);
    windowRef?.removeEventListener('online', markSuspended);
    windowRef?.removeEventListener('online', refreshIfAvailable);
  };
}
