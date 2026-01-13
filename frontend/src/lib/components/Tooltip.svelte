<script>
  import { createTooltip, melt } from '@melt-ui/svelte';

  let {
    content,
    children,
    placement = 'bottom',
    delay = { open: 300, close: 0 },
    class: className = ''
  } = $props();

  const {
    elements: { trigger, content: tooltipContent },
    states: { open }
  } = createTooltip({
    positioning: {
      placement: placement
    },
    openDelay: delay.open,
    closeDelay: delay.close,
    disableHoverableContent: true,
    forceVisible: true
  });
</script>

<span use:melt={$trigger} class="cursor-pointer {className}">
  {@render children()}
</span>

{#if $open}
  <div
    use:melt={$tooltipContent}
    class="z-[100] rounded-md bg-[#253858] px-2 py-1 text-xs text-white shadow-lg"
  >
    {content}
  </div>
{/if}