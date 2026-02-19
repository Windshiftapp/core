<script>
  import LinkComponent from '../../components/Link.svelte';

  export let item;
  export let workspace = null;
  export let style = "color: var(--ds-text-subtle);";
  export let href = null;
  export let onClick = null;

  $: displayKey = (() => {
    const key = item.workspace_key || workspace?.key;
    return key ? `${key}-${item.workspace_item_number}` : `ITEM-${item.workspace_item_number}`;
  })();

  $: interactive = !!(href || onClick);
  $: classes = `text-xs font-mono flex-shrink-0 whitespace-nowrap${interactive ? ' hover:underline cursor-pointer' : ''}`;
</script>

{#if href}
  <LinkComponent {href} {onClick} class={classes} {style}>
    {displayKey}
  </LinkComponent>
{:else if onClick}
  <button class={classes} {style} on:click={onClick} type="button">
    {displayKey}
  </button>
{:else}
  <span class={classes} {style}>
    {displayKey}
  </span>
{/if}
