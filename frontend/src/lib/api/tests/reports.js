import { fetchAPI } from '../core.js';

// Test reports dashboard
export const reports = {
  getSummary: (workspaceId, options = {}) => {
    const params = new URLSearchParams();
    if (options.milestoneId) params.append('milestone_id', options.milestoneId);
    if (options.days) params.append('days', options.days);
    const queryString = params.toString();
    return fetchAPI(
      `/workspaces/${workspaceId}/test-reports/summary${queryString ? `?${queryString}` : ''}`
    );
  },
};
