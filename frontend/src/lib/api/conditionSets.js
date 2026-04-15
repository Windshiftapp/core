import { fetchAPI } from './core.js';
import { buildQueryString } from './utils.js';

export const conditionSets = {
  getAll: (filters = {}) => {
    return fetchAPI(`/condition-sets${buildQueryString(filters)}`);
  },
  get: (id) => fetchAPI(`/condition-sets/${id}`),
  create: (data) =>
    fetchAPI('/condition-sets', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  update: (id, data) =>
    fetchAPI(`/condition-sets/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  delete: (id) =>
    fetchAPI(`/condition-sets/${id}`, {
      method: 'DELETE',
    }),
  getByWorkflow: (workflowId) => fetchAPI(`/workflows/${workflowId}/condition-sets`),
};
