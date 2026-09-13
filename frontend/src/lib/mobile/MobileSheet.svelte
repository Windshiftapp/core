<script>
  import { fade, fly } from 'svelte/transition';
  import { portal } from '../actions/portal.js';

  /**
   * Bottom sheet primitive for the phone surface. Replaces desktop
   * Modal/popover patterns on /m: anchored to the bottom edge, grabber,
   * drag-to-dismiss, and Android/iOS back gesture closes the sheet instead of
   * leaving the page (history sentinel).
   *
   * @type {{
   *   isOpen?: boolean,
   *   title?: string,
   *   onclose?: (() => void) | null,
   *   preventClose?: boolean,
   *   dataTestid?: string,
   *   children?: import('svelte').Snippet,
   * }}
   */
  let {
    isOpen = $bindable(false),
    title = '',
    onclose = null,
    preventClose = false,
    dataTestid = undefined,
    children,
  } = $props();

  let sheetEl = $state(null);
  // Drag-to-dismiss state: offset while dragging (px, >=0), whether a drag is
  // active. Applied to the inner card so it never fights the fly transition.
  let dragY = $state(0);
  let dragging = $state(false);
  let dragStartY = 0;
  let sheetHistoryId = null;
  let previouslyFocused = null;

  function close() {
    if (preventClose) return;
    consumeHistorySentinel();
    isOpen = false;
    onclose?.();
  }

  function handleKeydown(e) {
    if (e.key === 'Escape') {
      e.stopPropagation();
      close();
    }
  }

  // === Back-gesture support. While the sheet is open we push a sentinel
  // history entry; a popstate (Android back gesture, browser back) then closes
  // the sheet instead of navigating away. Explicit closes consume the sentinel
  // so the next back still leaves the page. The router's own popstate listener
  // re-derives the same route and is a no-op for that entry.
  $effect(() => {
    if (!isOpen) return;
    previouslyFocused = document.activeElement;
    sheetHistoryId = { mobileSheet: Date.now() + Math.random() };
    window.history.pushState(sheetHistoryId, '');
    window.addEventListener('popstate', onHistoryPop);
    const focusTimer = setTimeout(() => sheetEl?.focus(), 50);
    return () => {
      window.removeEventListener('popstate', onHistoryPop);
      clearTimeout(focusTimer);
      sheetHistoryId = null;
      dragY = 0;
      dragging = false;
      if (previouslyFocused?.focus) previouslyFocused.focus?.();
      previouslyFocused = null;
    };
  });

  function onHistoryPop() {
    // The sentinel entry was popped (back gesture): dismiss without touching
    // history again. If the sentinel was already consumed by an explicit
    // close (sheetHistoryId nulled) or the sheet is gone, this pop belongs to
    // someone else — stay out of the way.
    if (!isOpen || !sheetHistoryId) return;
    sheetHistoryId = null;
    isOpen = false;
    onclose?.();
  }

  function consumeHistorySentinel() {
    if (sheetHistoryId && window.history.state?.mobileSheet) {
      // Remove the sentinel from the stack without invoking onHistoryPop's
      // close path — we are already closing.
      sheetHistoryId = null;
      window.history.back();
    }
  }

  // === Drag-to-dismiss on the grabber/header zone. Body content stays
  // scrollable; only the handle area drags the sheet.
  function dragStart(e) {
    if (preventClose) return;
    dragging = true;
    dragStartY = e.clientY;
    e.currentTarget.setPointerCapture?.(e.pointerId);
  }

  function dragMove(e) {
    if (!dragging) return;
    dragY = Math.max(0, e.clientY - dragStartY);
  }

  function dragEnd() {
    if (!dragging) return;
    dragging = false;
    // Past ~120px (or a fast short flick) → dismiss; otherwise spring back.
    if (dragY > 120) {
      close();
    }
    dragY = 0;
  }
</script>

<svelte:window onkeydown={isOpen ? handleKeydown : undefined} />

{#if isOpen}
  <div
    use:portal
    class="sheet-layer"
    data-testid={dataTestid}
    transition:fade={{ duration: 150 }}
  >
    <!-- Scrim: tap to dismiss -->
    <button
      class="scrim"
      aria-label="Close"
      tabindex="-1"
      onclick={close}
      type="button"
    ></button>

    <div
      bind:this={sheetEl}
      class="sheet"
      class:dragging
      role="dialog"
      aria-modal="true"
      aria-label={title || 'Dialog'}
      tabindex="-1"
      style:transform={dragY > 0 ? `translateY(${dragY}px)` : ''}
      transition:fly={{ y: 320, duration: 260, opacity: 1 }}
    >
      <!-- Grabber zone: drag down to dismiss (pointer-only enhancement;
           dismissal also works via scrim tap, Escape, and back gesture). -->
      <div
        class="grabber-zone"
        aria-hidden="true"
        onpointerdown={dragStart}
        onpointermove={dragMove}
        onpointerup={dragEnd}
        onpointercancel={dragEnd}
      >
        <span class="grabber" aria-hidden="true"></span>
        {#if title}
          <h2 class="sheet-title">{title}</h2>
        {/if}
      </div>

      <div class="sheet-body">
        {@render children?.()}
      </div>
    </div>
  </div>
{/if}

<style>
  .sheet-layer {
    position: fixed;
    inset: 0;
    /* Above the sticky mobile header (z-30) and desktop dropdowns (z-60/70). */
    z-index: 80;
    display: flex;
    flex-direction: column;
    justify-content: flex-end;
  }

  .scrim {
    position: absolute;
    inset: 0;
    border: none;
    padding: 0;
    background-color: rgba(0, 0, 0, 0.4);
    cursor: default;
  }

  .sheet {
    position: relative;
    display: flex;
    flex-direction: column;
    max-height: 85dvh;
    border-radius: 16px 16px 0 0;
    background-color: var(--ds-surface-raised, var(--ds-surface));
    box-shadow: var(--shadow-float, 0 -8px 32px rgba(0, 0, 0, 0.25));
    padding-bottom: env(safe-area-inset-bottom, 0px);
  }
  .sheet.dragging {
    transition: none;
  }

  .grabber-zone {
    flex-shrink: 0;
    padding: 0.5rem 1rem 0.25rem;
    cursor: grab;
    touch-action: none;
  }
  .grabber {
    display: block;
    width: 36px;
    height: 4px;
    margin: 0 auto 0.25rem;
    border-radius: var(--radius-full, 9999px);
    background-color: var(--ds-border-bold, var(--ds-border));
  }
  .sheet-title {
    margin: 0.25rem 0 0.5rem;
    font-size: 1rem;
    font-weight: var(--font-semibold, 600);
    color: var(--ds-text);
    text-align: center;
  }

  .sheet-body {
    overflow-y: auto;
    -webkit-overflow-scrolling: touch;
    /* Keep scrolling inside the sheet instead of chaining to the page. */
    overscroll-behavior: contain;
    min-height: 0;
  }
</style>
