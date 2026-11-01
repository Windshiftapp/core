<script>
  import SidebarHeader from './SidebarHeader.svelte';
  import ScrollableSidebar from './ScrollableSidebar.svelte';

  let {
    title,
    description = '',
    header = undefined,
    children,
    footer = undefined,
    class: className = '',
    contentClass = 'px-6 pb-6',
    ariaLabel = undefined,
    ...restProps
  } = $props();
</script>

{#snippet defaultHeader()}
  <div class="px-6 pt-6 pb-3">
    <SidebarHeader {title} {description} noBorder />
  </div>
{/snippet}

<ScrollableSidebar
  data-testid="navigation-sidebar"
  {...restProps}
  class="navigation-sidebar h-full min-h-0 w-64 min-w-64 flex-shrink-0 border-r {className}"
  style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);"
  aria-label={ariaLabel || title}
  header={header || defaultHeader}
  {footer}
  scrollClass={contentClass}
>
  {@render children?.()}
</ScrollableSidebar>
