import { collections } from '../api/collections.js';
import {
  COLLECTION_VIEW_IDS,
  workspaceOnlyViews,
  workspaceViewItems,
} from '../navigation/workspaceNavigation.js';
import { collectionStore } from './collectionContext.js';

// Every collection-scoped view, used whenever a scope has no explicit
// enabled-views setting or its lookup fails.
const ALL_VIEW_IDS = workspaceViewItems.map((view) => view.id);

// Every toggleable workspace nav item (views plus the workspace-only tools
// entries). The ids mirror models.WorkspaceNavItemIDs on the backend;
// test-management entries are not toggleable and stay gated by module and
// permission only.
export const ALL_NAV_IDS = [...ALL_VIEW_IDS, ...workspaceOnlyViews.map((view) => view.id)];

// Registry order for nav ids, so locally applied sets match server ordering.
const NAV_ID_ORDER = new Map(ALL_NAV_IDS.map((id, index) => [id, index]));

// Workspace-scope fallback when a scope has no explicit enabled-views setting:
// everything except the support queue, which is opt-in via the helpdesk pack.
// Mirrors models.DefaultWorkspaceNavItemIDs on the backend.
export const DEFAULT_NAV_IDS = ALL_NAV_IDS.filter((id) => id !== 'queue');

/** @typedef {{ views: string[], inherited: boolean, loaded: boolean }} ViewSettingsEntry */

/** @type {ViewSettingsEntry} */
const DEFAULT_ENTRY = { views: DEFAULT_NAV_IDS, inherited: true, loaded: false };

function createViewSettingsStore() {
  /** @type {Record<string, ViewSettingsEntry>} */
  let entries = $state({});
  const inflight = new Map();

  function scopeKey(workspaceId, collectionId) {
    return `${collectionId ?? ''}|${workspaceId ?? ''}`;
  }

  /**
   * Loads the effective enabled views for a scope. Missing or invalid
   * settings resolve to the workspace default (queue off); failures are
   * cached as the default so navigation never blocks on the lookup. Writes
   * to the store happen exactly once per load so effects reading entries
   * settle.
   */
  async function load(workspaceId, collectionId = null) {
    const key = scopeKey(workspaceId, collectionId);
    if (entries[key]?.loaded) return entries[key];

    const pending = inflight.get(key);
    if (pending) return pending;

    const promise = (async () => {
      let entry = DEFAULT_ENTRY;
      try {
        const config = await resolveBoardConfiguration(workspaceId, collectionId);
        const enabled = config?.view_settings?.enabled_views;
        entry = {
          // Missing or empty settings mean the workspace default (everything
          // except the queue) — tools entries included.
          views: Array.isArray(enabled) && enabled.length > 0 ? enabled : DEFAULT_NAV_IDS,
          inherited: Boolean(config?.view_settings_inherited),
          loaded: true,
        };
      } catch {
        // Keep the default entry.
      }
      entries[key] = entry;
      return entry;
    })();
    inflight.set(key, promise);
    try {
      return await promise;
    } finally {
      inflight.delete(key);
    }
  }

  /**
   * Prefer the collection view's board-configuration request for the same
   * scope so navigation does not issue a duplicate plain-config read. The
   * one-microtask wait lets a sibling view effect register its bootstrap in
   * the same flush; navigation-only scopes still use the lightweight endpoint.
   */
  async function resolveBoardConfiguration(workspaceId, collectionId) {
    let peeked = collectionStore.peekBoardConfiguration(workspaceId, collectionId);
    if (!peeked) {
      await Promise.resolve();
      peeked = collectionStore.peekBoardConfiguration(workspaceId, collectionId);
    }
    if (peeked) {
      if (peeked.config !== undefined) return peeked.config;
      return await peeked.pending;
    }
    return collections.getBoardConfiguration(collectionId, workspaceId);
  }

  /** Enabled view ids for a scope; the workspace default while unloaded. */
  function enabledViewIds(workspaceId, collectionId = null) {
    return entries[scopeKey(workspaceId, collectionId)]?.views ?? DEFAULT_NAV_IDS;
  }

  /**
   * Effective workspace nav ids (views plus tools/test entries) for the
   * workspace default context. Only the workspace scope can toggle the
   * non-view entries, so collection scopes are never consulted here. The
   * workspace default applies while unloaded.
   */
  function enabledNavIds(workspaceId) {
    return entries[scopeKey(workspaceId, null)]?.views ?? DEFAULT_NAV_IDS;
  }

  /**
   * Records the view set a successful write just persisted for a scope, so
   * navigation reacts immediately instead of waiting for a refetch that may
   * read a stale board-configuration cache. A collection's override keeps the
   * workspace-only ids inherited from the workspace scope, mirroring the
   * server's merge.
   */
  function applyEnabledViews(workspaceId, collectionId, views, inherited = false) {
    const scopeViews = collectionId
      ? [
          ...views,
          ...(entries[scopeKey(workspaceId, null)]?.views ?? DEFAULT_NAV_IDS).filter(
            (id) => !COLLECTION_VIEW_IDS.has(id)
          ),
        ]
      : [...views];
    entries[scopeKey(workspaceId, collectionId)] = {
      views: [...new Set(scopeViews)].sort(
        (a, b) =>
          (NAV_ID_ORDER.get(a) ?? Number.MAX_SAFE_INTEGER) -
          (NAV_ID_ORDER.get(b) ?? Number.MAX_SAFE_INTEGER)
      ),
      inherited: Boolean(inherited),
      loaded: true,
    };
  }

  /** The raw scope entry (reactive), or null before the first load. */
  function entryFor(workspaceId, collectionId = null) {
    return entries[scopeKey(workspaceId, collectionId)] ?? null;
  }

  /** Whether the scope's settings come from the workspace default. */
  function inherited(workspaceId, collectionId = null) {
    return entries[scopeKey(workspaceId, collectionId)]?.inherited ?? true;
  }

  function invalidate(workspaceId, collectionId = null) {
    delete entries[scopeKey(workspaceId, collectionId)];
  }

  /** Drops the workspace scope and every collection cache under it. */
  function invalidateWorkspace(workspaceId) {
    const prefix = `${workspaceId ?? ''}`;
    for (const key of Object.keys(entries)) {
      if (key.split('|')[1] === prefix) delete entries[key];
    }
  }

  return {
    load,
    entryFor,
    applyEnabledViews,
    enabledViewIds,
    enabledNavIds,
    inherited,
    invalidate,
    invalidateWorkspace,
    allViewIds: ALL_VIEW_IDS,
    allNavIds: DEFAULT_NAV_IDS,
  };
}

export const viewSettingsStore = createViewSettingsStore();
