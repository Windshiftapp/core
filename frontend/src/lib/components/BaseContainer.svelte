<script>
  let {
    padding = 'default',  // 'none', 'compact', 'default', 'spacious'
    glass = false,        // Enable glassmorphism effect
    class: className = '',
    children
  } = $props();

  const paddingClasses = {
    none: '',
    compact: 'p-3',
    default: 'p-4',
    spacious: 'p-6'
  };

  const baseClasses = $derived([
    'rounded-lg',
    'border',
    'shadow-sm',
    paddingClasses[padding],
    className
  ].filter(Boolean).join(' '));

  const containerStyle = $derived(glass
    ? 'background-color: var(--ds-glass-bg); border-color: var(--ds-glass-border);'
    : 'background-color: var(--ds-surface-raised); border-color: var(--ds-border);'
  );
</script>

<div
  class={baseClasses}
  class:glass
  style={containerStyle}
>
  {@render children?.()}
</div>

<style>
  .glass {
    background-color: var(--ds-glass-bg);
    border-color: var(--ds-glass-border);
    backdrop-filter: blur(12px) saturate(180%);
    -webkit-backdrop-filter: blur(12px) saturate(180%);
  }
</style>
