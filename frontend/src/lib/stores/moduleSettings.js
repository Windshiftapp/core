import { writable } from 'svelte/store';
import { api } from '../api.js';

// Create the module settings store with default values
function createModuleSettingsStore() {
  const { subscribe, set, update } = writable({
    time_tracking_enabled: true,
    test_management_enabled: true,
    loaded: false,
    loading: false
  });

  return {
    subscribe,
    
    // Load module settings from API (only if not already loaded)
    async load() {
      update(state => {
        if (state.loaded || state.loading) {
          return state; // Already loaded or loading
        }
        return { ...state, loading: true };
      });
      
      try {
        const settings = await api.setup.getModuleSettings();
        set({
          ...settings,
          loaded: true,
          loading: false
        });
      } catch (error) {
        console.error('Failed to load module settings:', error);
        // Keep default values but mark as loaded to prevent retrying constantly
        update(state => ({
          ...state,
          loaded: true,
          loading: false
        }));
      }
    },

    // Update module settings both in store and API
    async update(newSettings) {
      try {
        await api.setup.updateModuleSettings(newSettings);
        update(state => ({
          ...state,
          ...newSettings
        }));
      } catch (error) {
        console.error('Failed to update module settings:', error);
        throw error;
      }
    },

    // Force reload from API
    async reload() {
      set({
        time_tracking_enabled: true,
        test_management_enabled: true,
        loaded: false,
        loading: false
      });
      await this.load();
    }
  };
}

export const moduleSettings = createModuleSettingsStore();