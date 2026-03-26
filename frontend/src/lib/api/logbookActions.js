import { fetchAPI } from './core.js';

export const logbookActions = {
  getAll: (bucketId) => fetchAPI(`/logbook/buckets/${bucketId}/actions`),

  get: (bucketId, id) => fetchAPI(`/logbook/buckets/${bucketId}/actions/${id}`),

  create: (bucketId, data) =>
    fetchAPI(`/logbook/buckets/${bucketId}/actions`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  update: (bucketId, id, data) =>
    fetchAPI(`/logbook/buckets/${bucketId}/actions/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  delete: (bucketId, id) =>
    fetchAPI(`/logbook/buckets/${bucketId}/actions/${id}`, {
      method: 'DELETE',
    }),

  toggle: (bucketId, id) =>
    fetchAPI(`/logbook/buckets/${bucketId}/actions/${id}/toggle`, {
      method: 'POST',
    }),

  execute: (bucketId, actionId, documentId) =>
    fetchAPI(`/logbook/buckets/${bucketId}/actions/${actionId}/execute`, {
      method: 'POST',
      body: JSON.stringify({ document_id: documentId }),
    }),

  getLogs: (bucketId, actionId) =>
    fetchAPI(`/logbook/buckets/${bucketId}/actions/${actionId}/logs`),

  getBucketLogs: (bucketId) => fetchAPI(`/logbook/buckets/${bucketId}/action-logs`),
};
