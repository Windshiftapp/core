<script>
  import { tick } from 'svelte';
  import { portal as portalAction } from '../actions/portal.js';
  import { createOverlayScope, mountOverlay, overlayOwnsFocus, overlayHasFocusedControl, isFocusEligible, tabStops } from '../utils/overlayScope.js';

  let {
    show = $bindable(false),
    opacity = 0.5,
    blur = 2,
    extraFilter = '',
    zIndex = 50,
    portal = false,
    align = 'center',
    paddingTop = '',
    scrollable = false,
    closeOnClick = true,
    closeOnEscape = true,
    ariaLabelledBy = undefined,
    ariaLabel = undefined,
    initialFocus = /** @type {string | false} */ ('[data-autofocus], [autofocus]'),
    focusOnOpen = true,
    backdropElement = $bindable(null),
    class: className = '',
    dataTestid = undefined,
    id = undefined,
    onBackdropClick = undefined,
    onKeydown = undefined,
    onclose = undefined,
    children = undefined,
  } = $props();

  const scope = createOverlayScope();
  const instanceId = $props.id();
  const titleId = `${instanceId}-title`;
  let headingId = $state(undefined);
  let validExplicitId = $state(undefined);
  const bgStyle = $derived(`rgba(0, 0, 0, ${opacity})`);
  const filterStyle = $derived([blur > 0 ? `blur(${blur}px)` : '', extraFilter].filter(Boolean).join(' ') || 'none');
  const layoutClasses = $derived(align === 'center' ? 'flex items-center justify-center p-4'
    : align === 'top' ? `flex items-start justify-center ${paddingTop}${scrollable ? ' overflow-y-auto' : ''}` : '');

  function resolveName(root) {
    const explicit = ariaLabelledBy?.split(/\s+/).filter(Boolean);
    validExplicitId = explicit?.every((id) => document.getElementById(id)) ? ariaLabelledBy : undefined;
    const heading = [...scope.headings, ...root.querySelectorAll('h1, h2, h3, [data-dialog-title]')]
      .find((element) => element.closest('[role="dialog"]') === root && element.textContent.trim());
    if (heading && !heading.id) heading.id = titleId;
    headingId = heading?.id;
  }

  $effect(() => {
    if (backdropElement) resolveName(backdropElement);
  });

  function manageOverlay(root) {
    const teardown = mountOverlay(scope, root, handleKeydown);
    let mounted = true;
    resolveName(root);
    const names = new MutationObserver(() => resolveName(root));
    names.observe(root, { childList: true, subtree: true, characterData: true });
    void tick().then(() => {
      if (!mounted || !focusOnOpen || !overlayOwnsFocus(root) || overlayHasFocusedControl(root)) return;
      const preferred = typeof initialFocus === 'string' ? root.querySelector(initialFocus) : null;
      const target = initialFocus === false ? root : isFocusEligible(preferred) ? preferred : tabStops(root)[0] || root;
      target.focus({ preventScroll: true });
    });
    return { destroy() { mounted = false; names.disconnect(); teardown(); } };
  }

  function handleClick(event) {
    if (!overlayOwnsFocus(backdropElement) || event.target !== event.currentTarget) return;
    onBackdropClick?.(event);
    // Editing dialogs opt out of outside-click dismissal to avoid losing input.
    if (show && closeOnClick && !event.defaultPrevented) close();
  }

  function handleKeydown(event) {
    onKeydown?.(event);
    if (show && !event.defaultPrevented && closeOnEscape && event.key === 'Escape') {
      event.preventDefault();
      close();
    }
  }

  function close() {
    if (!show) return;
    show = false;
    onclose?.();
  }
</script>

{#if show}
  <!-- Keyboard dispatch is shared across this root and its owned portals. -->
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div
    use:portalAction={portal ? document.body : null}
    bind:this={backdropElement}
    use:manageOverlay
    {id}
    class="fixed inset-0 {layoutClasses} focus:outline-none {className}"
    style:z-index={zIndex}
    style:background-color={bgStyle}
    style:backdrop-filter={filterStyle}
    onclick={handleClick}
    role="dialog"
    aria-modal="true"
    aria-labelledby={validExplicitId || (!ariaLabel ? headingId : undefined)}
    aria-label={!validExplicitId ? ariaLabel : undefined}
    data-testid={dataTestid}
    tabindex="-1"
  >
    {@render children?.()}
  </div>
{/if}
