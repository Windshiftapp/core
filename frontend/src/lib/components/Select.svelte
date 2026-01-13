<script>
  let {
    value = $bindable(''),
    disabled = false,
    required = false,
    size = 'medium',
    id = undefined,
    class: className = '',
    children
  } = $props();
  export { className as class };

  // Size variants
  const sizeClasses = $derived({
    small: 'px-3 py-2.5 text-sm',
    medium: 'px-4 py-3'
  }[size] || 'px-4 py-3');

  // Combine all classes
  const allClasses = $derived([
    'w-full rounded border transition-all duration-200',
    'focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-opacity-50',
    sizeClasses,
    className
  ].filter(Boolean).join(' '));
</script>

<select
  {id}
  bind:value
  {disabled}
  {required}
  class={allClasses}
  style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
  on:change
  on:focus
  on:blur
>
  {@render children?.()}
</select>
