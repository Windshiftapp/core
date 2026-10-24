import { ai } from '../api/ai.js';

let available = $state(false);
let chatEnabled = $state(false);
let features = $state({});
let loaded = $state(false);
let loading = $state(false);

async function load() {
  if (loaded || loading) {
    return;
  }
  loading = true;

  try {
    const result = await ai.status();
    available = result?.available === true;
    chatEnabled = result?.chat_enabled === true;
    features = result?.features ?? {};
    loaded = true;
  } catch (error) {
    console.error('Failed to load AI status:', error);
    available = false;
    chatEnabled = false;
    features = {};
    loaded = true;
  } finally {
    loading = false;
  }
}

async function reload() {
  loaded = false;
  loading = false;
  await load();
}

function hydrate(result) {
  if (!result) return;
  available = result.available === true;
  chatEnabled = result.chat_enabled === true;
  features = result.features ?? {};
  loaded = true;
  loading = false;
}

function isFeatureEnabled(key) {
  const f = features[key];
  if (!f) return true; // default: enabled
  return f.enabled !== false;
}

export const aiStore = {
  get available() {
    return available;
  },
  get chatEnabled() {
    return chatEnabled;
  },
  get chatAvailable() {
    return available && chatEnabled;
  },
  get features() {
    return features;
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
  isFeatureEnabled,
};
