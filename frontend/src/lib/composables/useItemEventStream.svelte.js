import { toExternal } from '../runtime/contextPath.js';
import { itemLiveUpdates } from '../stores/itemLiveUpdates.svelte.js';

const DEBOUNCE_MS = 250;

/**
 * Subscribes to an item's event stream and batches targeted reloads. A reconnect
 * performs one full reconcile; the initial connection does not duplicate the
 * initial load. Polling remains the fallback while disconnected.
 *
 * @param {() => (number|string|null|undefined)} getItemId
 * @param {{ onReconcile?: Function, onItem?: Function, onChildren?: Function, onComment?: Function, onLinks?: Function, onZammad?: Function, onDeleted?: Function }} handlers
 * @returns {{ readonly connected: boolean }}
 */
export function useItemEventStream(getItemId, handlers = {}) {
  let connected = $state(false);
  let streamItemId = $derived(normalizeItemEventStreamID(getItemId()));

  $effect(() => {
    const itemId = streamItemId;
    if (!itemId) return;
    if (typeof EventSource === 'undefined') return; // SSR / unsupported → polling stays the source of truth

    const pending = new Set();
    let timer = null;
    const connectionTracker = createConnectionReconcileTracker();

    const flush = () => {
      timer = null;
      const kinds = new Set(pending);
      pending.clear();
      // A full reconcile reloads everything, so skip the narrower reloads.
      if (kinds.has('reconcile')) {
        handlers.onReconcile?.();
        return;
      }
      if (kinds.has('item')) handlers.onItem?.();
      if (kinds.has('children')) handlers.onChildren?.();
      if (kinds.has('comment')) handlers.onComment?.();
      if (kinds.has('links')) handlers.onLinks?.();
      if (kinds.has('zammad')) handlers.onZammad?.();
      if (kinds.has('deleted')) handlers.onDeleted?.();
    };
    const schedule = (...kinds) => {
      for (const k of kinds) pending.add(k);
      if (!timer) timer = setTimeout(flush, DEBOUNCE_MS);
    };

    const es = new EventSource(toExternal(`/api/items/${itemId}/events`));
    const setConnected = (value) => {
      connected = value;
      itemLiveUpdates.set(itemId, value);
    };

    es.addEventListener('connected', () => {
      const shouldReconcile = connectionTracker.markConnected();
      setConnected(true);
      if (shouldReconcile) schedule('reconcile');
    });
    es.addEventListener('reload', () => schedule('reconcile'));
    es.addEventListener('comment', () => schedule('comment'));
    es.addEventListener('status', () => schedule('item'));
    es.addEventListener('updated', () => schedule('item', 'children'));
    es.addEventListener('created', () => schedule('item', 'children'));
    es.addEventListener('deleted', () => {
      // Deletion is authoritative and must tear down item-bound loaders before
      // their in-flight requests start returning expected 404s.
      if (timer) clearTimeout(timer);
      timer = null;
      pending.clear();
      handlers.onDeleted?.();
    });
    es.addEventListener('link', () => schedule('links'));
    es.addEventListener('zammad', () => schedule('zammad'));
    // The browser auto-reconnects (honoring the server's retry hint). Until it
    // does, mark disconnected so the components' pollers resume as the fallback.
    es.onerror = () => {
      connectionTracker.markDisconnected();
      setConnected(false);
    };

    return () => {
      if (timer) clearTimeout(timer);
      es.close();
      itemLiveUpdates.clear(itemId);
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
