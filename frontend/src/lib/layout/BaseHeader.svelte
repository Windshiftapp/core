<script>
  let {
    title = '',
    badge = '',
    badgeStyle = 'background-color: var(--ctx-active-bg, var(--ds-interactive-subtle)); color: var(--ctx-active-text, var(--ds-interactive)); backdrop-filter: var(--ctx-backdrop, none);',
    subtitle = '',
    description = '',
    icon = null,
    count = null,
    children = null,
    actions = null,
    textStyle = '',
    subtitleStyle = '',
    iconStyle = '',
    marginClass = 'mb-8'
  } = $props();

  const subtitleText = $derived(subtitle || description);
  const iconStyleProp = $derived(iconStyle || 'color: var(--ds-icon-subtle);');
  const subtitleStyleProp = $derived(subtitleStyle || 'color: var(--ds-text-subtle);');
</script>

<div class="flex flex-col items-stretch gap-4 sm:flex-row sm:items-center sm:justify-between {marginClass}">
  <!-- On `sm+` the action cluster keeps its natural width and this column takes
       the remainder (down to a readable floor). Sizing the text column from the
       actions instead of from its own content means late-arriving subtitle/count
       text cannot reflow the actions into an extra row after paint. -->
  <div class="min-w-0 sm:flex-1 sm:min-w-32">
    <div class="flex items-baseline gap-2 mb-2">
      <h1 class="text-xl font-medium" style="{textStyle || 'color: var(--ds-text);'}">
        {title}
      </h1>
      {#if badge}
        <span class="text-xs font-medium px-1.5 py-0.5 rounded" style={badgeStyle}>{badge}</span>
      {/if}
    </div>
    {#if subtitleText || count !== null}
      <div class="flex min-w-0 items-start gap-2 text-sm sm:items-center" style="{subtitleStyleProp}">
        {#if icon && subtitleText}
          {@const Icon = icon}
          <Icon class="w-3.5 h-3.5" style={iconStyleProp} />
        {/if}
        {#if subtitleText}
          <span class="min-w-0" data-testid="page-header-subtitle">{subtitleText}</span>
        {/if}
        {#if count !== null}
          {#if subtitleText}<span style="color: var(--ds-text-disabled);">•</span>{/if}
          <span style="color: var(--ds-text-disabled);">{count}</span>
        {/if}
      </div>
    {/if}
  </div>

  {#if actions}
    <div class="self-start sm:self-auto">
      {@render actions()}
    </div>
  {/if}
</div>
