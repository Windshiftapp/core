<script>
  import X from '@lucide/svelte/icons/x';
  import { t } from '../stores/i18n.svelte.js';

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
    appearance = 'soft',  // 'soft' | 'metadata'
    dotColor = null,
    removable = false,
    onRemove = null,
    icon: Icon = null,
    title = undefined,
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

  const appearanceClasses = $derived(
    appearance === 'metadata'
      ? 'max-w-full gap-1.5 rounded-[3px] px-1.5 py-0.5 text-[11px] leading-4'
      : 'gap-1.5 rounded-full px-2 py-1 text-xs',
  );

  const appearanceStyles = $derived(
    appearance === 'metadata'
      ? 'background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);'
      : colorStyles,
  );

  function handleRemove(e) {
    e.stopPropagation();
    onRemove?.();
  }
</script>

<span
  class="inline-flex items-center font-medium {appearanceClasses} {className}"
  style={appearanceStyles}
  {title}
>
  {#if dotColor}
    <span class="h-1.5 w-1.5 shrink-0 rounded-full" style="background-color: {dotColor};" aria-hidden="true"></span>
  {/if}
  {#if Icon}
    <Icon class="h-3 w-3 shrink-0" />
  {/if}
  {@render children?.()}
  {#if removable && onRemove}
    <button
      type="button"
      onclick={handleRemove}
      class="hover:opacity-70 transition-opacity -mr-0.5"
      aria-label={t('common.remove')}
    >
      <X class="w-3 h-3" />
    </button>
  {/if}
</span>
