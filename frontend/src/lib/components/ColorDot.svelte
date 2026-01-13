<script>
  let {
    color = '#9CA3AF',
    size = 'sm', // 'xs', 'sm', 'md'
    class: className = ''
  } = $props();

  const sizeClasses = $derived({
    xs: 'w-1.5 h-1.5',
    sm: 'w-2 h-2',
    md: 'w-3 h-3'
  }[size] || 'w-2 h-2');
</script>

<div
  class="{sizeClasses} rounded-full flex-shrink-0 {className}"
  style="background-color: {color};"
></div>
