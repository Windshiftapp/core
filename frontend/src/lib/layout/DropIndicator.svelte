<script>
  let { edge, gap = 4 } = $props();

  const minOffset = 6;
  const thickness = 4;
  let offset = $derived(Math.max(gap / 2 + 2, minOffset));
</script>

{#if edge}
  <div
    class="drop-indicator"
    aria-hidden="true"
    style:top={edge === 'top' ? `-${offset}px` : null}
    style:bottom={edge === 'bottom' ? `-${offset}px` : null}
    style:height={`${thickness}px`}
  >
    <div class="drop-indicator__cap drop-indicator__cap--left"></div>
    <div class="drop-indicator__cap drop-indicator__cap--right"></div>
  </div>
{/if}

<style>
  .drop-indicator {
    position: absolute;
    left: -6px;
    right: -6px;
    background: linear-gradient(90deg, var(--ds-interactive-subtle, #60a5fa), var(--ds-interactive, #2874bb));
    border-radius: 9999px;
    box-shadow:
      0 0 0 1px var(--ds-surface-raised, #ffffff),
      0 4px 10px rgba(59, 130, 246, 0.25);
    pointer-events: none;
    z-index: 40;
    opacity: 0.98;
  }

  .drop-indicator__cap {
    position: absolute;
    top: -3px;
    width: 8px;
    height: 8px;
    background: var(--ds-interactive, #2874bb);
    border-radius: 9999px;
    box-shadow: 0 0 0 1px var(--ds-surface-raised, #ffffff);
  }

  .drop-indicator__cap--left {
    left: -6px;
  }

  .drop-indicator__cap--right {
    right: -6px;
  }
</style>
