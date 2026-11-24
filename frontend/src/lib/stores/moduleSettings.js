import { get, writable } from 'svelte/store';
import { setup } from '../api/admin.js';

const DEFAULT_SETTINGS = {
  time_tracking_enabled: true,
  test_management_enabled: true,
  workspace_managed_agents: true,
};

// Create the module settings store with default values
function createModuleSettingsStore() {
  const store = writable({
    ...DEFAULT_SETTINGS,
    loaded: false,
    loading: false,
  });
  const { subscribe, set, update } = store;
  // Shared in-flight request so concurrent ensureLoaded() callers fetch once.
  let inFlight = null;
  // Bumped whenever a newer request, reload, or hydration takes ownership.
  // Only the request that still owns the generation may publish or clear
  // itself, so a slow GET cannot overwrite a forced reload or a shell snapshot.
  let generation = 0;

  return {
    subscribe,

    hydrate(settings) {
      if (!settings) return;
      // The shell snapshot supersedes any pending GET.
      generation += 1;
      inFlight = null;
      set({
        ...settings,
        loaded: true,
        loading: false,
      });
    },

    // Load module settings once. The shell hydrates this store during
    // authenticated bootstrap, so this is normally a no-op; it exists for
    // pre-bootstrap callers. A failed fetch stays unloaded so the next call
    // retries instead of serving defaults as if they were the server value.
    async ensureLoaded() {
      if (get(store).loaded) return;
      if (inFlight) return inFlight;
      const requestGeneration = ++generation;
      update((state) => ({ ...state, loading: true }));
      const request = (async () => {
        try {
          const settings = await setup.getModuleSettings();
          if (requestGeneration !== generation) return;
          set({
            ...settings,
            loaded: true,
            loading: false,
          });
        } catch (error) {
          console.error('Failed to load module settings:', error);
          if (requestGeneration !== generation) return;
          update((state) => ({ ...state, loading: false }));
        } finally {
          if (requestGeneration === generation) inFlight = null;
        }
      })();
      inFlight = request;
      return request;
    },

    // Update module settings both in store and API
    async update(newSettings) {
      try {
        await setup.updateModuleSettings(newSettings);
        update((state) => ({
          ...state,
          ...newSettings,
        }));
      } catch (error) {
        console.error('Failed to update module settings:', error);
        throw error;
      }
    },

    // Force reload from API. Supersede any in-flight GET so its late response
    // cannot restore the previous value.
    async reload() {
      generation += 1;
      inFlight = null;
      set({
        ...DEFAULT_SETTINGS,
        loaded: false,
        loading: false,
      });
      await this.ensureLoaded();
    },
  };
}

export const moduleSettings = createModuleSettingsStore();
