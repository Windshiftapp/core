<script>
  import { Loader2, AlertCircle } from '@lucide/svelte';
  import { t } from '../stores/i18n.svelte.js';

  let {
    loading = false,
    error = null,
    isEmpty = false,
    loadingText = null,
    emptyIcon: EmptyIcon = null,
    emptyTitle = null,
    emptySubtitle = '',
    onRetry = null,
    children
  } = $props();
</script>

<div class="min-h-[160px] space-y-3">
  {#if loading}
    <div class="flex items-center justify-center gap-3 rounded-xl border border-dashed border-ds-border px-4 py-6 text-sm text-ds-text-subtle">
      <Loader2 class="w-5 h-5 animate-spin text-ds-interactive" />
      <span>{loadingText || t('common.loading')}</span>
    </div>
  {:else if error}
    <div class="flex items-center justify-center gap-3 rounded-xl border border-dashed border-ds-status-danger-border px-4 py-4 text-sm text-ds-text-danger">
      <AlertCircle class="w-5 h-5" />
      <div class="text-left">
        <p class="font-medium">{error}</p>
        {#if onRetry}
          <button
            class="text-xs text-ds-text-link hover:text-ds-text-link-hovered underline"
            onclick={onRetry}
          >
            {t('common.retry')}
          </button>
        {/if}
      </div>
    </div>
  {:else if isEmpty}
    <div class="flex flex-col items-center justify-center rounded-xl border border-dashed border-ds-border px-4 py-8 text-center text-ds-text-subtle">
      {#if EmptyIcon}
        <EmptyIcon class="h-10 w-10 mb-2 opacity-30" />
      {/if}
      <p class="text-sm font-medium text-ds-text">{emptyTitle || t('items.noItems')}</p>
      {#if emptySubtitle}
        <p class="text-xs text-ds-text-subtlest">{emptySubtitle}</p>
      {/if}
    </div>
  {:else}
    {@render children()}
  {/if}
</div>
