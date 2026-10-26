import { fetchV2Data } from './core.js';

// Persisted named filters (WI-1603 / WI-1730). A queue row scoped to a
// collection — or, without a collection, to the workspace default view — is a
// reusable saved filter that collection views surface as a switchable view.
// Built-in support-queue presets share the table but are filtered out by the
// caller.

function scopePath(workspaceKey, collectionId) {
  if (collectionId) return `/collections/${collectionId}/queues`;
  if (workspaceKey) return `/workspaces/${encodeURIComponent(workspaceKey)}/queues`;
  return null;
}

export const queues = {
  list: (workspaceKey, collectionId) => fetchV2Data(scopePath(workspaceKey, collectionId)),
  create: (workspaceKey, collectionId, data) =>
    fetchV2Data(scopePath(workspaceKey, collectionId), {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  update: (id, data) =>
    fetchV2Data(`/queues/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(data),
    }),
  remove: (id) => fetchV2Data(`/queues/${id}`, { method: 'DELETE' }),
  reorder: (workspaceKey, collectionId, ids) =>
    fetchV2Data(`${scopePath(workspaceKey, collectionId)}/order`, {
      method: 'PUT',
      body: JSON.stringify({ ids }),
    }),
};
