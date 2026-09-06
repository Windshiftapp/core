import { api } from '../api.js';
import {
  RIGHTMOST_COLUMN_LIMIT,
  rightmostCapStatusIds,
} from '../features/collections/boardColumns.js';
import {
  fetchCollectionBacklog,
  fetchCollectionItemChanges,
  fetchCollectionItems,
  fetchItemsById,
  getCollection,
} from '../features/collections/collectionService.js';
import { currentRoute, GLOBAL_COLLECTION_VIEWS } from '../router.js';
import { isExpectedBackgroundSyncError } from '../utils/backgroundSync.js';
import { calcHasMore } from '../utils/paginationUtils.js';
import { workspaceDataStore } from './workspaceDataStore.svelte.js';

const COLLECTION_VIEWS = new Set([
  'workspace-board',
  'workspace-backlog',
  'workspace-list',
  'workspace-tree',
  'workspace-map',
  'workspace-roadmap',
]);

const BOARD_VIEWS = new Set(['workspace-board', 'collection-board']);
const LIST_VIEWS = new Set(['workspace-list', 'collection-list']);
const BACKLOG_VIEWS = new Set(['workspace-backlog', 'collection-backlog']);

const DEFAULT_PAGE_SIZE = 100;
const LIST_INITIAL_PAGE_SIZE = 50;
const LARGE_COLLECTION_PAGE_SIZE = 250;
const BOARD_UNFINISHED_PAGE_SIZE = 1000;
const BOARD_UNTHROTTLED_ITEM_COUNT = 1000;
const BOARD_BACKGROUND_PAGE_DELAY_MS = 250;
const BOARD_SEARCH_PAGE_SIZE = 100;

