// Board-specific swimlane model. Kept pure so lane grouping, ordering and the
// group-by preference can be unit-tested without mounting the board.
//
// The board supports three modes:
// - none: one lane holding every card
// - item-type: lanes are items of the selected type; their direct children are
//   the cards, everything else is unassigned
// - milestone: one lane per milestone using the item's primary milestone, plus
//   a trailing unassigned lane

import { primaryLaneKeyForItem, SWIMLANE_NONE_KEY, sortMilestonesForLanes } from './swimlanes.js';

export const BOARD_GROUP_BY_NONE = 'none';
export const BOARD_GROUP_BY_ITEM_TYPE = 'item-type';
export const BOARD_GROUP_BY_MILESTONE = 'milestone';

// Lane key for cards whose completed milestone lane is hidden by the board
// configuration. The lane keeps those cards visible instead of dropping them.
export const MILESTONE_HIDDEN_COMPLETED_KEY = 'milestone-completed-hidden';

// Lane key for open cards whose primary milestone is completed or cancelled.
// Surfaces work that is likely assigned to the wrong milestone so it can be
// moved to the right one.
export const MILESTONE_OPEN_FROM_COMPLETED_KEY = 'milestone-open-from-completed';

const INACTIVE_MILESTONE_STATUSES = new Set(['completed', 'cancelled']);

// Default for the milestone rescue lane when the caller does not supply a
// status-aware predicate: every card counts as open.
/** @type {(item: object) => boolean} */
const defaultIsItemOpen = () => true;

export function boardGroupPreferenceStorageKey(scope) {
  return `board-group-by-${scope}`;
}

export function legacyBoardGroupItemTypeStorageKey(scope) {
  return `board-group-by-item-type-${scope}`;
}

export function serializeBoardGroupBy(groupBy) {
  if (!groupBy) return null;
  if (groupBy.dimension === BOARD_GROUP_BY_MILESTONE) return BOARD_GROUP_BY_MILESTONE;
  if (groupBy.dimension === BOARD_GROUP_BY_ITEM_TYPE && groupBy.itemTypeId != null) {
    return `type:${groupBy.itemTypeId}`;
  }
  return null;
}

export function parseBoardGroupBy(value) {
  const raw = typeof value === 'string' ? value.trim() : '';
  if (!raw) return null;
  if (raw === BOARD_GROUP_BY_MILESTONE) return { dimension: BOARD_GROUP_BY_MILESTONE };
  const typeMatch = /^type:(\d+)$/.exec(raw);
  if (typeMatch) {
    return { dimension: BOARD_GROUP_BY_ITEM_TYPE, itemTypeId: Number(typeMatch[1]) };
  }
  return null;
}

// The item-type preference used a separate numeric storage key before the
// group-by menu gained milestone lanes. Read the new key first and fall back
// to the legacy one for users upgrading with a saved preference.
export function resolveStoredBoardGroupBy(currentValue, legacyItemTypeValue, itemTypes = []) {
  const current = parseBoardGroupBy(currentValue);
  if (current) {
    if (
      current.dimension === BOARD_GROUP_BY_ITEM_TYPE &&
      itemTypes.length > 0 &&
      !itemTypes.some((type) => type.id === current.itemTypeId)
    ) {
      return null;
    }
    return current;
  }

  const legacyId = parseInt(legacyItemTypeValue, 10);
  if (!Number.isFinite(legacyId)) return null;
  if (itemTypes.length > 0 && !itemTypes.some((type) => type.id === legacyId)) return null;
  return { dimension: BOARD_GROUP_BY_ITEM_TYPE, itemTypeId: legacyId };
}

export function isMilestoneSwimlaneActive(milestone) {
  return !INACTIVE_MILESTONE_STATUSES.has(String(milestone?.status || '').toLowerCase());
}

// Deduplicated milestone reference derived from the loaded cards. Used on
// cross-workspace boards where no global milestone catalog is loaded (WI-1587).
export function milestonesFromItems(items = []) {
  const byId = new Map();
  for (const item of items) {
    for (const milestone of item?.milestones || []) {
      if (milestone?.id == null || byId.has(milestone.id)) continue;
      byId.set(milestone.id, milestone);
    }
  }
  return sortMilestonesForLanes([...byId.values()]);
}

