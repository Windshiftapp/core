import { withCollectionInvalidation } from '../utils/collectionChanges.js';
import { fetchAPIV2, fetchV2Data } from './core.js';

export const boardPublications = {
  /** @param {Record<string, unknown>} filters
   * @param {{admin?: boolean, signal?: AbortSignal}} options */
  list(filters = {}, { admin = false, ...options } = {}) {
    const query = new URLSearchParams();
    for (const [key, value] of Object.entries(filters)) {
      if (value !== '' && value != null) query.set(key, String(value));
    }
    return fetchAPIV2(`${admin ? '/admin' : ''}/board-publications?${query}`, options);
  },
  get: (id, options = {}) => fetchV2Data(`/board-publications/${id}`, options),
  history: (id, options = {}) => fetchV2Data(`/board-publications/${id}/history`, options),
  save: (id, draft) =>
    withCollectionInvalidation(
      fetchV2Data(`/board-publications/${id}`, { method: 'PUT', body: JSON.stringify(draft) })
    ),
  revoke: (id, revision, reason) =>
    withCollectionInvalidation(
      fetchV2Data(`/board-publications/${id}/revoke`, {
        method: 'POST',
        body: JSON.stringify({ revision, reason }),
      })
    ),
  allowRepublish: (id, revision) =>
    withCollectionInvalidation(
      fetchV2Data(`/board-publications/${id}/allow-republish`, {
        method: 'POST',
        body: JSON.stringify({ revision }),
      })
    ),
};
