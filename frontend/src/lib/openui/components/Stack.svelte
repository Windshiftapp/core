<script>
  /** @type {{ props: { children?: unknown[], direction?: 'vertical'|'horizontal', gap?: 'none'|'sm'|'md'|'lg' }, renderNode: import('svelte').Snippet<[unknown]> }} */
  let { props, renderNode } = $props();

  const gaps = { none: 'gap-0', sm: 'gap-2', md: 'gap-4', lg: 'gap-6' };
  const gapClass = $derived(gaps[props.gap ?? 'md'] ?? gaps.md);
  const directionClass = $derived(props.direction === 'horizontal' ? 'flex-row items-start' : 'flex-col');
  const children = $derived(Array.isArray(props.children) ? props.children : []);
</script>

<div class="flex {directionClass} {gapClass}" data-testid="openui-stack">
  {#each children as child, index (index)}
    {@render renderNode(child)}
  {/each}
</div>