function unassignedLane(title, items, dimension) {
  return {
    // DOM/test id stays "unassigned"; the shared lane key is the neutral
    // SWIMLANE_NONE_KEY used by membership helpers.
    id: 'unassigned',
    key: SWIMLANE_NONE_KEY,
    dimension,
    title,
    parent: null,
    milestone: null,
    items,
    isDefault: false,
    isUnassigned: true,
    itemCount: items.length,
    parentIsVisible: false,
  };
}

function itemTypeLanes({
  items,
  filteredItems,
  itemTypeId,
  excludeRightmostSwimlaneParents,
  rightmostStatusIds,
  statusIdForItem,
  unassignedTitle,
}) {
  const isExcludedRightmostParent = (item) =>
    Boolean(excludeRightmostSwimlaneParents && rightmostStatusIds.has(statusIdForItem(item)));
  const isEligibleSwimlaneParent = (item) =>
    item.item_type_id === itemTypeId && !isExcludedRightmostParent(item);

  const parentItems = items.filter(isEligibleSwimlaneParent);
  const parentIds = new Set(parentItems.map((item) => item.id));
  const visibleItemIds = new Set(filteredItems.map((item) => item.id));

  // Bucket children by parent once instead of rescanning filteredItems per
  // parent; the scan is O(parents × items) on large boards.
  const childrenByParent = new Map();
  for (const item of filteredItems) {
    if (item.parent_id == null || item.parent_id === item.id) continue;
    const bucket = childrenByParent.get(item.parent_id);
    if (bucket) bucket.push(item);
    else childrenByParent.set(item.parent_id, [item]);
  }

  const lanes = parentItems
    .map((parent) => {
      const laneItems = childrenByParent.get(parent.id) ?? [];
      return {
        id: `parent-${parent.id}`,
        key: `parent-${parent.id}`,
        dimension: BOARD_GROUP_BY_ITEM_TYPE,
        title: parent.title,
        parent,
        milestone: null,
        items: laneItems,
        isDefault: false,
        isUnassigned: false,
        itemCount: laneItems.length,
        parentIsVisible: visibleItemIds.has(parent.id),
      };
    })
    .filter((lane) => lane.itemCount > 0 || lane.parentIsVisible);

  const unassignedItems = filteredItems.filter((item) => {
    if (item.item_type_id === itemTypeId) return false;
    if (item.parent_id && parentIds.has(item.parent_id)) return false;
    return true;
  });

  return [...lanes, unassignedLane(unassignedTitle, unassignedItems, BOARD_GROUP_BY_ITEM_TYPE)];
}

function hiddenCompletedLane(items, title) {
  return {
    id: MILESTONE_HIDDEN_COMPLETED_KEY,
    key: MILESTONE_HIDDEN_COMPLETED_KEY,
    dimension: BOARD_GROUP_BY_MILESTONE,
    title,
    parent: null,
    milestone: null,
    items,
    isDefault: false,
    isUnassigned: false,
    isAggregate: true,
    itemCount: items.length,
    parentIsVisible: false,
  };
}

function openFromCompletedLane(items, title) {
  return {
    id: MILESTONE_OPEN_FROM_COMPLETED_KEY,
    key: MILESTONE_OPEN_FROM_COMPLETED_KEY,
    dimension: BOARD_GROUP_BY_MILESTONE,
    title,
    parent: null,
    milestone: null,
    items,
    isDefault: false,
    isUnassigned: false,
    isAggregate: true,
    isRescue: true,
    itemCount: items.length,
    parentIsVisible: false,
  };
}

