<script>
  export let workspaceName = '';
  export let collection = '';
  export let viewName = '';
  export let itemCount = null;
  export let actionButtons = null; // Slot for action buttons on the right
  export let hasGradient = false; // Whether gradient is active
  export let textStyle = 'color: var(--ds-text);'; // Main text color style
  export let subtleTextStyle = 'color: var(--ds-text-subtle);'; // Subtle text color style
</script>

<div class="flex items-center justify-between mb-4">
  <div>
    <div class="flex items-baseline gap-2 mb-1">
      <h1 class="text-xl font-medium" style={textStyle}>
        {viewName}
      </h1>
      {#if collection}
        <span class="text-xs font-medium px-1.5 py-0.5 rounded" style={hasGradient ? 'background-color: rgba(255,255,255,0.2); color: white; backdrop-filter: blur(4px);' : 'background-color: var(--ds-accent-blue-subtler); color: var(--ds-accent-blue);'}>
          {collection}
        </span>
      {/if}
    </div>
    <div class="flex items-center">
      <span class="text-sm" style={subtleTextStyle}>
        {workspaceName}{#if itemCount !== null} • {itemCount} items{/if}
      </span>
    </div>
  </div>

  {#if actionButtons}
    <div>
      <slot name="actions" />
    </div>
  {:else if $$slots.actions}
    <div>
      <slot name="actions" />
    </div>
  {/if}
</div>
