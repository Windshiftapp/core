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
    tooltipSuffix = ''
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
      {href}
      class="{baseClasses} {variantClasses}"
      aria-current={isActive ? 'page' : undefined}
    >
      <Icon class="w-5 h-5 flex-shrink-0" />
      {#if expanded}<span class="ml-3 text-sm whitespace-nowrap">{label}</span>{/if}
    </a>
  {:else}
    <button
      {onclick}
      class="{baseClasses} {variantClasses}"
    >
      <Icon class="w-5 h-5 flex-shrink-0" />
      {#if expanded}<span class="ml-3 whitespace-nowrap">{label}</span>{/if}
    </button>
  {/if}
</Tooltip>
