import { fetchAPI, fetchV2Data } from './core.js';
import { buildQueryString } from './utils.js';

/**
 * Workspace knowledge-pages API client. Mirrors logbook.js style: every
 * method returns a Promise from fetchAPI; auth is automatic via cookies
 * (core.js sets credentials: 'same-origin').
 */
export const pages = {
  /** Fetch every visible page as an ordered, flat metadata list. */
  getAll: (workspaceId) => fetchV2Data(`/workspaces/${workspaceId}/pages`),

  /** Fetch a single page (404 on missing or no view permission). */
  getPage: (workspaceId, pageId) => fetchV2Data(`/workspaces/${workspaceId}/pages/${pageId}`),

  /** Create a new page. parentId is optional (null/undefined = root). */
  createPage: (
    workspaceId,
    { title, content = '', parentId = null, isHome = false, metadata = {} }
  ) =>
    fetchV2Data(`/workspaces/${workspaceId}/pages`, {
      method: 'POST',
      body: JSON.stringify({ title, content, parent_id: parentId, is_home: isHome, metadata }),
    }),

  /**
   * Update title/content on a page. Inheritance has its own admin-gated
   * endpoint (setInheritance below) — do not send the flag here; the
   * server rejects it as an unknown field shape and an editor without
   * admin would otherwise be able to flip inheritance via a normal save.
   */
  updatePage: (
    workspaceId,
    pageId,
    { title, content, metadata = undefined, expectedContentHash = undefined }
  ) =>
    fetchV2Data(`/workspaces/${workspaceId}/pages/${pageId}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/merge-patch+json' },
      body: JSON.stringify({
        title,
        content,
        ...(metadata === undefined ? {} : { metadata }),
        ...(expectedContentHash === undefined
          ? {}
          : { expected_content_hash: expectedContentHash }),
      }),
    }),

  /** Archive a page (and every descendant). */
  archivePage: (workspaceId, pageId) =>
    fetchV2Data(`/workspaces/${workspaceId}/pages/${pageId}`, { method: 'DELETE' }),

  /** Admin-only: list every archived page in the workspace with archiver display name. */
  listArchived: (workspaceId) => fetchV2Data(`/workspaces/${workspaceId}/pages/archived`),

  /** Admin-only: clear archived_at/archived_by on a single page (no content overwrite). */
  unarchive: (workspaceId, pageId) =>
    fetchV2Data(`/workspaces/${workspaceId}/pages/${pageId}/unarchive`, { method: 'POST' }),

  /**
   * Reparent a page; pass parentId=null to move it to the workspace root.
   * prevSiblingId / nextSiblingId position the page within its new parent's
   * children — either may be null for "start of list" / "end of list", and
   * omitting both preserves the legacy append-by-natural-order behavior.
   */
  movePage: (
    workspaceId,
    pageId,
    parentId,
    { destinationWorkspaceId = null, prevSiblingId = null, nextSiblingId = null } = {}
  ) =>
    fetchV2Data(`/workspaces/${workspaceId}/pages/${pageId}/move`, {
      method: 'POST',
      body: JSON.stringify({
        ...(destinationWorkspaceId != null
          ? { destination_workspace_id: destinationWorkspaceId }
          : {}),
        parent_id: parentId,
        prev_sibling_id: prevSiblingId,
        next_sibling_id: nextSiblingId,
      }),
    }),

  /** Paginated revision history for a page. */
  getHistory: (workspaceId, pageId, { limit = 50, offset = 0 } = {}) =>
    fetchV2Data(
      `/workspaces/${workspaceId}/pages/${pageId}/history${buildQueryString({ page: Math.floor(offset / limit) + 1, page_size: limit })}`
    ),

  /** Fetch a single revision; must belong to the page. */
  getRevision: (workspaceId, pageId, revisionId) =>
    fetchV2Data(`/workspaces/${workspaceId}/pages/${pageId}/history/${revisionId}`),

  /** Restore a revision; produces a new revision of type 'restore'. */
  restoreRevision: (workspaceId, pageId, revisionId) =>
    fetchV2Data(`/workspaces/${workspaceId}/pages/${pageId}/history/${revisionId}/restore`, {
      method: 'POST',
    }),

  /** Create an immutable diagram attachment and atomically insert its Page fence. */
  createDiagram: (
    workspaceId,
    pageId,
    {
      name,
      mermaid = undefined,
      excalidraw = undefined,
      placement = 'end',
      expectedContentHash = undefined,
    }
  ) =>
    fetchV2Data(`/workspaces/${workspaceId}/pages/${pageId}/diagrams`, {
      method: 'POST',
      body: JSON.stringify({
        name,
        ...(mermaid ? { mermaid } : {}),
        ...(excalidraw ? { excalidraw } : {}),
        placement,
        ...(expectedContentHash ? { expected_content_hash: expectedContentHash } : {}),
      }),
    }),

  /** Replace one Page diagram through the shared immutable lifecycle. */
  updateDiagram: (
    workspaceId,
    pageId,
    attachmentId,
    {
      name = undefined,
      mermaid = undefined,
      excalidraw = undefined,
      expectedContentHash = undefined,
    }
  ) =>
    fetchV2Data(`/workspaces/${workspaceId}/pages/${pageId}/diagrams/${attachmentId}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/merge-patch+json' },
      body: JSON.stringify({
        ...(name ? { name } : {}),
        ...(mermaid ? { mermaid } : {}),
        ...(excalidraw ? { excalidraw } : {}),
        ...(expectedContentHash ? { expected_content_hash: expectedContentHash } : {}),
      }),
    }),

  /** Read-only effective permissions + own ACL rows. */
  getPermissions: (workspaceId, pageId) =>
    fetchV2Data(`/workspaces/${workspaceId}/pages/${pageId}/permissions`),

  /** Grant a new ACL row on a page. Requires page.admin on the target. */
  grantPermission: (workspaceId, pageId, { principalType, principalId, permissionLevel }) =>
    fetchV2Data(`/workspaces/${workspaceId}/pages/${pageId}/permissions`, {
      method: 'POST',
      body: JSON.stringify({
        principal_type: principalType,
        principal_id: principalId,
        permission_level: permissionLevel,
      }),
    }),

  /** Revoke a single ACL row. The row must belong to the named page. */
  revokePermission: (workspaceId, pageId, permissionId) =>
    fetchV2Data(`/workspaces/${workspaceId}/pages/${pageId}/permissions/${permissionId}`, {
      method: 'DELETE',
    }),

  /** Toggle the inherit_permissions flag on a page. */
  setInheritance: (workspaceId, pageId, inheritPermissions) =>
    fetchV2Data(`/workspaces/${workspaceId}/pages/${pageId}/inheritance`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/merge-patch+json' },
      body: JSON.stringify({ inherit_permissions: inheritPermissions }),
    }),

  /** Unified knowledge search across pages (and future sources). */
  searchKnowledge: (workspaceId, query, { limit = 25 } = {}) =>
    fetchAPI(`/workspaces/${workspaceId}/knowledge/search${buildQueryString({ q: query, limit })}`),

  /**
   * Title-substring page search scoped to a workspace. Server-side and
   * permission-filtered. Used by the page picker in the link dialog and
   * the page-side work-item popover.
   */
  searchPages: (workspaceId, query, { limit = 20 } = {}) =>
    fetchV2Data(`/workspaces/${workspaceId}/pages/search${buildQueryString({ q: query, limit })}`),
};