function milestoneLanes({
  filteredItems,
  milestones,
  includeEmptyMilestones,
  globalMilestoneIds = [],
  showCompletedMilestones = true,
  showOpenFromCompletedMilestones = false,
  isItemOpen = defaultIsItemOpen,
  unassignedTitle,
  hiddenCompletedTitle,
  openFromCompletedTitle,
}) {
  const orderedMilestones = sortMilestonesForLanes(milestones);
  const referenceById = new Map(
    orderedMilestones.map((milestone) => [Number(milestone.id), milestone])
  );
  const pinnedGlobalIds = new Set((globalMilestoneIds || []).map(Number));

  // Bucket by primary lane once; every lane filter below is a Map lookup.
  const bucket = new Map();
  for (const item of filteredItems) {
    const key = primaryLaneKeyForItem(item, BOARD_GROUP_BY_MILESTONE, orderedMilestones);
    const laneItems = bucket.get(key);
    if (laneItems) laneItems.push(item);
    else bucket.set(key, [item]);
  }

  const embeddedById = new Map();
  for (const item of filteredItems) {
    for (const milestone of item?.milestones || []) {
      if (milestone?.id != null && !embeddedById.has(milestone.id)) {
        embeddedById.set(milestone.id, milestone);
      }
    }
  }

  // Open cards on a completed or cancelled milestone are likely mis-assigned.
  // Pull them into a dedicated rescue lane so they can be moved to the right
  // milestone instead of hiding inside a finished lane or the aggregate.
  const rescueItems = [];
  if (showOpenFromCompletedMilestones) {
    for (const [key, laneItems] of [...bucket]) {
      if (key === SWIMLANE_NONE_KEY) continue;
      const id = Number(String(key).split('-').pop());
      if (!Number.isFinite(id)) continue;
      const milestone = referenceById.get(id) || embeddedById.get(id);
      if (!milestone || isMilestoneSwimlaneActive(milestone)) continue;
      const remaining = [];
      for (const item of laneItems) {
        if (isItemOpen(item)) rescueItems.push(item);
        else remaining.push(item);
      }
      if (remaining.length > 0) bucket.set(key, remaining);
      else bucket.delete(key);
    }
  }

  const lanes = [];
  const emittedIds = new Set();
  // Cards whose completed lane is hidden still have to render somewhere; they
  // collect into one aggregate lane instead of disappearing from the board.
  const hiddenCompletedItems = [];

  for (const milestone of orderedMilestones) {
    const key = `milestone-${milestone.id}`;
    const laneItems = bucket.get(key) ?? [];
    const active = isMilestoneSwimlaneActive(milestone);
    if (!active && !showCompletedMilestones) {
      hiddenCompletedItems.push(...laneItems);
      emittedIds.add(milestone.id);
      continue;
    }
    if (laneItems.length === 0) {
      if (!includeEmptyMilestones || !active) continue;
      // Global milestones are shared across every workspace, so an empty lane
      // is only offered when the board explicitly pinned that milestone.
      if (milestone.is_global && !pinnedGlobalIds.has(Number(milestone.id))) continue;
    }
    emittedIds.add(milestone.id);
    lanes.push({
      id: key,
      key,
      dimension: BOARD_GROUP_BY_MILESTONE,
      title: milestone.name,
      parent: null,
      milestone,
      items: laneItems,
      isDefault: false,
      isUnassigned: false,
      itemCount: laneItems.length,
      parentIsVisible: false,
    });
  }

  // Guard against a stale catalog: cards attached to a milestone missing from
  // the reference still get a lane instead of silently dropping into unassigned.
  const fallback = [];
  for (const [key, laneItems] of bucket) {
    if (key === SWIMLANE_NONE_KEY || laneItems.length === 0) continue;
    const id = Number(String(key).split('-').pop());
    if (!Number.isFinite(id) || emittedIds.has(id)) continue;
    const milestone = referenceById.get(id) ||
      embeddedById.get(id) || { id, name: `Milestone #${id}` };
    if (!isMilestoneSwimlaneActive(milestone) && !showCompletedMilestones) {
      hiddenCompletedItems.push(...laneItems);
      emittedIds.add(id);
      continue;
    }
    emittedIds.add(id);
    fallback.push({
      id: key,
      key,
      dimension: BOARD_GROUP_BY_MILESTONE,
      title: milestone.name || `Milestone #${id}`,
      parent: null,
      milestone,
      items: laneItems,
      isDefault: false,
      isUnassigned: false,
      itemCount: laneItems.length,
      parentIsVisible: false,
    });
  }
  fallback.sort((a, b) => {
    const byDate = compareLaneDates(a.milestone, b.milestone);
    if (byDate !== 0) return byDate;
    return String(a.title).localeCompare(String(b.title));
  });

  const unassignedItems = bucket.get(SWIMLANE_NONE_KEY) ?? [];
  const result = [];
  if (rescueItems.length > 0) {
    result.push(openFromCompletedLane(rescueItems, openFromCompletedTitle));
  }
  result.push(...lanes, ...fallback);
  if (hiddenCompletedItems.length > 0) {
    result.push(hiddenCompletedLane(hiddenCompletedItems, hiddenCompletedTitle));
  }
  result.push(unassignedLane(unassignedTitle, unassignedItems, BOARD_GROUP_BY_MILESTONE));
  return result;
}

