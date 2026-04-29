import { fetchAPI } from './core.js';

export const leave = {
  list: (userId) => fetchAPI(`/users/${userId}/leave`),

  create: (userId, data) =>
    fetchAPI(`/users/${userId}/leave`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  update: (userId, leaveId, data) =>
    fetchAPI(`/users/${userId}/leave/${leaveId}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  delete: (userId, leaveId) =>
    fetchAPI(`/users/${userId}/leave/${leaveId}`, {
      method: 'DELETE',
    }),
};
