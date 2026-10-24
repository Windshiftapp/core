import { attachmentSettings } from '../api/misc.js';

// Create reactive attachment status state
let enabled = $state(true); // Default to true, will be updated on load
let configured = $state(true);
let writable = $state(true);
let loaded = $state(false);
let loading = $state(false);

function applyStatus(status) {
  configured = status?.enabled === true;
  writable = status?.writable === true;
  enabled = configured && writable;
}

// Load attachment status from API (only if not already loaded)
async function load() {
  if (loaded || loading) {
    return;
  }
  loading = true;

  try {
    const status = await attachmentSettings.getStatus();
    applyStatus(status);
    loaded = true;
  } catch (error) {
    console.error('Failed to load attachment status:', error);
    // Default to disabled if we can't get status
    applyStatus(null);
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

function hydrate(status) {
  if (!status) return;
  applyStatus(status);
  loaded = true;
  loading = false;
}

export const attachmentStatus = {
  get enabled() {
    return enabled;
  },
  get configured() {
    return configured;
  },
  get writable() {
    return writable;
  },
  get unavailableReason() {
    if (enabled) return null;
    return configured ? 'unwritable' : 'disabled';
  },
  get loaded() {
    return loaded;
  },
  get loading() {
    return loading;
  },
  load,
  reload,
  hydrate,
};
