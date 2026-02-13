import { api } from '../api.js';

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

let activeDocument = $state(null);
let activeDocumentLoading = $state(false);

async function checkAvailability() {
  if (loaded || loading) return;
  loading = true;
  try {
    await api.logbook.health();
    available = true;
    loaded = true;
  } catch {
    available = false;
    loaded = true;
  } finally {
    loading = false;
  }
}

async function loadBuckets() {
  if (bucketsLoading) return;
  bucketsLoading = true;
  try {
    const result = await api.logbook.getBuckets();
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
  activeBucketId = bucketId;
  if (!silent) documentsLoading = true;
  try {
    const result = await api.logbook.listDocuments(bucketId, params);
    if (Array.isArray(result)) {
      documents = result;
      totalDocuments = result.length;
    } else if (result && result.data) {
      documents = result.data;
      totalDocuments = result.pagination?.total ?? result.data.length;
    } else {
      documents = [];
      totalDocuments = 0;
    }
  } catch (error) {
    console.error('Failed to load documents:', error);
    documents = [];
    totalDocuments = 0;
  } finally {
    documentsLoading = false;
  }
}

async function loadAllDocuments(params = {}, { silent = false } = {}) {
  activeBucketId = null;
  if (!silent) documentsLoading = true;
  try {
    const result = await api.logbook.listAllDocuments(params);
    if (Array.isArray(result)) {
      documents = result;
      totalDocuments = result.length;
    } else if (result && result.data) {
      documents = result.data;
      totalDocuments = result.pagination?.total ?? result.data.length;
    } else {
      documents = [];
      totalDocuments = 0;
    }
  } catch (error) {
    console.error('Failed to load all documents:', error);
    documents = [];
    totalDocuments = 0;
  } finally {
    documentsLoading = false;
  }
}

async function loadDocument(documentId, { silent = false } = {}) {
  if (!silent) activeDocumentLoading = true;
  try {
    activeDocument = await api.logbook.getDocument(documentId);
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
  get available() { return available; },
  get loaded() { return loaded; },
  get loading() { return loading; },

  get buckets() { return buckets; },
  get bucketsLoaded() { return bucketsLoaded; },
  get bucketsLoading() { return bucketsLoading; },

  get activeBucketId() { return activeBucketId; },
  set activeBucketId(v) { activeBucketId = v; },
  get documents() { return documents; },
  get totalDocuments() { return totalDocuments; },
  get documentsLoading() { return documentsLoading; },

  get activeDocument() { return activeDocument; },
  get activeDocumentLoading() { return activeDocumentLoading; },

  checkAvailability,
  loadBuckets,
  loadDocuments,
  loadAllDocuments,
  loadDocument,
  clearActiveDocument,
};
