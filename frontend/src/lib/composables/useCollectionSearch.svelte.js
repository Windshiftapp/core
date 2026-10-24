import { onDestroy } from 'svelte';
import { collectionStore } from '../stores/collectionContext.svelte.js';

const DEBOUNCE_MS = 250;

/**
 * Server-side search shared by the collection views (board, list).
 *
 * The caller owns the input value; this composable debounces it into
 * `collectionStore.runSearch` and exposes the scoped result set alongside the
 * normal (unsearched) pagination, so a view renders the same shape whether or
 * not a query is active.
 *
 * @param {() => string} getScopeKey Identifies workspace/collection/sub-filter.
 *   Changing it restarts the search so a response for a stale scope never wins.
 */
export function useCollectionSearch(getScopeKey) {
  let query = $state('');
  let debouncing = $state(false);
  let effectId = 0;

  const active = $derived(Boolean(query.trim()));
  const items = $derived(active ? collectionStore.searchItems : collectionStore.items);
  const hasMore = $derived(active ? collectionStore.searchHasMore : collectionStore.itemsHasMore);
  const loadingMore = $derived(
    active
      ? collectionStore.searchLoadingMore
      : collectionStore.itemsLoadingMore || collectionStore.boardBackgroundLoading
  );
  const remainingCount = $derived(
    active ? collectionStore.searchRemainingCount : collectionStore.itemsRemainingCount
  );

  $effect(() => {
    const normalized = query.trim();
    const scope = getScopeKey();
    const id = ++effectId;
    collectionStore.clearSearch();

    if (!normalized) {
      debouncing = false;
      return;
    }

    debouncing = true;
    const timer = setTimeout(async () => {
      await collectionStore.runSearch(normalized);
      if (id === effectId && normalized === query.trim() && scope === getScopeKey()) {
        debouncing = false;
      }
    }, DEBOUNCE_MS);

    return () => clearTimeout(timer);
  });

  onDestroy(() => collectionStore.clearSearch());

  return {
    get query() {
      return query;
    },
    set query(value) {
      query = value;
    },
    get active() {
      return active;
    },
    get items() {
      return items;
    },
    get hasMore() {
      return hasMore;
    },
    get loadingMore() {
      return loadingMore;
    },
    get remainingCount() {
      return remainingCount;
    },
    get debouncing() {
      return debouncing;
    },
    loadMore() {
      return collectionStore.loadMoreSearchItems();
    },
  };
}
