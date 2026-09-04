import { notifyItemMutation } from '../utils/crossTabSync.js';
import { dateInputToISOString } from '../utils/dateFormatter.js';
import { fetchAPI, fetchAPIV2, fetchV2Data } from './core.js';
import { buildQueryString } from './utils.js';

// Item ids per POST /items/batch request. Kept under the server cap (500).
const ITEM_BATCH_CHUNK = 200;

function itemMutationBody(data) {
  const body = { ...data };
  for (const field of ['due_date', 'start_date', 'end_date']) {
    if (typeof body[field] === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(body[field])) {
      body[field] = dateInputToISOString(body[field]);
    }
  }
  return body;
}

function itemListQuery(/** @type {Record<string, any>} */ filters = {}) {
  const { limit, omit_descriptions, order_by, sort_direction, ...canonical } = filters;
  if (limit != null) canonical.page_size = Math.min(Number(limit), 100);
  if (omit_descriptions) canonical.fields = 'summary';
  if (order_by) canonical.sort = sort_direction === 'desc' ? `-${order_by}` : order_by;
  return buildQueryString(canonical);
}

/**
 * Wrap a mutating items API method so a successful call broadcasts a
 * cross-tab freshness notice to other open Windshift tabs. Failures are
 * surfaced unchanged (the original promise rejects) and never broadcast.
 *
 * @template {(...args: any[]) => Promise<any>} F
 * @param {F} fn
 * @param {string} type - coarse mutation category for the broadcast payload
 * @returns {F}
 */
function withCrossTabNotice(fn, type) {
  return /** @type {F} */ (
    async (...args) => {
      const result = await fn(...args);
      let itemId = null;
      if (typeof args[0] === 'number' || typeof args[0] === 'string') {
        itemId = args[0];
      } else if (result && typeof result === 'object' && result.id != null) {
        // create() takes a payload (no id arg) — pull it from the response.
        itemId = result.id;
      }
      notifyItemMutation({ type, itemId });
      return result;
    }
  );
}

/**
 * @param {number|string} id
 * @param {RequestInit & { surface?: string }} [options]
 */
function fetchItemDetailSummary(id, options = {}) {
  const { surface, ...requestOptions } = options;
  return fetchV2Data(
    `/items/${id}/detail-summary${surface ? `?surface=${encodeURIComponent(surface)}` : ''}`,
    requestOptions
  );
}

/**
 * @param {string} workspaceKey
 * @param {number|string} itemNumber
 * @param {RequestInit & { surface?: string }} [options]
 */
function fetchItemDetailSummaryByKey(workspaceKey, itemNumber, options = {}) {
  const { surface, ...requestOptions } = options;
  return fetchV2Data(
    `/workspaces/${encodeURIComponent(workspaceKey)}/items/${encodeURIComponent(itemNumber)}/detail-summary${surface ? `?surface=${encodeURIComponent(surface)}` : ''}`,
    requestOptions
  );
}

function fetchBacklog(
  workspaceId,
  ql = null,
  collectionId = null,
  /** @type {any} */ { page, limit, sub_ql, omit_descriptions, include_watermark } = {}
) {
  const params = new URLSearchParams();
  if (collectionId) {
    params.append('collection_id', collectionId);
  } else if (workspaceId) {
    params.append('workspace_id', workspaceId);
  }
  if (ql) params.append('ql', ql);
  if (sub_ql) params.append('sub_ql', sub_ql);
  if (omit_descriptions) params.append('fields', 'summary');
  if (include_watermark) params.append('include_watermark', 'true');
  if (page) params.append('page', String(page));
  if (limit) params.append('page_size', String(Math.min(Number(limit), 100)));
  return fetchAPIV2(`/items/backlog?${params}`);
}

