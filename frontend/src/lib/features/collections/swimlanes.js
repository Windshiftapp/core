// Swimlane logic shared by the story map (CollectionMap.svelte), the board
// (CollectionBoard.svelte) and the public board. Pure functions so lane
// derivation, membership, ordering and drop payloads stay unit-testable
// without mounting Svelte.
//
// A swimlane dimension slices cards into horizontal bands. Membership is
// single-valued for every dimension except milestone: items carry a set of
// attached milestones, and the map renders an item in each matching lane. The
// board and public board use a single primary milestone per card so a drag can
// move a card out of one lane unambiguously.

export const SWIMLANE_DIMENSIONS = [
  'status',
  'status_category',
  'assignee',
  'priority',
  'iteration',
  'milestone',
];

export const SWIMLANE_NONE_KEY = 'none';

export function mapSwimlaneStorageKey(scope) {
  return `map-swimlane-dimension-${scope}`;
}

function statusIdOf(item) {
  return item.status_id ?? item.status ?? null;
}

export function statusCategoryForItem(item, statuses, statusCategories) {
  const statusId = statusIdOf(item);
  if (statusId == null) return null;
  const status = statuses.find((s) => s.id === statusId);
  if (!status) return null;
  return statusCategories.find((c) => c.id === status.category_id) || null;
}

function milestoneIdsOf(item) {
  if (Array.isArray(item?.milestones)) {
    return item.milestones.map((milestone) => milestone.id).filter((id) => id != null);
  }
  if (Array.isArray(item?.milestone_ids)) return item.milestone_ids;
  return [];
}

export function milestoneIdFromLaneKey(laneKey) {
  if (laneKey == null || laneKey === SWIMLANE_NONE_KEY) return null;
  const id = Number(String(laneKey).split('-').pop());
  return Number.isFinite(id) ? id : null;
}

// Workspace-local milestones can only be attached to cards in that workspace;
// global milestones are assignable everywhere.
export function milestoneAssignableToWorkspace(milestone, workspaceId) {
  if (!milestone) return false;
  if (milestone.is_global) return true;
  if (milestone.workspace_id == null) return false;
  return Number(milestone.workspace_id) === Number(workspaceId);
}

// Lane key for an item under a dimension, or SWIMLANE_NONE_KEY when the item
// has no value for it. For milestones an item may belong to several lanes;
// callers use itemLaneKeys for membership, this returns the primary key.
export function laneKeyForItem(item, dimension) {
  switch (dimension) {
    case 'status': {
      const statusId = statusIdOf(item);
      return statusId == null ? SWIMLANE_NONE_KEY : `status-${statusId}`;
    }
    case 'status_category': {
      return null; // resolved with store data; see statusCategoryLaneKey
    }
    case 'assignee':
      return item.assignee_id == null ? SWIMLANE_NONE_KEY : `assignee-${item.assignee_id}`;
    case 'priority':
      return item.priority_id == null ? SWIMLANE_NONE_KEY : `priority-${item.priority_id}`;
    case 'iteration':
      return item.iteration_id == null ? SWIMLANE_NONE_KEY : `iteration-${item.iteration_id}`;
    case 'milestone': {
      const ids = milestoneIdsOf(item);
      return ids.length === 0 ? SWIMLANE_NONE_KEY : `milestone-${ids[0]}`;
    }
    default:
      return SWIMLANE_NONE_KEY;
  }
}

export function statusCategoryLaneKey(item, statuses, statusCategories) {
  const category = statusCategoryForItem(item, statuses, statusCategories);
  return category ? `category-${category.id}` : SWIMLANE_NONE_KEY;
}

// All lane keys an item belongs to under a dimension. Single key except the
// milestone dimension, where an item with several milestones sits in each.
export function itemLaneKeys(item, dimension, statuses, statusCategories) {
  if (dimension === 'milestone') {
    const ids = milestoneIdsOf(item);
    if (ids.length === 0) return [SWIMLANE_NONE_KEY];
    return ids.map((id) => `milestone-${id}`);
  }
  if (dimension === 'status_category') {
    return [statusCategoryLaneKey(item, statuses, statusCategories)];
  }
  return [laneKeyForItem(item, dimension)];
}

