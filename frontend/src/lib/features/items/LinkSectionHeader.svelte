<script>
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import Text from '../../components/Text.svelte';

  /**
   * Collapsible section header used by the item integration/SCM link panels:
   * divider, title, optional count badge, caller-provided action buttons, and
   * the expand/collapse chevron.
   *
   * @type {{
   *   title: string,
   *   count?: number,
   *   expanded: boolean,
   *   ontoggle: () => void,
   *   actions?: import('svelte').Snippet<[{ expanded: boolean }]>,
   * }}
   */
  let { title, count = 0, expanded, ontoggle, actions } = $props();
</script>

<div class="border-t my-4" style="border-color: var(--ds-border);"></div>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="w-full flex items-center justify-between mb-3 group cursor-pointer"
  onclick={ontoggle}
>
  <div class="flex items-center gap-2">
    <Text variant="subtle" size="xs" weight="semibold" class="uppercase tracking-wider">{title}</Text>
    {#if count > 0}
      <span
        class="text-xs px-1.5 py-0.5 rounded"
        style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);"
      >
        {count}
      </span>
    {/if}
  </div>
  <div class="flex items-center gap-1">
    {@render actions?.({ expanded })}
    {#if expanded}
      <ChevronDown class="w-4 h-4" style="color: var(--ds-text-subtle);" />
    {:else}
      <ChevronRight class="w-4 h-4" style="color: var(--ds-text-subtle);" />
    {/if}
  </div>
</div>
