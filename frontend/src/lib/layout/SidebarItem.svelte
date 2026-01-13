<script>
  let {
    label,
    icon = null,
    href = null,
    isActive = false,
    disabled = false,
    badge = null,
    onclick = null,
    class: className = ''
  } = $props();

  const baseClasses = 'flex items-center px-3 py-2.5 text-sm font-medium transition-all';
  const disabledClasses = 'opacity-50 cursor-not-allowed pointer-events-none';

  // Active state: raised card with shadow
  // Inactive state: subtle icon, no background
  const activeStyle = 'background: var(--ds-surface-raised); color: var(--ds-text); border-radius: 8px; box-shadow: 0 1px 3px rgba(0,0,0,0.08);';
  const inactiveStyle = 'background: transparent; color: var(--ds-text-subtle); border-radius: 8px;';
  const hoverStyle = 'background: var(--ds-background-neutral-hovered); color: var(--ds-text); border-radius: 8px;';
</script>

{#if href && !disabled}
  <a
    {href}
    class="{baseClasses} {className}"
    style={isActive ? activeStyle : inactiveStyle}
    onmouseenter={(e) => { if (!isActive) e.currentTarget.style.cssText = hoverStyle; }}
    onmouseleave={(e) => { if (!isActive) e.currentTarget.style.cssText = inactiveStyle; }}
    onclick={onclick}
  >
    {#if icon}
      <svelte:component this={icon} class="flex-shrink-0 mr-3 w-5 h-5" style="color: {isActive ? 'var(--ds-interactive)' : 'var(--ds-icon-subtle)'};" />
    {/if}
    <span class="truncate flex-1">{label}</span>
    {#if badge}
      <span class="ml-2 px-2 py-0.5 text-xs font-medium rounded-full" style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);">
        {badge}
      </span>
    {/if}
  </a>
{:else}
  <button
    type="button"
    class="{baseClasses} w-full {disabled ? disabledClasses : ''} {className}"
    style={isActive ? activeStyle : inactiveStyle}
    onmouseenter={(e) => { if (!isActive && !disabled) e.currentTarget.style.cssText = hoverStyle; }}
    onmouseleave={(e) => { if (!isActive && !disabled) e.currentTarget.style.cssText = inactiveStyle; }}
    onclick={onclick}
    {disabled}
  >
    {#if icon}
      <svelte:component this={icon} class="flex-shrink-0 mr-3 w-5 h-5" style="color: {isActive ? 'var(--ds-interactive)' : 'var(--ds-icon-subtle)'};" />
    {/if}
    <span class="truncate flex-1 text-left">{label}</span>
    {#if badge}
      <span class="ml-2 px-2 py-0.5 text-xs font-medium rounded-full" style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);">
        {badge}
      </span>
    {/if}
  </button>
{/if}
