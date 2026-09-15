import { api } from '../../api.js';

const LIST_LIMIT = 500;

/**
 * @typedef {{ key: string, requirement_number: number }} RequirementPageMeta
 */

/**
 * Load requirement display keys keyed by backing page id.
 * @param {number} workspaceId
 * @returns {Promise<Map<number, RequirementPageMeta>>}
 */
export async function buildRequirementKeyByPageId(workspaceId) {
  if (!workspaceId) return new Map();
  try {
    const rows = await api.requirements.list(workspaceId, { limit: LIST_LIMIT });
    const map = new Map();
    for (const row of rows || []) {
      if (row?.page_id && row?.key) {
        map.set(row.page_id, {
          key: row.key,
          requirement_number: row.requirement_number,
        });
      }
    }
    return map;
  } catch {
    return new Map();
  }
}

/**
 * @param {number} workspaceId
 * @param {number} pageId
 * @param {RequirementPageMeta | null | undefined} meta
 */
export function pageHref(workspaceId, pageId, meta) {
  if (meta?.requirement_number != null) {
    return `/workspaces/${workspaceId}/requirements/${meta.requirement_number}`;
  }
  return `/workspaces/${workspaceId}/pages/${pageId}`;
}
