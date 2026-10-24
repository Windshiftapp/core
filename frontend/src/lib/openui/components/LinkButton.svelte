<script>
  import Button from '../../components/Button.svelte';
  import { getWidgetDataContext } from '../context.js';

  /** @type {{ props: { label: string, href?: string, item_id?: number } }} */
  let { props } = $props();

  const context = getWidgetDataContext();

  // Only same-origin application routes are allowed. Protocol-relative and
  // absolute URLs are rejected, so a generated program cannot link off-site.
  const safeHref = $derived(
    typeof props.href === 'string' && props.href.startsWith('/') && !props.href.startsWith('//')
      ? props.href
      : null
  );

  function handleClick(event) {
    if (props.item_id && context?.openItem) {
      event.preventDefault();
      context.openItem(Number(props.item_id));
    }
  }
</script>

{#if safeHref || props.item_id}
  <Button
    variant="default"
    size="small"
    href={safeHref ?? undefined}
    onclick={handleClick}
    dataTestid="openui-link-button"
  >
    {props.label}
  </Button>
{:else}
  <span class="text-sm" style="color: var(--ds-text-subtle);">{props.label}</span>
{/if}
