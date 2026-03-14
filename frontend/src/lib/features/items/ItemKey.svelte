<script>
  import LinkComponent from '../../components/Link.svelte';

  let { item, workspace = null, style = "color: var(--ds-text-subtle);", href = null, onClick = null } = $props();

  let displayKey = $derived((() => {
    const key = item.workspace_key || workspace?.key;
    return key ? `${key}-${item.workspace_item_number}` : `ITEM-${item.workspace_item_number}`;
  })());

  let interactive = $derived(!!(href || onClick));
  let classes = $derived(`text-xs font-mono flex-shrink-0 whitespace-nowrap${interactive ? ' hover:underline cursor-pointer' : ''}`);
</script>

{#if href}
  <LinkComponent {href} {onClick} class={classes} {style}>
    {displayKey}
  </LinkComponent>
{:else if onClick}
  <button class={classes} {style} onclick={onClick} type="button">
    {displayKey}
  </button>
{:else}
  <span class={classes} {style}>
    {displayKey}
  </span>
{/if}
