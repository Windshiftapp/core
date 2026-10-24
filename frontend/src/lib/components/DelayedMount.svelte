<script>
  let { delay = 175, children } = $props();

  let visible = $state(false);

  // Loading states render through this wrapper so fast async loads (warm
  // route chunks, cached imports) never flash a spinner, while slow loads
  // still show one. The timer is cleared when the pending state unmounts.
  $effect(() => {
    const timer = setTimeout(() => (visible = true), delay);
    return () => clearTimeout(timer);
  });
</script>

{#if visible}
  {@render children()}
{/if}
