<script>
  let {
    for: forId = null,
    required = false,
    size = 'sm',           // 'xs' | 'sm' | 'md'
    color = 'subtle',      // 'default' | 'subtle'
    class: className = '',
    children
  } = $props();

  const sizeClass = $derived({
    xs: 'text-xs',
    sm: 'text-sm',
    md: 'text-base'
  }[size] || 'text-sm');

  const colorToken = $derived(color === 'subtle' ? '--ds-text-subtle' : '--ds-text');
</script>

<label
  for={forId}
  class="block font-medium {sizeClass} {className}"
  style="color: var({colorToken});"
>
  {@render children?.()}
  {#if required}
    <span class="text-red-500 ml-0.5">*</span>
  {/if}
</label>
