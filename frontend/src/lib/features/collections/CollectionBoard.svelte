<script>
  import { onDestroy, onMount, untrack } from 'svelte';
  import { useEventListener } from 'runed';
  import { t } from '../../stores/i18n.svelte.js';
  import { api } from '../../api.js';
  import { navigate } from '../../router.js';
  import { collectionStore, refreshCollectionDeltas, reloadCollection } from '../../stores/collectionContext.js';
  import { useGradientStyles, loadWorkspaceGradient } from '../../stores/workspaceGradient.svelte.js';
  import QuickAddForm from './QuickAddForm.svelte';
  import { getCollection, checkItemVisibility } from './collectionService.js';
  import {
    RIGHTMOST_COLUMN_LIMIT,
    boardStatusIdForItem,
    buildDisplayColumns,
    statusIdForBoardColumnMove,
  } from './boardColumns.js';
  import { infoToast, successToast, warningToast } from '../../stores/toasts.svelte.js';
  import { Plus, ChevronDown, ChevronRight, MoreHorizontal, Layers, ArrowDownUp } from '@lucide/svelte';
  import ItemPicker from '../../pickers/ItemPicker.svelte';
  import { buildIterationPickerConfig } from '../iterations/iterationPickerUtils.js';
  import ItemTypeIcon from '../../components/ItemTypeIcon.svelte';
  import { draggable, dropTargetForElements } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import { autoScrollWindowForElements } from '@atlaskit/pragmatic-drag-and-drop-auto-scroll/element';
  import { attachClosestEdge, extractClosestEdge } from '@atlaskit/pragmatic-drag-and-drop-hitbox/closest-edge';
  import ItemDetail from '../items/ItemDetail.svelte';
  import PersonalTaskDetail from '../personal/PersonalTaskDetail.svelte';
  import ViewHeader from '../../layout/ViewHeader.svelte';
  import StaticViewBackground from '../../layout/StaticViewBackground.svelte';
  import Button from '../../components/Button.svelte';
  import SearchInput from '../../components/SearchInput.svelte';
  import SubFilterBar from './SubFilterBar.svelte';
  import BoardColumn from './BoardColumn.svelte';
  import BoardEmptyState from './BoardEmptyState.svelte';
  import BoardItemCard from './BoardItemCard.svelte';
  import ItemKey from '../items/ItemKey.svelte';
  import CollectionViewSwitcher from './CollectionViewSwitcher.svelte';
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
  import { backlogStore, workspaceDataStore, workspacesStore } from '../../stores/index.js';
  import { useWorkItemPoller } from '../../composables/useWorkItemPoller.svelte.js';
  import { agentRuns } from '../../stores/agentRuns.svelte.js';
  import { getVisibleColor, hexToRgb } from '../../utils/colorUtils.js';
  import { showCreatedItemToast } from '../../utils/createdItemToast.js';
  import {
    childItemTypesForParent,
    isGenericSubtaskType,
    sortItemTypesByHierarchy,
  } from '../../utils/hierarchy.js';
  import { objectDisplayName } from '../../utils/systemLabels.js';

  // Props
  let { workspaceId, collectionId = null } = $props();

  // Reference data from shared workspace store
  let workspace = $derived(workspaceDataStore.workspace);
  let itemTypes = $derived(workspaceDataStore.itemTypes);
  let statuses = $derived(workspaceDataStore.statuses);
  let users = $derived(workspaceDataStore.users);
  let priorities = $derived(workspaceDataStore.priorities);
  let milestones = $derived(workspaceDataStore.milestones);
  let iterations = $derived(workspaceDataStore.iterations);
  let wdsLabels = $derived(workspaceDataStore.labels);
  let projects = $derived(workspaceDataStore.projects);
  let customFieldDefinitions = $derived(workspaceDataStore.customFieldDefinitions);

  let searchQuery = $state('');
  let searchDebouncing = $state(false);
  let searchEffectId = 0;
  let searchActive = $derived(Boolean(searchQuery.trim()));
  let activeItemsHasMore = $derived(
    searchActive ? collectionStore.boardSearchHasMore : collectionStore.itemsHasMore
  );
  let activeItemsLoadingMore = $derived(
    searchActive ? collectionStore.boardSearchLoadingMore : collectionStore.itemsLoadingMore
  );
  let activeItemsRemainingCount = $derived(
    searchActive ? collectionStore.boardSearchRemainingCount : collectionStore.itemsRemainingCount
  );

  // Dynamic view-specific state — board searches use their own scoped server
  // result set so normal board pagination and completed-item trimming remain
  // unchanged when the query is cleared.
  let items = $derived(searchActive ? collectionStore.boardSearchItems : collectionStore.items);
  let transitions = $state([]);
  let boardConfig = $state(null);
  let cardFields = $derived((boardConfig?.card_fields || []).slice().sort((a, b) => a.display_order - b.display_order));

  let loading = $state(true);
  let currentCollectionName = $derived(collectionStore.collectionName);
  let setupTimeout;
  let autoScrollCleanup;
  let setupElements = new Map(); // Track which elements have drag/drop set up and their cleanup functions
  let pendingDrops = new Set(); // Track pending drop operations to prevent duplicates
  let showItemModal = $state(false);
  let selectedItemId = $state(null);

  // Cached outgoing and incoming links for dependency hover summaries.
  let dependencyLinksByItem = $state({});
  let dependencyLinksToken = 0; // guards against stale async when items change
  const DEPENDENCY_LINK_CHUNK = 200; // ids per batched /links/batch request (server cap 500)

  // Quick-add state per column
  let quickAddState = $state({});
  let workspaces = $derived($workspacesStore.regularWorkspaces || []);
  let personalWorkspaceIds = $derived(new Set(
    ($workspacesStore.allWorkspaces || [])
      .filter((candidate) => candidate.is_personal)
      .map((candidate) => Number(candidate.id))
  ));
  let selectedItem = $derived(items.find((item) => item.id === selectedItemId) ?? null);
  let collectionAllowsAllWorkspaces = $derived(
    collectionStore.boardWorkspaceScopeLoaded &&
    !workspaceId &&
    Boolean(collectionId) &&
    !collectionStore.boardCollection?.workspace_id &&
    !collectionStore.boardCollection?.ql_query?.trim()
  );
  let availableWorkspaces = $derived(
    !collectionStore.boardWorkspaceScopeLoaded
      ? []
      : collectionAllowsAllWorkspaces
        ? workspaces
        : workspaces.filter(workspace => collectionStore.boardWorkspaceIds.includes(workspace.id))
  );
  const quickAddItemTypesByWorkspace = new Map();
  let quickAddTypeLoadToken = 0;

  // Backlog functionality
  let backlogItems = $derived(collectionStore.backlogItems);

  // Iteration filter state
  let allIterations = $state([]);
  let iterationFilterId = $state(null);

  // Swimlane grouping state
  let groupByItemTypeId = $state(null);
  let excludeRightmostSwimlaneParents = $state(false);
  let swimlaneCollapsed = $state({});

  // Per-board collapsed-column preferences.
  let collapsedColumns = $state({});

  // Rank enables manual ordering; bubble prioritizes recent activity.
  let sortMode = $state('rank');

  // Edge-based drag state
  let dragState = $state(new Map()); // Track drag state for each item: { isDragging: boolean, closestEdge: 'top'|'bottom'|null }
  let boardAnnouncement = $state('');
  let boardViewElement = $state(null);

  // Centralized gradient styling
  const styles = useGradientStyles();

  $effect(() => {
    const query = searchQuery.trim();
    const scope = `${workspaceId ?? ''}|${collectionId ?? ''}|${collectionStore.subFilterQL}`;
    const effectId = ++searchEffectId;
    collectionStore.clearBoardSearch();

    if (!query) {
      searchDebouncing = false;
      return;
    }

    searchDebouncing = true;
    const timer = setTimeout(async () => {
      await collectionStore.searchBoardItems(query);
      if (
        effectId === searchEffectId &&
        query === searchQuery.trim() &&
        scope === `${workspaceId ?? ''}|${collectionId ?? ''}|${collectionStore.subFilterQL}`
      ) {
        searchDebouncing = false;
      }
    }, 250);

    return () => clearTimeout(timer);
  });

  onDestroy(() => {
    collectionStore.clearBoardSearch();
    autoScrollCleanup?.();
  });

  // Listen for newly created items
  async function handleRefreshWorkItems(event) {
    if (event.detail?.itemId) {
      try {
        const newItem = await api.items.get(event.detail.itemId);
        // Collection membership may span workspaces; verify it server-side.
        const belongsToView = collectionId
          ? await checkItemVisibility(newItem.id, { collection_id: collectionId })
          : Number(newItem.workspace_id) === Number(workspaceId);
        if (belongsToView) {
          if (newItem.status_id) {
            // Item has a status, add it to the board (at the end, since board is ordered by rank)
            collectionStore.items = [...collectionStore.items, newItem];
          } else {
            // Item has no status, add it to backlog (at the end)
            collectionStore.backlogItems = [...collectionStore.backlogItems, newItem];
          }
          // Re-setup drag and drop for the new item
          setTimeout(() => {
            setupDragAndDrop();
          }, 100);
        }
      } catch (error) {
        console.error('Failed to load new item:', error);
      }
    }
  }

  useEventListener(() => window, 'refresh-work-items', handleRefreshWorkItems);

  // Quick-add functions

  // Children in a swimlane must be exactly one hierarchy level below its parent.
  function quickAddTypesFor(parentItem, sourceTypes = itemTypes) {
    const parentType = parentItem?.item_type_id
      ? (sourceTypes || []).find(type => type.id === parentItem.item_type_id)
      : null;
    const candidates = parentType
      ? childItemTypesForParent(sourceTypes, parentType)
      : (sourceTypes || []).filter((type) => !isGenericSubtaskType(type));
    return candidates.slice().sort((a, b) => (a.sort_order ?? 0) - (b.sort_order ?? 0));
  }

  function preferredQuickAddTypeId(availableTypes) {
    let preferredId = availableTypes[0]?.id ?? null;
    try {
      const savedId = parseInt(localStorage.getItem('board-quickadd-last-item-type-id') || '', 10);
      if (savedId && availableTypes.some(type => type.id === savedId)) {
        preferredId = savedId;
      }
    } catch (e) { /* ignore storage errors */ }
    return preferredId;
  }

  async function loadQuickAddItemTypes(quickAddKey, selectedWorkspaceId) {
    const state = quickAddState[quickAddKey];
    if (!state || !selectedWorkspaceId) return;

    const numericWorkspaceId = Number(selectedWorkspaceId);
    const loadToken = ++quickAddTypeLoadToken;
    state.typeLoadToken = loadToken;
    state.loadingTypes = true;
    state.availableTypes = [];
    state.itemTypeId = null;

    try {
      let workspaceItemTypes = quickAddItemTypesByWorkspace.get(numericWorkspaceId);
      if (!workspaceItemTypes) {
        const currentWorkspaceId = Number(workspaceId);
        const storeWorkspaceId = Number(workspaceDataStore.workspaceId);
        if (currentWorkspaceId === numericWorkspaceId && storeWorkspaceId === numericWorkspaceId) {
          workspaceItemTypes = itemTypes || [];
        } else {
          workspaceItemTypes = await api.workspaces.getItemTypes(numericWorkspaceId);
        }
        quickAddItemTypesByWorkspace.set(numericWorkspaceId, workspaceItemTypes || []);
      }

      const currentState = quickAddState[quickAddKey];
      if (
        !currentState ||
        currentState.typeLoadToken !== loadToken ||
        Number(currentState.workspaceId) !== numericWorkspaceId
      ) return;

      const availableTypes = quickAddTypesFor(currentState.parentItem, workspaceItemTypes);
      currentState.availableTypes = availableTypes;
      currentState.itemTypeId = preferredQuickAddTypeId(availableTypes);
      currentState.error = availableTypes.length > 0
        ? null
        : 'No item types are available for this workspace';
    } catch (error) {
      const currentState = quickAddState[quickAddKey];
      if (!currentState || currentState.typeLoadToken !== loadToken) return;
      console.error('Failed to load workspace item types:', error);
      currentState.error = 'Failed to load item types for this workspace';
    } finally {
      const currentState = quickAddState[quickAddKey];
      if (currentState?.typeLoadToken === loadToken) {
        currentState.loadingTypes = false;
      }
    }
  }

  function initQuickAdd(columnId, statusId, quickAddKey = columnId, parentItem = null) {
    const parentId = parentItem?.id ?? null;

    const preselectedWorkspaceId = parentItem?.workspace_id
      ?? (availableWorkspaces.length === 1 ? availableWorkspaces[0].id : null);

    quickAddState[quickAddKey] = {
      show: true,
      workspaceId: preselectedWorkspaceId,
      itemTypeId: null,
      availableTypes: [],
      loadingTypes: Boolean(preselectedWorkspaceId),
      statusId,
      parentId,
      parentItem,
      title: '',
      error: null
    };

    if (preselectedWorkspaceId) {
      void loadQuickAddItemTypes(quickAddKey, preselectedWorkspaceId);
    }

    setTimeout(() => {
      const textarea = /** @type {HTMLTextAreaElement | null} */ (document.querySelector(`textarea[data-quick-add-parent="${quickAddKey}"]`));
      if (textarea) textarea.focus();
    }, 0);
  }

  function cancelQuickAdd(quickAddKey) {
    delete quickAddState[quickAddKey];
  }

  function updateQuickAddField(quickAddKey, field, value) {
    if (quickAddState[quickAddKey]) {
      quickAddState[quickAddKey][field] = value;
      quickAddState[quickAddKey].error = null;
      if (field === 'workspaceId') {
        void loadQuickAddItemTypes(quickAddKey, value);
      }
    }
  }

  async function createColumnItem(quickAddKey) {
    const state = quickAddState[quickAddKey];
    if (!state) return;

    if (!state.workspaceId) {
      quickAddState[quickAddKey].error = 'Please select a workspace';
      return;
    }
    if (state.loadingTypes) {
      quickAddState[quickAddKey].error = 'Item types are still loading';
      return;
    }
    if (!state.itemTypeId) {
      quickAddState[quickAddKey].error = 'Please select an item type';
      return;
    }
    if (!state.title?.trim()) {
      quickAddState[quickAddKey].error = 'Please enter a title';
      return;
    }

    try {
      const payload = {
        workspace_id: state.workspaceId,
        item_type_id: state.itemTypeId,
        title: state.title.trim(),
        description: '',
        status_id: state.statusId
      };
      if (state.parentId) {
        payload.parent_id = state.parentId;
      }

      const newItem = await api.items.create(payload);

      // A collection can span several allowed workspaces while applying other
      // filters. Only add the new item locally when it matches the full view.
      let belongsToView = true;
      if (collectionId) {
        const collection = await getCollection(collectionId);
        if (collection) {
          belongsToView = await checkItemVisibility(newItem.id, { collection_id: collectionId });
        }
      } else if (workspaceId) {
        belongsToView = Number(newItem.workspace_id) === Number(workspaceId);
      }

      if (!belongsToView) {
        const selectedWorkspace = availableWorkspaces.find(w => w.id === state.workspaceId);
        const workspaceName = selectedWorkspace?.name || t('collections.anotherWorkspace');
        const reason = collectionId
          ? t('collections.collectionFiltersReason')
          : t('collections.workspaceFilterReason');
        infoToast(
          t('collections.cardCreatedOutsideView', { workspace: workspaceName, reason }),
          t('collections.cardCreatedSuccess')
        );
      }

      try {
        localStorage.setItem('board-quickadd-last-item-type-id', String(state.itemTypeId));
      } catch (e) { /* ignore storage errors */ }

      if (belongsToView) {
        // The create endpoint returns the complete permission-masked item, so it
        // can be added directly without an immediate GET of the same item.
        collectionStore.items = [...collectionStore.items, newItem];
        setTimeout(() => setupDragAndDrop(), 100);

        // Toast feedback
        showCreatedItemToast(newItem);
        if (collectionStore.itemsHasMore) {
          warningToast(t('collections.boardItemsTruncated'));
        }
      }

      cancelQuickAdd(quickAddKey);
    } catch (error) {
      console.error('Failed to create item:', error);
      quickAddState[quickAddKey].error = 'Failed to create item: ' + (error.message || error);
    }
  }

  onMount(async () => {
    autoScrollCleanup = autoScrollWindowForElements({
      canScroll: ({ source }) =>
        source.data.type === 'work-item' && Boolean(boardViewElement?.contains(source.element)),
      getAllowedAxis: () => 'horizontal',
    });

    await Promise.all([
      workspaceId ? loadWorkspaceGradient(workspaceId) : Promise.resolve(),
      workspaceId
        ? workspaceDataStore.initialize(workspaceId)
        : workspaceDataStore.initializeGlobal(),
      workspacesStore.load(),
    ]);
    loading = false;
  });

  async function loadWorkspaceBoardState(requestedView, requestedWorkspaceId) {
    if (!requestedWorkspaceId) return;
    await workspaceDataStore.initialize(requestedWorkspaceId);
    if (requestedView !== viewSignature) return;

    try {
      const iters = await api.iterations.getAll({
        workspace_id: requestedWorkspaceId,
        include_global: !workspaceDataStore.workspace?.is_personal,
      });
      if (requestedView !== viewSignature) return;
      allIterations = iters || [];

      const saved = localStorage.getItem(`board-iteration-filter-${requestedWorkspaceId}`);
      if (saved) {
        const id = parseInt(saved, 10);
        if (allIterations.some((iteration) => iteration.id === id)) {
          iterationFilterId = id;
        } else {
          localStorage.removeItem(`board-iteration-filter-${requestedWorkspaceId}`);
        }
      }
    } catch (error) {
      if (error?.name !== 'AbortError' && requestedView === viewSignature) {
        console.error('Failed to load iterations:', error);
      }
    }
  }

  $effect(() => {
    if (groupByItemTypeId && itemTypes.length > 0 && !itemTypes.some(type => type.id === groupByItemTypeId)) {
      setGroupByItemType(null);
    }
  });

  // Keep backlog count in sync
  $effect(() => {
    backlogStore.setCount(workspaceId, collectionStore.backlogPagination?.total ?? collectionStore.backlogItems.length);
  });

  // Reset dependency links when the viewed board changes.
  let viewSignature = $derived(`${collectionId ?? ''}|${workspaceId ?? ''}`);
  $effect(() => {
    // Board configuration depends on the view, not the loaded item set.
    viewSignature;
    dependencyLinksByItem = {};
    allIterations = [];
    iterationFilterId = null;
    groupByItemTypeId = null;
    excludeRightmostSwimlaneParents = false;
    swimlaneCollapsed = {};
    try {
      const savedGroupBy = localStorage.getItem(groupByStorageKey());
      const savedGroupById = savedGroupBy ? parseInt(savedGroupBy, 10) : null;
      if (savedGroupById) groupByItemTypeId = savedGroupById;
      const savedExcludeRightmost = localStorage.getItem(excludeRightmostSwimlaneParentsStorageKey());
      if (savedExcludeRightmost !== null) {
        excludeRightmostSwimlaneParents = savedExcludeRightmost === 'true';
      }
    } catch (e) { /* ignore storage errors */ }
    // MainApp reuses this component across boards, so restore per-view preferences here.
    collapsedColumns = loadCollapsedColumns();
    sortMode = loadSortMode();
    // Set sort mode before the store selects page-one items.
    untrack(() => collectionStore.setBoardSortMode(sortMode));
    if (collectionId || workspaceId) {
      loadBoardConfig();
    }
    if (workspaceId) {
      untrack(() => {
        loadWorkspaceBoardState(viewSignature, workspaceId);
      });
    }
  });

  // Preload dependency links when the loaded item set changes.
  $effect(() => {
    if (collectionStore.items.length > 0 && !collectionStore.loading) {
      // Avoid subscribing this effect to cache writes.
      untrack(() => loadDependencyLinksForItems(collectionStore.items));
    }
  });

  // Fetch and cache merged links for dependency summaries.
  async function loadDependencyLinksForItems(items) {
    const toFetch = items.filter((i) => i?.id != null && !dependencyLinksByItem[i.id]);
    if (toFetch.length === 0) return;
    const token = ++dependencyLinksToken;
    const ids = toFetch.map((i) => i.id);
    // Batch below the server's 500-ID cap.
    const chunks = [];
    for (let i = 0; i < ids.length; i += DEPENDENCY_LINK_CHUNK) {
      chunks.push(ids.slice(i, i + DEPENDENCY_LINK_CHUNK));
    }
    const groupsPerChunk = await Promise.all(
      chunks.map(async (chunk) => {
        try {
          return await api.links.getForItems(chunk);
        } catch {
          return {};
        }
      })
    );
    if (token !== dependencyLinksToken) return; // a newer load superseded us
    const next = { ...dependencyLinksByItem };
    // Seed every requested id so items with no links are cached and not re-fetched.
    for (const id of ids) next[id] = next[id] ?? [];
    for (const groups of groupsPerChunk) {
      for (const [id, group] of Object.entries(groups)) {
        const all = [];
        if (group?.outgoing) all.push(...group.outgoing);
        if (group?.incoming) all.push(...group.incoming);
        next[id] = all;
      }
    }
    dependencyLinksByItem = next;
  }

  async function loadBoardConfig() {
    const requestedView = viewSignature;
    boardConfig = null;
    try {
      const config = await collectionStore.getBoardConfiguration(workspaceId, collectionId);
      if (requestedView !== viewSignature) return;
      boardConfig = config;
    } catch (error) {
      if (requestedView !== viewSignature) return;
      if (error.status !== 404) {
        console.error('Failed to load board configuration:', error);
      }
      boardConfig = null;
    }
  }

  // Adaptive polling for board items: use cheap deltas, falling back to full refresh only when needed.
  const poller = useWorkItemPoller(() => refreshCollectionDeltas());

  // Instant refresh after an AI chat agent run — surfaces tool-call effects
  // (created items, status transitions, etc.) without waiting for the poll.
  $effect(() => agentRuns.subscribe(() => {
    reloadCollection();
  }));

  // Iteration filter derived values
  let activeLocalIteration = $derived(allIterations.find(i => !i.is_global && i.status === 'active'));

  let iterationFilterOptions = $derived.by(() => {
    const seen = new Set();
    return allIterations.filter(i => {
      if (i.status === 'completed' || i.status === 'cancelled') return false;
      if (seen.has(i.id)) return false;
      seen.add(i.id);
      return true;
    });
  });

  let filteredItems = $derived.by(() => {
    return iterationFilterId
      ? items.filter(item => item.iteration_id === iterationFilterId)
      : items;
  });

  function getItemsByStatus(statusId, itemSubset = filteredItems) {
    return itemSubset.filter(
      item => boardStatusIdForItem(item, validColumns, personalWorkspaceIds) === statusId
    );
  }

  function getItemsByColumn(column, itemSubset = filteredItems) {
    return itemSubset.filter(item => column.status_ids?.includes(
      boardStatusIdForItem(item, validColumns, personalWorkspaceIds)
    ));
  }

  function parseLaneParentId(value) {
    if (!groupByItemTypeId) return undefined;
    if (value == null || value === '') return null;
    const parsed = parseInt(value, 10);
    return Number.isFinite(parsed) ? parsed : null;
  }

  function isExcludedRightmostSwimlaneParent(item) {
    return Boolean(
      excludeRightmostSwimlaneParents &&
      rightmostBoardColumnStatusIds.has(
        boardStatusIdForItem(item, validColumns, personalWorkspaceIds)
      )
    );
  }

  function isEligibleSwimlaneParent(item) {
    return item.item_type_id === groupByItemTypeId && !isExcludedRightmostSwimlaneParent(item);
  }

  function getItemsForLaneParent(parentId) {
    if (!groupByItemTypeId) return filteredItems;
    const parentIds = new Set(items
      .filter(isEligibleSwimlaneParent)
      .map(item => item.id));

    if (parentId != null) {
      return filteredItems.filter(item => item.parent_id === parentId && item.id !== parentId);
    }

    return filteredItems.filter(item => {
      if (item.item_type_id === groupByItemTypeId) return false;
      if (item.parent_id && parentIds.has(item.parent_id)) return false;
      return true;
    });
  }

  function wouldChangeLaneParent(item, targetParentId) {
    if (!groupByItemTypeId || targetParentId === undefined) return false;
    return (item.parent_id ?? null) !== targetParentId;
  }

  function warnUnsupportedCombinedBoardMove() {
    warningToast(t('collections.combinedBoardMoveUnsupported'));
  }

  async function updateItemParentForLane(item, targetParentId) {
    if (!groupByItemTypeId || targetParentId === undefined) return item;

    const currentParentId = item.parent_id ?? null;
    if (currentParentId === targetParentId) return item;

    try {
      const updatedItem = await api.items.update(item.id, { parent_id: targetParentId });
      collectionStore.items = collectionStore.items.map(existing =>
        existing.id === item.id
          ? { ...existing, ...updatedItem, parent_id: targetParentId }
          : existing
      );
      return { ...item, ...updatedItem, parent_id: targetParentId };
    } catch (error) {
      console.error('Failed to move item to swimlane:', error);
      warningToast(t('collections.swimlaneMoveFailed'));
      if (error && typeof error === 'object') {
        error.swimlaneMoveFailed = true;
      }
      throw error;
    }
  }

  // Status badges use an accessible-contrast pass so colours read against the
  // gradient backdrop on the board; the other iteration picker call sites
  // don't need this and keep the simpler hex+15 default.
  const iterationPickerConfig = buildIterationPickerConfig({
    statusBadgeColors: ({ hex }) => {
      const visible = getVisibleColor(hex);
      const { r, g, b } = hexToRgb(visible);
      return {
        bgColor: `rgba(${r}, ${g}, ${b}, 0.15)`,
        textColor: visible,
      };
    },
  });

  let otherIterationOptions = $derived(iterationFilterOptions.filter(i => i.id !== activeLocalIteration?.id));

  let selectedOtherIteration = $derived(
    iterationFilterId && iterationFilterId !== activeLocalIteration?.id
      ? allIterations.find(i => i.id === iterationFilterId)
      : null
  );

  function setIterationFilter(iterationId) {
    iterationFilterId = iterationId;
    if (iterationId) {
      localStorage.setItem(`board-iteration-filter-${workspaceId}`, String(iterationId));
    } else {
      localStorage.removeItem(`board-iteration-filter-${workspaceId}`);
    }
  }

  function boardPreferenceScope() {
    return collectionId ? `collection-${collectionId}` : `workspace-${workspaceId || 'global'}`;
  }

  function groupByStorageKey() {
    return `board-group-by-item-type-${boardPreferenceScope()}`;
  }

  function excludeRightmostSwimlaneParentsStorageKey() {
    return `board-exclude-rightmost-swimlane-parents-${boardPreferenceScope()}`;
  }

  function sortModeStorageKey() {
    return `board-sort-mode-${boardPreferenceScope()}`;
  }

  function loadSortMode() {
    try {
      return localStorage.getItem(sortModeStorageKey()) === 'bubble' ? 'bubble' : 'rank';
    } catch (e) {
      return 'rank';
    }
  }

  function setSortMode(mode) {
    sortMode = mode === 'bubble' ? 'bubble' : 'rank';
    try {
      if (sortMode === 'bubble') {
        localStorage.setItem(sortModeStorageKey(), sortMode);
      } else {
        localStorage.removeItem(sortModeStorageKey());
      }
    } catch (e) { /* ignore storage errors */ }
    collectionStore.setBoardSortMode(sortMode);
  }

  function setGroupByItemType(itemTypeId) {
    groupByItemTypeId = itemTypeId;
    swimlaneCollapsed = {};
    try {
      if (itemTypeId) {
        localStorage.setItem(groupByStorageKey(), String(itemTypeId));
      } else {
        localStorage.removeItem(groupByStorageKey());
      }
    } catch (e) { /* ignore storage errors */ }
  }

  function setExcludeRightmostSwimlaneParents(value) {
    excludeRightmostSwimlaneParents = value;
    swimlaneCollapsed = {};
    try {
      localStorage.setItem(excludeRightmostSwimlaneParentsStorageKey(), String(value));
    } catch (e) { /* ignore storage errors */ }
  }

  function toggleSwimlane(swimlaneId) {
    swimlaneCollapsed = {
      ...swimlaneCollapsed,
      [swimlaneId]: !swimlaneCollapsed[swimlaneId]
    };
  }

  function isSwimlaneExpanded(swimlaneId) {
    return swimlaneCollapsed[swimlaneId] !== true;
  }

  // --- Collapsible board columns ---
  function collapsedColumnsStorageKey() {
    return `board-collapsed-columns-${boardPreferenceScope()}`;
  }

  function loadCollapsedColumns() {
    try {
      const saved = localStorage.getItem(collapsedColumnsStorageKey());
      if (!saved) return {};
      const ids = JSON.parse(saved);
      if (!Array.isArray(ids)) return {};
      const map = {};
      for (const id of ids) map[id] = true;
      return map;
    } catch (e) {
      return {};
    }
  }

  function saveCollapsedColumns() {
    try {
      const collapsed = Object.keys(collapsedColumns).filter((id) => collapsedColumns[id]);
      if (collapsed.length > 0) {
        localStorage.setItem(collapsedColumnsStorageKey(), JSON.stringify(collapsed));
      } else {
        localStorage.removeItem(collapsedColumnsStorageKey());
      }
    } catch (e) { /* ignore storage errors */ }
  }

  function isColumnCollapsed(columnId) {
    return collapsedColumns[columnId] === true;
  }

  function toggleColumnCollapse(columnId) {
    collapsedColumns = {
      ...collapsedColumns,
      [columnId]: !collapsedColumns[columnId]
    };
    saveCollapsedColumns();
  }

  // Grid template for a lane: collapsed columns take a fixed narrow width,
  // expanded columns stay flexible so they share the remaining space.
  function boardGridTemplate(columns = validColumns) {
    return columns
      .map((col) => (isColumnCollapsed(col.id) ? '44px' : 'minmax(300px, 1fr)'))
      .join(' ');
  }


  // Backlog items are loaded from backend in loadData()

  // Compute display columns based on board configuration or fall back to
  // sorted statuses. Shared with collectionContext's split-fetch logic so
  // the store excludes exactly the statuses rendered in the capped column.
  let displayColumns = $derived(buildDisplayColumns(boardConfig, statuses));

  let validColumns = $derived(displayColumns.filter(col => col.status_ids?.length > 0));
  let rightmostBoardColumn = $derived(validColumns[validColumns.length - 1] ?? null);
  let rightmostBoardColumnStatusIds = $derived(new Set(rightmostBoardColumn?.status_ids || []));

  function shouldLimitRightmostColumn(columnIndex, columnsForBoard = validColumns) {
    return Boolean(
      !searchActive &&
      boardConfig?.show_rightmost_column_last_50 &&
      columnIndex === columnsForBoard.length - 1
    );
  }

  function itemRecencyValue(item) {
    return new Date(item.last_active_at || item.updated_at || item.created_at || 0).getTime() || 0;
  }

  function sortByRecency(items) {
    return items.slice().sort((a, b) => itemRecencyValue(b) - itemRecencyValue(a) || b.id - a.id);
  }

  function getDisplayItemsByColumn(column, columnIndex, columnsForBoard = validColumns, itemSubset = filteredItems) {
    const columnItems = getItemsByColumn(column, itemSubset);
    // Bubble Mode: most-recently-active cards rise to the top of every column.
    if (sortMode === 'bubble') {
      return shouldLimitRightmostColumn(columnIndex, columnsForBoard)
        ? sortByRecency(columnItems).slice(0, RIGHTMOST_COLUMN_LIMIT)
        : sortByRecency(columnItems);
    }
    // Rank Mode: backend frac_index order, except the capped rightmost column
    // which always shows the latest by recency.
    return shouldLimitRightmostColumn(columnIndex, columnsForBoard)
      ? sortByRecency(columnItems).slice(0, RIGHTMOST_COLUMN_LIMIT)
      : columnItems;
  }

  // Use the deferred partition's server total when all of its statuses belong
  // to this column. Swimlanes and active client filters keep loaded counts.
  function getColumnTotal(column, allColumnItems) {
    const deferred = collectionStore.boardDeferred;
    const columnStatusIds = new Set(column.status_ids || []);
    const useServerTotal =
      deferred &&
      !selectedGroupByItemType &&
      !searchQuery.trim() &&
      !iterationFilterId &&
      deferred.statusIds.every((statusId) => columnStatusIds.has(statusId));
    if (!useServerTotal) return allColumnItems.length;

    const deferredStatusIds = new Set(deferred.statusIds);
    const loadedDeferredCount = allColumnItems.filter((item) =>
      deferredStatusIds.has(item.status_id)
    ).length;
    return Math.max(
      allColumnItems.length,
      allColumnItems.length - loadedDeferredCount + deferred.total
    );
  }

  let selectedGroupByItemType = $derived(
    groupByItemTypeId ? itemTypes.find(type => type.id === groupByItemTypeId) : null
  );

  let hiddenRightmostSwimlaneParentCount = $derived.by(() => {
    if (!groupByItemTypeId || !excludeRightmostSwimlaneParents || !rightmostBoardColumn) return 0;
    return items.filter(item => item.item_type_id === groupByItemTypeId && isExcludedRightmostSwimlaneParent(item)).length;
  });

  let sortByMenuItems = $derived([
    {
      id: 'sort-rank',
      testid: 'board-sort-rank',
      title: t('boardControls.rankMode'),
      subtitle: t('boardControls.rankModeDescription'),
      badge: sortMode === 'rank' ? t('common.selected') : '',
      onClick: () => setSortMode('rank')
    },
    {
      id: 'sort-bubble',
      testid: 'board-sort-bubble',
      title: t('boardControls.bubbleMode'),
      subtitle: t('boardControls.bubbleModeDescription'),
      badge: sortMode === 'bubble' ? t('common.selected') : '',
      onClick: () => setSortMode('bubble')
    }
  ]);

  let groupByMenuItems = $derived.by(() => {
    const sortedTypes = sortItemTypesByHierarchy(itemTypes);
    const rightmostColumnName = objectDisplayName(rightmostBoardColumn, 'status') || t('boardControls.rightmostColumn');
    return [
      {
        id: 'group-none',
        testid: 'board-group-by-none',
        title: t('boardControls.noSwimlanes'),
        subtitle: t('boardControls.noSwimlanesDescription'),
        badge: groupByItemTypeId ? '' : t('common.selected'),
        onClick: () => setGroupByItemType(null)
      },
      ...(groupByItemTypeId ? [
        { id: 'group-rightmost-toggle-divider', type: 'divider' },
        {
          id: 'group-rightmost-toggle',
          type: 'checkbox',
          title: t('boardControls.hideColumnSwimlanes', { name: rightmostColumnName }),
          subtitle: selectedGroupByItemType
            ? t('boardControls.groupOutsideColumnWithType', { type: selectedGroupByItemType.name, column: rightmostColumnName })
            : t('boardControls.groupOutsideColumn', { column: rightmostColumnName }),
          badge: hiddenRightmostSwimlaneParentCount > 0 ? t('boardControls.hiddenCount', { count: hiddenRightmostSwimlaneParentCount }) : '',
          checked: excludeRightmostSwimlaneParents,
          closeOnSelect: false,
          onChange: setExcludeRightmostSwimlaneParents
        }
      ] : []),
      ...(sortedTypes.length > 0 ? [{ id: 'group-divider', type: 'divider' }] : []),
      ...sortedTypes.map(type => ({
        id: `group-type-${type.id}`,
        testid: `board-group-by-type-${type.id}`,
        title: type.name,
        subtitle: t('boardControls.useAsSwimlanes'),
        itemType: type,
        badge: groupByItemTypeId === type.id ? t('common.selected') : '',
        onClick: () => setGroupByItemType(type.id)
      }))
    ];
  });

  let boardSwimlanes = $derived.by(() => {
    if (!groupByItemTypeId) {
      return [{
        id: 'all',
        title: t('collections.allItems'),
        parent: null,
        items: filteredItems,
        isDefault: true,
        isUnassigned: false,
        itemCount: filteredItems.length,
        parentIsVisible: false
      }];
    }

    const parentItems = items.filter(isEligibleSwimlaneParent);
    const parentIds = new Set(parentItems.map(item => item.id));
    const visibleItemIds = new Set(filteredItems.map(item => item.id));

    const lanes = parentItems
      .map(parent => {
        const laneItems = filteredItems.filter(item => item.parent_id === parent.id && item.id !== parent.id);
        return {
          id: `parent-${parent.id}`,
          title: parent.title,
          parent,
          items: laneItems,
          isDefault: false,
          isUnassigned: false,
          itemCount: laneItems.length,
          parentIsVisible: visibleItemIds.has(parent.id)
        };
      })
      .filter(lane => lane.itemCount > 0 || lane.parentIsVisible);

    const unassignedItems = filteredItems.filter(item => {
      if (item.item_type_id === groupByItemTypeId) return false;
      if (item.parent_id && parentIds.has(item.parent_id)) return false;
      return true;
    });

    return [
      ...lanes,
      {
        id: 'unassigned',
        title: t('common.unassigned'),
        parent: null,
        items: unassignedItems,
        isDefault: false,
        isUnassigned: true,
        itemCount: unassignedItems.length,
        parentIsVisible: false
      }
    ];
  });

  // Calculate total visible items across all display columns
  let totalVisibleItems = $derived.by(() => {
    return boardSwimlanes.reduce((laneTotal, lane) => {
      return laneTotal + validColumns.reduce((columnTotal, column, columnIndex) => {
        return columnTotal + getDisplayItemsByColumn(column, columnIndex, validColumns, lane.items).length;
      }, 0);
    }, 0);
  });

  function getStatusByName(statusName) {
    const normalizedName = statusName.toLowerCase().replace('_', ' ');
    return statuses.find(status =>
      status.name.toLowerCase() === normalizedName ||
      status.name.toLowerCase().replace(' ', '_') === statusName
    );
  }

  function getStatusColor(categoryColor) {
    // Convert hex color to Tailwind-compatible text classes
    const colorMap = {
      '#3b82f6': 'text-blue-800',
      '#ef4444': 'text-red-800',
      '#10b981': 'text-green-800',
      '#f59e0b': 'text-orange-800',
      '#6b7280': 'text-gray-800'
    };
    return colorMap[categoryColor] || 'text-gray-800';
  }

  function openItem(itemId, event) {
    // Prevent event bubbling to avoid triggering drag
    event.stopPropagation();
    selectedItemId = itemId;
    showItemModal = true;
  }

  function getMoveMenuItems(item) {
    const currentBoardStatusId = boardStatusIdForItem(item, validColumns, personalWorkspaceIds);
    const personalTask = personalWorkspaceIds.has(Number(item.workspace_id));
    return validColumns
      .map(column => ({
        column,
        targetStatusId: statusIdForBoardColumnMove(
          item,
          column,
          validColumns,
          personalWorkspaceIds,
        ),
      }))
      .filter(({ column, targetStatusId }) => (
        targetStatusId != null && !column.status_ids.includes(currentBoardStatusId)
      ))
      .map(({ column, targetStatusId }) => {
        const targetStatus = statuses.find(status => status.id === targetStatusId);
        const targetName = personalTask ? column.name : targetStatus?.name || column.name;
        return {
          id: `move-${item.id}-${targetStatusId}`,
          title: targetName,
          iconDot: true,
          iconColor: column.color || targetStatus?.category_color || targetStatus?.color || 'var(--ds-text-subtle)',
          onClick: () => moveItemToStatus(item, targetStatusId, targetName)
        };
      });
  }

  async function moveItemToStatus(item, targetStatusId, targetName) {
    const previousStatusId = item.status_id;
    updateLocalItemStatus(item.id, targetStatusId);
    try {
      const transitionedItem = await api.items.transition(item.id, targetStatusId);
      mergeLocalItem(item.id, transitionedItem);
      boardAnnouncement = `Moved ${item.title} to ${targetName}`;
      successToast(boardAnnouncement);
      reloadCollection();
    } catch (err) {
      updateLocalItemStatus(item.id, previousStatusId);
      console.error('Status transition failed:', err);
      warningToast(t('collections.transition_failed'));
      reloadCollection();
    }
  }

  async function closeItemModal(event) {
    showItemModal = false;
    selectedItemId = null;

    // If changes were made in the modal, reload data
    if (event?.hasChanges) {
      reloadCollection();
    }
  }

  // Drag and drop setup using Pragmatic DnD
  function setupDragAndDrop() {
    // Clear any pending setup
    if (setupTimeout) {
      clearTimeout(setupTimeout);
    }

    // Clean up existing registrations
    setupElements.forEach((cleanup, elementId) => {
      if (typeof cleanup === 'function') {
        cleanup();
      }
    });
    setupElements.clear();

    // Reset drag state
    dragState = new Map();

    // Setup work item cards as both draggable and drop targets with edge detection
    const itemCards = /** @type {NodeListOf<HTMLElement>} */ (document.querySelectorAll('[data-item-card]'));

    itemCards.forEach(element => {
      const itemId = parseInt(element.dataset.itemId);
      const elementId = `item-${itemId}`;

      const item = items.find(i => i.id === itemId);
      if (!item) return;
      const targetLaneParentId = parseLaneParentId(element.dataset.swimlaneParentId);

      // Initialize drag state for this item
      dragState.set(itemId, { isDragging: false, closestEdge: null });

      // Make draggable
      const draggableCleanup = draggable({
        element,
        getInitialData: () => ({
          item,
          type: 'work-item'
        }),
        onDragStart: () => {
          element.style.opacity = '0.5';
          document.body.classList.add('is-dragging');
          // Mark this item as being dragged
          const state = dragState.get(itemId) || {};
          dragState.set(itemId, { ...state, isDragging: true });
          dragState = new Map(dragState); // Trigger reactivity
        },
        onDrop: () => {
          element.style.opacity = '';
          document.body.classList.remove('is-dragging');
          // Reset all drag states
          dragState.forEach((state, id) => {
            dragState.set(id, { isDragging: false, closestEdge: null });
          });
          dragState = new Map(dragState); // Trigger reactivity
          // Reset all column border styles
          resetAllColumnStyles();
        }
      });

      // Make drop target with edge detection
      const dropTargetCleanup = dropTargetForElements({
        element,
        canDrop: ({ source }) => {
          const data = /** @type {any} */ (source.data);
          // Can't drop on self
          if (data.type !== 'work-item' || data.item.id === itemId) {
            return false;
          }
          const targetColumn = getBoardColumnForItem(item);
          return Boolean(
            targetColumn && statusIdForBoardColumnMove(
              data.item,
              targetColumn,
              validColumns,
              personalWorkspaceIds,
            ) != null
          );
        },
        getData: ({ input, element }) => {
          return attachClosestEdge({}, {
            input,
            element,
            allowedEdges: ['top', 'bottom']
          });
        },
        onDragEnter: ({ self, source }) => {
          const data = /** @type {any} */ (source.data);
          if (data.type === 'work-item' && data.item.id !== itemId) {
            const closestEdge = extractClosestEdge(self.data);
            const state = dragState.get(itemId) || {};
            dragState.set(itemId, { ...state, closestEdge });
            dragState = new Map(dragState); // Trigger reactivity
          }
        },
        onDragLeave: () => {
          const state = dragState.get(itemId) || {};
          dragState.set(itemId, { ...state, closestEdge: null });
          dragState = new Map(dragState); // Trigger reactivity
        },
        onDrop: ({ self, source }) => {
          const data = /** @type {any} */ (source.data);
          const closestEdge = extractClosestEdge(self.data);

          if (data.type === 'work-item' && closestEdge) {
            const targetStatus = getStatusByItemId(itemId);
            if (targetStatus) {
              handleEdgeBasedDrop(data.item, item, closestEdge, targetStatus, targetLaneParentId);
            }
          }
        }
      });

      setupElements.set(elementId, () => {
        draggableCleanup();
        dropTargetCleanup();
      });
    });

    // Setup status columns as drop targets
    const statusColumns = /** @type {NodeListOf<HTMLElement>} */ (document.querySelectorAll('[data-status-column]'));

    statusColumns.forEach(element => {
      const statusId = parseInt(element.dataset.statusId);
      const elementId = element.dataset.statusColumnKey || `status-${statusId}`;

      const status = statuses.find(s => s.id === statusId);
      if (!status) return;
      const targetColumn = validColumns.find(column => column.status_ids?.[0] === statusId);
      if (!targetColumn) return;
      const targetLaneParentId = parseLaneParentId(element.dataset.swimlaneParentId);

      const cleanup = dropTargetForElements({
        element,
        canDrop: ({ source }) => {
          const data = /** @type {any} */ (source.data);
          return data.type === 'work-item' && statusIdForBoardColumnMove(
            data.item,
            targetColumn,
            validColumns,
            personalWorkspaceIds,
          ) != null;
        },
        onDragEnter: ({ source }) => {
          const data = /** @type {any} */ (source.data);
          if (
            data.type === 'work-item' &&
            statusIdForBoardColumnMove(
              data.item,
              targetColumn,
              validColumns,
              personalWorkspaceIds,
            ) != null
          ) {
            // The server is authoritative for workflow validation. Highlight
            // every status target and let the optimistic drop roll back if the
            // transition is rejected.
            element.style.boxShadow = 'inset 0 0 0 2px var(--ds-border-focused)';
          }
        },
        onDragLeave: () => {
          // Reset styles
          element.style.boxShadow = '';
        },
        onDrop: async ({ source, location }) => {
          // Reset all column styles immediately
          resetAllColumnStyles();

          const data = /** @type {any} */ (source.data);
          if (data.type === 'work-item') {
            // If an inner item drop target exists, handleEdgeBasedDrop already handles status
            const dropTargets = location.current.dropTargets;
            if (dropTargets.length > 1 && dropTargets[0].element !== element) {
              return;
            }
            const transitionStatusId = statusIdForBoardColumnMove(
              data.item,
              targetColumn,
              validColumns,
              personalWorkspaceIds,
            );
            if (transitionStatusId == null) return;
            const isSameStatus = data.item.status_id === transitionStatusId;
            if (!isSameStatus && wouldChangeLaneParent(data.item, targetLaneParentId)) {
              warnUnsupportedCombinedBoardMove();
              return;
            }
            const previousStatusId = data.item.status_id;
            if (!isSameStatus) updateLocalItemStatus(data.item.id, transitionStatusId);
            try {
              let droppedItem = data.item;
              if (!isSameStatus) {
                droppedItem = await api.items.transition(data.item.id, transitionStatusId);
                mergeLocalItem(data.item.id, droppedItem);
              }
              await updateItemParentForLane(droppedItem, targetLaneParentId);
            } catch (err) {
              if (!isSameStatus) updateLocalItemStatus(data.item.id, previousStatusId);
              console.error('Board drop failed:', err);
              if (!err?.swimlaneMoveFailed) {
                warningToast(t('collections.transition_failed'));
              }
            }
            reloadCollection();
          }
        }
      });

      setupElements.set(elementId, cleanup);
    });

    // No longer using position drop zones - edge detection handles everything
  }

  // Helper functions
  function resetAllColumnStyles() {
    // Reset all status column styles to their default state
    const statusColumns = /** @type {NodeListOf<HTMLElement>} */ (document.querySelectorAll('[data-status-column]'));
    statusColumns.forEach(element => {
      element.style.boxShadow = '';
    });
  }

  function getStatusByItemId(itemId) {
    const item = items.find(i => i.id === itemId);
    if (!item) return null;
    const boardStatusId = boardStatusIdForItem(item, validColumns, personalWorkspaceIds);
    return statuses.find(s => s.id === boardStatusId);
  }

  function getBoardColumnForItem(item) {
    const boardStatusId = boardStatusIdForItem(item, validColumns, personalWorkspaceIds);
    return validColumns.find(column => column.status_ids?.includes(boardStatusId));
  }

  function updateLocalItemStatus(itemId, statusId) {
    collectionStore.items = collectionStore.items.map(item =>
      item.id === itemId ? { ...item, status_id: statusId } : item
    );
  }

  function mergeLocalItem(itemId, updatedItem) {
    if (!updatedItem) return;
    collectionStore.items = collectionStore.items.map(item =>
      item.id === itemId ? { ...item, ...updatedItem } : item
    );
  }

  async function updateItemStatus(itemId, newStatus) {
    try {
      await api.items.update(itemId, { status: newStatus });

      // Update store directly with a new array to ensure reactivity
      collectionStore.items = collectionStore.items.map(item =>
        item.id === itemId
          ? { ...item, status: newStatus }
          : item
      );

      // Force a re-setup of drag and drop with the updated items
      setTimeout(() => {
        setupDragAndDrop();
      }, 100);
    } catch (error) {
      console.error('Failed to update item status:', error);
      // Could add user notification here
    }
  }

  async function handleEdgeBasedDrop(draggedItem, targetItem, closestEdge, targetStatus, targetLaneParentId = undefined) {
    // Create a unique identifier for this drop operation
    const dropId = `${draggedItem.id}-edge-${targetItem.id}-${closestEdge}`;

    try {
      // Prevent duplicate drops
      if (pendingDrops.has(dropId)) {
        return;
      }

      pendingDrops.add(dropId);

      // Reset all column border styles immediately
      resetAllColumnStyles();

      const currentStatusId = draggedItem.status_id;
      const targetColumn = validColumns.find(column => column.status_ids?.includes(targetStatus.id));
      const targetStatusId = targetColumn
        ? statusIdForBoardColumnMove(
            draggedItem,
            targetColumn,
            validColumns,
            personalWorkspaceIds,
          )
        : null;
      if (targetStatusId == null) {
        reloadCollection();
        return;
      }

      // Check if we need to update status
      const isSameStatus = currentStatusId === targetStatusId;

      if (!isSameStatus && wouldChangeLaneParent(draggedItem, targetLaneParentId)) {
        warnUnsupportedCombinedBoardMove();
        reloadCollection();
        return;
      }

      // If changing status, update the status first
      if (!isSameStatus) {
        updateLocalItemStatus(draggedItem.id, targetStatusId);
        try {
          const transitionedItem = await api.items.transition(draggedItem.id, targetStatusId);
          draggedItem = { ...draggedItem, ...transitionedItem, status_id: targetStatusId };
          mergeLocalItem(draggedItem.id, draggedItem);
        } catch (err) {
          updateLocalItemStatus(draggedItem.id, currentStatusId);
          console.error('Status transition failed:', err);
          warningToast(t('collections.transition_failed'));
          reloadCollection();
          return;
        }
      }

      draggedItem = await updateItemParentForLane(draggedItem, targetLaneParentId);

      // Bubble Mode disables manual rank ordering. Cross-column drags above have
      // already transitioned status (which bumps the card to the top via
      // last_active_at); a same-column drag is a no-op. Either way, skip the
      // frac_index reorder and just refresh.
      if (sortMode === 'bubble') {
        if (isSameStatus && !wouldChangeLaneParent(draggedItem, targetLaneParentId)) {
          infoToast(
            t('collections.manual_sort_unavailable'),
            t('collections.manual_sort_unavailable_title')
          );
        }
        reloadCollection();
        return;
      }

      // Get items in the target status for position calculation
      const laneItems = targetLaneParentId === undefined ? filteredItems : getItemsForLaneParent(targetLaneParentId);
      const statusItems = getItemsByStatus(targetStatus.id, laneItems);

      // Find the target item's position in the sorted status items
      const targetIndex = statusItems.findIndex(item => item.id === targetItem.id);
      const draggedIndex = statusItems.findIndex(item => item.id === draggedItem.id);

      // Remove the dragged item from consideration to get accurate neighboring items
      const otherItems = statusItems.filter(item => item.id !== draggedItem.id);
      const adjustedTargetIndex = otherItems.findIndex(item => item.id === targetItem.id);

      // Check if we're trying to drop in the same position
      const isDroppingSamePosition = (
        (closestEdge === 'top' && draggedIndex === targetIndex - 1) ||
        (closestEdge === 'bottom' && draggedIndex === targetIndex + 1)
      );

      if (isDroppingSamePosition && isSameStatus) {
        return;
      }

      // Calculate item IDs based on edge (backend will determine actual global ranks)
      let prevItemId = null;
      let nextItemId = null;

      if (closestEdge === 'top') {
        // Insert before target item
        if (adjustedTargetIndex > 0) {
          const prevItem = otherItems[adjustedTargetIndex - 1];
          if (prevItem) prevItemId = prevItem.id;
        }
        if (targetItem) nextItemId = targetItem.id;
      } else if (closestEdge === 'bottom') {
        // Insert after target item
        if (targetItem) prevItemId = targetItem.id;
        if (adjustedTargetIndex < otherItems.length - 1) {
          const nextItem = otherItems[adjustedTargetIndex + 1];
          if (nextItem) nextItemId = nextItem.id;
        }
      }

      // Update the frac_index using item IDs
      const indexData = {
        prev_item_id: prevItemId,
        next_item_id: nextItemId
      };
      const updatedItem = await api.items.updateFracIndex(draggedItem.id, indexData);

      // Reload data from central store to get the correct ordering
      reloadCollection();

    } catch (error) {
      console.error('Failed to handle edge-based drop:', error);
      console.error('Error details:', error.message);

      // If we get a rank ordering error, reload fresh data
      if (error.status === 500) {
        reloadCollection();
      }
    } finally {
      // Always remove from pending drops
      setTimeout(() => {
        pendingDrops.delete(dropId);
      }, 500); // Small delay to prevent rapid re-triggering
    }
  }


  // Setup drag and drop when data changes. Track grouping/swimlane inputs too,
  // because switching group-by replaces the board DOM without changing item count.
  $effect(() => {
    const itemSignature = items.map(item => `${item.id}:${item.status_id ?? ''}:${item.parent_id ?? ''}`).join('|');
    const laneSignature = boardSwimlanes.map(lane => `${lane.id}:${lane.itemCount}:${isSwimlaneExpanded(lane.id)}`).join('|');
    const columnSignature = validColumns.map(column => `${column.id}:${(column.status_ids || []).join(',')}`).join('|');
    // Collapsing/expanding a column swaps the rendered column element
    // (narrow strip vs full column) without changing item counts, so the
    // drop targets must be rebuilt — track the collapse state here.
    const collapsedSignature = validColumns.map((column) => isColumnCollapsed(column.id) ? '1' : '0').join('');
    groupByItemTypeId;
    itemSignature;
    laneSignature;
    columnSignature;
    collapsedSignature;

    if (items.length > 0 && statuses.length > 0 && typeof document !== 'undefined') {
      if (setupTimeout) clearTimeout(setupTimeout);
      setupTimeout = setTimeout(() => {
        setupDragAndDrop();
      }, 100);
    }
  });

