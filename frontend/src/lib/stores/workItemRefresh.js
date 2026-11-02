// One debounced reconciliation for the global `refresh-work-items` broadcast.
//
// The event is a "work items changed" signal. Surfaces used to add their own
// window listener, so one mutation started several independent reads (homepage
// snapshot, item SLA, child items, collection delta). This module owns the
// single surface listener, merges a burst into one notification, and each
// subscriber reconciles its own surface at most once.

// Synchronous bursts (modal event plus an SSE frame plus a cross-tab frame)
// coalesce into one flush. Zero delay keeps the reconciliation on the next
// task without adding perceptible latency to a single mutation.
const RECONCILE_DELAY_MS = 0;

/**
 * @typedef {Object} WorkItemRefresh
 * @property {any} [item] Latest item in the burst.
 * @property {number} [itemId] Latest item id in the burst.
 * @property {number} [workspaceId] Latest workspace id in the burst.
 * @property {number|null} [parentId] Latest parent id in the burst.
 * @property {number[]} itemIds Every item id seen in the burst.
 * @property {number[]} parentIds Every parent id seen in the burst.
 */

/** @type {Set<(detail: WorkItemRefresh) => void>} */
const subscribers = new Set();
/** @type {ReturnType<typeof setTimeout>|null} */
let timer = null;
/** @type {WorkItemRefresh|null} */
let pending = null;

function emptyDetail() {
  return { itemIds: [], parentIds: [] };
}

function mergeDetail(current, event) {
  const detail = current ?? emptyDetail();
  const next = event?.detail ?? {};
  if (next.item) detail.item = next.item;
  if (next.itemId != null) detail.itemId = next.itemId;
  if (next.workspaceId != null) detail.workspaceId = next.workspaceId;
  if (next.parentId != null) detail.parentId = next.parentId;

  const itemId = next.itemId ?? next.item?.id;
  if (itemId != null && !detail.itemIds.includes(itemId)) detail.itemIds.push(itemId);
  if (next.parentId != null && !detail.parentIds.includes(next.parentId)) {
    detail.parentIds.push(next.parentId);
  }
  return detail;
}

function flush() {
  timer = null;
  const detail = pending ?? emptyDetail();
  pending = null;
  for (const subscriber of subscribers) {
    try {
      subscriber(detail);
    } catch (error) {
      console.error('refresh-work-items subscriber failed:', error);
    }
  }
}

function handleWindowEvent(event) {
  pending = mergeDetail(pending, event);
  if (timer != null) return;
  timer = setTimeout(flush, RECONCILE_DELAY_MS);
}

/**
 * Subscribe to one debounced reconciliation per `refresh-work-items` burst.
 *
 * @param {(detail: WorkItemRefresh) => void} subscriber
 * @returns {() => void} unsubscribe
 */
export function onWorkItemRefresh(subscriber) {
  if (typeof window === 'undefined') return () => {};
  subscribers.add(subscriber);
  if (subscribers.size === 1) {
    window.addEventListener('refresh-work-items', handleWindowEvent);
  }
  return () => {
    subscribers.delete(subscriber);
    if (subscribers.size === 0) {
      window.removeEventListener('refresh-work-items', handleWindowEvent);
      if (timer != null) {
        clearTimeout(timer);
        timer = null;
      }
      pending = null;
    }
  };
}

/**
 * Whether a burst names the given workspace, or carries no workspace to
 * filter on. Lets a workspace-scoped surface skip a reconciliation that
 * cannot affect it.
 *
 * @param {WorkItemRefresh} detail
 * @param {number|null|undefined} workspaceId
 * @returns {boolean}
 */
export function workItemRefreshTouchesWorkspace(detail, workspaceId) {
  const eventWorkspace = detail?.item?.workspace_id ?? detail?.workspaceId ?? null;
  if (eventWorkspace == null || workspaceId == null) return true;
  return Number(eventWorkspace) === Number(workspaceId);
}

/** Test-only: drop subscribers and any pending reconciliation. */
export function resetWorkItemRefresh() {
  subscribers.clear();
  if (typeof window !== 'undefined') {
    window.removeEventListener('refresh-work-items', handleWindowEvent);
  }
  if (timer != null) {
    clearTimeout(timer);
    timer = null;
  }
  pending = null;
}
