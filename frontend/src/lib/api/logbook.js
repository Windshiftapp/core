import { API_BASE, fetchAPI } from './core.js';
import { buildQueryString } from './utils.js';

/**
 * Creates a multipart form upload handler with consistent error handling.
 * @param {string} endpoint - The API endpoint template with {paramName} placeholders
 * @param {Object} paramMap - Maps placeholder names to actual values (e.g., { bucketId: 123 })
 * @returns {Promise<any>} Parsed response or null
 */
async function createUploadHandler(endpoint, paramMap = {}) {
  let resolvedEndpoint = endpoint;
  for (const [key, value] of Object.entries(paramMap)) {
    resolvedEndpoint = resolvedEndpoint.replace(`{${key}}`, value);
  }
  return async (formData) => {
    const response = await fetch(`${API_BASE}${resolvedEndpoint}`, {
      method: 'POST',
      body: formData,
      credentials: 'same-origin',
    });
    if (!response.ok) {
      let errorData = '';
      try {
        errorData = await response.text();
      } catch (_e) {
        // ignore
      }
      const error = new Error(errorData || `Upload failed: ${response.statusText}`);
      /** @type {any} */ (error).status = response.status;
      throw error;
    }
    if (response.status === 204 || response.status === 202) {
      return null;
    }
    const contentType = response.headers.get('content-type');
    if (contentType?.includes('application/json')) {
      return response.json();
    }
    return null;
  };
}

export const logbook = {
  // Health check (determines availability)
  health: () => fetchAPI('/logbook/health'),

  // Buckets
  getBuckets: () => fetchAPI('/logbook/buckets'),
  getBucket: (id) => fetchAPI(`/logbook/buckets/${id}`),
  createBucket: (data) =>
    fetchAPI('/logbook/buckets', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  updateBucket: (id, data) =>
    fetchAPI(`/logbook/buckets/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  deleteBucket: (id) =>
    fetchAPI(`/logbook/buckets/${id}`, {
      method: 'DELETE',
    }),

  // Documents
  listDocuments: (bucketId, params = {}) => {
    return fetchAPI(`/logbook/buckets/${bucketId}/documents${buildQueryString(params)}`);
  },
  listAllDocuments: (params = {}) => {
    return fetchAPI(`/logbook/documents${buildQueryString(params)}`);
  },
  listDocumentsByOrganisation: (customerOrgId, params = {}) => {
    return logbook.listAllDocuments({ ...params, customer_organisation_id: customerOrgId });
  },
  getDocument: (id) => fetchAPI(`/logbook/documents/${id}`),
  updateDocument: (id, data) =>
    fetchAPI(`/logbook/documents/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  archiveDocument: (id) =>
    fetchAPI(`/logbook/documents/${id}`, {
      method: 'DELETE',
    }),
  uploadDocument: (bucketId, formData) =>
    createUploadHandler('/logbook/buckets/{bucketId}/documents/upload', { bucketId })(formData),
  createNote: (bucketId, data) =>
    fetchAPI(`/logbook/buckets/${bucketId}/documents/notes`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  // Attachments
  uploadAttachment: (documentId, formData) =>
    createUploadHandler('/logbook/documents/{documentId}/attachments', { documentId })(formData),

  // Thumbnails
  getDocumentThumbnailUrl: (documentId) => `${API_BASE}/logbook/documents/${documentId}/thumbnail`,
  getDocumentPreviewUrl: (documentId) => `${API_BASE}/logbook/documents/${documentId}/preview`,
  getDocumentFileUrl: (documentId) => `${API_BASE}/logbook/documents/${documentId}/file`,

  // Search
  keywordSearch: (q, params = {}) => {
    return fetchAPI(`/logbook/search${buildQueryString({ q, ...params })}`);
  },
};
