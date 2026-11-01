import { userPreferences } from '../api/users.js';

// Per-user collection view configuration and other top-level user
// preferences, stored in user_preferences so they follow the user across
// devices. Existing localStorage values stay readable as a fallback; writes
// always go to the server. All fields share one GET/PATCH pair so the shell
// loads the whole preferences document exactly once.

const SAVE_DEBOUNCE_MS = 400;

/** Canonical scope id for a collection or workspace default view. */
export function collectionScopeId(workspaceId, collectionId) {
  return collectionId ? `collection-${collectionId}` : `workspace-${workspaceId ?? 'global'}`;
}

class CollectionPreferencesStore {
  /** @type {Record<string, Record<string, object>>} scope -> view -> state */
  viewState = $state({});
  /** @type {Record<string, unknown[]>} scope -> personal saved filters */
  savedFilters = $state({});
  /** @type {'oldest' | 'newest'} preferred comment thread order */
  commentSortOrder = $state('oldest');
  loaded = $state(false);

  #userId = null;
  #loadPromise = null;
  #generation = 0;
  #dirtyViewScopes = new Set();
  #dirtyFilterScopes = new Set();
  #dirtyCommentSort = false;
  #saveTimer = null;
  #saving = null;

  /**
   * Load the user's preferences once. Safe to call repeatedly; a call for a
   * different user resets the cache first.
   */
  ensureLoaded(userId = null, { timeout = 0 } = {}) {
    if (userId != null && this.#userId != null && this.#userId !== userId) {
      this.reset();
    }
    if (userId != null) this.#userId = userId;
    if (this.loaded) return Promise.resolve();
    if (this.#loadPromise) return this.#loadPromise;

    const generation = this.#generation;
    const promise = userPreferences
      .get({ timeout })
      .then((prefs) => {
        if (generation !== this.#generation) return;
        this.viewState = prefs?.view_state ?? {};
        this.savedFilters = prefs?.saved_filters ?? {};
        this.commentSortOrder = prefs?.comment_sort_order === 'newest' ? 'newest' : 'oldest';
        this.loaded = true;
      })
      .catch((error) => {
        console.warn('[collectionPreferences] failed to load preferences', error);
      })
      .finally(() => {
        if (this.#loadPromise === promise) this.#loadPromise = null;
      });
    this.#loadPromise = promise;
    return promise;
  }

  /** Clear the cache on logout or user change. */
  reset() {
    this.#generation += 1;
    this.viewState = {};
    this.savedFilters = {};
    this.commentSortOrder = 'oldest';
    this.loaded = false;
    this.#userId = null;
    this.#loadPromise = null;
    this.#dirtyViewScopes.clear();
    this.#dirtyFilterScopes.clear();
    this.#dirtyCommentSort = false;
    if (this.#saveTimer) {
      clearTimeout(this.#saveTimer);
      this.#saveTimer = null;
    }
  }

  /** View state for a scope, or null when nothing is stored server-side. */
  getViewState(workspaceId, collectionId, view) {
    return this.viewState[collectionScopeId(workspaceId, collectionId)]?.[view] ?? null;
  }

  /**
   * One setting's server value. Returns undefined when the key was never
   * stored, which lets callers fall back to localStorage while still honoring
   * an explicit null (a cleared setting).
   */
  viewStateValue(workspaceId, collectionId, view, key) {
    const state = this.getViewState(workspaceId, collectionId, view);
    return state && Object.hasOwn(state, key) ? state[key] : undefined;
  }

  /** Merge settings into one view's state and persist. */
  patchViewState(workspaceId, collectionId, view, patch) {
    const scope = collectionScopeId(workspaceId, collectionId);
    const currentScope = this.viewState[scope] ?? {};
    const currentView = currentScope[view] ?? {};
    this.viewState = {
      ...this.viewState,
      [scope]: { ...currentScope, [view]: { ...currentView, ...patch } },
    };
    this.#dirtyViewScopes.add(scope);
    this.#scheduleSave();
  }

  /** Personal saved filters for a scope, or null when nothing is stored. */
  getSavedFilters(workspaceId, collectionId) {
    return this.savedFilters[collectionScopeId(workspaceId, collectionId)] ?? null;
  }

  /** Replace a scope's personal saved filters and persist. */
  setSavedFilters(workspaceId, collectionId, filters) {
    const scope = collectionScopeId(workspaceId, collectionId);
    this.savedFilters = { ...this.savedFilters, [scope]: filters };
    this.#dirtyFilterScopes.add(scope);
    this.#scheduleSave();
  }

  /** Set the preferred comment thread order and persist it. */
  setCommentSortOrder(order) {
    const next = order === 'newest' ? 'newest' : 'oldest';
    if (this.commentSortOrder === next) return;
    this.commentSortOrder = next;
    this.#dirtyCommentSort = true;
    this.#scheduleSave();
  }

  #scheduleSave() {
    if (this.#saveTimer) clearTimeout(this.#saveTimer);
    this.#saveTimer = setTimeout(() => {
      this.#saveTimer = null;
      void this.flush();
    }, SAVE_DEBOUNCE_MS);
  }

  /** Flush pending writes immediately (also used by tests). */
  async flush({ keepalive = false } = {}) {
    if (this.#saveTimer) {
      clearTimeout(this.#saveTimer);
      this.#saveTimer = null;
    }
    if (this.#saving) {
      // A save is already in flight; the new dirty scopes ride the next flush.
      await this.#saving;
    }
    const viewScopes = [...this.#dirtyViewScopes];
    const filterScopes = [...this.#dirtyFilterScopes];
    const commentSortDirty = this.#dirtyCommentSort;
    if (viewScopes.length === 0 && filterScopes.length === 0 && !commentSortDirty) return;

    this.#dirtyViewScopes.clear();
    this.#dirtyFilterScopes.clear();
    this.#dirtyCommentSort = false;

    const body = {};
    if (viewScopes.length > 0) {
      body.view_state = Object.fromEntries(
        viewScopes.map((scope) => [scope, this.viewState[scope] ?? null])
      );
    }
    if (filterScopes.length > 0) {
      body.saved_filters = Object.fromEntries(
        filterScopes.map((scope) => [scope, this.savedFilters[scope] ?? null])
      );
    }
    if (commentSortDirty) {
      body.comment_sort_order = this.commentSortOrder;
    }

    const save = keepalive
      ? userPreferences.update(body, { keepalive: true })
      : userPreferences.update(body);
    this.#saving = save
      .catch((error) => {
        // Re-mark for retry; the local state stays authoritative.
        for (const scope of viewScopes) this.#dirtyViewScopes.add(scope);
        for (const scope of filterScopes) this.#dirtyFilterScopes.add(scope);
        if (commentSortDirty) this.#dirtyCommentSort = true;
        console.warn('[collectionPreferences] failed to save preferences', error);
      })
      .finally(() => {
        this.#saving = null;
      });
    await this.#saving;
  }
}

export const collectionPreferences = new CollectionPreferencesStore();

// Debounced writes must not be lost when the user reloads or closes the tab
// before the timer fires. A keepalive PATCH survives the unload.
if (typeof window !== 'undefined') {
  window.addEventListener('pagehide', () => {
    void collectionPreferences.flush({ keepalive: true });
  });
}
