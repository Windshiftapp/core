<script>
  import Card from '../../components/Card.svelte';

  /** @type {{ props: { children?: unknown[], padding?: 'none'|'compact'|'default'|'spacious' }, renderNode: import('svelte').Snippet<[unknown]> }} */
  let { props, renderNode } = $props();

  const children = $derived(Array.isArray(props.children) ? props.children : []);
</script>

<Card variant="flat" padding={props.padding ?? 'default'}>
  {#each children as child, index (index)}
    {@render renderNode(child)}
  {/each}
</Card>
