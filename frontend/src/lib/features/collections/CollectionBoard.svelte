<script>
  import { onMount, untrack } from 'svelte';
  import { useEventListener } from 'runed';
  import { t } from '../../stores/i18n.svelte.js';
  import { items as itemsApi } from '../../api/items.js';
  import { collections as collectionsApi } from '../../api/collections.js';
  import { workspaces as workspacesApi } from '../../api/workspaces.js';
  import { iterations as iterationsApi } from '../../api/milestones.js';
  import { navigate } from '../../router.js';
  import {
    collectionStore,
    reloadCollection,
    scheduleCollectionReconciliation,
  } from '../../stores/collectionContext.js';
  import { useGradientStyles, loadWorkspaceGradient } from '../../stores/workspaceGradient.svelte.js';
  import QuickAddForm from './QuickAddForm.svelte';
  import { resolveCreatedItemMembership } from './collectionService.js';
  import {
    RIGHTMOST_COLUMN_LIMIT,
    boardStatusIdForItem,
    buildDisplayColumns,
    statusIdForBoardColumnMove,
    statusIsCompleted,
  } from './boardColumns.js';
  import {
    BOARD_GROUP_BY_ITEM_TYPE,
    BOARD_GROUP_BY_MILESTONE,
    MILESTONE_HIDDEN_COMPLETED_KEY,
    MILESTONE_OPEN_FROM_COMPLETED_KEY,
    boardGroupPreferenceStorageKey,
    buildBoardSwimlanes,
    legacyBoardGroupItemTypeStorageKey,
    milestonesFromItems,
    resolveStoredBoardGroupBy,
    serializeBoardGroupBy,
  } from './boardSwimlanes.js';
  import { buildBoardConfigurationPayload } from './boardConfigPayload.js';
  import {
    milestoneAssignableToWorkspace,
    milestoneDropRemovals,
    primaryLaneKeyForItem,
    SWIMLANE_NONE_KEY,
  } from './swimlanes.js';
  import { confirm } from '../../composables/useConfirm.js';
  import { resolveBoardWorkspaces } from './boardWorkspaces.js';
  import { infoToast, successToast, warningToast } from '../../stores/toasts.svelte.js';
  import Plus from '@lucide/svelte/icons/plus';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import MoreHorizontal from '@lucide/svelte/icons/more-horizontal';
  import Layers from '@lucide/svelte/icons/layers';
  import Milestone from '@lucide/svelte/icons/milestone';
  import ArrowDownUp from '@lucide/svelte/icons/arrow-down-up';
  import ItemPicker from '../../pickers/ItemPicker.svelte';
  import { buildIterationPickerConfig } from '../iterations/iterationPickerUtils.js';
  import ItemTypeIcon from '../../components/ItemTypeIcon.svelte';
  import { draggable, dropTargetForElements } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import { autoScrollForElements } from '@atlaskit/pragmatic-drag-and-drop-auto-scroll/element';
  import { attachClosestEdge, extractClosestEdge } from '@atlaskit/pragmatic-drag-and-drop-hitbox/closest-edge';
  import PersonalTaskDetail from '../personal/PersonalTaskDetail.svelte';
  import ViewHeader from '../../layout/ViewHeader.svelte';
  import StaticViewBackground from '../../layout/StaticViewBackground.svelte';
  import Button from '../../components/Button.svelte';
  import SearchInput from '../../components/SearchInput.svelte';
  import SubFilterBar from './SubFilterBar.svelte';
  import BoardColumn from './BoardColumn.svelte';
  import BoardEmptyState from './BoardEmptyState.svelte';
  import BoardItemCard from './BoardItemCard.svelte';
  import ItemKey from '../../components/ItemKey.svelte';
  import CollectionViewSwitcher from './CollectionViewSwitcher.svelte';
  import MilestoneSwimlaneConfigDialog from './MilestoneSwimlaneConfigDialog.svelte';
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
  import { backlogStore } from '../../stores/backlogStore.svelte.js';
  import { workspaceDataStore } from '../../stores/workspaceDataStore.svelte.js';
  import { workspacesStore } from '../../stores/workspaces.svelte.js';
  import { workspacePermissions } from '../../stores/workspacePermissions.svelte.js';
  import { itemModalStore } from '../../stores/itemModalStore.svelte.js';
  import { useCollectionEventStream } from '../../composables/useCollectionEventStream.svelte.js';
  import { createDependencyLoader } from './dependencyLoader.svelte.js';
  import { useCollectionSearch } from '../../composables/useCollectionSearch.svelte.js';
  import { invalidateSLAState } from '../sla/slaState.js';
  import { agentRuns } from '../../stores/agentRuns.svelte.js';
  import { getVisibleColor, hexToRgb } from '../../utils/colorUtils.js';
  import { formatDateOnly } from '../../utils/dateFormatter.js';
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

  // Search is server-scoped and shared with the list view so both find items on
  // every page (GH #220, GH #328).
  const search = useCollectionSearch(
    () => `${workspaceId ?? ''}|${collectionId ?? ''}|${collectionStore.effectiveSubFilterQL}`
  );
  let searchActive = $derived(search.active);
  let searchDebouncing = $derived(search.debouncing);
  let activeItemsHasMore = $derived(search.hasMore);
  let activeItemsLoadingMore = $derived(search.loadingMore);
  let activeItemsRemainingCount = $derived(search.remainingCount);

  let items = $derived(search.items);
  let itemsById = $derived(new Map(items.map((item) => [item.id, item])));

  // Hydrate only the assignees the visible cards reference.
  $effect(() => {
    const ids = items.map((item) => item.assignee_id).filter(Boolean);
    if (ids.length > 0) void workspaceDataStore.hydrateUsers(ids);
  });
  let transitions = $state([]);
  let boardConfig = $state(null);
  let cardFields = $derived((boardConfig?.card_fields || []).slice().sort((a, b) => a.display_order - b.display_order));
  // Milestone-swimlane preferences live on the board configuration so they are
  // shared by every viewer and survive reloads. Missing config means the
  // pre-configuration defaults: no pinned global milestones, completed shown.
  let milestoneSwimlaneConfig = $derived(
    boardConfig?.view_settings?.milestone_swimlanes ?? {
      global_milestone_ids: [],
      show_completed: true,
    }
  );
  let pinnedGlobalMilestoneIds = $derived(milestoneSwimlaneConfig.global_milestone_ids ?? []);
  let showCompletedMilestones = $derived(milestoneSwimlaneConfig.show_completed !== false);
  let showOpenFromCompletedMilestones = $derived(
    milestoneSwimlaneConfig.show_open_from_completed === true
  );
  let milestoneSwimlaneDialogOpen = $state(false);
  let savingMilestoneSwimlaneConfig = $state(false);

  let loading = $state(true);
  let currentCollectionName = $derived(collectionStore.collectionName);
  const boardColumnElements = new Set();
  let pendingDrops = new Set(); // Track pending drop operations to prevent duplicates
  let showItemModal = $state(false);
  let selectedItemId = $state(null);

  // Scope-owned dependency-link loader for hover summaries and blocker counts.
  const dependencyLoader = createDependencyLoader();

  // Quick-add state per column
  let quickAddState = $state({});
  let workspaces = $derived($workspacesStore.regularWorkspaces || []);
  // The workspace directory caches only its first page, so the personal
  // workspace is not guaranteed to be in allWorkspaces. Merge the on-demand
  // personal workspace too, otherwise a personal task opens the generic item
  // detail instead of the personal task surface.
  let personalWorkspaceIds = $derived(new Set(
    [
      ...($workspacesStore.allWorkspaces || []),
      ...($workspacesStore.personalWorkspace ? [$workspacesStore.personalWorkspace] : []),
    ]
      .filter((candidate) => candidate.is_personal)
      .map((candidate) => Number(candidate.id))
  ));
  let selectedItem = $derived(itemsById.get(selectedItemId) ?? null);
  let collectionAllowsAllWorkspaces = $derived(
    collectionStore.boardWorkspaceScopeLoaded &&
    !workspaceId &&
    Boolean(collectionId) &&
    !collectionStore.boardCollection?.workspace_id &&
    !collectionStore.boardCollection?.ql_query?.trim()
  );
  let availableWorkspaces = $derived(
    resolveBoardWorkspaces({
      scopeLoaded: collectionStore.boardWorkspaceScopeLoaded,
      allowsAllWorkspaces: collectionAllowsAllWorkspaces,
      directoryWorkspaces: workspaces,
      scopedIds: collectionStore.boardWorkspaceIds,
      currentWorkspace: workspace,
    })
  );
  // Quick-add needs item.create in the target workspace; a cross-workspace
  // board offers the affordance only while at least one target allows it, and
  // the form's workspace picker is limited to those targets (the backend
  // rejects creation elsewhere with 404).
  let creatableWorkspaces = $derived(
    availableWorkspaces.filter((workspace) => workspacePermissions.canCreate(workspace.id))
  );
  const quickAddItemTypesByWorkspace = new Map();
  let quickAddTypeLoadToken = 0;

  // Backlog functionality
  let backlogItems = $derived(collectionStore.backlogItems);

  // Iteration filter state
  let allIterations = $state([]);
  let iterationFilterId = $state(null);

  // Board grouping: null, item-type lanes, or milestone lanes.
  let boardGroupBy = $state(null);
  let excludeRightmostSwimlaneParents = $state(false);
  let swimlaneCollapsed = $state({});

  // Per-board collapsed-column preferences.
  let collapsedColumns = $state({});

  // Rank enables manual ordering; bubble prioritizes recent activity.
  let sortMode = $state('rank');

  // Edge-based drag state
  let dragState = $state(new Map()); // Track drag state for each item: { isDragging: boolean, closestEdge: 'top'|'bottom'|null }
  let boardAnnouncement = $state('');
  let boardScrollElement = $state(null);

  // Centralized gradient styling
  const styles = useGradientStyles();

  $effect(() => {
    if (!boardScrollElement) return;
    return autoScrollForElements({
      element: boardScrollElement,
      getAllowedAxis: () => 'horizontal',
    });
  });

  // An active sub-filter makes workspace equality insufficient; verify it too.
  function createdItemScope() {
    return {
      workspaceId,
      collectionId,
      subFilterQL: collectionStore.subFilterQL ? collectionStore.effectiveSubFilterQL : '',
    };
  }

  // Listen for newly created items
  async function handleRefreshWorkItems(event) {
    if (!event.detail?.itemId) return;
    try {
      const newItem = event.detail.item ?? (await itemsApi.get(event.detail.itemId));
      const membership = await resolveCreatedItemMembership(newItem, createdItemScope());
      if (membership === 'member') {
        // A create event can arrive more than once (modal event plus a delta or
        // cross-tab frame). insertCreatedItem deduplicates by id and routes an
        // item without a status to the backlog.
        collectionStore.insertCreatedItem(newItem, { backlog: !newItem.status_id });
      } else if (membership === 'unknown') {
        scheduleCollectionReconciliation();
      }
    } catch (error) {
      console.error('Failed to load new item:', error);
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
          workspaceItemTypes = await workspacesApi.getItemTypes(numericWorkspaceId);
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

  function quickAddWorkspacesForLane(lane) {
    const laneMilestoneId = lane?.dimension === BOARD_GROUP_BY_MILESTONE
      ? milestoneIdForLaneKey(lane.key)
      : null;
    const laneMilestone = laneMilestoneId == null
      ? null
      : milestoneLanesReferenceById.get(laneMilestoneId);
    // A workspace-local milestone can only be attached to an item in its own
    // workspace; global milestones work everywhere.
    return laneMilestone && !laneMilestone.is_global && laneMilestone.workspace_id != null
      ? creatableWorkspaces.filter(
          (candidate) => Number(candidate.id) === Number(laneMilestone.workspace_id)
        )
      : creatableWorkspaces;
  }

  function initQuickAdd(columnId, statusId, quickAddKey = columnId, parentItem = null, lane = null) {
    const parentId = parentItem?.id ?? null;
    const laneMilestoneId = lane?.dimension === BOARD_GROUP_BY_MILESTONE
      ? milestoneIdForLaneKey(lane.key)
      : null;
    const laneMilestone = laneMilestoneId == null
      ? null
      : milestoneLanesReferenceById.get(laneMilestoneId);
    const laneWorkspaces = quickAddWorkspacesForLane(lane);

    const preselectedWorkspaceId = parentItem?.workspace_id
      ?? laneMilestone?.workspace_id
      ?? (laneWorkspaces.length === 1 ? laneWorkspaces[0].id : null);

    quickAddState[quickAddKey] = {
      show: true,
      workspaceId: preselectedWorkspaceId,
      workspaces: laneWorkspaces,
      itemTypeId: null,
      availableTypes: [],
      loadingTypes: Boolean(preselectedWorkspaceId),
      statusId,
      parentId,
      parentItem,
      laneMilestoneId,
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
      if (state.laneMilestoneId != null) {
        payload.milestone_ids = [state.laneMilestoneId];
      }

      const newItem = await itemsApi.create(payload);

      // A collection can span several allowed workspaces while applying other
      // filters. Only add the new item locally when it matches the full view.
      const membership = await resolveCreatedItemMembership(newItem, createdItemScope());

      if (membership === 'unknown') {
        // Never guess membership after a failed verification; reconcile instead.
        scheduleCollectionReconciliation();
      } else if (membership === 'nonmember') {
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

      if (membership === 'member') {
        // The create endpoint returns the complete permission-masked item, so it
        // can be added directly without an immediate GET of the same item.
        collectionStore.insertCreatedItem(newItem);

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
    await Promise.all([
      workspaceId ? loadWorkspaceGradient(workspaceId) : Promise.resolve(),
      workspaceId
        ? workspaceDataStore.initialize(workspaceId)
        : workspaceDataStore.initializeGlobal(),
      workspacesStore.load(),
      workspacesStore.loadPersonalWorkspace(),
    ]);
    loading = false;
  });

  async function loadWorkspaceBoardState(requestedView, requestedWorkspaceId) {
    if (!requestedWorkspaceId) return;
    await workspaceDataStore.initialize(requestedWorkspaceId);
    if (requestedView !== viewSignature) return;

    try {
      const isPersonal = workspaceDataStore.workspace?.is_personal === true;
      let iters;
      if (workspaceDataStore.planningCompleteness?.iterations === true) {
        // Complete bootstrap data avoids a second full iteration fetch.
        iters = (workspaceDataStore.iterations || []).filter(
          (iteration) => !isPersonal || !iteration.is_global
        );
      } else {
        iters = await iterationsApi.getAll({
          workspace_id: requestedWorkspaceId,
          include_global: !isPersonal,
        });
      }
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
    if (
      boardGroupBy?.dimension === BOARD_GROUP_BY_ITEM_TYPE &&
      itemTypes.length > 0 &&
      !itemTypes.some(type => type.id === boardGroupBy.itemTypeId)
    ) {
      setBoardGroupBy(null);
    }
  });

  // Keep backlog count in sync
  $effect(() => {
    backlogStore.setCount(workspaceId, collectionStore.backlogPagination?.total_items ?? collectionStore.backlogItems.length);
  });

  // Reset dependency links when the viewed board changes.
  let viewSignature = $derived(`${collectionId ?? ''}|${workspaceId ?? ''}`);
  // The route props can flip between string and number representations for the
  // same board. Key the one-shot setup on the normalized view so that does not
  // reset state and re-fetch the board's iterations a second time.
  let setupViewSignature = null;
  $effect(() => {
    // Board configuration depends on the view, not the loaded item set.
    const view = viewSignature;
    if (view === setupViewSignature) return;
    setupViewSignature = view;
    dependencyLoader.reset();
    allIterations = [];
    iterationFilterId = null;
    boardGroupBy = null;
    excludeRightmostSwimlaneParents = false;
    swimlaneCollapsed = {};
    try {
      boardGroupBy = resolveStoredBoardGroupBy(
        localStorage.getItem(boardGroupStorageKey()),
        localStorage.getItem(legacyGroupByItemTypeStorageKey()),
        []
      );
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
        loadWorkspaceBoardState(view, workspaceId);
      });
    }
  });

  // Preload dependency links when the loaded item set changes. Overlapping
  // waves join pending ids; each successful chunk commits independently.
  $effect(() => {
    if (collectionStore.items.length > 0 && !collectionStore.loading) {
      // Avoid subscribing this effect to cache writes.
      untrack(() => dependencyLoader.ensure(collectionStore.items.map((item) => item.id)));
    }
  });

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

  async function saveMilestoneSwimlaneConfig(next) {
    savingMilestoneSwimlaneConfig = true;
    try {
      const payload = buildBoardConfigurationPayload(boardConfig, next);
      if (boardConfig?.id) {
        await collectionsApi.updateBoardConfiguration(
          collectionId,
          boardConfig.id,
          payload,
          workspaceId
        );
      } else {
        await collectionsApi.createBoardConfiguration(collectionId, workspaceId, payload);
      }
      collectionStore.invalidateBoardConfiguration(workspaceId, collectionId);
      await loadBoardConfig();
      // Lane membership changed; drop stale collapse state so the new lanes
      // render expanded.
      swimlaneCollapsed = {};
      milestoneSwimlaneDialogOpen = false;
      successToast(t('boardControls.milestoneSwimlanesSaved'));
    } catch (error) {
      console.error('Failed to save milestone swimlane configuration:', error);
      warningToast(error?.message || t('dialogs.alerts.failedToSave', { error: error?.message ?? '' }));
    } finally {
      savingMilestoneSwimlaneConfig = false;
    }
  }

  // Scoped SSE invalidations (WI-1624): any frame runs the existing delta fetch,
  // and a reconnect reconciles. No polling fallback.
  useCollectionEventStream(
    () => (collectionId ? { kind: 'collection', id: collectionId } : { kind: 'workspace', id: workspaceId }),
    { onInvalidate: () => scheduleCollectionReconciliation() }
  );

  // SLA badges ride a board-level cadence instead of the SSE stream. Coupling
  // them to per-change events would scale with board activity and do work for
  // users who are not showing SLA; a visibility-gated TTL tick is flat and
  // covers the wall-clock threshold crossings SSE cannot (WI-1642).
  let slaRefreshToken = $state(0);
  const showsSLAField = $derived(
    cardFields.some((field) => field.field_type === 'system' && field.field_identifier === 'sla')
  );
  $effect(() => {
    if (!showsSLAField || typeof window === 'undefined') return;
    const base = 30_000;
    const jitter = base * 0.1;
    let timer = null;
    const tick = () => {
      if (typeof document !== 'undefined' && document.visibilityState !== 'visible') return;
      invalidateSLAState();
      slaRefreshToken += 1;
    };
    const schedule = () => {
      timer = setTimeout(() => {
        tick();
        schedule();
      }, base + (Math.random() * 2 - 1) * jitter);
    };
    const onVisibility = () => {
      if (document.visibilityState === 'visible') tick();
    };
    schedule();
    document.addEventListener('visibilitychange', onVisibility);
    return () => {
      if (timer) clearTimeout(timer);
      document.removeEventListener('visibilitychange', onVisibility);
    };
  });

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

  // Workspace boards use the workspace + global milestone catalog so empty
  // active lanes can act as drop targets. Cross-workspace boards have no global
  // catalog, so derive the reference from the loaded cards (WI-1587).
  let milestoneLanesReference = $derived.by(() => {
    return workspaceId ? milestones : milestonesFromItems(items);
  });
  let milestoneLanesReferenceById = $derived(
    new Map(milestoneLanesReference.map((milestone) => [Number(milestone.id), milestone]))
  );
  // Global milestones available to pin as always-present lanes. Workspace
  // boards read the catalog; cross-workspace boards fall back to the milestones
  // embedded in the loaded cards.
  let globalMilestonesForConfig = $derived(
    milestoneLanesReference
      .filter((milestone) => milestone.is_global)
      .slice()
      .sort((a, b) => String(a.name || '').localeCompare(String(b.name || '')))
  );
  let boardGroupingActive = $derived(Boolean(boardGroupBy));

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

  function parseLaneTarget(element) {
    const dimension = element?.dataset?.swimlaneDimension || '';
    if (dimension !== BOARD_GROUP_BY_ITEM_TYPE && dimension !== BOARD_GROUP_BY_MILESTONE) {
      return null;
    }
    return {
      dimension,
      key: element.dataset.swimlaneKey || SWIMLANE_NONE_KEY,
    };
  }

  function isExcludedRightmostSwimlaneParent(item) {
    return Boolean(
      excludeRightmostSwimlaneParents &&
      rightmostBoardColumnStatusIds.has(
        boardStatusIdForItem(item, validColumns, personalWorkspaceIds)
      )
    );
  }

  function milestoneLaneKeyForItem(item) {
    return primaryLaneKeyForItem(item, BOARD_GROUP_BY_MILESTONE, milestoneLanesReference);
  }

  function milestoneIdForLaneKey(key) {
    if (key === SWIMLANE_NONE_KEY) return null;
    const id = Number(String(key).split('-').pop());
    return Number.isFinite(id) ? id : null;
  }

  function milestoneForLaneKey(key) {
    const id = milestoneIdForLaneKey(key);
    if (id == null) return null;
    return (
      milestoneLanesReferenceById.get(id) ??
      boardSwimlanes.find((lane) => lane.milestone?.id === id)?.milestone ??
      null
    );
  }

  function milestoneNameForId(id, item) {
    return (
      item?.milestones?.find((milestone) => milestone.id === id)?.name ??
      milestoneLanesReferenceById.get(id)?.name ??
      `Milestone #${id}`
    );
  }

  function wouldChangeBoardLane(item, target) {
    if (!boardGroupBy || !target) return false;
    if (target.dimension === BOARD_GROUP_BY_ITEM_TYPE) {
      const targetParentId =
        target.key === SWIMLANE_NONE_KEY ? null : Number(String(target.key).split('-').pop());
      return (item.parent_id ?? null) !== targetParentId;
    }
    if (target.dimension === BOARD_GROUP_BY_MILESTONE) {
      // Aggregate lanes are not milestone assignments; a drop there keeps the
      // card's milestone and only changes its column/rank.
      if (
        target.key === MILESTONE_HIDDEN_COMPLETED_KEY ||
        target.key === MILESTONE_OPEN_FROM_COMPLETED_KEY
      ) {
        return false;
      }
      return milestoneLaneKeyForItem(item) !== target.key;
    }
    return false;
  }

  function getItemsForBoardLane(lane) {
    if (!boardGroupBy) return filteredItems;

    if (boardGroupBy.dimension === BOARD_GROUP_BY_ITEM_TYPE) {
      if (!lane || lane.dimension !== BOARD_GROUP_BY_ITEM_TYPE) return filteredItems;
      if (lane.key !== SWIMLANE_NONE_KEY) {
        const parentId = Number(String(lane.key).split('-').pop());
        if (!Number.isFinite(parentId)) return [];
        return filteredItems.filter((item) => item.parent_id === parentId && item.id !== parentId);
      }
      const parentIds = new Set(items
        .filter((item) => item.item_type_id === boardGroupBy.itemTypeId && !isExcludedRightmostSwimlaneParent(item))
        .map((item) => item.id));
      return filteredItems.filter((item) => {
        if (item.item_type_id === boardGroupBy.itemTypeId) return false;
        if (item.parent_id && parentIds.has(item.parent_id)) return false;
        return true;
      });
    }

    if (boardGroupBy.dimension === BOARD_GROUP_BY_MILESTONE) {
      if (!lane || lane.dimension !== BOARD_GROUP_BY_MILESTONE) return filteredItems;
      if (
        lane.key === MILESTONE_HIDDEN_COMPLETED_KEY ||
        lane.key === MILESTONE_OPEN_FROM_COMPLETED_KEY
      ) {
        return boardSwimlanes.find((entry) => entry.key === lane.key)?.items ?? [];
      }
      return filteredItems.filter((item) => milestoneLaneKeyForItem(item) === lane.key);
    }

    return filteredItems;
  }

  function warnUnsupportedCombinedBoardMove() {
    warningToast(t('collections.combinedBoardMoveUnsupported'));
  }

  function markSwimlaneMoveFailed(error) {
    if (error && typeof error === 'object') {
      error.swimlaneMoveFailed = true;
    }
    return error;
  }

  async function updateItemParentForLane(item, target) {
    if (boardGroupBy?.dimension !== BOARD_GROUP_BY_ITEM_TYPE) return item;
    if (!target || target.dimension !== BOARD_GROUP_BY_ITEM_TYPE) return item;

    const targetParentId =
      target.key === SWIMLANE_NONE_KEY ? null : Number(String(target.key).split('-').pop());
    const currentParentId = item.parent_id ?? null;
    if (currentParentId === targetParentId) return item;

    try {
      const updatedItem = await itemsApi.update(item.id, { parent_id: targetParentId });
      collectionStore.patchItem(item.id, { ...updatedItem, parent_id: targetParentId });
      return { ...item, ...updatedItem, parent_id: targetParentId };
    } catch (error) {
      console.error('Failed to move item to swimlane:', error);
      warningToast(t('collections.swimlaneMoveFailed'));
      throw markSwimlaneMoveFailed(error);
    }
  }

  async function updateItemMilestoneLane(item, target) {
    if (boardGroupBy?.dimension !== BOARD_GROUP_BY_MILESTONE) return item;
    if (!target || target.dimension !== BOARD_GROUP_BY_MILESTONE) return item;
    if (!wouldChangeBoardLane(item, target)) return item;

    const targetId = milestoneIdForLaneKey(target.key);
    const milestoneIds = targetId == null ? [] : [targetId];

    if (targetId != null) {
      const targetMilestone = milestoneLanesReferenceById.get(targetId);
      if (
        targetMilestone &&
        !milestoneAssignableToWorkspace(targetMilestone, item.workspace_id)
      ) {
        warningToast(t('collections.milestoneWorkspaceMismatch'));
        const error = new Error('milestone workspace mismatch');
        throw markSwimlaneMoveFailed(error);
      }
    }

    try {
      const updatedItem = await itemsApi.update(item.id, { milestone_ids: milestoneIds });
      collectionStore.patchItem(item.id, updatedItem);
      return { ...item, ...updatedItem };
    } catch (error) {
      console.error('Failed to move item to milestone swimlane:', error);
      warningToast(t('collections.swimlaneMoveFailed'));
      throw markSwimlaneMoveFailed(error);
    }
  }

  async function applyLaneTarget(item, target) {
    if (!boardGroupBy || !target) return item;
    if (boardGroupBy.dimension === BOARD_GROUP_BY_ITEM_TYPE) {
      return updateItemParentForLane(item, target);
    }
    if (boardGroupBy.dimension === BOARD_GROUP_BY_MILESTONE) {
      return updateItemMilestoneLane(item, target);
    }
    return item;
  }

  async function confirmMilestoneLaneDrop(item, target) {
    if (target?.dimension !== BOARD_GROUP_BY_MILESTONE) return true;
    if (!wouldChangeBoardLane(item, target)) return true;

    const removedIds = milestoneDropRemovals(
      item,
      BOARD_GROUP_BY_MILESTONE,
      target.key,
      milestoneLanesReference
    );
    if (!removedIds?.length) return true;

    const removedNames = removedIds
      .map((id) => milestoneNameForId(id, item))
      .join(', ');
    const targetMilestone = milestoneForLaneKey(target.key);
    const message = target.key === SWIMLANE_NONE_KEY
      ? t('collections.milestoneLaneConfirmUnassign', { milestones: removedNames })
      : t('collections.milestoneLaneConfirmAssign', {
          milestone: targetMilestone?.name || t('common.unassigned'),
          milestones: removedNames,
        });

    return confirm({
      title: t('collections.milestoneLaneConfirmTitle'),
      message,
      confirmText: t('collections.milestoneLaneConfirmAction'),
      cancelText: t('common.cancel'),
      variant: 'warning',
    });
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

  function boardGroupStorageKey() {
    return boardGroupPreferenceStorageKey(boardPreferenceScope());
  }

  function legacyGroupByItemTypeStorageKey() {
    return legacyBoardGroupItemTypeStorageKey(boardPreferenceScope());
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

  function setBoardGroupBy(groupBy) {
    boardGroupBy = groupBy;
    swimlaneCollapsed = {};
    try {
      const serialized = serializeBoardGroupBy(groupBy);
      if (serialized) {
        localStorage.setItem(boardGroupStorageKey(), serialized);
      } else {
        localStorage.removeItem(boardGroupStorageKey());
      }
      localStorage.removeItem(legacyGroupByItemTypeStorageKey());
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
  // Statuses in completed categories. A card in one of these is treated as
  // closed for the milestone rescue lane.
  let completedStatusIds = $derived(
    new Set(statuses.filter((status) => statusIsCompleted(status)).map((status) => Number(status.id)))
  );

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
      !boardGroupingActive &&
      !search.query.trim() &&
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
    boardGroupBy?.dimension === BOARD_GROUP_BY_ITEM_TYPE
      ? itemTypes.find(type => type.id === boardGroupBy.itemTypeId) ?? null
      : null
  );
  let selectedGroupByLabel = $derived(
    boardGroupBy?.dimension === BOARD_GROUP_BY_MILESTONE
      ? t('boardControls.milestoneSwimlanes')
      : selectedGroupByItemType
        ? t('boardControls.groupByValue', { name: selectedGroupByItemType.name })
        : t('boardControls.groupBy')
  );

  let hiddenRightmostSwimlaneParentCount = $derived.by(() => {
    if (
      boardGroupBy?.dimension !== BOARD_GROUP_BY_ITEM_TYPE ||
      !excludeRightmostSwimlaneParents ||
      !rightmostBoardColumn
    ) return 0;
    return items.filter(
      item => item.item_type_id === boardGroupBy.itemTypeId && isExcludedRightmostSwimlaneParent(item)
    ).length;
  });

  let sortByMenuItems = $derived([
    {
      id: 'sort-rank',
      testid: 'board-sort-rank',
      title: t('boardControls.rankMode'),
      subtitle: t('boardControls.rankModeDescription'),
      selected: sortMode === 'rank',
      onClick: () => setSortMode('rank')
    },
    {
      id: 'sort-bubble',
      testid: 'board-sort-bubble',
      title: t('boardControls.bubbleMode'),
      subtitle: t('boardControls.bubbleModeDescription'),
      selected: sortMode === 'bubble',
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
        selected: !boardGroupBy,
        onClick: () => setBoardGroupBy(null)
      },
      {
        id: 'group-milestone',
        testid: 'board-group-by-milestone',
        title: t('boardControls.milestoneSwimlanes'),
        subtitle: t('boardControls.milestoneSwimlanesDescription'),
        selected: boardGroupBy?.dimension === BOARD_GROUP_BY_MILESTONE,
        onClick: () => setBoardGroupBy({ dimension: BOARD_GROUP_BY_MILESTONE })
      },
      ...(boardGroupBy?.dimension === BOARD_GROUP_BY_ITEM_TYPE ? [
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
        selected:
          boardGroupBy?.dimension === BOARD_GROUP_BY_ITEM_TYPE && boardGroupBy.itemTypeId === type.id,
        onClick: () => setBoardGroupBy({ dimension: BOARD_GROUP_BY_ITEM_TYPE, itemTypeId: type.id })
      })),
      { id: 'group-config-divider', type: 'divider' },
      {
        id: 'group-milestone-config',
        testid: 'board-group-by-configure',
        title: t('boardControls.configureMilestones'),
        subtitle: t('boardControls.configureMilestonesDescription'),
        onClick: () => { milestoneSwimlaneDialogOpen = true; }
      }
    ];
  });

  let boardSwimlanes = $derived(
    buildBoardSwimlanes({
      groupBy: boardGroupBy,
      items,
      filteredItems,
      milestones: milestoneLanesReference,
      statusIdForItem: (item) =>
        boardStatusIdForItem(item, validColumns, personalWorkspaceIds),
      rightmostStatusIds: rightmostBoardColumnStatusIds,
      excludeRightmostSwimlaneParents,
      includeEmptyMilestones: true,
      globalMilestoneIds: pinnedGlobalMilestoneIds,
      showCompletedMilestones,
      showOpenFromCompletedMilestones,
      isItemOpen: (item) =>
        !completedStatusIds.has(
          Number(boardStatusIdForItem(item, validColumns, personalWorkspaceIds))
        ),
      unassignedTitle: t('common.unassigned'),
      hiddenCompletedTitle: t('boardControls.completedMilestones'),
      openFromCompletedTitle: t('boardControls.openFromCompletedMilestones'),
      allItemsTitle: t('collections.allItems'),
    })
  );

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
      '#3b82f6': 'text-ds-text-accent-blue',
      '#ef4444': 'text-ds-text-accent-red',
      '#10b981': 'text-ds-text-accent-green',
      '#f59e0b': 'text-ds-text-accent-orange',
      '#6b7280': 'text-ds-text'
    };
    return colorMap[categoryColor] || 'text-ds-text';
  }

  function openItem(itemId, event) {
    // Prevent event bubbling to avoid triggering drag
    event.stopPropagation();
    const item = itemsById.get(itemId);
    if (item && personalWorkspaceIds.has(Number(item.workspace_id))) {
      selectedItemId = itemId;
      showItemModal = true;
      return;
    }
    itemModalStore.open(itemId, item?.workspace_id ?? workspaceId);
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
      const transitionedItem = await itemsApi.transition(item.id, targetStatusId);
      mergeLocalItem(item.id, transitionedItem);
      boardAnnouncement = `Moved ${item.title} to ${targetName}`;
      successToast(boardAnnouncement);
      reloadCollection();
    } catch (err) {
      updateLocalItemStatus(item.id, previousStatusId);
      console.error('Status transition failed:', err);
      warningToast(err?.message || t('collections.transition_failed'));
      reloadCollection();
    }
  }

  function closeItemModal() {
    showItemModal = false;
    selectedItemId = null;
  }

  function registerBoardCard(element, [itemId, canDrag]) {
    dragState.set(itemId, { isDragging: false, closestEdge: null });

    let draggableCleanup = null;
    function attachDraggable(canDragValue) {
      draggableCleanup?.();
      draggableCleanup = null;
      if (!canDragValue) return;
      draggableCleanup = draggable({
        element,
        getInitialData: () => ({ item: itemsById.get(itemId), type: 'work-item' }),
        onDragStart: () => {
          element.style.opacity = '0.5';
          document.body.classList.add('is-dragging');
          const state = dragState.get(itemId) || {};
          dragState.set(itemId, { ...state, isDragging: true });
          dragState = new Map(dragState);
        },
        onDrop: () => {
          element.style.opacity = '';
          document.body.classList.remove('is-dragging');
          dragState = new Map([...dragState].map(([id]) => [id, { isDragging: false, closestEdge: null }]));
          resetAllColumnStyles();
        }
      });
    }

    // Re-evaluated when the canDrag flag flips (e.g. permissions finish loading).
    attachDraggable(canDrag);

    const dropTargetCleanup = dropTargetForElements({
      element,
      canDrop: ({ source }) => {
        const data = /** @type {any} */ (source.data);
        const item = itemsById.get(itemId);
        if (!item || data.type !== 'work-item' || data.item.id === itemId) return false;
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
      getData: ({ input, element: targetElement }) => attachClosestEdge({}, {
        input,
        element: targetElement,
        allowedEdges: ['top', 'bottom']
      }),
      onDragEnter: ({ self, source }) => {
        const data = /** @type {any} */ (source.data);
        if (data.type !== 'work-item' || data.item.id === itemId) return;
        const state = dragState.get(itemId) || {};
        dragState.set(itemId, { ...state, closestEdge: extractClosestEdge(self.data) });
        dragState = new Map(dragState);
      },
      onDragLeave: () => {
        const state = dragState.get(itemId) || {};
        dragState.set(itemId, { ...state, closestEdge: null });
        dragState = new Map(dragState);
      },
      onDrop: ({ self, source }) => {
        const data = /** @type {any} */ (source.data);
        const closestEdge = extractClosestEdge(self.data);
        const item = itemsById.get(itemId);
        const targetStatus = getStatusByItemId(itemId);
        if (item && data.type === 'work-item' && closestEdge && targetStatus) {
          handleEdgeBasedDrop(
            data.item,
            item,
            closestEdge,
            targetStatus,
            parseLaneTarget(element)
          );
        }
      }
    });

    return {
      update([, nextCanDrag]) {
        attachDraggable(nextCanDrag);
      },
      destroy() {
        element.style.opacity = '';
        document.body.classList.remove('is-dragging');
        draggableCleanup?.();
        dropTargetCleanup();
        dragState.delete(itemId);
      }
    };
  }

  function registerBoardColumn(element, _statusId) {
    const getStatusId = () => Number(element.dataset.statusId);
    boardColumnElements.add(element);
    const cleanup = dropTargetForElements({
      element,
      canDrop: ({ source }) => {
        const data = /** @type {any} */ (source.data);
        const targetColumn = validColumns.find(column => column.status_ids?.[0] === getStatusId());
        return Boolean(
          targetColumn &&
          data.type === 'work-item' &&
          statusIdForBoardColumnMove(data.item, targetColumn, validColumns, personalWorkspaceIds) != null
        );
      },
      onDragEnter: ({ source }) => {
        const data = /** @type {any} */ (source.data);
        const targetColumn = validColumns.find(column => column.status_ids?.[0] === getStatusId());
        if (
          targetColumn &&
          data.type === 'work-item' &&
          statusIdForBoardColumnMove(data.item, targetColumn, validColumns, personalWorkspaceIds) != null
        ) {
          element.style.boxShadow = 'inset 0 0 0 2px var(--ds-border-focused)';
        }
      },
      onDragLeave: () => {
        element.style.boxShadow = '';
      },
      onDrop: async ({ source, location }) => {
        resetAllColumnStyles();
        const data = /** @type {any} */ (source.data);
        const targetColumn = validColumns.find(column => column.status_ids?.[0] === getStatusId());
        if (!targetColumn || data.type !== 'work-item') return;
        const dropTargets = location.current.dropTargets;
        if (dropTargets.length > 1 && dropTargets[0].element !== element) return;

        const transitionStatusId = statusIdForBoardColumnMove(
          data.item,
          targetColumn,
          validColumns,
          personalWorkspaceIds,
        );
        if (transitionStatusId == null) return;
        const laneTarget = parseLaneTarget(element);
        const isSameStatus = data.item.status_id === transitionStatusId;
        if (
          !isSameStatus &&
          boardGroupBy?.dimension === BOARD_GROUP_BY_ITEM_TYPE &&
          wouldChangeBoardLane(data.item, laneTarget)
        ) {
          warnUnsupportedCombinedBoardMove();
          return;
        }
        if (!(await confirmMilestoneLaneDrop(data.item, laneTarget))) return;

        const previousStatusId = data.item.status_id;
        if (!isSameStatus) updateLocalItemStatus(data.item.id, transitionStatusId);
        try {
          let droppedItem = data.item;
          if (!isSameStatus) {
            droppedItem = await itemsApi.transition(data.item.id, transitionStatusId);
            mergeLocalItem(data.item.id, droppedItem);
          }
          await applyLaneTarget(droppedItem, laneTarget);
        } catch (err) {
          if (!isSameStatus) updateLocalItemStatus(data.item.id, previousStatusId);
          console.error('Board drop failed:', err);
          if (!err?.swimlaneMoveFailed) warningToast(err?.message || t('collections.transition_failed'));
        }
        reloadCollection();
      }
    });

    return {
      destroy() {
        boardColumnElements.delete(element);
        element.style.boxShadow = '';
        cleanup();
      }
    };
  }

  // Helper functions
  function resetAllColumnStyles() {
    boardColumnElements.forEach(element => {
      element.style.boxShadow = '';
    });
  }

  function getStatusByItemId(itemId) {
    const item = itemsById.get(itemId);
    if (!item) return null;
    const boardStatusId = boardStatusIdForItem(item, validColumns, personalWorkspaceIds);
    return statuses.find(s => s.id === boardStatusId);
  }

  function getBoardColumnForItem(item) {
    const boardStatusId = boardStatusIdForItem(item, validColumns, personalWorkspaceIds);
    return validColumns.find(column => column.status_ids?.includes(boardStatusId));
  }

  function updateLocalItemStatus(itemId, statusId) {
    collectionStore.patchItem(itemId, { status_id: statusId });
  }

  function mergeLocalItem(itemId, updatedItem) {
    if (!updatedItem) return;
    collectionStore.patchItem(itemId, updatedItem);
  }

  async function handleEdgeBasedDrop(draggedItem, targetItem, closestEdge, targetStatus, laneTarget = null) {
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

      if (
        !isSameStatus &&
        boardGroupBy?.dimension === BOARD_GROUP_BY_ITEM_TYPE &&
        wouldChangeBoardLane(draggedItem, laneTarget)
      ) {
        warnUnsupportedCombinedBoardMove();
        reloadCollection();
        return;
      }

      if (!(await confirmMilestoneLaneDrop(draggedItem, laneTarget))) {
        return;
      }

      // If changing status, update the status first
      if (!isSameStatus) {
        updateLocalItemStatus(draggedItem.id, targetStatusId);
        try {
          const transitionedItem = await itemsApi.transition(draggedItem.id, targetStatusId);
          draggedItem = { ...draggedItem, ...transitionedItem, status_id: targetStatusId };
          mergeLocalItem(draggedItem.id, draggedItem);
        } catch (err) {
          updateLocalItemStatus(draggedItem.id, currentStatusId);
          console.error('Status transition failed:', err);
          warningToast(err?.message || t('collections.transition_failed'));
          reloadCollection();
          return;
        }
      }

      draggedItem = await applyLaneTarget(draggedItem, laneTarget);

      // Bubble Mode disables manual rank ordering. Cross-column drags above have
      // already transitioned status (which bumps the card to the top via
      // last_active_at); a same-column drag is a no-op. Either way, skip the
      // frac_index reorder and just refresh.
      if (sortMode === 'bubble') {
        if (isSameStatus && !wouldChangeBoardLane(draggedItem, laneTarget)) {
          infoToast(
            t('collections.manual_sort_unavailable'),
            t('collections.manual_sort_unavailable_title')
          );
        }
        reloadCollection();
        return;
      }

      // Get items in the target status for position calculation
      const laneItems = boardGroupingActive ? getItemsForBoardLane(laneTarget) : filteredItems;
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
        previous_item_id: prevItemId,
        next_item_id: nextItemId
      };
      const updatedItem = await itemsApi.updateFracIndex(draggedItem.id, indexData);

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


</script>

{#if loading || collectionStore.loading || workspaceDataStore.initialLoading}
  <div class="p-6">
    <div class="animate-pulse">{t('common.loading')}</div>
  </div>
{:else if workspace || !workspaceId}
  <!-- The board chrome (view header and filters) is fixed to the viewport
       width; only the lane canvas scrolls, so wide boards pan underneath a
       toolbar that never has to be scrolled to be reached. -->
  <StaticViewBackground
    backgroundStyle={styles.backgroundStyle}
    contextVars={styles.contextVars}
    fullWidth
    rootClass="h-full"
    contentClass="flex h-full flex-col"
    testid="collection-board-background"
  >
      <!-- Header with view tabs. The chrome stays in normal flow above the
           scrolling canvas so the toolbar never pans with the lanes. -->
      <div class="flex-shrink-0 px-6 pt-6" data-testid="board-header">
        <ViewHeader
          workspaceName={workspace?.name || ''}
          collection={currentCollectionName === 'Default' ? t('common.default') : currentCollectionName}
          viewName={t('workspaceSettings.views.board')}
          itemCount={collectionStore.collectionTotal}
          shownCount={collectionStore.loading ? null : totalVisibleItems}
          marginClass="mb-6"
        >
          {#snippet actions()}
            <div class="flex flex-wrap items-center justify-end gap-3">
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
                  triggerText={selectedGroupByLabel}
                  placement="bottom-end"
                  maxWidth="max-w-xs"
                  triggerClass="px-3 py-1.5 rounded text-sm font-medium transition-colors {boardGroupingActive ? 'shadow-sm' : ''}"
                  triggerStyle={boardGroupingActive ? 'color: var(--ds-text); background-color: var(--ctx-surface-raised, var(--ds-surface-raised));' : 'color: var(--ds-text);'}
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

        <!-- Controls Bar -->
        <div class="flex flex-wrap items-center gap-4" data-testid="board-controls">
          <SearchInput
            bind:value={search.query}
            placeholder={t('common.search')}
            dataTestid="board-search-input"
          />
          <SubFilterBar {workspaceId} showCompletionToggle={false} />
        </div>
      </div>

      <!-- Scrolling canvas: everything below the fixed chrome pans in both
           axes; the horizontal scrollbar sits at the bottom of the area. -->
      <div
        bind:this={boardScrollElement}
        class="min-h-0 min-w-0 max-w-full flex-1 w-full overflow-auto overscroll-contain"
        data-testid="board-scroll-container"
      >
      <div class="px-6 py-6 min-w-fit">

      {#if searchActive && (searchDebouncing || collectionStore.searchLoading)}
        <p class="-mt-2 mb-4 text-sm" style="color: var(--ds-text-subtle);" data-testid="board-search-status">
          {t('common.searching')}
        </p>
      {:else if searchActive && collectionStore.searchError}
        <p class="-mt-2 mb-4 text-sm" style="color: var(--ds-text-danger);" data-testid="board-search-error">
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
            class="px-4 py-2 bg-ds-interactive text-white rounded hover:bg-ds-interactive-hovered transition-colors"
          >
            {t('statuses.createStatus')}
          </button>
        </div>
      {:else}
        <!-- Board Columns / Swimlanes -->
        <div
          class={boardGroupingActive ? 'space-y-4' : ''}
          data-testid="board-view"
        >
          {#each boardSwimlanes as lane (lane.id)}
            {@const laneExpanded = isSwimlaneExpanded(lane.id)}
            <!-- No type can legally be created under this lane's item (the lane
                 groups by the lowest hierarchy level), so the column loses its
                 add affordance rather than opening a form that cannot submit. -->
            {@const laneQuickAddTypes = quickAddTypesFor(lane.parent ?? null)}
            <section
              class={boardGroupingActive ? 'rounded-lg border overflow-hidden' : ''}
              style={boardGroupingActive ? `${styles.glassStyle?.(10) ?? ''} border-color: var(--ctx-border, var(--ds-border));` : ''}
              data-board-swimlane={lane.id}
              data-testid={`board-lane-${lane.id}`}
            >
              {#if boardGroupingActive}
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
                    {#if lane.dimension === BOARD_GROUP_BY_ITEM_TYPE && selectedGroupByItemType}
                      <ItemTypeIcon
                        icon={selectedGroupByItemType.icon}
                        color={lane.isUnassigned ? 'var(--ds-background-neutral-bold, #6b7280)' : selectedGroupByItemType.color}
                        title={selectedGroupByItemType.name}
                      />
                    {:else if lane.dimension === BOARD_GROUP_BY_MILESTONE}
                      <Milestone
                        class="w-4 h-4 flex-shrink-0"
                        style="color: {lane.isRescue
                          ? 'var(--ds-text-warning)'
                          : lane.isUnassigned || lane.isAggregate
                            ? 'var(--ds-background-neutral-bold, #6b7280)'
                            : (lane.milestone?.category_color || 'var(--ds-text-subtle)')};"
                      />
                    {/if}
                    <span class="min-w-0 flex-1 text-left">
                      <span class="block font-semibold truncate">{lane.title}</span>
                      {#if lane.parent}
                        <span class="block text-xs font-normal truncate" style="color: var(--ds-text-subtle);">
                          <ItemKey item={lane.parent} {workspace} clickable={false} /> · {t('collections.itemTypeLaneHint', { type: selectedGroupByItemType?.name ?? '' })}
                        </span>
                      {:else if lane.dimension === BOARD_GROUP_BY_MILESTONE}
                        <span class="block text-xs font-normal truncate" style="color: var(--ds-text-subtle);">
                          {#if lane.isUnassigned}
                            {t('collections.milestoneLaneUnassignedHint')}
                          {:else if lane.isRescue}
                            {t('collections.milestoneLaneOpenFromCompletedHint')}
                          {:else if lane.isAggregate}
                            {t('collections.milestoneLaneHiddenCompletedHint')}
                          {:else if lane.milestone?.target_date}
                            {formatDateOnly(lane.milestone.target_date)}
                          {:else}
                            {t('collections.milestoneLaneHint')}
                          {/if}
                        </span>
                      {:else}
                        <span class="block text-xs font-normal truncate" style="color: var(--ds-text-subtle);">
                          {excludeRightmostSwimlaneParents
                            ? t('collections.itemTypeLaneUnassignedVisibleHint', { type: selectedGroupByItemType?.name ?? '' })
                            : t('collections.itemTypeLaneUnassignedHint', { type: selectedGroupByItemType?.name ?? '' })}
                        </span>
                      {/if}
                    </span>
                  </Button>
                  <span class="text-xs px-2 py-0.5 rounded-full flex-shrink-0" style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                    {lane.itemCount} {lane.itemCount === 1 ? 'item' : 'items'}
                  </span>
                </div>
              {/if}

              {#if !boardGroupingActive || laneExpanded}
                <div class="grid gap-6 {boardGroupingActive ? 'p-4' : ''}" style="grid-template-columns: {boardGridTemplate()};">
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
                        use:registerBoardColumn={column.status_ids[0]}
                        type="button"
                        class="relative rounded border shadow-sm flex flex-col items-center justify-between gap-2 pt-5 pb-3 px-1 text-center cursor-pointer transition-colors"
                        style="{styles.columnStyle(12)} border-top: 4px solid {column.color};"
                        data-testid="board-column"
                        id={`board-column-status-${column.status_ids[0]}`}
                        data-status-column
                        data-status-column-key={`${lane.id}-${column.id}-${column.status_ids[0]}`}
                        data-swimlane-dimension={lane.dimension ?? ''}
                        data-swimlane-key={lane.key ?? ''}
                        data-status-id={column.status_ids[0]}
                        aria-label={t('collections.expandColumn', { name: objectDisplayName(column, 'status') })}
                        aria-expanded="false"
                        title={t('collections.expandColumn', { name: objectDisplayName(column, 'status') })}
                        onclick={() => toggleColumnCollapse(column.id)}
                      >
                        <span class="sr-only" data-testid={`board-column-status-${column.status_ids[0]}`} aria-hidden="true"></span>
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
                      swimlaneDimension={lane.dimension ?? ''}
                      swimlaneKey={lane.key ?? ''}
                      statusId={column.status_ids[0]}
                      quickAddOpen={quickAddState[quickAddKey]?.show}
                      columnStyle={styles.columnStyle(12)}
                      textStyle={styles.glassTextStyle}
                      subtleTextStyle={styles.glassSubtleTextStyle}
                      dndAction={registerBoardColumn}
                      onadd={laneQuickAddTypes.length > 0 && quickAddWorkspacesForLane(lane).length > 0
                        ? () => initQuickAdd(column.id, column.status_ids[0], quickAddKey, lane.parent ?? null, lane)
                        : null}
                      oncollapse={() => toggleColumnCollapse(column.id)}
                    >
                        {#if quickAddState[quickAddKey]?.show}
                          <div class="mb-3">
                            <QuickAddForm
                              parentId={quickAddKey}
                              formState={quickAddState[quickAddKey]}
                              workspaces={quickAddState[quickAddKey]?.workspaces ?? creatableWorkspaces}
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
                              {@const canDragCard = workspacePermissions.canEdit(item.workspace_id)}
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
                                dependencyLinks={dependencyLoader.merged(item.id)}
                                moveMenuItems={canDragCard ? moveMenuItems : []}
                                showMoveMenu={canDragCard}
                                closestEdge={dragState.get(item.id)?.closestEdge}
                                swimlaneDimension={lane.dimension ?? ''}
                                swimlaneKey={lane.key ?? ''}
                                cardStyle={styles.cardStyle(0)}
                                textStyle={styles.glassTextStyle}
                                dndAction={registerBoardCard}
                                canDrag={canDragCard}
                                refreshToken={slaRefreshToken}
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
                ? search.loadMore()
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
        <div class="mt-8 text-center" data-testid="board-summary">
          <p class="text-sm" style="color: var(--ctx-text-subtle, var(--ds-text-subtle));">
            {t('collections.boardSummary', { itemCount: totalVisibleItems, columnCount: displayColumns.length })}
          </p>
        </div>
      {/if}
      </div>
      </div>
    </StaticViewBackground>
{:else}
  <div class="p-6">
    <div class="text-center" style="color: var(--ds-text-subtle);">
      {t('workspaces.noWorkspaces')}
    </div>
  </div>
{/if}

<!-- Personal task detail stays local: it renders a different component. -->
{#if showItemModal && selectedItemId && selectedItem && personalWorkspaceIds.has(Number(selectedItem.workspace_id))}
  <PersonalTaskDetail
    itemId={selectedItemId}
    workspaceId={selectedItem.workspace_id}
    onclose={closeItemModal}
    onupdate={reloadCollection}
  />
{/if}

<MilestoneSwimlaneConfigDialog
  isOpen={milestoneSwimlaneDialogOpen}
  globalMilestones={globalMilestonesForConfig}
  selectedGlobalMilestoneIds={pinnedGlobalMilestoneIds}
  showCompleted={showCompletedMilestones}
  showOpenFromCompleted={showOpenFromCompletedMilestones}
  saving={savingMilestoneSwimlaneConfig}
  onclose={() => (milestoneSwimlaneDialogOpen = false)}
  onsave={saveMilestoneSwimlaneConfig}
/>

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
