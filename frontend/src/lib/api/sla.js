import { fetchAPI, fetchV2Data } from './core.js';

// Extract a readable message from an API error whose body is often JSON.
function readableError(err) {
  const raw = err?.message || '';
  try {
    const parsed = JSON.parse(raw);
    const payload = parsed?.error && typeof parsed.error === 'object' ? parsed.error : parsed;
    if (payload?.message) return payload.message;
  } catch {
    // Body was not JSON; fall through to the raw message.
  }
  return raw || 'Invalid query';
}

// Service-level-agreement configuration and read surfaces. All calls use the
// internal cookie-auth API; the frontend has no v2/bearer path.
const workspaceBase = (workspaceId) => `/workspaces/${workspaceId}/sla`;
const teamBase = (teamId) => `/teams/${teamId}`;

export const sla = {
  // --- Workspace working calendars ---
  getCalendars: (workspaceId) => fetchAPI(`${workspaceBase(workspaceId)}/calendars`),
  getAvailableCalendars: (workspaceId) =>
    fetchAPI(`${workspaceBase(workspaceId)}/available-calendars`),
  createCalendar: (workspaceId, data) =>
    fetchAPI(`${workspaceBase(workspaceId)}/calendars`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  updateCalendar: (workspaceId, calendarId, data) =>
    fetchAPI(`${workspaceBase(workspaceId)}/calendars/${calendarId}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  deleteCalendar: (workspaceId, calendarId) =>
    fetchAPI(`${workspaceBase(workspaceId)}/calendars/${calendarId}`, { method: 'DELETE' }),
  getCalendarImpact: (workspaceId, calendarId) =>
    fetchAPI(`${workspaceBase(workspaceId)}/calendars/${calendarId}/impact`),
  getCoveragePreview: (workspaceId, calendarId) =>
    fetchAPI(`${workspaceBase(workspaceId)}/coverage-preview?calendar_id=${calendarId}`),

  // --- Metrics (conditions, goals, and targets ride in the payload) ---
  getMetrics: (workspaceId) => fetchAPI(`${workspaceBase(workspaceId)}/metrics`),
  getMetric: (workspaceId, metricId) =>
    fetchAPI(`${workspaceBase(workspaceId)}/metrics/${metricId}`),
  createMetric: (workspaceId, data) =>
    fetchAPI(`${workspaceBase(workspaceId)}/metrics`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  updateMetric: (workspaceId, metricId, data) =>
    fetchAPI(`${workspaceBase(workspaceId)}/metrics/${metricId}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  deleteMetric: (workspaceId, metricId) =>
    fetchAPI(`${workspaceBase(workspaceId)}/metrics/${metricId}`, { method: 'DELETE' }),

  // --- Warning thresholds ---
  getWarningThresholds: (workspaceId, requestOptions = {}) =>
    fetchAPI(`${workspaceBase(workspaceId)}/warning-thresholds`, requestOptions),
  createWarningThreshold: (workspaceId, data) =>
    fetchAPI(`${workspaceBase(workspaceId)}/warning-thresholds`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  updateWarningThreshold: (workspaceId, thresholdId, data) =>
    fetchAPI(`${workspaceBase(workspaceId)}/warning-thresholds/${thresholdId}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  deleteWarningThreshold: (workspaceId, thresholdId) =>
    fetchAPI(`${workspaceBase(workspaceId)}/warning-thresholds/${thresholdId}`, {
      method: 'DELETE',
    }),

  // --- Recalculation ---
  getRecalculations: (workspaceId) => fetchAPI(`${workspaceBase(workspaceId)}/recalculations`),
  startRecalculation: (workspaceId, metricId = 0) =>
    fetchAPI(`${workspaceBase(workspaceId)}/recalculations`, {
      method: 'POST',
      body: JSON.stringify({ metric_id: metricId }),
    }),

  // --- Compliance report ---
  getReport: (workspaceId, { from = '', to = '' } = {}) => {
    const params = new URLSearchParams();
    if (from) params.set('from', from);
    if (to) params.set('to', to);
    const query = params.toString();
    return fetchAPI(`${workspaceBase(workspaceId)}/report${query ? `?${query}` : ''}`);
  },

  // --- Item state ---
  getItemSLA: (itemId, requestOptions = {}) => fetchAPI(`/items/${itemId}/sla`, requestOptions),
  // Batch read for list/board badges: one request amortizes the workspace
  // metric and reference loads across every visible row.
  getItemSLABatch: (workspaceId, itemIds, requestOptions = {}) =>
    fetchAPI(`${workspaceBase(workspaceId)}/items?ids=${itemIds.join(',')}`, requestOptions),

  // Validate a goal QL by parsing it through the item query endpoint.
  validateGoalQuery: async (workspaceId, ql) => {
    const trimmed = (ql || '').trim();
    if (!trimmed) return null;
    const params = new URLSearchParams({ ql: trimmed, page_size: '1' });
    if (workspaceId) params.set('workspace_id', String(workspaceId));
    try {
      await fetchV2Data(`/items?${params.toString()}`);
      return null;
    } catch (err) {
      return readableError(err);
    }
  },

  // --- Team service-hours calendars ---
  getTeamCalendars: (teamId) => fetchAPI(`${teamBase(teamId)}/working-calendars`),
  createTeamCalendar: (teamId, data) =>
    fetchAPI(`${teamBase(teamId)}/working-calendars`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  updateTeamCalendar: (teamId, calendarId, data) =>
    fetchAPI(`${teamBase(teamId)}/working-calendars/${calendarId}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  deleteTeamCalendar: (teamId, calendarId) =>
    fetchAPI(`${teamBase(teamId)}/working-calendars/${calendarId}`, { method: 'DELETE' }),
  getTeamCalendarImpact: (teamId, calendarId) =>
    fetchAPI(`${teamBase(teamId)}/working-calendars/${calendarId}/impact`),

  // --- Team-workspace bindings ---
  getWorkspaceTeamBindings: (workspaceId) => fetchAPI(`/workspaces/${workspaceId}/team-bindings`),
  createWorkspaceTeamBinding: (workspaceId, teamId) =>
    fetchAPI(`/workspaces/${workspaceId}/team-bindings`, {
      method: 'POST',
      body: JSON.stringify({ team_id: teamId }),
    }),
  deleteWorkspaceTeamBinding: (workspaceId, bindingId) =>
    fetchAPI(`/workspaces/${workspaceId}/team-bindings/${bindingId}`, { method: 'DELETE' }),
  getTeamWorkspaceBindings: (teamId) => fetchAPI(`${teamBase(teamId)}/workspace-bindings`),
  createTeamWorkspaceBinding: (teamId, workspaceId) =>
    fetchAPI(`${teamBase(teamId)}/workspace-bindings`, {
      method: 'POST',
      body: JSON.stringify({ workspace_id: workspaceId }),
    }),
  deleteTeamWorkspaceBinding: (teamId, bindingId) =>
    fetchAPI(`${teamBase(teamId)}/workspace-bindings/${bindingId}`, { method: 'DELETE' }),
};
