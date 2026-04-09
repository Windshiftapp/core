import { fetchAPI } from './core.js';

export const analytics = {
  getAnalytics: (workspaceId, params = {}) => {
    const search = new URLSearchParams();
    if (params.start_date) search.set('start_date', params.start_date);
    if (params.end_date) search.set('end_date', params.end_date);
    if (params.collection_id) search.set('collection_id', params.collection_id);
    if (params.ql) search.set('ql', params.ql);
    const q = search.toString();
    return fetchAPI(`/workspaces/${workspaceId}/analytics${q ? `?${q}` : ''}`);
  },
};
