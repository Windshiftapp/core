<script>
    import Tooltip from "./Tooltip.svelte";

  let {
    variant = 'default', // 'primary', 'default', 'secondary', 'danger', 'selected', 'ghost', 'link'
    size = 'medium', // 'small', 'sm', 'medium', 'large'
    disabled = false,
    icon = null, // Lucide icon component
    iconPosition = 'left', // 'left', 'right'
    type = 'button', // 'button', 'submit', 'reset'
    href = null, // If provided, renders as link instead of button
    target = null, // For links
    loading = false,
    fullWidth = false,
    keyboardHint = null, // Keyboard shortcut hint (e.g., "C", "⌃L")
    title = null,
    onclick = null, // Svelte 5 style click handler
    class: className = ''
  } = $props();
  export { className as class };
  
  // Base styles
  const baseClasses = $derived([
    'inline-flex items-center justify-center font-medium transition-all duration-200 cursor-pointer',
    'focus:outline-none focus:ring-2 focus:ring-offset-2',
    'disabled:opacity-50 disabled:cursor-not-allowed disabled:pointer-events-none',
    fullWidth ? 'w-full' : '',
    className
  ].filter(Boolean).join(' '));
  
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
    primary: 'bg-[var(--ds-interactive)] hover:bg-[var(--ds-interactive-hovered)] focus:ring-[var(--ds-border-focused)] text-white border border-transparent',
    default: 'bg-[var(--ds-surface-raised,white)] hover:bg-[var(--ds-background-neutral-hovered,#f9fafb)] focus:ring-2 focus:ring-gray-500 focus:ring-opacity-50 border border-[var(--ds-border,#d1d5db)] text-[var(--ds-text,#111827)]',
    secondary: 'bg-[var(--ds-background-neutral,#f3f4f6)] hover:bg-[var(--ds-background-neutral-hovered,#e5e7eb)] focus:ring-2 focus:ring-gray-400 focus:ring-opacity-50 text-[var(--ds-text,#374151)] border border-[var(--ds-border,#e5e7eb)]',
    danger: 'bg-red-600 text-white hover:bg-red-700 focus:ring-red-500 border border-transparent',
    selected: 'bg-[var(--ds-interactive-pressed)] hover:bg-[var(--ds-interactive-hovered)] focus:ring-[var(--ds-border-focused)] text-white border border-transparent',
    ghost: 'bg-transparent hover:bg-[var(--ds-background-neutral-hovered,#f3f4f6)] focus:ring-2 focus:ring-gray-500 focus:ring-opacity-50 text-[var(--ds-text,#374151)]',
    link: 'bg-transparent hover:underline focus:ring-0 text-[var(--ds-text-link)] hover:text-[var(--ds-text-link-hovered)] border-none p-0'
  }[variant]);
  
  // No custom styles needed anymore
  
  // Combine all classes
  const allClasses = $derived(`${baseClasses} ${sizeClasses} ${variantClasses}`);
  
  // Icon size based on button size
  const iconSize = $derived({
    small: 'w-4 h-4',
    medium: 'w-4 h-4', 
    large: 'w-5 h-5'
  }[normalizedSize]);

  // Keyboard hint styling based on variant using CSS variables
  const kbdClasses = $derived({
    primary: 'bg-[var(--ds-interactive-hovered)] bg-opacity-50 border-[var(--ds-border-focused)] text-white',
    default: 'bg-[var(--ds-background-neutral)] border-[var(--ds-border)] text-[var(--ds-text)]',
    secondary: 'bg-[var(--ds-background-neutral)] border-[var(--ds-border)] text-[var(--ds-text)]',
    danger: 'bg-red-700 bg-opacity-50 border-red-500 text-white',
    selected: 'bg-[var(--ds-interactive-subtle)] border-[var(--ds-border)] text-[var(--ds-interactive-pressed)]',
    ghost: 'bg-[var(--ds-background-neutral)] border-[var(--ds-border)] text-[var(--ds-text-subtle)]',
    link: 'bg-[var(--ds-background-neutral)] border-[var(--ds-border)] text-[var(--ds-text-subtle)]'
  }[variant]);
</script>

{#if title}
<Tooltip content={title}>
  {#if href}
    <!-- Render as link -->
    <a
      {href}
      {target}
      class={allClasses}
      onclick={(e) => onclick?.(e)}
    >
      {#if icon && iconPosition === 'left'}
        <svelte:component this={icon} class={iconSize} />
      {/if}

      <slot />

      {#if keyboardHint}
        <kbd class="ml-auto px-1.5 text-xs rounded border {kbdClasses}">
          {keyboardHint}
        </kbd>
      {/if}

      {#if icon && iconPosition === 'right'}
        <svelte:component this={icon} class={iconSize} />
      {/if}
    </a>
  {:else}
    <!-- Render as button -->
    <button
      {type}
      {disabled}
      class={allClasses}
      onclick={(e) => onclick?.(e)}
    >
      {#if loading}
        <!-- Loading spinner -->
        <svg class="animate-spin -ml-1 mr-2 {iconSize} text-current" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
        </svg>
      {:else if icon && iconPosition === 'left'}
        <svelte:component this={icon} class={iconSize} />
      {/if}

      <slot />

      {#if keyboardHint}
        <kbd class="ml-auto px-1.5 text-xs rounded border {kbdClasses}">
          {keyboardHint}
        </kbd>
      {/if}

      {#if !loading && icon && iconPosition === 'right'}
        <svelte:component this={icon} class={iconSize} />
      {/if}
    </button>
  {/if}
</Tooltip>
{:else}
  {#if href}
    <!-- Render as link -->
    <a
      {href}
      {target}
      class={allClasses}
      onclick={(e) => onclick?.(e)}
    >
      {#if icon && iconPosition === 'left'}
        <svelte:component this={icon} class={iconSize} />
      {/if}

      <slot />

      {#if keyboardHint}
        <kbd class="ml-auto px-1.5 text-xs rounded border {kbdClasses}">
          {keyboardHint}
        </kbd>
      {/if}

      {#if icon && iconPosition === 'right'}
        <svelte:component this={icon} class={iconSize} />
      {/if}
    </a>
  {:else}
    <!-- Render as button -->
    <button
      {type}
      {disabled}
      class={allClasses}
      onclick={(e) => onclick?.(e)}
    >
      {#if loading}
        <!-- Loading spinner -->
        <svg class="animate-spin -ml-1 mr-2 {iconSize} text-current" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
        </svg>
      {:else if icon && iconPosition === 'left'}
        <svelte:component this={icon} class={iconSize} />
      {/if}

      <slot />

      {#if keyboardHint}
        <kbd class="ml-auto px-1.5 text-xs rounded border {kbdClasses}">
          {keyboardHint}
        </kbd>
      {/if}

      {#if !loading && icon && iconPosition === 'right'}
        <svelte:component this={icon} class={iconSize} />
      {/if}
    </button>
  {/if}
{/if}
