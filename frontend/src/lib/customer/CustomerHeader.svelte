<script>
  import { onMount } from 'svelte';
  import Sun from '@lucide/svelte/icons/sun';
  import Moon from '@lucide/svelte/icons/moon';
  import Button from '../components/Button.svelte';
  import Link from '../components/Link.svelte';
  import { themeStore } from '../stores/theme.svelte.js';

  let { title = '', description = '', logo = null, navigation = [], actions = null } = $props();
  onMount(() => themeStore.init());
</script>

<header class="flex flex-wrap items-center justify-between gap-3 border-b border-ds-border bg-ds-surface-raised px-4 py-3 sm:px-6" data-testid="customer-header">
  <div class="flex min-w-0 items-center gap-3">
    {#if logo}<img src={logo} alt="" class="h-7 w-7 object-contain" />{/if}
    <div class="min-w-0"><h1 class="truncate text-lg font-semibold">{title}</h1>{#if description}<p class="text-sm text-ds-text-subtle">{description}</p>{/if}</div>
  </div>
  {#if navigation.length > 1}
    <nav class="order-last flex w-full gap-2 overflow-x-auto sm:order-none sm:w-auto" aria-label="Portal sections" data-testid="customer-module-navigation">
      {#each navigation as item (item.id)}<Link href={item.href} active={item.active} data-testid={`customer-module-${item.id}`} class="shrink-0 rounded px-3 py-2 text-sm">{item.label}</Link>{/each}
    </nav>
  {/if}
  <div class="flex items-center gap-2">
    {#if actions}{@render actions()}{/if}
    <Button variant="ghost" size="small" icon={themeStore.resolvedTheme === 'dark' ? Sun : Moon} ariaLabel="Toggle theme" dataTestid="public-board-theme-toggle" onclick={() => themeStore.setColorMode(themeStore.resolvedTheme === 'dark' ? 'light' : 'dark')} />
  </div>
</header>
