import { del, get, post, put } from './core.js';

export const ai = {
  status: () => get('/ai/status'),
  planMyDay: (connectionId) =>
    get(`/ai/plan-my-day${connectionId ? `?connection_id=${connectionId}` : ''}`),
  planMyDayPreview: () => get('/ai/plan-my-day?preview=true'),
  catchMeUp: (itemId) => post(`/ai/items/${itemId}/catch-me-up`),
  findSimilar: (itemId) => post(`/ai/items/${itemId}/find-similar`),
  decompose: (itemId) => post(`/ai/items/${itemId}/decompose`),
  generateReleaseNotes: (milestoneId, connectionId) =>
    post(
      `/ai/milestones/${milestoneId}/generate-release-notes${connectionId ? `?connection_id=${connectionId}` : ''}`
    ),
  analyzeDependencies: (iterationId, body = {}, connectionId) =>
    post(
      `/ai/iterations/${iterationId}/analyze-dependencies${connectionId ? `?connection_id=${connectionId}` : ''}`,
      body
    ),
  acceptDependencies: (iterationId, suggestions) =>
    post(`/ai/iterations/${iterationId}/accept-dependencies`, { suggestions }),
};

export const llmConnections = {
  getAll: () => get('/admin/llm-connections'),
  get: (id) => get(`/admin/llm-connections/${id}`),
  create: (data) => post('/admin/llm-connections', data),
  update: (id, data) => put(`/admin/llm-connections/${id}`, data),
  delete: (id) => del(`/admin/llm-connections/${id}`),
  test: (id) => post(`/admin/llm-connections/${id}/test`),
};

export const llmProviders = {
  getProviders: () => get('/llm/providers'),
  getEnabled: () => get('/llm/connections'),
};
