<script>
  import { createOwnedPopup } from '../utils/overlayScope.js';
  const ownedPopup = createOwnedPopup();
  import { createTooltip, melt } from '@melt-ui/svelte';
  import { get } from 'svelte/store';
  import { descriptionIds } from '../utils/fieldContext.svelte.js';

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
    disabled = false,
    /** Supply the actual trigger when the snippet contains several controls. */
    triggerElement = null
  } = $props();

  const {
    elements: { trigger, content: tooltipContent },
    states: { open },
    ids
  } = createTooltip({
    // svelte-ignore state_referenced_locally
    positioning: {
      placement: /** @type {any} */ (placement)
    },
    // svelte-ignore state_referenced_locally
    openDelay: delay.open,
    // svelte-ignore state_referenced_locally
    closeDelay: delay.close,
    disableHoverableContent: false,
    forceVisible: true
  });
  let wrapper = $state(null);
  let activeTrigger = $state(null);
  $effect(() => {
    if (disabled || !wrapper) return;
    const node = triggerElement ?? wrapper.querySelector('button, a[href], input, select, textarea, [tabindex]:not([tabindex="-1"])') ?? wrapper;
    activeTrigger = node;
    const originalId = node.getAttribute('id');
    const originalDescription = node.getAttribute('aria-describedby');
    ids.trigger.set(originalId || get(ids.trigger));
    const unsubscribe = trigger.subscribe(attributes => {
      for (const [name, value] of Object.entries(attributes)) {
        if (name === 'aria-describedby') {
          const description = descriptionIds(originalDescription, get(open) ? value : undefined);
          if (description) node.setAttribute(name, description);
          else node.removeAttribute(name);
        } else node.setAttribute(name, String(value));
      }
    });
    const action = trigger(node);
    function handleEscape(event) {
      if (event.key !== 'Escape' || !get(open)) return;
      open.set(false);
      event.preventDefault();
      event.stopPropagation();
    }
    node.addEventListener('keydown', handleEscape, true);
    return () => {
      node.removeEventListener('keydown', handleEscape, true);
      action?.destroy?.();
      unsubscribe();
      for (const name of ['data-melt-tooltip-trigger', 'data-state']) node.removeAttribute(name);
      if (originalId) node.id = originalId;
      else node.removeAttribute('id');
      if (originalDescription) node.setAttribute('aria-describedby', originalDescription);
      else node.removeAttribute('aria-describedby');
    };
  });
</script>

{#if disabled}
  <span class="cursor-pointer {className}">
    {@render children()}
  </span>
{:else}
  <span bind:this={wrapper} class="cursor-pointer {className}">
    {@render children()}
  </span>

  {#if $open}
    <div
      use:melt={$tooltipContent}
      use:ownedPopup={{ trigger: () => activeTrigger, close: () => { $open = false; } }}
      data-testid="tooltip"
      class="z-[100] rounded-md shadow-lg bg-ds-surface-overlay text-ds-text border border-ds-border {contentClass}"
    >
      {#if tip}
        {@render tip()}
      {:else}
        {content}
      {/if}
    </div>
  {/if}
{/if}
