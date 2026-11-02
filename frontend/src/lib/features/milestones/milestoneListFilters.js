// Client-side filters for the global milestones page. The page
// loads global milestones plus milestones from every workspace the viewer can
// access, then narrows the list by workspace, status, and name.

/** Workspace filter value that stands for global milestones. */
export const GLOBAL_WORKSPACE_KEY = 'global';

export const MILESTONE_STATUSES = ['planning', 'in-progress', 'completed', 'cancelled'];

/** Completed and cancelled milestones are hidden by default. */
export const DEFAULT_STATUS_FILTER = ['planning', 'in-progress'];

/** Workspace filter key of a milestone: 'global' or its workspace id. */
export function milestoneWorkspaceKey(milestone) {
  return milestone.is_global ? GLOBAL_WORKSPACE_KEY : String(milestone.workspace_id ?? '');
}

/**
 * Distinct workspaces of the local milestones in `milestones`, sorted by name.
 * Names come from the list response (`workspace_name`), falling back to the
 * loaded workspaces, then to the id.
 * @returns {{ key: string, name: string }[]}
 */
export function milestoneWorkspaceOptions(milestones, workspaces = []) {
  const byKey = new Map();
  for (const milestone of milestones) {
    if (milestone.is_global || milestone.workspace_id == null) continue;
    const key = milestoneWorkspaceKey(milestone);
    if (byKey.has(key)) continue;
    const name =
      milestone.workspace_name ||
      workspaces.find((workspace) => String(workspace.id) === key)?.name ||
      `#${key}`;
    byKey.set(key, { key, name });
  }
  return [...byKey.values()].sort((a, b) => a.name.localeCompare(b.name));
}

/**
 * Applies the page filters. An empty `statuses` or `workspaceKeys` list does
 * not restrict; `search` matches the name case-insensitively.
 */
export function filterMilestones(
  milestones,
  { statuses = [], workspaceKeys = [], search = '' } = {}
) {
  const query = search.trim().toLowerCase();
  return milestones.filter((milestone) => {
    if (statuses.length > 0 && !statuses.includes(milestone.status || 'planning')) return false;
    if (workspaceKeys.length > 0 && !workspaceKeys.includes(milestoneWorkspaceKey(milestone))) {
      return false;
    }
    if (query && !(milestone.name || '').toLowerCase().includes(query)) return false;
    return true;
  });
}

/**
 * The page's default filters, which "Clear filters" restores: open statuses,
 * every workspace, no search.
 * @returns {{ statuses: string[], workspaceKeys: string[], search: string }}
 */
export function defaultMilestoneListFilters() {
  return { statuses: [...DEFAULT_STATUS_FILTER], workspaceKeys: [], search: '' };
}

/**
 * Whether `filters` already equal the defaults, so clearing them would change
 * nothing. Status order and surrounding search whitespace are ignored.
 */
export function milestoneListFiltersAreDefault({
  statuses = [],
  workspaceKeys = [],
  search = '',
} = {}) {
  const defaults = defaultMilestoneListFilters();
  const sameStatuses =
    new Set(statuses).size === defaults.statuses.length &&
    defaults.statuses.every((status) => statuses.includes(status));
  return sameStatuses && workspaceKeys.length === 0 && search.trim() === '';
}

/** Parses a stored status selection; anything invalid falls back to the default. */
export function parseStatusFilter(raw) {
  if (raw == null) return [...DEFAULT_STATUS_FILTER];
  try {
    const parsed = JSON.parse(raw);
    if (Array.isArray(parsed) && parsed.every((status) => MILESTONE_STATUSES.includes(status))) {
      return [...new Set(parsed)];
    }
  } catch {
    // Fall through to the default.
  }
  return [...DEFAULT_STATUS_FILTER];
}
