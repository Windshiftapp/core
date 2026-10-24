import { pages } from '../../api/pages.js';
export function normalizePageHistoryResponse(response) {
  if (Array.isArray(response?.revisions)) return response.revisions;
  if (Array.isArray(response?.items)) return response.items;
  return Array.isArray(response) ? response : [];
}

/** Load revisions with their permission-filtered author summaries in one request. */
export async function loadPageHistory(workspaceId, pageId, options = {}) {
  const response = await pages.getHistory(workspaceId, pageId, options);
  return normalizePageHistoryResponse(response);
}

export function pageRevisionAuthorName(revision) {
  const author = revision?.author;
  if (author?.name) return author.name;
  if (author?.username) return author.username;
  return revision?.created_by != null ? `#${revision.created_by}` : '—';
}