</script>

{#if loading || collectionStore.loading || workspaceDataStore.initialLoading}
  <div class="p-6">
    <div class="animate-pulse">{t('common.loading')}</div>
  </div>
{:else if workspace || !workspaceId}
  <StaticViewBackground
    backgroundStyle={styles.backgroundStyle}
    contextVars={styles.contextVars}
    contentClass="p-6 min-w-fit"
    testid="collection-board-background"
  >
    <!-- Content Container -->
      <!-- Header with view tabs -->
      <div class="mb-8">
        <ViewHeader
          workspaceName={workspace?.name || ''}
          collection={currentCollectionName === 'Default' ? t('common.default') : currentCollectionName}
          viewName={t('workspaceSettings.views.board')}
          itemCount={collectionStore.itemsTotalCount}
        >
          {#snippet actions()}
            <div class="flex items-center gap-3">
              {#if allIterations.length > 0}
                <div class="inline-flex items-center rounded-lg border overflow-hidden text-sm"
                     style="background-color: var(--ctx-surface, transparent); backdrop-filter: var(--ctx-backdrop, none); border-color: var(--ctx-border, var(--ds-border));">
                  <button
                    class="px-3 py-1.5 transition-colors"
                    style={!iterationFilterId
                      ? 'background-color: var(--ctx-surface-raised, var(--ds-surface-raised)); color: var(--ds-text); font-weight: 500;'
                      : 'color: var(--ds-text); background-color: transparent;'}
                    onclick={() => setIterationFilter(null)}
                  >
                    {t('collections.allItems')}
                  </button>
                  {#if activeLocalIteration}
                    <button
                      class="px-3 py-1.5 transition-colors border-l"
                      style="border-color: var(--ctx-border, var(--ds-border)); {iterationFilterId === activeLocalIteration.id
                        ? 'background-color: var(--ctx-surface-raised, var(--ds-surface-raised)); color: var(--ds-text); font-weight: 500;'
                        : 'color: var(--ds-text); background-color: transparent;'}"
                      onclick={() => setIterationFilter(activeLocalIteration.id)}
                    >
                      {activeLocalIteration.name}
                    </button>
                  {/if}
                  {#if otherIterationOptions.length > 0}
                    <ItemPicker
                      items={otherIterationOptions}
                      value={iterationFilterId && iterationFilterId !== activeLocalIteration?.id ? iterationFilterId : null}
                      config={iterationPickerConfig}
                      placeholder={t('iterations.filterByIteration')}
                      showUnassigned={false}
                      allowClear={false}
                      showSelectedInTrigger={false}
                      onSelect={(iter) => {
                        if (iter) {
                          setIterationFilter(iter.id);
                        }
                      }}
                    >
                      {#snippet children()}
                        <span
                          class="px-3 py-1.5 text-sm border-l flex items-center gap-1 transition-colors"
                          style="border-color: var(--ctx-border, var(--ds-border)); {selectedOtherIteration
                            ? 'color: var(--ds-text); font-weight: 500; background-color: var(--ctx-surface-raised, var(--ds-surface-raised));'
                            : 'color: var(--ds-text);'}"
                        >
                          {selectedOtherIteration ? selectedOtherIteration.name : t('iterations.filterByIteration')}
                          <ChevronDown size={12} />
                        </span>
                      {/snippet}
                    </ItemPicker>
                  {/if}
                </div>
              {/if}
              <!-- Group by / Sort by segmented group, styled to match the
                   board/backlog view switcher beside it. The trigger is
                   highlighted (raised + shadow) when a non-default selection is
                   active. -->
              <div class="flex rounded p-1" style="background-color: var(--ctx-surface, var(--ds-background-neutral)); backdrop-filter: var(--ctx-backdrop, none);">
                <DropdownMenu
                  items={groupByMenuItems}
                  triggerIcon={Layers}
                  triggerText={selectedGroupByItemType ? t('boardControls.groupByValue', { name: selectedGroupByItemType.name }) : t('boardControls.groupBy')}
                  placement="bottom-end"
                  maxWidth="max-w-xs"
                  triggerClass="px-3 py-1.5 rounded text-sm font-medium transition-colors {selectedGroupByItemType ? 'shadow-sm' : ''}"
                  triggerStyle={selectedGroupByItemType ? 'color: var(--ds-text); background-color: var(--ctx-surface-raised, var(--ds-surface-raised));' : 'color: var(--ds-text);'}
                  triggerTestid="board-group-by-menu"
                />
                <DropdownMenu
                  items={sortByMenuItems}
                  triggerIcon={ArrowDownUp}
                  triggerText={sortMode === 'bubble' ? t('boardControls.sortBubble') : t('boardControls.sortBy')}
                  placement="bottom-end"
                  maxWidth="max-w-xs"
                  triggerClass="px-3 py-1.5 rounded text-sm font-medium transition-colors {sortMode === 'bubble' ? 'shadow-sm' : ''}"
                  triggerStyle={sortMode === 'bubble' ? 'color: var(--ds-text); background-color: var(--ctx-surface-raised, var(--ds-surface-raised));' : 'color: var(--ds-text);'}
                  triggerTestid="board-sort-by-menu"
                />
              </div>
              <CollectionViewSwitcher
                {workspaceId}
                {collectionId}
                activeView="board"
                publicSlug={collectionStore.publicSlug}
              />
            </div>
          {/snippet}
        </ViewHeader>
      </div>

      <!-- Controls Bar -->
      <div class="flex items-center gap-4 mb-6">
        <SearchInput
          bind:value={searchQuery}
          placeholder={t('common.search')}
          dataTestid="board-search-input"
        />
        <SubFilterBar {workspaceId} />
      </div>

      {#if searchActive && (searchDebouncing || collectionStore.boardSearchLoading)}
        <p class="-mt-4 mb-4 text-sm" style="color: var(--ds-text-subtle);" data-testid="board-search-status">
          {t('common.searching')}
        </p>
      {:else if searchActive && collectionStore.boardSearchError}
        <p class="-mt-4 mb-4 text-sm" style="color: var(--ds-text-danger);" data-testid="board-search-error">
          {t('common.error')}
        </p>
      {/if}

      <div class="sr-only" aria-live="polite">{boardAnnouncement}</div>

      {#if statuses.length === 0}
        <!-- No Statuses State -->
        <div class="text-center py-12">
          <div class="mb-4" style="color: var(--ctx-text-subtlest, var(--ds-icon-disabled));">
            <Plus class="w-16 h-16 mx-auto" />
          </div>
          <h3 class="text-lg font mb-2" style="color: var(--ctx-text, var(--ds-text));">{t('items.noItemsInFilter')}</h3>
          <p class="text-sm mb-4" style="color: var(--ctx-text-subtle, var(--ds-text-subtle));">
            {t('items.createToStart')}
          </p>
          <button
            onclick={() => navigate('/admin/workflows')}
            class="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 transition-colors"
          >
            {t('statuses.createStatus')}
          </button>
        </div>
      {:else}
        <!-- Board Columns / Swimlanes -->
        <div
          bind:this={boardViewElement}
          class={selectedGroupByItemType ? 'space-y-4' : ''}
          data-testid="board-view"
        >
          {#each boardSwimlanes as lane (lane.id)}
            {@const laneExpanded = isSwimlaneExpanded(lane.id)}
            <!-- No type can legally be created under this lane's item (the lane
                 groups by the lowest hierarchy level), so the column loses its
                 add affordance rather than opening a form that cannot submit. -->
            {@const laneQuickAddTypes = quickAddTypesFor(lane.parent ?? null)}
            <section
              class={selectedGroupByItemType ? 'rounded-lg border overflow-hidden' : ''}
              style={selectedGroupByItemType ? `${styles.glassStyle?.(10) ?? ''} border-color: var(--ctx-border, var(--ds-border));` : ''}
              data-board-swimlane={lane.id}
            >
              {#if selectedGroupByItemType}
                <div class="flex items-center justify-between gap-3 px-3 py-2 border-b" style="border-color: var(--ctx-border, var(--ds-border));">
                  <Button
                    variant="ghost"
                    size="sm"
                    class="flex-1 justify-start px-2"
                    style="color: var(--ds-text);"
                    onclick={() => toggleSwimlane(lane.id)}
                  >
                    {#if laneExpanded}
                      <ChevronDown class="w-4 h-4 flex-shrink-0" />
                    {:else}
                      <ChevronRight class="w-4 h-4 flex-shrink-0" />
                    {/if}
                    {#if selectedGroupByItemType}
                      <ItemTypeIcon
                        icon={selectedGroupByItemType.icon}
                        color={lane.isUnassigned ? 'var(--ds-background-neutral-bold, #6b7280)' : selectedGroupByItemType.color}
                        title={selectedGroupByItemType.name}
                      />
                    {/if}
                    <span class="min-w-0 flex-1 text-left">
                      <span class="block font-semibold truncate">{lane.title}</span>
                      {#if lane.parent}
                        <span class="block text-xs font-normal truncate" style="color: var(--ds-text-subtle);">
                          <ItemKey item={lane.parent} {workspace} /> · {selectedGroupByItemType.name} swimlane
                        </span>
                      {:else}
                        <span class="block text-xs font-normal truncate" style="color: var(--ds-text-subtle);">
                          Items without {excludeRightmostSwimlaneParents ? 'a visible' : 'a'} {selectedGroupByItemType.name} parent
                        </span>
                      {/if}
                    </span>
                  </Button>
                  <span class="text-xs px-2 py-0.5 rounded-full flex-shrink-0" style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                    {lane.itemCount} {lane.itemCount === 1 ? 'item' : 'items'}
                  </span>
                </div>
              {/if}

              {#if !selectedGroupByItemType || laneExpanded}
                <div class="grid gap-6 {selectedGroupByItemType ? 'p-4' : ''}" style="grid-template-columns: {boardGridTemplate()};">
                  {#each validColumns as column, columnIndex (column.id)}
                    {@const quickAddKey = `${lane.id}-${column.id}`}
                    {@const allColumnItems = getItemsByColumn(column, lane.items)}
                    {@const columnItems = getDisplayItemsByColumn(column, columnIndex, validColumns, lane.items)}
                    {@const columnTotal = getColumnTotal(column, allColumnItems)}
                    {@const hiddenColumnItemCount = columnTotal - columnItems.length}
                    {@const isOverWip = column.wip_limit && allColumnItems.length > column.wip_limit}
                    {#if isColumnCollapsed(column.id)}
                      <!-- Collapsed column: narrow vertical strip showing the rotated
                           column name and item count. Clicking it re-expands. It keeps
                           its status drop target so items can still be dropped here.
                           pt-5 puts the expand chevron on the same baseline as the
                           collapse chevron in BoardColumn's header (border-t-4 + p-4 +
                           the button's p-1). -->
                      <button
                        type="button"
                        class="relative rounded border shadow-sm flex flex-col items-center justify-between gap-2 pt-5 pb-3 px-1 text-center cursor-pointer transition-colors"
                        style="{styles.columnStyle(12)} border-top: 4px solid {column.color};"
                        data-testid="board-column"
                        id={`board-column-status-${column.status_ids[0]}`}
                        data-status-column
                        data-status-column-key={`${lane.id}-${column.id}-${column.status_ids[0]}`}
                        data-swimlane-parent-id={selectedGroupByItemType && lane.parent ? lane.parent.id : ''}
                        data-status-id={column.status_ids[0]}
                        aria-label={t('collections.expandColumn', { name: objectDisplayName(column, 'status') })}
                        aria-expanded="false"
                        title={t('collections.expandColumn', { name: objectDisplayName(column, 'status') })}
                        onclick={() => toggleColumnCollapse(column.id)}
                      >
                        <ChevronRight class="w-4 h-4 flex-shrink-0" style={styles.glassTextStyle} />
                        <span class="board-column-collapsed-name font-semibold text-sm break-words" style={styles.glassTextStyle}>
                          {objectDisplayName(column, 'status')}
                        </span>
                        <span class="text-xs px-1.5 py-0.5 rounded-full flex-shrink-0" style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                          {columnTotal}
                        </span>
                      </button>
                    {:else}
                    <BoardColumn
                      {column}
                      itemCount={columnTotal}
                      wipCount={allColumnItems.length}
                      visibleItemCount={columnItems.length}
                      hiddenItemCount={hiddenColumnItemCount}
                      {isOverWip}
                      statusColumnKey={`${lane.id}-${column.id}-${column.status_ids[0]}`}
                      swimlaneParentId={selectedGroupByItemType && lane.parent ? lane.parent.id : ''}
                      statusId={column.status_ids[0]}
                      quickAddOpen={quickAddState[quickAddKey]?.show}
                      columnStyle={styles.columnStyle(12)}
                      textStyle={styles.glassTextStyle}
                      subtleTextStyle={styles.glassSubtleTextStyle}
                      onadd={laneQuickAddTypes.length > 0 && availableWorkspaces.length > 0
                        ? () => initQuickAdd(column.id, column.status_ids[0], quickAddKey, lane.parent ?? null)
                        : null}
                      oncollapse={() => toggleColumnCollapse(column.id)}
                    >
                        {#if quickAddState[quickAddKey]?.show}
                          <div class="mb-3">
                            <QuickAddForm
                              parentId={quickAddKey}
                              formState={quickAddState[quickAddKey]}
                              workspaces={availableWorkspaces}
                              cardBgStyle={styles.cardStyle(8)}
                              onUpdateField={updateQuickAddField}
                              onCreate={createColumnItem}
                              onCancel={cancelQuickAdd}
                            />
                          </div>
                        {/if}
                        {#if columnItems.length === 0 && !quickAddState[quickAddKey]?.show}
                          <BoardEmptyState textStyle={styles.glassSubtleTextStyle} />
                        {:else}
                          {#if hiddenColumnItemCount > 0}
                            <p class="text-xs mb-3" style={styles.glassSubtleTextStyle}>
                              {#if collectionStore.boardDeferred?.capped}
                                Showing latest {columnItems.length} of {columnTotal} items in this column.
                              {:else}
                                Showing {columnItems.length} of {columnTotal} items in this column. Load more to see the remaining completed work.
                              {/if}
                            </p>
                          {/if}
                          <div class="space-y-1">
                            {#each columnItems as item (item.id)}
                              {@const moveMenuItems = getMoveMenuItems(item)}
                              <BoardItemCard
                                {item}
                                {workspace}
                                {itemTypes}
                                {cardFields}
                                {priorities}
                                {statuses}
                                {iterations}
                                {projects}
                                labels={wdsLabels}
                                {customFieldDefinitions}
                                {users}
                                dependencyLinks={dependencyLinksByItem[item.id] ?? []}
                                {moveMenuItems}
                                closestEdge={dragState.get(item.id)?.closestEdge}
                                swimlaneParentId={selectedGroupByItemType && lane.parent ? lane.parent.id : ''}
                                cardStyle={styles.cardStyle(0)}
                                textStyle={styles.glassTextStyle}
                                onopen={openItem}
                              />
                            {/each}
                          </div>
                        {/if}
                    </BoardColumn>
                    {/if}
                  {/each}
                </div>
              {/if}
            </section>
          {/each}
        </div>

        <!-- Load More -->
        {#if activeItemsHasMore}
          <div class="mt-6 text-center">
            <button
              data-testid="board-load-more"
              onclick={() => searchActive
                ? collectionStore.loadMoreBoardSearchItems()
                : collectionStore.loadMoreItems()}
              disabled={activeItemsLoadingMore}
              class="px-4 py-2 text-sm  rounded-lg border transition-colors"
              style="{styles.glassStyle?.(12) ?? ''} {styles.glassTextStyle ?? ''}"
            >
              {activeItemsLoadingMore ? t('common.loading') : t('common.loadMore')}
              {#if activeItemsRemainingCount > 0 && !iterationFilterId}
                ({activeItemsRemainingCount} {t('common.remaining')})
              {/if}
            </button>
          </div>
        {/if}

        <!-- Summary -->
        <div class="mt-8 text-center">
          <p class="text-sm" style="color: var(--ctx-text-subtle, var(--ds-text-subtle));">
            {t('collections.boardSummary', { itemCount: totalVisibleItems, columnCount: displayColumns.length })}
          </p>
        </div>
      {/if}
  </StaticViewBackground>
{:else}
  <div class="p-6">
    <div class="text-center" style="color: var(--ds-text-subtle);">
      {t('workspaces.noWorkspaces')}
    </div>
  </div>
{/if}

<!-- Item Detail Modal -->
{#if showItemModal && selectedItemId}
  {#if selectedItem && personalWorkspaceIds.has(Number(selectedItem.workspace_id))}
    <PersonalTaskDetail
      itemId={selectedItemId}
      workspaceId={selectedItem.workspace_id}
      onclose={closeItemModal}
      onupdate={reloadCollection}
    />
  {:else}
    <ItemDetail
      workspaceId={workspaceId}
      itemId={selectedItemId}
      isModal={true}
      onclose={closeItemModal}
    />
  {/if}
{/if}

<style>
  /* Collapsed board column: write the column name vertically so the narrow
     strip can still show it without overflowing. */
  .board-column-collapsed-name {
    writing-mode: vertical-rl;
    transform: rotate(180deg);
    max-height: 240px;
    overflow: hidden;
    line-height: 1.1;
  }

</style>
