// Shared board-column derivation used by CollectionBoard (rendering) and
// collectionContext store (split fetching). Keeping both on one code path
// guarantees the store excludes exactly the statuses the board renders in
// its capped rightmost column.

/** Max cards rendered in the rightmost column when the board configuration
 * enables show_rightmost_column_last_50. Also the page size the store uses
 * for the separate rightmost-column fetch. */
export const RIGHTMOST_COLUMN_LIMIT = 50;

// Personal tasks use the system Open and Done statuses without a workflow.
export const PERSONAL_TASK_OPEN_STATUS_ID = 1;
export const PERSONAL_TASK_DONE_STATUS_ID = 3;

const CATEGORY_ORDER = {
  to_do: 1,
  in_progress: 2,
  done: 3,
};

/**
 * Sorts statuses into board order: To Do -> In Progress -> Done categories,
 * alphabetical within a category.
 */
export function sortStatusesForBoard(statuses = []) {
  return statuses.slice().sort((a, b) => {
    const aOrder = CATEGORY_ORDER[a.category_builtin_key] || 999;
    const bOrder = CATEGORY_ORDER[b.category_builtin_key] || 999;
    if (aOrder !== bOrder) return aOrder - bOrder;
    return a.name.localeCompare(b.name);
  });
}

/**
 * Computes the board's display columns: configured columns when the board
 * configuration has any, otherwise one column per status in board order.
 */
export function buildDisplayColumns(boardConfig, statuses = []) {
  if (boardConfig?.columns?.length > 0) {
    return boardConfig.columns
      .map((column) => {
        if (column.status_ids?.length !== 1) return column;
        const status = statuses.find((candidate) => candidate.id === column.status_ids[0]);
        return status?.builtin_key && status.name === column.name
          ? { ...column, builtin_key: status.builtin_key }
          : column;
      })
      .sort((a, b) => a.display_order - b.display_order);
  }
  return sortStatusesForBoard(statuses).map((status) => ({
    id: status.id,
    name: status.name,
    builtin_key: status.builtin_key,
    status_ids: [status.id],
    color: status.category_color,
    wip_limit: null,
    is_default_column: true,
  }));
}

function firstStatusId(column) {
  return column?.status_ids?.[0] ?? null;
}

function isPersonalTask(item, personalWorkspaceIds) {
  return personalWorkspaceIds?.has(Number(item?.workspace_id)) ?? false;
}

/**
 * Resolves the status used to place an item on a board. Personal Open and Done
 * map to the first and last visible columns while regular items keep their
 * persisted workflow status.
 */
export function boardStatusIdForItem(item, columns = [], personalWorkspaceIds = new Set()) {
  if (!isPersonalTask(item, personalWorkspaceIds)) return item?.status_id ?? null;

  const validColumns = columns.filter((column) => column.status_ids?.length > 0);
  if (validColumns.length === 0) return item?.status_id ?? null;

  const endpointColumn =
    item?.status_id === PERSONAL_TASK_DONE_STATUS_ID
      ? validColumns[validColumns.length - 1]
      : validColumns[0];
  return firstStatusId(endpointColumn);
}

/**
 * Resolves the persisted status for a board move. Personal tasks can only be
 * moved to board endpoints, which translate back to Open or Done.
 */
export function statusIdForBoardColumnMove(
  item,
  targetColumn,
  columns = [],
  personalWorkspaceIds = new Set()
) {
  if (!isPersonalTask(item, personalWorkspaceIds)) return firstStatusId(targetColumn);

  const validColumns = columns.filter((column) => column.status_ids?.length > 0);
  const targetIndex = validColumns.indexOf(targetColumn);
  if (targetIndex < 0) return null;
  if (validColumns.length === 1) {
    return item?.status_id === PERSONAL_TASK_DONE_STATUS_ID
      ? PERSONAL_TASK_DONE_STATUS_ID
      : PERSONAL_TASK_OPEN_STATUS_ID;
  }
  if (targetIndex === 0) return PERSONAL_TASK_OPEN_STATUS_ID;
  if (targetIndex === validColumns.length - 1) return PERSONAL_TASK_DONE_STATUS_ID;
  return null;
}

/**
 * Returns the status IDs of the board's capped rightmost column, or null
 * when the cap doesn't apply (no config, cap disabled, or no resolvable
 * rightmost column).
 */
export function rightmostCapStatusIds(boardConfig, statuses = []) {
  if (!boardConfig?.show_rightmost_column_last_50) return null;
  const columns = buildDisplayColumns(boardConfig, statuses).filter(
    (col) => col.status_ids?.length > 0
  );
  const rightmost = columns[columns.length - 1];
  return rightmost?.status_ids?.length ? rightmost.status_ids : null;
}
