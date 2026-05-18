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
  chat: (message, connectionId, history, context) =>
    post('/ai/chat', {
      message,
      ...(connectionId ? { connection_id: connectionId } : {}),
      ...(history?.length ? { history } : {}),
      ...(context && Object.keys(context).length ? { context } : {}),
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
  // Workspace-scoped picker list — returns enabled capabilities the workspace
  // can reference (applies-to-all OR explicitly scoped). Optional type filter.
  getForWorkspace: (workspaceId, type) => {
    const qs = type ? `?type=${encodeURIComponent(type)}` : '';
    return get(`/workspaces/${workspaceId}/action-capabilities${qs}`);
  },
};

// actionCredentials: workspace-aware credential store referenced by HTTP
// capabilities. The plaintext secret travels only on create/rotate; every
// response is the sanitized DTO (has_secret + prefix; no ciphertext).
export const actionCredentials = {
  // Global (workspace_id IS NULL) credentials — system-admin only.
  getAllGlobal: () => get('/admin/action-credentials'),
  createGlobal: (data) => post('/admin/action-credentials', data),
  updateGlobal: (id, data) => put(`/admin/action-credentials/${id}`, data),
  rotateGlobal: (id, secret) => post(`/admin/action-credentials/${id}/rotate`, { secret }),
  deleteGlobal: (id) => del(`/admin/action-credentials/${id}`),

  // Workspace-scoped credentials — gated by action.credential.manage. The
  // list endpoint also returns globals (workspace_id null) so a single call
  // populates the credential picker.
  getForWorkspace: (workspaceId) => get(`/workspaces/${workspaceId}/action-credentials`),
  createForWorkspace: (workspaceId, data) =>
    post(`/workspaces/${workspaceId}/action-credentials`, data),
  updateForWorkspace: (workspaceId, id, data) =>
    put(`/workspaces/${workspaceId}/action-credentials/${id}`, data),
  rotateForWorkspace: (workspaceId, id, secret) =>
    post(`/workspaces/${workspaceId}/action-credentials/${id}/rotate`, { secret }),
  deleteForWorkspace: (workspaceId, id) =>
    del(`/workspaces/${workspaceId}/action-credentials/${id}`),
};
