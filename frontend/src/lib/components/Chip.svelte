<script>
  import X from '@lucide/svelte/icons/x';
  import { t } from '../stores/i18n.svelte.js';
  import { tick } from 'svelte';
  import { isFocusEligible, tabStops } from '../utils/overlayScope.js';

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
    /** Full localized name for rich/non-text chip content. */
    removeLabel = undefined,
    removeTestid = undefined,
    icon: Icon = null,
    title = undefined,
    class: className = '',
    children
  } = $props();
  const instanceId = $props.id();

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
      ? 'max-w-full gap-1.5 rounded-[3px] px-1.5 py-0.5 ds-text-metadata'
      : 'gap-1.5 rounded-full px-2 py-1 ds-text-metadata',
  );

  const appearanceStyles = $derived(
    appearance === 'metadata'
      ? 'background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);'
      : colorStyles,
  );

  async function handleRemove(e) {
    e.stopPropagation();
    const button = e.currentTarget;
    const hadFocus = document.activeElement === button;
    const scope = button.closest('[role="dialog"]') ?? document;
    const controls = tabStops(scope);
    const index = controls.indexOf(button);
    await onRemove?.();
    await tick();
    if (hadFocus && !button.isConnected && document.activeElement === document.body) {
      const candidates = [...controls.slice(index + 1), ...controls.slice(0, index).reverse()];
      const target = candidates.find(node => node.tabIndex >= 0 && isFocusEligible(node));
      target?.focus({ preventScroll: true });
    }
  }
</script>

<span
  class="inline-flex items-center font-medium {appearanceClasses} {className}"
  style={appearanceStyles}
  dir="auto"
  {title}
>
  {#if dotColor}
    <span class="h-1.5 w-1.5 shrink-0 rounded-full" style="background-color: {dotColor};" aria-hidden="true"></span>
  {/if}
  {#if Icon}
    <Icon class="h-3 w-3 shrink-0" />
  {/if}
  <span id={`${instanceId}-label`} class="min-w-0">{@render children?.()}</span>
  {#if removable && onRemove}
    <button
      type="button"
      data-testid={removeTestid}
      onclick={handleRemove}
      class="ds-focus-ring inline-flex shrink-0 items-center justify-center min-w-6 min-h-6 hover:opacity-70 transition-opacity rounded"
      aria-label={removeLabel}
      aria-labelledby={removeLabel ? undefined : `${instanceId}-remove ${instanceId}-label`}
    >
      <span id={`${instanceId}-remove`} class="sr-only">{t('common.remove')}</span>
      <X class="w-3 h-3" />
    </button>
  {/if}
</span>
