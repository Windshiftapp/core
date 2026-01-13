<script>
  let {
    title,
    subtitle = null,
    icon = null,        // Lucide icon component
    size = 'default',   // 'sm', 'default', 'lg'
    noBorder = false,   // Hide bottom border (for use outside Card)
    actions,            // Snippet for action buttons
    class: className = ''
  } = $props();

  // Size mappings
  const sizeClasses = {
    sm: 'text-sm',
    default: 'text-base',
    lg: 'text-lg'
  };

  const iconSizeClasses = {
    sm: 'w-4 h-4',
    default: 'w-5 h-5',
    lg: 'w-6 h-6'
  };
</script>

<div class="flex items-center justify-between gap-3 {className}">
  <div class="flex items-center gap-2 min-w-0">
    {#if icon}
      <svelte:component this={icon} class="{iconSizeClasses[size]} flex-shrink-0" style="color: var(--ds-icon);" />
    {/if}
    <div class="min-w-0">
      <h3 class="{sizeClasses[size]} font-medium truncate" style="color: var(--ds-text);">
        {title}
      </h3>
      {#if subtitle}
        <p class="text-sm truncate" style="color: var(--ds-text-subtle);">
          {subtitle}
        </p>
      {/if}
    </div>
  </div>
  {#if actions}
    <div class="flex items-center gap-2 flex-shrink-0">
      {@render actions()}
    </div>
  {/if}
</div>
