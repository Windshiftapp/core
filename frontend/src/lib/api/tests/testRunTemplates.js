import { fetchAPI } from '../core.js';

export const testRunTemplates = {
  getAll: (workspaceId) => fetchAPI(`/workspaces/${workspaceId}/test-run-templates`),
  get: (workspaceId, id) => fetchAPI(`/workspaces/${workspaceId}/test-run-templates/${id}`),
  create: (workspaceId, data) => fetchAPI(`/workspaces/${workspaceId}/test-run-templates`, {
    method: 'POST',
    body: JSON.stringify(data),
  }),
  update: (workspaceId, id, data) => fetchAPI(`/workspaces/${workspaceId}/test-run-templates/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  }),
  delete: (workspaceId, id) => fetchAPI(`/workspaces/${workspaceId}/test-run-templates/${id}`, {
    method: 'DELETE',
  }),
  getExecutions: (workspaceId, id) => fetchAPI(`/workspaces/${workspaceId}/test-run-templates/${id}/executions`),
  execute: (workspaceId, id) => fetchAPI(`/workspaces/${workspaceId}/test-run-templates/${id}/execute`, {
    method: 'POST',
  }),
};
