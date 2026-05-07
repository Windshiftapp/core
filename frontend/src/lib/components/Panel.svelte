<script>
  import { cn } from '../utils/cn.js';

  let {
    padding = 'default',   // 'none', 'compact', 'default', 'spacious', 'loose', 'generous'
    rounded = 'lg',        // 'none', 'sm', 'md', 'lg'
    hoverable = false,     // border-color brightens on hover (action-list pattern)
    interactive = false,   // adds cursor-pointer + hover bg
    href = null,
    onclick = null,
    class: className = '',
    style: userStyle = '',
    children
  } = $props();

  const paddingClasses = {
    none: '',
    compact: 'p-3',
    default: 'p-4',
    spacious: 'p-6',
    loose: 'p-8',
    generous: 'p-12'
  };

  const roundedClasses = {
    none: '',
    sm: 'rounded-sm',
    md: 'rounded-md',
    lg: 'rounded-lg'
  };

  const baseClasses = $derived(cn(
    'panel border',
    roundedClasses[rounded],
    paddingClasses[padding],
    hoverable && 'panel-hoverable',
    interactive && 'panel-interactive cursor-pointer',
    className
  ));

  const computedStyle = $derived([
    'background-color: var(--ds-surface-raised); border-color: var(--ds-border);',
    userStyle
  ].filter(Boolean).join(' '));
</script>

{#if href}
  <a {href} class={baseClasses} style={computedStyle} {onclick}>
    {@render children?.()}
  </a>
{:else if onclick}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class={baseClasses} style={computedStyle} {onclick}>
    {@render children?.()}
  </div>
{:else}
  <div class={baseClasses} style={computedStyle}>
    {@render children?.()}
  </div>
{/if}

<style>
  .panel-hoverable {
    transition: border-color 140ms ease-in-out;
  }
  .panel-hoverable:hover {
    border-color: var(--ds-border-bold) !important;
  }
  .panel-interactive {
    transition: border-color 140ms ease-in-out, background-color 140ms ease-in-out;
  }
  .panel-interactive:hover {
    border-color: var(--ds-border-bold) !important;
    background-color: var(--ds-surface-raised-hovered) !important;
  }
</style>
