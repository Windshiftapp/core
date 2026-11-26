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
    /** Suppress the popover while keeping the same trigger node mounted (e.g. an
     *  open dropdown menu). Unlike `disabled`, this does not swap render branches. */
    suppressOpen = false,
    /** Attach hover listeners to the wrapper instead of the first focusable
     *  child. Needed when the child is disabled (pointer events suppressed). */
    triggerOnWrapper = false,
    /** Make the wrapper a focusable disclosure so keyboard and touch users can
     *  reach the hint (implies `triggerOnWrapper`). */
    wrapperFocusable = false,
    /** Toggle the popover on click/tap in addition to hover/focus. */
    toggleOnClick = false,
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
    // svelte-ignore state_referenced_locally
    closeOnPointerDown: !(toggleOnClick || wrapperFocusable),
    forceVisible: true
  });
  // The id melt generated at construction. Callers may override the trigger id
  // via their own element; keep this fallback so a wrapper trigger never reuses
  // the inner control's id.
  const generatedTriggerId = get(ids.trigger);
  let wrapper = $state(null);
  let activeTrigger = $state(null);
  $effect(() => {
    if (!suppressOpen) return;
    // Cancel a delayed open scheduled before suppression began: melt clears its
    // pending timer when the trigger receives pointerleave.
    activeTrigger?.dispatchEvent(new Event('pointerleave'));
    if ($open) open.set(false);
  });
  $effect(() => {
    if (disabled || !wrapper) return;
    const interactive = toggleOnClick || wrapperFocusable;
    const useWrapper = triggerOnWrapper || wrapperFocusable;
    const node = useWrapper
      ? wrapper
      : (triggerElement ?? wrapper.querySelector('button, a[href], input, select, textarea, [tabindex]:not([tabindex="-1"])') ?? wrapper);
    activeTrigger = node;
    const originalId = node.getAttribute('id');
    const originalDescription = node.getAttribute('aria-describedby');
    // A wrapper trigger owns a generated id; the inner control keeps its own.
    ids.trigger.set(useWrapper ? generatedTriggerId : originalId || generatedTriggerId);
    // While suppressed, keep the node mounted but detach all tooltip listeners
    // so a hover cannot reopen the popover behind an open menu.
    if (suppressOpen) {
      open.set(false);
      return;
    }
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
    function handleToggle(event) {
      event?.stopPropagation?.();
      open.set(!get(open));
    }
    function handleActivate(event) {
      if (event.key !== 'Enter' && event.key !== ' ') return;
      event.preventDefault();
      event.stopPropagation();
      open.set(!get(open));
    }
    node.addEventListener('keydown', handleEscape, true);
    if (interactive) {
      node.addEventListener('click', handleToggle);
      node.addEventListener('keydown', handleActivate, true);
    }
    return () => {
      node.removeEventListener('keydown', handleEscape, true);
      if (interactive) {
        node.removeEventListener('click', handleToggle);
        node.removeEventListener('keydown', handleActivate, true);
      }
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
  {#if wrapperFocusable}
    <span
      bind:this={wrapper}
      class="cursor-pointer {className}"
      tabindex="0"
      role="button"
      aria-label={typeof content === 'string' ? content : undefined}
    >
      {@render children()}
    </span>
  {:else}
    <span bind:this={wrapper} class="cursor-pointer {className}">
      {@render children()}
    </span>
  {/if}

  {#if $open && !suppressOpen}
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
