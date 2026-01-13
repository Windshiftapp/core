<script>
  let {
    variant = 'raised',    // 'raised', 'flat', 'outlined', 'dashed'
    padding = 'default',   // 'none', 'compact', 'default', 'spacious'
    shadow = false,        // Add shadow-sm
    hoverable = false,     // Add hover effects (shadow-md, translate-y)
    href = null,           // If provided, renders as <a>
    onclick = null,        // Click handler
    rounded = 'lg',        // 'none', 'sm', 'md', 'lg', 'xl'
    class: className = '',
    children,
    header,                // Snippet for header content
    footer                 // Snippet for footer content
  } = $props();

  // Variant styles using design tokens
  const variantStyles = {
    raised: 'background-color: var(--ds-surface-raised); border-color: var(--ds-border);',
    flat: 'background-color: var(--ds-surface); border-color: var(--ds-border);',
    outlined: 'background-color: transparent; border-color: var(--ds-border);',
    dashed: 'background-color: var(--ds-surface-raised); border-color: var(--ds-border); border-style: dashed;'
  };

  // Padding classes for the body
  const paddingClasses = {
    none: '',
    compact: 'p-3',
    default: 'p-4',
    spacious: 'p-6'
  };

  // Rounded classes
  const roundedClasses = {
    none: '',
    sm: 'rounded-sm',
    md: 'rounded-md',
    lg: 'rounded-lg',
    xl: 'rounded-xl'
  };

  const baseClasses = $derived([
    'border',
    roundedClasses[rounded],
    shadow ? 'shadow-sm' : '',
    hoverable ? 'transition-all duration-150 hover:shadow-md hover:-translate-y-px' : '',
    className
  ].filter(Boolean).join(' '));

  const bodyClasses = $derived(paddingClasses[padding]);
</script>

{#if href}
  <a {href} class={baseClasses} style={variantStyles[variant]} onclick={onclick}>
    {#if header}
      <div class="px-4 py-3 border-b" style="border-color: var(--ds-border);">
        {@render header()}
      </div>
    {/if}
    <div class={bodyClasses}>
      {@render children?.()}
    </div>
    {#if footer}
      <div class="px-4 py-3 border-t" style="border-color: var(--ds-border);">
        {@render footer()}
      </div>
    {/if}
  </a>
{:else}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class={baseClasses} style={variantStyles[variant]} onclick={onclick}>
    {#if header}
      <div class="px-4 py-3 border-b" style="border-color: var(--ds-border);">
        {@render header()}
      </div>
    {/if}
    <div class={bodyClasses}>
      {@render children?.()}
    </div>
    {#if footer}
      <div class="px-4 py-3 border-t" style="border-color: var(--ds-border);">
        {@render footer()}
      </div>
    {/if}
  </div>
{/if}
