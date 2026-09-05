import { api } from '../api.js';

let context = {
  getKnowledgeBaseShareLink: () => '',
  getSlug: () => null,
};

let query = $state('');
let visible = $state(false);
let results = $state(/** @type {any} */ (null));
let loading = $state(false);
let error = $state(null);
let debounceTimer = null;
let searchId = 0;

export function configurePortalSearchStore(nextContext) {
  context = nextContext;
}

async function search() {
  const normalizedQuery = query.trim();
  if (!normalizedQuery) return;

  if (!context.getKnowledgeBaseShareLink()) {
    error = 'Knowledge base not configured';
    visible = true;
    return;
  }

  const slug = context.getSlug();
  const currentSearchId = ++searchId;
  loading = true;
  error = null;
  results = null;
  visible = true;
  try {
    const response = await api.portal.searchKnowledgeBase(slug, normalizedQuery);
    if (currentSearchId === searchId) results = response;
  } catch (err) {
    if (currentSearchId !== searchId) return;
    console.error('Failed to search knowledge base:', err);
    error = err.message || 'Failed to search knowledge base';
  } finally {
    if (currentSearchId === searchId) loading = false;
  }
}

function searchDebounced() {
  if (debounceTimer) clearTimeout(debounceTimer);
  if (query.length < 3) {
    visible = false;
    return;
  }
  debounceTimer = setTimeout(search, 400);
}

function close() {
  visible = false;
}

function reset() {
  searchId++;
  if (debounceTimer) clearTimeout(debounceTimer);
  debounceTimer = null;
  query = '';
  visible = false;
  results = null;
  loading = false;
  error = null;
}

export const portalSearchStore = {
  get query() {
    return query;
  },
  set query(value) {
    query = value;
  },
  get visible() {
    return visible;
  },
  get results() {
    return results;
  },
  get loading() {
    return loading;
  },
  get error() {
    return error;
  },
  search,
  searchDebounced,
  close,
  reset,
};