async function fetchBacklogBoundary(workspaceId, collectionId, subQL, boundary) {
  const options = {
    page: 1,
    limit: 1,
    sub_ql: subQL || undefined,
    omit_descriptions: true,
  };

  for (let attempt = 0; attempt < 2; attempt++) {
    const firstPage = await fetchBacklog(workspaceId, null, collectionId, options);
    const firstItems = firstPage?.data ?? [];
    if (boundary === 'start' || firstItems.length === 0) return firstItems[0] ?? null;

    const total = firstPage?.pagination?.total_items ?? firstItems.length;
    if (total <= 1) return firstItems[0] ?? null;

    const lastPage = await fetchBacklog(workspaceId, null, collectionId, {
      ...options,
      page: total,
    });
    const lastItems = lastPage?.data ?? [];
    if (lastItems.length > 0) return lastItems[0];
  }

  return null;
}

export const items = {
  getAll: (filters = {}, requestOptions = {}) => {
    return fetchAPIV2(`/items${itemListQuery(filters)}`, requestOptions);
  },
  get: (id, requestOptions = {}) => fetchV2Data(`/items/${id}`, requestOptions),
  getDetailSummary: fetchItemDetailSummary,
  getByKey: (workspaceKey, itemNumber, requestOptions = {}) =>
    fetchV2Data(
      `/workspaces/${encodeURIComponent(workspaceKey)}/items/${encodeURIComponent(itemNumber)}`,
      requestOptions
    ),
  getDetailSummaryByKey: fetchItemDetailSummaryByKey,
  /**
   * Fetch many items in one (or a few) bulk requests instead of one
   * GET /items/{id} per id. Returns an array of full item-detail objects in
   * unspecified order; ids the caller can't view or that don't exist are
   * silently omitted (consumers patch loaded rows by id and no-op on the
   * rest). Chunked under the server's 500-id cap. Replaces the former
   * Promise.all(...map(id => /items/{id})) fan-out that could exhaust the
   * Postgres pool on a collection delta refresh.
   */
  getMany: async (ids = []) => {
    const unique = [...new Set(ids)];
    if (unique.length === 0) return [];
    const chunks = [];
    for (let i = 0; i < unique.length; i += ITEM_BATCH_CHUNK) {
      chunks.push(unique.slice(i, i + ITEM_BATCH_CHUNK));
    }
    const results = await Promise.all(
      chunks.map((chunk) =>
        fetchV2Data('/items/batch', { method: 'POST', body: JSON.stringify({ ids: chunk }) })
      )
    );
    return results.flat();
  },
  getChanges: (filters = {}) => fetchV2Data(`/items/changes${buildQueryString(filters)}`),
  create: withCrossTabNotice(
    (data) =>
      fetchV2Data('/items', {
        method: 'POST',
        body: JSON.stringify(itemMutationBody(data)),
      }),
    'create'
  ),
  update: withCrossTabNotice(
    (id, data) =>
      fetchV2Data(`/items/${id}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/merge-patch+json' },
        body: JSON.stringify(itemMutationBody(data)),
      }),
    'update'
  ),
  // Atomically apply one field patch to up to 500 work items. The server
  // returns only changed items; unchanged retries produce no duplicate events.
  bulkUpdate: withCrossTabNotice(
    (itemIds, fields) =>
      fetchV2Data('/items/bulk-update', {
        method: 'POST',
        body: JSON.stringify({ item_ids: itemIds, set: fields }),
      }),
    'update'
  ),
  bulkPatch: withCrossTabNotice(
    (patches) =>
      fetchV2Data('/items/bulk-patch', {
        method: 'POST',
        body: JSON.stringify({ patches }),
      }),
    'update'
  ),
  getRoadmapHierarchyDates: (rootIds) =>
    fetchV2Data('/items/roadmap-hierarchy-dates', {
      method: 'POST',
      body: JSON.stringify({ root_ids: rootIds }),
    }),
  // Perform a workflow status transition. Use this instead of passing
  // status_id to update() — the update endpoint rejects status_id so that
  // validator-mode and condition-mode workflow rules are always enforced.
  // Returns the updated item (unwrapped from the {item, old_status_id, ...} envelope).
  transition: withCrossTabNotice(async (id, toStatusId) => {
    const response = await fetchV2Data(`/items/${id}/transition`, {
      method: 'POST',
      body: JSON.stringify({ to_status_id: toStatusId }),
    });
    return response.item;
  }, 'transition'),
  delete: withCrossTabNotice(
    (id) =>
      fetchAPIV2(`/items/${id}`, {
        method: 'DELETE',
      }),
    'delete'
  ),
  getDeleteInfo: (id) => fetchV2Data(`/items/${id}/delete-info`),
  deleteCascade: withCrossTabNotice(
    (id) =>
      fetchV2Data(`/items/${id}/cascade-deletion`, {
        method: 'POST',
      }),
    'delete'
  ),
  reparentChildren: (id, newParentId) =>
    fetchV2Data(`/items/${id}/reparent-children`, {
      method: 'PUT',
      body: JSON.stringify({ parent_id: newParentId }),
    }),
  copy: withCrossTabNotice(
    (id) =>
      fetchV2Data(`/items/${id}/copy`, {
        method: 'POST',
      }),
    'create'
  ),
  previewWorkspaceMove: (id, data) =>
    fetchV2Data(`/items/${id}/move-workspace/preview`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  moveWorkspace: withCrossTabNotice(
    (id, data) =>
      fetchV2Data(`/items/${id}/move-workspace`, {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    'update'
  ),
  updateFracIndex: withCrossTabNotice(
    (id, data) =>
      fetchV2Data(`/items/${id}/rank`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/merge-patch+json' },
        body: JSON.stringify(data),
      }),
    'reorder'
  ),
  getBacklog: fetchBacklog,
  getBacklogBoundary: fetchBacklogBoundary,
  getChildren: (itemId, requestOptions = {}) =>
    fetchV2Data(`/items/${itemId}/children`, requestOptions),
  getAncestors: (itemId, requestOptions = {}) =>
    fetchV2Data(`/items/${itemId}/ancestors`, requestOptions),
  getDescendants: (itemId, maxDepth = null) => {
    const params = maxDepth ? `?max_depth=${maxDepth}` : '';
    return fetchV2Data(`/items/${itemId}/descendants${params}`);
  },
  getTimeRollup: (itemId, { maxDepth = 10 } = {}) =>
    fetchV2Data(`/items/${itemId}/time-rollup?max_depth=${maxDepth}`),
  // Get available status transitions for a specific item based on workflow configuration
  getAvailableStatusTransitions: (itemId, requestOptions = {}) =>
    fetchV2Data(`/items/${itemId}/available-transitions`, requestOptions),
  analyzeTypeChange: (itemId, targetItemTypeId) =>
    fetchV2Data(`/items/${itemId}/type-change-analysis?target_item_type_id=${targetItemTypeId}`),
  changeType: withCrossTabNotice(
    (itemId, data) =>
      fetchV2Data(`/items/${itemId}/change-type`, {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    'update'
  ),
  // Get history of changes for an item
  getHistory: (itemId) => fetchV2Data(`/items/${itemId}/history`),
  getStatusDurations: (itemId, requestOptions = {}) =>
    fetchV2Data(`/items/${itemId}/status-durations`, requestOptions),

  // Get items created in the last N days
  getRecentlyCreated: (workspaceId, days = 7) => {
    const sevenDaysAgo = new Date();
    sevenDaysAgo.setDate(sevenDaysAgo.getDate() - days);
    const createdSince = sevenDaysAgo.toISOString();
    const params = new URLSearchParams({
      workspace_id: workspaceId,
      created_since: createdSince,
    });
    return fetchAPI(`/items?${params}`);
  },

  // Watch/unwatch items
  addWatch: (id) =>
    fetchV2Data(`/items/${id}/watch`, {
      method: 'POST',
      body: JSON.stringify({}),
    }),
  removeWatch: (id) =>
    fetchV2Data(`/items/${id}/watch`, {
      method: 'DELETE',
    }),
  getWatchStatus: (id, requestOptions = {}) => fetchV2Data(`/items/${id}/watch`, requestOptions),

  // Personal tasks relationship
  getPersonalTasks: (itemId, requestOptions = {}) =>
    fetchV2Data(`/items/${itemId}/personal-tasks`, requestOptions),
  unlinkPersonalTask: (itemId) =>
    fetchAPIV2(`/items/${itemId}/related-work-item`, {
      method: 'DELETE',
    }),
};
