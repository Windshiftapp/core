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

// Workspaces store - manages the list of all workspaces
function createWorkspacesStore() {
  const workspaces = writable([]);
  const personalWorkspace = writable(null);
  const loaded = writable(false);
  const loading = writable(false);
  let lifecycleGeneration = 0;
  let listLoadGeneration = 0;
  let listLoadPromise = null;
  let personalLoadGeneration = 0;
  let personalLoadPromise = null;

  // Derived store for regular (non-personal) workspaces
  const regularWorkspaces = derived(workspaces, ($workspaces) =>
    $workspaces.filter((ws) => !ws.is_personal)
  );

  // Create a derived store that combines all state for easy subscription
  const combined = derived(
    [workspaces, personalWorkspace, loaded, loading, regularWorkspaces],
    ([$workspaces, $personalWorkspace, $loaded, $loading, $regularWorkspaces]) => ({
      workspaces: $workspaces,
      allWorkspaces: $workspaces,
      personalWorkspace: $personalWorkspace,
      loaded: $loaded,
      loading: $loading,
      regularWorkspaces: $regularWorkspaces,
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
        .getAll()
        .then((allWorkspaces) => {
          if (lifecycle !== lifecycleGeneration || generation !== listLoadGeneration) return [];

          const nextWorkspaces = allWorkspaces || [];
          workspaces.set(nextWorkspaces);
          // Don't set personalWorkspace here - it's loaded on-demand
          loaded.set(true);
          return nextWorkspaces;
        })
        .catch((error) => {
          if (lifecycle !== lifecycleGeneration || generation !== listLoadGeneration) return [];
          if (isExpectedBackgroundSyncError(error)) return [];
          console.error('Failed to load workspaces:', error);
          workspaces.set([]);
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
      workspaces.update((ws) => ws.filter((w) => String(w.id) !== workspaceId));
    },

    // Clear the store
    clear() {
      lifecycleGeneration += 1;
      listLoadGeneration += 1;
      listLoadPromise = null;
      personalLoadGeneration += 1;
      personalLoadPromise = null;
      workspaces.set([]);
      personalWorkspace.set(null);
      loaded.set(false);
      loading.set(false);
    },
  };
}

export const currentWorkspace = createCurrentWorkspaceStore();
export const workspacesStore = createWorkspacesStore();
