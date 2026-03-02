<script>
  import { onMount } from 'svelte';
  import { useEventListener } from 'runed';
  import { api } from '../../api.js';
  import { navigate } from '../../router.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { collectionStore, reloadCollection } from '../../stores/collectionContext.js';
  import { useGradientStyles, loadWorkspaceGradient } from '../../stores/workspaceGradient.svelte.js';
  import { GripVertical, List } from 'lucide-svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import { draggable, dropTargetForElements } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import { attachClosestEdge, extractClosestEdge } from '@atlaskit/pragmatic-drag-and-drop-hitbox/closest-edge';
  import ItemDetail from '../items/ItemDetail.svelte';
  import WorkItemRow from '../items/WorkItemRow.svelte';
  import DropIndicator from '../../layout/DropIndicator.svelte';
  import ViewHeader from '../../layout/ViewHeader.svelte';
  import CollectionViewSwitcher from './CollectionViewSwitcher.svelte';
  import { backlogStore, workspaceDataStore } from '../../stores/index.js';
  import { useWorkItemPoller } from '../../composables/useWorkItemPoller.svelte.js';

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
          type: 'work-item'
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
          return attachClosestEdge({}, {
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
            handleEdgeBasedDrop(data.item, item, closestEdge);
          }
        }
      });
      
      setupElements.set(elementId, () => {
        draggableCleanup();
        dropTargetCleanup();
      });
    });
  }

  async function handleEdgeBasedDrop(draggedItem, targetItem, closestEdge) {
    // Create a unique identifier for this drop operation
    const dropId = `${draggedItem.id}-edge-${targetItem.id}-${closestEdge}`;
    
    try {
      // Prevent duplicate drops
      if (pendingDrops.has(dropId)) {
        return;
      }
      
      pendingDrops.add(dropId);
      
      // Find the target item's position in the sorted backlog
      const targetIndex = backlogItems.findIndex(item => item.id === targetItem.id);
      const draggedIndex = backlogItems.findIndex(item => item.id === draggedItem.id);
      
      // Remove the dragged item from consideration to get accurate neighboring items
      const otherItems = backlogItems.filter(item => item.id !== draggedItem.id);
      const adjustedTargetIndex = otherItems.findIndex(item => item.id === targetItem.id);
      
      
      // Check if we're trying to drop in the same position
      const isDroppingSamePosition = (
        (closestEdge === 'top' && draggedIndex === targetIndex - 1) ||
        (closestEdge === 'bottom' && draggedIndex === targetIndex + 1)
      );
      
      if (isDroppingSamePosition) {
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

  // Setup drag and drop when data changes
  $effect(() => {
    if (backlogItems.length > 0 && typeof document !== 'undefined') {
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
          itemCount={backlogItems.length}
          hasGradient={styles.hasCustomBackground}
          textStyle={styles.textStyle}
          subtleTextStyle={styles.subtleTextStyle}
        >
          <CollectionViewSwitcher
            slot="actions"
            {workspaceId}
            {collectionId}
            activeView="backlog"
            hasGradient={styles.hasCustomBackground}
          />
        </ViewHeader>
      </div>

      {#if backlogItems.length === 0}
        <EmptyState
          icon={List}
          title={t('collections.noItemsInBacklog')}
          description={t('collections.noItemsInBacklogDesc')}
          hasGradient={styles.hasCustomBackground}
        />
      {:else}
        <!-- Backlog items list -->
        <div class="w-full">
          
          <div class="flex flex-col" style={`row-gap: ${backlogRowGap}px;`}>
            {#each backlogItems as item (item.id)}
              <div
                class="relative"
                data-item-card
                data-item-id={item.id}
              >
                {#if dragState.get(item.id)?.closestEdge}
                  <DropIndicator edge={dragState.get(item.id)?.closestEdge} gap={backlogRowGap} />
                {/if}

                <WorkItemRow
                  {item}
                  {workspace}
                  {itemTypes}
                  {statuses}
                  {statusCategories}
                  onclick={(e) => openItem(item.id, e)}
                  showStatus={true}
                  hasGradient={styles.hasCustomBackground}
                >
                  {#snippet leading()}
                    <div class="cursor-grab active:cursor-grabbing" style={styles.dragHandleStyle}>
                      <GripVertical class="w-4 h-4" />
                    </div>
                  {/snippet}
                </WorkItemRow>
              </div>
            {/each}
          </div>
          
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
