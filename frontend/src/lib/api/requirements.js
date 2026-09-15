import { fetchV2Data } from './core.js';
import { buildQueryString } from './utils.js';

/**
 * Workspace requirements API client (page-backed managed documents).
 */
/**
 * @typedef {Object} RequirementListParams
 * @property {string} [q]
 * @property {string} [requirement_type]
 * @property {string} [status]
 * @property {number} [owner_id]
 * @property {'true'|'false'} [has_item_links]
 * @property {'true'|'false'} [has_test_links]
 * @property {string} [label_ids] comma-separated page label IDs
 * @property {number} [limit]
 * @property {number} [offset]
 */

export const requirements = {
  /** @param {number} workspaceId @param {RequirementListParams} [params] */
  list: (workspaceId, params = {}) =>
    fetchV2Data(`/workspaces/${workspaceId}/requirements${buildQueryString(params)}`),

  get: (workspaceId, requirementNumber) =>
    fetchV2Data(`/workspaces/${workspaceId}/requirements/${requirementNumber}`),

  create: (workspaceId, body) =>
    fetchV2Data(`/workspaces/${workspaceId}/requirements`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),

  update: (workspaceId, requirementNumber, patch) =>
    fetchV2Data(`/workspaces/${workspaceId}/requirements/${requirementNumber}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/merge-patch+json' },
      body: JSON.stringify(patch),
    }),

  getHistory: (workspaceId, requirementNumber) =>
    fetchV2Data(`/workspaces/${workspaceId}/requirements/${requirementNumber}/history`),

  promote: (workspaceId, pageId, body) =>
    fetchV2Data(`/workspaces/${workspaceId}/pages/${pageId}/promote-to-requirement`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),

  getByPage: (workspaceId, pageId) =>
    fetchV2Data(`/workspaces/${workspaceId}/pages/${pageId}/requirement`),
};