function delay(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function appendUniqueItems(existing, incoming) {
  const ids = new Set(existing.map((item) => item.id));
  return [...existing, ...incoming.filter((item) => !ids.has(item.id))];
}

function initialItemsPageSize(view) {
  if (view === 'workspace-list' || view === 'collection-list') return LIST_INITIAL_PAGE_SIZE;
  if (
    view === 'workspace-tree' ||
    view === 'collection-tree' ||
    view === 'workspace-map' ||
    view === 'collection-map' ||
    view === 'workspace-roadmap' ||
    view === 'collection-roadmap'
  ) {
    return LARGE_COLLECTION_PAGE_SIZE;
  }
  return DEFAULT_PAGE_SIZE;
}

function loadsItems(view) {
  return !BACKLOG_VIEWS.has(view);
}

function loadsBacklog(view) {
  return BOARD_VIEWS.has(view) || BACKLOG_VIEWS.has(view);
}

function minimumWatermark(...values) {
  const watermarks = values
    .filter((value) => value != null)
    .map(Number)
    .filter(Number.isFinite);
  return watermarks.length > 0 ? Math.min(...watermarks) : null;
}

// Parallel item/backlog reads can start on opposite sides of a concurrent
// mutation. Retaining the oldest response watermark guarantees the delta poll
// will replay anything that was not present in every returned snapshot.
function snapshotWatermark(...results) {
  return minimumWatermark(...results.filter(Boolean).map((result) => result.watermark ?? 0));
}

class CollectionStore {
  // Reactive state
  items = $state([]);
  backlogItems = $state([]);
  collectionName = $state('Default');
  publicSlug = $state(null);
  loading = $state(false);

  // Items pagination
  itemsPagination = $state(null);
  itemsHasMore = $state(false);
  itemsLoadingMore = $state(false);
  boardBackgroundLoading = $state(false);

  // Board views load every unfinished item separately from completed work.
  // The rightmost cap is retained as a specialized view of that partition.
  boardDeferred = $state(null);
  rightmostCap = $state(null);

  // Board search is a separate server-scoped result set. Keeping it apart
  // prevents a search from changing the board's normal cap or pagination.
  boardSearchItems = $state([]);
  boardSearchPagination = $state(null);
  boardSearchLoading = $state(false);
  boardSearchLoadingMore = $state(false);
  boardSearchError = $state(false);

  // Backlog pagination
  backlogPagination = $state(null);
  backlogHasMore = $state(false);
  backlogLoadingMore = $state(false);

  // Sub-filter QL (clears on navigation)
  subFilterQL = $state('');
  // Raw filter rows backing the QL — kept so the SubFilterBar UI can hydrate
  // its builder when remounted on a different view of the same collection.
  subFilterRows = $state([]);
  showCompleted = $state(false);

  get effectiveSubFilterQL() {
    if (this.showCompleted) return this.subFilterQL;
    return this.subFilterQL
      ? `(${this.subFilterQL}) AND status_completed = false`
      : 'status_completed = false';
  }

  #completionPreferenceKey(wsId, colId, view) {
    const scope = colId ? `collection-${colId}` : `workspace-${wsId}`;
    return `collection-show-completed:${scope}:${view?.split('-').at(-1)}`;
  }

  async setShowCompleted(value) {
    this.showCompleted = value;
    try {
      localStorage.setItem(
        this.#completionPreferenceKey(this.#wsId, this.#colId, this.#currentView),
        String(value)
      );
    } catch {
      // Keep the setting usable when browser storage is unavailable.
    }
    this.clearBoardSearch();
    if (this.#wsId || this.#colId) {
      await this.load(this.#wsId, this.#colId, this.#currentView);
    }
  }

  // Server-side sort state
  sortableFields = $state([]);
  boardConfiguration = $state(null);
  boardCollection = $state(null);
  boardWorkspaceIds = $state([]);
  boardStatuses = $state([]);
  boardWorkspaceScopeLoaded = $state(false);
  #sortBy = null;
  #sortDirection = null;
  #boardSortMode = 'rank';

  // Internal tracking
  #wsId = null;
  #colId = null;
  #loadId = 0;
  #changesWatermark = null;
  #previousRouteKey = null;
  #currentView = null;
  #unsubscribe = null;
  #boardConfigurationKey = null;
  #boardConfigurationPromise = null;
  #boardConfigurationLoaded = false;
  #boardSearchId = 0;
  #boardSearchQuery = '';

  constructor() {
    this.#unsubscribe = currentRoute.subscribe(($route) => {
      const view = $route.view;

      // Global collection views: /collections/:id/board etc.
      if (GLOBAL_COLLECTION_VIEWS.has(view)) {
        const colId = $route.params?.id || null;
        if (!colId) return;

        const routeKey = `${view}-global-${colId}`;
        if (routeKey === this.#previousRouteKey) return;
        this.#previousRouteKey = routeKey;

        this.load(null, colId, view);
        return;
      }

      // Workspace collection views
      const wsId = $route.params?.id;
      const colId = $route.params?.collectionId || null;

      if (!wsId || !COLLECTION_VIEWS.has(view)) {
        // Navigated away from a collection view — clear the route key
        // so that returning to the same collection triggers a fresh load.
        this.#previousRouteKey = null;
        return;
      }

      const routeKey = `${view}-${wsId}-${colId}`;
      if (routeKey === this.#previousRouteKey) return;
      this.#previousRouteKey = routeKey;

      this.load(wsId, colId, view);
    });
  }

  /**
   * Initial load: fetches page 1 of items and backlog, resets all pagination state.
   */
  async load(wsId, colId, view = this.#currentView) {
    const sameCollection = wsId === this.#wsId && colId === this.#colId;
    const viewChanged = view !== this.#currentView;
    const targetInitialLimit = initialItemsPageSize(view);
    const previousShowCompleted = this.showCompleted;
    if (BOARD_VIEWS.has(view)) {
      this.showCompleted = true;
    } else if (!sameCollection || viewChanged) {
      try {
        this.showCompleted =
          localStorage.getItem(this.#completionPreferenceKey(wsId, colId, view)) === 'true';
      } catch {
        this.showCompleted = false;
      }
    }

    // Switching between passive collection views does not need another network
    // roundtrip when the already-loaded item page is large enough and there is
    // no active server-side sort/filter. Board views may need capped-column
    // fetches, so they intentionally keep loading.
    const canReuseTargetData =
      loadsItems(view) &&
      loadsItems(this.#currentView) &&
      this.items.length > 0 &&
      !this.boardDeferred &&
      (this.itemsPagination?.limit ?? 0) >= targetInitialLimit;
    if (
      sameCollection &&
      viewChanged &&
      previousShowCompleted === this.showCompleted &&
      !this.loading &&
      !BOARD_VIEWS.has(this.#currentView) &&
      canReuseTargetData &&
      !this.subFilterQL &&
      !this.#sortBy &&
      !this.#sortDirection &&
      !BOARD_VIEWS.has(view)
    ) {
      this.#currentView = view;
      return;
    }

    // Clear all scope-owned state synchronously on workspace/collection changes.
    // MainApp reuses collection view component instances, so retaining the old
    // arrays until the next request resolves briefly renders one workspace's
    // board in another workspace (and leaves it there if the request fails).
    if (!sameCollection) {
      this.items = [];
      this.backlogItems = [];
      this.collectionName = 'Default';
      this.itemsPagination = null;
      this.itemsHasMore = false;
      this.itemsLoadingMore = false;
      this.boardBackgroundLoading = false;
      this.boardDeferred = null;
      this.rightmostCap = null;
      this.backlogPagination = null;
      this.backlogHasMore = false;
      this.backlogLoadingMore = false;
      this.subFilterQL = '';
      this.subFilterRows = [];
      this.publicSlug = null;
      this.#sortBy = null;
      this.#sortDirection = null;
      this.sortableFields = [];
      this.boardConfiguration = null;
      this.boardCollection = null;
      this.boardWorkspaceIds = [];
      this.boardStatuses = [];
      this.boardWorkspaceScopeLoaded = false;
      this.#boardConfigurationKey = null;
      this.#boardConfigurationPromise = null;
      this.#boardConfigurationLoaded = false;
      this.#changesWatermark = null;
      this.clearBoardSearch();
    }
    this.#wsId = wsId;
    this.#colId = colId;
    this.#currentView = view;
    const loadId = ++this.#loadId;

    this.boardBackgroundLoading = false;
    this.loading = true;

    try {
      const boardPartition = await this.#resolveBoardPartition(wsId, colId, view);
      const collection = colId
        ? BOARD_VIEWS.has(view)
          ? (this.boardCollection ?? (await getCollection(colId)))
          : await getCollection(colId)
        : null;
      if (loadId !== this.#loadId) return; // stale

      const [itemsResult, backlogResult, deferredResult] = await Promise.all([
        loadsItems(view)
          ? this.#fetchMainItems(
              wsId,
              colId,
              targetInitialLimit,
              boardPartition,
              collection,
              BOARD_VIEWS.has(view)
            )
          : Promise.resolve(null),
        loadsBacklog(view)
          ? fetchCollectionBacklog(wsId, colId, {
              page: 1,
              limit: DEFAULT_PAGE_SIZE,
              sub_ql: this.effectiveSubFilterQL || undefined,
              collection,
            })
          : Promise.resolve(null),
        loadsItems(view)
          ? this.#fetchBoardDeferredItems(wsId, colId, boardPartition, collection)
          : Promise.resolve(null),
      ]);

      if (loadId !== this.#loadId) return; // stale

      if (itemsResult) {
        this.items = deferredResult
          ? [...itemsResult.items, ...deferredResult.items]
          : itemsResult.items;
        this.boardDeferred = deferredResult
          ? {
              ...boardPartition,
              pagination: deferredResult.pagination,
              total: deferredResult.pagination?.total_items ?? deferredResult.items.length,
            }
          : null;
        this.rightmostCap = this.boardDeferred?.capped
          ? {
              statusIds: this.boardDeferred.statusIds,
              total: this.boardDeferred.total,
            }
          : null;
        this.collectionName = itemsResult.collectionName;
        this.publicSlug = itemsResult.publicSlug ?? null;
        this.itemsPagination = itemsResult.pagination;
        this.itemsHasMore = this.#hasMoreItems();
        if (itemsResult.sortableFields?.length) {
          this.sortableFields = itemsResult.sortableFields;
        }
      }

      if (backlogResult) {
        this.backlogItems = backlogResult.items;
        this.backlogPagination = backlogResult.pagination;
        this.backlogHasMore = calcHasMore(backlogResult.pagination);
        if (!itemsResult) {
          this.collectionName = backlogResult.collectionName;
          this.publicSlug =
            collection?.is_public && collection?.public_slug ? collection.public_slug : null;
        }
      }
      this.#changesWatermark = snapshotWatermark(itemsResult, backlogResult, deferredResult);
      if (itemsResult && BOARD_VIEWS.has(view)) {
        void this.#loadRemainingBoardItems({
          first: itemsResult,
          wsId,
          colId,
          boardPartition,
          collection,
          loadId,
        });
      }
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
   * Resolves the statuses deferred from the board's full unfinished fetch.
   * A capped rightmost column keeps its existing 50-card behavior; otherwise
   * completed statuses are paged separately so they cannot hide active work.
   */
  async #resolveBoardPartition(wsId, colId, view) {
    if (!BOARD_VIEWS.has(view) || !this.showCompleted) return null;
    try {
      const config = await this.getBoardConfiguration(wsId, colId);
      let statuses = this.boardStatuses;
      if (statuses.length === 0) {
        if (wsId) {
          await workspaceDataStore.initialize(wsId);
        } else {
          await workspaceDataStore.initializeGlobal();
        }
        statuses = workspaceDataStore.statuses;
      }

      const cappedStatusIds = rightmostCapStatusIds(config, statuses);
      if (cappedStatusIds?.length) {
        return { statusIds: cappedStatusIds, limit: RIGHTMOST_COLUMN_LIMIT, capped: true };
      }

      const completedStatusIds = statuses
        .filter((status) => status.is_completed)
        .map((status) => status.id);
      const retentionDays = Number(config?.completed_item_retention_days);
      const completedActivityDays =
        Number.isInteger(retentionDays) && retentionDays >= 1 && retentionDays <= 3650
          ? retentionDays
          : null;
      return completedStatusIds.length
        ? {
            statusIds: completedStatusIds,
            limit: DEFAULT_PAGE_SIZE,
            capped: false,
            completedActivityDays,
          }
        : null;
    } catch (error) {
      if (error?.status !== 404) {
        console.error('[collectionStore] board configuration lookup failed:', error);
      }
      return null;
    }
  }

  async getBoardConfiguration(wsId = this.#wsId, colId = this.#colId, { force = false } = {}) {
    const key = `${colId ?? ''}|${wsId ?? ''}`;
    if (!force && this.#boardConfigurationLoaded && this.#boardConfigurationKey === key) {
      return this.boardConfiguration;
    }
    if (!force && this.#boardConfigurationPromise && this.#boardConfigurationKey === key) {
      return this.#boardConfigurationPromise;
    }

    this.#boardConfigurationKey = key;
    this.#boardConfigurationLoaded = false;
    this.boardWorkspaceScopeLoaded = false;
    const request = api.collections
      .getBoardConfigurationBootstrap(colId || null, wsId || null)
      .catch((error) => {
        if (error?.status !== 404) throw error;
        return null;
      })
      .then((bootstrap) => {
        const config = bootstrap?.board_configuration ?? null;
        if (this.#boardConfigurationKey === key) {
          this.boardConfiguration = config;
          this.boardCollection = bootstrap?.collection ?? null;
          this.boardWorkspaceIds = Array.isArray(bootstrap?.referenced_workspace_ids)
            ? bootstrap.referenced_workspace_ids
            : [];
          this.boardStatuses = Array.isArray(bootstrap?.statuses) ? bootstrap.statuses : [];
          this.boardWorkspaceScopeLoaded = true;
          this.#boardConfigurationLoaded = true;
        }
        return config;
      })
      .finally(() => {
        if (this.#boardConfigurationPromise === request) {
          this.#boardConfigurationPromise = null;
        }
      });
    this.#boardConfigurationPromise = request;
    return request;
  }

  invalidateBoardConfiguration(wsId = this.#wsId, colId = this.#colId) {
    const key = `${colId ?? ''}|${wsId ?? ''}`;
    if (this.#boardConfigurationKey !== key) return;
    this.#boardConfigurationKey = null;
    this.#boardConfigurationPromise = null;
    this.#boardConfigurationLoaded = false;
    this.boardConfiguration = null;
    this.boardCollection = null;
    this.boardWorkspaceIds = [];
    this.boardStatuses = [];
    this.boardWorkspaceScopeLoaded = false;
  }

  #boardExclusionFilter(statusIds = this.boardDeferred?.statusIds) {
    return statusIds?.length ? { status_id_not: statusIds.join(',') } : {};
  }

  #fetchMainItems(wsId, colId, limit, boardPartition, collection, isBoard = false) {
    const fetchPage = (page, pageLimit) =>
      fetchCollectionItems(wsId, colId, {
        page,
        limit: pageLimit,
        sub_ql: this.effectiveSubFilterQL || undefined,
        collection,
        ...this.#itemSortOptions(),
        ...this.#boardExclusionFilter(boardPartition?.statusIds),
      });

    if (!isBoard) return fetchPage(1, limit);
    return fetchPage(1, BOARD_UNFINISHED_PAGE_SIZE);
  }

  async #loadRemainingBoardItems({ first, wsId, colId, boardPartition, collection, loadId }) {
    const totalPages = first.pagination?.total_pages ?? 1;
    if (totalPages <= 1 || loadId !== this.#loadId) return;

    this.boardBackgroundLoading = true;
    let loadedCount = first.items.length;
    try {
      for (let page = 2; page <= totalPages; page++) {
        if (loadedCount >= BOARD_UNTHROTTLED_ITEM_COUNT) {
          await delay(BOARD_BACKGROUND_PAGE_DELAY_MS);
        }
        if (loadId !== this.#loadId) return;

        const result = await fetchCollectionItems(wsId, colId, {
          page,
          limit: BOARD_UNFINISHED_PAGE_SIZE,
          sub_ql: this.effectiveSubFilterQL || undefined,
          collection,
          ...this.#itemSortOptions(),
          ...this.#boardExclusionFilter(boardPartition?.statusIds),
        });
        if (loadId !== this.#loadId) return;

        this.items = appendUniqueItems(this.items, result.items);
        this.itemsPagination = result.pagination;
        if (!this.boardDeferred) this.itemsHasMore = calcHasMore(result.pagination);
        this.#changesWatermark = minimumWatermark(this.#changesWatermark, result.watermark ?? 0);
        loadedCount += result.items.length;
      }
    } catch (error) {
      if (loadId === this.#loadId && !isExpectedBackgroundSyncError(error)) {
        console.error('[collectionStore] background board load failed:', error);
      }
    } finally {
      if (loadId === this.#loadId) {
        this.boardBackgroundLoading = false;
      }
    }
  }

  async #fetchCompleteMainItems(wsId, colId, limit, boardPartition, collection, loadId, isBoard) {
    const first = await this.#fetchMainItems(
      wsId,
      colId,
      limit,
      boardPartition,
      collection,
      isBoard
    );
    const totalPages = first.pagination?.total_pages ?? 1;
    if (!isBoard || totalPages <= 1) return first;

    const pages = [first];
    let loadedCount = first.items.length;
    for (let page = 2; page <= totalPages; page++) {
      if (loadedCount >= BOARD_UNTHROTTLED_ITEM_COUNT) {
        await delay(BOARD_BACKGROUND_PAGE_DELAY_MS);
      }
      if (loadId !== this.#loadId) return first;

      const result = await fetchCollectionItems(wsId, colId, {
        page,
        limit: BOARD_UNFINISHED_PAGE_SIZE,
        sub_ql: this.effectiveSubFilterQL || undefined,
        collection,
        ...this.#itemSortOptions(),
        ...this.#boardExclusionFilter(boardPartition?.statusIds),
      });
      pages.push(result);
      loadedCount += result.items.length;
    }
    return {
      ...first,
      items: pages.flatMap((result) => result.items),
      pagination: pages.at(-1).pagination,
      watermark: snapshotWatermark(...pages),
    };
  }

  #fetchBoardDeferredItems(wsId, colId, boardPartition, collection, limitOverride = null) {
    if (!boardPartition?.statusIds?.length) return Promise.resolve(null);
    const limit = boardPartition.capped
      ? RIGHTMOST_COLUMN_LIMIT
      : Math.max(DEFAULT_PAGE_SIZE, limitOverride ?? boardPartition.limit ?? 0);
    return fetchCollectionItems(wsId, colId, {
      page: 1,
      limit,
      sub_ql: this.effectiveSubFilterQL || undefined,
      collection,
      status_id: boardPartition.statusIds.join(','),
      completed_activity_days: boardPartition.completedActivityDays || undefined,
      ...(boardPartition.capped
        ? { order_by: 'last_active_at', sort_direction: 'desc' }
        : this.#itemSortOptions()),
    });
  }

  /**
   * Number of loaded items outside the board's deferred status partition.
   */
  get mainItemsLoadedCount() {
    if (!this.boardDeferred) return this.items.length;
    const deferredSet = new Set(this.boardDeferred.statusIds);
    return this.items.filter((item) => !deferredSet.has(item.status_id)).length;
  }

  get itemsRemainingCount() {
    if (this.boardDeferred && !this.boardDeferred.capped) {
      const deferredSet = new Set(this.boardDeferred.statusIds);
      const loaded = this.items.filter((item) => deferredSet.has(item.status_id)).length;
      return Math.max(0, this.boardDeferred.total - loaded);
    }
    return Math.max(0, (this.itemsPagination?.total_items ?? 0) - this.mainItemsLoadedCount);
  }

  get itemsTotalCount() {
    const mainTotal = this.itemsPagination?.total_items ?? this.mainItemsLoadedCount;
    return mainTotal + (this.boardDeferred?.total ?? 0);
  }

  get boardSearchHasMore() {
    return calcHasMore(this.boardSearchPagination);
  }

  get boardSearchRemainingCount() {
    const loaded = this.boardSearchItems.length;
    return Math.max(0, (this.boardSearchPagination?.total_items ?? loaded) - loaded);
  }

  clearBoardSearch() {
    this.#boardSearchId++;
    this.#boardSearchQuery = '';
    this.boardSearchItems = [];
    this.boardSearchPagination = null;
    this.boardSearchLoading = false;
    this.boardSearchLoadingMore = false;
    this.boardSearchError = false;
  }

  async searchBoardItems(query) {
    const normalized = query.trim();
    if (!normalized) {
      this.clearBoardSearch();
      return;
    }

    const searchId = ++this.#boardSearchId;
    this.#boardSearchQuery = normalized;
    this.boardSearchItems = [];
    this.boardSearchPagination = null;
    this.boardSearchLoading = true;
    this.boardSearchError = false;

    try {
      const result = await this.#fetchBoardSearchPage(normalized, 1);
      if (searchId !== this.#boardSearchId || normalized !== this.#boardSearchQuery) return;
      this.boardSearchItems = result.items;
      this.boardSearchPagination = result.pagination;
    } catch (error) {
      if (searchId !== this.#boardSearchId) return;
      console.error('[collectionStore] board search failed:', error);
      this.boardSearchError = true;
    } finally {
      if (searchId === this.#boardSearchId) {
        this.boardSearchLoading = false;
      }
    }
  }

  async loadMoreBoardSearchItems() {
    if (!this.#boardSearchQuery || !this.boardSearchHasMore || this.boardSearchLoadingMore) return;

    const searchId = ++this.#boardSearchId;
    const query = this.#boardSearchQuery;
    const nextPage = (this.boardSearchPagination?.page ?? 0) + 1;
    this.boardSearchLoadingMore = true;
    this.boardSearchError = false;

    try {
      const result = await this.#fetchBoardSearchPage(query, nextPage);
      if (searchId !== this.#boardSearchId || query !== this.#boardSearchQuery) return;
      this.boardSearchItems = [...this.boardSearchItems, ...result.items];
      this.boardSearchPagination = result.pagination;
    } catch (error) {
      if (searchId !== this.#boardSearchId) return;
      console.error('[collectionStore] loading more board search results failed:', error);
      this.boardSearchError = true;
    } finally {
      if (searchId === this.#boardSearchId) {
        this.boardSearchLoadingMore = false;
      }
    }
  }

  #fetchBoardSearchPage(query, page) {
    return fetchCollectionItems(this.#wsId, this.#colId, {
      page,
      limit: BOARD_SEARCH_PAGE_SIZE,
      search: query,
      sub_ql: this.effectiveSubFilterQL || undefined,
      collection: this.boardCollection ?? undefined,
      ...this.#itemSortOptions(),
    });
  }

  #hasMoreItems() {
    if (this.boardDeferred) {
      return !this.boardDeferred.capped && calcHasMore(this.boardDeferred.pagination);
    }
    return calcHasMore(this.itemsPagination);
  }

  /**
   * Append mode: fetch next items page and append to existing items.
   */
  async loadMoreItems() {
    if (
      !this.itemsHasMore ||
      this.itemsLoadingMore ||
      (this.boardBackgroundLoading && !this.boardDeferred)
    ) {
      return;
    }

    const deferred = this.boardDeferred && !this.boardDeferred.capped ? this.boardDeferred : null;
    const pagination = deferred?.pagination ?? this.itemsPagination;
    const nextPage = (pagination?.page ?? 0) + 1;
    const loadId = this.#loadId;
    this.itemsLoadingMore = true;

    try {
      const result = await fetchCollectionItems(this.#wsId, this.#colId, {
        page: nextPage,
        limit: pagination?.limit ?? DEFAULT_PAGE_SIZE,
        sub_ql: this.effectiveSubFilterQL || undefined,
        ...this.#itemSortOptions(),
        ...(deferred
          ? {
              status_id: deferred.statusIds.join(','),
              completed_activity_days: deferred.completedActivityDays || undefined,
            }
          : this.#boardExclusionFilter()),
      });

      if (loadId !== this.#loadId) return;
      this.items = [...this.items, ...result.items];
      if (deferred) {
        this.boardDeferred = {
          ...deferred,
          pagination: result.pagination,
          total: result.pagination?.total_items ?? deferred.total,
        };
      } else {
        this.itemsPagination = result.pagination;
      }
      this.itemsHasMore = this.#hasMoreItems();
      this.#changesWatermark = minimumWatermark(this.#changesWatermark, result.watermark ?? 0);
    } catch (error) {
      console.error('[collectionStore] loadMoreItems failed:', error);
    } finally {
      if (loadId === this.#loadId) {
        this.itemsLoadingMore = false;
      }
    }
  }

  /**
   * Append mode: fetch next backlog page and append to existing backlog items.
   */
  async loadMoreBacklog() {
    if (!this.backlogHasMore || this.backlogLoadingMore) return;

    const nextPage = (this.backlogPagination?.page ?? 0) + 1;
    const loadId = this.#loadId;
    this.backlogLoadingMore = true;

    try {
      const result = await fetchCollectionBacklog(this.#wsId, this.#colId, {
        page: nextPage,
        limit: this.backlogPagination?.limit ?? DEFAULT_PAGE_SIZE,
        sub_ql: this.effectiveSubFilterQL || undefined,
      });

      if (loadId !== this.#loadId) return;
      this.backlogItems = [...this.backlogItems, ...result.items];
      this.backlogPagination = result.pagination;
      this.backlogHasMore = result.pagination
        ? result.pagination.page < result.pagination.total_pages
        : false;
      this.#changesWatermark = minimumWatermark(this.#changesWatermark, result.watermark ?? 0);
    } catch (error) {
      console.error('[collectionStore] loadMoreBacklog failed:', error);
    } finally {
      if (loadId === this.#loadId) {
        this.backlogLoadingMore = false;
      }
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
        sub_ql: this.effectiveSubFilterQL || undefined,
        ...this.#itemSortOptions(),
      });

      if (loadId !== this.#loadId) return;

      this.items = result.items;
      this.boardDeferred = null;
      this.rightmostCap = null;
      this.collectionName = result.collectionName;
      this.publicSlug = result.publicSlug ?? null;
      this.itemsPagination = result.pagination;
      this.itemsHasMore = result.pagination
        ? result.pagination.page < result.pagination.total_pages
        : false;
      if (result.sortableFields?.length) {
        this.sortableFields = result.sortableFields;
      }
      this.#changesWatermark = minimumWatermark(this.#changesWatermark, result.watermark ?? 0);
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
    if (!this.#wsId && !this.#colId) return;
    const loadId = ++this.#loadId;
    this.boardBackgroundLoading = false;

    const itemsLimit = Math.max(initialItemsPageSize(this.#currentView), this.mainItemsLoadedCount);
    const backlogLimit = Math.max(DEFAULT_PAGE_SIZE, this.backlogItems.length);

    try {
      const boardPartition = await this.#resolveBoardPartition(
        this.#wsId,
        this.#colId,
        this.#currentView
      );
      const collection = this.#colId
        ? BOARD_VIEWS.has(this.#currentView)
          ? (this.boardCollection ?? (await getCollection(this.#colId)))
          : await getCollection(this.#colId)
        : null;
      if (loadId !== this.#loadId) return;

      const deferredLoadedCount = this.items.length - this.mainItemsLoadedCount;
      const [itemsResult, backlogResult, deferredResult] = await Promise.all([
        loadsItems(this.#currentView)
          ? this.#fetchCompleteMainItems(
              this.#wsId,
              this.#colId,
              itemsLimit,
              boardPartition,
              collection,
              loadId,
              BOARD_VIEWS.has(this.#currentView)
            )
          : Promise.resolve(null),
        loadsBacklog(this.#currentView)
          ? fetchCollectionBacklog(this.#wsId, this.#colId, {
              page: 1,
              limit: backlogLimit,
              sub_ql: this.effectiveSubFilterQL || undefined,
              collection,
            })
          : Promise.resolve(null),
        loadsItems(this.#currentView)
          ? this.#fetchBoardDeferredItems(
              this.#wsId,
              this.#colId,
              boardPartition,
              collection,
              deferredLoadedCount
            )
          : Promise.resolve(null),
      ]);
      if (loadId !== this.#loadId) return;

      if (itemsResult) {
        this.items = deferredResult
          ? [...itemsResult.items, ...deferredResult.items]
          : itemsResult.items;
        this.boardDeferred = deferredResult
          ? {
              ...boardPartition,
              pagination: deferredResult.pagination,
              total: deferredResult.pagination?.total_items ?? deferredResult.items.length,
            }
          : null;
        this.rightmostCap = this.boardDeferred?.capped
          ? {
              statusIds: this.boardDeferred.statusIds,
              total: this.boardDeferred.total,
            }
          : null;
        this.collectionName = itemsResult.collectionName;
        this.publicSlug = itemsResult.publicSlug ?? null;
        this.itemsPagination = itemsResult.pagination;
        this.itemsHasMore = this.#hasMoreItems();
      }

      if (backlogResult) {
        this.backlogItems = backlogResult.items;
        this.backlogPagination = backlogResult.pagination;
        this.backlogHasMore = calcHasMore(backlogResult.pagination);
      }
      this.#changesWatermark = snapshotWatermark(itemsResult, backlogResult, deferredResult);
    } catch (error) {
      if (loadId !== this.#loadId) return;
      if (!isExpectedBackgroundSyncError(error)) {
        console.error('[collectionStore] Refresh failed:', error);
      }
    }
  }

  /**
   * Apply a sub-filter QL query and reload items.
   */
  setSubFilter(ql, rows = []) {
    this.subFilterQL = ql;
    this.subFilterRows = rows;
    if (this.#wsId || this.#colId) {
      this.load(this.#wsId, this.#colId, this.#currentView);
    }
  }

  /**
   * Clear the sub-filter and reload items.
   */
  clearSubFilter() {
    this.subFilterQL = '';
    this.subFilterRows = [];
    if (this.#wsId || this.#colId) {
      this.load(this.#wsId, this.#colId, this.#currentView);
    }
  }

  /**
   * Set server-side sort and reload from page 1.
   */
  setSorting(sortBy, sortDirection) {
    this.#sortBy = sortBy;
    this.#sortDirection = sortDirection;
    if (this.#wsId || this.#colId) {
      this.setItemsPage(1);
    }
  }

  /**
   * Set the board's effective server-side ordering and reload from page 1.
   * Bubble Mode must be applied before pagination so recently active items
   * cannot be hidden on a later frac_index-ordered page.
   */
  setBoardSortMode(mode) {
    const nextMode = mode === 'bubble' ? 'bubble' : 'rank';
    if (nextMode === this.#boardSortMode) return;

    this.#boardSortMode = nextMode;
    if (BOARD_VIEWS.has(this.#currentView) && (this.#wsId || this.#colId)) {
      this.load(this.#wsId, this.#colId, this.#currentView);
    }
  }

  /**
   * Re-trigger load() with current wsId/colId.
   */
  reload() {
    if (this.#wsId || this.#colId) {
      this.load(this.#wsId, this.#colId, this.#currentView);
    }
  }

  /**
   * Clear the route guard so the next navigation always triggers a fresh load.
   */
  invalidate() {
    this.#previousRouteKey = null;
  }

  async refreshItem(itemId) {
    try {
      const updated = await api.items.get(itemId);
      this.#applyUpdatedItem(updated);
    } catch (e) {
      console.error('[collectionStore] refreshItem failed:', e);
    }
  }

  applyItem(item) {
    if (!item?.id) return;
    if (this.#hideCompletedItem(item)) return;
    const index = this.items.findIndex((current) => current.id === item.id);
    if (index === -1) {
      this.items = [...this.items, item];
      return;
    }
    this.items = this.items.map((current, currentIndex) =>
      currentIndex === index ? item : current
    );
  }

  /**
   * Poll for cheap deltas and patch loaded rows by ID. Falls back to a full
   * refresh when the delta implies structural uncertainty (new visible item,
   * server-side sort, or backlog membership changes).
   */
  async refreshDeltas() {
    if ((!this.#wsId && !this.#colId) || this.loading) return;
    if (this.#changesWatermark === null) {
      await this.#primeChangesWatermark();
      return;
    }

    const loadId = this.#loadId;
    try {
      const changes = await fetchCollectionItemChanges(this.#wsId, this.#colId, {
        since: this.#changesWatermark,
        sub_ql: this.effectiveSubFilterQL || undefined,
      });
      if (loadId !== this.#loadId) return;
      this.#changesWatermark = changes?.watermark ?? this.#changesWatermark;

      if (changes?.requires_full_reload) {
        await this.refresh();
        return;
      }

      const removedIds = new Set(changes?.removed_item_ids ?? []);
      if (removedIds.size > 0) {
        this.#removeItemsById(removedIds);
      }

      const changedIds = [...new Set(changes?.changed_item_ids ?? [])].filter(
        (id) => !removedIds.has(id)
      );
      if (changedIds.length === 0) return;

      const loadedMainIds = new Set(this.items.map((item) => item.id));
      const loadedBacklogIds = new Set(this.backlogItems.map((item) => item.id));
      const loadedChangedIds = changedIds.filter(
        (id) => loadedMainIds.has(id) || loadedBacklogIds.has(id)
      );

      const hasNewVisibleItem = loadedChangedIds.length !== changedIds.length;
      const touchesBacklog = loadedChangedIds.some((id) => loadedBacklogIds.has(id));
      const usesServerOrderedItems =
        BOARD_VIEWS.has(this.#currentView) ||
        (LIST_VIEWS.has(this.#currentView) && (this.#sortBy || this.#sortDirection));
      if (hasNewVisibleItem || touchesBacklog || usesServerOrderedItems) {
        await this.refresh();
        return;
      }

      const updatedItems = await fetchItemsById(loadedChangedIds);
      for (const updated of updatedItems) {
        this.#applyUpdatedItem(updated);
      }
      // Nudge consumers that depend on array identity while preserving item
      // object identity for rows/cards that were patched in place.
      this.items = [...this.items];
      this.backlogItems = [...this.backlogItems];
    } catch (error) {
      if (!isExpectedBackgroundSyncError(error)) {
        console.error('[collectionStore] Delta refresh failed:', error);
      }
    }
  }

  async #primeChangesWatermark() {
    const loadId = this.#loadId;
    const wsId = this.#wsId;
    const colId = this.#colId;
    const changes = await fetchCollectionItemChanges(wsId, colId, {
      sub_ql: this.effectiveSubFilterQL || undefined,
    });
    if (loadId !== this.#loadId || wsId !== this.#wsId || colId !== this.#colId) return;
    this.#changesWatermark = changes?.watermark ?? 0;
  }

  #applyUpdatedItem(updated) {
    if (this.#hideCompletedItem(updated)) return;
    const idx = this.items.findIndex((i) => i.id === updated.id);
    if (idx !== -1) Object.assign(this.items[idx], updated);
    const bIdx = this.backlogItems.findIndex((i) => i.id === updated.id);
    if (bIdx !== -1) Object.assign(this.backlogItems[bIdx], updated);
    const searchIdx = this.boardSearchItems.findIndex((i) => i.id === updated.id);
    if (searchIdx !== -1) Object.assign(this.boardSearchItems[searchIdx], updated);
  }

  #hideCompletedItem(item) {
    if (this.showCompleted) return false;
    const status =
      workspaceDataStore.statuses.find((candidate) => candidate.id === item.status_id) ??
      this.boardStatuses.find((candidate) => candidate.id === item.status_id);
    if (!status?.is_completed) return false;
    this.#removeItemsById(new Set([item.id]));
    return true;
  }

  #removeItemsById(ids) {
    const beforeBacklog = this.backlogItems.length;
    const deferredSet = new Set(this.boardDeferred?.statusIds ?? []);
    let removedItems = 0;
    let removedDeferredItems = 0;
    this.items = this.items.filter((item) => {
      if (!ids.has(item.id)) return true;
      if (deferredSet.has(item.status_id)) {
        removedDeferredItems++;
      } else {
        removedItems++;
      }
      return false;
    });
    this.backlogItems = this.backlogItems.filter((item) => !ids.has(item.id));
    this.boardSearchItems = this.boardSearchItems.filter((item) => !ids.has(item.id));

    const removedBacklog = beforeBacklog - this.backlogItems.length;
    if (removedItems > 0 && this.itemsPagination) {
      this.itemsPagination = {
        ...this.itemsPagination,
        total_items: Math.max(0, (this.itemsPagination.total_items ?? 0) - removedItems),
      };
    }
    if (removedDeferredItems > 0 && this.boardDeferred) {
      const total = Math.max(0, this.boardDeferred.total - removedDeferredItems);
      const limit = this.boardDeferred.pagination?.limit ?? this.boardDeferred.limit;
      const pagination = this.boardDeferred.pagination
        ? {
            ...this.boardDeferred.pagination,
            total_items: total,
            total_pages: Math.ceil(total / limit),
          }
        : null;
      this.boardDeferred = {
        ...this.boardDeferred,
        total,
        pagination,
      };
      this.rightmostCap = this.boardDeferred.capped
        ? { statusIds: this.boardDeferred.statusIds, total }
        : null;
    }
    this.itemsHasMore = this.#hasMoreItems();
    if (removedBacklog > 0 && this.backlogPagination) {
      this.backlogPagination = {
        ...this.backlogPagination,
        total_items: Math.max(0, (this.backlogPagination.total_items ?? 0) - removedBacklog),
      };
      this.backlogHasMore = calcHasMore(this.backlogPagination);
    }
  }

  #itemSortOptions() {
    if (BOARD_VIEWS.has(this.#currentView)) {
      if (this.#boardSortMode === 'bubble') {
        return { order_by: 'last_active_at', sort_direction: 'desc' };
      }
      return { order_by: 'frac_index', sort_direction: 'asc' };
    }

    if (LIST_VIEWS.has(this.#currentView)) {
      const opts = {};
      if (this.#sortBy) opts.order_by = this.#sortBy;
      if (this.#sortDirection) opts.sort_direction = this.#sortDirection;
      return opts;
    }

    return {};
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

/** Poll for cheap collection deltas and patch loaded items when safe */
export function refreshCollectionDeltas() {
  return collectionStore.refreshDeltas();
}

/** Refresh a single item in the store without reloading the entire collection */
export function refreshCollectionItem(itemId) {
  return collectionStore.refreshItem(itemId);
}

/** Apply a mutation response directly to the loaded collection page. */
export function applyCollectionItem(item) {
  collectionStore.applyItem(item);
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
