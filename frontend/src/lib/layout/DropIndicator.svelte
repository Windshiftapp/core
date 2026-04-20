<script>
  let { edge, gap = 4 } = $props();

  const minOffset = 6;
  const thickness = 4;
  let offset = $derived(Math.max(gap / 2 + 2, minOffset));
  let isHorizontal = $derived(edge === 'left' || edge === 'right');
</script>

{#if edge}
  <div
    class="drop-indicator"
    class:drop-indicator--vertical={isHorizontal}
    aria-hidden="true"
    style:top={edge === 'top' ? `-${offset}px` : (isHorizontal ? '0' : null)}
    style:bottom={edge === 'bottom' ? `-${offset}px` : (isHorizontal ? '0' : null)}
    style:left={edge === 'left' ? `-${offset}px` : (isHorizontal ? null : '-6px')}
    style:right={edge === 'right' ? `-${offset}px` : (isHorizontal ? null : '-6px')}
    style:height={isHorizontal ? null : `${thickness}px`}
    style:width={isHorizontal ? `${thickness}px` : null}
  >
    <div class="drop-indicator__cap drop-indicator__cap--start"></div>
    <div class="drop-indicator__cap drop-indicator__cap--end"></div>
  </div>
{/if}

<style>
  .drop-indicator {
    position: absolute;
    background: linear-gradient(90deg, var(--ds-interactive-subtle, #60a5fa), var(--ds-interactive, #2874bb));
    border-radius: 9999px;
    box-shadow:
      0 0 0 1px var(--ds-surface-raised, #ffffff),
      0 4px 10px rgba(59, 130, 246, 0.25);
    pointer-events: none;
    z-index: 40;
    opacity: 0.98;
  }

  .drop-indicator--vertical {
    background: linear-gradient(180deg, var(--ds-interactive-subtle, #60a5fa), var(--ds-interactive, #2874bb));
  }

  .drop-indicator__cap {
    position: absolute;
    width: 8px;
    height: 8px;
    background: var(--ds-interactive, #2874bb);
    border-radius: 9999px;
    box-shadow: 0 0 0 1px var(--ds-surface-raised, #ffffff);
  }

  .drop-indicator:not(.drop-indicator--vertical) .drop-indicator__cap {
    top: -3px;
  }
  .drop-indicator:not(.drop-indicator--vertical) .drop-indicator__cap--start {
    left: -6px;
  }
  .drop-indicator:not(.drop-indicator--vertical) .drop-indicator__cap--end {
    right: -6px;
  }

  .drop-indicator--vertical .drop-indicator__cap {
    left: -3px;
  }
  .drop-indicator--vertical .drop-indicator__cap--start {
    top: -6px;
  }
  .drop-indicator--vertical .drop-indicator__cap--end {
    bottom: -6px;
  }
</style>
