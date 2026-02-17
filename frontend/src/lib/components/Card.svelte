<script>
  let {
    variant = 'raised',    // 'raised', 'flat', 'outlined', 'dashed'
    padding = 'default',   // 'none', 'compact', 'default', 'spacious', 'loose', 'generous'
    shadow = false,        // Add shadow-sm
    hoverable = false,     // Add hover effects (shadow-md, translate-y)
    glass = false,         // Glassmorphism effect
    href = null,           // If provided, renders as <a>
    onclick = null,        // Click handler
    rounded = 'lg',        // 'none', 'sm', 'md', 'lg', 'xl'
    class: className = '',
    style: userStyle = '',
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

  const glassStyle = 'background-color: var(--ds-glass-bg); border-color: var(--ds-glass-border);';

  // Padding classes for the body
  const paddingClasses = {
    none: '',
    compact: 'p-3',
    default: 'p-4',
    spacious: 'p-6',
    loose: 'p-8',
    generous: 'p-12'
  };

  // Rounded classes
  const roundedClasses = {
    none: '',
    sm: 'rounded-sm',
    md: 'rounded-md',
    lg: 'rounded-lg',
    xl: 'rounded-xl'
  };

  const hasStructure = $derived(!!header || !!footer);

  const baseClasses = $derived([
    'border',
    roundedClasses[rounded],
    shadow ? 'shadow-sm' : '',
    hoverable ? 'transition-all duration-150 hover:shadow-md hover:-translate-y-px' : '',
    // When no header/footer, apply padding directly on outer element
    !hasStructure ? paddingClasses[padding] : '',
    className
  ].filter(Boolean).join(' '));

  const bodyClasses = $derived(paddingClasses[padding]);

  const computedStyle = $derived([
    glass ? glassStyle : variantStyles[variant],
    userStyle
  ].filter(Boolean).join(' '));
</script>

{#if href}
  <a {href} class={baseClasses} style={computedStyle} class:glass onclick={onclick}>
    {#if hasStructure}
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
    {:else}
      {@render children?.()}
    {/if}
  </a>
{:else}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class={baseClasses} style={computedStyle} class:glass onclick={onclick}>
    {#if hasStructure}
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
    {:else}
      {@render children?.()}
    {/if}
  </div>
{/if}

<style>
  .glass {
    backdrop-filter: blur(12px) saturate(180%);
    -webkit-backdrop-filter: blur(12px) saturate(180%);
  }
</style>
