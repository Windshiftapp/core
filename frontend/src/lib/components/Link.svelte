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
    class: className = '',
    element: anchorElement = $bindable(undefined),
    children
  } = $props();


</script>

<a
  bind:this={anchorElement}
  {href}
  onclick={onClick}
  {onmouseenter}
  {onmouseleave}
  class={className}
  {style}
  aria-current={active ? 'page' : undefined}
  aria-disabled={disabled}
  tabindex={disabled ? -1 : 0}
>
  {@render children?.()}
</a>
