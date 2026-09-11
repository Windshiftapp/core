import { navigate } from '../../router.js';

/**
 * Default window for completed items in dashboard task lists: items that
 * entered a done status more than this many days ago are hidden. Mirrors the
 * per-workspace TodoList "Done range" default (WI-473).
 */
export const DEFAULT_DONE_RANGE_DAYS = 7;

/**
 * Widget types that support the row-count / density controls.
 */
export const ROW_COUNT_WIDGETS = new Set(['assigned-to-me', 'personal-tasks', 'saved-search']);

/**
 * Row count options offered in the widget menu.
 */
export const ROW_COUNT_OPTIONS = [5, 10, 15, 'all'];

/**
 * Row controls are useful only after a Saved Search has a collection to show.
 */
export function shouldShowRowControls(widgetType, config) {
  return (
    ROW_COUNT_WIDGETS.has(widgetType) &&
    (widgetType !== 'saved-search' || Boolean(config?.collectionId))
  );
}

/**
 * Default row count for a widget that has no explicit override yet.
 * Scales with width: below half width shows 6, at or above shows 10.
 * @param {number} width - current widget column span (1..12)
 * @returns {number}
 */
export function defaultRowCount(width) {
  return width < 6 ? 6 : 10;
}

/**
 * Resolve the effective row count for a widget, applying the seeded default
 * when the user has not set an explicit override.
 * @param {{ rowCount?: number | 'all' } | null | undefined} config
 * @param {number} width
 * @returns {number | 'all'}
 */
export function resolveRowCount(config, width) {
  const explicit = config?.rowCount;
  if (explicit !== undefined && explicit !== null) return explicit;
  return defaultRowCount(width);
}

/**
 * Resolve density, falling back to 'comfortable'.
 * @param {{ density?: 'comfortable' | 'compact' } | null | undefined} config
 * @returns {'comfortable' | 'compact'}
 */
export function resolveDensity(config) {
  return config?.density === 'compact' ? 'compact' : 'comfortable';
}

/**
 * API request limit for a given row count. 'all' maps to a generous cap.
 * @param {number | 'all'} rowCount
 * @returns {number}
 */
export function rowCountToLimit(rowCount) {
  if (rowCount === 'all') return 100;
  return Math.max(rowCount, 5);
}

/**
 * ISO date (YYYY-MM-DD) `days` ago, for the backend `completed_since` filter.
 * Matches the cutoff format TodoList sends so the server treats both the same.
 * @param {number} [days]
 */
export function completedSinceCutoff(days = DEFAULT_DONE_RANGE_DAYS) {
  const d = new Date();
  d.setDate(d.getDate() - days);
  return d.toISOString().slice(0, 10);
}

/**
 * Query params for "items assigned to me and not yet completed", newest first.
 * Single source of truth for this QL contract — reused by the dashboard widget
 * and the mobile My Work view so the two can't drift.
 * @param {number|string} userId
 * @param {number} [limit]
 */
export function assignedToMeQuery(userId, limit = 30) {
  return {
    ql: `assignee_id = ${userId} AND status_completed = false`,
    limit,
    order_by: 'updated_at',
  };
}

/**
 * @param {unknown} response
 * @param {number | 'all'} [maxItems]
 */
export function normalizeTaskResponse(response, maxItems = 6) {
  const wrapped = /** @type {any} */ (response);
  const raw = wrapped?.data ?? [];
  const active = raw
    .filter((i) => i?.id)
    .map((i) => ({
      ...i,
      dueDate: i.due_date || null,
    }));
  active.sort((a, b) => {
    if (a.dueDate && b.dueDate) return a.dueDate.localeCompare(b.dueDate);
    if (a.dueDate) return -1;
    if (b.dueDate) return 1;
    return 0;
  });
  const cap = maxItems === 'all' ? active.length : maxItems;
  return active.slice(0, cap);
}

export function openTask(task) {
  navigate(`/workspaces/${task.workspace_id}/items/${task.id}`);
}
