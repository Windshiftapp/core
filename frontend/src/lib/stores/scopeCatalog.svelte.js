import { api } from '../api.js';

let catalog = $state([]);
let loading = $state(false);
let loaded = $state(false);
let error = $state(null);
let loadPromise = null;
let loadId = 0;

async function load({ force = false } = {}) {
  if (loaded && !force) return catalog;
  if (loadPromise && !force) return loadPromise;

  loading = true;
  error = null;
  const currentLoadId = ++loadId;
  loadPromise = Promise.resolve()
    .then(() => api.getScopeCatalog?.())
    .then((result) => {
      if (currentLoadId !== loadId) return catalog;
      catalog = result || [];
      loaded = true;
      return catalog;
    })
    .catch((err) => {
      if (currentLoadId !== loadId) return catalog;
      console.warn('Failed to load scope catalog:', err);
      catalog = [];
      loaded = false;
      error = err;
      return catalog;
    })
    .finally(() => {
      if (currentLoadId === loadId) {
        loading = false;
        loadPromise = null;
      }
    });
  return loadPromise;
}

function reset() {
  loadId++;
  catalog = [];
  loading = false;
  loaded = false;
  error = null;
  loadPromise = null;
}

export const scopeCatalogStore = {
  get catalog() {
    return catalog;
  },
  get loading() {
    return loading;
  },
  get loaded() {
    return loaded;
  },
  get error() {
    return error;
  },
  load,
  reset,
};
