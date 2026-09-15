import { api } from '../api.js';

const LIST_LIMIT = 100;

/**
 * Fetch visible requirements for each workspace, grouped for list rendering.
 * Workspaces that fail (permission or deletion) are skipped.
 *
 * @param {Array<{id: number, name: string}>} workspaces
 * @returns {Promise<Array<{workspace: Object, requirements: any[], totalItems: number}>>}
 */
export async function fetchWorkspaceRequirementSections(workspaces) {
  const settled = await Promise.allSettled(
    (workspaces ?? []).map((ws) => api.requirements.list(ws.id, { limit: LIST_LIMIT }))
  );
  const sections = [];
  for (let i = 0; i < (workspaces ?? []).length; i++) {
    const result = settled[i];
    if (result.status !== 'fulfilled') continue;
    const items = result.value?.items ?? [];
    if (items.length === 0) continue;
    sections.push({
      workspace: workspaces[i],
      requirements: items,
      totalItems: result.value?.pagination?.total_items ?? items.length,
    });
  }
  return sections;
}

/**
 * Search requirements across workspaces by key, title, or number.
 *
 * @param {Array<{id: number, name: string}>} workspaces
 * @param {string} query
 * @param {{cap?: number}} options
 * @returns {Promise<any[]>}
 */
export async function searchRequirementsAcrossWorkspaces(workspaces, query, { cap = 12 } = {}) {
  const trimmed = query.trim().toLowerCase();
  if (!trimmed) return [];

  const sections = await fetchWorkspaceRequirementSections(workspaces);
  const out = [];
  for (const section of sections) {
    for (const req of section.requirements) {
      const haystack = [req.key, req.page_title, String(req.requirement_number)]
        .filter(Boolean)
        .join(' ')
        .toLowerCase();
      if (!haystack.includes(trimmed)) continue;
      out.push({
        ...req,
        workspace_id: section.workspace.id,
        workspace_name: section.workspace.name,
      });
      if (out.length >= cap) return out;
    }
  }
  return out;
}
