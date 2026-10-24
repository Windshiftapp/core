<script>
  import { onDestroy, onMount } from 'svelte';
  import IconRefresh from '@tabler/icons-svelte-runes/icons/refresh';
  import IconAlertCircle from '@tabler/icons-svelte-runes/icons/alert-circle';
  import IconTrash from '@tabler/icons-svelte-runes/icons/trash';
  import Card from '../../components/Card.svelte';
  import Input from '../../components/Input.svelte';

  let {
    title,
    subtitle,
    dataTestId = '',
    onLoad,
    refreshInterval = 30_000,
    showRefresh = true,
    refreshLabel = 'Refresh',
    loadingLabel = 'Refreshing…',
    errorTitle = 'Failed to load',
    lastRefreshed = null,
    loading = $bindable(false),
    error = $bindable(null),
    purgeOlderThan = $bindable('30d'),
    purging = false,
    purgeLabel = '',
    purgeHint = '(format: 30d, 168h, 60m)',
    purgeButtonLabel = 'Purge old rows',
    purgeLoadingLabel = 'Purging…',
    onPurge = null,
    children,
  } = $props();

  let interval;

  function refresh() {
    onLoad?.();
  }

  onMount(() => {
    refresh();
    if (refreshInterval) {
      interval = setInterval(refresh, refreshInterval);
    }
  });

  onDestroy(() => {
    if (interval) clearInterval(interval);
  });
</script>

<section class="space-y-6" data-testid={dataTestId}>
  <div class="flex items-start justify-between gap-4">
    <div>
      <h3 class="text-base font-semibold" style="color: var(--ds-text);">{title}</h3>
      {#if subtitle}
        <p class="text-sm" style="color: var(--ds-text-subtle);">{subtitle}</p>
      {/if}
    </div>
    {#if showRefresh}
      <button
        type="button"
        onclick={refresh}
        disabled={loading}
        class="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-md border"
        style="border-color: var(--ds-border); background-color: var(--ds-surface-raised); color: var(--ds-text);"
      >
        <IconRefresh size={14} stroke={1.75} />
        {loading ? loadingLabel : refreshLabel}
      </button>
    {/if}
  </div>

  {#if error}
    <Card variant="outlined" dataTestid="diagnostics-section-error">
      <div class="flex items-start gap-3" style="color: var(--ds-text-danger);">
        <IconAlertCircle size={18} stroke={1.75} style="flex-shrink: 0; margin-top: 2px;" />
        <div class="text-sm">
          <p class="font-medium">{errorTitle}</p>
          <p style="color: var(--ds-text-subtle);">{error}</p>
        </div>
      </div>
    </Card>
  {/if}

  {#if children}
    {@render children()}
  {/if}

  {#if onPurge}
    <Card variant="outlined">
      <div class="flex items-center gap-3 flex-wrap">
        <IconTrash size={16} stroke={1.75} style="color: var(--ds-icon-subtle);" />
        <div class="text-sm flex-1" style="color: var(--ds-text);">
          {purgeLabel}
          <Input
            type="text"
            bind:value={purgeOlderThan}
            placeholder="30d"
            class="inline-block mx-2 w-20 font-mono"
            size="small"
          />
          <span style="color: var(--ds-text-subtle);">{purgeHint}</span>
        </div>
        <button
          type="button"
          onclick={onPurge}
          disabled={purging}
          class="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-md border"
          style="border-color: var(--ds-border-danger); background-color: var(--ds-surface-raised); color: var(--ds-text-danger);"
        >
          {purging ? purgeLoadingLabel : purgeButtonLabel}
        </button>
      </div>
    </Card>
  {/if}

  {#if lastRefreshed}
    <p class="text-xs" style="color: var(--ds-text-subtle);">
      Last refreshed {lastRefreshed.toLocaleTimeString()}
    </p>
  {/if}
</section>
