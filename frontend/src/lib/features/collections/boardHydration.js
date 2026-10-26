// The board loads every page, but optional card reads stay viewport-scoped.
// These helpers keep that boundary explicit and testable.

/** @param {Iterable<number>} visibleItemIds @param {Map<number, object>} itemsById */
export function loadedVisibleItemIDs(visibleItemIds, itemsById) {
  const ids = [];
  for (const itemId of visibleItemIds) {
    if (itemsById.has(itemId)) ids.push(itemId);
  }
  return ids;
}

/** @param {Iterable<number>} visibleItemIds @param {Map<number, object>} itemsById */
export function visibleAssigneeIDs(visibleItemIds, itemsById) {
  const ids = new Set();
  for (const itemId of visibleItemIds) {
    const assigneeID = itemsById.get(itemId)?.assignee_id;
    if (assigneeID) ids.add(assigneeID);
  }
  return [...ids];
}
