function itemRecencyValue(item) {
  return new Date(item.last_active_at || item.updated_at || item.created_at || 0).getTime() || 0;
}

// Rendering, counts and keyboard navigation share these ordered buckets.
// Collapse/focus/selection state stays outside the expensive partition and sort.
export function buildBoardColumnModel({
  lanes = [],
  columns = [],
  statusIdForItem = (item) => item.status_id,
  sortMode = 'rank',
  deferred = null,
} = {}) {
  const columnsByStatus = new Map();
  const deferredIds = new Set(deferred?.statusIds || []);
  const descriptors = columns.map((column, columnIndex) => {
    const statusIds = new Set(column.status_ids || []);
    for (const id of statusIds) {
      const matches = columnsByStatus.get(id);
      if (matches) matches.push(columnIndex);
      else columnsByStatus.set(id, [columnIndex]);
    }
    return {
      column,
      columnIndex,
      usesDeferredTotal: Boolean(deferred?.statusIds.every((id) => statusIds.has(id))),
    };
  });
  const resolvedStatuses = new Map();
  const recencies = new Map();
  let totalVisibleItems = 0;
  const laneModels = lanes.map((lane) => {
    const buckets = columns.map(() => []);
    const deferredCounts = columns.map(() => 0);
    for (const item of lane.items) {
      if (!resolvedStatuses.has(item)) resolvedStatuses.set(item, statusIdForItem(item));
      const matches = columnsByStatus.get(resolvedStatuses.get(item));
      if (!matches) continue;
      if (sortMode === 'bubble' && !recencies.has(item)) {
        recencies.set(item, itemRecencyValue(item));
      }
      for (const index of matches) {
        buckets[index].push(item);
        if (descriptors[index].usesDeferredTotal && deferredIds.has(item.status_id)) {
          deferredCounts[index]++;
        }
      }
    }
    const laneColumns = descriptors.map(({ column, columnIndex, usesDeferredTotal }) => {
      const items = buckets[columnIndex];
      if (sortMode === 'bubble') {
        items.sort((a, b) => recencies.get(b) - recencies.get(a) || b.id - a.id);
      }
      const total = usesDeferredTotal
        ? Math.max(items.length, items.length - deferredCounts[columnIndex] + deferred.total)
        : items.length;
      totalVisibleItems += items.length;
      return {
        column,
        columnIndex,
        items,
        total,
        hiddenItemCount: total - items.length,
        wipCount: items.length,
        isOverWip: Boolean(column.wip_limit && items.length > column.wip_limit),
      };
    });
    return { lane, columns: laneColumns };
  });
  return { lanes: laneModels, totalVisibleItems };
}
