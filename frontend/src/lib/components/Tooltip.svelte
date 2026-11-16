<script>
  import { createOwnedPopup } from '../utils/overlayScope.js';
  const ownedPopup = createOwnedPopup();
  import { createTooltip, melt } from '@melt-ui/svelte';

  let {
    /** Plain-string tooltip body. Optional — provide this or `tip`. */
    content = undefined,
    children,
    /** Optional rich-content snippet rendered inside the popover instead of
     *  the plain `content` string (e.g. a list of linked items). */
    tip = undefined,
    /** @type {import('@floating-ui/dom').Placement} */
    placement = 'bottom',
    delay = { open: 300, close: 0 },
    /** Class applied to the trigger wrapper. */
    class: className = '',
    /** Extra classes appended to the popover (padding, max-width, etc.). */
    contentClass = 'px-2 py-1 text-xs',
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
      use:ownedPopup={{ close: () => { $open = false; } }}
      data-testid="tooltip"
      class="z-[100] rounded-md bg-[#253858] text-white shadow-lg {contentClass}"
    >
      {#if tip}
        {@render tip()}
      {:else}
        {content}
      {/if}
    </div>
  {/if}
{/if}
