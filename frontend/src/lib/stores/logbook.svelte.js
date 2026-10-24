import { logbook } from '../api/logbook.js';

function parseDocumentResult(result) {
  if (Array.isArray(result)) {
    return { docs: result, total: result.length };
  } else if (result?.data) {
    return { docs: result.data, total: result.pagination?.total ?? result.data.length };
  }
  return { docs: [], total: 0 };
}

let available = $state(false);
let loaded = $state(false);
let loading = $state(false);

let buckets = $state([]);
let bucketsLoaded = $state(false);
let bucketsLoading = $state(false);

let activeBucketId = $state(null);
let documents = $state([]);
let totalDocuments = $state(0);
let documentsLoading = $state(false);
// Both document loaders share one generation so a slower response from a
// previous bucket, query or refresh can never overwrite the current list.
let documentsLoadId = 0;

let activeDocument = $state(null);
let activeDocumentLoading = $state(false);

async function checkAvailability() {
  if (loaded || loading) return;
  loading = true;
  try {
    await logbook.health();
    available = true;
    loaded = true;
  } catch {
    available = false;
    loaded = true;
  } finally {
    loading = false;
  }
}

function hydrateAvailability(value) {
  available = value === true;
  loaded = true;
  loading = false;
}

async function loadBuckets() {
  if (bucketsLoading) return;
  bucketsLoading = true;
  try {
    const result = await logbook.getBuckets();
    buckets = result || [];
    bucketsLoaded = true;
  } catch (error) {
    console.error('Failed to load buckets:', error);
    buckets = [];
  } finally {
    bucketsLoading = false;
  }
}

async function loadDocuments(bucketId, params = {}, { silent = false } = {}) {
  const loadId = ++documentsLoadId;
  activeBucketId = bucketId;
  if (!silent) documentsLoading = true;
  try {
    const result = await logbook.listDocuments(bucketId, params);
    if (loadId !== documentsLoadId) return;
    const parsed = parseDocumentResult(result);
    documents = parsed.docs;
    totalDocuments = parsed.total;
  } catch (error) {
    if (loadId !== documentsLoadId) return;
    console.error('Failed to load documents:', error);
    documents = [];
    totalDocuments = 0;
  } finally {
    if (loadId === documentsLoadId) documentsLoading = false;
  }
}

async function loadAllDocuments(params = {}, { silent = false } = {}) {
  const loadId = ++documentsLoadId;
  activeBucketId = null;
  if (!silent) documentsLoading = true;
  try {
    const result = await logbook.listAllDocuments(params);
    if (loadId !== documentsLoadId) return;
    const parsed = parseDocumentResult(result);
    documents = parsed.docs;
    totalDocuments = parsed.total;
  } catch (error) {
    if (loadId !== documentsLoadId) return;
    console.error('Failed to load all documents:', error);
    documents = [];
    totalDocuments = 0;
  } finally {
    if (loadId === documentsLoadId) documentsLoading = false;
  }
}

/** Drop in-flight document loads and their state, e.g. on session change. */
function resetDocuments() {
  documentsLoadId += 1;
  activeBucketId = null;
  documents = [];
  totalDocuments = 0;
  documentsLoading = false;
}

async function loadDocument(documentId, { silent = false } = {}) {
  if (!silent) activeDocumentLoading = true;
  try {
    activeDocument = await logbook.getDocument(documentId);
  } catch (error) {
    console.error('Failed to load document:', error);
    activeDocument = null;
  } finally {
    activeDocumentLoading = false;
  }
}

function clearActiveDocument() {
  activeDocument = null;
}

export const logbookStore = {
  get available() {
    return available;
  },
  get loaded() {
    return loaded;
  },
  get loading() {
    return loading;
  },

  get buckets() {
    return buckets;
  },
  get bucketsLoaded() {
    return bucketsLoaded;
  },
  get bucketsLoading() {
    return bucketsLoading;
  },

  get activeBucketId() {
    return activeBucketId;
  },
  set activeBucketId(v) {
    activeBucketId = v;
  },
  get documents() {
    return documents;
  },
  get totalDocuments() {
    return totalDocuments;
  },
  get documentsLoading() {
    return documentsLoading;
  },

  get activeDocument() {
    return activeDocument;
  },
  get activeDocumentLoading() {
    return activeDocumentLoading;
  },

  checkAvailability,
  hydrateAvailability,
  loadBuckets,
  loadDocuments,
  loadAllDocuments,
  loadDocument,
  clearActiveDocument,
  resetDocuments,
};
