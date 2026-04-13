import { fetchAPI } from './core.js';

export const conditionSets = {
  getAll: (filters = {}) => {
    const params = new URLSearchParams();
    Object.entries(filters).forEach(([key, value]) => {
      if (value != null) params.append(key, value);
    });
    const qs = params.toString();
    return fetchAPI(`/condition-sets${qs ? `?${qs}` : ''}`);
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
