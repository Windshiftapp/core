<script>
  import LinkComponent from './Link.svelte';
  import { itemModalStore } from '../stores/itemModalStore.svelte.js';
  import { formatItemKey } from '../utils/itemKey.js';

  /**
   * Inline work-item key. `plain` renders the mono key on its own; `badge`
   * renders the neutral metadata chip used in ticket lists and card rows.
   * Accepts an `item` object or a pre-formatted `itemKey` string. A standalone
   * key opens the global item modal unless `clickable={false}` or an explicit
   * `href`/`onClick` takes over.
   */
  let {
    item = null,
    itemKey = null,
    workspace = null,
    size = 'default',
    variant = 'plain',
    style = null,
    href = null,
    onClick = null,
    monospace = true,
    clickable = true,
    dataTestid = undefined,
    title = undefined,
    class: className = '',
  } = $props();

  const displayKey = $derived.by(() => {
    if (itemKey) return itemKey;
    if (!item) return '';
    if (item.item_key) return item.item_key;
    const fromFields = formatItemKey(item);
    if (fromFields) return fromFields;
    const key = item.workspace_key || workspace?.key;
    return key ? `${key}-${item.workspace_item_number}` : `ITEM-${item.workspace_item_number}`;
  });

  // A standalone key opens the global item modal. Keys nested in an existing
  // link or button pass `clickable={false}` so they stay plain text.
  const canOpenModal = $derived(clickable && !href && !onClick && item?.id != null);
  const interactive = $derived(!!(href || onClick || canOpenModal));
  const badge = $derived(variant === 'badge');

  // Keep a real href so cmd/ctrl/shift/middle-click still open a new tab; only
  // an unmodified left-click is diverted to the modal.
  const itemHref = $derived.by(() => {
    const wsId = item?.workspace_id ?? workspace?.id;
    return item?.id != null && wsId != null ? `/workspaces/${wsId}/items/${item.id}` : null;
  });
  const linkHref = $derived(href ?? (canOpenModal ? itemHref : null));

  function handleClick(event) {
    if (onClick) {
      onClick(event);
      return;
    }
    if (canOpenModal) {
      event?.preventDefault?.();
      event?.stopPropagation?.();
      itemModalStore.open(item.id, item.workspace_id ?? workspace?.id ?? null);
    }
  }

  const sizeClass = $derived.by(() => {
    if (badge) {
      return size === 'compact'
        ? 'text-[10px] leading-4 px-1 py-0.5'
        : 'text-xs leading-4 px-1.5 py-0.5';
    }
    return size === 'compact' ? 'text-[10px] leading-4 tracking-[0.02em]' : 'text-xs';
  });

  const classes = $derived.by(() => {
    if (badge) {
      return [
        'inline-flex items-center rounded flex-shrink-0 whitespace-nowrap',
        monospace ? 'font-mono' : '',
        sizeClass,
        interactive ? 'transition-colors hover:bg-[var(--ds-background-neutral-hovered)] cursor-pointer' : '',
        className,
      ].filter(Boolean).join(' ');
    }
    return [
      sizeClass,
      monospace ? 'font-mono' : '',
      'flex-shrink-0 whitespace-nowrap',
      interactive ? 'hover:underline cursor-pointer' : '',
      className,
    ].filter(Boolean).join(' ');
  });

  const resolvedStyle = $derived(
    style ??
      (badge
        ? 'background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);'
        : 'color: var(--ds-text-subtle);'),
  );
</script>

{#if linkHref}
  <LinkComponent href={linkHref} onClick={handleClick} class={classes} style={resolvedStyle} data-testid={dataTestid} {title}>
    {displayKey}
  </LinkComponent>
{:else if interactive}
  <button class={classes} style={resolvedStyle} onclick={handleClick} type="button" data-testid={dataTestid} {title}>
    {displayKey}
  </button>
{:else}
  <span class={classes} style={resolvedStyle} data-testid={dataTestid} {title}>
    {displayKey}
  </span>
{/if}
