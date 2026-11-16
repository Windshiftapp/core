<script>
    import { install, uninstall } from '@github/hotkey';
    import { useEventListener } from 'runed';
    import Tooltip from "./Tooltip.svelte";
    import { cn } from '../utils/cn.js';

  let {
    /** @type {'default'|'primary'|'secondary'|'context'|'danger'|'danger-ghost'|'selected'|'ghost'|'link'} */
    variant = 'default',
    /** @type {'small'|'medium'|'large'|'sm'} */
    size = 'medium',
    disabled = false,
    /** Toggle state for toggle buttons; renders aria-pressed when set. */
    pressed = undefined,
    icon = null,
    iconPosition = 'left',
    /** @type {"button" | "submit" | "reset"} */
    type = 'button',
    href = null,
    target = null,
    loading = false,
    fullWidth = false,
    keyboardHint = null,
    id = undefined,
    title = null,
    ariaLabel = undefined,
    ariaExpanded = undefined,
    ariaControls = undefined,
    onclick = null,
    hotkeyConfig = null,
    dataTestid = undefined,
    dataPageId = undefined,
    style = '',
    class: className = '',
    children = undefined
  } = $props();
  export { className as class };

  const buttonType = $derived(/** @type {"button"|"submit"|"reset"} */ (type));
  const effectiveDisabled = $derived(disabled || loading);
  let hasTextLabel = $state(false);
  const accessibleName = $derived(ariaLabel ?? (!hasTextLabel ? title ?? undefined : undefined));

  let buttonEl = $state(null);
  $effect(() => {
    if (!buttonEl) return;
    const node = buttonEl;
    function updateLabel() {
      const walker = document.createTreeWalker(node, NodeFilter.SHOW_TEXT);
      let text;
      let labelled = false;
      while ((text = walker.nextNode())) {
        if (text.textContent.trim() && !text.parentElement.closest('svg, kbd, [aria-hidden="true"]')) {
          labelled = true;
          break;
        }
      }
      hasTextLabel = labelled;
    }
    updateLabel();
    const observer = new MutationObserver(updateLabel);
    observer.observe(node, { childList: true, characterData: true, subtree: true });
    return () => observer.disconnect();
  });

  function handleClick(event) {
    if (effectiveDisabled) {
      event.preventDefault();
      return;
    }
    onclick?.(event);
  }

  // Install/uninstall @github/hotkey on the button element
  $effect(() => {
    if (!buttonEl || !hotkeyConfig?.key) return;
    install(buttonEl, hotkeyConfig.key);
    return () => uninstall(buttonEl);
  });

  // Guard: cancel hotkey-fire when guard returns false or a modal is open above this button
  useEventListener(() => buttonEl, 'hotkey-fire', (e) => {
    const openModal = document.querySelector('[role="dialog"][aria-modal="true"]');
    if (openModal && !openModal.contains(buttonEl)) {
      e.preventDefault();
      return;
    }
    if (hotkeyConfig?.guard && !hotkeyConfig.guard()) e.preventDefault();
  });

  // Base styles
  const baseClasses = $derived(cn(
    'ds-focus-ring inline-flex items-center justify-center font-medium transition-colors duration-200 cursor-pointer border border-transparent',
    'disabled:opacity-50 disabled:cursor-not-allowed disabled:pointer-events-none',
    fullWidth && 'w-full'
  ));

  // Normalize size aliases
  const normalizedSize = $derived(size === 'sm' ? 'small' : size);

  // Size variants
  const sizeClasses = $derived({
    small: 'px-3.5 py-1.5 text-sm rounded gap-2',
    medium: 'px-4 py-1.5 text-sm rounded gap-2',
    large: 'px-6 py-2.5 text-base rounded gap-2'
  }[normalizedSize]);

  // Color variants using Tailwind arbitrary values with CSS variables
  // Using design system tokens for dark mode support
  const variantClasses = $derived({
    primary: 'bg-[var(--ds-background-brand-bold)] hover:bg-[var(--ds-background-brand-bold-hovered)] active:bg-[var(--ds-background-brand-bold-pressed)] text-[var(--ds-text-on-interactive)] hover:text-[var(--ds-text-on-interactive-hovered)] active:text-[var(--ds-text-on-interactive-pressed)]',
    default: 'bg-[var(--ds-surface-raised,white)] hover:bg-[var(--ds-background-neutral-hovered,#f9fafb)] focus:ring-2 focus:ring-ds-border-bold border border-[var(--ds-border,#d1d5db)] text-[var(--ds-text,#111827)]',
    secondary: 'bg-[var(--ds-background-neutral,#f3f4f6)] hover:bg-[var(--ds-background-neutral-hovered,#e5e7eb)] focus:ring-2 focus:ring-ds-border-bold text-[var(--ds-text,#374151)] border border-[var(--ds-border,#e5e7eb)]',
    context: 'bg-[var(--ctx-surface,var(--ds-background-neutral))] hover:bg-[var(--ctx-hover-bg,var(--ds-surface-hovered))] focus:ring-2 focus:ring-[var(--ctx-border-focused,var(--ds-border-focused))] text-[var(--ds-text)]',
    danger: 'bg-[var(--ds-background-danger-bold)] hover:bg-[var(--ds-background-danger-bold-hovered)] text-[var(--ds-text-on-danger)]',
    'danger-ghost': 'bg-transparent text-[var(--ds-text-danger)] hover:bg-[var(--ds-background-danger-bold)] hover:text-[var(--ds-text-on-danger)]',
    selected: 'bg-[var(--ds-background-brand-bold-pressed)] hover:bg-[var(--ds-background-brand-bold-hovered)] text-[var(--ds-text-on-interactive-pressed)] hover:text-[var(--ds-text-on-interactive-hovered)]',
    ghost: 'bg-transparent hover:bg-[var(--ctx-active-bg,var(--ds-background-neutral-hovered,#f3f4f6))] focus:ring-2 focus:ring-ds-border-bold text-[var(--ctx-text,var(--ds-text,#374151))]',
    link: 'bg-transparent hover:underline focus-visible:underline text-[var(--ds-text-link)] hover:text-[var(--ds-text-link-hovered)] border-none p-0'
  }[variant]);

  // Combine all classes
  const allClasses = $derived(cn(baseClasses, sizeClasses, variantClasses, className));

  // Icon size based on button size
  const iconSize = $derived({
    small: 'w-4 h-4',
    medium: 'w-4 h-4',
    large: 'w-5 h-5'
  }[normalizedSize]);

  // Keyboard hint styling based on variant using CSS variables
  const kbdClasses = $derived({
    primary: 'border-[var(--ds-border-focused)] text-current',
    default: 'bg-[var(--ds-background-neutral)] border-[var(--ds-border)] text-[var(--ds-text)]',
    secondary: 'bg-[var(--ds-background-neutral)] border-[var(--ds-border)] text-[var(--ds-text)]',
    context: 'bg-[var(--ds-background-neutral)] border-[var(--ctx-border,var(--ds-border))] text-[var(--ds-text)]',
    danger: 'bg-[var(--ds-background-danger-bold-hovered)] border-[var(--ds-danger)] text-[var(--ds-text-on-danger)]',
    'danger-ghost': 'bg-[var(--ds-background-danger-bold-hovered)] border-[var(--ds-danger)] text-[var(--ds-text-on-danger)]',
    selected: 'border-[var(--ds-border)] text-current',
    ghost: 'bg-[var(--ds-background-neutral)] border-[var(--ds-border)] text-[var(--ds-text-subtle)]',
    link: 'bg-[var(--ds-background-neutral)] border-[var(--ds-border)] text-[var(--ds-text-subtle)]'
  }[variant]);
