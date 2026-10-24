<script>
  import { t } from '../../stores/i18n.svelte.js';
  import { collectionStore } from '../../stores/collectionContext.js';

  let { testid = 'collection-load-more' } = $props();
</script>

<!-- Map and roadmap build their hierarchy from the loaded page, so a partial
     page is an incomplete hierarchy. The button is the explicit continuation
     contract; it disappears once every page is loaded. -->
{#if collectionStore.itemsHasMore}
  <div class="mt-6 shrink-0 text-center" data-testid={testid}>
    <button
      data-testid="{testid}-button"
      onclick={() => collectionStore.loadMoreItems()}
      disabled={collectionStore.itemsLoadingMore}
      class="px-4 py-2 text-sm font-medium rounded-lg border transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
      style="border-color: var(--ctx-border, var(--ds-border));"
    >
      {collectionStore.itemsLoadingMore ? t('common.loading') : t('common.loadMore')}
      {#if collectionStore.itemsRemainingCount > 0}
        ({collectionStore.itemsRemainingCount} {t('common.remaining')})
      {/if}
    </button>
  </div>
{/if}
