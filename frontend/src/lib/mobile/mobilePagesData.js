import { pages } from '../api/pages.js';
import { BACKGROUND_READ } from '../api/readScheduler.js';

/**
 * Shared data helpers for the mobile pages surface. One cross-workspace
 * summaries request covers many workspaces at once, so a large workspace set
 * is fetched in bounded batches instead of one request per workspace. Every
 * batch opts into the shared read scheduler so the per-tab budget is
 * authoritative; a batch that fails (permission or deletion) is skipped
 * rather than failing the whole surface.
 */

// Matches the server's per-request workspace cap.
export const WORKSPACE_BATCH_SIZE = 50;

function chunk(items, size) {
  const out = [];
  for (let i = 0; i < items.length; i += size) out.push(items.slice(i, i + size));
  return out;
}

function pageFromSummary(row) {
  return {
    id: row.page_id,
    parent_id: row.parent_id ?? null,
    title: row.title,
    depth: row.depth ?? 0,
    is_home: row.is_home ?? false,
    updated_at: row.updated_at,
  };
}

/**
 * Fetch page summaries for a workspace list in bounded batches. Each batch is
 * one request; failed batches are skipped. When stopAt is set, batching stops
 * once enough rows are collected (search only needs the first page of hits).
 *
 * @param {Array<{id: number, name: string}>} workspaces
 * @param {{q?: string, limit?: number, signal?: AbortSignal, stopAt?: number}} [options]
 * @returns {Promise<any[]>}
 */
async function fetchSummaryBatches(workspaces, { q = '', limit, signal, stopAt } = {}) {
  const rows = [];
  for (const batch of chunk(workspaces, WORKSPACE_BATCH_SIZE)) {
    if (signal?.aborted) break;
    try {
      const batchRows = await pages.getSummaries(
        batch.map((ws) => ws.id),
        { q, limit, signal, ...BACKGROUND_READ }
      );
      if (Array.isArray(batchRows)) rows.push(...batchRows);
    } catch {
      // A permission-denied or deleted workspace must not fail the surface.
      if (signal?.aborted) break;
    }
    if (stopAt && rows.length >= stopAt) break;
  }
  return rows;
}

/**
 * Fetch every visible page for each workspace, grouped for list rendering.
 * Empty sections are dropped so the phone list only shows workspaces that
 * actually have pages.
 *
 * @param {Array<{id: number, name: string}>} workspaces
 * @param {{signal?: AbortSignal}} [options]
 * @returns {Promise<Array<{workspace: Object, pages: any[]}>>}
 */
export async function fetchWorkspacePageSections(workspaces, { signal } = {}) {
  const list = workspaces ?? [];
  const rows = await fetchSummaryBatches(list, { signal });
  const sections = new Map(list.map((ws) => [ws.id, { workspace: ws, pages: [] }]));
  for (const row of rows) {
    const section = sections.get(row.workspace_id);
    if (section) section.pages.push(pageFromSummary(row));
  }
  return [...sections.values()].filter((section) => section.pages.length > 0);
}

/**
 * Title/body page search across workspaces in one bounded batch sequence.
 * Results are tagged with the workspace name for display, since the page
 * summary alone does not carry it.
 *
 * @param {Array<{id: number, name: string}>} workspaces
 * @param {string} query
 * @param {{cap?: number, signal?: AbortSignal}} options
 * @returns {Promise<any[]>}
 */
export async function searchPagesAcrossWorkspaces(workspaces, query, { cap = 12, signal } = {}) {
  const list = workspaces ?? [];
  const nameById = new Map(list.map((ws) => [ws.id, ws.name]));
  const rows = await fetchSummaryBatches(list, { q: query, limit: cap, signal, stopAt: cap });
  return rows.slice(0, cap).map((row) => ({
    ...pageFromSummary(row),
    workspace_id: row.workspace_id,
    workspace_name: nameById.get(row.workspace_id) ?? '',
  }));
}

/**
 * Ancestor chain for a page (root first) computed from a flat page list.
 * Returns [] when the list is unavailable (permission-degraded rendering).
 *
 * @param {any[]} flatPages
 * @param {Object} page
 * @returns {any[]}
 */
export function pageAncestors(flatPages, page) {
  if (!Array.isArray(flatPages) || !page) return [];
  const byId = new Map(flatPages.map((p) => [p.id, p]));
  const chain = [];
  let cursor = page.parent_id ? byId.get(page.parent_id) : null;
  let guard = 0;
  while (cursor && guard++ < 32) {
    chain.unshift(cursor);
    cursor = cursor.parent_id ? byId.get(cursor.parent_id) : null;
  }
  return chain;
}

/**
 * Direct children of a page from a flat page list, in server order.
 *
 * @param {any[]} flatPages
 * @param {number} pageId
 * @returns {any[]}
 */
export function pageChildren(flatPages, pageId) {
  if (!Array.isArray(flatPages)) return [];
  return flatPages.filter((p) => p.parent_id === pageId);
}
