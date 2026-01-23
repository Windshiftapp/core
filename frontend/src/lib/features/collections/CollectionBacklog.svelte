<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { navigate } from '../../router.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { getCollection } from '../collections/collectionService.js';
  import { getStatusCategory } from '../../utils/statusColors.js';
  import Lozenge from '../../components/Lozenge.svelte';
  import { useGradientStyles, loadWorkspaceGradient } from '../../stores/workspaceGradient.svelte.js';
  import { Plus, GripVertical, List } from 'lucide-svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import { itemTypeIconMap } from '../../utils/icons.js';
  import { draggable, dropTargetForElements } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import { attachClosestEdge, extractClosestEdge } from '@atlaskit/pragmatic-drag-and-drop-hitbox/closest-edge';
  import ItemDetail from '../items/ItemDetail.svelte';
  import DropIndicator from '../../layout/DropIndicator.svelte';
  import ViewHeader from '../../layout/ViewHeader.svelte';
  import ItemKey from '../items/ItemKey.svelte';
  import CollectionViewSwitcher from './CollectionViewSwitcher.svelte';
  import { backlogStore } from '../../stores/index.js';

  let { workspaceId, collectionId = null } = $props();

  let workspace = $state(null);
  let items = $state([]);
  let itemTypes = $state([]);
  let statuses = $state([]);
  let statusCategories = $state([]);
  let statusMap = $state(new Map());
  $effect(() => {
    const entries = [];
    (statuses || []).forEach((status) => {
      if (!status || status.id === undefined || status.id === null) {
        return;
      }
      const numericId = Number(status.id);
      if (!Number.isNaN(numericId)) {
        entries.push([numericId, status]);
      }
    });
    statusMap = new Map(entries);
  });
  
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

  onMount(async () => {
    if (workspaceId) {
      await loadWorkspaceGradient(workspaceId);
      await loadData();
    }
    loading = false;

    // Listen for newly created items
    const handleRefreshWorkItems = async (event) => {
      if (event.detail?.itemId) {
        try {
          const newItem = await api.items.get(event.detail.itemId);
          // Add the new item to the end of the items array if it belongs to this workspace
          if (Number(newItem.workspace_id) === Number(workspaceId)) {
            items = [...items, newItem];
          }
        } catch (error) {
          console.error('Failed to load new item:', error);
        }
      }
    };

    window.addEventListener('refresh-work-items', handleRefreshWorkItems);

    return () => {
      window.removeEventListener('refresh-work-items', handleRefreshWorkItems);
    };
  });

  // Reactive statement to reload when workspaceId or collectionId changes
  $effect(() => {
    if (workspaceId) {
      loadData();
    }
  });

  async function loadData() {
    loading = true;
    try {
      // Get collection data once if needed
      let cqlQuery = null;
      if (collectionId) {
        const collection = await getCollection(collectionId);
        if (collection) {
          currentCollectionName = collection.name;
          if (collection.cql_query) {
            cqlQuery = collection.cql_query;
          }
        }
      } else {
        currentCollectionName = 'Default';
      }
      
      // Load workspace, backlog items, item types, and status data
      const [workspaceData, backlogItemsData, itemTypesData, statusesData, statusCategoriesData] = await Promise.all([
        api.workspaces.get(workspaceId),
        api.items.getBacklog(workspaceId, cqlQuery),
        api.itemTypes.getAll(),
        api.workspaces.getStatuses(workspaceId),
        api.statusCategories.getAll()
      ]);

      workspace = workspaceData;
      items = backlogItemsData || [];
      backlogStore.setCount(workspaceId, items.length);
      itemTypes = itemTypesData || [];
      statuses = statusesData || [];
      statusCategories = statusCategoriesData || [];
      
    } catch (error) {
      console.error('Failed to load backlog data:', error);
    } finally {
      loading = false;
    }
  }

  function getStatusName(item) {
    if (!item) return '';
    if (item.status_name && item.status_name.trim()) {
      return item.status_name;
    }
    const candidateIds = [item.status_id, item.statusId];
    for (const candidate of candidateIds) {
      if (candidate === null || candidate === undefined) continue;
      const numericId = Number(candidate);
      if (!Number.isNaN(numericId) && statusMap.has(numericId)) {
        const status = statusMap.get(numericId);
        if (status?.name) {
          return status.name;
        }
      }
    }
    if (typeof item.status === 'string' && item.status.trim()) {
      return item.status;
    }
    return '';
  }

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
    if (event?.detail?.hasChanges) {
      await loadData();
      // Re-setup drag and drop after data reload
      setTimeout(() => {
        setupDragAndDrop();
      }, 100);
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
            console.log('[Backlog DnD] onDragEnter', { itemId, closestEdge, selfData: self.data });
            const state = dragState.get(itemId) || {};
            // Create new Map to trigger Svelte 5 reactivity
            const newMap = new Map(dragState);
            newMap.set(itemId, { ...state, closestEdge });
            dragState = newMap;
            console.log('[Backlog DnD] dragState after set', { itemId, newState: dragState.get(itemId), mapSize: dragState.size });
          }
        },
        onDragLeave: () => {
          console.log('[Backlog DnD] onDragLeave', { itemId });
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
      
      // Reload data from backend to get the correct ordering
      // The backend is the single source of truth for item order
      await loadData();
      
      
      // Re-setup drag and drop
      setTimeout(() => {
        setupDragAndDrop();
      }, 100);
      
    } catch (error) {
      console.error('Failed to handle edge-based drop:', error);
      console.error('Error details:', error.message);
      
      // If we get a rank ordering error, reload fresh data from the API
      if (error.message.includes('Internal Server Error')) {
        await loadData();
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
          hasGradient={styles.hasGradient}
          textStyle={styles.textStyle}
          subtleTextStyle={styles.subtleTextStyle}
        >
          <CollectionViewSwitcher
            slot="actions"
            {workspaceId}
            {collectionId}
            activeView="backlog"
            hasGradient={styles.hasGradient}
          />
        </ViewHeader>
      </div>

      {#if backlogItems.length === 0}
        <EmptyState
          icon={List}
          title={t('collections.noItemsInBacklog')}
          description={t('collections.noItemsInBacklogDesc')}
        />
      {:else}
        <!-- Backlog items list -->
        <div class="w-full">
          
          <div class="flex flex-col" style={`row-gap: ${backlogRowGap}px;`}>
            {#each backlogItems as item, index (item.id)}
              {@const statusName = getStatusName(item)}
              {@const statusCategory = statusName ? getStatusCategory(statusName, statuses, statusCategories) : null}
              <div
                class="relative border rounded px-4 py-3 shadow-sm hover:shadow-md transition-shadow overflow-visible"
                style="{styles.cardStyle(8)}"
                data-item-card
                data-item-id={item.id}
                role="button"
                tabindex="0"
                onclick={event => openItem(item.id, event)}
                onkeydown={event => (event.key === 'Enter' || event.key === ' ') && openItem(item.id, event)}
              >
                {#if dragState.get(item.id)?.closestEdge}
                  <DropIndicator edge={dragState.get(item.id)?.closestEdge} gap={backlogRowGap} />
                {/if}

                <div class="flex items-center justify-between">
                  <div class="flex items-center gap-3 flex-1">
                    <!-- Drag handle -->
                    <div class="cursor-grab active:cursor-grabbing" style={styles.dragHandleStyle}>
                      <GripVertical class="w-4 h-4" />
                    </div>
                    <!-- Title with key and item type icon -->
                    <div class="flex items-center gap-2">
                      <ItemKey {item} {workspace} className="text-xs font-mono" style={styles.glassSubtleTextStyle} />
                      {#if item.item_type_id && itemTypes.length > 0}
                        {@const itemType = itemTypes.find(type => type.id === item.item_type_id)}
                        {#if itemType}
                          <div
                            class="w-4 h-4 rounded flex items-center justify-center text-white text-xs"
                            style="background-color: {itemType.color};"
                            title={itemType.name}
                          >
                            <svelte:component this={itemTypeIconMap[itemType.icon] || itemTypeIconMap.FileText} class="w-3 h-3" />
                          </div>
                        {/if}
                      {/if}
                      <h4 class="font-medium text-sm" style="color: var(--ds-text);">
                        {item.title}
                      </h4>
                    </div>

                  </div>

                  <!-- Status on the right -->
                  <Lozenge
                    text={statusName ? statusName.replace(/_/g, ' ') : 'Status'}
                    customBg={statusCategory?.color || '#6b7280'}
                  />
                </div>
              </div>
            {/each}
          </div>
          
          <!-- Summary -->
          <div class="mt-8 text-center">
            <p class="text-sm" style={styles.subtleTextStyle}>
              {t('collections.showingItemsFromBacklog', { count: backlogItems.length })}
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
