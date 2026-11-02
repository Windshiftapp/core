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
 * When the milestone or anything in it last changed: the later of its newest
 * Activity entry (`last_activity_at`, unset until the first one) and its own
 * `updated_at`.
 */
export function milestoneLastUpdated(milestone) {
  const activity = milestone.last_activity_at;
  const updated = milestone.updated_at;
  if (!activity || !updated) return activity || updated || null;
  return timestampSortValue(activity) > timestampSortValue(updated) ? activity : updated;
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
