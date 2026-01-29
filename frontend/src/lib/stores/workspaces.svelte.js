import { writable, derived } from 'svelte/store';
import { api } from '../api.js';

// Current workspace store - automatically syncs with route
function createCurrentWorkspaceStore() {
  const { subscribe, set, update } = writable(null);
  let lastWorkspaceId = null;

  return {
    subscribe,

    // Patch workspace with partial updates (no API call)
    patch(updates) {
      update(ws => ws ? { ...ws, ...updates } : null);
    },

    // Load workspace by ID
    async load(workspaceId) {
      if (!workspaceId) {
        set(null);
        lastWorkspaceId = null;
        return;
      }

      // Avoid unnecessary API calls if workspace ID hasn't changed
      if (workspaceId === lastWorkspaceId) {
        return;
      }

      lastWorkspaceId = workspaceId;

      try {
        const workspace = await api.workspaces.get(workspaceId);
        set(workspace);
      } catch (error) {
        console.error('Failed to load workspace:', error);
        set(null);
      }
    },

    // Clear workspace
    clear() {
      set(null);
      lastWorkspaceId = null;
    }
  };
}

// Workspaces store - manages the list of all workspaces
function createWorkspacesStore() {
  const workspaces = writable([]);
  const personalWorkspace = writable(null);
  const loaded = writable(false);
  const loading = writable(false);

  // Derived store for regular (non-personal) workspaces
  const regularWorkspaces = derived(workspaces, $workspaces =>
    $workspaces.filter(ws => !ws.is_personal)
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
      regularWorkspaces: $regularWorkspaces
    })
  );

  return {
    // Subscribe to combined state
    subscribe: combined.subscribe,

    // Load all workspaces (but not personal workspace - that's loaded on-demand)
    async load() {
      loading.set(true);

      try {
        const allWorkspaces = await api.workspaces.getAll();

        workspaces.set(allWorkspaces || []);
        // Don't set personalWorkspace here - it's loaded on-demand
        loaded.set(true);
        loading.set(false);
      } catch (error) {
        console.error('Failed to load workspaces:', error);
        workspaces.set([]);
        loaded.set(true);
        loading.set(false);
      }
    },

    // Load personal workspace on-demand
    async loadPersonalWorkspace() {
      try {
        const personal = await api.workspaces.getOrCreatePersonal();
        personalWorkspace.set(personal);
        return personal;
      } catch (error) {
        console.error('Failed to load personal workspace:', error);
        return null;
      }
    },

    // Force reload from API
    async reload() {
      loaded.set(false);
      loading.set(false);
      await this.load();
    },

    // Add a new workspace to the store
    add(workspace) {
      workspaces.update(ws => [...ws, workspace]);
    },

    // Update an existing workspace in the store
    updateWorkspace(id, updates) {
      workspaces.update(ws =>
        ws.map(w => w.id === id ? { ...w, ...updates } : w)
      );
    },

    // Remove a workspace from the store
    remove(id) {
      workspaces.update(ws => ws.filter(w => w.id !== id));
    },

    // Clear the store
    clear() {
      workspaces.set([]);
      personalWorkspace.set(null);
      loaded.set(false);
      loading.set(false);
    }
  };
}

export const currentWorkspace = createCurrentWorkspaceStore();
export const workspacesStore = createWorkspacesStore();
