<script>
  import { t } from '../stores/i18n.svelte.js';
  // Shared loading/error/empty/skeleton state for mobile list views.
  // Renders the caller's rows snippet once content is available.
  let {
    loading = false,
    errored = false,
    rowCount = 0,
    skeletonCount = 5,
    skeletonRowHeight = 56,
    errorTestId = 'mobile-list-error',
    emptyTestId = 'mobile-list-empty',
    errorMessage = undefined,
    emptyMessage = undefined,
    onretry = null,
    children,
  } = $props();
</script>

{#if loading && rowCount === 0}
  <div class="skeleton">
    {#each Array(skeletonCount) as _}
      <div class="sk-row" style="height: {skeletonRowHeight}px"></div>
    {/each}
  </div>
{:else if errored}
  <div class="msg" data-testid={errorTestId}>
    <p>{errorMessage ?? t('mobile.common.loadFailed')}</p>
    {#if onretry}
      <button class="retry" onclick={onretry} disabled={loading} type="button">{t('common.retry')}</button>
    {/if}
  </div>
{:else if rowCount === 0}
  <p class="msg" data-testid={emptyTestId}>{emptyMessage ?? t('mobile.common.empty')}</p>
{:else}
  {@render children()}
{/if}

<style>
  .skeleton { display: flex; flex-direction: column; }

  .sk-row {
    border-bottom: 1px solid var(--ds-border);
    background: linear-gradient(90deg, var(--ds-surface) 0%, var(--ds-background-neutral) 50%, var(--ds-surface) 100%);
    background-size: 200% 100%;
    animation: shimmer 1.2s ease-in-out infinite;
  }

  @keyframes shimmer {
    0% { background-position: 200% 0; }
    100% { background-position: -200% 0; }
  }

  .msg {
    padding: 2rem 1.25rem;
    text-align: center;
    color: var(--ds-text-subtle);
    font-size: 0.875rem;
  }

  .msg p { margin: 0; }

  .retry {
    min-height: 40px;
    margin-top: 0.75rem;
    padding: 0.45rem 1rem;
    border: 1px solid var(--ds-interactive);
    border-radius: var(--radius-md, 6px);
    background: var(--ds-interactive);
    color: var(--ds-text-inverse, #fff);
    font: inherit;
    font-weight: var(--font-semibold, 600);
    cursor: pointer;
  }

  .retry:disabled { opacity: 0.6; }
</style>
