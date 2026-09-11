<script>
  let {
    width,
    minWidth,
    maxWidth,
    defaultWidth,
    keyboardStep = 16,
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
  } = $props();

  let isResizing = $state(false);
  let resizeStartX = 0;
  let resizeStartWidth = 0;
  let liveWidth = $state(0);
  let liveCollapsed = $state(false);
  /** @type {number | null} */
  let resizePointerId = null;

  const canCollapse = $derived(
    Number.isFinite(collapseThreshold) && collapsedWidth < minWidth
  );
  const resizeDirection = $derived(edge === 'left' ? -1 : 1);
  const displayedWidth = $derived(collapsed ? collapsedWidth : clampWidth(width));
  const ariaMinWidth = $derived(canCollapse ? collapsedWidth : minWidth);

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
    isResizing = true;
    const handle = /** @type {HTMLElement} */ (event.currentTarget);
    handle.setPointerCapture?.(event.pointerId);
  }

  /** @param {PointerEvent} event */
  function resize(event) {
    if (!isResizing || event.pointerId !== resizePointerId) return;
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
    isResizing = false;
    resizePointerId = null;
    onresizeend?.(liveWidth, liveCollapsed);
  }

  /** @param {number} nextWidth @param {boolean} nextCollapsed */
  function publishAndCommit(nextWidth, nextCollapsed) {
    publish(nextWidth, nextCollapsed);
    onresizeend?.(liveWidth, liveCollapsed);
  }

  /** @param {KeyboardEvent} event */
  function resizeWithKeyboard(event) {
    let nextWidth = liveWidth;
    let nextCollapsed = liveCollapsed;

    if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') {
      const physicalDelta = event.key === 'ArrowLeft' ? -keyboardStep : keyboardStep;
      const widthDelta = physicalDelta * resizeDirection;
      if (liveCollapsed) {
        if (widthDelta < 0) return;
        nextWidth = minWidth;
        nextCollapsed = false;
      } else if (canCollapse && liveWidth <= minWidth && widthDelta < 0) {
        nextCollapsed = true;
      } else {
        nextWidth += widthDelta;
      }
    } else if (event.key === 'Home') {
      nextWidth = minWidth;
      nextCollapsed = canCollapse;
    } else if (event.key === 'End') {
      nextWidth = maxWidth;
      nextCollapsed = false;
    } else {
      return;
    }

    event.preventDefault();
    publishAndCommit(nextWidth, nextCollapsed);
  }

  function reset() {
    publishAndCommit(defaultWidth, false);
  }
</script>

<svelte:window onblur={finishResize} />

<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div
  class="sidebar-resize-handle"
  class:sidebar-resize-handle--active={isResizing}
  class:sidebar-resize-handle--left={edge === 'left'}
  role="separator"
  aria-label={label}
  aria-orientation="vertical"
  aria-valuemin={ariaMinWidth}
  aria-valuemax={maxWidth}
  aria-valuenow={displayedWidth}
  tabindex="0"
  {title}
  data-testid={testId}
  onpointerdown={startResize}
  onpointermove={resize}
  onpointerup={finishResize}
  onpointercancel={finishResize}
  onlostpointercapture={finishResize}
  onkeydown={resizeWithKeyboard}
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
  .sidebar-resize-handle:focus-visible::after,
  .sidebar-resize-handle--active::after {
    background: var(--ds-border-focused);
  }

  .sidebar-resize-handle:focus-visible {
    outline: 2px solid var(--ds-border-focused);
    outline-offset: -2px;
  }

  :global(body.sidebar-resize-active),
  :global(body.sidebar-resize-active *) {
    cursor: col-resize !important;
    user-select: none !important;
  }
</style>