function isUndated(value) {
  return value == null || value === '';
}

function compareMilestoneLanes(a, b) {
  const aUndated = isUndated(a.target_date);
  const bUndated = isUndated(b.target_date);
  if (aUndated !== bUndated) return aUndated ? 1 : -1;
  if (!aUndated) {
    const byDate = String(a.target_date).localeCompare(String(b.target_date));
    if (byDate !== 0) return byDate;
  }
  const byName = String(a.name || '').localeCompare(String(b.name || ''));
  if (byName !== 0) return byName;
  return Number(a.id) - Number(b.id);
}

// Canonical milestone lane order: target date ascending, undated last, then
// name and id as stable tie-breakers.
export function sortMilestonesForLanes(milestones = []) {
  return [...milestones].sort(compareMilestoneLanes);
}

// The board and public board place a card in one milestone lane. Resolve the
// primary milestone from the attached rows, falling back to a reference list
// when the card only carries ids. The earliest target date wins; undated rows
// sort last.
export function primaryMilestoneForItem(item, milestones = []) {
  const referenceById = new Map(
    (milestones || [])
      .filter((milestone) => milestone?.id != null)
      .map((milestone) => [Number(milestone.id), milestone])
  );

  const attached =
    Array.isArray(item?.milestones) && item.milestones.length > 0
      ? item.milestones
      : milestoneIdsOf(item).map((id) => ({ id }));

  const candidates = attached
    .map((milestone) => {
      const id = Number(milestone?.id ?? milestone);
      if (!Number.isFinite(id)) return null;
      const reference = referenceById.get(id) || {};
      return {
        id,
        name: milestone?.name ?? reference.name ?? '',
        target_date: milestone?.target_date ?? reference.target_date ?? null,
      };
    })
    .filter(Boolean);

  if (candidates.length === 0) return null;
  candidates.sort(compareMilestoneLanes);
  return candidates[0];
}

// Single-lane membership used by the board and the public board. Other
// dimensions keep their existing single-valued lane key.
export function primaryLaneKeyForItem(item, dimension, milestones = []) {
  if (dimension !== 'milestone') return laneKeyForItem(item, dimension);
  const primary = primaryMilestoneForItem(item, milestones);
  return primary ? `milestone-${primary.id}` : SWIMLANE_NONE_KEY;
}

// Milestones a board lane drop would detach. Returns null when no confirmation
// is needed: a card with fewer than two milestones, a drop into its current
// primary lane, or a drop that removes nothing.
export function milestoneDropRemovals(item, dimension, laneKey, milestones = []) {
  if (dimension !== 'milestone') return null;
  const attachedIds = milestoneIdsOf(item);
  if (attachedIds.length < 2) return null;

  const targetId = laneKey === SWIMLANE_NONE_KEY ? null : milestoneIdFromLaneKey(laneKey);
  const primary = primaryMilestoneForItem(item, milestones);
  if (targetId != null && primary?.id === targetId) return null;

  const nextIds = targetId == null ? [] : [targetId];
  const removed = attachedIds.filter((id) => !nextIds.includes(id));
  return removed.length > 0 ? removed : null;
}

