import { collections } from '../api/collections.js';
import { items as itemsApi } from '../api/items.js';
import { BACKGROUND_READ, FOREGROUND_READ } from '../api/readScheduler.js';
import {
  RIGHTMOST_COLUMN_LIMIT,
  rightmostCapStatusIds,
} from '../features/collections/boardColumns.js';
import {
  fetchCollectionBacklog,
  fetchCollectionItemChanges,
  fetchCollectionItems,
  fetchCollectionTotal,
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
// Tree pages stay smaller than map/roadmap: every loaded item can require an
// ancestors lookup, so the page bounds the hierarchy-repair fan-out.
const TREE_PAGE_SIZE = 100;
const LARGE_COLLECTION_PAGE_SIZE = 250;
const BOARD_UNFINISHED_PAGE_SIZE = 1000;
const BOARD_UNTHROTTLED_ITEM_COUNT = 1000;
const BOARD_BACKGROUND_PAGE_DELAY_MS = 250;
const SEARCH_PAGE_SIZE = 100;
// Local refresh-work-items bursts coalesce into one authoritative delta
// reconciliation instead of one visibility query per item.
const LOCAL_RECONCILE_DEBOUNCE_MS = 200;

function delay(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function appendUniqueItems(existing, incoming) {
  const ids = new Set(existing.map((item) => item.id));
  return [...existing, ...incoming.filter((item) => !ids.has(item.id))];
}

function initialItemsPageSize(view) {
  if (view === 'workspace-list' || view === 'collection-list') return LIST_INITIAL_PAGE_SIZE;
  if (view === 'workspace-tree' || view === 'collection-tree') return TREE_PAGE_SIZE;
  if (
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
  collectionTotal = $state(null);
  #countRequestId = 0;
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

  // View search is a separate server-scoped result set, shared by the board
  // and list views. Keeping it apart prevents a search from changing the
  // view's normal pagination or completed-item trimming.
  searchItems = $state([]);
  searchPagination = $state(null);
  searchLoading = $state(false);
  searchLoadingMore = $state(false);
  searchError = $state(false);

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
    this.clearSearch();
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
  #searchId = 0;
  #searchQuery = '';
  #reconcileTimer = null;
  #reconcileInFlight = null;
  #reconcilePending = false;

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
      this.collectionTotal = null;
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
      this.clearSearch();
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

      const [itemsResult, backlogResult, deferredResult, countResult] = await Promise.all([
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
              requestOptions: FOREGROUND_READ,
            })
          : Promise.resolve(null),
        loadsItems(view)
          ? this.#fetchBoardDeferredItems(wsId, colId, boardPartition, collection)
          : Promise.resolve(null),
        this.#refreshCollectionTotal(),
      ]);

      if (loadId !== this.#loadId) return; // stale

      this.#applyResults({
        itemsResult,
        backlogResult,
        deferredResult,
        countResult,
        boardPartition,
        collection,
      });
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
      // Page teardown and session replacement abort in-flight reads; that is
      // expected control flow, not a load failure to surface.
      if (!isExpectedBackgroundSyncError(error)) {
        console.error('[collectionStore] Load failed:', error);
      }
    } finally {
      if (loadId === this.#loadId) {
        this.loading = false;
      }
      // Local events that arrived during the initial load get one trailing run.
      if (this.#reconcilePending) {
        this.#reconcilePending = false;
        void this.refreshDeltas();
      }
    }
  }

  /**
   * Apply one round of fetched results to store state. Load and refresh share
   * this so their state shapes cannot drift apart.
   */
  #applyResults({
    itemsResult,
    backlogResult,
    deferredResult,
    countResult,
    boardPartition,
    collection,
  }) {
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
    this.#changesWatermark = snapshotWatermark(
      itemsResult,
      backlogResult,
      deferredResult,
      countResult
    );
  }

  async #refreshCollectionTotal() {
    if (!this.#wsId && !this.#colId) return null;
    const requestId = ++this.#countRequestId;
    const loadId = this.#loadId;
    try {
      const result = await fetchCollectionTotal(this.#wsId, this.#colId, FOREGROUND_READ);
      if (requestId === this.#countRequestId && loadId === this.#loadId) {
        this.collectionTotal = result.total;
        this.#changesWatermark = minimumWatermark(this.#changesWatermark, result.watermark);
      }
      return result;
    } catch (error) {
      if (requestId !== this.#countRequestId || loadId !== this.#loadId) return;
      this.collectionTotal = null;
      if (!isExpectedBackgroundSyncError(error)) {
        console.error('[collectionStore] Count refresh failed:', error);
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
    const request = collections
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

  /**
   * Non-starting view of the board-configuration request for a scope. Returns
   * the cached config, the in-flight bootstrap promise, or null when this store
   * has not loaded that scope. Navigation joins this instead of issuing a
   * duplicate plain-config request.
   */
  peekBoardConfiguration(wsId = this.#wsId, colId = this.#colId) {
    const key = `${colId ?? ''}|${wsId ?? ''}`;
    if (this.#boardConfigurationKey !== key) return null;
    if (this.#boardConfigurationLoaded) return { config: this.boardConfiguration };
    if (this.#boardConfigurationPromise) return { pending: this.#boardConfigurationPromise };
    return null;
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
        requestOptions: FOREGROUND_READ,
        ...this.#itemSortOptions(),
        ...this.#boardExclusionFilter(boardPartition?.statusIds ?? null),
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
          requestOptions: BACKGROUND_READ,
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

  // The backlog API clamps page_size to 100, so a single request cannot return
  // an accumulated backlog larger than that. Fetch the pages needed to cover
  // the loaded range and merge them.
  async #fetchCompleteBacklog(wsId, colId, limit, subQL, collection, loadId) {
    const first = await fetchCollectionBacklog(wsId, colId, {
      page: 1,
      limit,
      sub_ql: subQL,
      collection,
      requestOptions: FOREGROUND_READ,
    });
    const totalPages = first.pagination?.total_pages ?? 1;
    const pages = [first];
    let loadedCount = first.items.length;
    for (let page = 2; loadedCount < limit && page <= totalPages; page++) {
      if (loadId !== this.#loadId) return first;
      const result = await fetchCollectionBacklog(wsId, colId, {
        page,
        limit,
        sub_ql: subQL,
        collection,
        requestOptions: BACKGROUND_READ,
      });
      pages.push(result);
      if (result.items.length === 0) break;
      loadedCount += result.items.length;
    }
    return {
      ...first,
      items: pages.flatMap((result) => result.items),
      pagination: pages.at(-1).pagination,
      watermark: snapshotWatermark(...pages),
    };
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
        requestOptions: BACKGROUND_READ,
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
      requestOptions: FOREGROUND_READ,
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

  get searchHasMore() {
    return calcHasMore(this.searchPagination);
  }

  get searchRemainingCount() {
    const loaded = this.searchItems.length;
    return Math.max(0, (this.searchPagination?.total_items ?? loaded) - loaded);
  }

  clearSearch() {
    this.#searchId++;
    this.#searchQuery = '';
    this.searchItems = [];
    this.searchPagination = null;
    this.searchLoading = false;
    this.searchLoadingMore = false;
    this.searchError = false;
  }

  async runSearch(query) {
    const normalized = query.trim();
    if (!normalized) {
      this.clearSearch();
      return;
    }

    const searchId = ++this.#searchId;
    this.#searchQuery = normalized;
    this.searchItems = [];
    this.searchPagination = null;
    this.searchLoading = true;
    this.searchError = false;

    try {
      const result = await this.#fetchSearchPage(normalized, 1);
      if (searchId !== this.#searchId || normalized !== this.#searchQuery) return;
      this.searchItems = result.items;
      this.searchPagination = result.pagination;
    } catch (error) {
      if (searchId !== this.#searchId) return;
      console.error('[collectionStore] search failed:', error);
      this.searchError = true;
    } finally {
      if (searchId === this.#searchId) {
        this.searchLoading = false;
      }
    }
  }

  async loadMoreSearchItems() {
    if (!this.#searchQuery || !this.searchHasMore || this.searchLoadingMore) return;

    const searchId = ++this.#searchId;
    const query = this.#searchQuery;
    const nextPage = (this.searchPagination?.page ?? 0) + 1;
    this.searchLoadingMore = true;
    this.searchError = false;

    try {
      const result = await this.#fetchSearchPage(query, nextPage);
      if (searchId !== this.#searchId || query !== this.#searchQuery) return;
      this.searchItems = [...this.searchItems, ...result.items];
      this.searchPagination = result.pagination;
    } catch (error) {
      if (searchId !== this.#searchId) return;
      console.error('[collectionStore] loading more search results failed:', error);
      this.searchError = true;
    } finally {
      if (searchId === this.#searchId) {
        this.searchLoadingMore = false;
      }
    }
  }

  #fetchSearchPage(query, page) {
    return fetchCollectionItems(this.#wsId, this.#colId, {
      page,
      limit: SEARCH_PAGE_SIZE,
      search: query,
      sub_ql: this.effectiveSubFilterQL || undefined,
      collection: this.boardCollection ?? undefined,
      requestOptions: FOREGROUND_READ,
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
        requestOptions: BACKGROUND_READ,
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
        requestOptions: BACKGROUND_READ,
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
      const [result] = await Promise.all([
        fetchCollectionItems(this.#wsId, this.#colId, {
          page,
          limit,
          sub_ql: this.effectiveSubFilterQL || undefined,
          requestOptions: FOREGROUND_READ,
          ...this.#itemSortOptions(),
        }),
        this.#refreshCollectionTotal(),
      ]);

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
    if (!this.#wsId && !this.#colId) return false;
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
      if (loadId !== this.#loadId) return false;

      const deferredLoadedCount = this.items.length - this.mainItemsLoadedCount;
      const [itemsResult, backlogResult, deferredResult, countResult] = await Promise.all([
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
          ? this.#fetchCompleteBacklog(
              this.#wsId,
              this.#colId,
              backlogLimit,
              this.effectiveSubFilterQL || undefined,
              collection,
              loadId
            )
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
        this.#refreshCollectionTotal(),
      ]);
      if (loadId !== this.#loadId) return false;

      this.#applyResults({
        itemsResult,
        backlogResult,
        deferredResult,
        countResult,
        boardPartition,
        collection,
      });
      return true;
    } catch (error) {
      if (loadId !== this.#loadId) return false;
      if (!isExpectedBackgroundSyncError(error)) {
        console.error('[collectionStore] Refresh failed:', error);
      }
      return false;
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
      const updated = await itemsApi.get(itemId);
      this.#applyUpdatedItem(updated);
      await this.#refreshCollectionTotal();
    } catch (e) {
      console.error('[collectionStore] refreshItem failed:', e);
    }
  }

  applyItem(item) {
    if (!item?.id) return;
    void this.#refreshCollectionTotal();
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
   * Insert a just-created item into the loaded projection. `backlog` targets
   * the backlog array, otherwise the main items array. Deduplicates by id so
   * repeated create events (modal plus delta or cross-tab frame) cannot append
   * the same row twice.
   */
  insertCreatedItem(item, { backlog = false } = {}) {
    if (!item?.id) return;
    const target = backlog ? this.backlogItems : this.items;
    if (target.some((existing) => existing.id === item.id)) return;
    if (backlog) this.backlogItems = [...this.backlogItems, item];
    else this.items = [...this.items, item];
  }

  /**
   * Merge a patch into every loaded projection that holds the item. This is the
   * only path components use to reflect a confirmed mutation. Update-only: a
   * missing row is off-page or filtered out, so the next reconciliation owns it.
   */
  patchItem(itemId, patch) {
    if (itemId == null || !patch) return;
    const apply = (list) => {
      let found = false;
      const next = list.map((item) => {
        if (item.id !== itemId) return item;
        found = true;
        return { ...item, ...patch };
      });
      return found ? next : list;
    };
    const items = apply(this.items);
    if (items !== this.items) this.items = items;
    const backlog = apply(this.backlogItems);
    if (backlog !== this.backlogItems) this.backlogItems = backlog;
    const search = apply(this.searchItems);
    if (search !== this.searchItems) this.searchItems = search;
  }

  /**
   * Move a loaded backlog row to the start or end of the backlog list after the
   * server confirmed the new rank.
   */
  moveBacklogItemToBoundary(item, boundary) {
    if (!item?.id) return;
    const others = this.backlogItems.filter((existing) => existing.id !== item.id);
    this.backlogItems = boundary === 'start' ? [item, ...others] : [...others, item];
  }

  /**
   * Replace the cached board configuration for a scope after a successful save.
   * A scope that is not currently loaded is left alone; its next load fetches
   * the saved config.
   */
  setBoardConfiguration(config, workspaceId = this.#wsId, collectionId = this.#colId) {
    const key = `${collectionId ?? ''}|${workspaceId ?? ''}`;
    if (this.#boardConfigurationKey !== key) return;
    this.boardConfiguration = config;
    this.#boardConfigurationLoaded = true;
  }

  /**
   * The single reconciliation entry point. Local events, SSE invalidations and
   * cross-tab broadcasts all funnel here; callers must not fetch deltas on
   * their own. Single-flight: concurrent requests join the active run, and a
   * request that arrives during a run or the initial load is replayed exactly
   * once after it settles.
   */
  refreshDeltas() {
    if (this.loading || this.#reconcileInFlight) {
      this.#reconcilePending = true;
      return this.#reconcileInFlight ?? Promise.resolve(false);
    }
    const run = this.#fetchDeltas();
    this.#reconcileInFlight = run;
    const settle = () => {
      if (this.#reconcileInFlight === run) this.#reconcileInFlight = null;
      if (this.#reconcilePending) {
        this.#reconcilePending = false;
        void this.refreshDeltas();
      }
    };
    void run.then(settle, settle);
    return run;
  }

  /**
   * Fetch cheap deltas and patch loaded rows by ID. Falls back to a full
   * refresh when the delta implies structural uncertainty (new visible item,
   * server-side sort, or backlog membership changes).
   */
  async #fetchDeltas() {
    if (!this.#wsId && !this.#colId) return false;

    const loadId = this.#loadId;
    try {
      if (this.#changesWatermark === null) {
        await this.#primeChangesWatermark();
        return true;
      }

      const changes = await fetchCollectionItemChanges(this.#wsId, this.#colId, {
        since: this.#changesWatermark,
        sub_ql: this.effectiveSubFilterQL || undefined,
      });
      if (loadId !== this.#loadId) return false;
      const nextWatermark = changes?.watermark ?? this.#changesWatermark;

      if (changes?.requires_full_reload) {
        return await this.refresh();
      }

      const removedIds = new Set(changes?.removed_item_ids ?? []);
      if (removedIds.size > 0) {
        // Removed IDs may be off-page or outside the saved query altogether.
        return await this.refresh();
      }

      const changedIds = [...new Set(changes?.changed_item_ids ?? [])].filter(
        (id) => !removedIds.has(id)
      );
      if (changedIds.length === 0) {
        // Nothing to patch; advancing the cursor is safe.
        this.#changesWatermark = nextWatermark;
        return true;
      }

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
        return await this.refresh();
      }

      const updatedItems = await fetchItemsById(loadedChangedIds);
      if (loadId !== this.#loadId) return false;
      for (const updated of updatedItems) {
        this.#applyUpdatedItem(updated);
      }
      // Nudge consumers that depend on array identity while preserving item
      // object identity for rows/cards that were patched in place.
      this.items = [...this.items];
      this.backlogItems = [...this.backlogItems];
      await this.#refreshCollectionTotal();
      // Commit the cursor only after the patch applied successfully so a
      // failure preserves the previous cursor for replay.
      this.#changesWatermark = nextWatermark;
      return true;
    } catch (error) {
      if (!isExpectedBackgroundSyncError(error)) {
        console.error('[collectionStore] Delta refresh failed:', error);
      }
      return false;
    }
  }

  /**
   * Debounce local refresh-work-items, SSE and cross-tab bursts into one
   * reconciliation. refreshDeltas() remains the single owner that serializes
   * runs and retains a trailing run.
   */
  scheduleLocalReconciliation() {
    if (this.#reconcileTimer) clearTimeout(this.#reconcileTimer);
    this.#reconcileTimer = setTimeout(() => {
      this.#reconcileTimer = null;
      void this.refreshDeltas();
    }, LOCAL_RECONCILE_DEBOUNCE_MS);
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
    const searchIdx = this.searchItems.findIndex((i) => i.id === updated.id);
    if (searchIdx !== -1) Object.assign(this.searchItems[searchIdx], updated);
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
    this.searchItems = this.searchItems.filter((item) => !ids.has(item.id));

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
    if (this.#reconcileTimer) {
      clearTimeout(this.#reconcileTimer);
      this.#reconcileTimer = null;
    }
  }
}

export const collectionStore = new CollectionStore();

/** Trigger a background refresh preserving current pagination */
export function reloadCollection() {
  collectionStore.refresh();
}

/** Coalesce local refresh-work-items, SSE and cross-tab events into one reconciliation. */
export function scheduleCollectionReconciliation() {
  collectionStore.scheduleLocalReconciliation();
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
