import { toExternal } from '../runtime/contextPath.js';

/**
 * One multiplexed live-update connection per browser tab (WI-1656).
 *
 * Every topic family the tab needs — notifications, workspace/collection
 * invalidations, and open item changes — is carried by a single EventSource at
 * /api/events. Components register a scope token (`item:12`, `workspace:3`,
 * `collection:5`, `notifications`) and receive `onReconcile`/`onDeleted`
 * callbacks. The connection is replaced, not multiplied, as the tab navigates.
 *
 * Scope changes use make-before-break: the replacement connection is opened
 * while the current one stays live, and the current one is closed only once the
 * replacement reports `connected`. That way a navigation never opens a gap in
 * which an invalidation could be dropped, so no reconcile is needed for an
 * intentional swap. Only a real reconnect (browser auto-reconnect after an
 * error) and an explicit `resync` frame reconcile every scope.
 *
 * The class is exported so tests can drive a fresh connection; the app uses the
 * `eventStream` singleton through the module helpers.
 */
export class EventStreamConnection {
  /** @type {Map<string, Set<{onReconcile?: Function, onDeleted?: Function}>>} */
  #subscriptions = new Map();
  /** @type {{es: EventSource, url: string, connectedOnce: boolean}[]} */
  #connections = [];
  #scheduled = false;

  /** True while the newest connection is healthy. */
  connected = $state(false);

  /**
   * Register `handlers` for a scope token. Returns an unsubscribe function.
   * @param {string} token
   * @param {{onReconcile?: Function, onDeleted?: Function}} handlers
   */
  subscribe(token, handlers) {
    let handlersForToken = this.#subscriptions.get(token);
    if (!handlersForToken) {
      handlersForToken = new Set();
      this.#subscriptions.set(token, handlersForToken);
      this.#scheduleSwap();
    }
    handlersForToken.add(handlers);
    return () => {
      const current = this.#subscriptions.get(token);
      if (!current) return;
      current.delete(handlers);
      if (current.size === 0) {
        this.#subscriptions.delete(token);
        this.#scheduleSwap();
      }
    };
  }

  /** Close every connection and drop every subscription (test/teardown helper). */
  reset() {
    for (const entry of this.#connections) entry.es.close();
    this.#connections = [];
    this.#subscriptions.clear();
    this.connected = false;
  }

  // Coalesce the subscribe/unsubscribe burst a single navigation produces into
  // one connection swap.
  #scheduleSwap() {
    if (this.#scheduled) return;
    this.#scheduled = true;
    queueMicrotask(() => {
      this.#scheduled = false;
      this.#swap();
    });
  }

  #swap() {
    if (typeof EventSource === 'undefined') return;
    if (this.#subscriptions.size === 0) {
      for (const entry of this.#connections) entry.es.close();
      this.#connections = [];
      this.connected = false;
      return;
    }
    const url = this.#buildURL();
    const newest = this.#connections[this.#connections.length - 1];
    // Same scope set: the browser's own auto-reconnect owns the socket.
    if (newest && newest.url === url) return;
    this.#open(url);
  }

  #buildURL() {
    const items = [];
    const workspaces = [];
    const collections = [];
    for (const token of this.#subscriptions.keys()) {
      const separator = token.indexOf(':');
      const kind = separator === -1 ? token : token.slice(0, separator);
      const id = separator === -1 ? '' : token.slice(separator + 1);
      if (kind === 'item') items.push(id);
      else if (kind === 'workspace') workspaces.push(id);
      else if (kind === 'collection') collections.push(id);
    }
    const params = new URLSearchParams();
    if (items.length) params.set('items', items.join(','));
    if (workspaces.length) params.set('workspaces', workspaces.join(','));
    if (collections.length) params.set('collections', collections.join(','));
    const query = params.toString();
    return toExternal(`/api/events${query ? `?${query}` : ''}`);
  }

  #open(url) {
    const entry = { es: new EventSource(url), url, connectedOnce: false };
    this.#connections.push(entry);
    const es = entry.es;

    es.addEventListener('connected', () => {
      if (this.#connections[this.#connections.length - 1] !== entry) {
        // Superseded by a newer swap before it ever connected.
        es.close();
        this.#connections = this.#connections.filter((candidate) => candidate !== entry);
        return;
      }
      const reconnected = entry.connectedOnce;
      entry.connectedOnce = true;
      this.connected = true;
      if (!reconnected) {
        // The replacement is live; close the connections it supersedes. They
        // covered every scope until this moment, so nothing needs reconciling.
        for (const old of this.#connections) {
          if (old !== entry) old.es.close();
        }
        this.#connections = [entry];
        return;
      }
      // A real reconnect after a gap: every scope may have missed frames.
      this.#reconcileAll();
    });

    es.addEventListener('notification.changed', () => this.#dispatch('notifications', false));
    es.addEventListener('workspace.changed', (event) =>
      this.#dispatch(`workspace:${readScopeID(event, 'workspace_id')}`, false)
    );
    es.addEventListener('collection.changed', (event) =>
      this.#dispatch(`collection:${readScopeID(event, 'collection_id')}`, false)
    );
    es.addEventListener('item.changed', (event) =>
      this.#dispatch(`item:${readScopeID(event, 'item_id')}`, false)
    );
    es.addEventListener('item.deleted', (event) =>
      this.#dispatch(`item:${readScopeID(event, 'item_id')}`, true)
    );
    es.addEventListener('resync', () => this.#reconcileAll());

    es.onerror = () => {
      if (this.#connections[this.#connections.length - 1] !== entry) return;
      this.connected = false;
    };
  }

  #dispatch(token, deleted) {
    const handlersForToken = this.#subscriptions.get(token);
    if (!handlersForToken) return;
    for (const handler of handlersForToken) {
      if (deleted) handler.onDeleted?.();
      else handler.onReconcile?.();
    }
  }

  #reconcileAll() {
    for (const handlersForToken of this.#subscriptions.values()) {
      for (const handler of handlersForToken) handler.onReconcile?.();
    }
  }
}

function readScopeID(event, key) {
  try {
    const data = JSON.parse(event.data);
    return data?.[key] ?? '';
  } catch {
    return '';
  }
}

export const eventStream = new EventStreamConnection();

/** Subscribe to one scope token on the tab's shared connection. */
export function subscribeEventStream(token, handlers) {
  return eventStream.subscribe(token, handlers);
}

/** True while the tab's shared connection is healthy. */
export function eventStreamConnected() {
  return eventStream.connected;
}
