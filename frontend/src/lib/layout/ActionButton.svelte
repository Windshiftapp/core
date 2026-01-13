<script>
  /**
   * ActionButton - Small action button with icon and optional label
   *
   * Use in toolbars, action groups, and compact UI areas.
   * Different from Button which is for primary actions.
   *
   * @example
   * <ActionButton icon={Edit} label="Edit" onclick={handleEdit} />
   * <ActionButton icon={Trash2} variant="subtle" onclick={handleDelete} />
   * <ActionButton icon={Copy} size="xs" />
   */
  let {
    icon,
    label = '',
    showLabel = true,     // Can hide label on mobile
    variant = 'ghost',    // 'ghost' | 'subtle' | 'danger'
    size = 'sm',          // 'xs' | 'sm'
    disabled = false,
    loading = false,
    title: titleProp = '',
    onclick = null,
    class: className = ''
  } = $props();

  const sizeClasses = $derived({
    xs: 'px-1.5 py-1 text-xs',
    sm: 'px-2 py-1.5 text-sm'
  }[size] || 'px-2 py-1.5 text-sm');

  const iconSize = $derived({
    xs: 'w-3.5 h-3.5',
    sm: 'w-4 h-4'
  }[size] || 'w-4 h-4');

  const variantStyles = $derived({
    ghost: `
      background-color: transparent;
      color: var(--ds-text-subtle);
    `,
    subtle: `
      background-color: var(--ds-background-neutral);
      color: var(--ds-text-subtle);
    `,
    danger: `
      background-color: transparent;
      color: var(--ds-text-danger);
    `
  }[variant] || 'background-color: transparent; color: var(--ds-text-subtle);');

  const hoverClass = $derived({
    ghost: 'hover:bg-[var(--ds-background-neutral-hovered)]',
    subtle: 'hover:bg-[var(--ds-background-neutral-hovered)]',
    danger: 'hover:bg-[var(--ds-accent-red-subtle)]'
  }[variant] || 'hover:bg-[var(--ds-background-neutral-hovered)]');

  const buttonTitle = $derived(titleProp || label || '');
</script>

<button
  type="button"
  {onclick}
  disabled={disabled || loading}
  title={buttonTitle}
  class="inline-flex items-center gap-1.5 rounded transition-all {sizeClasses} {hoverClass} {className} disabled:opacity-50 disabled:cursor-not-allowed"
  style={variantStyles}
>
  {#if loading}
    <svg class="animate-spin {iconSize}" viewBox="0 0 24 24" fill="none">
      <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
      <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
    </svg>
  {:else}
    <svelte:component this={icon} class={iconSize} />
  {/if}
  {#if label && showLabel}
    <span>{label}</span>
  {/if}
</button>