function compareLaneDates(a, b) {
  const aDate = a?.target_date == null || a.target_date === '' ? null : String(a.target_date);
  const bDate = b?.target_date == null || b.target_date === '' ? null : String(b.target_date);
  if (aDate === bDate) return 0;
  if (aDate === null) return 1;
  if (bDate === null) return -1;
  return aDate.localeCompare(bDate);
}

/**
 * Build the board's swimlanes for the current grouping mode.
 *
 * @param {object} options
 * @param {{dimension: string, itemTypeId?: number}|null} options.groupBy
 * @param {Array} options.items all loaded cards (used for item-type parent eligibility)
 * @param {Array} options.filteredItems cards after client filters
 * @param {Array} options.milestones milestone reference for lane headers/order
 * @param {(item: object) => number|null} [options.statusIdForItem]
 * @param {Set<number>} [options.rightmostStatusIds]
 * @param {boolean} [options.excludeRightmostSwimlaneParents]
 * @param {boolean} [options.includeEmptyMilestones]
 * @param {number[]} [options.globalMilestoneIds] pinned global milestones that keep an empty lane
 * @param {boolean} [options.showCompletedMilestones] render completed milestone lanes
 * @param {boolean} [options.showOpenFromCompletedMilestones] collect open items on completed milestones into a rescue lane
 * @param {(item: object) => boolean} [options.isItemOpen] whether a card is still open
 * @param {string} [options.unassignedTitle]
 * @param {string} [options.hiddenCompletedTitle] title of the aggregate lane for hidden completed milestones
 * @param {string} [options.openFromCompletedTitle] title of the rescue lane for open items on completed milestones
 * @param {string} [options.allItemsTitle]
 */
export function buildBoardSwimlanes({
  groupBy,
  items = [],
  filteredItems = items,
  milestones = [],
  statusIdForItem = (item) => item.status_id ?? null,
  rightmostStatusIds = new Set(),
  excludeRightmostSwimlaneParents = false,
  includeEmptyMilestones = true,
  globalMilestoneIds = [],
  showCompletedMilestones = true,
  showOpenFromCompletedMilestones = false,
  isItemOpen = defaultIsItemOpen,
  unassignedTitle = 'Unassigned',
  hiddenCompletedTitle = 'Completed milestones',
  openFromCompletedTitle = 'Open from completed milestones',
  allItemsTitle = 'All items',
}) {
  if (!groupBy) {
    return [
      {
        id: 'all',
        key: null,
        dimension: null,
        title: allItemsTitle,
        parent: null,
        milestone: null,
        items: filteredItems,
        isDefault: true,
        isUnassigned: false,
        itemCount: filteredItems.length,
        parentIsVisible: false,
      },
    ];
  }

  if (groupBy.dimension === BOARD_GROUP_BY_MILESTONE) {
    return milestoneLanes({
      filteredItems,
      milestones,
      includeEmptyMilestones,
      globalMilestoneIds,
      showCompletedMilestones,
      showOpenFromCompletedMilestones,
      isItemOpen,
      unassignedTitle,
      hiddenCompletedTitle,
      openFromCompletedTitle,
    });
  }

  if (groupBy.dimension === BOARD_GROUP_BY_ITEM_TYPE && groupBy.itemTypeId != null) {
    return itemTypeLanes({
      items,
      filteredItems,
      itemTypeId: groupBy.itemTypeId,
      excludeRightmostSwimlaneParents,
      rightmostStatusIds,
      statusIdForItem,
      unassignedTitle,
    });
  }

  return buildBoardSwimlanes({
    groupBy: null,
    items,
    filteredItems,
    milestones,
    allItemsTitle,
  });
}
