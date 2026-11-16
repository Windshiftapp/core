<script>
  import VirtualRows from '../../components/VirtualRows.svelte';
  import BacklogItemRow from './BacklogItemRow.svelte';
  import { projectBacklogRows } from './backlogVirtualRows';
  import { claimVirtualDrop, virtualDropAllowed } from '../../virtualization/virtualDragSession';
  import { onMount } from 'svelte';
  import { items } from '../../api/items.js';
  import { iterations } from '../../api/milestones.js';
  import { t } from '../../stores/i18n.svelte.js';
  import {
    collectionStore,
    reloadCollection,
    scheduleCollectionReconciliation,
  } from '../../stores/collectionContext.js';
  import { resolveCreatedItemMembership } from './collectionService.js';
  import { useGradientStyles, loadWorkspaceGradient } from '../../stores/workspaceGradient.svelte.js';
  import List from '@lucide/svelte/icons/list';
  import Plus from '@lucide/svelte/icons/plus';
  import EmptyState from '../../components/EmptyState.svelte';
  import Toggle from '../../components/Toggle.svelte';
  import { dropTargetForElements } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import { registerReorderable } from '../../composables/reorderableDrag.js';
  import ViewHeader from '../../layout/ViewHeader.svelte';
  import StaticViewBackground from '../../layout/StaticViewBackground.svelte';
  import SubFilterBar from './SubFilterBar.svelte';
  import CollectionViewSwitcher from './CollectionViewSwitcher.svelte';
  import BacklogIterationSection from './BacklogIterationSection.svelte';
  import { sortBacklogIterations } from './backlogIterationOrder.js';
  import ItemPicker from '../../pickers/ItemPicker.svelte';
  import { backlogStore } from '../../stores/backlogStore.svelte.js';
  import { onWorkItemRefresh } from '../../stores/workItemRefresh.js';
  import { workspaceDataStore } from '../../stores/workspaceDataStore.svelte.js';
  import { collectionPreferences } from '../../stores/collectionPreferences.svelte.js';
  import { itemModalStore } from '../../stores/itemModalStore.svelte.js';
  import { useCollectionEventStream } from '../../composables/useCollectionEventStream.svelte.js';
  import { useCollectionDragDrop } from '../../composables/useCollectionDragDrop.js';
  import { useItemBulkSelection } from '../../composables/useItemBulkSelection.svelte.js';
  import { errorToast, successToast, warningToast } from '../../stores/toasts.svelte.js';
  import { getIncompleteIterationItems } from './iterationCompletion.js';
  import CompleteIterationDialog from '../../dialogs/CompleteIterationDialog.svelte';
  import { formatItemCount } from '../../utils/itemCount.js';
  import { isSystemFieldAvailableInEffectiveConfig } from '../../utils/screenFields.js';
  import BacklogFieldSelector from './BacklogFieldSelector.svelte';
  import BulkActionToolbar from './BulkActionToolbar.svelte';
  import ButtonGroup from '../../components/ButtonGroup.svelte';
  import CheckSquare from '@lucide/svelte/icons/square-check';
  import { useListColumnConfig } from './listColumnConfig.svelte.js';
  import { buildDisplayColumns } from './boardColumns.js';
  import { objectDisplayName } from '../../utils/systemLabels.js';
  import { useEventListener } from 'runed';
  import { DEFAULT_BACKLOG_FIELDS, normalizeBacklogFields } from './backlogFields.js';

  let { workspaceId, collectionId = null } = $props();
  let backlogListElement = $state(null);

  // Reference data from shared workspace store
  let workspace = $derived(workspaceDataStore.workspace);
  let itemTypes = $derived(workspaceDataStore.itemTypes);
  let statuses = $derived(workspaceDataStore.statuses);
  let statusCategories = $derived(workspaceDataStore.statusCategories);
  let priorities = $derived(workspaceDataStore.priorities);
  let wdsLabels = $derived(workspaceDataStore.labels);

  let backlogItems = $derived(collectionStore.backlogItems);
  let backlogItemsById = $derived(new Map(backlogItems.map((item) => [item.id, item])));

  // Multi-select mirrors the board and list: the same composable owns the
  // selection set and bulk operations so every view exposes identical actions.
  const selection = useItemBulkSelection({
    items: () => backlogItems,
    scopeWorkspaceId: () => workspaceId,
    reload: reloadCollection,
    labelNameLookup: () => wdsLabels,
  });

  const columnConfig = useListColumnConfig(() => ({ workspaceId, collectionId }));
  let bulkMoveTargets = $derived(
    buildDisplayColumns(columnConfig.boardConfig, statuses)
      .filter((column) => column.status_ids?.length > 0)
      .map((column) => ({
        id: column.id,
        statusId: column.status_ids[0],
        title: objectDisplayName(column, 'status'),
        color: column.color,
      }))
  );
  let loading = $state(true);
  let currentCollectionName = $derived(collectionStore.collectionName);

  // Edge-based drag state (must use $state for Svelte 5 reactivity)
  let dragState = $state(new Map()); // Track drag state for each item: { isDragging: boolean, closestEdge: 'top'|'bottom'|null }
  const backlogRowGap = 2; // px gap between rows to keep the list tight and align the drop indicator

  // --- Iteration section state ---
  let allIterations = $state([]);
  let addedGlobalIds = $state(new Set());
  let collapsedSections = $state(new Set());
  // Completed and cancelled iterations are terminal, so their sections stay
  // hidden by default; the toggle mirrors the board's completed-milestone
  // control and is persisted per view.
  let showCompletedIterations = $state(false);
  let sectionDropHighlight = $state(new Map()); // iterationId|'unassigned' -> boolean
  let pendingActionItemIds = $state(new Set());
  let pendingStoryPointsItemIds = $state(new Set());
  let visibleFields = $state([...DEFAULT_BACKLOG_FIELDS]);

  // --- Complete Iteration dialog state ---
  let completeIterationShow = $state(false);
  let iterationBeingCompleted = $state(null);
  let iterationIncompleteItems = $state([]);
  let iterationCompleteTargets = $state([]);

  // Legacy localStorage keys, read only as a fallback for settings saved
  // before backlog view state moved into user preferences.
  const globalIdsKey = $derived(`backlog-global-iterations-${workspaceId}`);
  const collapsedKey = $derived(`backlog-collapsed-sections-${workspaceId}`);

  function restorePersistedState() {
    const storedGlobal = collectionPreferences.viewStateValue(
      workspaceId,
      collectionId,
      'backlog',
      'global_iteration_ids'
    );
    if (Array.isArray(storedGlobal)) {
      addedGlobalIds = new Set(storedGlobal);
    } else {
      try {
        const savedGlobal = localStorage.getItem(globalIdsKey);
        if (savedGlobal) addedGlobalIds = new Set(JSON.parse(savedGlobal));
      } catch { /* ignore */ }
    }

    const storedCollapsed = collectionPreferences.viewStateValue(
      workspaceId,
      collectionId,
      'backlog',
      'collapsed_sections'
    );
    if (Array.isArray(storedCollapsed)) {
      collapsedSections = new Set(storedCollapsed);
    } else {
      try {
        const savedCollapsed = localStorage.getItem(collapsedKey);
        if (savedCollapsed) collapsedSections = new Set(JSON.parse(savedCollapsed));
      } catch { /* ignore */ }
    }

    const storedShowCompleted = collectionPreferences.viewStateValue(
      workspaceId,
      collectionId,
      'backlog',
      'show_completed_iterations'
    );
    if (typeof storedShowCompleted === 'boolean') {
      showCompletedIterations = storedShowCompleted;
    }

    const storedVisibleFields = collectionPreferences.viewStateValue(
      workspaceId,
      collectionId,
      'backlog',
      'visible_fields'
    );
    visibleFields = normalizeBacklogFields(storedVisibleFields);
  }

  function setShowCompletedIterations(value) {
    showCompletedIterations = value;
    collectionPreferences.patchViewState(workspaceId, collectionId, 'backlog', {
      show_completed_iterations: value,
    });
  }

  function setVisibleFields(fields) {
    visibleFields = normalizeBacklogFields(fields);
    collectionPreferences.patchViewState(workspaceId, collectionId, 'backlog', {
      visible_fields: visibleFields,
    });
  }

  function persistGlobalIds() {
    collectionPreferences.patchViewState(workspaceId, collectionId, 'backlog', {
      global_iteration_ids: [...addedGlobalIds],
    });
  }

  function persistCollapsed() {
    collectionPreferences.patchViewState(workspaceId, collectionId, 'backlog', {
      collapsed_sections: [...collapsedSections],
    });
  }

  // Derived iteration groupings
  function isClosedIteration(iteration) {
    return iteration?.status === 'completed' || iteration?.status === 'cancelled';
  }

  let closedIterationIds = $derived(
    new Set(allIterations.filter(isClosedIteration).map(i => i.id))
  );

  function isSectionClosed(sectionId) {
    if (sectionId == null || sectionId === 'unassigned') return false;
    const id = typeof sectionId === 'string' ? parseInt(sectionId, 10) : sectionId;
    return closedIterationIds.has(id);
  }

  let localIterations = $derived(
    allIterations.filter(i => !i.is_global && (showCompletedIterations || !isClosedIteration(i)))
  );
  let addedGlobalIterations = $derived(
    allIterations.filter(
      i => i.is_global && addedGlobalIds.has(i.id) && (showCompletedIterations || !isClosedIteration(i))
    )
  );
  let assignableIterations = $derived(
    allIterations.filter(i => i.status === 'planned' || i.status === 'active')
  );

  let visibleIterations = $derived.by(() =>
    sortBacklogIterations([...localIterations, ...addedGlobalIterations])
  );

  let visibleIterationIds = $derived(new Set(visibleIterations.map(i => i.id)));

  // Group items by iteration
  let iterationSections = $derived.by(() => {
    return visibleIterations.map(iteration => ({
      iteration,
      items: backlogItems.filter(i => i.iteration_id === iteration.id),
    }));
  });

  let unassignedItems = $derived(
    backlogItems.filter(i => !i.iteration_id || !visibleIterationIds.has(i.iteration_id))
  );
  const virtualBacklogRows = $derived(projectBacklogRows(
    [...iterationSections, { iteration: null, items: unassignedItems }], collapsedSections,
  ));

  // Shift selection extends across the rendered item rows, matching the order
  // the user sees rather than the flat store order.
  function selectRangeTo(itemId) {
    selection.selectionMode = true;
    const ids = virtualBacklogRows.filter((row) => row.kind === 'item').map((row) => row.item.id);
    const from = ids.indexOf(selection.selectionAnchorId);
    const to = ids.indexOf(itemId);
    if (from === -1 || to === -1) {
      selection.toggleSelected(itemId);
      return;
    }
    const start = Math.min(from, to);
    const end = Math.max(from, to);
    selection.selectedIds = [...new Set([...selection.selectedIds, ...ids.slice(start, end + 1)])];
  }

  useEventListener(() => window, 'keydown', (event) => {
    const target = event.target;
    const inEditable =
      target instanceof HTMLElement &&
      (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable);
    if (event.key === 'Escape' && selection.selectionMode && !inEditable) {
      event.preventDefault();
      selection.exitSelectionMode();
    }
  });

  const backlogDragRevision = $derived(backlogItems.map((item) => `${item.id}:${item.iteration_id}:${item.frac_index}`).join('|'));

  // Global iterations available to add (not already visible, not completed/cancelled, deduplicated)
  let availableGlobalIterations = $derived.by(() => {
    const seen = new Set();
    return allIterations.filter(i => {
      if (!i.is_global || addedGlobalIds.has(i.id) || isClosedIteration(i)) return false;
      if (seen.has(i.id)) return false;
      seen.add(i.id);
      return true;
    });
  });

  let addIterationPickerValue = $state(null);

  const iterationPickerConfig = {
    primary: { text: (item) => item.name },
    secondary: { text: (item) => item.status },
    searchFields: ['name'],
    getValue: (item) => item.id,
    getLabel: (item) => item.name,
  };

  function handleIterationPickerSelect(iter) {
    if (iter?.id) {
      addGlobalIteration(iter.id);
    }
    // Reset picker so it can be used again
    addIterationPickerValue = null;
  }

  // Field availability resolves through the shared effective-config cache,
  // keyed by (workspace, item type). Rows render before metadata; a pair that
  // is still unknown leaves its story-points control hidden.
  $effect(() => {
    const pairs = new Map();
    for (const item of backlogItems) {
      if (item.workspace_id == null) continue;
      const key = `${item.workspace_id}:${item.item_type_id ?? 'default'}`;
      if (!pairs.has(key)) pairs.set(key, [item.workspace_id, item.item_type_id ?? null]);
    }
    for (const [workspaceIdForItem, itemTypeId] of pairs.values()) {
      void workspaceDataStore.screenConfigFor(workspaceIdForItem, itemTypeId);
    }
  });

  function storyPointsConfiguredForItem(item) {
    if (item.workspace_id == null) return false;
    const key = `${item.workspace_id}:${item.item_type_id == null ? 'default' : item.item_type_id}`;
    const config = workspaceDataStore.screenConfigs[key];
    return !!config && isSystemFieldAvailableInEffectiveConfig(config, 'story_points');
  }

  function setStoryPointsPending(itemId, pending) {
    const next = new Set(pendingStoryPointsItemIds);
    if (pending) next.add(itemId);
    else next.delete(itemId);
    pendingStoryPointsItemIds = next;
  }

  async function updateStoryPoints(item, value) {
    if (pendingStoryPointsItemIds.has(item.id)) return;
    setStoryPointsPending(item.id, true);

    try {
      await items.update(item.id, { story_points: value });
      collectionStore.patchItem(item.id, { story_points: value });
    } catch (error) {
      console.error('Failed to update story points:', error);
      errorToast(t('collections.backlogActionFailed'));
    } finally {
      setStoryPointsPending(item.id, false);
    }
  }

  let shownItemCount = $derived(
    iterationSections.reduce((count, section) => count + (
      collapsedSections.has(section.iteration.id) ? 0 : section.items.length
    ), collapsedSections.has('unassigned') ? 0 : unassignedItems.length)
  );

  let backlogTotal = $derived(collectionStore.backlogPagination?.total_items ?? backlogItems.length);
  let backlogShownCount = $derived(collectionStore.loading ? null : shownItemCount);
  let backlogRemaining = $derived(Math.max(0, backlogTotal - backlogItems.length));
  let backlogCountSummary = $derived(formatItemCount(backlogTotal, backlogShownCount, t));

  // Centralized gradient styling
  const styles = useGradientStyles();

  // Listen for newly created items
  async function handleRefreshWorkItems(detail) {
    if (!detail?.itemId) return;
    try {
      const newItem = detail.item ?? await items.get(detail.itemId);
      // Collection QL and sub-filters depend on server state, so verify rather
      // than assume membership. Unknown never permits an insert; it reconciles.
      const membership = await resolveCreatedItemMembership(newItem, {
        workspaceId,
        collectionId,
        subFilterQL: collectionStore.effectiveBacklogSubFilterQL,
      });
      if (membership === 'member') {
        // Backlog membership also depends on configured statuses, so let the
        // authoritative backlog request decide whether the new item belongs.
        scheduleCollectionReconciliation();
      } else if (membership === 'unknown') {
        scheduleCollectionReconciliation();
      }
    } catch (error) {
      console.error('Failed to load new item:', error);
    }
  }

  $effect(() => onWorkItemRefresh(handleRefreshWorkItems));

  onMount(async () => {
    if (workspaceId) {
      await loadWorkspaceGradient(workspaceId);
      await workspaceDataStore.initialize(workspaceId);

      // Load iterations for this workspace
      try {
        const isPersonal = workspaceDataStore.workspace?.is_personal === true;
        if (workspaceDataStore.planningCompleteness?.iterations === true) {
          // Complete bootstrap data avoids a second full iteration fetch.
          allIterations = (workspaceDataStore.iterations || []).filter(
            (iteration) => !isPersonal || !iteration.is_global
          );
        } else {
          const iters = await iterations.getAll({
            workspace_id: workspaceId,
            include_global: !isPersonal,
          });
          allIterations = iters || [];
        }
      } catch (error) {
        console.error('Failed to load iterations:', error);
      }
    } else {
      await workspaceDataStore.initializeGlobal();
    }
    restorePersistedState();
    loading = false;
  });

  // Keep backlog count in sync
  $effect(() => {
    backlogStore.setCount(workspaceId, backlogTotal);
  });

  // Scoped SSE invalidations (WI-1624): any frame runs the existing delta fetch,
  // and a reconnect reconciles. No polling fallback.
  useCollectionEventStream(
    () => (collectionId ? { kind: 'collection', id: collectionId } : { kind: 'workspace', id: workspaceId }),
    { onInvalidate: () => scheduleCollectionReconciliation() }
  );

  function openItem(itemId, event) {
    // Don't open item if we're dragging
    if (document.body.classList.contains('is-dragging')) {
      return;
    }
    event.preventDefault();
    event.stopPropagation();
    itemModalStore.open(itemId, workspaceId);
  }

  // --- Section collapse / expand ---
  function toggleCollapse(sectionId) {
    const next = new Set(collapsedSections);
    if (next.has(sectionId)) {
      next.delete(sectionId);
    } else {
      next.add(sectionId);
    }
    collapsedSections = next;
    persistCollapsed();
  }

  // --- Start / Complete iteration ---
  async function startIteration(iteration) {
    try {
      await iterations.update(iteration.id, {
        status: 'active',
        is_global: iteration.is_global,
        workspace_id: iteration.workspace_id,
      });
      allIterations = allIterations.map(i =>
        i.id === iteration.id ? { ...i, status: 'active' } : i
      );
      successToast(t('iterations.iterationStarted', { name: iteration.name }));
    } catch (error) {
      console.error('Failed to start iteration:', error);
      errorToast(error?.message || t('iterations.failedToSaveIteration'));
    }
  }

  function completeIteration(iteration) {
    // Compute incomplete items for this iteration
    const iterationItems = backlogItems.filter(i => i.iteration_id === iteration.id);
    const incomplete = getIncompleteIterationItems(iterationItems, statuses, statusCategories);

    iterationBeingCompleted = { ...iteration, _totalItems: iterationItems.length };
    iterationIncompleteItems = incomplete;
    iterationCompleteTargets = allIterations.filter(
      i => i.id !== iteration.id && (i.status === 'planned' || i.status === 'active')
    );
    completeIterationShow = true;
  }

  async function handleCompleteIterationConfirm(moveTarget) {
    const iteration = iterationBeingCompleted;
    if (!iteration) return;

    try {
      const targetIterationId = moveTarget.type === 'iteration' ? moveTarget.iterationId : null;
      await iterations.complete(iteration.id, targetIterationId);
      allIterations = allIterations.map(i =>
        i.id === iteration.id ? { ...i, status: 'completed' } : i
      );
      successToast(t('iterations.iterationCompleted', { name: iteration.name }));
      reloadCollection();
    } catch (error) {
      console.error('Failed to complete iteration:', error);
      errorToast(error?.message || t('iterations.failedToSaveIteration'));
    }
  }

  // --- Global iteration add / remove ---
  function addGlobalIteration(iterationId) {
    const next = new Set(addedGlobalIds);
    next.add(iterationId);
    addedGlobalIds = next;
    persistGlobalIds();
  }

  function removeGlobalIteration(iteration) {
    const next = new Set(addedGlobalIds);
    next.delete(iteration.id);
    addedGlobalIds = next;
    persistGlobalIds();
  }

  function setItemActionPending(itemId, pending) {
    const next = new Set(pendingActionItemIds);
    if (pending) next.add(itemId);
    else next.delete(itemId);
    pendingActionItemIds = next;
  }

  async function moveItemToBoundary(item, boundary) {
    if (pendingActionItemIds.has(item.id)) return;
    setItemActionPending(item.id, true);

    try {
      const boundaryItem = await items.getBacklogBoundary(
        workspaceId,
        collectionId,
        collectionStore.effectiveBacklogSubFilterQL,
        boundary,
      );
      if (!boundaryItem || boundaryItem.id === item.id) return;

      await items.updateFracIndex(item.id, boundary === 'start'
        ? { previous_item_id: null, next_item_id: boundaryItem.id }
        : { previous_item_id: boundaryItem.id, next_item_id: null });

      collectionStore.moveBacklogItemToBoundary(item, boundary);
      successToast(t(boundary === 'start'
        ? 'collections.movedToBeginningOfBacklog'
        : 'collections.sentToEndOfBacklog', { title: item.title }));
      reloadCollection();
    } catch (error) {
      console.error(`Failed to move backlog item to ${boundary}:`, error);
      errorToast(t('collections.backlogActionFailed'));
    } finally {
      setItemActionPending(item.id, false);
    }
  }

  async function assignItemToIteration(item, iteration) {
    if (pendingActionItemIds.has(item.id)) return;
    setItemActionPending(item.id, true);

    try {
      if (iteration.status === 'active') {
        warningToast(t('iterations.activeScopeWarning'));
      }
      await items.update(item.id, { iteration_id: iteration.id });
      collectionStore.patchItem(item.id, {
        iteration_id: iteration.id,
        iteration_name: iteration.name,
      });
      successToast(t('collections.assignedToIteration', {
        title: item.title,
        iteration: iteration.name,
      }));
      reloadCollection();
    } catch (error) {
      console.error('Failed to assign backlog item to iteration:', error);
      errorToast(t('collections.backlogActionFailed'));
    } finally {
      setItemActionPending(item.id, false);
    }
  }

  // --- Drag and Drop ---

  // Get items belonging to a specific section
  function getSectionItems(sectionId) {
    if (sectionId === 'unassigned') return unassignedItems;
    const numId = typeof sectionId === 'string' ? parseInt(sectionId) : sectionId;
    return backlogItems.filter(i => i.iteration_id === numId);
  }

  function registerBacklogItem(element, itemId) {
    dragState.set(itemId, { isDragging: false, closestEdge: null });

    const cleanup = registerReorderable({
      element,
      getDragData: () => {
        const item = backlogItemsById.get(itemId);
        return { item, type: 'work-item', sectionId: item?.iteration_id || 'unassigned' };
      },
      getDropData: () => ({ sectionId: element.dataset.sectionId || 'unassigned' }),
      canDrop: ({ source }) => {
        const data = /** @type {any} */ (source.data);
        return virtualDropAllowed(data) && data.type === 'work-item' && data.item?.id !== itemId &&
          !isSectionClosed(element.dataset.sectionId || 'unassigned');
      },
      onDragStart: () => {
        document.body.classList.add('is-dragging');
        dragState.set(itemId, { ...(dragState.get(itemId) || {}), isDragging: true });
        dragState = new Map(dragState);
      },
      onDragEnd: () => {
        document.body.classList.remove('is-dragging');
        dragState = new Map([...dragState].map(([id]) => [id, { isDragging: false, closestEdge: null }]));
        sectionDropHighlight = new Map();
      },
      onEnter: (closestEdge, source) => {
        const data = /** @type {any} */ (source.data);
        if (data.type !== 'work-item' || data.item?.id === itemId) return;
        dragState.set(itemId, { ...(dragState.get(itemId) || {}), closestEdge });
        dragState = new Map(dragState);
      },
      onLeave: () => {
        dragState.set(itemId, { ...(dragState.get(itemId) || {}), closestEdge: null });
        dragState = new Map(dragState);
      },
      onDrop: ({ source, closestEdge }) => {
        const data = /** @type {any} */ (source.data);
        const item = backlogItemsById.get(itemId);
        if (item && data.type === 'work-item' && closestEdge && claimVirtualDrop(data)) {
          handleEdgeBasedDrop(data.item, item, closestEdge, {
            sectionId: element.dataset.sectionId || 'unassigned',
          });
        }
      },
      onCleanup: () => {
        dragState.delete(itemId);
      },
    });

    return { destroy: cleanup };
  }

  function registerBacklogSection(element, iterationId) {
    const key = String(iterationId);
    const cleanup = dropTargetForElements({
      element,
      canDrop: ({ source }) =>
        virtualDropAllowed(source.data) && (/** @type {any} */ (source.data)).type === 'work-item' && !isSectionClosed(key),
      getData: () => ({ type: 'section-drop', iterationId: key }),
      onDragEnter: ({ source }) => {
        if ((/** @type {any} */ (source.data)).type !== 'work-item') return;
        const next = new Map(sectionDropHighlight);
        next.set(key, true);
        sectionDropHighlight = next;
      },
      onDragLeave: () => {
        const next = new Map(sectionDropHighlight);
        next.delete(key);
        sectionDropHighlight = next;
      },
      onDrop: ({ source, location }) => {
        const data = /** @type {any} */ (source.data);
        const itemTarget = location.current.dropTargets.some((target) => target.element.hasAttribute('data-item-card'));
        if (!itemTarget && data.type === 'work-item' && !isSectionClosed(key) && claimVirtualDrop(data)) handleSectionDrop(data.item, key);
        sectionDropHighlight = new Map();
      },
    });
    return {
      destroy() {
        const next = new Map(sectionDropHighlight);
        next.delete(key);
        sectionDropHighlight = next;
        cleanup();
      }
    };
  }

  // Cross-section moves update the iteration first; the shared composable then
  // ranks the item within its target section.
  const { pendingDrops, handleEdgeBasedDrop } = useCollectionDragDrop({
    reload: () => reloadCollection(),
    getSiblingItems: ({ target }) => getSectionItems(target.sectionId),
    beforeReorder: async ({ draggedItem, target }) => {
      const targetIterationId =
        target.sectionId === 'unassigned'
          ? null
          : typeof target.sectionId === 'string'
            ? parseInt(target.sectionId)
            : target.sectionId;
      const sourceSectionId = draggedItem.iteration_id || null;

      const crossSection = sourceSectionId !== targetIterationId;
      if (crossSection) {
        // Warn if target iteration is active
        const targetIteration = allIterations.find(i => i.id === targetIterationId);
        if (targetIteration?.status === 'active') {
          warningToast(t('iterations.activeScopeWarning'));
        }
        await items.update(draggedItem.id, { iteration_id: targetIterationId });
        collectionStore.patchItem(draggedItem.id, { iteration_id: targetIterationId });
      }

      return {
        item: { ...draggedItem, iteration_id: targetIterationId },
        checkSamePosition: !crossSection,
      };
    },
  });

  async function handleSectionDrop(draggedItem, targetIterationId) {
    if (isSectionClosed(targetIterationId)) return;
    const dropId = `${draggedItem.id}-section-${targetIterationId}`;
    if (pendingDrops.has(dropId)) return;
    pendingDrops.add(dropId);

    try {
      const newIterationId = targetIterationId === 'unassigned' ? null : (typeof targetIterationId === 'string' ? parseInt(targetIterationId) : targetIterationId);
      const currentIterationId = draggedItem.iteration_id || null;

      if (currentIterationId === newIterationId) return;

      // Warn if target is active
      const targetIteration = allIterations.find(i => i.id === newIterationId);
      if (targetIteration?.status === 'active') {
        warningToast(t('iterations.activeScopeWarning'));
      }

      await items.update(draggedItem.id, { iteration_id: newIterationId });
      collectionStore.patchItem(draggedItem.id, { iteration_id: newIterationId });
      reloadCollection();
    } catch (error) {
      console.error('Failed to handle section drop:', error);
      errorToast(error?.message || t('common.error'));
    } finally {
      setTimeout(() => pendingDrops.delete(dropId), 500);
    }
  }

