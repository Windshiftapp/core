<script>
  import { t } from '../stores/i18n.svelte.js';

  /**
   * Clamps tall, free-form content (rich text, long Markdown) to a fixed height
   * and offers an Expand/Collapse toggle. The toggle only appears when the
   * content actually overflows, so short fields keep their natural height.
   *
   * `enabled` lets a caller turn the clamp off (e.g. while the block is being
   * edited) without unmounting the children.
   */
  let {
    /** Collapsed height in px. */
    maxHeight = 220,
    enabled = true,
    testId = undefined,
    children,
  } = $props();

  let inner = $state(null);
  let expanded = $state(false);
  let overflows = $state(false);

  // Measure the inner element, which is never height-constrained, so the
  // overflow check stays stable while the outer block is expanded.
  function measure() {
    if (!inner) return;
    overflows = enabled && inner.scrollHeight > maxHeight + 1;
  }

  $effect(() => {
    if (!inner) return;
    measure();
    const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(measure);
    observer?.observe(inner);
    window.addEventListener('resize', measure);
    return () => {
      observer?.disconnect();
      window.removeEventListener('resize', measure);
    };
  });
</script>

<div class="relative min-w-0">
  <div
    class="overflow-hidden"
    class:clamped={enabled && overflows && !expanded}
    style:max-height={enabled && !expanded ? `${maxHeight}px` : 'none'}
  >
    <div bind:this={inner}>{@render children()}</div>
  </div>
</div>
{#if enabled && overflows}
  <button
    type="button"
    data-testid={testId}
    aria-expanded={expanded}
    onclick={() => (expanded = !expanded)}
    class="mt-1 rounded-sm text-xs font-medium text-ds-text-link hover:underline focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ds-border-focused"
  >
    {expanded ? t('common.collapse') : t('common.expand')}
  </button>
{/if}

<style>
  /* Mask rather than a background gradient so the fade works on any surface. */
  .clamped {
    -webkit-mask-image: linear-gradient(to bottom, #000 calc(100% - 2rem), transparent);
    mask-image: linear-gradient(to bottom, #000 calc(100% - 2rem), transparent);
  }
</style>
