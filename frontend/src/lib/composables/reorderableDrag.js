import {
  draggable,
  dropTargetForElements,
} from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
import {
  attachClosestEdge,
  extractClosestEdge,
} from '@atlaskit/pragmatic-drag-and-drop-hitbox/closest-edge';

/**
 * Register one reorderable element: it can be dragged and dropped onto, shows
 * an edge highlight, and reports the drop through a single callback. Owns the
 * pragmatic-drag-and-drop wiring, the drag opacity, and the cleanup so callers
 * only supply the data shape and the reorder behaviour.
 *
 * @param {Object} options
 * @param {HTMLElement} options.element
 * @param {HTMLElement | null} [options.dragHandle]
 * @param {() => object} options.getDragData
 * @param {(args: { source: any }) => boolean} options.canDrop
 * @param {Array<'top'|'bottom'|'left'|'right'>} [options.allowedEdges]
 * @param {() => object} [options.getDropData] - payload attached to the drop edge
 * @param {() => void} [options.onDragStart]
 * @param {() => void} [options.onDragEnd]
 * @param {(closestEdge: string | null, source: any) => void} [options.onEnter]
 * @param {() => void} [options.onLeave]
 * @param {(args: { source: any, closestEdge: string | null }) => void} [options.onDrop]
 * @param {() => void} [options.onCleanup]
 * @returns {() => void} cleanup
 */
export function registerReorderable({
  element,
  dragHandle = null,
  getDragData,
  canDrop,
  allowedEdges = ['top', 'bottom'],
  getDropData,
  onDragStart,
  onDragEnd,
  onEnter,
  onLeave,
  onDrop,
  onCleanup,
}) {
  const draggableCleanup = draggable({
    element,
    dragHandle: dragHandle || element,
    getInitialData: getDragData,
    onDragStart: () => {
      element.style.opacity = '0.5';
      onDragStart?.();
    },
    onDrop: () => {
      element.style.opacity = '';
      onDragEnd?.();
    },
  });

  const dropTargetCleanup = dropTargetForElements({
    element,
    canDrop,
    getData: ({ input, element: target }) =>
      attachClosestEdge(getDropData?.() ?? {}, { input, element: target, allowedEdges }),
    onDragEnter: ({ self, source }) => onEnter?.(extractClosestEdge(self.data), source),
    onDragLeave: () => onLeave?.(),
    onDrop: ({ self, source }) => onDrop?.({ source, closestEdge: extractClosestEdge(self.data) }),
  });

  return () => {
    element.style.opacity = '';
    onCleanup?.();
    draggableCleanup();
    dropTargetCleanup();
  };
}
