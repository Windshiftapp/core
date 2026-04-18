<script>
  import { cn } from '../utils/cn.js';

  /**
   * GlassButton - glassmorphism-styled button for portal/hub heroes.
   *
   * Uses the .glass-btn utility (defined in design-system/animations.css)
   * for the backdrop-filter + translucent background. Matches Button's
   * prop conventions (variant/size/icon/href/onclick).
   *
   * @example default rect
   *   <GlassButton icon={Inbox} onclick={openInbox}>Inbox</GlassButton>
   *
   * @example round icon-only
   *   <GlassButton variant="round" icon={Settings} title="Settings" onclick={...} />
   *
   * @example large CTA
   *   <GlassButton variant="cta" onclick={signIn}>Sign in</GlassButton>
   */
  let {
    variant = 'default', // 'default' | 'round' | 'cta'
    size = 'medium',     // 'small' | 'medium' (only applies to 'default' variant)
    icon = null,
    iconSize = null,
    href = null,
    type = 'button',
    title = null,
    id = undefined,
    disabled = false,
    onclick = null,
    class: className = '',
    children = undefined
  } = $props();
  export { className as class };

  const defaultSizeClasses = $derived({
    small: 'flex items-center gap-2 px-3 py-1.5 rounded text-white text-sm transition-all',
    medium: 'flex items-center gap-2 px-3 py-2 rounded text-white text-sm transition-all shadow-lg'
  }[size] || 'flex items-center gap-2 px-3 py-2 rounded text-white text-sm transition-all shadow-lg');

  const variantClasses = $derived({
    default: defaultSizeClasses,
    round: 'w-10 h-10 rounded-full flex items-center justify-center text-white transition-all shadow-lg',
    cta: 'px-8 py-3 rounded-xl font-semibold text-lg text-white transition-all duration-200 shadow-xl hover:shadow-2xl hover:scale-[1.02] active:scale-[0.98]'
  }[variant]);

  const allClasses = $derived(cn('glass-btn', variantClasses, className));

  const resolvedIconSize = $derived(iconSize || (variant === 'round' ? 'w-5 h-5' : 'w-4 h-4'));
</script>

{#snippet content()}
  {#if icon}
    {@const Icon = icon}
    <Icon class={resolvedIconSize} />
  {/if}
  {#if children}{@render children()}{/if}
{/snippet}

{#if href}
  <a {id} {href} {title} class={allClasses} onclick={(e) => onclick?.(e)}>
    {@render content()}
  </a>
{:else}
  <button {id} {type} {title} {disabled} class={allClasses} onclick={(e) => onclick?.(e)}>
    {@render content()}
  </button>
{/if}
