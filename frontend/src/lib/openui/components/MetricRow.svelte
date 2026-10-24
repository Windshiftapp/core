<script>
  /** @type {{ props: { children?: unknown[] }, renderNode: import('svelte').Snippet<[unknown]> }} */
  let { props, renderNode } = $props();

  const children = $derived(Array.isArray(props.children) ? props.children : []);
</script>

<div class="grid grid-cols-2 gap-4 sm:grid-cols-3" data-testid="openui-metric-row">
  {#each children as child, index (index)}
    {@render renderNode(child)}
  {/each}
</div>
