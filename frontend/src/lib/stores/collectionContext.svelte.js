import {
  fetchCollectionBacklog,
  fetchCollectionItems,
} from '../features/collections/collectionService.js';
import { currentRoute } from '../router.js';

const COLLECTION_VIEWS = new Set([
  'workspace-board',
  'workspace-board-config',
  'workspace-backlog',
  'workspace-list',
  'workspace-tree',
  'workspace-map',
]);

const DEFAULT_PAGE_SIZE = 250;

class CollectionStore {
  // Reactive state
  items = $state([]);
  backlogItems = $state([]);
  collectionName = $state('Default');
  loading = $state(false);

  // Items pagination
  itemsPagination = $state(null);
  itemsHasMore = $state(false);
  itemsLoadingMore = $state(false);

  // Backlog pagination
  backlogPagination = $state(null);
  backlogHasMore = $state(false);
  backlogLoadingMore = $state(false);

  // Sub-filter QL (clears on navigation)
  subFilterQL = $state('');

  // Internal tracking
  #wsId = null;
  #colId = null;
  #loadId = 0;
  #previousRouteKey = null;
  #unsubscribe = null;

  constructor() {
    this.#unsubscribe = currentRoute.subscribe(($route) => {
      const wsId = $route.params?.id;
      const colId = $route.params?.collectionId || null;
      const view = $route.view;

      if (!wsId || !COLLECTION_VIEWS.has(view)) return;

      const routeKey = `${wsId}-${colId}-${view}`;
      if (routeKey === this.#previousRouteKey) return;
      this.#previousRouteKey = routeKey;

      this.load(wsId, colId);
    });
  }

  /**
   * Initial load: fetches page 1 of items and backlog, resets all pagination state.
   */
  async load(wsId, colId) {
    // Clear sub-filter on navigation (workspace or collection change)
    if (wsId !== this.#wsId || colId !== this.#colId) {
      this.subFilterQL = '';
    }
    this.#wsId = wsId;
    this.#colId = colId;
    const loadId = ++this.#loadId;

    this.loading = true;

    try {
      const [itemsResult, backlogResult] = await Promise.all([
        fetchCollectionItems(wsId, colId, {
          page: 1,
          limit: DEFAULT_PAGE_SIZE,
          sub_ql: this.subFilterQL || undefined,
        }),
        fetchCollectionBacklog(wsId, colId, { page: 1, limit: DEFAULT_PAGE_SIZE }),
      ]);

      if (loadId !== this.#loadId) return; // stale

      this.items = itemsResult.items;
      this.collectionName = itemsResult.collectionName;
      this.itemsPagination = itemsResult.pagination;
      this.itemsHasMore = itemsResult.pagination
        ? itemsResult.pagination.page < itemsResult.pagination.total_pages
        : false;

      this.backlogItems = backlogResult.items;
      this.backlogPagination = backlogResult.pagination;
      this.backlogHasMore = backlogResult.pagination
        ? backlogResult.pagination.page < backlogResult.pagination.total_pages
        : false;
    } catch (error) {
      if (loadId !== this.#loadId) return;
      console.error('[collectionStore] Load failed:', error);
    } finally {
      if (loadId === this.#loadId) {
        this.loading = false;
      }
    }
  }

  /**
   * Append mode: fetch next items page and append to existing items.
   */
  async loadMoreItems() {
    if (!this.itemsHasMore || this.itemsLoadingMore) return;

    const nextPage = (this.itemsPagination?.page ?? 0) + 1;
    this.itemsLoadingMore = true;

    try {
      const result = await fetchCollectionItems(this.#wsId, this.#colId, {
        page: nextPage,
        limit: this.itemsPagination?.limit ?? DEFAULT_PAGE_SIZE,
        sub_ql: this.subFilterQL || undefined,
      });

      this.items = [...this.items, ...result.items];
      this.itemsPagination = result.pagination;
      this.itemsHasMore = result.pagination
        ? result.pagination.page < result.pagination.total_pages
        : false;
    } catch (error) {
      console.error('[collectionStore] loadMoreItems failed:', error);
    } finally {
      this.itemsLoadingMore = false;
    }
  }

  /**
   * Append mode: fetch next backlog page and append to existing backlog items.
   */
  async loadMoreBacklog() {
    if (!this.backlogHasMore || this.backlogLoadingMore) return;

    const nextPage = (this.backlogPagination?.page ?? 0) + 1;
    this.backlogLoadingMore = true;

    try {
      const result = await fetchCollectionBacklog(this.#wsId, this.#colId, {
        page: nextPage,
        limit: this.backlogPagination?.limit ?? DEFAULT_PAGE_SIZE,
      });

      this.backlogItems = [...this.backlogItems, ...result.items];
      this.backlogPagination = result.pagination;
      this.backlogHasMore = result.pagination
        ? result.pagination.page < result.pagination.total_pages
        : false;
    } catch (error) {
      console.error('[collectionStore] loadMoreBacklog failed:', error);
    } finally {
      this.backlogLoadingMore = false;
    }
  }

  /**
   * Replace mode: fetch a specific page of items (replaces current items).
   * Used by List view for page-based navigation and by Tree/Map for large fetches.
   */
  async setItemsPage(page, limit = DEFAULT_PAGE_SIZE) {
    this.loading = true;
    const loadId = ++this.#loadId;

    try {
      const result = await fetchCollectionItems(this.#wsId, this.#colId, {
        page,
        limit,
        sub_ql: this.subFilterQL || undefined,
      });

      if (loadId !== this.#loadId) return;

      this.items = result.items;
      this.collectionName = result.collectionName;
      this.itemsPagination = result.pagination;
      this.itemsHasMore = result.pagination
        ? result.pagination.page < result.pagination.total_pages
        : false;
    } catch (error) {
      if (loadId !== this.#loadId) return;
      console.error('[collectionStore] setItemsPage failed:', error);
    } finally {
      if (loadId === this.#loadId) {
        this.loading = false;
      }
    }
  }

  /**
   * Refresh current data without resetting pagination.
   * Re-fetches page 1 with limit = current item count, preserving accumulated items.
   * Used by pollers and background updates.
   */
  async refresh() {
    if (!this.#wsId) return;
    const loadId = ++this.#loadId;

    const itemsLimit = Math.max(DEFAULT_PAGE_SIZE, this.items.length);
    const backlogLimit = Math.max(DEFAULT_PAGE_SIZE, this.backlogItems.length);

    try {
      const [itemsResult, backlogResult] = await Promise.all([
        fetchCollectionItems(this.#wsId, this.#colId, {
          page: 1,
          limit: itemsLimit,
          sub_ql: this.subFilterQL || undefined,
        }),
        fetchCollectionBacklog(this.#wsId, this.#colId, { page: 1, limit: backlogLimit }),
      ]);
      if (loadId !== this.#loadId) return;

      // Merge: preserve locally-present items that are beyond the server's pagination window
      const serverItemIds = new Set(itemsResult.items.map((i) => i.id));
      const localOnlyItems = this.items.filter((i) => !serverItemIds.has(i.id));
      this.items = [...itemsResult.items, ...localOnlyItems];

      this.collectionName = itemsResult.collectionName;
      this.itemsPagination = itemsResult.pagination;
      this.itemsHasMore = itemsResult.pagination
        ? itemsResult.pagination.page < itemsResult.pagination.total_pages
        : false;

      const serverBacklogIds = new Set(backlogResult.items.map((i) => i.id));
      const localOnlyBacklog = this.backlogItems.filter((i) => !serverBacklogIds.has(i.id));
      this.backlogItems = [...backlogResult.items, ...localOnlyBacklog];

      this.backlogPagination = backlogResult.pagination;
      this.backlogHasMore = backlogResult.pagination
        ? backlogResult.pagination.page < backlogResult.pagination.total_pages
        : false;
    } catch (error) {
      if (loadId !== this.#loadId) return;
      console.error('[collectionStore] Refresh failed:', error);
    }
  }

  /**
   * Apply a sub-filter QL query and reload items.
   */
  setSubFilter(ql) {
    this.subFilterQL = ql;
    if (this.#wsId) {
      this.load(this.#wsId, this.#colId);
    }
  }

  /**
   * Clear the sub-filter and reload items.
   */
  clearSubFilter() {
    this.subFilterQL = '';
    if (this.#wsId) {
      this.load(this.#wsId, this.#colId);
    }
  }

  /**
   * Re-trigger load() with current wsId/colId.
   */
  reload() {
    if (this.#wsId) {
      this.load(this.#wsId, this.#colId);
    }
  }

  destroy() {
    if (this.#unsubscribe) {
      this.#unsubscribe();
    }
  }
}

export const collectionStore = new CollectionStore();

/** Trigger a background refresh preserving current pagination */
export function reloadCollection() {
  collectionStore.refresh();
}

/**
 * Backward-compatible derived-like store object.
 * Components using $collectionData will continue to work.
 */
export const collectionData = {
  subscribe(fn) {
    // Use $effect.root for reactive subscriptions to the class-based store
    let cleanup;
    const run = () => {
      const value = {
        items: collectionStore.items,
        backlogItems: collectionStore.backlogItems,
        collectionName: collectionStore.collectionName,
        loading: collectionStore.loading,
      };
      fn(value);
    };

    cleanup = $effect.root(() => {
      $effect(() => {
        run();
      });
    });

    return () => {
      if (cleanup) cleanup();
    };
  },
};
