import { get } from 'svelte/store';
import { api } from '../../api.js';
import { currentRoute, isMobileRoute } from '../../router.js';

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
    const rows = await api.requirements.listKeys(workspaceId);
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
  const route = get(currentRoute);
  const onMobile = isMobileRoute(route?.view);
  if (meta?.requirement_number != null) {
    if (onMobile) {
      return `/m/requirements/${workspaceId}/${meta.requirement_number}`;
    }
    return `/workspaces/${workspaceId}/requirements/${meta.requirement_number}`;
  }
  if (onMobile) {
    return `/m/pages/${workspaceId}/${pageId}`;
  }
  return `/workspaces/${workspaceId}/pages/${pageId}`;
}

/**
 * Mobile navigation href for a traceability peer entity, or null when no mobile route exists.
 * @param {{ type?: string, id?: number, workspaceId?: number }} entity
 * @param {number} workspaceId
 * @param {RequirementPageMeta | null | undefined} pageMeta
 */
export function traceabilityEntityHref(entity, workspaceId, pageMeta) {
  if (!entity?.id) return null;
  if (entity.type === 'item') {
    return `/m/items/${entity.id}`;
  }
  if (entity.type === 'page') {
    return pageHref(workspaceId, entity.id, pageMeta);
  }
  return null;
}
