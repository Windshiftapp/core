import { fetchAPI } from './core.js';
import { createCrudClient } from './createCrudClient.js';

export const statusCategories = createCrudClient('/status-categories');

export const statuses = {
  ...createCrudClient('/statuses'),
  getNonDoneIds: () => fetchAPI('/statuses/non-done-ids'),
};

export const workflows = {
  ...createCrudClient('/workflows'),
  getTransitions: (id) => fetchAPI(`/workflows/${id}/transitions`),
  updateTransitions: (id, data) =>
    fetchAPI(`/workflows/${id}/transitions`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  getAvailableTransitions: (id, statusId) =>
    fetchAPI(`/workflows/${id}/available-transitions/${statusId}`),
};
