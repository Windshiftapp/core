import { fetchAPI } from './core.js';

export const onCallSchedules = {
  listForTeam: (teamId) => fetchAPI(`/teams/${teamId}/on-call/schedules`),

  createForTeam: (teamId, data) =>
    fetchAPI(`/teams/${teamId}/on-call/schedules`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  get: (scheduleId) => fetchAPI(`/on-call/schedules/${scheduleId}`),

  update: (scheduleId, data) =>
    fetchAPI(`/on-call/schedules/${scheduleId}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  delete: (scheduleId) =>
    fetchAPI(`/on-call/schedules/${scheduleId}`, {
      method: 'DELETE',
    }),

  getCurrent: (scheduleId) => fetchAPI(`/on-call/schedules/${scheduleId}/current`),

  addLayer: (scheduleId, data) =>
    fetchAPI(`/on-call/schedules/${scheduleId}/layers`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  updateLayer: (scheduleId, layerId, data) =>
    fetchAPI(`/on-call/schedules/${scheduleId}/layers/${layerId}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  deleteLayer: (scheduleId, layerId) =>
    fetchAPI(`/on-call/schedules/${scheduleId}/layers/${layerId}`, {
      method: 'DELETE',
    }),

  setLayerMembers: (scheduleId, layerId, userIds) =>
    fetchAPI(`/on-call/schedules/${scheduleId}/layers/${layerId}/members`, {
      method: 'PUT',
      body: JSON.stringify({ user_ids: userIds }),
    }),

  createOverride: (scheduleId, data) =>
    fetchAPI(`/on-call/schedules/${scheduleId}/overrides`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  deleteOverride: (scheduleId, overrideId) =>
    fetchAPI(`/on-call/schedules/${scheduleId}/overrides/${overrideId}`, {
      method: 'DELETE',
    }),

  createSwapRequest: (scheduleId, data) =>
    fetchAPI(`/on-call/schedules/${scheduleId}/swap-requests`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  respondToSwapRequest: (swapId, data) =>
    fetchAPI(`/on-call/swap-requests/${swapId}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
};
