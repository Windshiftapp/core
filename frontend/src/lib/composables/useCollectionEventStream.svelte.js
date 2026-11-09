import { toExternal } from '../runtime/contextPath.js';
import { createConnectionReconcileTracker } from './useItemEventStream.svelte.js';

const DEBOUNCE_MS = 250;

/**
 * Subscribes to a collection's (or workspace's) coarse invalidation stream and
 * batches refreshes. Every frame maps to the caller's existing delta fetch, so
 * the stream never has to carry item-level detail.
 *
 * @param {() => ({ kind: 'collection'|'workspace', id: number|string|null }|null)} getScope
 * @param {{ onInvalidate?: () => void }} [handlers]
 * @returns {{ readonly connected: boolean }}
 */
export function useCollectionEventStream(getScope, handlers = {}) {
  let connected = $state(false);
  let scopeKey = $derived.by(() => {
    const scope = getScope();
    if (!scope || scope.id == null || scope.id === '') return null;
    return `${scope.kind}:${scope.id}`;
  });

  $effect(() => {
    const key = scopeKey;
    if (!key) return;
    // SSR / unsupported browsers keep the HTTP poller as the source of truth.
    if (typeof EventSource === 'undefined') return;

    const [kind, id] = key.split(':');
    const url =
      kind === 'workspace'
        ? toExternal(`/api/workspaces/${id}/events`)
        : toExternal(`/api/collections/${id}/events`);

    let pending = false;
    let timer = null;
    const connectionTracker = createConnectionReconcileTracker();
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

    const es = new EventSource(url);
    es.addEventListener('connected', () => {
      connected = true;
      // Callers subscribe before their initial snapshot, so the first healthy
      // connection is already covered. Reconnect (or an error before the first
      // connect) still reconciles.
      if (connectionTracker.markConnected()) schedule();
    });
    es.addEventListener('changed', schedule);
    // The browser auto-reconnects; until it does, mark disconnected so the
    // poller resumes as the fallback.
    es.onerror = () => {
      connectionTracker.markDisconnected();
      connected = false;
    };

    return () => {
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
