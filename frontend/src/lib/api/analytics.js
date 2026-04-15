import { fetchAPI } from './core.js';
import { buildQueryString } from './utils.js';

export const analytics = {
  getAnalytics: (workspaceId, params = {}) => {
    return fetchAPI(`/workspaces/${workspaceId}/analytics${buildQueryString(params)}`);
  },
};