// Ordered lane definitions for a dimension. By default only lanes with at
// least one matching item are returned (plus the none-lane when something is
// unlaned). Board drop targets pass includeEmpty so active reference lanes
// stay present even without cards.
//
// labelFor(key) resolves a display title for the none lane; data-driven lanes
// are titled from the reference entity and carry an optional accent color.
export function buildSwimlanes({
  dimension,
  items,
  statuses = [],
  statusCategories = [],
  users = [],
  priorities = [],
  iterations = [],
  milestones = [],
  noneTitle = 'No value',
  includeEmpty = false,
}) {
  if (!dimension) return null;

  const membership = new Map(); // laneKey -> count
  for (const item of items) {
    for (const key of itemLaneKeys(item, dimension, statuses, statusCategories)) {
      membership.set(key, (membership.get(key) || 0) + 1);
    }
  }

  const lanes = [];
  const push = (key, title, color = null, sublabel = '') => {
    const count = membership.get(key) || 0;
    if (count === 0 && !includeEmpty) return;
    lanes.push({ key, title, color, sublabel, count });
  };

  if (dimension === 'status') {
    for (const status of statuses) {
      push(`status-${status.id}`, status.display_name || status.name);
    }
  } else if (dimension === 'status_category') {
    for (const category of statusCategories) {
      push(`category-${category.id}`, category.display_name || category.name, category.color);
    }
  } else if (dimension === 'assignee') {
    for (const user of users) {
      push(`assignee-${user.id}`, user.full_name || user.name || `User #${user.id}`);
    }
  } else if (dimension === 'priority') {
    for (const priority of priorities) {
      push(`priority-${priority.id}`, priority.display_name || priority.name, priority.color);
    }
  } else if (dimension === 'iteration') {
    const ordered = [...iterations].sort((a, b) =>
      (a.start_date || '').localeCompare(b.start_date || '')
    );
    for (const iteration of ordered) {
      push(
        `iteration-${iteration.id}`,
        iteration.name,
        iteration.type_color,
        iteration.type_name || ''
      );
    }
  } else if (dimension === 'milestone') {
    for (const milestone of sortMilestonesForLanes(milestones)) {
      push(`milestone-${milestone.id}`, milestone.name, milestone.category_color);
    }
  }

  // The global (cross-workspace) scope loads no iteration or milestone
  // catalogs, so reference-driven lanes above would silently drop every
  // assigned card. Derive the missing lanes from card memberships, titled
  // from embedded reference data when the card carries it (WI-1587).
  const fallbackLabels = new Map();
  if (dimension === 'iteration') {
    for (const item of items) {
      if (item.iteration_id == null) continue;
      const key = `iteration-${item.iteration_id}`;
      if (!fallbackLabels.has(key)) {
        fallbackLabels.set(key, item.iteration_name || `Iteration #${item.iteration_id}`);
      }
    }
  } else if (dimension === 'milestone') {
    for (const item of items) {
      for (const milestone of item.milestones || []) {
        if (milestone?.id == null) continue;
        const key = `milestone-${milestone.id}`;
        if (!fallbackLabels.has(key)) {
          fallbackLabels.set(
            key,
            milestone.name || milestone.title || `Milestone #${milestone.id}`
          );
        }
      }
    }
  }
  const emittedLanes = new Set(lanes.map((lane) => lane.key));
  for (const [key, count] of membership) {
    if (key === SWIMLANE_NONE_KEY || count === 0 || emittedLanes.has(key)) continue;
    if (!fallbackLabels.has(key)) continue;
    lanes.push({ key, title: fallbackLabels.get(key), color: null, sublabel: '', count });
  }

  if (membership.get(SWIMLANE_NONE_KEY)) {
    lanes.push({
      key: SWIMLANE_NONE_KEY,
      title: noneTitle,
      color: null,
      sublabel: '',
      count: membership.get(SWIMLANE_NONE_KEY),
    });
  }

  return lanes;
}

// Children of one backbone column that belong to a lane.
export function childrenForLane(children, lane, dimension, statuses, statusCategories) {
  return (children || []).filter((item) =>
    itemLaneKeys(item, dimension, statuses, statusCategories).includes(lane.key)
  );
}

// The attribute payload a drop into a lane implies, as a partial item update.
// Status moves are transitions, not updates, so the status dimension returns
// { transitionToStatusId } and the caller routes it through api.items.transition.
export function laneDropUpdate(lane, dimension, statuses) {
  const id = Number(lane.key.split('-').pop());
  const isNone = lane.key === SWIMLANE_NONE_KEY;
  switch (dimension) {
    case 'status':
      return isNone ? {} : { transitionToStatusId: id };
    case 'status_category': {
      if (isNone) return {};
      const inCategory = statuses.filter((s) => s.category_id === id);
      return inCategory.length > 0 ? { transitionToStatusId: inCategory[0].id } : {};
    }
    case 'assignee':
      return { assignee_id: isNone ? null : id };
    case 'priority':
      return { priority_id: isNone ? null : id };
    case 'iteration':
      return { iteration_id: isNone ? null : id };
    case 'milestone':
      return { milestone_ids: isNone ? [] : [id] };
    default:
      return {};
  }
}
