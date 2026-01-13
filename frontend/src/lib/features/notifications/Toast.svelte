<script>
  // Props - all readonly (controlled component pattern)
  let {
    show = false,
    duration = 5000, // Auto-hide duration in milliseconds (0 = no auto-hide)
    position = 'bottom-right', // 'bottom-right', 'bottom-left', 'top-right', 'top-left', 'bottom-center', 'top-center'
    clickable = false, // Whether the entire toast is clickable
    showCloseButton = true, // Whether to show the close button
    width = 'auto', // CSS width value or 'auto'
    variant = 'default', // 'default', 'error', 'success', 'warning'
    onClose = () => {},
    onHide = () => {},
    onClick = () => {},
    children,
    icon
  } = $props();

  // Internal state (not reactive - just for tracking timeout)
  let timeoutId;

  // Position classes mapping (with more spacing from edges)
  const positionClasses = {
    'bottom-right': 'bottom-8 right-8',
    'bottom-left': 'bottom-8 left-8',
    'top-right': 'top-8 right-8',
    'top-left': 'top-8 left-8',
    'bottom-center': 'bottom-8 left-1/2 transform -translate-x-1/2',
    'top-center': 'top-8 left-1/2 transform -translate-x-1/2'
  };

  // Variant border colors (left border accent)
  const variantBorders = {
    'default': 'var(--ds-border)',
    'error': 'var(--ds-border-danger)',
    'success': 'var(--ds-border-success)',
    'warning': 'var(--ds-border-warning)',
    'info': 'var(--ds-border-info)'
  };

  // Position-aware animations (slide from nearest edge)
  const positionAnimations = {
    'bottom-right': 'slideInFromBottom',
    'bottom-left': 'slideInFromBottom',
    'top-right': 'slideInFromTop',
    'top-left': 'slideInFromTop',
    'bottom-center': 'slideInFromBottom',
    'top-center': 'slideInFromTop'
  };

  // Derived values
  let positionClass = $derived(positionClasses[position] || positionClasses['bottom-right']);
  let variantBorder = $derived(variantBorders[variant] || variantBorders['default']);
  let positionAnimation = $derived(positionAnimations[position] || 'slideInFromBottom');

  // Auto-hide effect
  $effect(() => {
    console.log('[Toast] Effect running, show:', show, 'duration:', duration);
    if (show && duration > 0) {
      console.log('[Toast] Setting up auto-hide timeout');
      clearTimeout(timeoutId);
      timeoutId = setTimeout(() => {
        console.log('[Toast] Auto-hide timeout fired, calling onHide');
        onHide();
      }, duration);
    }

    // Cleanup
    return () => {
      if (timeoutId) {
        console.log('[Toast] Cleanup: clearing timeout');
        clearTimeout(timeoutId);
      }
    };
  });

  function handleClick() {
    if (clickable) {
      onClick();
    }
  }

  function handleClose(event) {
    console.log('[Toast] Close button clicked');
    event?.stopPropagation();
    clearTimeout(timeoutId);
    onClose();
  }

  function handleKeydown(event) {
    if (clickable && event.key === 'Enter') {
      onClick();
    }
  }
</script>

{#if show}
  <div
    class="fixed z-50 {positionClass}"
    style="animation: {positionAnimation} 0.4s cubic-bezier(0.21, 1.02, 0.73, 1);"
  >
    <div
      class="rounded shadow-xl flex items-start gap-3 transition-shadow duration-200 {clickable ? 'cursor-pointer hover:shadow-2xl' : ''}"
      style="background: var(--ds-surface-raised); border: 1px solid var(--ds-border); border-left: 4px solid {variantBorder};{width !== 'auto' ? ` width: ${width};` : ''}"
      class:px-4={!children}
      class:py-3={!children}
      onclick={handleClick}
      role={clickable ? 'button' : undefined}
      tabindex={clickable ? '0' : undefined}
      onkeydown={handleKeydown}
    >
      {#if icon}
        <div class="flex-shrink-0 pl-2 pt-3">
          {@render icon()}
        </div>
      {/if}

      {#if children}
        <div class="flex-grow">
          {@render children()}
        </div>
      {/if}

      {#if showCloseButton}
        <button
          class="flex-shrink-0 transition-colors duration-150 p-2 mt-1"
          style="color: var(--ds-text-subtle);"
          onclick={handleClose}
          aria-label="Close toast"
        >
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
          </svg>
        </button>
      {/if}
    </div>
  </div>
{/if}

<style>
  @keyframes slideInFromBottom {
    from {
      opacity: 0;
      transform: translateY(100%);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }

  @keyframes slideInFromTop {
    from {
      opacity: 0;
      transform: translateY(-100%);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }
</style>