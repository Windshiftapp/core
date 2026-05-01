import { fetchAPI } from './core.js';
import { createCrudClient } from './createCrudClient.js';

// Approval-set CRUD (admin). Mirrors conditionSets.js exactly — same shape,
// same lifecycle, same nested set_statuses + steps payload.
export const approvalSets = {
  ...createCrudClient('/approval-sets'),
  getByWorkflow: (workflowId) => fetchAPI(`/workflows/${workflowId}/approval-sets`),
};
