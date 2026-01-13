<script>
  import { Loader2, AlertCircle } from 'lucide-svelte';

  let {
    loading = false,
    error = null,
    isEmpty = false,
    loadingText = 'Loading...',
    emptyIcon = null,
    emptyTitle = 'No items',
    emptySubtitle = '',
    onRetry = null,
    children
  } = $props();
</script>

<div class="min-h-[160px] space-y-3">
  {#if loading}
    <div class="flex items-center justify-center gap-3 rounded-xl border border-dashed border-gray-200 px-4 py-6 text-sm text-gray-600">
      <Loader2 class="w-5 h-5 animate-spin text-blue-500" />
      <span>{loadingText}</span>
    </div>
  {:else if error}
    <div class="flex items-center justify-center gap-3 rounded-xl border border-dashed border-rose-200 px-4 py-4 text-sm text-rose-600">
      <AlertCircle class="w-5 h-5" />
      <div class="text-left">
        <p class="font-medium">{error}</p>
        {#if onRetry}
          <button
            class="text-xs text-blue-600 hover:text-blue-800 underline"
            onclick={onRetry}
          >
            Try again
          </button>
        {/if}
      </div>
    </div>
  {:else if isEmpty}
    <div class="flex flex-col items-center justify-center rounded-xl border border-dashed border-gray-200 px-4 py-8 text-center text-gray-500">
      {#if emptyIcon}
        <svelte:component this={emptyIcon} class="h-10 w-10 mb-2 opacity-30" />
      {/if}
      <p class="text-sm font-medium text-gray-700">{emptyTitle}</p>
      {#if emptySubtitle}
        <p class="text-xs text-gray-400">{emptySubtitle}</p>
      {/if}
    </div>
  {:else}
    {@render children()}
  {/if}
</div>
