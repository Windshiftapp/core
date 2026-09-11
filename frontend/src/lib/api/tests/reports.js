import { fetchV2Data } from '../core.js';
import { buildQueryString } from '../utils.js';

// Test reports dashboard
export const reports = {
  getSummary: (workspaceId, options = {}) => {
    const mapped = {};
    if (options.milestoneId) mapped.milestone_id = options.milestoneId;
    if (options.days) mapped.days = options.days;
    return fetchV2Data(
      `/workspaces/${workspaceId}/test-reports/summary${buildQueryString(mapped)}`
    );
  },
};
