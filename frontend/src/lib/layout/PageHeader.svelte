<script>
  let {
    icon = null,
    title = '',
    subtitle = '',
    description = '',
    count = null,
    textStyle = '',
    subtitleStyle = '',
    children = null,
    actions = null
  } = $props();
  const subtitleText = $derived(subtitle || description);
</script>

<div class="flex items-center justify-between mb-8">
  <div>
    <h1 class="text-xl font-medium mb-2" style="{textStyle || 'color: var(--ds-text);'}">
      {title}
    </h1>
    {#if subtitleText || count !== null}
      <div class="flex items-center gap-2 text-sm" style="{subtitleStyle || 'color: var(--ds-text-subtle);'}">
        {#if icon && subtitleText}
          {@const Icon = icon}
          <Icon class="w-3.5 h-3.5" style={subtitleStyle || 'color: var(--ds-icon-subtle);'} />
        {/if}
        {#if subtitleText}
          <span>{subtitleText}</span>
        {/if}
        {#if count !== null}
          {#if subtitleText}<span style="color: var(--ds-text-disabled);">•</span>{/if}
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