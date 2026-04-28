import { fetchAPI } from '../core.js';
import { createCrudClient } from '../createCrudClient.js';

export const testRunTemplates = {
  ...createCrudClient('/test-run-templates', { parentPath: '/workspaces' }),
  getExecutions: (workspaceId, id) =>
    fetchAPI(`/workspaces/${workspaceId}/test-run-templates/${id}/executions`),
  execute: (workspaceId, id) =>
    fetchAPI(`/workspaces/${workspaceId}/test-run-templates/${id}/execute`, {
      method: 'POST',
    }),
};
