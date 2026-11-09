import { eventStreamConnected, subscribeEventStream } from './eventStream.svelte.js';

const DEBOUNCE_MS = 250;

/**
 * Subscribes to a collection's (or workspace's) coarse invalidation stream on
 * the tab's shared multiplexed connection and batches refreshes. Every frame
 * maps to the caller's existing delta fetch, so the stream never has to carry
 * item-level detail.
 *
 * @param {() => ({ kind: 'collection'|'workspace', id: number|string|null }|null)} getScope
 * @param {{ onInvalidate?: Function }} [handlers]
 * @returns {{ readonly connected: boolean }}
 */
export function useCollectionEventStream(getScope, handlers = {}) {
  let scopeKey = $derived.by(() => {
    const scope = getScope();
    if (!scope || scope.id == null || scope.id === '') return null;
    return `${scope.kind}:${scope.id}`;
  });

  $effect(() => {
    const key = scopeKey;
    if (!key) return;

    let pending = false;
    let timer = null;
    const schedule = () => {
      pending = true;
      if (timer) return;
      timer = setTimeout(() => {
        timer = null;
        if (!pending) return;
        pending = false;
        handlers.onInvalidate?.();
      }, DEBOUNCE_MS);
    };

    const unsubscribe = subscribeEventStream(key, { onReconcile: schedule });

    return () => {
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
