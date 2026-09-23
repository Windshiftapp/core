// Swimlane logic for the story map (CollectionMap.svelte). Pure functions so
// lane derivation and membership stay unit-testable without mounting Svelte.
//
// A swimlane dimension slices every backbone column's child cards into
// horizontal bands. Membership is single-valued for every dimension except
// milestone (items carry a set of attached milestones); milestone items render
// in each matching lane.

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
  return (item.milestones || []).map((m) => m.id);
}

// Lane key for an item under a dimension, or SWIMLANE_NONE_KEY when the item
// has no value for it. For milestones an item may belong to several lanes;
// callers use itemLanesForItem for membership, this returns the primary key.
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

// Ordered lane definitions for a dimension. Only lanes with at least one
// matching item are returned (plus the none-lane when something is unlaned).
// reference lists arrive in their canonical display order; lanes follow it.
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
    if (count === 0) return;
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
    const ordered = [...milestones].sort((a, b) =>
      (a.target_date || '9999').localeCompare(b.target_date || '9999')
    );
    for (const milestone of ordered) {
      push(`milestone-${milestone.id}`, milestone.name, milestone.category_color);
    }
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
