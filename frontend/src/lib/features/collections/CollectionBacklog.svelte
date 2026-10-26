<script>
  import { onMount } from 'svelte';
  import { useEventListener } from 'runed';
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
  import { draggable, dropTargetForElements } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import { attachClosestEdge, extractClosestEdge } from '@atlaskit/pragmatic-drag-and-drop-hitbox/closest-edge';
  import ViewHeader from '../../layout/ViewHeader.svelte';
  import StaticViewBackground from '../../layout/StaticViewBackground.svelte';
  import SubFilterBar from './SubFilterBar.svelte';
  import CollectionViewSwitcher from './CollectionViewSwitcher.svelte';
  import BacklogIterationSection from './BacklogIterationSection.svelte';
  import { sortBacklogIterations } from './backlogIterationOrder.js';
  import ItemPicker from '../../pickers/ItemPicker.svelte';
  import { backlogStore } from '../../stores/backlogStore.svelte.js';
  import { workspaceDataStore } from '../../stores/workspaceDataStore.svelte.js';
  import { collectionPreferences } from '../../stores/collectionPreferences.svelte.js';
  import { itemModalStore } from '../../stores/itemModalStore.svelte.js';
  import { useCollectionEventStream } from '../../composables/useCollectionEventStream.svelte.js';
  import { errorToast, successToast, warningToast } from '../../stores/toasts.svelte.js';
  import { getIncompleteIterationItems } from './iterationCompletion.js';
  import CompleteIterationDialog from '../../dialogs/CompleteIterationDialog.svelte';
  import { formatItemCount } from '../../utils/itemCount.js';
  import { isSystemFieldAvailableInEffectiveConfig } from '../../utils/screenFields.js';

  let { workspaceId, collectionId = null } = $props();

  // Reference data from shared workspace store
  let workspace = $derived(workspaceDataStore.workspace);
  let itemTypes = $derived(workspaceDataStore.itemTypes);
  let statuses = $derived(workspaceDataStore.statuses);
  let statusCategories = $derived(workspaceDataStore.statusCategories);

  let backlogItems = $derived(collectionStore.backlogItems);
  let backlogItemsById = $derived(new Map(backlogItems.map((item) => [item.id, item])));
  let loading = $state(true);
  let currentCollectionName = $derived(collectionStore.collectionName);
  let pendingDrops = new Set(); // Track pending drop operations to prevent duplicates

  // Edge-based drag state (must use $state for Svelte 5 reactivity)
  let dragState = $state(new Map()); // Track drag state for each item: { isDragging: boolean, closestEdge: 'top'|'bottom'|null }
  const backlogRowGap = 2; // px gap between rows to keep the list tight and align the drop indicator

  // --- Iteration section state ---
  let allIterations = $state([]);
  let addedGlobalIds = $state(new Set());
  let collapsedSections = $state(new Set());
  let sectionDropHighlight = $state(new Map()); // iterationId|'unassigned' -> boolean
  let pendingActionItemIds = $state(new Set());
  let pendingStoryPointsItemIds = $state(new Set());

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
  let localIterations = $derived(allIterations.filter(i => !i.is_global));
  let addedGlobalIterations = $derived(allIterations.filter(i => i.is_global && addedGlobalIds.has(i.id)));
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

  // Global iterations available to add (not already visible, not completed/cancelled, deduplicated)
  let availableGlobalIterations = $derived.by(() => {
    const seen = new Set();
    return allIterations.filter(i => {
      if (!i.is_global || addedGlobalIds.has(i.id) || i.status === 'completed' || i.status === 'cancelled') return false;
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
  async function handleRefreshWorkItems(event) {
    if (!event.detail?.itemId) return;
    try {
      const newItem = event.detail.item ?? await items.get(event.detail.itemId);
      // Collection QL and sub-filters depend on server state, so verify rather
      // than assume membership. Unknown never permits an insert; it reconciles.
      const membership = await resolveCreatedItemMembership(newItem, {
        workspaceId,
        collectionId,
        subFilterQL: collectionStore.subFilterQL ? collectionStore.effectiveSubFilterQL : '',
      });
      if (membership === 'member') {
        // insertCreatedItem deduplicates by id so a repeated create event cannot
        // append the same backlog row twice.
        collectionStore.insertCreatedItem(newItem, { backlog: true });
      } else if (membership === 'unknown') {
        scheduleCollectionReconciliation();
      }
    } catch (error) {
      console.error('Failed to load new item:', error);
    }
  }

  useEventListener(() => window, 'refresh-work-items', handleRefreshWorkItems);

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

      restorePersistedState();
    } else {
      await workspaceDataStore.initializeGlobal();
    }
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
        collectionStore.effectiveSubFilterQL,
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

    const draggableCleanup = draggable({
      element,
      getInitialData: () => {
        const item = backlogItemsById.get(itemId);
        return { item, type: 'work-item', sectionId: item?.iteration_id || 'unassigned' };
      },
      onDragStart: () => {
        element.style.opacity = '0.5';
        document.body.classList.add('is-dragging');
        const state = dragState.get(itemId) || {};
        const next = new Map(dragState);
        next.set(itemId, { ...state, isDragging: true });
        dragState = next;
      },
      onDrop: () => {
        element.style.opacity = '';
        document.body.classList.remove('is-dragging');
        dragState = new Map([...dragState].map(([id]) => [id, { isDragging: false, closestEdge: null }]));
        sectionDropHighlight = new Map();
      }
    });

    const dropTargetCleanup = dropTargetForElements({
      element,
      canDrop: ({ source }) => {
        const data = /** @type {any} */ (source.data);
        return data.type === 'work-item' && data.item?.id !== itemId;
      },
      getData: ({ input, element: targetElement }) => attachClosestEdge(
        { sectionId: element.dataset.sectionId || 'unassigned' },
        { input, element: targetElement, allowedEdges: ['top', 'bottom'] }
      ),
      onDragEnter: ({ self, source }) => {
        const data = /** @type {any} */ (source.data);
        if (data.type !== 'work-item' || data.item?.id === itemId) return;
        const next = new Map(dragState);
        next.set(itemId, {
          ...(dragState.get(itemId) || {}),
          closestEdge: extractClosestEdge(self.data),
        });
        dragState = next;
      },
      onDragLeave: () => {
        const next = new Map(dragState);
        next.set(itemId, { ...(dragState.get(itemId) || {}), closestEdge: null });
        dragState = next;
      },
      onDrop: ({ self, source }) => {
        const data = /** @type {any} */ (source.data);
        const item = backlogItemsById.get(itemId);
        const closestEdge = extractClosestEdge(self.data);
        if (item && data.type === 'work-item' && closestEdge) {
          handleEdgeBasedDrop(
            data.item,
            item,
            closestEdge,
            element.dataset.sectionId || 'unassigned'
          );
        }
      }
    });

    return {
      destroy() {
        element.style.opacity = '';
        document.body.classList.remove('is-dragging');
        draggableCleanup();
        dropTargetCleanup();
        dragState.delete(itemId);
      }
    };
  }

  function registerBacklogSection(element, iterationId) {
    const key = String(iterationId);
    const cleanup = dropTargetForElements({
      element,
      canDrop: ({ source }) => (/** @type {any} */ (source.data)).type === 'work-item',
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
      onDrop: ({ source }) => {
        const data = /** @type {any} */ (source.data);
        if (data.type === 'work-item') handleSectionDrop(data.item, key);
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

  async function handleEdgeBasedDrop(draggedItem, targetItem, closestEdge, targetSectionId) {
    // Create a unique identifier for this drop operation
    const dropId = `${draggedItem.id}-edge-${targetItem.id}-${closestEdge}`;

    try {
      // Prevent duplicate drops
      if (pendingDrops.has(dropId)) {
        return;
      }

      pendingDrops.add(dropId);

      // Determine target iteration_id from the section the target item lives in
      const targetIterationId = targetSectionId === 'unassigned' ? null : (typeof targetSectionId === 'string' ? parseInt(targetSectionId) : targetSectionId);
      const sourceSectionId = draggedItem.iteration_id || null;

      // Cross-section move: update iteration_id
      const crossSection = (sourceSectionId !== targetIterationId);
      if (crossSection) {
        // Warn if target iteration is active
        const targetIteration = allIterations.find(i => i.id === targetIterationId);
        if (targetIteration?.status === 'active') {
          warningToast(t('iterations.activeScopeWarning'));
        }
        await items.update(draggedItem.id, { iteration_id: targetIterationId });
        collectionStore.patchItem(draggedItem.id, { iteration_id: targetIterationId });
      }

      // Compute prev/next within the target section
      const sectionItems = getSectionItems(targetSectionId).filter(i => i.id !== draggedItem.id);
      const targetIndex = sectionItems.findIndex(i => i.id === targetItem.id);

      // Check if we're trying to drop in the same position (only matters for within-section)
      if (!crossSection) {
        const fullSectionItems = getSectionItems(targetSectionId);
        const draggedIndex = fullSectionItems.findIndex(i => i.id === draggedItem.id);
        const origTargetIndex = fullSectionItems.findIndex(i => i.id === targetItem.id);
        const isDroppingSamePosition = (
          (closestEdge === 'top' && draggedIndex === origTargetIndex - 1) ||
          (closestEdge === 'bottom' && draggedIndex === origTargetIndex + 1)
        );
        if (isDroppingSamePosition) return;
      }

      let prevItemId = null;
      let nextItemId = null;

      if (closestEdge === 'top') {
        if (targetIndex > 0) {
          const prevItem = sectionItems[targetIndex - 1];
          if (prevItem) prevItemId = prevItem.id;
        }
        nextItemId = targetItem.id;
      } else if (closestEdge === 'bottom') {
        prevItemId = targetItem.id;
        if (targetIndex < sectionItems.length - 1) {
          const nextItem = sectionItems[targetIndex + 1];
          if (nextItem) nextItemId = nextItem.id;
        }
      }

      // Update the frac_index using item IDs
      const indexData = {
        previous_item_id: prevItemId,
        next_item_id: nextItemId
      };
      await items.updateFracIndex(draggedItem.id, indexData);

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

  async function handleSectionDrop(draggedItem, targetIterationId) {
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
      <div class="flex items-center mb-6">
        <SubFilterBar {workspaceId} {collectionId} showCompletionToggle={false} />
      </div>

      {#if backlogItems.length === 0 && visibleIterations.length === 0}
        <EmptyState
          icon={List}
          title={t('collections.noItemsInBacklog')}
          description={t('collections.noItemsInBacklogDesc')}
        />
      {:else}
        <!-- Backlog items grouped by iteration sections -->
        <div class="w-full">

          {#each iterationSections as section (section.iteration.id)}
            <BacklogIterationSection
              iteration={section.iteration}
              items={section.items}
              collapsed={collapsedSections.has(section.iteration.id)}
              {workspace}
              {itemTypes}
              {statuses}
              {statusCategories}
              {styles}
              {dragState}
              {backlogRowGap}
              {assignableIterations}
              {pendingActionItemIds}
              isGlobalAdded={addedGlobalIds.has(section.iteration.id)}
              sectionHighlight={sectionDropHighlight.get(String(section.iteration.id)) || false}
              onToggleCollapse={toggleCollapse}
              onOpenItem={openItem}
              onMoveItemToBoundary={moveItemToBoundary}
              onAssignItemToIteration={assignItemToIteration}
              onStartIteration={startIteration}
              onCompleteIteration={completeIteration}
              onRemoveGlobal={removeGlobalIteration}
              storyPointsConfiguredForItem={storyPointsConfiguredForItem}
              storyPointsPendingItemIds={pendingStoryPointsItemIds}
              onUpdateStoryPoints={updateStoryPoints}
              itemDndAction={registerBacklogItem}
              sectionDndAction={registerBacklogSection}
            />
          {/each}

          <!-- Unassigned / Backlog section -->
          <BacklogIterationSection
            iteration={null}
            items={unassignedItems}
            collapsed={collapsedSections.has('unassigned')}
            {workspace}
            {itemTypes}
            {statuses}
            {statusCategories}
            {styles}
            {dragState}
            {backlogRowGap}
            {assignableIterations}
            {pendingActionItemIds}
            sectionHighlight={sectionDropHighlight.get('unassigned') || false}
            onToggleCollapse={toggleCollapse}
            onOpenItem={openItem}
            onMoveItemToBoundary={moveItemToBoundary}
            onAssignItemToIteration={assignItemToIteration}
            storyPointsConfiguredForItem={storyPointsConfiguredForItem}
            storyPointsPendingItemIds={pendingStoryPointsItemIds}
            onUpdateStoryPoints={updateStoryPoints}
            itemDndAction={registerBacklogItem}
            sectionDndAction={registerBacklogSection}
          />

          <!-- Load More -->
          {#if collectionStore.backlogHasMore}
            <div class="mt-6 text-center">
              <button
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
