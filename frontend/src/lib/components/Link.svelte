<script>

  /**
   * A proper link component that uses <a> tags for semantic HTML
   * while maintaining SPA navigation behavior.
   *
   * Supports:
   * - Right-click "Open in New Tab"
   * - Ctrl/Cmd+click to open in new tab
   * - URL preview in browser status bar
   * - Proper accessibility and keyboard navigation
   * - Optional onClick for modal/custom behavior while preserving link benefits
   */

  let {
    href = '',
    active = false,
    disabled = false,
    onClick = null,
    style = '',
    onmouseenter = null,
    onmouseleave = null,
    target = undefined,
    rel = undefined,
    class: className = '',
    element: anchorElement = $bindable(undefined),
    children,
    ...rest
  } = $props();

  // Modifier-key filter: let the browser handle new-tab / new-window / download
  // variants natively (cmd+click on mac, ctrl+click elsewhere, shift, alt,
  // middle-click) by skipping the custom onClick entirely. Also bails if the
  // anchor has an explicit target (e.g. _blank), or if something upstream has
  // already prevented the default.
  function handleClick(event) {
    if (disabled) {
      event.preventDefault();
      return;
    }
    if (event.defaultPrevented) return;
    if (target && target !== '_self') return;
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    if (event.button !== undefined && event.button !== 0) return;
    onClick?.(event);
  }
</script>

<a
  bind:this={anchorElement}
  {href}
  {target}
  {rel}
  onclick={handleClick}
  {onmouseenter}
  {onmouseleave}
  class={className}
  {style}
  aria-current={active ? 'page' : undefined}
  aria-disabled={disabled}
  tabindex={disabled ? -1 : 0}
  {...rest}
>
  {@render children?.()}
</a>
