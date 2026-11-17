<script>
  let {
    width,
    minWidth,
    maxWidth,
    defaultWidth,
    edge = 'right',
    collapsed = false,
    collapsedWidth = 48,
    collapseThreshold = null,
    label,
    title,
    testId = undefined,
    onresize = null,
    onresizeend = null,
    oncollapsechange = null,
    ontoggle = null,
  } = $props();

  let isResizing = $state(false);
  let resizeStartX = 0;
  let resizeStartWidth = 0;
  let liveWidth = $state(0);
  let liveCollapsed = $state(false);
  // A pointerup that never moved is a click, not a drag, and may toggle the
  // sidebar instead of resizing it.
  let dragMoved = false;
  /** @type {number | null} */
  let resizePointerId = null;

  const canCollapse = $derived(
    Number.isFinite(collapseThreshold) && collapsedWidth < minWidth
  );
  const resizeDirection = $derived(edge === 'left' ? -1 : 1);

  $effect(() => {
    if (isResizing) return;
    liveWidth = clampWidth(width);
    liveCollapsed = canCollapse && collapsed;
  });

  $effect(() => {
    if (typeof document === 'undefined') return;
    document.body.classList.toggle('sidebar-resize-active', isResizing);
    return () => document.body.classList.remove('sidebar-resize-active');
  });

  /** @param {number} value */
  function clampWidth(value) {
    return Math.min(maxWidth, Math.max(minWidth, value));
  }

  /** @param {number} nextWidth @param {boolean} nextCollapsed */
  function publish(nextWidth, nextCollapsed) {
    liveWidth = clampWidth(nextWidth);
    liveCollapsed = canCollapse && nextCollapsed;
    onresize?.(liveWidth);
    oncollapsechange?.(liveCollapsed);
  }

  /** @param {PointerEvent} event */
  function startResize(event) {
    if (event.button !== 0 || isResizing) return;
    event.preventDefault();
    resizeStartX = event.clientX;
    resizeStartWidth = collapsed ? collapsedWidth : clampWidth(width);
    liveWidth = clampWidth(width);
    liveCollapsed = canCollapse && collapsed;
    resizePointerId = event.pointerId;
    dragMoved = false;
    isResizing = true;
    const handle = /** @type {HTMLElement} */ (event.currentTarget);
    handle.setPointerCapture?.(event.pointerId);
  }

  /** @param {PointerEvent} event */
  function resize(event) {
    if (!isResizing || event.pointerId !== resizePointerId) return;
    if (Math.abs(event.clientX - resizeStartX) > 3) dragMoved = true;
    const rawWidth = resizeStartWidth + (event.clientX - resizeStartX) * resizeDirection;
    const shouldCollapse = canCollapse && rawWidth < collapseThreshold;
    publish(shouldCollapse ? liveWidth : rawWidth, shouldCollapse);
  }

  /** @param {PointerEvent | FocusEvent | undefined} event */
  function finishResize(event = undefined) {
    if (!isResizing) return;
    if (
      event
      && 'pointerId' in event
      && resizePointerId !== null
      && event.pointerId !== resizePointerId
    ) return;
    const toggled = !dragMoved && event?.type === 'pointerup';
    isResizing = false;
    resizePointerId = null;
    onresizeend?.(liveWidth, liveCollapsed);
    if (toggled) ontoggle?.();
  }

  function reset() {
    publish(defaultWidth, false);
    onresizeend?.(liveWidth, liveCollapsed);
  }
</script>

<svelte:window onblur={finishResize} />

<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div
  class="sidebar-resize-handle"
  class:sidebar-resize-handle--active={isResizing}
  class:sidebar-resize-handle--left={edge === 'left'}
  role="separator"
  aria-label={label}
  aria-orientation="vertical"
  {title}
  data-testid={testId}
  onpointerdown={startResize}
  onpointermove={resize}
  onpointerup={finishResize}
  onpointercancel={finishResize}
  onlostpointercapture={finishResize}
  ondblclick={reset}
></div>

<style>
  .sidebar-resize-handle {
    position: absolute;
    z-index: 10;
    top: 0;
    right: -4px;
    width: 8px;
    height: 100%;
    cursor: col-resize;
    touch-action: none;
  }

  .sidebar-resize-handle--left {
    right: auto;
    left: -4px;
  }

  .sidebar-resize-handle::after {
    position: absolute;
    top: 0;
    bottom: 0;
    left: 3px;
    width: 2px;
    content: '';
    background: transparent;
    transition: background-color 120ms ease;
  }

  .sidebar-resize-handle:hover::after,
  .sidebar-resize-handle--active::after {
    background: var(--ds-border-focused);
  }

  :global(body.sidebar-resize-active),
  :global(body.sidebar-resize-active *) {
    cursor: col-resize !important;
    user-select: none !important;
  }
</style>
