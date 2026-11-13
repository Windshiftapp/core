import { monitorForElements } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
import { autoScrollForElements } from '@atlaskit/pragmatic-drag-and-drop-auto-scroll/element';
import { DRAG_OVERSCAN, ROW_OVERSCAN } from './rowProjection';
import type { VirtualRowsController } from './virtualRowsController';

const cancelledSources = new WeakSet<object>();
const committedSources = new WeakSet<object>();
// Keep the outcome through all target/monitor callbacks for one native drop.
// Reset on each new gesture: consumers may reuse their drag-data object.

export function claimVirtualDrop(source: object) {
  if (cancelledSources.has(source) || committedSources.has(source)) return false;
  committedSources.add(source);
  return true;
}

export function virtualDropAllowed(source: object) {
  return !cancelledSources.has(source);
}

export function createVirtualDragSession(
  controller: VirtualRowsController<any>,
  root: HTMLElement
) {
  let active: { data: object; release: () => void } | null = null;
  let disposed = false;
  let stopScroll: (() => void) | null = null;
  const end = (cancelled = false) => {
    if (!active) return;
    if (cancelled) cancelledSources.add(active.data);
    active.release();
    active = null;
    stopScroll?.();
    stopScroll = null;
    root.ownerDocument.body.classList.remove('is-dragging');
    controller.setOverscan(ROW_OVERSCAN);
  };
  const stopMonitor = monitorForElements({
    canMonitor: ({ source }) => {
      const node = source.element.closest<HTMLElement>('[data-virtual-key]');
      return node !== null && controller.ownsNode(node);
    },
    onDragStart: ({ source }) => {
      const key = source.element.closest<HTMLElement>('[data-virtual-key]')?.dataset.virtualKey;
      if (!key) return;
      cancelledSources.delete(source.data);
      committedSources.delete(source.data);
      active = { data: source.data, release: controller.acquirePin(key, 'drag') };
      controller.setOverscan(DRAG_OVERSCAN);
      root.ownerDocument.body.classList.add('is-dragging');
      stopScroll = autoScrollForElements({ element: root });
    },
    onDrop: () => end(),
  });
  const handleEscape = (event: KeyboardEvent) => {
    if (event.key === 'Escape') end(true);
  };
  root.ownerDocument.addEventListener('keydown', handleEscape, true);
  return {
    invalidate() {
      end(true);
    },
    destroy() {
      if (disposed) return;
      disposed = true;
      end(true);
      stopMonitor();
      root.ownerDocument.removeEventListener('keydown', handleEscape, true);
    },
  };
}
