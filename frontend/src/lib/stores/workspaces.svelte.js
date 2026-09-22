import { derived, get, writable } from 'svelte/store';
import { api } from '../api.js';
import { isExpectedBackgroundSyncError } from '../utils/backgroundSync.js';

// Current workspace store - automatically syncs with route
function createCurrentWorkspaceStore() {
  const { subscribe, set, update } = writable(null);
  let lastWorkspaceId = null;
  let loadGeneration = 0;

  return {
    subscribe,

    // Hydrate from an already-fetched workspace snapshot. This is used by the
    // shared workspace bootstrap/store so route consumers do not issue a
    // second GET for the same workspace.
    hydrate(workspace) {
      if (!workspace?.id) return;
      loadGeneration += 1;
      set(workspace);
      lastWorkspaceId = String(workspace.id);
    },

    // Patch workspace with partial updates (no API call)
    patch(updates) {
      update((ws) => (ws ? { ...ws, ...updates } : null));
    },

    // Load workspace by ID
    async load(workspaceId) {
      if (!workspaceId) {
        loadGeneration += 1;
        set(null);
        lastWorkspaceId = null;
        return;
      }

      // Avoid unnecessary API calls if workspace ID hasn't changed
      const workspaceKey = String(workspaceId);
      if (workspaceKey === lastWorkspaceId) {
        return;
      }

      const generation = ++loadGeneration;
      try {
        const workspace = await api.workspaces.get(workspaceId);
        if (generation !== loadGeneration) return;
        set(workspace);
        // Only mark this id as loaded once the fetch actually succeeded —
        // otherwise a transient failure would suppress all retries.
        lastWorkspaceId = workspaceKey;
      } catch (error) {
        if (generation !== loadGeneration) return;
        if (isExpectedBackgroundSyncError(error)) return;
        console.error('Failed to load workspace:', error);
        set(null);
        lastWorkspaceId = null;
      }
    },

    // Clear workspace
    clear() {
      loadGeneration += 1;
      set(null);
      lastWorkspaceId = null;
    },
  };
}

// Workspaces store - manages the workspace directory.
//
// The directory loads the first server page only (WI-1442): at a 10k-workspace
// tenant the old fetch-all loop cost ~100 sequential requests on every app
// boot. Surfaces that need rows beyond the cached page use searchWorkspaces(),
// which searches server-side; truncated reports whether the cache is partial.
const DIRECTORY_PAGE_SIZE = 200;

