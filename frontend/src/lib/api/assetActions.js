import { fetchAPI } from './core.js';

export const assetActions = {
  getAll: (setId) => fetchAPI(`/asset-sets/${setId}/actions`),

  get: (setId, id) => fetchAPI(`/asset-sets/${setId}/actions/${id}`),

  create: (setId, data) =>
    fetchAPI(`/asset-sets/${setId}/actions`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  update: (setId, id, data) =>
    fetchAPI(`/asset-sets/${setId}/actions/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  delete: (setId, id) =>
    fetchAPI(`/asset-sets/${setId}/actions/${id}`, {
      method: 'DELETE',
    }),

  toggle: (setId, id) =>
    fetchAPI(`/asset-sets/${setId}/actions/${id}/toggle`, {
      method: 'POST',
    }),

  execute: (setId, actionId, assetId) =>
    fetchAPI(`/asset-sets/${setId}/actions/${actionId}/execute`, {
      method: 'POST',
      body: JSON.stringify({ asset_id: assetId }),
    }),

  getLogs: (setId, actionId) =>
    fetchAPI(`/asset-sets/${setId}/actions/${actionId}/logs`),

  getSetLogs: (setId) => fetchAPI(`/asset-sets/${setId}/action-logs`),
};
