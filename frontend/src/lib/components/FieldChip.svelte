<script>
  import { createPopover, melt } from '@melt-ui/svelte';
  import { ChevronDown } from 'lucide-svelte';
  import { getVisibleColor } from '../utils/colorUtils.js';

  let {
    label = '',
    value = null,
    displayValue = '',
    icon = null,
    colorDot = null,
    placeholder = '',
    disabled = false,
    required = false,
    children = null,
    onOpen = null,
    onClose = null
  } = $props();

  const {
    elements: { trigger, content },
    states: { open }
  } = createPopover({
    positioning: {
      placement: 'bottom-start',
      gutter: 4,
      flip: true,
      shift: true
    },
    portal: 'body',
    forceVisible: true,
    onOpenChange: ({ next }) => {
      if (next && onOpen) {
        onOpen();
      } else if (!next && onClose) {
        onClose();
      }
      return next;
    }
  });

  // Expose open state for parent to control
  export function closePopover() {
    $open = false;
  }

  export function openPopover() {
    $open = true;
  }

  let showValue = $derived(displayValue || (value !== null && value !== undefined));
</script>

<!-- Chip Trigger Button -->
<button
  use:melt={$trigger}
  {disabled}
  class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-sm transition-colors"
  style="
    background-color: var(--ds-surface);
    border: 1px solid {required && !showValue ? 'var(--ds-border-danger, #ef4444)' : 'var(--ds-border)'};
    color: {showValue ? 'var(--ds-text)' : 'var(--ds-text-subtle)'};
    opacity: {disabled ? 0.5 : 1};
    cursor: {disabled ? 'not-allowed' : 'pointer'};
  "
  onmouseover={(e) => {
    if (!disabled) {
      e.currentTarget.style.backgroundColor = 'var(--ds-surface-hovered, var(--ds-background-neutral-hovered))';
    }
  }}
  onmouseout={(e) => {
    e.currentTarget.style.backgroundColor = 'var(--ds-surface)';
  }}
>
  {#if icon}
    <svelte:component this={icon} size={14} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
  {/if}
  {#if colorDot}
    <div class="w-2 h-2 rounded-full flex-shrink-0" style="background-color: {getVisibleColor(colorDot)};"></div>
  {/if}
  <span class="truncate max-w-[120px]">
    {displayValue || placeholder || label}
  </span>
  <ChevronDown size={12} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
</button>

<!-- Popover Content -->
{#if $open && children}
  <div
    use:melt={$content}
    class="z-50 rounded-lg shadow-lg overflow-hidden"
    style="
      background-color: var(--ds-surface-raised);
      border: 1px solid var(--ds-border);
      min-width: 200px;
      max-width: 320px;
    "
  >
    {@render children({ close: closePopover })}
  </div>
{/if}
