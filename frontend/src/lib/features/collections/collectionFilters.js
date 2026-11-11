/**
 * Applies the optional completed-item visibility filter used by collection
 * views. Backlogs pass hideCompleted=false because their configured status IDs
 * are already the authoritative visibility rule.
 */
export function collectionSubFilterQL(subFilterQL = '', { hideCompleted = true } = {}) {
  if (!hideCompleted) return subFilterQL;
  return subFilterQL ? `(${subFilterQL}) AND status_completed = false` : 'status_completed = false';
}
