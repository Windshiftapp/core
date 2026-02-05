import { api } from '../api.js';

let available = $state(false);
let loaded = $state(false);
let loading = $state(false);

async function load() {
  if (loaded || loading) {
    return;
  }
  loading = true;

  try {
    const result = await api.ai.status();
    available = result?.available === true;
    loaded = true;
  } catch (error) {
    console.error('Failed to load AI status:', error);
    available = false;
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

export const aiStore = {
  get available() {
    return available;
  },
  get loaded() {
    return loaded;
  },
  get loading() {
    return loading;
  },
  load,
  reload,
};
