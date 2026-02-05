import { get, post, put, del } from './core.js';

export const ai = {
  status: () => get('/ai/status'),
  planMyDay: (connectionId) =>
    get(`/ai/plan-my-day${connectionId ? `?connection_id=${connectionId}` : ''}`),
  planMyDayPreview: () => get('/ai/plan-my-day?preview=true'),
  catchMeUp: (itemId) => post(`/ai/items/${itemId}/catch-me-up`),
  findSimilar: (itemId) => post(`/ai/items/${itemId}/find-similar`),
  decompose: (itemId) => post(`/ai/items/${itemId}/decompose`),
};

export const llmConnections = {
  getAll: () => get('/admin/llm-connections'),
  get: (id) => get(`/admin/llm-connections/${id}`),
  create: (data) => post('/admin/llm-connections', data),
  update: (id, data) => put(`/admin/llm-connections/${id}`, data),
  delete: (id) => del(`/admin/llm-connections/${id}`),
  test: (id) => post(`/admin/llm-connections/${id}/test`),
  setFeatures: (id, features) => put(`/admin/llm-connections/${id}/features`, { features }),
};

export const llmProviders = {
  getProviders: () => get('/llm/providers'),
  getForFeature: (feature) => get(`/llm/connections?feature=${feature}`),
};
