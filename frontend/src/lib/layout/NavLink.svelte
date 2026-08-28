<script>
  import Tooltip from '../components/Tooltip.svelte';

  let {
    icon: Icon,
    label,
    href = null,
    onclick = null,
    isActive = false,
    expanded = false,
    variant = 'default',
    tooltipSuffix = '',
    id = undefined
  } = $props();

  const baseClasses = $derived(
    `${expanded ? 'w-full px-3' : 'w-10'} h-10 rounded flex items-center ${expanded ? '' : 'justify-center'} cursor-pointer`
  );

  const variantClasses = $derived(
    variant === 'primary'
      ? 'bg-[var(--ds-interactive)] bg-primary text-white text-sm font-medium transition'
      : `nav-button ${isActive ? 'nav-button-selected' : ''}`
  );
</script>

<Tooltip content="{label}{tooltipSuffix}" placement="right" disabled={expanded}>
  {#if href}
    <a
      {id}
      {href}
      class="{baseClasses} {variantClasses}"
      aria-label={label}
      aria-current={isActive ? 'page' : undefined}
    >
      <Icon class="w-5 h-5 flex-shrink-0" />
      {#if expanded}<span class="ml-3 min-w-0 flex-1 overflow-hidden text-ellipsis whitespace-nowrap text-sm">{label}</span>{/if}
    </a>
  {:else}
    <button
      {id}
      {onclick}
      class="{baseClasses} {variantClasses}"
      aria-label={label}
    >
      <Icon class="w-5 h-5 flex-shrink-0" />
      {#if expanded}<span class="ml-3 min-w-0 flex-1 overflow-hidden text-ellipsis whitespace-nowrap">{label}</span>{/if}
    </button>
  {/if}
</Tooltip>
