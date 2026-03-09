<script>
  import { t } from '../stores/i18n.svelte.js';

  let {
    src = null,          // Image URL
    alt = '',            // Alt text for image
    name = '',           // Full name for initials fallback
    firstName = '',      // First name (alternative to name)
    lastName = '',       // Last name (alternative to name)
    size = 'md',         // 'xs' (w-6), 'sm' (w-8), 'md' (w-10), 'lg' (w-12), 'xl' (w-16), '2xl' (w-20)
    variant = 'primary', // 'primary', 'neutral', 'blue', 'green', 'purple', 'teal', 'orange'
    rounded = 'full',    // 'full', 'lg', 'md', 'sm'
    ring = false,        // Add ring/border
    interactive = false, // Add hover effects
    onclick = null,      // Click handler
    class: className = ''
  } = $props();

  // Compute initials from name or firstName/lastName
  function getInitials() {
    // Use firstName/lastName if provided
    if (firstName || lastName) {
      return ((firstName?.[0] || '') + (lastName?.[0] || '')).toUpperCase();
    }
    // Fall back to parsing full name
    if (!name) return '??';
    const parts = name.trim().split(' ');
    if (parts.length >= 2) {
      return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
    }
    return name.substring(0, 2).toUpperCase();
  }

  // Size classes with text sizing
  const sizeClasses = {
    xs: 'w-6 h-6 text-[10px]',
    sm: 'w-8 h-8 text-xs',
    md: 'w-10 h-10 text-sm',
    lg: 'w-12 h-12 text-base',
    xl: 'w-16 h-16 text-lg',
    '2xl': 'w-20 h-20 text-xl'
  };

  // Variant styles using design tokens
  const variantStyles = {
    primary: 'background-color: var(--ds-interactive); color: var(--ds-text-inverse);',
    neutral: 'background-color: var(--ds-background-neutral); color: var(--ds-text);',
    blue: 'background-color: var(--ds-accent-blue-subtler); color: var(--ds-accent-blue);',
    green: 'background-color: var(--ds-accent-green-subtler); color: var(--ds-accent-green);',
    purple: 'background-color: var(--ds-accent-purple-subtler); color: var(--ds-accent-purple);',
    teal: 'background-color: var(--ds-accent-teal-subtler); color: var(--ds-accent-teal);',
    orange: 'background-color: var(--ds-accent-orange-subtler); color: var(--ds-accent-orange);'
  };

  // Rounded classes
  const roundedClasses = {
    full: 'rounded-full',
    lg: 'rounded-lg',
    md: 'rounded-md',
    sm: 'rounded-sm'
  };

  const initials = $derived(getInitials());
  const computedAlt = $derived(alt || name || `${firstName} ${lastName}`.trim() || t('components.avatar.defaultAlt'));

  const baseClasses = $derived([
    sizeClasses[size],
    roundedClasses[rounded],
    'flex items-center justify-center font-medium flex-shrink-0 overflow-hidden',
    ring ? 'ring-2 ring-offset-2 ring-[var(--ds-border)]' : '',
    interactive ? 'cursor-pointer hover:opacity-80 transition-opacity' : '',
    className
  ].filter(Boolean).join(' '));
</script>

{#if src}
  {#if onclick}
    <button type="button" class="p-0 border-0 bg-transparent cursor-pointer" onclick={onclick}>
      <img
        {src}
        alt={computedAlt}
        class="{baseClasses} object-cover"
      />
    </button>
  {:else}
    <img
      {src}
      alt={computedAlt}
      class="{baseClasses} object-cover"
    />
  {/if}
{:else}
  {#if onclick}
    <button type="button" class="appearance-none bg-transparent border-none p-0 m-0 font-[inherit] text-[inherit] cursor-pointer {baseClasses}" style={variantStyles[variant]} onclick={onclick}>
      {initials}
    </button>
  {:else}
    <div class={baseClasses} style={variantStyles[variant]}>
      {initials}
    </div>
  {/if}
{/if}
