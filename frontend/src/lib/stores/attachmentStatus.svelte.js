import { api } from '../api.js';

// Create reactive attachment status state
let enabled = $state(true); // Default to true, will be updated on load
let loaded = $state(false);
let loading = $state(false);

// Load attachment status from API (only if not already loaded)
async function load() {
  if (loaded || loading) {
    return;
  }
  loading = true;

  try {
    const status = await api.attachmentSettings.getStatus();
    enabled = status.enabled && status.writable;
    loaded = true;
  } catch (error) {
    console.error('Failed to load attachment status:', error);
    // Default to disabled if we can't get status
    enabled = false;
    loaded = true;
  } finally {
    loading = false;
  }
}

// Force reload from API
async function reload() {
  loaded = false;
  loading = false;
  await load();
}

export const attachmentStatus = {
  get enabled() { return enabled; },
  get loaded() { return loaded; },
  get loading() { return loading; },
  load,
  reload
};
