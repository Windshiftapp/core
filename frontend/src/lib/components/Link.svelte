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

  export let href;
  export let active = false;
  export let disabled = false;
  export let onClick = null; // Optional click handler for modals or custom behavior
  export let style = ''; // Inline styles
  export let onmouseenter = null;
  export let onmouseleave = null;

  // Allow passing through additional classes
  let className = '';
  export { className as class };

  // Expose the anchor element reference for parent components
  let anchorElement;
  export { anchorElement as element };


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
  <slot />
</a>
