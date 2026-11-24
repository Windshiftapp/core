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

  return {
    subscribe,

    hydrate(settings) {
      if (!settings) return;
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
      update((state) => ({ ...state, loading: true }));
      inFlight = (async () => {
        try {
          const settings = await setup.getModuleSettings();
          set({
            ...settings,
            loaded: true,
            loading: false,
          });
        } catch (error) {
          console.error('Failed to load module settings:', error);
          update((state) => ({ ...state, loading: false }));
        } finally {
          inFlight = null;
        }
      })();
      return inFlight;
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

    // Force reload from API
    async reload() {
      set({
        ...DEFAULT_SETTINGS,
        loaded: false,
        loading: false,
      });
      inFlight = null;
      await this.ensureLoaded();
    },
  };
}

export const moduleSettings = createModuleSettingsStore();
