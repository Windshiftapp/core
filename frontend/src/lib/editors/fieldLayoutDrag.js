import {
  draggable,
  dropTargetForElements,
} from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
import {
  attachClosestEdge,
  extractClosestEdge,
} from '@atlaskit/pragmatic-drag-and-drop-hitbox/closest-edge';

/**
 * Wire the pragmatic-drag-and-drop behaviour for a field-layout editor:
 * available fields drag into selected fields, and selected fields reorder with
 * top/bottom edge detection. Shared by FieldLayoutEditor and the Screens page.
 *
 * @param {{
 *   availableSelector: string,
 *   availableDataKey: string,
 *   selectedSelector: string,
 *   dropZoneSelector: string,
 *   selectedType: string,
 *   onDragState: (fieldId: string, state: { closestEdge: string | null }) => void,
 *   clearDragState: () => void,
 *   setDraggedField: (value: boolean) => void,
 *   addFieldAtPosition: (field: any, index: number, edge: string | null) => void,
 *   reorderFieldWithEdge: (fromIndex: number, toIndex: number, edge: string | null) => void,
 *   addFieldToEnd: (field: any) => void,
 * }} config
 */
export function createFieldLayoutDrag(config) {
  const {
    availableSelector,
    availableDataKey,
    selectedSelector,
    dropZoneSelector,
    selectedType,
    onDragState,
    clearDragState,
    setDraggedField,
    addFieldAtPosition,
    reorderFieldWithEdge,
    addFieldToEnd,
  } = config;

  let setupCleanups = [];
  let setupTimeout;

  function cleanup() {
    if (setupTimeout) clearTimeout(setupTimeout);
    setupCleanups.forEach((fn) => fn());
    setupCleanups = [];
    clearDragState();
  }

  function setup() {
    cleanup();

    // Available fields are drag sources only.
    /** @type {NodeListOf<HTMLElement>} */ (document.querySelectorAll(availableSelector)).forEach(
      (element) => {
        const fieldData = JSON.parse(element.dataset[availableDataKey]);

        const cleanupFn = draggable({
          element,
          getInitialData: () => ({ field: fieldData, type: 'available-field' }),
          onDragStart: () => {
            element.style.opacity = '0.5';
          },
          onDrop: () => {
            element.style.opacity = '';
          },
        });

        setupCleanups.push(cleanupFn);
      }
    );

    // Selected fields are both drag sources and edge-aware drop targets.
    /** @type {NodeListOf<HTMLElement>} */ (document.querySelectorAll(selectedSelector)).forEach(
      (element) => {
        const fieldIndex = parseInt(element.dataset.fieldIndex, 10);
        const fieldId = element.dataset.fieldId;

        onDragState(fieldId, { closestEdge: null });

        const dragHandle = element.querySelector('.cursor-grab');
        const draggableCleanup = draggable({
          element,
          dragHandle: dragHandle || element,
          getInitialData: () => ({ fieldIndex, fieldId, type: selectedType }),
          onDragStart: () => {
            element.style.opacity = '0.5';
          },
          onDrop: () => {
            element.style.opacity = '';
            clearDragState();
          },
        });

        const dropTargetCleanup = dropTargetForElements({
          element,
          canDrop: ({ source }) => {
            const data = source.data;
            if (data.type === selectedType && data.fieldIndex === fieldIndex) return false;
            return data.type === 'available-field' || data.type === selectedType;
          },
          getData: ({ input, element: target }) =>
            attachClosestEdge({}, { input, element: target, allowedEdges: ['top', 'bottom'] }),
          onDragEnter: ({ self }) => {
            onDragState(fieldId, { closestEdge: extractClosestEdge(self.data) });
          },
          onDragLeave: () => {
            onDragState(fieldId, { closestEdge: null });
          },
          onDrop: ({ self, source }) => {
            const closestEdge = extractClosestEdge(self.data);
            const data = source.data;

            if (data.type === 'available-field') {
              addFieldAtPosition(data.field, fieldIndex, closestEdge);
            } else if (data.type === selectedType) {
              reorderFieldWithEdge(
                /** @type {number} */ (data.fieldIndex),
                fieldIndex,
                closestEdge
              );
            }

            onDragState(fieldId, { closestEdge: null });
          },
        });

        setupCleanups.push(() => {
          draggableCleanup();
          dropTargetCleanup();
        });
      }
    );

    // Trailing drop zone appends to the end.
    const dropZone = /** @type {HTMLElement | null} */ (document.querySelector(dropZoneSelector));
    if (dropZone) {
      const cleanupFn = dropTargetForElements({
        element: dropZone,
        canDrop: ({ source }) => source.data.type === 'available-field',
        onDragEnter: () => setDraggedField(true),
        onDragLeave: () => setDraggedField(false),
        onDrop: ({ source }) => {
          if (source.data.type === 'available-field') {
            addFieldToEnd(source.data.field);
          }
          setDraggedField(false);
        },
      });
      setupCleanups.push(cleanupFn);
    }
  }

  /** Debounced setup used after the editor mounts or its fields change. */
  function schedule(delay = 50) {
    if (setupTimeout) clearTimeout(setupTimeout);
    setupTimeout = setTimeout(() => setup(), delay);
  }

  return { cleanup, setup, schedule };
}
