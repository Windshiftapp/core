import { fetchAPI, API_BASE } from './core.js';

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
    const query = new URLSearchParams();
    Object.entries(params).forEach(([key, value]) => {
      if (value !== null && value !== undefined && value !== '') {
        query.append(key, value);
      }
    });
    const queryString = query.toString();
    return fetchAPI(`/logbook/buckets/${bucketId}/documents${queryString ? `?${queryString}` : ''}`);
  },
  listAllDocuments: (params = {}) => {
    const query = new URLSearchParams();
    Object.entries(params).forEach(([key, value]) => {
      if (value !== null && value !== undefined && value !== '') {
        query.append(key, value);
      }
    });
    const queryString = query.toString();
    return fetchAPI(`/logbook/documents${queryString ? `?${queryString}` : ''}`);
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
  uploadDocument: async (bucketId, formData) => {
    const response = await fetch(`${API_BASE}/logbook/buckets/${bucketId}/documents/upload`, {
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
      error.status = response.status;
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
  },
  createNote: (bucketId, data) =>
    fetchAPI(`/logbook/buckets/${bucketId}/documents/notes`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  // Attachments
  uploadAttachment: async (documentId, formData) => {
    const response = await fetch(`${API_BASE}/logbook/documents/${documentId}/attachments`, {
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
      error.status = response.status;
      throw error;
    }
    return response.json();
  },

  // Thumbnails
  getDocumentThumbnailUrl: (documentId) => `${API_BASE}/logbook/documents/${documentId}/thumbnail`,
  getDocumentFileUrl: (documentId) => `${API_BASE}/logbook/documents/${documentId}/file`,

  // Search
  keywordSearch: (q, params = {}) => {
    const query = new URLSearchParams({ q });
    Object.entries(params).forEach(([key, value]) => {
      if (value !== null && value !== undefined && value !== '') {
        query.append(key, value);
      }
    });
    return fetchAPI(`/logbook/search?${query.toString()}`);
  },
};
