<script>
  import { XCircle, CircleCheck, AlertTriangle, Info } from 'lucide-svelte';
  import { toasts, removeToast } from '../../stores/toasts.svelte.js';
  import { t } from '../../stores/i18n.svelte.js';

  // Icon mapping for variants
  const variantIcons = {
    'default': null,
    'error': XCircle,
    'success': CircleCheck,
    'warning': AlertTriangle,
    'info': Info
  };

  // Variant border colors (left border accent)
  const variantBorders = {
    'default': 'var(--ds-border)',
    'error': 'var(--ds-border-danger)',
    'success': 'var(--ds-border-success)',
    'warning': 'var(--ds-border-warning)',
    'info': 'var(--ds-border-info)'
  };

  // Icon colors using design system tokens
  const variantIconColors = {
    'default': 'var(--ds-icon)',
    'error': 'var(--ds-icon-danger)',
    'success': 'var(--ds-icon-success)',
    'warning': 'var(--ds-icon-warning)',
    'info': 'var(--ds-icon-info)'
  };

  // Track active timeouts
  let timeouts = new Map();

  // Set up auto-hide for toasts
  $effect(() => {
    const currentToasts = toasts.value;

    // Set up timeouts for new toasts
    for (const toast of currentToasts) {
      if (toast.duration > 0 && !timeouts.has(toast.id)) {
        const timeoutId = setTimeout(() => {
          removeToast(toast.id);
          timeouts.delete(toast.id);
        }, toast.duration);
        timeouts.set(toast.id, timeoutId);
      }
    }

    // Clean up timeouts for removed toasts
    const currentIds = new Set(currentToasts.map(t => t.id));
    for (const [id, timeoutId] of timeouts) {
      if (!currentIds.has(id)) {
        clearTimeout(timeoutId);
        timeouts.delete(id);
      }
    }
  });

  function handleClose(id) {
    if (timeouts.has(id)) {
      clearTimeout(timeouts.get(id));
      timeouts.delete(id);
    }
    removeToast(id);
  }

  function handleClick(toast) {
    if (toast.clickable && toast.onClick) {
      toast.onClick();
      handleClose(toast.id);
    }
  }

  function getStackStyles(index) {
    // Newest toast (index 0) is on top with full size
    // Older toasts get progressively smaller offset and scale
    const maxVisible = 3;
    if (index >= maxVisible) {
      return { display: 'none' };
    }

    const offset = index * 14; // 14px offset per toast (more visible stacking)
    const scale = 1 - (index * 0.03); // Scale down 3% per toast
    const opacity = 1 - (index * 0.12); // Reduce opacity 12% per toast
    const zIndex = 100 - index;

    return {
      transform: `translateY(${offset}px) scale(${scale})`,
      opacity,
      zIndex
    };
  }
</script>

{#if toasts.value.length > 0}
  <div class="fixed top-12 left-1/2 -translate-x-1/2 z-50 flex flex-col items-center">
    {#each toasts.value as toast, index (toast.id)}
      {@const Icon = variantIcons[toast.variant]}
      {@const borderColor = variantBorders[toast.variant] || variantBorders['default']}
      {@const iconColor = variantIconColors[toast.variant] || variantIconColors['default']}
      {@const stackStyles = getStackStyles(index)}

      {#if index < 3}
        <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
        <div
          class="absolute w-[360px] rounded shadow-xl flex items-start gap-3 transition-all duration-300 ease-out {toast.clickable ? 'cursor-pointer hover:shadow-2xl' : ''}"
          style="
            background: var(--ds-surface-raised);
            border: 1px solid var(--ds-border);
            border-left: 4px solid {borderColor};
            transform: {stackStyles.transform};
            opacity: {stackStyles.opacity};
            z-index: {stackStyles.zIndex};
            animation: {index === 0 ? 'slideInFromTop 0.4s cubic-bezier(0.21, 1.02, 0.73, 1)' : 'none'};
          "
          onclick={() => handleClick(toast)}
          role={toast.clickable ? 'button' : undefined}
          tabindex={toast.clickable ? '0' : undefined}
          onkeydown={(e) => toast.clickable && e.key === 'Enter' && handleClick(toast)}
        >
          {#if Icon}
            <div class="flex-shrink-0 pl-3 pt-3">
              <Icon class="w-5 h-5" style="color: {iconColor};" />
            </div>
          {/if}

          <div class="flex-grow py-3 {Icon ? '' : 'pl-3'}">
            {#if toast.title}
              <div class="font-semibold text-sm mb-1" style="color: var(--ds-text);">
                {toast.title}
              </div>
            {/if}
            {#if toast.message}
              <div class="text-sm" style="color: var(--ds-text-subtle);">
                {toast.message}
              </div>
            {/if}
          </div>

          {#if toast.showCloseButton}
            <button
              class="flex-shrink-0 transition-colors duration-150 p-2 mt-1 mr-1"
              style="color: var(--ds-text-subtle);"
              onclick={() => handleClose(toast.id)}
              aria-label={t('notifications.closeToast')}
            >
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
              </svg>
            </button>
          {/if}
        </div>
      {/if}
    {/each}
  </div>
{/if}

<style>
  @keyframes slideInFromTop {
    from {
      opacity: 0;
      transform: translateY(-100%) scale(1);
    }
    to {
      opacity: 1;
      transform: translateY(0) scale(1);
    }
  }
</style>
