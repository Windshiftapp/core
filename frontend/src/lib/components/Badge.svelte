<script>
  /**
   * Badge - Status pill for informational badges
   *
   * Different from Lozenge (which is for colored semantic tags).
   * Badge is for small status indicators, counts, and info pills.
   *
   * @example
   * <Badge variant="success">Active</Badge>
   * <Badge variant="info" icon={Info}>New</Badge>
   * <Badge size="xs">3</Badge>
   */
  let {
    variant = 'neutral',  // 'neutral' | 'info' | 'success' | 'warning' | 'danger'
    size = 'sm',          // 'xs' | 'sm' | 'md'
    icon = null,          // Optional Lucide icon component
    class: className = '',
    children
  } = $props();

  const sizeClasses = $derived({
    xs: 'text-xs px-1.5 py-0.5',
    sm: 'text-xs px-2 py-0.5',
    md: 'text-sm px-2.5 py-1'
  }[size] || 'text-xs px-2 py-0.5');

  const iconSize = $derived({
    xs: 'w-3 h-3',
    sm: 'w-3.5 h-3.5',
    md: 'w-4 h-4'
  }[size] || 'w-3.5 h-3.5');

  const variantStyles = $derived({
    neutral: 'background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);',
    info: 'background-color: var(--ds-accent-blue-subtle); color: var(--ds-text-accent-blue);',
    success: 'background-color: var(--ds-accent-green-subtle); color: var(--ds-text-accent-green);',
    warning: 'background-color: var(--ds-accent-yellow-subtle); color: var(--ds-text-accent-yellow);',
    danger: 'background-color: var(--ds-accent-red-subtle); color: var(--ds-text-danger);'
  }[variant] || 'background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);');
</script>

<span
  class="inline-flex items-center gap-1 rounded-full font-medium {sizeClasses} {className}"
  style={variantStyles}
>
  {#if icon}
    <svelte:component this={icon} class={iconSize} />
  {/if}
  {@render children?.()}
</span>
