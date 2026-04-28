import { fetchAPI } from './core.js';
import { createCrudClient } from './createCrudClient.js';

export const conditionSets = {
  ...createCrudClient('/condition-sets'),
  getByWorkflow: (workflowId) => fetchAPI(`/workflows/${workflowId}/condition-sets`),
};
