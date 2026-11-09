import { toExternal } from '../runtime/contextPath.js';

const DEBOUNCE_MS = 250;

/**
 * Subscribes to an item's coarse change stream. Any `changed` frame batches a
 * full reload; `deleted` tears the view down. Callers open the stream before
 * their initial snapshot, so the first healthy connection needs no reconcile;
 * only a reconnect (or an error before the first connect) reconciles to cover
 * changes missed while disconnected.
 *
 * @param {() => (number|string|null|undefined)} getItemId
 * @param {{ onReconcile?: Function, onDeleted?: Function }} handlers
 * @returns {{ readonly connected: boolean }}
 */
export function useItemEventStream(getItemId, handlers = {}) {
  let connected = $state(false);
  let streamItemId = $derived(normalizeItemEventStreamID(getItemId()));

  $effect(() => {
    const itemId = streamItemId;
    if (!itemId) return;
    if (typeof EventSource === 'undefined') return; // SSR / unsupported

    let timer = null;
    // A handler that was already running when the subscription was torn down
    // must not reconcile against the next item's view.
    let active = true;
    const connectionTracker = createConnectionReconcileTracker();

    const reconcile = async () => {
      timer = null;
      if (!active) return;
      try {
        await handlers.onReconcile?.();
      } catch (error) {
        if (!active) return;
        // The stream stays connected; the next change or reconnect retries.
        console.warn('Item event stream reconcile failed:', error);
      }
    };
    const scheduleReconcile = () => {
      if (!timer) timer = setTimeout(reconcile, DEBOUNCE_MS);
    };

    const es = new EventSource(toExternal(`/api/items/${itemId}/events`));

    es.addEventListener('connected', () => {
      const shouldReconcile = connectionTracker.markConnected();
      connected = true;
      if (shouldReconcile) scheduleReconcile();
    });
    es.addEventListener('changed', scheduleReconcile);
    es.addEventListener('deleted', () => {
      // Deletion is authoritative and must tear down item-bound loaders before
      // their in-flight requests start returning expected 404s.
      if (timer) clearTimeout(timer);
      timer = null;
      handlers.onDeleted?.();
    });
    // The browser auto-reconnects (honoring the server's retry hint). Until it
    // does, mark disconnected so the UI reflects that changes may be missed.
    es.onerror = () => {
      connectionTracker.markDisconnected();
      connected = false;
    };

    return () => {
      active = false;
      if (timer) clearTimeout(timer);
      es.close();
      connected = false;
    };
  });

  return {
    get connected() {
      return connected;
    },
  };
}

// Route parameters begin as strings and are later hydrated from API records as
// numbers. Keep that representation-only change from reopening the stream.
export function normalizeItemEventStreamID(itemId) {
  return itemId ? String(itemId) : null;
}

/**
 * Track whether a `connected` event represents the initial healthy stream or
 * recovery after a gap. Exported as a pure helper for regression tests.
 */
export function createConnectionReconcileTracker() {
  let connectedOnce = false;
  let disconnected = false;

  return {
    // The caller subscribes before taking its initial snapshot, so the first
    // healthy connection has no gap to reconcile. A reconnect or an error
    // before the first connect means changes may have been missed.
    markConnected() {
      const shouldReconcile = connectedOnce || disconnected;
      connectedOnce = true;
      disconnected = false;
      return shouldReconcile;
    },
    markDisconnected() {
      disconnected = true;
    },
  };
}
