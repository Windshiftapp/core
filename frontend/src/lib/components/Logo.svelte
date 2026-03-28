<script>
  let { size = 'medium' } = $props();

  const sizes = {
    small: 'w-6 h-6',
    medium: 'w-8 h-8',
    large: 'w-12 h-12',
  };
</script>

<img src="/windshift-3.svg" alt="Windshift" class="{sizes[size] || sizes.medium}" />
