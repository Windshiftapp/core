import { fetchV2Data } from '../core.js';
import { createCrudClient } from '../createCrudClient.js';

export const testRunTemplates = {
  ...createCrudClient('/test-run-templates', { parentPath: '/workspaces', v2: true, allV2: true }),
  getExecutions: (workspaceId, id) =>
    fetchV2Data(`/workspaces/${workspaceId}/test-run-templates/${id}/executions`),
  execute: (workspaceId, id) =>
    fetchV2Data(`/workspaces/${workspaceId}/test-run-templates/${id}/execute`, {
      method: 'POST',
    }),
};
