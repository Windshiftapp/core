<script>
  import { onMount } from 'svelte';
  import { useEventListener } from 'runed';
  import { api } from '../../api.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { collectionStore, reloadCollection } from '../../stores/collectionContext.js';
  import { useGradientStyles, loadWorkspaceGradient } from '../../stores/workspaceGradient.svelte.js';
  import { List, Plus } from 'lucide-svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import { draggable, dropTargetForElements } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import { attachClosestEdge, extractClosestEdge } from '@atlaskit/pragmatic-drag-and-drop-hitbox/closest-edge';
  import ItemDetail from '../items/ItemDetail.svelte';
  import ViewHeader from '../../layout/ViewHeader.svelte';
  import CollectionViewSwitcher from './CollectionViewSwitcher.svelte';
  import BacklogSprintSection from './BacklogSprintSection.svelte';
  import ItemPicker from '../../pickers/ItemPicker.svelte';
  import { backlogStore, workspaceDataStore } from '../../stores/index.js';
  import { useWorkItemPoller } from '../../composables/useWorkItemPoller.svelte.js';
  import { successToast, warningToast } from '../../stores/toasts.svelte.js';
  import { confirm } from '../../composables/useConfirm.js';

  let { workspaceId, collectionId = null } = $props();

  // Reference data from shared workspace store
  let workspace = $derived(workspaceDataStore.workspace);
  let itemTypes = $derived(workspaceDataStore.itemTypes);
  let statuses = $derived(workspaceDataStore.statuses);
  let statusCategories = $derived(workspaceDataStore.statusCategories);

  let items = $state([]);

  let backlogItems = $derived(items);
  let loading = $state(true);
  let currentCollectionName = $state('Default');
  let showItemModal = $state(false);
  let selectedItemId = $state(null);
  let setupTimeout;
  let setupElements = new Map(); // Track which elements have drag/drop set up and their cleanup functions
  let pendingDrops = new Set(); // Track pending drop operations to prevent duplicates

  // Edge-based drag state (must use $state for Svelte 5 reactivity)
  let dragState = $state(new Map()); // Track drag state for each item: { isDragging: boolean, closestEdge: 'top'|'bottom'|null }
  const backlogRowGap = 2; // px gap between rows to keep the list tight and align the drop indicator

  // --- Iteration / Sprint section state ---
  let allIterations = $state([]);
  let addedGlobalIds = $state(new Set());
  let collapsedSections = $state(new Set());
  let sectionDropHighlight = $state(new Map()); // iterationId|'unassigned' -> boolean

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

  let addSprintPickerValue = $state(null);

  const sprintPickerConfig = {
    primary: { text: (item) => item.name },
    secondary: { text: (item) => item.status },
    searchFields: ['name'],
    getValue: (item) => item.id,
    getLabel: (item) => item.name,
  };

  function handleSprintPickerSelect(iter) {
    if (iter?.id) {
      addGlobalIteration(iter.id);
    }
    // Reset picker so it can be used again
    addSprintPickerValue = null;
  }

  // Total item count across all sections
  let totalItemCount = $derived(backlogItems.length);

  // Centralized gradient styling
  const styles = useGradientStyles();

  // Listen for newly created items
  async function handleRefreshWorkItems(event) {
    if (event.detail?.itemId) {
      try {
        const newItem = await api.items.get(event.detail.itemId);
        // When viewing a collection, accept items from any workspace (the collection defines scope).
        // Otherwise fall back to current-workspace check.
        const belongsToView = collectionId
          ? true
          : Number(newItem.workspace_id) === Number(workspaceId);
        if (belongsToView) {
          items = [...items, newItem];
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

      // Load iterations for this workspace
      try {
        const iters = await api.iterations.getAll({
          workspace_id: workspaceId,
          include_global: true,
        });
        allIterations = iters || [];
      } catch (error) {
        console.error('Failed to load iterations:', error);
      }

      restorePersistedState();
    }
    loading = false;
  });

  // Sync items from central store
  $effect(() => {
    items = collectionStore.backlogItems;
    currentCollectionName = collectionStore.collectionName;
    backlogStore.setCount(workspaceId, collectionStore.backlogPagination?.total ?? collectionStore.backlogItems.length);
  });

  // Adaptive polling for backlog items
  const poller = useWorkItemPoller(() => reloadCollection());

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

  // --- Start / Complete sprint ---
  async function startSprint(iteration) {
    try {
      await api.iterations.update(iteration.id, { status: 'active' });
      allIterations = allIterations.map(i =>
        i.id === iteration.id ? { ...i, status: 'active' } : i
      );
      successToast(t('iterations.sprintStarted', { name: iteration.name }));
    } catch (error) {
      console.error('Failed to start sprint:', error);
    }
  }

  async function completeSprint(iteration) {
    const confirmed = await confirm({
      title: t('iterations.completeSprint'),
      message: t('iterations.completeSprintConfirm', { name: iteration.name }),
      confirmText: t('iterations.complete'),
      variant: 'danger',
    });
    if (!confirmed) return;

    try {
      await api.iterations.update(iteration.id, { status: 'completed' });
      allIterations = allIterations.map(i =>
        i.id === iteration.id ? { ...i, status: 'completed' } : i
      );
      successToast(t('iterations.sprintCompleted', { name: iteration.name }));
    } catch (error) {
      console.error('Failed to complete sprint:', error);
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

  // --- Drag and Drop ---

  // Get items belonging to a specific section
  function getSectionItems(sectionId) {
    if (sectionId === 'unassigned') return unassignedItems;
    const numId = typeof sectionId === 'string' ? parseInt(sectionId) : sectionId;
    return backlogItems.filter(i => i.iteration_id === numId);
  }

  // Edge-based drag and drop setup using Pragmatic DnD
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
    dragState.clear();

    // Setup work item cards as both draggable and drop targets
    const itemCards = document.querySelectorAll('[data-item-card]');

    itemCards.forEach(element => {
      const itemId = parseInt(element.dataset.itemId);
      const sectionId = element.dataset.sectionId || 'unassigned';
      const elementId = `item-${itemId}`;

      const item = items.find(i => i.id === itemId);
      if (!item) return;

      // Initialize drag state for this item
      dragState.set(itemId, { isDragging: false, closestEdge: null });

      // Make draggable
      const draggableCleanup = draggable({
        element,
        getInitialData: () => ({
          item,
          type: 'work-item',
          sectionId: item.iteration_id || 'unassigned',
        }),
        onDragStart: () => {
          element.style.opacity = '0.5';
          document.body.classList.add('is-dragging');
          // Mark this item as being dragged - create new Map for Svelte 5 reactivity
          const state = dragState.get(itemId) || {};
          const newMap = new Map(dragState);
          newMap.set(itemId, { ...state, isDragging: true });
          dragState = newMap;
        },
        onDrop: () => {
          element.style.opacity = '';
          document.body.classList.remove('is-dragging');
          // Reset all drag states - create new Map for Svelte 5 reactivity
          const newMap = new Map();
          dragState.forEach((state, id) => {
            newMap.set(id, { isDragging: false, closestEdge: null });
          });
          dragState = newMap;
          // Clear section highlights
          sectionDropHighlight = new Map();
        }
      });

      // Make drop target with edge detection
      const dropTargetCleanup = dropTargetForElements({
        element,
        canDrop: ({ source }) => {
          const data = source.data;
          // Can't drop on self
          return data.type === 'work-item' && data.item.id !== itemId;
        },
        getData: ({ input, element }) => {
          return attachClosestEdge({ sectionId }, {
            input,
            element,
            allowedEdges: ['top', 'bottom']
          });
        },
        onDragEnter: ({ self, source }) => {
          const data = source.data;
          if (data.type === 'work-item' && data.item.id !== itemId) {
            const closestEdge = extractClosestEdge(self.data);
            const state = dragState.get(itemId) || {};
            // Create new Map to trigger Svelte 5 reactivity
            const newMap = new Map(dragState);
            newMap.set(itemId, { ...state, closestEdge });
            dragState = newMap;
          }
        },
        onDragLeave: () => {
          const state = dragState.get(itemId) || {};
          // Create new Map to trigger Svelte 5 reactivity
          const newMap = new Map(dragState);
          newMap.set(itemId, { ...state, closestEdge: null });
          dragState = newMap;
        },
        onDrop: ({ self, source }) => {
          const data = source.data;
          const closestEdge = extractClosestEdge(self.data);

          if (data.type === 'work-item' && closestEdge) {
            handleEdgeBasedDrop(data.item, item, closestEdge, sectionId);
          }
        }
      });

      setupElements.set(elementId, () => {
        draggableCleanup();
        dropTargetCleanup();
      });
    });

    // Setup section drop zones (empty sections and section headers)
    const sectionDropZones = document.querySelectorAll('[data-section-drop-zone], [data-section-header]');
    sectionDropZones.forEach(element => {
      const iterationId = element.dataset.iterationId;
      if (!iterationId) return;

      const zoneId = `section-${iterationId}-${element.dataset.sectionDropZone !== undefined ? 'zone' : 'header'}`;

      const dropTargetCleanup = dropTargetForElements({
        element,
        canDrop: ({ source }) => source.data.type === 'work-item',
        getData: () => ({ type: 'section-drop', iterationId }),
        onDragEnter: ({ source }) => {
          if (source.data.type === 'work-item') {
            const newMap = new Map(sectionDropHighlight);
            newMap.set(iterationId, true);
            sectionDropHighlight = newMap;
          }
        },
        onDragLeave: () => {
          const newMap = new Map(sectionDropHighlight);
          newMap.delete(iterationId);
          sectionDropHighlight = newMap;
        },
        onDrop: ({ source }) => {
          const data = source.data;
          if (data.type === 'work-item') {
            handleSectionDrop(data.item, iterationId);
          }
          sectionDropHighlight = new Map();
        },
      });

      setupElements.set(zoneId, dropTargetCleanup);
    });
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
        // Update local item state
        items = items.map(i => i.id === draggedItem.id ? { ...i, iteration_id: targetIterationId } : i);
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
        prev_item_id: prevItemId,
        next_item_id: nextItemId
      };
      await api.items.updateFracIndex(draggedItem.id, indexData);

      // Reload data from central store to get the correct ordering
      reloadCollection();

    } catch (error) {
      console.error('Failed to handle edge-based drop:', error);
      console.error('Error details:', error.message);

      // If we get a rank ordering error, reload fresh data
      if (error.message.includes('Internal Server Error')) {
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
      items = items.map(i => i.id === draggedItem.id ? { ...i, iteration_id: newIterationId } : i);
      reloadCollection();
    } catch (error) {
      console.error('Failed to handle section drop:', error);
    } finally {
      setTimeout(() => pendingDrops.delete(dropId), 500);
    }
  }

  // Setup drag and drop when data changes
  $effect(() => {
    // Track both items and visible iterations so drag-drop re-initializes
    // when global sprints are added/removed
    const _items = backlogItems.length;
    const _iterations = visibleIterations.length;
    if (_items > 0 && typeof document !== 'undefined') {
      if (setupTimeout) clearTimeout(setupTimeout);
      setupTimeout = setTimeout(() => {
        setupDragAndDrop();
      }, 100);
    }
  });
</script>

{#if loading}
  <div class="p-6">
    <div class="animate-pulse">{t('common.loading')}</div>
  </div>
{:else if workspace}
  <div class="min-h-screen" style="{styles.backgroundStyle}">
    <!-- Content Container -->
    <div class="p-6">
      <!-- Header with view tabs -->
      <div class="mb-8">
        <ViewHeader
          workspaceName={workspace.name}
          collection={currentCollectionName}
          viewName="Backlog"
          itemCount={totalItemCount}
          hasGradient={styles.hasCustomBackground}
          textStyle={styles.textStyle}
          subtleTextStyle={styles.subtleTextStyle}
        >
          <div slot="actions" class="flex items-center gap-2">
            {#if availableGlobalIterations.length > 0}
              <ItemPicker
                bind:value={addSprintPickerValue}
                items={availableGlobalIterations}
                config={sprintPickerConfig}
                placeholder={t('iterations.addGlobalSprint')}
                allowClear={false}
                showSelectedInTrigger={false}
                onSelect={handleSprintPickerSelect}
              >
                {#snippet children()}
                  <span
                    class="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium rounded-lg border transition-colors"
                    style="{styles.glassStyle?.(12) ?? ''} {styles.glassTextStyle ?? ''}"
                  >
                    <Plus class="w-4 h-4" />
                    {t('iterations.addGlobalSprint')}
                  </span>
                {/snippet}
              </ItemPicker>
            {/if}
            <CollectionViewSwitcher
              {workspaceId}
              {collectionId}
              activeView="backlog"
              hasGradient={styles.hasCustomBackground}
            />
          </div>
        </ViewHeader>
      </div>

      {#if backlogItems.length === 0 && visibleIterations.length === 0}
        <EmptyState
          icon={List}
          title={t('collections.noItemsInBacklog')}
          description={t('collections.noItemsInBacklogDesc')}
          hasGradient={styles.hasCustomBackground}
        />
      {:else}
        <!-- Backlog items grouped by iteration sections -->
        <div class="w-full">

          {#each iterationSections as section (section.iteration.id)}
            <BacklogSprintSection
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
              isGlobalAdded={addedGlobalIds.has(section.iteration.id)}
              sectionHighlight={sectionDropHighlight.get(String(section.iteration.id)) || false}
              onToggleCollapse={toggleCollapse}
              onOpenItem={openItem}
              onStartSprint={startSprint}
              onCompleteSprint={completeSprint}
              onRemoveGlobal={removeGlobalIteration}
            />
          {/each}

          <!-- Unassigned / Backlog section -->
          <BacklogSprintSection
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
            sectionHighlight={sectionDropHighlight.get('unassigned') || false}
            onToggleCollapse={toggleCollapse}
            onOpenItem={openItem}
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
                {#if collectionStore.backlogPagination?.total}
                  ({collectionStore.backlogPagination.total - collectionStore.backlogItems.length} {t('common.remaining')})
                {/if}
              </button>
            </div>
          {/if}

          <!-- Summary -->
          <div class="mt-8 text-center">
            <p class="text-sm" style={styles.subtleTextStyle}>
              {t('collections.showingItemsFromBacklog', { count: collectionStore.backlogPagination?.total ?? backlogItems.length })}
            </p>
          </div>
        </div>
      {/if}
    </div>
  </div>
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

<style>
  /* Improve drag feedback without layout shifts */
  [data-item-card]:hover {
    transform: translateY(-1px);
  }

  /* During drag, reduce opacity of non-dragged items slightly */
  :global(body.is-dragging) [data-item-card] {
    transition: opacity 0.2s ease;
  }
</style>
