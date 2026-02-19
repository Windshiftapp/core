<script>
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
    onclose = undefined,
    children,
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

  // Focus management: save on open, restore on close
  $effect(() => {
    if (show && !previouslyFocusedElement) {
      previouslyFocusedElement = document.activeElement;
    }
    if (!show && previouslyFocusedElement) {
      previouslyFocusedElement?.focus();
      previouslyFocusedElement = null;
    }
  });

  function handleIntroEnd() {
    backdropRef?.focus();
  }

  function handleClick(event) {
    if (closeOnClick && event.target === event.currentTarget) {
      close();
    }
  }

  function handleKeydown(event) {
    if (closeOnEscape && event.key === 'Escape') {
      close();
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
