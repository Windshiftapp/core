<script>
  import { X } from 'lucide-svelte';

  /**
   * Chip - Metadata container with optional icon and remove button
   *
   * Use for tags, labels, filters, and removable metadata items.
   *
   * @example
   * <Chip color="blue">Frontend</Chip>
   * <Chip color="green" icon={Tag}>Label</Chip>
   * <Chip removable onRemove={() => handleRemove()}>Removable</Chip>
   */
  let {
    color = 'blue',       // 'blue' | 'green' | 'purple' | 'teal' | 'gray' | 'red' | 'yellow' | 'orange'
    removable = false,
    onRemove = null,
    icon = null,
    class: className = '',
    children
  } = $props();

  const colorStyles = $derived({
    blue: 'background-color: var(--ds-accent-blue-subtle); color: var(--ds-text-accent-blue);',
    green: 'background-color: var(--ds-accent-green-subtle); color: var(--ds-text-accent-green);',
    purple: 'background-color: var(--ds-accent-purple-subtle); color: var(--ds-text-accent-purple);',
    teal: 'background-color: var(--ds-accent-teal-subtle); color: var(--ds-text-accent-teal);',
    gray: 'background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);',
    red: 'background-color: var(--ds-accent-red-subtle); color: var(--ds-text-danger);',
    yellow: 'background-color: var(--ds-accent-yellow-subtle); color: var(--ds-text-accent-yellow);',
    orange: 'background-color: var(--ds-accent-orange-subtle); color: var(--ds-text-accent-orange);'
  }[color] || 'background-color: var(--ds-accent-blue-subtle); color: var(--ds-text-accent-blue);');

  function handleRemove(e) {
    e.stopPropagation();
    onRemove?.();
  }
</script>

<span
  class="inline-flex items-center gap-1.5 px-2 py-1 rounded-full text-xs font-medium {className}"
  style={colorStyles}
>
  {#if icon}
    <svelte:component this={icon} class="w-3 h-3" />
  {/if}
  {@render children?.()}
  {#if removable && onRemove}
    <button
      type="button"
      onclick={handleRemove}
      class="hover:opacity-70 transition-opacity -mr-0.5"
      aria-label="Remove"
    >
      <X class="w-3 h-3" />
    </button>
  {/if}
</span>
