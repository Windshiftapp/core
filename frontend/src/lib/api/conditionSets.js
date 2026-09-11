import { fetchV2Data } from './core.js';
import { createCrudClient } from './createCrudClient.js';

export const conditionSets = {
  ...createCrudClient('/condition-sets', { v2: true, allV2: true }),
  getByWorkflow: (workflowId) => fetchV2Data(`/workflows/${workflowId}/condition-sets`),
};
