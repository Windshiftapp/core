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
  chat: (message, connectionId, sessionId, context, historyAfterMessageId) =>
    post('/ai/chat', {
      message,
      ...(connectionId ? { connection_id: connectionId } : {}),
      ...(sessionId ? { session_id: sessionId } : {}),
      ...(historyAfterMessageId ? { history_after_message_id: historyAfterMessageId } : {}),
      ...(context && Object.keys(context).length ? { context } : {}),
    }),
  getGeneralSession: () => get('/ai/sessions/general'),
  listSessions: (includeArchived = false) =>
    get(`/ai/sessions${includeArchived ? '?include_archived=true' : ''}`),
  getSessionMessages: (sessionId) => get(`/ai/sessions/${sessionId}/messages`),
  archiveSession: (sessionId) => post(`/ai/sessions/${sessionId}/archive`),
  listAvailableStandardAgents: (workspaceId) =>
    get(`/workspaces/${workspaceId}/available-standard-agents`),
  createStandardSession: (workspaceId, data) =>
    post(`/workspaces/${workspaceId}/agent-sessions`, data),
  dailyBriefing: () => get('/ai/daily-briefing'),
};

export const aiFeatures = {
  getConfig: () => get('/admin/ai-features'),
  updateConfig: (data) => put('/admin/ai-features', data),
};

export const workItemStaleness = {
  get: () => get('/admin/work-item-staleness'),
  update: (data) => put('/admin/work-item-staleness', data),
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
  refreshModels: (type, options = {}) =>
    post(`/admin/llm/providers/${encodeURIComponent(type)}/refresh-models`, options),
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

// Runner pools (WI-177). A runner_pool is an ActionCapability; these manage its
// child resources — registration tokens (mint/list/revoke) and registered
// runner instances (list/revoke). The plaintext token is returned once on mint.
export const runnerPools = {
  listTokens: (capabilityId) => get(`/admin/action-capabilities/${capabilityId}/runner-tokens`),
  mintToken: (capabilityId, data = {}) =>
    post(`/admin/action-capabilities/${capabilityId}/runner-tokens`, data),
  revokeToken: (capabilityId, tokenId) =>
    del(`/admin/action-capabilities/${capabilityId}/runner-tokens/${tokenId}`),
  listInstances: (capabilityId) =>
    get(`/admin/action-capabilities/${capabilityId}/runner-instances`),
  revokeInstance: (capabilityId, instanceId) =>
    del(`/admin/action-capabilities/${capabilityId}/runner-instances/${instanceId}`),
  listWorkspaceTokens: (workspaceId, poolId) =>
    get(`/workspaces/${workspaceId}/agent-runner-pools/${poolId}/tokens`),
  mintWorkspaceToken: (workspaceId, poolId, data = {}) =>
    post(`/workspaces/${workspaceId}/agent-runner-pools/${poolId}/tokens`, data),
  revokeWorkspaceToken: (workspaceId, poolId, tokenId) =>
    del(`/workspaces/${workspaceId}/agent-runner-pools/${poolId}/tokens/${tokenId}`),
  listWorkspaceInstances: (workspaceId, poolId) =>
    get(`/workspaces/${workspaceId}/agent-runner-pools/${poolId}/instances`),
};

// actionCredentials: workspace-aware credential store referenced by HTTP
// capabilities. The plaintext secret travels only on create/rotate; every
// response is the sanitized DTO (has_secret + prefix; no ciphertext).
export const actionCredentials = {
  // Admin view — system-admin only. The create/update body chooses scope via
  // applies_to_all_workspaces + workspace_ids; the list returns every row.
  getAllGlobal: () => get('/admin/action-credentials'),
  createGlobal: (data) => post('/admin/action-credentials', data),
  updateGlobal: (id, data) => put(`/admin/action-credentials/${id}`, data),
  rotateGlobal: (id, secret) => post(`/admin/action-credentials/${id}/rotate`, { secret }),
  deleteGlobal: (id) => del(`/admin/action-credentials/${id}`),

  // Workspace-scoped credentials — gated by action.credential.manage. The
  // list returns credentials usable in the workspace (those that apply to all
  // workspaces, plus those scoped to this one).
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
