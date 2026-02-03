import { fetchAPI } from '../core.js';

export const defects = {
  getAll: () => fetchAPI('/defects'),
  get: (id) => fetchAPI(`/defects/${id}`),
  create: (data) =>
    fetchAPI('/defects', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  update: (id, data) =>
    fetchAPI(`/defects/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  delete: (id) =>
    fetchAPI(`/defects/${id}`, {
      method: 'DELETE',
    }),
  linkToStep: (data) =>
    fetchAPI('/defects/link-to-step', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
};
