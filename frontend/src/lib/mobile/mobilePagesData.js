import { api } from '../api.js';

/**
 * Shared data helpers for the mobile pages surface. Workspace-scoped page
 * endpoints are fanned out across the user's workspaces in parallel; a
 * workspace that fails (permission or deletion) is skipped rather than
 * failing the whole surface.
 */

/**
 * Fetch every visible page for each workspace, grouped for list rendering.
 * Empty sections are dropped so the phone list only shows workspaces that
 * actually have pages.
 *
 * @param {Array<{id: number, name: string}>} workspaces
 * @returns {Promise<Array<{workspace: Object, pages: any[]}>>}
 */
export async function fetchWorkspacePageSections(workspaces) {
  const settled = await Promise.allSettled((workspaces ?? []).map((ws) => api.pages.getAll(ws.id)));
  const sections = [];
  for (let i = 0; i < (workspaces ?? []).length; i++) {
    const result = settled[i];
    if (result.status !== 'fulfilled' || !Array.isArray(result.value)) continue;
    if (result.value.length === 0) continue;
    sections.push({ workspace: workspaces[i], pages: result.value });
  }
  return sections;
}

/**
 * Title-substring page search across workspaces. Results are tagged with the
 * workspace name for display, since the page DTO alone does not carry it.
 *
 * @param {Array<{id: number, name: string}>} workspaces
 * @param {string} query
 * @param {{limitPerWorkspace?: number, cap?: number}} options
 * @returns {Promise<any[]>}
 */
export async function searchPagesAcrossWorkspaces(
  workspaces,
  query,
  { limitPerWorkspace = 5, cap = 12 } = {}
) {
  const settled = await Promise.allSettled(
    (workspaces ?? []).map((ws) =>
      api.pages.searchPages(ws.id, query, { limit: limitPerWorkspace })
    )
  );
  const out = [];
  for (let i = 0; i < (workspaces ?? []).length; i++) {
    const result = settled[i];
    if (result.status !== 'fulfilled' || !Array.isArray(result.value)) continue;
    for (const page of result.value) {
      out.push({
        ...page,
        workspace_id: page.workspace_id ?? workspaces[i].id,
        workspace_name: workspaces[i].name,
      });
    }
  }
  return out.slice(0, cap);
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
