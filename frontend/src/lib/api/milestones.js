import { fetchAllV2Pages, fetchV2Data } from './core.js';
import { createCrudClient } from './createCrudClient.js';

export const milestoneCategories = createCrudClient('/milestone-categories');

function planningQuery(filters = {}) {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(filters)) {
    if (key === 'workspace_id') continue;
    if (value == null || value === '') continue;
    // include_global defaults to false server-side; only send explicit opt-ins.
    if (key === 'include_global' && value === false) continue;
    params.set(key, String(value));
  }
  return params.toString();
}

/**
 * List planning resources without merging scopes client-side. Workspace-scoped
 * requests return the workspace's rows plus global rows in one server-side
 * response (include_global); unscoped requests return global and
 * accessible-workspace rows, optionally narrowed with is_global. Mixing the
 * two sources client-side would duplicate rows and break keyed renders.
 */
async function listPlanning(path, filters = {}, requestOptions = {}) {
  const query = planningQuery(filters);
  const globalPath = `${path}${query ? `?${query}` : ''}`;
  if (filters.workspace_id == null) {
    return fetchAllV2Pages(globalPath, requestOptions);
  }
  const effective =
    filters.include_global === undefined ? { ...filters, include_global: true } : filters;
  const scopedQuery = planningQuery(effective);
  const workspacePath = `/workspaces/${filters.workspace_id}${path}${scopedQuery ? `?${scopedQuery}` : ''}`;
  return fetchAllV2Pages(workspacePath, requestOptions);
}

function planningCreate(path, data) {
  const { is_global: isGlobal, workspace_id: workspaceId, ...body } = data;
  const collection = isGlobal ? path : `/workspaces/${workspaceId}${path}`;
  return fetchV2Data(collection, { method: 'POST', body: JSON.stringify(body) });
}

function milestonePatch(data) {
  const { name, description, target_date, status, category_id } = data;
  return { name, description, target_date, status, category_id };
}

function iterationPatch(data) {
  const { name, description, start_date, end_date, status, type_id } = data;
  return { name, description, start_date, end_date, status, type_id };
}

export const milestones = {
  getAll: (filters = {}, requestOptions = {}) =>
    listPlanning('/milestones', filters, requestOptions),
  get: (id) => fetchV2Data(`/milestones/${id}`),
  create: (data) => planningCreate('/milestones', data),
  update: (id, data) =>
    fetchV2Data(`/milestones/${id}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/merge-patch+json' },
      body: JSON.stringify(milestonePatch(data)),
    }),
  delete: (id) => fetchV2Data(`/milestones/${id}`, { method: 'DELETE' }),
  getTestStatistics: (id) => fetchV2Data(`/milestones/${id}/test-statistics`),
  getTestStatisticsMany: async (ids = []) => {
    const entries = await fetchV2Data('/milestones/test-statistics', {
      method: 'POST',
      body: JSON.stringify({ ids: [...new Set(ids)] }),
    });
    return Object.fromEntries(
      (entries || []).map(({ milestone_id: milestoneId, statistics }) => [milestoneId, statistics])
    );
  },
  getProgress: (id) => fetchV2Data(`/milestones/${id}/progress`),
  release: (id, data, idempotencyKey) =>
    fetchV2Data(`/milestones/${id}/release`, {
      method: 'POST',
      headers: idempotencyKey ? { 'Idempotency-Key': idempotencyKey } : undefined,
      body: JSON.stringify(data),
    }),
  reorder: (scope, orderedIds) => {
    const path = scope?.is_global
      ? '/milestones/reorder'
      : `/workspaces/${scope?.workspace_id}/milestones/reorder`;
    return fetchV2Data(path, {
      method: 'POST',
      body: JSON.stringify({
        ordered_ids: orderedIds,
        category_id: scope?.category_id ?? undefined,
      }),
    });
  },
};

export const iterationTypes = createCrudClient('/iteration-types');

export const iterations = {
  getAll: (filters = {}, requestOptions = {}) =>
    listPlanning('/iterations', filters, requestOptions),
  get: (id) => fetchV2Data(`/iterations/${id}`),
  create: (data) => planningCreate('/iterations', data),
  update: (id, data) =>
    fetchV2Data(`/iterations/${id}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/merge-patch+json' },
      body: JSON.stringify(iterationPatch(data)),
    }),
  delete: (id) => fetchV2Data(`/iterations/${id}`, { method: 'DELETE' }),
  getProgress: (id) => fetchV2Data(`/iterations/${id}/progress`),
  // Bulk progress for many iterations in one request, keyed by iteration id.
  // Replaces one getProgress() per iteration on the dashboard timeline.
  getProgressMany: async (ids = []) => {
    const entries = await fetchV2Data('/iterations/progress', {
      method: 'POST',
      body: JSON.stringify({ ids: [...new Set(ids)] }),
    });
    return Object.fromEntries(
      (entries || []).map(({ iteration_id: iterationId, progress }) => [iterationId, progress])
    );
  },
  getBurndown: (id) => fetchV2Data(`/iterations/${id}/burndown`),
  complete: (id, moveIncompleteToIterationId = null) =>
    fetchV2Data(`/iterations/${id}/complete`, {
      method: 'POST',
      body: JSON.stringify({
        move_incomplete_to_iteration_id: moveIncompleteToIterationId,
      }),
    }),
};