</script>

{#if loading}
  <div class="p-6">
    <div class="animate-pulse">{t('common.loading')}</div>
  </div>
{:else if workspace || !workspaceId}
  <StaticViewBackground
    backgroundStyle={styles.backgroundStyle}
    contextVars={styles.contextVars}
    testid="backlog-view"
  >
    <!-- Content Container -->
      <!-- Header with view tabs -->
      <div class="mb-8">
        <ViewHeader
          workspaceName={workspace?.name || ''}
          collection={currentCollectionName}
          viewName="Backlog"
          itemCount={backlogTotal}
          shownCount={backlogShownCount}
        >
          {#snippet actions()}
            <div class="flex items-center gap-2">
              {#if availableGlobalIterations.length > 0}
                <ItemPicker
                  bind:value={addIterationPickerValue}
                  items={availableGlobalIterations}
                  config={iterationPickerConfig}
                  placeholder={t('iterations.addGlobalIteration')}
                  allowClear={false}
                  showSelectedInTrigger={false}
                  onSelect={handleIterationPickerSelect}
                >
                  {#snippet children()}
                    <span
                      class="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium rounded-lg border transition-colors"
                      style="{styles.glassStyle?.(12) ?? ''} {styles.glassTextStyle ?? ''}"
                    >
                      <Plus class="w-4 h-4" />
                      {t('iterations.addGlobalIteration')}
                    </span>
                  {/snippet}
                </ItemPicker>
              {/if}
              <CollectionViewSwitcher
                {workspaceId}
                {collectionId}
                activeView="backlog"
                publicSlug={collectionStore.publicSlug}
              />
            </div>
          {/snippet}
        </ViewHeader>
      </div>

      <!-- Controls Bar -->
      <div class="flex flex-wrap items-center gap-3 mb-6">
        <SubFilterBar {workspaceId} {collectionId} showCompletionToggle={false} />
        <BacklogFieldSelector fields={visibleFields} onchange={setVisibleFields} />
        <Toggle
          checked={showCompletedIterations}
          onchange={setShowCompletedIterations}
          label={t('iterations.showCompletedIterations')}
          labelPosition="left"
          size="small"
          dataTestid="backlog-show-completed-iterations"
        />
        <!-- Selection is an explicit mode, matching the board and list. -->
        <ButtonGroup testid="backlog-controls-group">
          {#snippet children(segment)}
            <button
              type="button"
              class={segment}
              data-active={selection.selectionMode}
              aria-pressed={selection.selectionMode}
              title={t('collections.selectItems')}
              onclick={selection.toggleSelectionMode}
              data-testid="backlog-select-mode-toggle"
            >
              <CheckSquare class="w-4 h-4" />
              {t('common.select')}
            </button>
          {/snippet}
        </ButtonGroup>
      </div>

      {#if backlogItems.length === 0 && visibleIterations.length === 0}
        <EmptyState
          icon={List}
          title={t('collections.noItemsInBacklog')}
          description={t('collections.noItemsInBacklogDesc')}
        />
      {:else}
        <!-- Backlog items grouped by iteration sections -->
        <div class="w-full" bind:this={backlogListElement} style="overflow-anchor: none;">
          <VirtualRows items={virtualBacklogRows} getKey={(row) => row.key}
            listElement={backlogListElement}
            estimateSize={(row) => row.kind === 'item' ? 56 : 64}
            identity={`backlog:${workspaceId ?? ''}:${collectionId ?? ''}:${collectionStore.effectiveBacklogSubFilterQL}`}
            layoutRevision={visibleFields.join('|')}
            dragSources dragRevision={backlogDragRevision}
          >
            {#snippet row(entry, virtual)}
              {#if entry.kind === 'header'}
                <BacklogIterationSection
                  headerOnly virtualRow={virtual}
                  iteration={entry.section.iteration} items={entry.section.items}
                  collapsed={collapsedSections.has(entry.sectionId)}
                  {workspace} {itemTypes} {statuses} {statusCategories} {styles} {dragState}
                  isGlobalAdded={addedGlobalIds.has(entry.sectionId)}
                  sectionHighlight={sectionDropHighlight.get(String(entry.sectionId)) || false}
                  onToggleCollapse={toggleCollapse} onStartIteration={startIteration}
                  onCompleteIteration={completeIteration} onRemoveGlobal={removeGlobalIteration}
                  sectionDndAction={registerBacklogSection}
                  selectionMode={selection.selectionMode} selectedIds={selection.selectedIdSet}
                  onSelect={selection.toggleSelected} onShiftSelect={selectRangeTo}
                />
              {:else if entry.kind === 'empty'}
                <div use:virtual.attach use:registerBacklogSection={entry.sectionId}
                  class="flex items-center justify-center py-6 px-4 rounded-lg border-2 border-dashed text-[color:var(--ctx-text-subtle,var(--ds-text-subtle))] border-[color:var(--ctx-border,var(--ds-border))]"
                  data-section-drop-zone data-iteration-id={entry.sectionId}
                  data-testid={`backlog-empty-section-${entry.sectionId}`}
                >{t('collections.dragItemsHere')}</div>
              {:else}
                <BacklogItemRow virtualRow={virtual} deferred={false}
                  item={entry.item} sectionId={entry.sectionId}
                  {workspace} {itemTypes} {statuses} {statusCategories} {styles} {dragState}
                  {backlogRowGap} {assignableIterations} {pendingActionItemIds} {visibleFields}
                  onOpenItem={openItem} onMoveItemToBoundary={moveItemToBoundary}
                  onAssignItemToIteration={assignItemToIteration}
                  {storyPointsConfiguredForItem} storyPointsPendingItemIds={pendingStoryPointsItemIds}
                  onUpdateStoryPoints={updateStoryPoints} itemDndAction={registerBacklogItem}
                  selectionMode={selection.selectionMode}
                  selected={selection.selectedIdSet.has(entry.item.id)}
                  onSelect={selection.toggleSelected}
                  onShiftSelect={selectRangeTo}
                />
              {/if}
            {/snippet}
          </VirtualRows>

          <!-- Load More -->
          {#if collectionStore.backlogHasMore}
            <div class="mt-6 text-center">
              <button
                data-testid="backlog-load-more"
                onclick={() => collectionStore.loadMoreBacklog()}
                disabled={collectionStore.backlogLoadingMore}
                class="px-4 py-2 text-sm font-medium rounded-lg border transition-colors"
                style="{styles.glassStyle?.(12) ?? ''} {styles.glassTextStyle ?? ''}"
              >
                {collectionStore.backlogLoadingMore ? t('common.loading') : t('common.loadMore')}
                {#if collectionStore.backlogPagination?.total_items}
                  ({backlogRemaining} {t('common.remaining')})
                {/if}
              </button>
            </div>
          {/if}

          <!-- Summary -->
          <div class="mt-8 text-center">
            <p data-testid="backlog-count-summary" class="text-sm" style="color: var(--ctx-text-subtle, var(--ds-text-subtle));">
              {backlogCountSummary}
            </p>
          </div>
        </div>
      {/if}
  </StaticViewBackground>

  <BulkActionToolbar
    {selection}
    moveTargets={bulkMoveTargets}
    {priorities}
    testidPrefix="backlog"
  />
{:else}
  <div class="p-6">
    <div class="text-center" style="color: var(--ds-text-subtle);">
      {t('collections.workspaceNotFound')}
    </div>
  </div>
{/if}

<!-- Complete Iteration Dialog -->
<CompleteIterationDialog
  bind:show={completeIterationShow}
  iteration={iterationBeingCompleted}
  incompleteItems={iterationIncompleteItems}
  targetIterations={iterationCompleteTargets}
  onconfirm={handleCompleteIterationConfirm}
/>
