<script>
  import Tooltip from '../components/Tooltip.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let {
    icon: Icon,
    label,
    href = null,
    onclick = null,
    isActive = false,
    expanded = false,
    variant = 'default',
    tooltipSuffix = '',
    shortcut = '',
    id = undefined
  } = $props();

  const baseClasses = $derived(
    'w-full px-3 h-10 rounded flex items-center justify-start text-left cursor-pointer'
  );

  const variantClasses = $derived(
    variant === 'primary'
      ? 'bg-[var(--ds-interactive)] bg-primary text-white text-sm font-medium transition'
      : variant === 'accent'
        ? `nav-button nav-button-accent ${isActive ? 'nav-button-selected' : ''}`
      : `nav-button ${isActive ? 'nav-button-selected' : ''}`
  );

  const shortcutKeys = $derived(shortcut.trim().split(/\s+/).filter(Boolean));
</script>

<Tooltip content="{label}{tooltipSuffix}" placement="right" disabled={expanded}>
  {#if href}
    <a
      {id}
      {href}
      {onclick}
      class="{baseClasses} {variantClasses}"
      aria-label={label}
      aria-current={isActive ? 'page' : undefined}
    >
      <Icon class="w-5 h-5 flex-shrink-0" />
      {#if expanded}<span class="ml-3 min-w-0 flex-1 overflow-hidden text-ellipsis whitespace-nowrap text-sm">{label}</span>{/if}
      {#if expanded && shortcut}
        <span
          class="nav-shortcut"
          title={t('commandPalette.pressToOpen', { shortcut })}
          aria-label={t('commandPalette.pressToOpen', { shortcut })}
        >
          {#each shortcutKeys as key, index (`${key}-${index}`)}
            <kbd class="nav-shortcut-key">{key}</kbd>
          {/each}
        </span>
      {/if}
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
      {#if expanded && shortcut}
        <span
          class="nav-shortcut"
          title={t('commandPalette.pressToOpen', { shortcut })}
          aria-label={t('commandPalette.pressToOpen', { shortcut })}
        >
          {#each shortcutKeys as key, index (`${key}-${index}`)}
            <kbd class="nav-shortcut-key">{key}</kbd>
          {/each}
        </span>
      {/if}
    </button>
  {/if}
</Tooltip>
