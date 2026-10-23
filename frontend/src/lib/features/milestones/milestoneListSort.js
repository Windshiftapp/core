// Column sorting on the global milestones page. DataTable sorts the
// rows by these values; null values sort last in either direction.

import { MILESTONE_STATUSES } from './milestoneListFilters.js';

/** Milliseconds since the epoch, or null for a missing or invalid date. */
export function timestampSortValue(value) {
  if (!value) return null;
  const time = Date.parse(value);
  return Number.isNaN(time) ? null : time;
}

/**
 * When the milestone or anything in it last changed. List responses carry
 * `last_updated_at`; a milestone just saved on this page only has the
 * `updated_at` of that save, which is then the latest change.
 */
export function milestoneLastUpdated(milestone) {
  return milestone.last_updated_at ?? milestone.updated_at ?? null;
}

/** Lifecycle order: planning, in progress, completed, cancelled. */
export function milestoneStatusSortValue(milestone) {
  const index = MILESTONE_STATUSES.indexOf(milestone.status);
  return index === -1 ? null : index;
}

/**
 * Timeline order: open milestones by target date (most urgent first when
 * ascending); completed, cancelled, and open-ended milestones last.
 */
export function milestoneTimelineSortValue(milestone) {
  if (milestone.status === 'completed' || milestone.status === 'cancelled') return null;
  return timestampSortValue(milestone.target_date);
}
