<script>
  import LinkComponent from '../../components/Link.svelte';

  /**
   * ItemKey component - Standard display for work item keys
   * Shows workspace key + workspace-specific item number (e.g., WORKSPACE-1)
   *
   * Can be used as a link by providing href prop, or as plain text without it.
   */
  export let item;
  export let workspace = null;
  export let className = "text-xs font-mono";
  export let style = "color: var(--ds-text-subtle);"; // Default style
  export let href = null; // Optional: if provided, renders as a link
  export let onClick = null; // Optional: custom click handler (e.g., for modals)

  // Compute the display key
  $: displayKey = (() => {
    const key = item.workspace_key || workspace?.key;
    return key ? `${key}-${item.workspace_item_number}` : `ITEM-${item.workspace_item_number}`;
  })();
</script>

{#if href}
  <LinkComponent {href} {onClick} class={className} {style}>
    {displayKey}
  </LinkComponent>
{:else}
  <span class={className} {style} title="Work item key">
    {displayKey}
  </span>
{/if}
