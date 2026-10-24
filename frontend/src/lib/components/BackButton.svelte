<script>
  import ArrowLeft from '@lucide/svelte/icons/arrow-left';
  import Link from './Link.svelte';
  import { cn } from '../utils/cn.js';

  /**
   * BackButton - Standard back navigation affordance.
   *
   * Renders a router-aware link when `href` is provided, otherwise a button
   * with `onclick`. Pass `label` for the inline text variant; omit it for an
   * icon-only control (supply `tooltip`/`label` for its accessible name).
   *
   * `tooltip` uses the native `title` attribute so the control adds no wrapper
   * element and stays a reliable click target.
   *
   * @example
   * <BackButton href="/agents" label="Back to agents" testid="agent-back" />
   * @example
   * <BackButton onclick={() => history.back()} tooltip="Back" size="small" />
   */
  let {
    label = '',
    href = null,
    onclick = null,
    icon = ArrowLeft,
    size = 'medium', // 'small' | 'medium'
    tooltip = null,
    disabled = false,
    testid = undefined,
    style = '',
    class: className = ''
  } = $props();

  const sizeClasses = $derived({
    small: 'gap-1.5 text-xs',
    medium: 'gap-2 text-sm'
  }[size]);

  const iconSize = $derived(size === 'small' ? 'w-3.5 h-3.5' : 'w-4 h-4');

  const classes = $derived(
    cn(
      'inline-flex items-center rounded-sm font-medium no-underline transition-colors',
      'text-[var(--ds-text-subtle)] hover:text-[var(--ds-text)]',
      'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--ds-border-focused)]',
      disabled && 'pointer-events-none opacity-50',
      sizeClasses,
      className
    )
  );

  const accessibleName = $derived(tooltip || label || 'Back');

  function handleClick(event) {
    if (disabled) {
      event.preventDefault();
      return;
    }
    onclick?.(event);
  }
</script>

{#snippet content()}
  {@const Icon = icon}
  <Icon class={iconSize} aria-hidden="true" />
  {#if label}<span>{label}</span>{/if}
{/snippet}

{#if href && !disabled}
  <Link
    {href}
    {style}
    class={classes}
    data-testid={testid}
    title={tooltip ?? undefined}
    aria-label={label ? undefined : accessibleName}
    onClick={handleClick}
  >
    {@render content()}
  </Link>
{:else}
  <button
    type="button"
    {style}
    class={classes}
    data-testid={testid}
    title={tooltip ?? undefined}
    aria-label={label ? undefined : accessibleName}
    {disabled}
    onclick={handleClick}
  >
    {@render content()}
  </button>
{/if}
