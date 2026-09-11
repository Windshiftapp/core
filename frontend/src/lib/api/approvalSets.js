import { fetchV2Data } from './core.js';
import { createCrudClient } from './createCrudClient.js';

// Approval-set CRUD (admin). Mirrors conditionSets.js exactly — same shape,
// same lifecycle, same nested set_statuses + steps payload.
export const approvalSets = {
  ...createCrudClient('/approval-sets', { v2: true, allV2: true }),
  getByWorkflow: (workflowId) => fetchV2Data(`/workflows/${workflowId}/approval-sets`),
};
