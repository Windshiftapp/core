<script>
  let {
    icon = null, // Icon component to display
    title = '',
    subtitle = '',
    count = null, // Optional count to display (e.g., "12 items")
    textStyle = '',
    subtitleStyle = '',
    children = null,
    actions = null
  } = $props();
</script>

<div class="flex items-center justify-between mb-8">
  <div>
    <h1 class="text-xl font-medium mb-2" style="{textStyle || 'color: var(--ds-text);'}">
      {title}
    </h1>
    {#if subtitle || count !== null}
      <div class="flex items-center gap-2 text-sm" style="{subtitleStyle || 'color: var(--ds-text-subtle);'}">
        {#if icon && subtitle}
          {@const Icon = icon}
          <Icon class="w-3.5 h-3.5" style="{subtitleStyle || 'color: var(--ds-icon-subtle);'}" />
        {/if}
        {#if subtitle}
          <span>{subtitle}</span>
        {/if}
        {#if count !== null}
          {#if subtitle}<span style="color: var(--ds-text-disabled);">•</span>{/if}
          <span style="color: var(--ds-text-disabled);">{count}</span>
        {/if}
      </div>
    {/if}
  </div>

  {#if actions}
    <div>
      {@render actions()}
    </div>
  {/if}
</div>