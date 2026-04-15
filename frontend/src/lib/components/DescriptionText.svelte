<script>
  import Text from './Text.svelte';

  let {
    as = 'p',
    variant = 'subtle',
    class: className = '',
    children
  } = $props();
</script>

<Text {as} {variant} size="xs" class="mt-1 {className}">
  {@render children?.()}
</Text>