/**
 * Page label API client. Workspace-scoped labels that attach to pages only;
 * separate system from work-item labels (no shared rows or endpoints).
 *
 * Label CRUD requires workspace `page.edit`; attach/detach requires
 * per-page edit (evaluated by PagePermissionService). Permission failures
 * surface as 404 to avoid leaking page-label existence.
 */
export const pageLabels = {
  list: (workspaceId) => fetchV2Data(`/workspaces/${workspaceId}/page-labels`),
  get: (workspaceId, id) => fetchV2Data(`/workspaces/${workspaceId}/page-labels/${id}`),
  create: (workspaceId, { name, color }) =>
    fetchV2Data(`/workspaces/${workspaceId}/page-labels`, {
      method: 'POST',
      body: JSON.stringify({ name, color }),
    }),
  update: (workspaceId, id, { name, color }) =>
    fetchV2Data(`/workspaces/${workspaceId}/page-labels/${id}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/merge-patch+json' },
      body: JSON.stringify({ name, color }),
    }),
  delete: (workspaceId, id) =>
    fetchV2Data(`/workspaces/${workspaceId}/page-labels/${id}`, { method: 'DELETE' }),

  listForPage: (workspaceId, pageId) =>
    fetchV2Data(`/workspaces/${workspaceId}/pages/${pageId}/labels`),
  setForPage: (workspaceId, pageId, labelIds) =>
    fetchV2Data(`/workspaces/${workspaceId}/pages/${pageId}/labels`, {
      method: 'PUT',
      body: JSON.stringify({ label_ids: labelIds }),
    }),
  addToPage: (workspaceId, pageId, labelId) =>
    fetchV2Data(`/workspaces/${workspaceId}/pages/${pageId}/labels`, {
      method: 'POST',
      body: JSON.stringify({ label_id: labelId }),
    }),
  removeFromPage: (workspaceId, pageId, labelId) =>
    fetchV2Data(`/workspaces/${workspaceId}/pages/${pageId}/labels/${labelId}`, {
      method: 'DELETE',
    }),
};
