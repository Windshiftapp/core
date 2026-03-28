<script>
  import { createTooltip, melt } from '@melt-ui/svelte';

  let {
    content,
    children,
    /** @type {import('@floating-ui/dom').Placement} */
    placement = 'bottom',
    delay = { open: 300, close: 0 },
    class: className = '',
    disabled = false
  } = $props();

  const {
    elements: { trigger, content: tooltipContent },
    states: { open }
  } = createTooltip({
    // svelte-ignore state_referenced_locally
    positioning: {
      placement: /** @type {any} */ (placement)
    },
    // svelte-ignore state_referenced_locally
    openDelay: delay.open,
    // svelte-ignore state_referenced_locally
    closeDelay: delay.close,
    disableHoverableContent: true,
    forceVisible: true
  });
</script>

{#if disabled}
  <span class="cursor-pointer {className}">
    {@render children()}
  </span>
{:else}
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
{/if}