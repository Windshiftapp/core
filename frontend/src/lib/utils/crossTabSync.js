// Broadcasts local work-item mutations so other tabs can refresh active
// collection views. Polling still covers server-side changes.

import { createId } from './uuid.js';

const CHANNEL_NAME = 'windshift-work-items';

/** Unique-per-tab id used to identify the origin of a broadcast. */
export const tabId = createId();

let channel = null;
let initialized = false;
let activeHandlers = null;

/** @returns {BroadcastChannel | null} */
function getChannel() {
  if (channel) return channel;
  if (typeof BroadcastChannel === 'undefined') return null;
  try {
    channel = new BroadcastChannel(CHANNEL_NAME);
  } catch {
    channel = null;
  }
  return channel;
}

/**
 * Best-effort mutation broadcast.
 * @param {{ type?: string, itemId?: number|string, parentId?: number|string|null }} [detail]
 */
export function notifyItemMutation(detail = {}) {
  const ch = getChannel();
  if (!ch) return;
  try {
    ch.postMessage({
      type: detail.type || 'update',
      itemId: detail.itemId ?? null,
      parentId: detail.parentId ?? null,
      origin: tabId,
      ts: Date.now(),
    });
  } catch {
    // Broadcasting must not break the originating mutation.
  }
}

/**
 * Installs one debounced listener for other tabs' mutations.
 * @param {{ onInvalidate?: () => Promise<void>|void, debounceMs?: number }} [handlers]
 * @returns {() => void} disposer
 */
export function initCrossTabSync(handlers = {}) {
  // HMR re-executes the injecting module, so a repeat init swaps the handlers
  // instead of leaving a listener that calls into stale code.
  activeHandlers = handlers;
  if (initialized) return () => {};
  initialized = true;

  const ch = getChannel();
  if (!ch) {
    // Preserve a uniform disposer when BroadcastChannel is unavailable.
    return () => {
      initialized = false;
    };
  }

  const debounceMs = handlers.debounceMs ?? 200;
  let refreshTimer = null;

  const onMessage = (/** @type {MessageEvent} */ event) => {
    const data = event?.data;
    if (!data || data.origin === tabId) return; // ignore self echoes
    if (typeof activeHandlers?.onInvalidate !== 'function') return;

    // Coalesce bulk mutations into one invalidation.
    if (refreshTimer !== null) clearTimeout(refreshTimer);
    refreshTimer = setTimeout(() => {
      refreshTimer = null;
      Promise.resolve(activeHandlers.onInvalidate()).catch((err) =>
        console.warn('[crossTabSync] invalidation handler failed:', err)
      );
    }, activeHandlers.debounceMs ?? debounceMs);
  };

  ch.addEventListener('message', onMessage);

  return () => {
    if (refreshTimer !== null) clearTimeout(refreshTimer);
    refreshTimer = null;
    ch.removeEventListener('message', onMessage);
    try {
      ch.close();
    } catch {
      /* ignore */
    }
    channel = null;
    activeHandlers = null;
    initialized = false;
  };
}
