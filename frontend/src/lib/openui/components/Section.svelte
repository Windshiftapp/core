<script>
  /** @type {{ props: { children?: unknown[], heading?: string }, renderNode: import('svelte').Snippet<[unknown]> }} */
  let { props, renderNode } = $props();

  const children = $derived(Array.isArray(props.children) ? props.children : []);
</script>

<section class="space-y-2" data-testid="openui-section">
  {#if props.heading}
    <h2 class="text-xs font-semibold uppercase tracking-wide" style="color: var(--ds-text-subtle);">
      {props.heading}
    </h2>
  {/if}
  {#each children as child, index (index)}
    {@render renderNode(child)}
  {/each}
</section>
