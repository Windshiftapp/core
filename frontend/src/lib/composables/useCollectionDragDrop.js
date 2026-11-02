import { items } from '../api/items.js';

/**
 * Shared edge-drop orchestration for the collection backlog and board.
 *
 * Both surfaces reorder by computing the neighbouring item ids around a
 * top/bottom edge drop and sending them to the frac_index endpoint. They differ
 * in how they prepare a drop — the backlog moves an item between iterations,
 * the board transitions status and resolves lanes — so those steps are injected.
 * The composable owns the shared part: the pending-drop guard, the neighbour
 * computation, the reorder call, and the reload on a rank error.
 *
 * @param {{
 *   reload: () => void,
 *   beforeReorder: (ctx: {
 *     draggedItem: object,
 *     targetItem: object,
 *     closestEdge: 'top' | 'bottom',
 *     target: any,
 *   }) => Promise<{ item: object, checkSamePosition?: boolean } | null> | { item: object, checkSamePosition?: boolean } | null,
 *   getSiblingItems: (ctx: { item: object, targetItem: object, target: any }) => object[],
 * }} config
 */
export function useCollectionDragDrop({ reload, beforeReorder, getSiblingItems }) {
  // A drop fires repeatedly while the pointer moves; guard by the drop identity
  // so one gesture issues one reorder.
  const pendingDrops = new Set();

  async function handleEdgeBasedDrop(draggedItem, targetItem, closestEdge, target) {
    const dropId = `${draggedItem.id}-edge-${targetItem.id}-${closestEdge}`;
    if (pendingDrops.has(dropId)) return;
    pendingDrops.add(dropId);

    try {
      const prepared = await beforeReorder({ draggedItem, targetItem, closestEdge, target });
      if (!prepared) return;

      const siblings = getSiblingItems({ item: prepared.item, targetItem, target });
      const neighbors = computeNeighborIds(
        siblings,
        prepared.item,
        targetItem,
        closestEdge,
        prepared.checkSamePosition !== false
      );
      if (neighbors.samePosition) return;

      await items.updateFracIndex(prepared.item.id, {
        previous_item_id: neighbors.prevItemId,
        next_item_id: neighbors.nextItemId,
      });

      reload();
    } catch (error) {
      console.error('Failed to handle edge-based drop:', error);
      console.error('Error details:', error.message);

      // A rank-ordering error means the client's neighbours are stale; reload.
      if (error.status === 500) {
        reload();
      }
    } finally {
      setTimeout(() => pendingDrops.delete(dropId), 500);
    }
  }

  return { pendingDrops, handleEdgeBasedDrop };
}

/**
 * Resolve the item ids a top/bottom edge drop should sit between. `siblingItems`
 * is the ordered list for the drop's scope, including the dragged item when it
 * is already in that scope. `checkSamePosition` is false for cross-scope drops,
 * where the dragged item is absent and a "same position" match is meaningless.
 */
export function computeNeighborIds(
  siblingItems,
  draggedItem,
  targetItem,
  closestEdge,
  checkSamePosition
) {
  const targetIndex = siblingItems.findIndex((item) => item.id === targetItem.id);
  const draggedIndex = siblingItems.findIndex((item) => item.id === draggedItem.id);

  if (
    checkSamePosition &&
    ((closestEdge === 'top' && draggedIndex === targetIndex - 1) ||
      (closestEdge === 'bottom' && draggedIndex === targetIndex + 1))
  ) {
    return { samePosition: true, prevItemId: null, nextItemId: null };
  }

  const otherItems = siblingItems.filter((item) => item.id !== draggedItem.id);
  const adjustedTargetIndex = otherItems.findIndex((item) => item.id === targetItem.id);

  let prevItemId = null;
  let nextItemId = null;
  if (closestEdge === 'top') {
    if (adjustedTargetIndex > 0) prevItemId = otherItems[adjustedTargetIndex - 1]?.id ?? null;
    nextItemId = targetItem.id;
  } else if (closestEdge === 'bottom') {
    prevItemId = targetItem.id;
    if (adjustedTargetIndex < otherItems.length - 1) {
      nextItemId = otherItems[adjustedTargetIndex + 1]?.id ?? null;
    }
  }

  return { samePosition: false, prevItemId, nextItemId };
}
