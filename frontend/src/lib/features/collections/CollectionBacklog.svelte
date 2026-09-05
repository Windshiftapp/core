<script>
  import { onMount } from 'svelte';
  import { useEventListener } from 'runed';
  import { api } from '../../api.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { collectionStore, refreshCollectionDeltas, reloadCollection } from '../../stores/collectionContext.js';
  import { useGradientStyles, loadWorkspaceGradient } from '../../stores/workspaceGradient.svelte.js';
  import { List, Plus } from '@lucide/svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import { draggable, dropTargetForElements } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import { attachClosestEdge, extractClosestEdge } from '@atlaskit/pragmatic-drag-and-drop-hitbox/closest-edge';
  import ItemDetail from '../items/ItemDetail.svelte';
  import ViewHeader from '../../layout/ViewHeader.svelte';
  import StaticViewBackground from '../../layout/StaticViewBackground.svelte';
  import SubFilterBar from './SubFilterBar.svelte';
  import CollectionViewSwitcher from './CollectionViewSwitcher.svelte';
  import BacklogIterationSection from './BacklogIterationSection.svelte';
  import ItemPicker from '../../pickers/ItemPicker.svelte';
  import { backlogStore, workspaceDataStore } from '../../stores/index.js';
  import { useWorkItemPoller } from '../../composables/useWorkItemPoller.svelte.js';
  import { errorToast, successToast, warningToast } from '../../stores/toasts.svelte.js';
  import { getIncompleteIterationItems } from './iterationCompletion.js';
  import CompleteIterationDialog from '../../dialogs/CompleteIterationDialog.svelte';
  import { workspacesStore } from '../../stores/workspaces.svelte.js';
  import { isSystemFieldAvailableForItem } from '../../utils/screenFields.js';

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
  let showItemModal = $state(false);
  let selectedItemId = $state(null);
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
  let storyPointsScreenConfiguration = $state({
    ready: false,
    configSetsByWorkspaceId: new Map(),
    screensById: new Map(),
  });

  // --- Complete Iteration dialog state ---
  let completeIterationShow = $state(false);
  let iterationBeingCompleted = $state(null);
  let iterationIncompleteItems = $state([]);
  let iterationCompleteTargets = $state([]);

  // localStorage keys
  const globalIdsKey = $derived(`backlog-global-iterations-${workspaceId}`);
  const collapsedKey = $derived(`backlog-collapsed-sections-${workspaceId}`);

  // Restore persisted state from localStorage
  function restorePersistedState() {
    try {
      const savedGlobal = localStorage.getItem(globalIdsKey);
      if (savedGlobal) addedGlobalIds = new Set(JSON.parse(savedGlobal));
    } catch { /* ignore */ }
    try {
      const savedCollapsed = localStorage.getItem(collapsedKey);
      if (savedCollapsed) collapsedSections = new Set(JSON.parse(savedCollapsed));
    } catch { /* ignore */ }
  }

  function persistGlobalIds() {
    localStorage.setItem(globalIdsKey, JSON.stringify([...addedGlobalIds]));
  }

  function persistCollapsed() {
    localStorage.setItem(collapsedKey, JSON.stringify([...collapsedSections]));
  }

  // Derived iteration groupings
  let localIterations = $derived(allIterations.filter(i => !i.is_global));
  let addedGlobalIterations = $derived(allIterations.filter(i => i.is_global && addedGlobalIds.has(i.id)));
  let assignableIterations = $derived(
    allIterations.filter(i => i.status === 'planned' || i.status === 'active')
  );

  // Sort order: active first, then planned, then completed/cancelled
  const statusOrder = { active: 0, planned: 1, completed: 2, cancelled: 3 };

  let visibleIterations = $derived.by(() => {
    const combined = [...localIterations, ...addedGlobalIterations];
    return combined.sort((a, b) => (statusOrder[a.status] ?? 9) - (statusOrder[b.status] ?? 9));
  });

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

  async function loadStoryPointsScreenConfiguration() {
    const workspaceRecords = workspaceId
      ? [{
          id: workspaceDataStore.workspace?.id ?? Number(workspaceId),
          configuration_set_id: workspaceDataStore.workspace?.configuration_set_id ?? null,
        }]
      : (await workspacesStore.load()).map((availableWorkspace) => ({
          id: availableWorkspace.id,
          configuration_set_id: availableWorkspace.configuration_set_id ?? null,
        }));

    const configSetIds = [...new Set(
      workspaceRecords
        .map((record) => record.configuration_set_id)
        .filter((configSetId) => configSetId != null)
        .map((configSetId) => String(configSetId))
    )];

    const [screensOutcome, configSetOutcomes] = await Promise.all([
      api.screens.getAllWithFields()
        .then((screens) => ({ status: 'fulfilled', value: screens }))
        .catch((error) => ({ status: 'rejected', reason: error })),
      Promise.allSettled(configSetIds.map((configSetId) => api.configurationSets.get(configSetId))),
    ]);

    if (screensOutcome.status === 'rejected') {
      console.error('Failed to load screens for backlog fields:', screensOutcome.reason);
    }

    const screensById = new Map(
      (Array.isArray(screensOutcome.value) ? screensOutcome.value : [])
        .filter((screen) => screen?.id != null)
        .map((screen) => [screen.id, screen])
    );
    const configSetsById = new Map();
    configSetOutcomes.forEach((outcome, index) => {
      if (outcome.status === 'fulfilled' && outcome.value) {
        configSetsById.set(configSetIds[index], outcome.value);
      } else if (outcome.status === 'rejected') {
        console.error(`Failed to load configuration set ${configSetIds[index]} for backlog fields:`, outcome.reason);
      }
    });

    const configSetsByWorkspaceId = new Map();
    workspaceRecords.forEach((record) => {
      if (record?.id == null) return;
      const workspaceKey = String(record.id);
      const configSetId = record.configuration_set_id;
      configSetsByWorkspaceId.set(
        workspaceKey,
        configSetId == null ? null : configSetsById.get(String(configSetId))
      );
    });

    storyPointsScreenConfiguration = {
      ready: true,
      configSetsByWorkspaceId,
      screensById,
    };
  }

  function storyPointsConfiguredForItem(item) {
    if (!storyPointsScreenConfiguration.ready) return false;
    return isSystemFieldAvailableForItem(
      item,
      'story_points',
      storyPointsScreenConfiguration.configSetsByWorkspaceId,
      storyPointsScreenConfiguration.screensById
    );
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
      await api.items.update(item.id, { story_points: value });
      collectionStore.backlogItems = collectionStore.backlogItems.map((backlogItem) =>
        backlogItem.id === item.id ? { ...backlogItem, story_points: value } : backlogItem
      );
    } catch (error) {
      console.error('Failed to update story points:', error);
      errorToast(t('collections.backlogActionFailed'));
    } finally {
      setStoryPointsPending(item.id, false);
    }
  }

  // Total item count across all sections
  let totalItemCount = $derived(collectionStore.backlogPagination?.total_items ?? backlogItems.length);

  // Centralized gradient styling
  const styles = useGradientStyles();

  // Listen for newly created items
  async function handleRefreshWorkItems(event) {
    if (event.detail?.itemId) {
      try {
        const newItem = event.detail.item ?? await api.items.get(event.detail.itemId);
        // When viewing a collection, accept items from any workspace (the collection defines scope).
        // Otherwise fall back to current-workspace check.
        const belongsToView = collectionId
          ? true
          : Number(newItem.workspace_id) === Number(workspaceId);
        if (belongsToView) {
          collectionStore.backlogItems = [...collectionStore.backlogItems, newItem];
        }
      } catch (error) {
        console.error('Failed to load new item:', error);
      }
    }
  }

  useEventListener(() => window, 'refresh-work-items', handleRefreshWorkItems);

  onMount(async () => {
    if (workspaceId) {
      await loadWorkspaceGradient(workspaceId);
      await workspaceDataStore.initialize(workspaceId);
      await loadStoryPointsScreenConfiguration();

      // Load iterations for this workspace
      try {
        const iters = await api.iterations.getAll({
          workspace_id: workspaceId,
          include_global: !workspace?.is_personal,
        });
        allIterations = iters || [];
      } catch (error) {
        console.error('Failed to load iterations:', error);
      }

      restorePersistedState();
    } else {
      await workspaceDataStore.initializeGlobal();
      await loadStoryPointsScreenConfiguration();
    }
    loading = false;
  });

  // Keep backlog count in sync
  $effect(() => {
    backlogStore.setCount(workspaceId, collectionStore.backlogPagination?.total_items ?? collectionStore.backlogItems.length);
  });

  // Adaptive polling for backlog items: use cheap deltas, falling back to full refresh only when needed.
  const poller = useWorkItemPoller(() => refreshCollectionDeltas());

  function openItem(itemId, event) {
    // Don't open item if we're dragging
    if (document.body.classList.contains('is-dragging')) {
      return;
    }
    event.preventDefault();
    event.stopPropagation();
    selectedItemId = itemId;
    showItemModal = true;
  }

  async function closeItemModal(event) {
    showItemModal = false;
    selectedItemId = null;

    // If changes were made in the modal, reload data
    if (event?.hasChanges) {
      reloadCollection();
    }
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
      await api.iterations.update(iteration.id, {
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
      await api.iterations.complete(iteration.id, targetIterationId);
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
      const boundaryItem = await api.items.getBacklogBoundary(
        workspaceId,
        collectionId,
        collectionStore.subFilterQL,
        boundary,
      );
      if (!boundaryItem || boundaryItem.id === item.id) return;

      await api.items.updateFracIndex(item.id, boundary === 'start'
        ? { previous_item_id: null, next_item_id: boundaryItem.id }
        : { previous_item_id: boundaryItem.id, next_item_id: null });

      const otherItems = collectionStore.backlogItems.filter(i => i.id !== item.id);
      collectionStore.backlogItems = boundary === 'start'
        ? [item, ...otherItems]
        : [...otherItems, item];
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
      await api.items.update(item.id, { iteration_id: iteration.id });
      collectionStore.backlogItems = collectionStore.backlogItems.map(i =>
        i.id === item.id
          ? { ...i, iteration_id: iteration.id, iteration_name: iteration.name }
          : i
      );
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
        await api.items.update(draggedItem.id, { iteration_id: targetIterationId });
        // Update store directly
        collectionStore.backlogItems = collectionStore.backlogItems.map(i => i.id === draggedItem.id ? { ...i, iteration_id: targetIterationId } : i);
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
      await api.items.updateFracIndex(draggedItem.id, indexData);

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

      await api.items.update(draggedItem.id, { iteration_id: newIterationId });
      collectionStore.backlogItems = collectionStore.backlogItems.map(i => i.id === draggedItem.id ? { ...i, iteration_id: newIterationId } : i);
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
  >
    <!-- Content Container -->
      <!-- Header with view tabs -->
      <div class="mb-8">
        <ViewHeader
          workspaceName={workspace?.name || ''}
          collection={currentCollectionName}
          viewName="Backlog"
          itemCount={totalItemCount}
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
        <SubFilterBar {workspaceId} />
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
                  ({collectionStore.backlogPagination.total - collectionStore.backlogItems.length} {t('common.remaining')})
                {/if}
              </button>
            </div>
          {/if}

          <!-- Summary -->
          <div class="mt-8 text-center">
            <p class="text-sm" style="color: var(--ctx-text-subtle, var(--ds-text-subtle));">
              {t('collections.showingItemsFromBacklog', { count: collectionStore.backlogPagination?.total_items ?? backlogItems.length })}
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

<!-- Item Detail Modal -->
{#if showItemModal && selectedItemId}
  <ItemDetail
    workspaceId={workspaceId}
    itemId={selectedItemId}
    isModal={true}
    onclose={closeItemModal}
  />
{/if}

<!-- Complete Iteration Dialog -->
<CompleteIterationDialog
  bind:show={completeIterationShow}
  iteration={iterationBeingCompleted}
  incompleteItems={iterationIncompleteItems}
  targetIterations={iterationCompleteTargets}
  onconfirm={handleCompleteIterationConfirm}
/>
