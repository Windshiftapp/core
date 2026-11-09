import { eventStreamConnected, subscribeEventStream } from './eventStream.svelte.js';

const DEBOUNCE_MS = 250;

/**
 * Subscribes to an item's coarse change stream on the tab's shared multiplexed
 * connection. Any `changed` frame batches a full reload; `deleted` tears the
 * view down. The connection is opened before the initial snapshot, so the first
 * healthy connection needs no reconcile; only a reconnect (or an error before
 * the first connect) reconciles to cover changes missed while disconnected.
 *
 * @param {() => (number|string|null|undefined)} getItemId
 * @param {{ onReconcile?: Function, onDeleted?: Function }} handlers
 * @returns {{ readonly connected: boolean }}
 */
export function useItemEventStream(getItemId, handlers = {}) {
  let streamItemId = $derived(normalizeItemEventStreamID(getItemId()));

  $effect(() => {
    const itemId = streamItemId;
    if (!itemId) return;

    let timer = null;
    // A handler that was already running when the subscription was torn down
    // must not reconcile against the next item's view.
    let active = true;
    const scheduleReconcile = () => {
      if (!active || timer) return;
      timer = setTimeout(() => {
        timer = null;
        if (!active) return;
        Promise.resolve()
          .then(() => handlers.onReconcile?.())
          .catch((error) => {
            if (!active) return;
            // The stream stays connected; the next change or reconnect retries.
            console.warn('Item event stream reconcile failed:', error);
          });
      }, DEBOUNCE_MS);
    };

    const unsubscribe = subscribeEventStream(`item:${itemId}`, {
      onReconcile: scheduleReconcile,
      onDeleted: () => {
        // Deletion is authoritative and must tear down item-bound loaders before
        // their in-flight requests start returning expected 404s.
        if (timer) clearTimeout(timer);
        timer = null;
        handlers.onDeleted?.();
      },
    });

    return () => {
      active = false;
      if (timer) clearTimeout(timer);
      unsubscribe();
    };
  });

  return {
    get connected() {
      return eventStreamConnected();
    },
  };
}

// Route parameters begin as strings and are later hydrated from API records as
// numbers. Keep that representation-only change from reopening the stream.
export function normalizeItemEventStreamID(itemId) {
  return itemId ? String(itemId) : null;
}
