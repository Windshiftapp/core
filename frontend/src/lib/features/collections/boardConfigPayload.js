// Builds the full board-configuration request from a loaded config so the
// milestone-swimlane dialog can persist its preferences without dropping the
// columns, card fields, or other settings owned by the board settings page.
//
// The milestone preference rides in view_settings, which the backend merges
// with the stored value, so enabled_views is left untouched.

/**
 * @param {object|null} config loaded board configuration
 * @param {{globalMilestoneIds?: number[], showCompleted?: boolean, showOpenFromCompleted?: boolean}} [milestoneSwimlanes]
 */
export function buildBoardConfigurationPayload(config, milestoneSwimlanes = {}) {
  const source = config ?? {};
  const globalMilestoneIds = [
    ...new Set((milestoneSwimlanes.globalMilestoneIds || []).map(Number).filter(Number.isFinite)),
  ];

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
    show_rightmost_column_last_50: source.show_rightmost_column_last_50 ?? false,
    completed_item_retention_days: source.completed_item_retention_days ?? null,
    // Milestone-swimlane preferences live in the board's view-settings JSON so
    // no schema change is needed. The backend merges this object with the
    // stored one, so enabled_views is left untouched.
    view_settings: {
      milestone_swimlanes: {
        global_milestone_ids: globalMilestoneIds,
        show_completed: milestoneSwimlanes.showCompleted !== false,
        show_open_from_completed: milestoneSwimlanes.showOpenFromCompleted === true,
      },
    },
  };
}
