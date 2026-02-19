<script>
  let { href = null, onclick = null, compact = false, hasGradient = false, children } = $props();

  let bgStyle = $derived(hasGradient
    ? 'backdrop-filter: blur(12px); background-color: var(--ds-glass-bg);'
    : 'background-color: var(--ds-surface-card);');
  let borderStyle = $derived(hasGradient
    ? 'border-color: var(--ds-glass-border);'
    : 'border-color: transparent;');
</script>

{#if href}
  <a
    {href}
    class="item-card block p-3 rounded-lg border no-underline text-inherit group"
    style="{bgStyle} {borderStyle}"
    {onclick}
  >
    <div class="item-card-content relative">
      {@render children()}
    </div>
  </a>
{:else}
  <div
    class="item-card block p-3 rounded-lg border group"
    style="{bgStyle} {borderStyle}"
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
    box-shadow: var(--ds-shadow-raised);
    transition:
      background-color 140ms ease-in-out,
      box-shadow 140ms ease-in-out,
      border-color var(--duration-fast, 100ms) var(--ease-smooth, ease);
  }

  .item-card:hover {
    box-shadow: var(--ds-shadow-raised);
    background-color: var(--ds-surface-raised-hovered) !important;
  }

  .item-card:active {
    box-shadow: var(--ds-shadow-raised);
  }

</style>
