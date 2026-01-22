<script>
  import Text from './Text.svelte';

  /**
   * @deprecated Use Text component instead:
   *   <Text as="p" variant="subtle" size="xs" class="mt-1">...</Text>
   *   <Text as="p" variant="danger" size="xs" class="mt-1">...</Text>
   */
  let {
    variant = 'default',  // 'default' | 'error' | 'success' | 'warning'
    class: className = '',
    children
  } = $props();

  // Map HelperText variants to Text variants
  const textVariant = $derived({
    default: 'subtle',
    error: 'danger',
    success: 'success',
    warning: 'warning'
  }[variant] || 'subtle');
</script>

<Text as="p" variant={textVariant} size="xs" class="mt-1 {className}">
  {@render children?.()}
</Text>
