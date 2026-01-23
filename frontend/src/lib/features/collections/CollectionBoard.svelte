<script>
  import { onMount, onDestroy } from 'svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { api } from '../../api.js';
  import { navigate } from '../../router.js';
  import { getCollection } from '../collections/collectionService.js';
  import { useGradientStyles, loadWorkspaceGradient } from '../../stores/workspaceGradient.svelte.js';
  import { Plus, GripVertical } from 'lucide-svelte';
  import { itemTypeIconMap } from '../../utils/icons.js';
  import { draggable, dropTargetForElements } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import { attachClosestEdge, extractClosestEdge } from '@atlaskit/pragmatic-drag-and-drop-hitbox/closest-edge';
  import ItemDetail from '../items/ItemDetail.svelte';
  import DropIndicator from '../../layout/DropIndicator.svelte';
  import ViewHeader from '../../layout/ViewHeader.svelte';
  import ItemKey from '../items/ItemKey.svelte';
  import CollectionViewSwitcher from './CollectionViewSwitcher.svelte';
  import { backlogStore } from '../../stores/index.js';

  // Props
  let { workspaceId, collectionId = null } = $props();

  // State
  let workspace = $state(null);
  let items = $state([]);
  let itemTypes = $state([]);
  let statuses = $state([]);
  let transitions = $state([]);
  let boardConfig = $state(null);

  let loading = $state(true);
  let currentCollectionName = $state('Default');
  let setupTimeout;
  let setupElements = new Map(); // Track which elements have drag/drop set up and their cleanup functions
  let pendingDrops = new Set(); // Track pending drop operations to prevent duplicates
  let showItemModal = $state(false);
  let selectedItemId = $state(null);

  // Backlog functionality
  let backlogItems = $state([]);

  // Edge-based drag state
  let dragState = $state(new Map()); // Track drag state for each item: { isDragging: boolean, closestEdge: 'top'|'bottom'|null }

  // Status transition caching for lazy loading
  let itemTransitions = new Map(); // Cache transitions per item ID
  let loadingTransitions = new Set(); // Track which items are currently loading transitions

  // Event handler for refresh-work-items event
  let handleRefreshWorkItems = null;

  // Centralized gradient styling
  const styles = useGradientStyles();

  onMount(async () => {
    if (workspaceId) {
      await loadWorkspaceGradient(workspaceId);
      await loadData();

      // Listen for newly created items
      handleRefreshWorkItems = async (event) => {
        if (event.detail?.itemId) {
          try {
            const newItem = await api.items.get(event.detail.itemId);
            // Add the new item if it belongs to this workspace
            if (Number(newItem.workspace_id) === Number(workspaceId)) {
              if (newItem.status_id) {
                // Item has a status, add it to the board (at the end, since board is ordered by rank)
                items = [...items, newItem];
              } else {
                // Item has no status, add it to backlog (at the end)
                backlogItems = [...backlogItems, newItem];
              }
              // Preload transitions for the new item before setting up drag and drop
              await loadStatusTransitions(newItem.id);
              // Re-setup drag and drop for the new item
              setTimeout(() => {
                setupDragAndDrop();
              }, 100);
            }
          } catch (error) {
            console.error('Failed to load new item:', error);
          }
        }
      };

      window.addEventListener('refresh-work-items', handleRefreshWorkItems);
    }
    loading = false;
  });

  onDestroy(() => {
    // Clean up event listener
    if (handleRefreshWorkItems) {
      window.removeEventListener('refresh-work-items', handleRefreshWorkItems);
      handleRefreshWorkItems = null;
    }
  });

  // Effect to reload when workspaceId or collectionId changes
  $effect(() => {
    if (workspaceId) {
      // Track dependencies
      workspaceId;
      collectionId;
      loadData();
    }
  });

  async function loadData() {
    try {
      // Build filters based on collection
      const filters = { workspace_id: workspaceId, limit: 100 };

      if (collectionId) {
        const collection = await getCollection(collectionId);
        if (collection) {
          currentCollectionName = collection.name;
          if (collection.cql_query) {
            filters.vql = collection.cql_query;
          }
        }
      } else {
        currentCollectionName = 'Default';
      }

      // First load workspace, items, backlog count, and item types
      const [workspaceData, itemsData, backlogData, itemTypesData] = await Promise.all([
        api.workspaces.get(workspaceId),
        api.items.getAll(filters),
        api.items.getBacklog(workspaceId, filters.cql || null),
        api.itemTypes.getAll()
      ]);

      workspace = workspaceData;
      // Handle paginated response from items API
      if (itemsData && itemsData.items) {
        items = itemsData.items;
      } else {
        // Backward compatibility for non-paginated response
        items = itemsData || [];
      }

      // Set backlog items and item types from backend
      backlogItems = backlogData || [];
      backlogStore.setCount(workspaceId, backlogItems.length);
      itemTypes = itemTypesData || [];

      // Get statuses for this workspace (uses workflow from config set or default workflow)
      statuses = await api.workspaces.getStatuses(workspaceId) || [];

      // Load board configuration if exists
      try {
        boardConfig = await api.collections.getBoardConfiguration(collectionId, workspaceId);
      } catch (error) {
        if (error.status !== 404) {
          console.error('Failed to load board configuration:', error);
        }
        boardConfig = null;
      }

      // Preload transitions for all items so drag validation works correctly
      await preloadAllTransitions();

    } catch (error) {
      console.error('Failed to load data:', error);
    }
  }

  // Preload status transitions for all items
  async function preloadAllTransitions() {
    const allItems = [...items, ...backlogItems];
    await Promise.all(allItems.map(item => loadStatusTransitions(item.id)));
  }

  function getItemsByStatus(statusId) {
    // Filter items by status_id
    return items.filter(item => item.status_id === statusId);
  }

  function getItemsByColumn(column) {
    // Filter items by status IDs in this column
    // column.status_ids is an array of status IDs this column should display
    return items.filter(item => column.status_ids && column.status_ids.includes(item.status_id));
  }


  // Backlog items are loaded from backend in loadData()

  // Sort statuses by category order (To Do -> In Progress -> Done categories)
  let sortedStatuses = $derived.by(() => {
    return statuses.slice().sort((a, b) => {
      // First sort by category priority
      const categoryOrder = {
        'To Do': 1,
        'In Progress': 2,
        'Done': 3
      };

      const aCategoryOrder = categoryOrder[a.category_name] || 999;
      const bCategoryOrder = categoryOrder[b.category_name] || 999;

      if (aCategoryOrder !== bCategoryOrder) {
        return aCategoryOrder - bCategoryOrder;
      }

      // Within same category, sort by name
      return a.name.localeCompare(b.name);
    });
  });

  // Compute display columns based on board configuration or fall back to sorted statuses
  let displayColumns = $derived.by(() => {
    if (boardConfig?.columns?.length > 0) {
      return boardConfig.columns.slice().sort((a, b) => a.display_order - b.display_order);
    }
    return sortedStatuses.map(status => ({
      id: status.id,
      name: status.name,
      status_ids: [status.id],
      color: status.category_color,
      wip_limit: null,
      is_default_column: true
    }));
  });

  // Calculate total visible items across all display columns
  let totalVisibleItems = $derived.by(() => {
    return displayColumns.reduce((total, column) => {
      return total + getItemsByColumn(column).length;
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
          const data = source.data;
          // Can't drop on self
          if (data.type !== 'work-item' || data.item.id === itemId) {
            return false;
          }

          // If items are in different status columns, validate the transition
          const sourceStatus = getStatusByItemId(data.item.id);
          const targetStatus = getStatusByItemId(itemId);

          if (sourceStatus && targetStatus && sourceStatus.id !== targetStatus.id) {
            // Different statuses - check if transition is valid
            return isValidTransition(data.item.id, sourceStatus.id, targetStatus.id);
          }

          // Same status or no status info - allow reordering
          return true;
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
          const data = source.data;
          const closestEdge = extractClosestEdge(self.data);

          if (data.type === 'work-item' && closestEdge) {
            const targetStatus = getStatusByItemId(itemId);
            if (targetStatus) {
              handleEdgeBasedDrop(data.item, item, closestEdge, targetStatus);
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
    const statusColumns = document.querySelectorAll('[data-status-column]');

    statusColumns.forEach(element => {
      const statusId = parseInt(element.dataset.statusId);
      const elementId = `status-${statusId}`;

      const status = statuses.find(s => s.id === statusId);
      if (!status) return;

      const cleanup = dropTargetForElements({
        element,
        canDrop: ({ source }) => {
          // Allow all work items to enter so we can show valid/invalid feedback
          // Actual validation happens in onDrop
          return source.data.type === 'work-item';
        },
        onDragEnter: ({ source }) => {
          const data = source.data;
          if (data.type === 'work-item') {
            if (isValidTransition(data.item.id, data.item.status_id, statusId)) {
              // Valid drop - use border and shadow for highlight
              element.style.borderColor = 'var(--ds-border-focused)';
              element.style.boxShadow = 'inset 0 0 0 2px var(--ds-border-focused)';
            } else {
              // Invalid drop - use border and shadow for highlight
              element.style.borderColor = 'var(--ds-border-danger)';
              element.style.boxShadow = 'inset 0 0 0 2px var(--ds-border-danger)';
            }
          }
        },
        onDragLeave: () => {
          // Reset styles
          element.style.borderColor = styles.hasGradient ? 'var(--ds-glass-border)' : 'var(--ds-border)';
          element.style.boxShadow = '';
        },
        onDrop: async ({ source }) => {
          // Reset all column styles immediately
          resetAllColumnStyles();

          const data = source.data;
          if (data.type === 'work-item') {
            if (isValidTransition(data.item.id, data.item.status_id, statusId)) {
              // Update status on backend
              await api.items.update(data.item.id, { status_id: statusId });

              // Reload data from backend to get correct ordering and state
              await loadData();

              // Re-setup drag and drop
              setTimeout(() => {
                setupDragAndDrop();
              }, 100);
            }
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
    const statusColumns = document.querySelectorAll('[data-status-column]');
    statusColumns.forEach(element => {
      element.style.borderColor = styles.hasGradient ? 'var(--ds-glass-border)' : 'var(--ds-border)';
      element.style.boxShadow = '';
    });
  }

  function getStatusByItemId(itemId) {
    const item = items.find(i => i.id === itemId);
    if (!item || !item.status_id) return null;
    return statuses.find(s => s.id === item.status_id);
  }

  // Lazy load status transitions for an item
  async function loadStatusTransitions(itemId) {
    // Return cached result if available
    if (itemTransitions.has(itemId)) {
      return itemTransitions.get(itemId);
    }

    // Don't load if already loading
    if (loadingTransitions.has(itemId)) {
      return null;
    }

    try {
      loadingTransitions.add(itemId);
      const result = await api.items.getAvailableStatusTransitions(itemId);

      // Cache the result
      itemTransitions.set(itemId, result.available_transitions || []);
      return result.available_transitions || [];
    } catch (error) {
      console.error('Failed to load status transitions:', error);
      return [];
    } finally {
      loadingTransitions.delete(itemId);
    }
  }

  // Check if a status transition is valid for an item (synchronous, uses cached data)
  function isValidTransition(itemId, fromStatusId, toStatusId) {
    if (!fromStatusId || !toStatusId) return false;
    if (fromStatusId === toStatusId) return true; // Allow staying in same status

    // Get cached transitions for this item
    const availableTransitions = itemTransitions.get(itemId);
    if (!availableTransitions) {
      // If no cached data, be restrictive - don't allow until we know it's valid
      return false;
    }

    // Check if target status ID is in the available transitions (compare by ID for reliability)
    return availableTransitions.some(transition => transition.id === toStatusId);
  }

  // Async function to check if a status transition is valid for an item
  async function isValidTransitionAsync(itemId, fromStatusId, toStatusId) {
    if (!fromStatusId || !toStatusId) return false;
    if (fromStatusId === toStatusId) return true; // Allow staying in same status

    // Get available transitions for this item
    const availableTransitions = await loadStatusTransitions(itemId);
    if (!availableTransitions) return false;

    // Check if target status ID is in the available transitions (compare by ID for reliability)
    return availableTransitions.some(transition => transition.id === toStatusId);
  }

  async function updateItemStatus(itemId, newStatus) {
    try {
      await api.items.update(itemId, { status: newStatus });

      // Update local state with a completely new array to ensure reactivity
      items = items.map(item =>
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

  async function handleEdgeBasedDrop(draggedItem, targetItem, closestEdge, targetStatus) {
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
      const targetStatusId = targetStatus.id;

      // Check if we need to update status
      const isSameStatus = currentStatusId === targetStatusId;

      // If changing status, update the status first
      if (!isSameStatus && isValidTransition(draggedItem.id, currentStatusId, targetStatusId)) {
        await api.items.update(draggedItem.id, { status_id: targetStatusId });

        // Update local state immediately for the status change
        items = items.map(item =>
          item.id === draggedItem.id
            ? { ...item, status_id: targetStatusId }
            : item
        );
      }

      // Get items in the target status for position calculation
      const statusItems = getItemsByStatus(targetStatusId);

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
    if (items.length > 0 && statuses.length > 0 && typeof document !== 'undefined') {
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
          viewName="Board"
          itemCount={items.length}
          hasGradient={styles.hasGradient}
          textStyle={styles.textStyle}
          subtleTextStyle={styles.subtleTextStyle}
        >
          <CollectionViewSwitcher
            slot="actions"
            {workspaceId}
            {collectionId}
            activeView="board"
            hasGradient={styles.hasGradient}
          />
        </ViewHeader>
      </div>

      {#if statuses.length === 0}
        <!-- No Statuses State -->
        <div class="text-center py-12">
          <div class="mb-4" style={styles.emptyStateStyle}>
            <Plus class="w-16 h-16 mx-auto" />
          </div>
          <h3 class="text-lg font-medium mb-2" style={styles.textStyle}>{t('items.noItemsInFilter')}</h3>
          <p class="text-sm mb-4" style={styles.subtleTextStyle}>
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
        <!-- Board Columns -->
        {@const validColumns = displayColumns.filter(col => col.status_ids?.length > 0)}
        <div class="grid gap-6" style="grid-template-columns: repeat({validColumns.length}, minmax(300px, 1fr));">
          {#each validColumns as column (column.id)}
            {@const columnItems = getItemsByColumn(column)}
            {@const isOverWip = column.wip_limit && columnItems.length > column.wip_limit}
            <div
              class="rounded border shadow-sm transition-colors"
              style="{styles.columnStyle(12)}"
              data-status-column
              data-status-id={column.status_ids[0]}
            >
              <div class="p-4 border-b border-l-4" style="border-bottom-color: {styles.hasGradient ? 'var(--ds-glass-border)' : 'var(--ds-border)'}; border-left-color: {column.color};">
                <h3 class="font-semibold" style={styles.glassTextStyle}>{column.name}</h3>
                <div class="flex items-center justify-between">
                  <span class="text-sm" style={styles.glassSubtleTextStyle}>{columnItems.length} {t('items.item')}</span>
                  {#if column.wip_limit}
                    <span class="text-xs px-2 py-0.5 rounded"
                          style={isOverWip
                            ? 'background-color: #ef44441A; color: #dc2626;'
                            : 'background-color: var(--ds-background-neutral, #091e420f); color: var(--ds-text-subtle, #6b778c);'}>
                      WIP: {columnItems.length}/{column.wip_limit}
                    </span>
                  {/if}
                </div>
              </div>
              <div class="p-4 min-h-32">
                {#if columnItems.length === 0}
                  <!-- Empty column state -->
                  <div class="text-center py-8" style={styles.glassSubtleTextStyle}>
                    <Plus class="w-8 h-8 mx-auto mb-2" />
                    <p class="text-sm">{t('items.noItems')}</p>
                  </div>
                {:else}
                  <div class="space-y-1">
                    {#each columnItems as item, index (item.id)}
                      {@const itemStatus = statuses.find(s => s.name.toLowerCase().replace(/ /g, '_') === item.status)}
                      <!-- Item card with edge-based drop detection -->
                      <div
                        class="relative border rounded px-3 py-3 shadow-sm hover:shadow-md transition-shadow"
                        style="{styles.cardStyle(4)}"
                        data-item-card
                        data-item-id={item.id}
                        role="button"
                        tabindex="0"
                        onclick={event => openItem(item.id, event)}
                        onkeydown={event => (event.key === 'Enter' || event.key === ' ') && openItem(item.id, event)}
                      >
                        <!-- Drop indicator -->
                        {#if dragState.get(item.id)?.closestEdge}
                          <DropIndicator edge={dragState.get(item.id)?.closestEdge} />
                        {/if}

                        <div class="flex gap-2">
                          <!-- Drag handle -->
                          <div class="cursor-grab active:cursor-grabbing flex-shrink-0" style={styles.dragHandleStyle}>
                            <GripVertical class="w-4 h-4" />
                          </div>

                          <!-- Content -->
                          <div class="flex-1 min-w-0">
                            <!-- Title - allows wrapping -->
                            <h4 class="font-medium text-sm mb-2 leading-snug" style={styles.glassTextStyle}>
                              {item.title}
                            </h4>

                            <!-- Bottom row: Key, Icon, Priority -->
                            <div class="flex items-center gap-2">
                              <ItemKey {item} {workspace} className="text-xs font-mono flex-shrink-0" style="color: var(--ds-text-subtle);" />
                              {#if item.item_type_id && itemTypes.length > 0}
                                {@const itemType = itemTypes.find(type => type.id === item.item_type_id)}
                                {#if itemType}
                                  <div
                                    class="w-4 h-4 rounded flex items-center justify-center text-white text-xs flex-shrink-0"
                                    style="background-color: {itemType.color};"
                                    title={itemType.name}
                                  >
                                    <svelte:component this={itemTypeIconMap[itemType.icon] || itemTypeIconMap.FileText} class="w-3 h-3" />
                                  </div>
                                {/if}
                              {/if}
                            </div>
                          </div>
                        </div>
                      </div>
                    {/each}
                  </div>
                {/if}
              </div>
            </div>
          {/each}
        </div>

        <!-- Summary -->
        <div class="mt-8 text-center">
          <p class="text-sm" style={styles.subtleTextStyle}>
            {t('collections.boardSummary', { itemCount: totalVisibleItems, columnCount: displayColumns.length })}
          </p>
        </div>
      {/if}
    </div>
  </div>
{:else}
  <div class="p-6">
    <div class="text-center" style="color: var(--ds-text-subtle);">
      {t('workspaces.noWorkspaces')}
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
