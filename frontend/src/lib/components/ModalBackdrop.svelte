<script>
  import { tick } from 'svelte';
  import { fade } from 'svelte/transition';

  let {
    show = $bindable(false),
    opacity = 0.5,
    blur = 2,
    extraFilter = '',
    zIndex = 50,
    align = 'center',
    paddingTop = '',
    scrollable = false,
    closeOnClick = true,
    closeOnEscape = true,
    transition = true,
    ariaLabelledBy = undefined,
    /** CSS selector for the element that should receive focus first. */
    initialFocus = '[data-autofocus], [autofocus]',
    onclose = undefined,
    children = undefined,
  } = $props();

  let backdropRef = $state(null);
  let previouslyFocusedElement = null;

  const bgStyle = $derived(`rgba(0, 0, 0, ${opacity})`);
  const filterStyle = $derived(
    [blur > 0 ? `blur(${blur}px)` : '', extraFilter].filter(Boolean).join(' ') || 'none'
  );

  const layoutClasses = $derived(
    align === 'center'
      ? 'flex items-center justify-center p-4'
      : align === 'top'
        ? `flex items-start justify-center ${paddingTop}${scrollable ? ' overflow-y-auto' : ''}`
        : ''
  );

  const focusableSelector = [
    'a[href]',
    'area[href]',
    'button:not([disabled])',
    'input:not([disabled]):not([type="hidden"])',
    'select:not([disabled])',
    'textarea:not([disabled])',
    'iframe',
    'object',
    'embed',
    '[contenteditable="true"]',
    '[tabindex]:not([tabindex="-1"])'
  ].join(',');

  // Focus management: save focus before opening, put focus into the modal,
  // trap Tab/Shift+Tab while open, and restore focus on close. This makes
  // modal workflows efficient for keyboard and screen-reader users instead of
  // dropping them on the page behind the overlay.
  $effect(() => {
    if (show && !previouslyFocusedElement) {
      previouslyFocusedElement = document.activeElement;
      void focusInitialElement();
    }
    if (!show && previouslyFocusedElement) {
      previouslyFocusedElement?.focus?.();
      previouslyFocusedElement = null;
    }
  });

  function getFocusableElements() {
    if (!backdropRef) return [];
    return Array.from(backdropRef.querySelectorAll(focusableSelector)).filter((el) => {
      if (!(el instanceof HTMLElement)) return false;
      if (el.hasAttribute('disabled') || el.getAttribute('aria-hidden') === 'true') return false;
      const style = window.getComputedStyle(el);
      return style.display !== 'none' && style.visibility !== 'hidden' && el.offsetParent !== null;
    });
  }

  async function focusInitialElement() {
    await tick();
    if (!show || !backdropRef) return;

    const preferred = initialFocus ? backdropRef.querySelector(initialFocus) : null;
    if (preferred instanceof HTMLElement && !preferred.hasAttribute('disabled')) {
      preferred.focus();
      return;
    }

    const [first] = getFocusableElements();
    (first || backdropRef)?.focus();
  }

  function handleIntroEnd() {
    void focusInitialElement();
  }

  function handleClick(event) {
    if (closeOnClick && event.target === event.currentTarget) {
      close();
    }
  }

  function handleKeydown(event) {
    if (closeOnEscape && event.key === 'Escape') {
      close();
      return;
    }

    if (event.key !== 'Tab') return;

    const focusable = getFocusableElements();
    if (focusable.length === 0) {
      event.preventDefault();
      backdropRef?.focus();
      return;
    }

    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    const active = document.activeElement;

    if (event.shiftKey) {
      if (active === first || !backdropRef?.contains(active)) {
        event.preventDefault();
        last.focus();
      }
      return;
    }

    if (active === last || !backdropRef?.contains(active)) {
      event.preventDefault();
      first.focus();
    }
  }

  function close() {
    show = false;
    onclose?.();
  }
</script>

{#if show}
  <div
    bind:this={backdropRef}
    transition:fade={{ duration: transition ? 150 : 0 }}
    onintroend={handleIntroEnd}
    class="fixed inset-0 {layoutClasses} focus:outline-none"
    style="z-index: {zIndex}; background-color: {bgStyle}; backdrop-filter: {filterStyle};"
    onclick={handleClick}
    onkeydown={handleKeydown}
    role="dialog"
    aria-modal="true"
    aria-labelledby={ariaLabelledBy}
    tabindex="-1"
  >
    {@render children?.()}
  </div>
{/if}
