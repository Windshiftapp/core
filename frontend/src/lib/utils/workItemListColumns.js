import { getSystemFieldName } from '../stores/fieldConfig.js';

export const DEFAULT_LIST_COLUMNS = [
  { field_identifier: 'key', field_type: 'system', display_order: 0, width: 1 },
  { field_identifier: 'title', field_type: 'system', display_order: 1, width: 4 },
  { field_identifier: 'status', field_type: 'system', display_order: 2, width: 2 },
  { field_identifier: 'priority', field_type: 'system', display_order: 3, width: 2 },
  { field_identifier: 'created_at', field_type: 'system', display_order: 4, width: 2 },
];

export function sortListColumns(columns) {
  return [...(columns || [])].sort((a, b) => (a.display_order ?? 0) - (b.display_order ?? 0));
}

// Map fetched board columns back to the save payload shape. Responses carry
// server-managed fields (board_configuration_id, created_at, updated_at) that
// the typed v2 request schema rejects.
export function boardColumnsForSave(columns) {
  return [...(columns || [])].map(({ id, name, display_order, wip_limit, color, status_ids }) => ({
    id: id ?? null,
    name,
    display_order,
    wip_limit: wip_limit ?? null,
    color: color ?? '',
    status_ids: status_ids || [],
  }));
}

export function listColumnsFromConfig(config) {
  return config?.list_columns?.length > 0
    ? sortListColumns(config.list_columns)
    : [...DEFAULT_LIST_COLUMNS];
}

export function buildListColumnConfiguration(config, listColumns) {
  return {
    columns: boardColumnsForSave(config?.columns),
    backlog_status_ids: config?.backlog_status_ids || [],
    list_columns: listColumns,
    card_fields: config?.card_fields || [],
    roadmap_config: config?.roadmap_config || null,
    show_rightmost_column_last_50: Boolean(config?.show_rightmost_column_last_50),
    completed_item_retention_days: config?.completed_item_retention_days ?? null,
  };
}

export function getListColumnLabel(column, customFieldDefinitions = []) {
  if (column.field_type === 'workspace') return 'Workspace';

  if (column.field_type === 'system') {
    return getSystemFieldName(column.field_identifier);
  }

  const customField = customFieldDefinitions.find(
    (field) => String(field.id) === String(column.field_identifier)
  );
  return customField?.name || column.field_identifier;
}

export function getListColumnTableWidth(column) {
  const width = Number(column.width) || 2;
  const widths = {
    1: 'w-24',
    2: 'w-32',
    3: 'w-40',
    4: 'w-56',
  };

  if (column.field_identifier === 'title') return widths[width] || widths[4];
  if (column.field_identifier === 'key') return 'w-32';
  return widths[width] || widths[2];
}
