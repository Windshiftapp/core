// Builds the full board-configuration request from a loaded config so the
// board-level dialogs can persist their preferences without dropping the
// columns, card fields, or other settings owned by the board settings page.
//
// Milestone-swimlane and completed-visibility preferences ride in
// view_settings, which the backend merges with the stored value, so
// enabled_views is left untouched.

/** Bounds for the "limit" completed-visibility preset. The upper bound
 * matches the API's maximum page size, so one read returns the whole cap. */
export const DEFAULT_COMPLETED_LIMIT = 100;
export const MIN_COMPLETED_LIMIT = 1;
export const MAX_COMPLETED_LIMIT = 1000;

/** Coerces a stored completed-item cap into the accepted range. */
export function normalizeCompletedLimit(value) {
  const parsed = Number(value);
  if (!Number.isInteger(parsed)) return DEFAULT_COMPLETED_LIMIT;
  return Math.min(MAX_COMPLETED_LIMIT, Math.max(MIN_COMPLETED_LIMIT, parsed));
}

/** Legacy fields that back each completed-work preset. Kept in sync so a
 * preset switch does not leave the old cap/retention controls active. */
export function legacyFieldsForCompletedVisibility(visibility) {
  switch (visibility) {
    case 'last_day':
      return { showRightmostColumnLast50: false, completedItemRetentionDays: 1 };
    case 'last_week':
      return { showRightmostColumnLast50: false, completedItemRetentionDays: 7 };
    default:
      // all / limit / none all fetch completed work without a retention window.
      return { showRightmostColumnLast50: false, completedItemRetentionDays: null };
  }
}

/**
 * @param {object|null} config loaded board configuration
 * @param {{globalMilestoneIds?: number[], showCompleted?: boolean, showOpenFromCompleted?: boolean, completedVisibility?: string, completedLimit?: number}} [options]
 */
export function buildBoardConfigurationPayload(config, options = {}) {
  const source = config ?? {};
  const {
    globalMilestoneIds = [],
    showCompleted = true,
    showOpenFromCompleted = false,
    completedVisibility,
    completedLimit,
  } = options;
  const globalMilestoneIdsList = [
    ...new Set(globalMilestoneIds.map(Number).filter(Number.isFinite)),
  ];
  const visibility =
    completedVisibility !== undefined
      ? completedVisibility
      : (source.view_settings?.completed_visibility ?? '');
  const limit =
    completedLimit !== undefined
      ? completedLimit
      : (source.view_settings?.completed_limit ?? DEFAULT_COMPLETED_LIMIT);
  const legacy = visibility
    ? legacyFieldsForCompletedVisibility(visibility)
    : {
        showRightmostColumnLast50: source.show_rightmost_column_last_50 ?? false,
        completedItemRetentionDays: source.completed_item_retention_days ?? null,
      };

  return {
    columns: (source.columns || []).map((column, index) => ({
      id: column.id ?? null,
      name: column.name,
      display_order: index,
      wip_limit: column.wip_limit ?? null,
      color: column.color ?? '',
      status_ids: column.status_ids || [],
    })),
    backlog_status_ids: source.backlog_status_ids || [],
    list_columns: source.list_columns || [],
    roadmap_config: source.roadmap_config || null,
    card_fields: (source.card_fields || []).map((field, index) => ({
      field_identifier: field.field_identifier,
      field_type: field.field_type,
      display_order: index,
      width: field.width ?? 0,
    })),
    show_rightmost_column_last_50: legacy.showRightmostColumnLast50,
    completed_item_retention_days: legacy.completedItemRetentionDays,
    // Milestone-swimlane and completed-visibility preferences live in the
    // board's view-settings JSON so no schema change is needed. The backend
    // merges this object with the stored one, so enabled_views is left
    // untouched.
    view_settings: {
      milestone_swimlanes: {
        global_milestone_ids: globalMilestoneIdsList,
        show_completed: showCompleted !== false,
        show_open_from_completed: showOpenFromCompleted === true,
      },
      ...(visibility ? { completed_visibility: visibility } : {}),
      ...(visibility === 'limit' ? { completed_limit: normalizeCompletedLimit(limit) } : {}),
    },
  };
}
