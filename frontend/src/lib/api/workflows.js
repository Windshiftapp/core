import { fetchAPI } from './core.js';

export const statusCategories = {
  getAll: () => fetchAPI('/status-categories'),
  get: (id) => fetchAPI(`/status-categories/${id}`),
  create: (data) => fetchAPI('/status-categories', {
    method: 'POST',
    body: JSON.stringify(data),
  }),
  update: (id, data) => fetchAPI(`/status-categories/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  }),
  delete: (id) => fetchAPI(`/status-categories/${id}`, {
    method: 'DELETE',
  }),
};

export const statuses = {
  getAll: () => fetchAPI('/statuses'),
  getNonDoneIds: () => fetchAPI('/statuses/non-done-ids'),
  get: (id) => fetchAPI(`/statuses/${id}`),
  create: (data) => fetchAPI('/statuses', {
    method: 'POST',
    body: JSON.stringify(data),
  }),
  update: (id, data) => fetchAPI(`/statuses/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  }),
  delete: (id) => fetchAPI(`/statuses/${id}`, {
    method: 'DELETE',
  }),
};

export const workflows = {
  getAll: () => fetchAPI('/workflows'),
  get: (id) => fetchAPI(`/workflows/${id}`),
  create: (data) => fetchAPI('/workflows', {
    method: 'POST',
    body: JSON.stringify(data),
  }),
  update: (id, data) => fetchAPI(`/workflows/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  }),
  delete: (id) => fetchAPI(`/workflows/${id}`, {
    method: 'DELETE',
  }),
  getTransitions: (id) => fetchAPI(`/workflows/${id}/transitions`),
  updateTransitions: (id, data) => fetchAPI(`/workflows/${id}/transitions`, {
    method: 'PUT',
    body: JSON.stringify(data),
  }),
  getAvailableTransitions: (id, statusId) => fetchAPI(`/workflows/${id}/available-transitions/${statusId}`),
};