</script>

<!-- Snippet for button inner content -->
{#snippet buttonContent()}
  {#if loading}
    <svg class="animate-spin -ml-1 mr-2 {iconSize} text-current" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
      <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
      <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
    </svg>
  {:else if icon && iconPosition === 'left'}
    {@const Icon = icon}
    <Icon class={iconSize} />
  {/if}

  {#if children}{@render children()}{/if}

  {#if keyboardHint}
    <kbd class="ml-auto px-1.5 text-xs rounded border {kbdClasses}">
      {keyboardHint}
    </kbd>
  {/if}

  {#if !loading && icon && iconPosition === 'right'}
    {@const Icon = icon}
    <Icon class={iconSize} />
  {/if}
{/snippet}

<!-- Snippet for link inner content (no loading state) -->
{#snippet linkContent()}
  {#if icon && iconPosition === 'left'}
    {@const Icon = icon}
    <Icon class={iconSize} />
  {/if}

  {#if children}{@render children()}{/if}

  {#if keyboardHint}
    <kbd class="ml-auto px-1.5 text-xs rounded border {kbdClasses}">
      {keyboardHint}
    </kbd>
  {/if}

  {#if icon && iconPosition === 'right'}
    {@const Icon = icon}
    <Icon class={iconSize} />
  {/if}
{/snippet}

<!-- Snippet for the button/link element -->
{#snippet buttonElement()}
  {#if href}
    <a bind:this={buttonEl} {id} {href} {target} {style} aria-label={accessibleName} aria-disabled={effectiveDisabled} tabindex={effectiveDisabled ? -1 : undefined} data-testid={dataTestid} data-page-id={dataPageId} class={allClasses} onclick={handleClick}>
      {@render linkContent()}
    </a>
  {:else}
    <button bind:this={buttonEl} {id} type={buttonType} disabled={effectiveDisabled} aria-label={accessibleName} aria-expanded={ariaExpanded} aria-controls={ariaControls} aria-busy={loading} aria-pressed={pressed} {style} data-testid={dataTestid} data-page-id={dataPageId} class={allClasses} onclick={handleClick}>
      {@render buttonContent()}
    </button>
  {/if}
{/snippet}

<!-- Main render: wrap with tooltip if title is provided -->
{#if title}
  <Tooltip content={title} triggerElement={buttonEl}>
    {@render buttonElement()}
  </Tooltip>
{:else}
  {@render buttonElement()}
{/if}
