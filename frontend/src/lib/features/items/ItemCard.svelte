<script>
  let { href = null, onclick = null, compact = false, children } = $props();
</script>

{#if href}
  <a
    {href}
    class="item-card block {compact ? 'p-2' : 'p-3'} rounded-lg border no-underline text-inherit group"
    style="background-color: var(--ds-surface-card); border-color: var(--ds-border);"
    {onclick}
  >
    <div class="item-card-content relative">
      {@render children()}
    </div>
  </a>
{:else}
  <div
    class="item-card block {compact ? 'p-2' : 'p-3'} rounded-lg border group"
    style="background-color: var(--ds-surface-card); border-color: var(--ds-border);"
    role={onclick ? 'button' : undefined}
    tabindex={onclick ? 0 : undefined}
    {onclick}
    onkeydown={(e) => { if (onclick && (e.key === 'Enter' || e.key === ' ')) { e.preventDefault(); onclick(e); } }}
  >
    <div class="item-card-content relative">
      {@render children()}
    </div>
  </div>
{/if}

<style>
  .item-card {
    position: relative;
    box-shadow: 0 1px 2px 0 rgba(0, 0, 0, 0.05);
    transition:
      transform var(--duration-fast, 100ms) var(--ease-spring, cubic-bezier(0.34, 1.56, 0.64, 1)),
      box-shadow var(--duration-normal, 200ms) var(--ease-smooth, ease),
      border-color var(--duration-fast, 100ms) var(--ease-smooth, ease);
  }

  .item-card:hover {
    transform: translateY(-4px);
    box-shadow: var(--shadow-lift, 0 8px 30px rgba(0, 0, 0, 0.12));
    border-color: var(--ds-border-bold);
  }

  .item-card:active {
    transform: translateY(-2px);
    box-shadow: 0 4px 15px rgba(0, 0, 0, 0.1);
  }

  /* Subtle gradient overlay on hover */
  .item-card::after {
    content: '';
    position: absolute;
    inset: 0;
    border-radius: inherit;
    background: linear-gradient(
      135deg,
      transparent 0%,
      transparent 50%,
      rgba(59, 130, 246, 0.03) 100%
    );
    opacity: 0;
    transition: opacity var(--duration-normal, 200ms) var(--ease-smooth, ease);
    pointer-events: none;
  }

  .item-card:hover::after {
    opacity: 1;
  }

  /* Content stays above overlay */
  .item-card-content {
    position: relative;
    z-index: 1;
  }

  /* Reduced motion support */
  @media (prefers-reduced-motion: reduce) {
    .item-card:hover,
    .item-card:active {
      transform: none;
    }
  }
</style>
