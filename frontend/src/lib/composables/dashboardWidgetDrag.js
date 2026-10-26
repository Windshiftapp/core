import {
  draggable,
  dropTargetForElements,
} from '@atlaskit/pragmatic-drag-and-drop/element/adapter';

/**
 * Wire the pragmatic-drag-and-drop behaviour for the dashboard customize
 * mode: widget cards are drag sources and section drop zones accept them.
 * Shared by the personal Homepage and the workspace Welcome dashboard, which
 * use different DOM hooks and drop handlers.
 *
 * State stays in the caller; it is updated through the callbacks below.
 *
 * @param {{
 *   widgetCardSelector: string,
 *   dropZoneSelector: string,
 *   dragType: string,
 *   setDraggedWidget: (value: { type: string } | null) => void,
 *   setDropZoneState: (sectionId: string, state: { isOver: boolean }) => void,
 *   resetDropZones: () => void,
 *   onDropWidget: (sectionId: string, widgetType: string) => void,
 * }} config
 */
export function createDashboardWidgetDrag(config) {
  const {
    widgetCardSelector,
    dropZoneSelector,
    dragType,
    setDraggedWidget,
    setDropZoneState,
    resetDropZones,
    onDropWidget,
  } = config;

  let cleanups = [];
  let setupTimeout;

  function cleanup() {
    if (setupTimeout) clearTimeout(setupTimeout);
    cleanups.forEach((fn) => fn());
    cleanups = [];
  }

  function setup() {
    cleanup();

    /** @type {NodeListOf<HTMLElement>} */ (document.querySelectorAll(widgetCardSelector)).forEach(
      (cardElement) => {
        const cleanupFn = draggable({
          element: cardElement,
          getInitialData: () => ({
            type: dragType,
            widgetType: cardElement.dataset.widgetType,
          }),
          onDragStart: () => {
            const currentType = cardElement.dataset.widgetType;
            setDraggedWidget(currentType ? { type: currentType } : null);
            cardElement.style.opacity = '0.5';
          },
          onDrop: () => {
            setDraggedWidget(null);
            cardElement.style.opacity = '';
            resetDropZones();
          },
        });
        cleanups.push(cleanupFn);
      }
    );

    /** @type {NodeListOf<HTMLElement>} */ (document.querySelectorAll(dropZoneSelector)).forEach(
      (element) => {
        const sectionId = element.dataset.sectionId;
        setDropZoneState(sectionId, { isOver: false });

        const cleanupFn = dropTargetForElements({
          element,
          canDrop: ({ source }) => source.data.type === dragType,
          onDragEnter: () => setDropZoneState(sectionId, { isOver: true }),
          onDragLeave: () => setDropZoneState(sectionId, { isOver: false }),
          onDrop: ({ source }) => {
            setDropZoneState(sectionId, { isOver: false });
            const data = source.data;
            if (data.type === dragType) {
              onDropWidget(sectionId, /** @type {string} */ (data.widgetType));
            }
          },
        });
        cleanups.push(cleanupFn);
      }
    );
  }

  /** Debounced setup used after entering customize mode. */
  function schedule(delay = 350) {
    if (setupTimeout) clearTimeout(setupTimeout);
    setupTimeout = setTimeout(() => setup(), delay);
  }

  return { setup, cleanup, schedule };
}
