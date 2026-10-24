// Backlog iteration-section ordering. Iterations are grouped by lifecycle
// (active first, then planned, then completed/cancelled) so the current work
// stays on top. Inside a group the newest iteration leads: later start_date
// first, and the most recently created iteration (highest id) breaks ties.
// This keeps the order deterministic even when several iterations share a
// start date instead of falling back to alphabetical name order.

const STATUS_PRIORITY = { active: 0, planned: 1, completed: 2, cancelled: 3 };
const UNKNOWN_STATUS_PRIORITY = 9;

function dateKey(iteration) {
  return String(iteration?.start_date ?? '');
}

function compareBacklogIterations(a, b) {
  const statusDelta =
    (STATUS_PRIORITY[a?.status] ?? UNKNOWN_STATUS_PRIORITY) -
    (STATUS_PRIORITY[b?.status] ?? UNKNOWN_STATUS_PRIORITY);
  if (statusDelta !== 0) return statusDelta;

  const dateDelta = dateKey(b).localeCompare(dateKey(a));
  if (dateDelta !== 0) return dateDelta;

  return (b?.id ?? 0) - (a?.id ?? 0);
}

export function sortBacklogIterations(iterations) {
  return [...iterations].sort(compareBacklogIterations);
}
