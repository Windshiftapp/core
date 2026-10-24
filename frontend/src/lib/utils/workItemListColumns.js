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

// Per-column baselines (rem) for the list grid — what "M" looks like today.
// S/L/XL widths scale around this so the size picker has visible effect.
const GRID_BASE_FIXED_WIDTHS = {
  status: 8,
  priority: 7,
  assignee: 9,
  milestone: 12,
  iteration: 9,
  due_date: 7,
  created_at: 7,
  updated_at: 7,
  project: 9,
};

// width values: 1=S, 2=M, 3=L, 4=XL
const GRID_WIDTH_SCALE = { 1: 0.75, 2: 1, 3: 1.5, 4: 2 };

// Fixed columns may shrink down to their S size when a row is tight. Without
// a floor they hold their configured width and squeeze the flexible Title
// track until its text spills over the next cell.
const GRID_MIN_FIXED_SCALE = 0.75;
const GRID_MIN_TITLE_WIDTH = 16;
const GRID_MIN_FLEXIBLE_WIDTH = 10;

// The header and every row each build their own grid from the same template,
// so any content-sized track (`max-content`/`auto`) resolves differently per
// grid and shifts every later column. Key and actions must therefore be fixed.
const GRID_KEY_WIDTH = 5;
const GRID_ACTIONS_WIDTH = 4;

function gridColumnTrack(col) {
  if (col.field_identifier === 'key') return `${GRID_KEY_WIDTH}rem`;

  const base = GRID_BASE_FIXED_WIDTHS[col.field_identifier];
  if (base !== undefined) {
    const min = base * GRID_MIN_FIXED_SCALE;
    const max = base * (GRID_WIDTH_SCALE[col.width] ?? 1);
    return max > min ? `minmax(${min}rem, ${max}rem)` : `${min}rem`;
  }

  const min = col.field_identifier === 'title' ? GRID_MIN_TITLE_WIDTH : GRID_MIN_FLEXIBLE_WIDTH;
  const fr = Number(col.width) || 2;
  return `minmax(${min}rem, ${fr}fr)`;
}

// Floor for the whole row. When it exceeds the viewport the list scrolls
// horizontally instead of letting columns collapse into each other.
function gridColumnMinWidth(col) {
  if (col.field_identifier === 'key') return GRID_KEY_WIDTH;
  const base = GRID_BASE_FIXED_WIDTHS[col.field_identifier];
  if (base !== undefined) return base * GRID_MIN_FIXED_SCALE;
  return col.field_identifier === 'title' ? GRID_MIN_TITLE_WIDTH : GRID_MIN_FLEXIBLE_WIDTH;
}

/** CSS grid-template-columns for a list-shaped row of columns. */
export function listGridTemplateColumns(columns) {
  return `${columns.map(gridColumnTrack).join(' ')} ${GRID_ACTIONS_WIDTH}rem`;
}

/** Row min-width matching listGridTemplateColumns; the actions track is fixed
 * and `columns.length` covers the gap between every track. */
export function listGridMinWidth(columns) {
  const total = columns.reduce((sum, col) => sum + gridColumnMinWidth(col), 0);
  return `${total + GRID_ACTIONS_WIDTH + columns.length}rem`;
}
