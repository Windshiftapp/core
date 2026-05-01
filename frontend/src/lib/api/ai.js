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
  summarizeTestPlanDescription: (testSetId, connectionId) =>
    post(
      `/ai/test-sets/${testSetId}/summarize-description${connectionId ? `?connection_id=${connectionId}` : ''}`
    ),
  analyzeDependencies: (iterationId, body = {}, connectionId) =>
    post(
      `/ai/iterations/${iterationId}/analyze-dependencies${connectionId ? `?connection_id=${connectionId}` : ''}`,
      body
    ),
  acceptDependencies: (iterationId, suggestions) =>
    post(`/ai/iterations/${iterationId}/accept-dependencies`, { suggestions }),
  chat: (message, connectionId, history) =>
    post('/ai/chat', {
      message,
      ...(connectionId ? { connection_id: connectionId } : {}),
      ...(history?.length ? { history } : {}),
    }),
  dailyBriefing: () => get('/ai/daily-briefing'),
};

export const aiFeatures = {
  getConfig: () => get('/admin/ai-features'),
  updateConfig: (data) => put('/admin/ai-features', data),
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

export const actionCapabilities = {
  getAll: () => get('/admin/action-capabilities'),
  get: (id) => get(`/admin/action-capabilities/${id}`),
  create: (data) => post('/admin/action-capabilities', data),
  update: (id, data) => put(`/admin/action-capabilities/${id}`, data),
  delete: (id) => del(`/admin/action-capabilities/${id}`),
};
