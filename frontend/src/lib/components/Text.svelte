<script>
  let {
    variant = 'default', // 'default', 'subtle', 'subtlest', 'disabled', 'inverse', 'link', 'danger', 'success', 'warning', 'info'
    size = 'base',       // 'xs', 'sm', 'base', 'lg', 'xl', '2xl', '3xl', '4xl'
    weight = 'normal',   // 'normal', 'medium', 'semibold', 'bold'
    as = 'span',         // 'span', 'p', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'label', 'div'
    truncate = false,
    leading = null,      // 'none', 'tight', 'snug', 'normal', 'relaxed', 'loose'
    align = null,        // 'left', 'center', 'right'
    class: className = '',
    children
  } = $props();

  // Map variants to design tokens
  const variantTokens = {
    default: 'var(--ds-text)',
    subtle: 'var(--ds-text-subtle)',
    subtlest: 'var(--ds-text-subtlest)',
    disabled: 'var(--ds-text-disabled)',
    inverse: 'var(--ds-text-inverse)',
    link: 'var(--ds-text-link)',
    danger: 'var(--ds-text-danger)',
    success: 'var(--ds-text-success)',
    warning: 'var(--ds-text-warning)',
    info: 'var(--ds-text-info)'
  };

  // Map sizes to Tailwind classes
  const sizeClasses = {
    xs: 'text-xs',
    sm: 'text-sm',
    base: 'text-base',
    lg: 'text-lg',
    xl: 'text-xl',
    '2xl': 'text-2xl',
    '3xl': 'text-3xl',
    '4xl': 'text-4xl'
  };

  // Map weights to Tailwind classes
  const weightClasses = {
    normal: 'font-normal',
    medium: 'font-medium',
    semibold: 'font-semibold',
    bold: 'font-bold'
  };

  // Map leading to Tailwind classes
  const leadingClasses = {
    none: 'leading-none',
    tight: 'leading-tight',
    snug: 'leading-snug',
    normal: 'leading-normal',
    relaxed: 'leading-relaxed',
    loose: 'leading-loose'
  };

  // Map align to Tailwind classes
  const alignClasses = {
    left: 'text-left',
    center: 'text-center',
    right: 'text-right'
  };

  const colorStyle = $derived(`color: ${variantTokens[variant]};`);

  const allClasses = $derived([
    sizeClasses[size],
    weightClasses[weight],
    leading ? leadingClasses[leading] : '',
    align ? alignClasses[align] : '',
    truncate ? 'truncate' : '',
    className
  ].filter(Boolean).join(' '));
</script>

<svelte:element this={as} class={allClasses} style={colorStyle}>
  {@render children?.()}
</svelte:element>
