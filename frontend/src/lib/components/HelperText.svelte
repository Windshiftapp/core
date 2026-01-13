<script>
  let {
    variant = 'default',  // 'default' | 'error' | 'success' | 'warning'
    class: className = '',
    children
  } = $props();

  const colorToken = $derived({
    default: '--ds-text-subtle',
    error: '--ds-text-danger',
    success: '--ds-text-success',
    warning: '--ds-text-warning'
  }[variant] || '--ds-text-subtle');
</script>

<p
  class="text-xs mt-1 {className}"
  style="color: var({colorToken});"
>
  {@render children?.()}
</p>