function createWorkspacesStore() {
  const workspaces = writable([]);
  const personalWorkspace = writable(null);
  const loaded = writable(false);
  const loading = writable(false);
  const total = writable(0);
  let lifecycleGeneration = 0;
  let listLoadGeneration = 0;
  let listLoadPromise = null;
  let personalLoadGeneration = 0;
  let personalLoadPromise = null;
  let searchGeneration = 0;
  let searchPromise = null;

  // Derived store for regular (non-personal) workspaces
  const regularWorkspaces = derived(workspaces, ($workspaces) =>
    $workspaces.filter((ws) => !ws.is_personal)
  );

  const truncated = derived(
    [workspaces, total],
    ([$workspaces, $total]) => $total > $workspaces.length
  );

  // Create a derived store that combines all state for easy subscription
  const combined = derived(
    [workspaces, personalWorkspace, loaded, loading, regularWorkspaces, total, truncated],
    ([
      $workspaces,
      $personalWorkspace,
      $loaded,
      $loading,
      $regularWorkspaces,
      $total,
      $truncated,
    ]) => ({
      workspaces: $workspaces,
      allWorkspaces: $workspaces,
      personalWorkspace: $personalWorkspace,
      loaded: $loaded,
      loading: $loading,
      regularWorkspaces: $regularWorkspaces,
      total: $total,
      truncated: $truncated,
    })
  );

  return {
    // Subscribe to combined state
    subscribe: combined.subscribe,

    // Load all workspaces (but not personal workspace - that's loaded on-demand)
    load({ force = false } = {}) {
      if (!force && get(loaded)) return Promise.resolve(get(workspaces));
      if (!force && listLoadPromise) return listLoadPromise;

      const lifecycle = lifecycleGeneration;
      const generation = ++listLoadGeneration;
      loading.set(true);

      const request = api.workspaces
        .getPage({ page: 1, page_size: DIRECTORY_PAGE_SIZE })
        .then((document) => {
          if (lifecycle !== lifecycleGeneration || generation !== listLoadGeneration) return [];

          const nextWorkspaces = document?.data || [];
          workspaces.set(nextWorkspaces);
          total.set(document?.pagination?.total ?? nextWorkspaces.length);
          // Don't set personalWorkspace here - it's loaded on-demand
          loaded.set(true);
          return nextWorkspaces;
        })
        .catch((error) => {
          if (lifecycle !== lifecycleGeneration || generation !== listLoadGeneration) return [];
          if (isExpectedBackgroundSyncError(error)) return [];
          console.error('Failed to load workspaces:', error);
          workspaces.set([]);
          total.set(0);
          loaded.set(true);
          return [];
        })
        .finally(() => {
          if (lifecycle === lifecycleGeneration && generation === listLoadGeneration) {
            loading.set(false);
          }
          if (listLoadPromise === request) listLoadPromise = null;
        });

      listLoadPromise = request;
      return request;
    },

    // Server-backed directory search for rows beyond the cached first page.
    // Matching mirrors the server contract: case-insensitive substring on
    // name, key, and description. Identical concurrent calls share one
    // request; a newer query supersedes older in-flight ones.
    searchWorkspaces(query, { limit = 25 } = {}) {
      const trimmed = String(query ?? '').trim();
      const key = `${limit}\u0000${trimmed}`;
      if (searchPromise?.key === key) return searchPromise.promise;

      const generation = ++searchGeneration;
      const request = api.workspaces
        .getPage({ page: 1, page_size: limit, search: trimmed })
        .then((document) => {
          if (searchPromise?.promise === request) searchPromise = null;
          if (generation !== searchGeneration) return { workspaces: [], total: 0 };
          return {
            workspaces: document?.data || [],
            total: document?.pagination?.total ?? 0,
          };
        })
        .catch((error) => {
          if (searchPromise?.promise === request) searchPromise = null;
          if (generation !== searchGeneration) return { workspaces: [], total: 0 };
          if (!isExpectedBackgroundSyncError(error)) {
            console.error('Failed to search workspaces:', error);
          }
          return { workspaces: [], total: 0 };
        });
      searchPromise = { key, promise: request };
      return request;
    },

    // Load personal workspace on-demand
    loadPersonalWorkspace() {
      const current = get(personalWorkspace);
      if (current) return Promise.resolve(current);
      if (personalLoadPromise) return personalLoadPromise;

      const lifecycle = lifecycleGeneration;
      const generation = ++personalLoadGeneration;
      const request = api.workspaces
        .getOrCreatePersonal()
        .then((personal) => {
          if (lifecycle !== lifecycleGeneration || generation !== personalLoadGeneration) {
            return null;
          }
          personalWorkspace.set(personal);
          return personal;
        })
        .catch((error) => {
          if (lifecycle !== lifecycleGeneration || generation !== personalLoadGeneration) {
            return null;
          }
          if (isExpectedBackgroundSyncError(error)) return null;
          console.error('Failed to load personal workspace:', error);
          return null;
        })
        .finally(() => {
          if (personalLoadPromise === request) personalLoadPromise = null;
        });

      personalLoadPromise = request;
      return request;
    },

    // Force reload from API
    async reload() {
      loaded.set(false);
      loading.set(false);
      await this.load({ force: true });
    },

    // Add a new workspace to the store
    add(workspace) {
      // A list request started before the create cannot know about this row.
      // Invalidate it before applying the POST result so it cannot erase the
      // newly created workspace when its stale response arrives.
      listLoadGeneration += 1;
      listLoadPromise = null;
      loading.set(false);
      workspaces.update((ws) =>
        ws.some((existing) => String(existing.id) === String(workspace.id))
          ? ws.map((existing) =>
              String(existing.id) === String(workspace.id) ? workspace : existing
            )
          : [...ws, workspace]
      );
      // The cache only holds the first page, but the directory total did grow.
      total.update((count) => count + 1);
    },

    // Update an existing workspace in the store
    updateWorkspace(id, updates) {
      const workspaceId = String(id);
      workspaces.update((ws) =>
        ws.map((w) => (String(w.id) === workspaceId ? { ...w, ...updates } : w))
      );
    },

    // Remove a workspace from the store
    remove(id) {
      // A list request started before the delete may still contain this
      // workspace. Invalidate it so its eventual response cannot restore the
      // deleted row after this local mutation.
      listLoadGeneration += 1;
      listLoadPromise = null;
      loading.set(false);
      const workspaceId = String(id);
      const existed = get(workspaces).some((w) => String(w.id) === workspaceId);
      workspaces.update((ws) => ws.filter((w) => String(w.id) !== workspaceId));
      if (existed) total.update((count) => Math.max(0, count - 1));
    },

    // Clear the store
    clear() {
      lifecycleGeneration += 1;
      listLoadGeneration += 1;
      listLoadPromise = null;
      searchGeneration += 1;
      searchPromise = null;
      personalLoadGeneration += 1;
      personalLoadPromise = null;
      workspaces.set([]);
      personalWorkspace.set(null);
      total.set(0);
      loaded.set(false);
      loading.set(false);
    },
  };
}

export const currentWorkspace = createCurrentWorkspaceStore();
export const workspacesStore = createWorkspacesStore();
