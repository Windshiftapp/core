<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { navigate } from '../../router.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { getCollection, checkItemVisibility } from '../collections/collectionService.js';
  import { workspaceGradientIndex, applyToAllViews, loadWorkspaceGradient, getGradientStyle } from '../../stores/workspaceGradient.js';
  import { gradients } from '../../utils/gradients.js';
  import { FileText, Plus, ChevronDown, Package, ChevronRight, Home, MapPin } from 'lucide-svelte';
  import { itemTypeIconMap } from '../../utils/icons.js';
  import EmptyState from '../../components/EmptyState.svelte';
  import { monitorForElements } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import { draggable, dropTargetForElements } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import Tooltip from '../../components/Tooltip.svelte';
  import ViewHeader from '../../layout/ViewHeader.svelte';
  import ItemDetail from '../items/ItemDetail.svelte';
  import { infoToast, errorToast } from '../../stores/toasts.svelte.js';
  import ItemKey from '../items/ItemKey.svelte';
  import ItemCard from '../items/ItemCard.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import { getStatusCategory } from '../../utils/statusColors.js';
  import CollectionViewSwitcher from './CollectionViewSwitcher.svelte';

  export let workspaceId;
  export let collectionId = null;

  let workspace = null;
  let loading = true;
  let backboneItems = []; // Current backbone items (horizontal)
  let childItemsByParent = {}; // Child items grouped by parent ID
  let itemTypes = [];
  let statuses = [];
  let statusCategories = [];
  let workspaces = [];
  let hierarchyLevels = [];
  let currentParentId = null; // null = root level, otherwise parent ID for current backbone
  let hierarchyBreadcrumbs = []; // Navigation breadcrumbs for hierarchy levels

  // Use centralized icon map for item types
  const iconMap = itemTypeIconMap;

  let currentCollectionName = 'Default';

  // Quick-add state per parent
  let quickAddState = {}; // { [parentId]: { show: boolean, workspace: null, itemType: null, title: '', error: null } }

  // Inline editing state
  let editingItemId = null;
  let editingTitle = '';

  // Item detail modal state
  let selectedItemId = null;
  let showItemModal = false;


  // Reactive gradient styling
  $: gradientStyle = ($applyToAllViews && $workspaceGradientIndex > 0) ? getGradientStyle($workspaceGradientIndex) : null;
  $: hasGradient = gradientStyle !== null;
  $: backgroundStyle = hasGradient ? `background: ${gradientStyle};` : 'background-color: var(--ds-surface);';

  // Text directly on gradient background (white for visibility)
  $: textStyle = hasGradient ? 'color: white;' : 'color: var(--ds-text);';
  $: subtleTextStyle = hasGradient ? 'color: rgba(255, 255, 255, 0.8);' : 'color: var(--ds-text-subtle);';
  $: emptyStateStyle = hasGradient ? 'color: rgba(255, 255, 255, 0.6);' : 'color: var(--ds-text-subtlest);';

  // Glass styling for cards (theme-aware)
  $: cardBgStyle = hasGradient
    ? 'background-color: var(--ds-glass-bg); backdrop-filter: blur(8px); border-color: var(--ds-glass-border);'
    : 'background-color: var(--ds-surface-raised); border-color: var(--ds-border);';
  $: glassTextStyle = 'color: var(--ds-text);';
  $: glassSubtleTextStyle = 'color: var(--ds-text-subtle);';
  $: dragHandleStyle = 'color: var(--ds-text-subtlest);';

  onMount(async () => {
    await loadWorkspaceGradient(workspaceId);
    await loadAllData();

    // Listen for browser back/forward navigation
    const handlePopState = () => {
      loadStoryMapDataFromURL();
    };

    // Close dropdowns when clicking outside
    const handleClickOutside = (event) => {
      const dropdowns = document.querySelectorAll('[id^="workspace-dropdown-"], [id^="itemtype-dropdown-"]');
      dropdowns.forEach(dropdown => {
        if (!dropdown.classList.contains('hidden') && !dropdown.contains(event.target) && !event.target.closest('button')) {
          dropdown.classList.add('hidden');
        }
      });
    };

    window.addEventListener('popstate', handlePopState);
    document.addEventListener('click', handleClickOutside);

    // Cleanup listener on component destroy
    return () => {
      window.removeEventListener('popstate', handlePopState);
      document.removeEventListener('click', handleClickOutside);
    };
  });

  // Reactive statement to reload when workspaceId or collectionId changes
  $: if (workspaceId) {
    // Watch both workspaceId and collectionId for changes
    workspaceId, collectionId, loadAllData();
  }

  async function loadAllData() {
    loading = true;
    if (workspaceId) {
      await Promise.all([
        loadWorkspace(),
        loadWorkspaces(),
        loadItemTypesAndHierarchyLevels(),
        loadStoryMapDataFromURL(),
        loadStatuses()
      ]);
    }
    loading = false;
  }

async function loadWorkspaces() {
  try {
    workspaces = await api.workspaces.getAll() || [];
  } catch (error) {
    console.error('Failed to load workspaces:', error);
    workspaces = [];
  }
}

async function loadStatuses() {
  try {
    const [statusesData, statusCategoriesData] = await Promise.all([
      api.workspaces.getStatuses(workspaceId),
      api.statusCategories.getAll()
    ]);
    statuses = statusesData || [];
    statusCategories = statusCategoriesData || [];
  } catch (error) {
    console.error('Failed to load statuses:', error);
    statuses = [];
    statusCategories = [];
  }
}

  async function loadItemTypesAndHierarchyLevels() {
    try {
      const [itemTypesResult, hierarchyLevelsResult] = await Promise.all([
        api.itemTypes.getAll(),
        api.hierarchyLevels.getAll()
      ]);
      itemTypes = itemTypesResult || [];
      hierarchyLevels = hierarchyLevelsResult || [];
    } catch (error) {
      console.error('Failed to load item types:', error);
      itemTypes = [];
      hierarchyLevels = [];
    }
  }

  function loadStoryMapDataFromURL() {
    // Get parent ID from URL parameters
    const urlParams = new URLSearchParams(window.location.search);
    const parentParam = urlParams.get('parent');
    const parentId = parentParam ? parseInt(parentParam) : null;
    
    return loadStoryMapData(parentId);
  }

  // Set up drag and drop whenever the data changes
  $: if (backboneItems.length > 0 && !loading) {
    // Use setTimeout to ensure DOM has been updated
    setTimeout(setupDragAndDrop, 0);
  }

  async function loadWorkspace() {
    try {
      workspace = await api.workspaces.get(workspaceId);
    } catch (error) {
      console.error('Failed to load workspace:', error);
    }
  }

  async function updateHierarchyBreadcrumbs() {
    const newBreadcrumbs = [];

    if (currentParentId === null) {
      // At root level
      newBreadcrumbs.push({
        id: null,
        title: t('collections.rootLevel'),
        level: 'root',
        itemType: null
      });
    } else {
      // Build the full hierarchy path by walking up from current parent
      const pathItems = [];
      let currentId = currentParentId;

      try {
        // Walk up the hierarchy to build the path
        while (currentId !== null) {
          const item = await api.items.get(currentId);
          pathItems.unshift(item); // Add to beginning of array
          currentId = item.parent_id;
        }

        // Add root level first
        newBreadcrumbs.push({
          id: null,
          title: t('collections.rootLevel'),
          level: 'root',
          itemType: null
        });

        // Add each level in the path
        pathItems.forEach((item, index) => {
          const isLast = index === pathItems.length - 1;
          const itemType = getItemTypeInfo(item.item_type_id);
          newBreadcrumbs.push({
            id: item.id,
            title: item.title,
            level: isLast ? 'current' : 'intermediate',
            itemType: itemType,
            isCurrent: isLast
          });
        });

      } catch (error) {
        console.error('Failed to build hierarchy path:', error);
        // Fallback to simple breadcrumb
        newBreadcrumbs.push({
          id: null,
          title: t('collections.rootLevel'),
          level: 'root',
          itemType: null
        });
        newBreadcrumbs.push({
          id: currentParentId,
          title: t('collections.currentLevel'),
          level: 'current',
          itemType: null,
          isCurrent: true
        });
      }
    }

    // Force reactive update by reassigning
    hierarchyBreadcrumbs = newBreadcrumbs;
  }

  async function loadStoryMapData(parentId = null) {
    try {
      // Build filters based on collection
      const filters = { workspace_id: workspaceId };
      
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
      
      // Load all items and item types
      const [itemsResponse, itemTypesResponse] = await Promise.all([
        api.items.getAll(filters),
        api.itemTypes.getAll()
      ]);
      
      const items = Array.isArray(itemsResponse) ? itemsResponse : itemsResponse.items || [];
      itemTypes = itemTypesResponse;

      // Set current parent ID
      currentParentId = parentId;

      // Get backbone items based on current parent level
      if (parentId === null) {
        // Root level - items with no parent
        backboneItems = items
          .filter(item => !item.parent_id)
          .sort((a, b) => a.id - b.id);
      } else {
        // Child level - direct children of the specified parent
        backboneItems = items
          .filter(item => item.parent_id === parentId)
          .sort((a, b) => a.id - b.id);
      }

      // Group child items by their parent ID (children of current backbone items)
      childItemsByParent = {};
      items
        .filter(item => item.parent_id && backboneItems.some(backbone => backbone.id === item.parent_id))
        .forEach(child => {
          if (!childItemsByParent[child.parent_id]) {
            childItemsByParent[child.parent_id] = [];
          }
          childItemsByParent[child.parent_id].push(child);
        });

      // Sort child items within each parent group
      Object.keys(childItemsByParent).forEach(parentId => {
        childItemsByParent[parentId].sort((a, b) => a.id - b.id);
      });

      // Update breadcrumbs
      await updateHierarchyBreadcrumbs();

    } catch (error) {
      console.error('Failed to load story map data:', error);
    }
  }

  function updateURL(parentId) {
    const url = new URL(window.location);
    if (parentId === null) {
      url.searchParams.delete('parent');
    } else {
      url.searchParams.set('parent', parentId.toString());
    }
    window.history.pushState({}, '', url);
  }

  function navigateToLevel(parentId) {
    updateURL(parentId);
    loadStoryMapData(parentId);
  }

  function drillDown(backboneItemId) {
    // Navigate to show the children of this backbone item as the new backbone
    updateURL(backboneItemId);
    loadStoryMapData(backboneItemId);
  }

  let dragDropCleanup = null;

  function setupDragAndDrop() {
    // Clean up existing drag/drop registrations
    if (dragDropCleanup) {
      dragDropCleanup();
    }

    // Monitor for drag and drop events
    const monitor = monitorForElements({
      onDrop({ source, location }) {
        const draggedItemId = parseInt(source.data.itemId);
        const targetParentId = location.current.dropTargets.length > 0 
          ? parseInt(location.current.dropTargets[0].data.parentId)
          : null;


        if (targetParentId && draggedItemId) {
          moveItemToParent(draggedItemId, targetParentId);
        } else {
        }
      }
    });

    // Set up draggable items
    const draggableCleanups = [];
    document.querySelectorAll('[data-testid^="draggable-item"]').forEach(element => {
      const cleanup = draggable({
        element,
        getInitialData: () => ({
          itemId: element.getAttribute('data-item-id')
        })
      });
      draggableCleanups.push(cleanup);
    });

    // Set up drop zones
    const dropTargetCleanups = [];
    document.querySelectorAll('[data-testid^="drop-zone"]').forEach(element => {
      const cleanup = dropTargetForElements({
        element,
        getData: () => ({
          parentId: element.getAttribute('data-parent-id')
        }),
        onDragEnter: () => {
          element.style.borderColor = 'var(--ds-border-focused)';
          element.style.boxShadow = 'inset 0 0 0 2px var(--ds-border-focused)';
        },
        onDragLeave: () => {
          element.style.borderColor = hasGradient ? 'var(--ds-glass-border)' : 'var(--ds-border)';
          element.style.boxShadow = '';
        },
        onDrop: () => {
          element.style.borderColor = hasGradient ? 'var(--ds-glass-border)' : 'var(--ds-border)';
          element.style.boxShadow = '';
        }
      });
      dropTargetCleanups.push(cleanup);
    });

    // Store cleanup function for next time
    dragDropCleanup = () => {
      monitor();
      draggableCleanups.forEach(cleanup => cleanup());
      dropTargetCleanups.forEach(cleanup => cleanup());
    };
  }

  async function moveItemToParent(itemId, newParentId) {
    try {

      // Update the item's parent_id
      const result = await api.items.update(itemId, { parent_id: newParentId });

      // Reload the story map data, maintaining the current hierarchy level
      await loadStoryMapData(currentParentId);

      // The reactive statement will handle drag and drop re-setup automatically
    } catch (error) {
      console.error('Failed to move item:', error);

      // Show user-friendly error toast
      errorToast(error.message || 'Failed to move item due to hierarchy constraints', 'Cannot move item');
    }
  }

  function getItemTypeInfo(itemTypeId) {
    return itemTypes.find(type => type.id === itemTypeId);
  }

  function navigateToItem(itemId) {
    const url = collectionId 
      ? `/workspaces/${workspaceId}/collections/${collectionId}/items/${itemId}`
      : `/workspaces/${workspaceId}/items/${itemId}`;
    navigate(url);
  }

  function getPriorityColor(priority) {
    switch (priority?.toLowerCase()) {
      case 'critical': return 'bg-red-100 text-red-800';
      case 'high': return 'bg-orange-100 text-orange-800';
      case 'medium': return 'bg-yellow-100 text-yellow-800';
      case 'low': return 'bg-green-100 text-green-800';
      default: return 'bg-gray-100 text-gray-800';
    }
  }

  // Quick-add functions
  function canAddChildren(parentId) {
    // Get parent item to determine hierarchy level
    const parentItem = backboneItems.find(item => item.id === parentId);

    if (!parentItem || !parentItem.item_type_id) {
      return true; // Allow adding children if we can't determine parent type
    }

    const parentType = getItemTypeInfo(parentItem.item_type_id);
    if (!parentType) {
      return true;
    }

    // Check if there are any item types at the next hierarchy level
    const childTypes = itemTypes.filter(type =>
      type.hierarchy_level === parentType.hierarchy_level + 1
    );

    return childTypes.length > 0;
  }

  function initQuickAdd(parentId) {
    // Check if this parent can have children
    if (!canAddChildren(parentId)) {
      return; // Don't initialize quick-add for items at lowest hierarchy level
    }

    // Get parent item to determine hierarchy level
    const parentItem = backboneItems.find(item => item.id === parentId);

    // Get available item types for children (next hierarchy level)
    let availableTypes = [];
    if (parentItem && parentItem.item_type_id) {
      const parentType = getItemTypeInfo(parentItem.item_type_id);
      if (parentType) {
        // Find item types that are one level below the parent
        availableTypes = itemTypes.filter(type =>
          type.hierarchy_level === parentType.hierarchy_level + 1
        ).sort((a, b) => a.sort_order - b.sort_order);
      }
    }

    // If still no types found after all checks, don't proceed
    if (availableTypes.length === 0) {
      return;
    }

    // Preselect workspace: current workspace > single workspace > null
    let preselectedWorkspaceId = null;
    if (workspaceId) {
      // Use current workspace if available
      preselectedWorkspaceId = parseInt(workspaceId);
    } else if (workspaces.length === 1) {
      // Fall back to single workspace if only one exists
      preselectedWorkspaceId = workspaces[0].id;
    }

    quickAddState = {
      ...quickAddState,
      [parentId]: {
        show: true,
        workspaceId: preselectedWorkspaceId,
        itemTypeId: availableTypes.length > 0 ? availableTypes[0].id : null,
        availableTypes: availableTypes,
        title: '',
        error: null
      }
    };

    // Focus the textarea after it's rendered
    setTimeout(() => {
      const textarea = document.querySelector(`textarea[data-quick-add-parent="${parentId}"]`);
      if (textarea) {
        textarea.focus();
      }
    }, 0);
  }

  function cancelQuickAdd(parentId) {
    const newState = { ...quickAddState };
    delete newState[parentId];
    quickAddState = newState;
  }

  async function createChildItem(parentId) {
    const state = quickAddState[parentId];
    if (!state) return;

    // Validate
    if (!state.workspaceId) {
      quickAddState[parentId].error = 'Please select a workspace';
      quickAddState = { ...quickAddState };
      return;
    }
    if (!state.itemTypeId) {
      quickAddState[parentId].error = 'Please select an item type';
      quickAddState = { ...quickAddState };
      return;
    }
    if (!state.title?.trim()) {
      quickAddState[parentId].error = 'Please enter a title';
      quickAddState = { ...quickAddState };
      return;
    }

    try {
      // Create the item
      const newItem = await api.items.create({
        workspace_id: state.workspaceId,
        item_type_id: state.itemTypeId,
        title: state.title.trim(),
        description: '',
        status: 'open',
        priority: 'medium',
        parent_id: parentId
      });

      // Check if the created item will be visible in the current collection view
      const filters = { workspace_id: workspaceId };

      if (collectionId) {
        const collection = await getCollection(collectionId);
        if (collection?.cql_query) {
          filters.vql = collection.cql_query;
        }
      }

      const isVisible = await checkItemVisibility(newItem.id, filters);

      // Show toast notification if item won't be visible
      if (!isVisible) {
        const selectedWorkspace = workspaces.find(w => w.id === state.workspaceId);
        const workspaceName = selectedWorkspace?.name || 'another workspace';
        infoToast(`Card created in ${workspaceName} but won't appear here due to collection filters`, 'Card created successfully');
      }

      // Reset state and reload
      cancelQuickAdd(parentId);
      await loadStoryMapData(currentParentId);
    } catch (error) {
      console.error('Failed to create child item:', error);
      quickAddState[parentId].error = 'Failed to create item: ' + (error.message || error);
      quickAddState = { ...quickAddState };
    }
  }

  function updateQuickAddField(parentId, field, value) {
    if (quickAddState[parentId]) {
      quickAddState[parentId][field] = value;
      quickAddState[parentId].error = null;
      quickAddState = { ...quickAddState };
    }
  }

  // Inline editing functions
  function startEditingItem(item, event) {
    event.stopPropagation();
    event.preventDefault(); // Prevent default double-click behavior

    editingItemId = item.id;
    editingTitle = item.title;

    // Focus the textarea after it's rendered
    setTimeout(() => {
      const textarea = document.querySelector(`textarea[data-item-id="${item.id}"]`);
      if (textarea) {
        textarea.focus();
        textarea.select();
      }
    }, 0);
  }

  function cancelEditingItem() {
    editingItemId = null;
    editingTitle = '';
  }

  async function saveEditingItem(item) {
    if (!editingTitle.trim()) {
      cancelEditingItem();
      return;
    }

    if (editingTitle === item.title) {
      cancelEditingItem();
      return;
    }

    try {
      await api.items.update(item.id, { title: editingTitle.trim() });

      // Update the local item
      item.title = editingTitle.trim();

      // Force reactivity
      backboneItems = [...backboneItems];
      childItemsByParent = { ...childItemsByParent };

      cancelEditingItem();
    } catch (error) {
      console.error('Failed to update item title:', error);
      cancelEditingItem();
    }
  }

  function handleKeyClick(item, event) {
    event.stopPropagation();
    openItemModal(item.id, event);
  }

  function openItemModal(itemId, event) {
    if (event) {
      event.stopPropagation();
    }
    selectedItemId = itemId;
    showItemModal = true;
  }

  async function closeItemModal(event) {
    showItemModal = false;
    selectedItemId = null;

    // If changes were made in the modal, reload data
    if (event?.detail?.hasChanges) {
      await loadStoryMapDataFromURL();
    }
  }
</script>

{#if loading}
  <div class="p-6">
    <div class="animate-pulse">{t('collections.loadingStoryMap')}</div>
  </div>
{:else if workspace}
  <div style="{backgroundStyle} min-height: 100vh;">
    <!-- Header -->
    <div class="p-6 border-b" style="border-color: {hasGradient ? 'var(--ds-glass-border)' : 'var(--ds-border)'};">

      <ViewHeader
        workspaceName={workspace.name}
        collection={currentCollectionName}
        viewName="Map"
        itemCount={backboneItems.length + Object.values(childItemsByParent).flat().length}
        hasGradient={hasGradient}
        textStyle={textStyle}
        subtleTextStyle={subtleTextStyle}
      />
      
      <!-- Hierarchy Breadcrumbs -->
      {#if hierarchyBreadcrumbs.length > 0}
        <div class="mt-4">
          <!-- Breadcrumb Navigation -->
          <div class="flex items-center gap-1 flex-wrap">
            {#each hierarchyBreadcrumbs as breadcrumb, index (breadcrumb.id)}
              <!-- Separator -->
              {#if index > 0}
                <ChevronRight class="w-4 h-4 " />
              {/if}

              <!-- Breadcrumb Button -->
              <button
                onclick={() => navigateToLevel(breadcrumb.id)}
                class="flex items-center gap-1.5 px-2.5 py-1.5 rounded-md transition-all {breadcrumb.isCurrent && !hasGradient ? 'bg-blue-50' : ''} {!breadcrumb.isCurrent && !hasGradient ? 'hover-bg' : ''} {hasGradient ? 'backdrop-blur-md' : ''} {breadcrumb.isCurrent ? 'font-medium' : ''}"
                style="{breadcrumb.isCurrent ? `color: ${hasGradient ? 'var(--ds-text)' : 'var(--ds-interactive)'}; background-color: ${hasGradient ? 'var(--ds-glass-bg)' : ''}; border: 1px solid ${hasGradient ? 'var(--ds-glass-border)' : 'var(--ds-border-focused)'};` : `color: var(--ds-text); background-color: ${hasGradient ? 'var(--ds-glass-bg)' : ''}; border: 1px solid transparent;`}"
              >
                <!-- Icon -->
                {#if breadcrumb.level === 'root'}
                  <Home class="w-3.5 h-3.5" />
                {:else if breadcrumb.itemType}
                  <div
                    class="w-4 h-4 rounded flex items-center justify-center"
                    style="background-color: {breadcrumb.itemType.color};"
                  >
                    <svelte:component
                      this={iconMap[breadcrumb.itemType.icon] || FileText}
                      class="w-2.5 h-2.5 text-white"
                    />
                  </div>
                {/if}

                <!-- Text -->
                <span class="text-sm">
                  {#if breadcrumb.itemType && breadcrumb.level !== 'root'}
                    <span class="font-medium">{breadcrumb.itemType.name}:</span>
                  {/if}
                  {breadcrumb.title}
                </span>
              </button>
            {/each}
          </div>

          <!-- Level Summary (for current level) -->
          {#if hierarchyBreadcrumbs.length > 0}
            {@const currentBreadcrumb = hierarchyBreadcrumbs[hierarchyBreadcrumbs.length - 1]}
            {#if currentBreadcrumb.isCurrent}
              <div class="mt-3 flex items-center gap-3 text-xs ">
                <span>
                  Showing <strong>{backboneItems.length}</strong> {currentBreadcrumb.itemType?.name || 'item'}{backboneItems.length !== 1 ? 's' : ''}
                </span>
                <span>•</span>
                <span>
                  <strong>{Object.values(childItemsByParent).flat().length}</strong> child item{Object.values(childItemsByParent).flat().length !== 1 ? 's' : ''}
                </span>
              </div>
            {/if}
          {/if}
        </div>
      {/if}
    </div>

    <!-- Story Map Container -->
    <div class="p-6 overflow-x-auto">
      <div class="min-w-max">
        <!-- Backbone (Horizontal) -->
        <div class="flex gap-6 mb-8">
          {#each backboneItems as backboneItem}
            {@const itemType = getItemTypeInfo(backboneItem.item_type_id)}
            <div class="flex-none w-64">
              <!-- Backbone Item -->
              <div class="mb-3">
                <ItemCard {hasGradient} compact>
                  <!-- Title -->
                  <button
                    onclick={() => navigateToItem(backboneItem.id)}
                    class="font-medium text-sm mb-2 leading-snug text-left w-full line-clamp-3 transition-colors"
                    style="{glassTextStyle}"
                  >
                    {backboneItem.title}
                  </button>

                  <!-- Bottom row: Key, Icon, Status, Drill Down -->
                  <div class="flex items-center justify-between">
                    <div class="flex items-center gap-2">
                      <button
                        class="text-xs font-mono flex-shrink-0 hover:underline cursor-pointer"
                        style="{glassSubtleTextStyle}"
                        onclick={(e) => handleKeyClick(backboneItem, e)}
                      >
                        <ItemKey item={backboneItem} {workspace} className="" />
                      </button>
                      {#if itemType}
                        <div
                          class="w-4 h-4 rounded flex items-center justify-center text-white text-xs flex-shrink-0"
                          style="background-color: {itemType.color};"
                          title={itemType.name}
                        >
                          <svelte:component this={iconMap[itemType.icon] || FileText} class="w-3 h-3" />
                        </div>
                      {/if}
                      <Lozenge
                        text={(backboneItem.status_name || backboneItem.status)?.replace('_', ' ') || 'Status'}
                        customBg={getStatusCategory(backboneItem.status_name || backboneItem.status, statuses, statusCategories)?.color || 'var(--ds-text-subtle)'}
                      />
                    </div>

                    <!-- Drill Down Arrow (only show if item has children) -->
                    {#if childItemsByParent[backboneItem.id]?.length > 0}
                      <Tooltip content={t('collections.drillDown')}>
                        {#snippet children()}
                          <button
                            onclick={() => drillDown(backboneItem.id)}
                            class="p-1.5 rounded-full transition-colors group"
                            style="color: var(--ds-interactive);"
                            onmouseenter={(e) => e.currentTarget.style.background = 'var(--ds-surface-hovered)'}
                            onmouseleave={(e) => e.currentTarget.style.background = ''}
                          >
                            <ChevronDown class="w-3.5 h-3.5 group-hover:scale-110 transition-transform" />
                          </button>
                        {/snippet}
                      </Tooltip>
                    {/if}
                  </div>
                </ItemCard>
              </div>

              <!-- Drop Zone for this parent -->
              <div
                class="min-h-96 p-3 rounded border-2 border-dashed transition-all {hasGradient ? 'backdrop-blur-sm' : ''}"
                style="{hasGradient ? 'border-color: var(--ds-glass-border); background-color: var(--ds-glass-bg);' : 'border-color: var(--ds-border); background-color: var(--ds-surface-overlay);'}"
                data-parent-id={backboneItem.id}
                data-testid="drop-zone-{backboneItem.id}"
              >
                <h3 class="text-sm font-medium mb-3 text-center" style={glassTextStyle}>
                  {t('collections.childWorkItems', { count: childItemsByParent[backboneItem.id]?.length || 0 })}
                </h3>

                <!-- Child Items Column -->
                <div class="space-y-2">
                  {#each childItemsByParent[backboneItem.id] || [] as childItem}
                    {@const childItemType = getItemTypeInfo(childItem.item_type_id)}
                    <div
                      class="item-card rounded-lg border p-2 cursor-move"
                      style="{hasGradient ? 'backdrop-filter: blur(12px); background-color: var(--ds-glass-bg);' : 'background-color: var(--ds-surface-card);'} {hasGradient ? 'border-color: var(--ds-glass-border);' : 'border-color: var(--ds-border);'}"
                      data-item-id={childItem.id}
                      data-testid="draggable-item-{childItem.id}"
                      ondblclick={(e) => startEditingItem(childItem, e)}
                    >
                      <!-- Title -->
                      {#if editingItemId === childItem.id}
                        <textarea
                          bind:value={editingTitle}
                          data-item-id={childItem.id}
                          class="font-medium text-sm mb-2 leading-snug w-full resize-none overflow-hidden bg-transparent border-none outline-none p-0 m-0"
                          style="color: var(--ds-text); caret-color: var(--ds-text);"
                          rows="3"
                          onblur={() => saveEditingItem(childItem)}
                          onkeydown={(e) => {
                            if (e.key === 'Enter' && !e.shiftKey) {
                              e.preventDefault();
                              saveEditingItem(childItem);
                            } else if (e.key === 'Escape') {
                              e.preventDefault();
                              cancelEditingItem();
                            }
                          }}
                          onclick={(e) => e.stopPropagation()}
                        />
                      {:else}
                        <h4 class="font-medium text-sm mb-2 leading-snug line-clamp-3" style="{glassTextStyle}">
                          {childItem.title}
                        </h4>
                      {/if}

                      <!-- Bottom row: Key, Icon, Status -->
                      <div class="flex items-center gap-2 flex-wrap">
                        <button
                          class="text-xs font-mono flex-shrink-0 hover:underline cursor-pointer"
                          style="{glassSubtleTextStyle}"
                          onclick={(e) => handleKeyClick(childItem, e)}
                        >
                          <ItemKey item={childItem} {workspace} className="" />
                        </button>
                        {#if childItemType}
                          <div
                            class="w-4 h-4 rounded flex items-center justify-center text-white text-xs flex-shrink-0"
                            style="background-color: {childItemType.color};"
                            title={childItemType.name}
                          >
                            <svelte:component this={iconMap[childItemType.icon] || FileText} class="w-3 h-3" />
                          </div>
                        {/if}
                        <Lozenge
                          text={(childItem.status_name || childItem.status)?.replace('_', ' ') || 'Status'}
                          customBg={getStatusCategory(childItem.status_name || childItem.status, statuses, statusCategories)?.color || 'var(--ds-text-subtle)'}
                        />
                      </div>
                    </div>
                  {/each}

                  <!-- Add Card button when there are existing items -->
                  {#if !quickAddState[backboneItem.id]?.show && childItemsByParent[backboneItem.id]?.length > 0 && canAddChildren(backboneItem.id)}
                    <button
                      onclick={() => initQuickAdd(backboneItem.id)}
                      class="w-full flex items-center gap-2 px-3 py-2 text-sm font-medium rounded border-2 border-dashed transition-colors "
                      style="{hasGradient ? 'border-color: var(--ds-glass-border);' : 'border-color: var(--ds-border);'} background-color: transparent; color: var(--ds-text-subtle);"
                      onmouseenter={(e) => {
                        e.currentTarget.style.borderColor = 'var(--ds-border-focused)';
                        e.currentTarget.style.color = 'var(--ds-interactive)';
                      }}
                      onmouseleave={(e) => {
                        e.currentTarget.style.borderColor = hasGradient ? 'var(--ds-glass-border)' : 'var(--ds-border)';
                        e.currentTarget.style.color = 'var(--ds-text-subtle)';
                      }}
                    >
                      <Plus class="w-4 h-4" />
                      {t('collections.addCard')}
                    </button>
                  {/if}

                  <!-- Quick Add / Empty State -->
                  {#if !quickAddState[backboneItem.id]?.show && (!childItemsByParent[backboneItem.id] || childItemsByParent[backboneItem.id].length === 0)}
                    <div class="text-center py-8">
                      <div class="text-sm mb-3" style={glassSubtleTextStyle}>
                        {#if canAddChildren(backboneItem.id)}
                          {t('collections.noChildItems')}
                        {:else}
                          {t('collections.noChildItemsLowest')}
                        {/if}
                      </div>
                      {#if canAddChildren(backboneItem.id)}
                        <button
                          onclick={() => initQuickAdd(backboneItem.id)}
                          class="inline-flex items-center gap-2 px-3 py-2 text-sm font-medium rounded border-2 border-dashed transition-colors {hasGradient ? 'backdrop-blur-sm' : ''}"
                          style="{hasGradient ? 'border-color: var(--ds-glass-border); background-color: var(--ds-glass-bg);' : 'border-color: var(--ds-border); background-color: var(--ds-surface-overlay);'} color: var(--ds-interactive);"
                          onmouseenter={(e) => e.currentTarget.style.borderColor = 'var(--ds-border-focused)'}
                          onmouseleave={(e) => e.currentTarget.style.borderColor = hasGradient ? 'var(--ds-glass-border)' : 'var(--ds-border)'}
                        >
                          <Plus class="w-4 h-4" />
                          {t('collections.addCard')}
                        </button>
                      {/if}
                    </div>
                  {/if}

                  <!-- Quick Add Form -->
                  {#if quickAddState[backboneItem.id]?.show}
                    {@const state = quickAddState[backboneItem.id]}
                    {@const selectedWorkspace = workspaces.find(w => w.id === state.workspaceId)}
                    {@const selectedItemType = state.availableTypes?.find(t => t.id === state.itemTypeId)}
                    <div class="rounded shadow-md border" style="{cardBgStyle}">
                      <!-- Textarea area -->
                      <div class="p-3 pb-2">
                        <!-- Title Textarea - Full width, blends in -->
                        <textarea
                          bind:value={state.title}
                          data-quick-add-parent={backboneItem.id}
                          oninput={(e) => updateQuickAddField(backboneItem.id, 'title', e.target.value)}
                          onkeydown={(e) => {
                            if (e.key === 'Enter' && !e.shiftKey) {
                              e.preventDefault();
                              createChildItem(backboneItem.id);
                            } else if (e.key === 'Escape') {
                              cancelQuickAdd(backboneItem.id);
                            }
                          }}
                          placeholder={t('collections.enterSummary')}
                          rows="2"
                          class="w-full px-0 py-0 text-sm resize-none border-0 focus:outline-none focus:ring-0"
                          style="background-color: transparent; color: var(--ds-text); caret-color: var(--ds-text);"
                        ></textarea>
                      </div>

                      <!-- Divider -->
                      <div class="border-t mx-3" style="border-color: {hasGradient ? 'var(--ds-glass-border)' : 'var(--ds-border)'};">
</div>

                      <!-- Actions Footer -->
                      <div class="p-3 pt-2 flex items-center justify-between flex-wrap gap-2">
                        <!-- Left side: Icon buttons + Create/Cancel -->
                        <div class="flex items-center gap-2 flex-shrink-0">
                          <!-- Workspace Selector - Icon Only Button -->
                          <div class="relative">
                            <button
                              type="button"
                              onclick={() => {
                                const dropdownId = `workspace-dropdown-${backboneItem.id}`;
                                const dropdown = document.getElementById(dropdownId);
                                if (dropdown) {
                                  dropdown.classList.toggle('hidden');
                                }
                              }}
                              class="w-7 h-7 rounded-md flex items-center justify-center border overflow-hidden transition-all hover:scale-105"
                              style="{selectedWorkspace?.avatar_url ? '' : `background-color: ${selectedWorkspace?.color || 'var(--ds-interactive)'};`} border-color: var(--ds-border);"
                              title={selectedWorkspace?.name || 'Select workspace'}
                            >
                              {#if selectedWorkspace?.avatar_url}
                                <img src={selectedWorkspace.avatar_url} alt="{selectedWorkspace.name} avatar" class="w-full h-full object-cover" />
                              {:else if selectedWorkspace?.icon}
                                <svelte:component this={iconMap[selectedWorkspace.icon] || Package} class="w-3 h-3 text-white" />
                              {:else}
                                <Package class="w-3 h-3 text-white" />
                              {/if}
                            </button>

                            <!-- Workspace Dropdown -->
                            <div
                              id="workspace-dropdown-{backboneItem.id}"
                              class="hidden absolute bottom-full left-0 mb-1 w-48 rounded border shadow-lg z-50"
                              style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
                            >
                              <div class="max-h-48 overflow-y-auto">
                                {#each workspaces as ws}
                                  <button
                                    type="button"
                                    onclick={() => {
                                      updateQuickAddField(backboneItem.id, 'workspaceId', ws.id);
                                      const dropdownId = `workspace-dropdown-${backboneItem.id}`;
                                      const dropdown = document.getElementById(dropdownId);
                                      if (dropdown) dropdown.classList.add('hidden');
                                    }}
                                    class="w-full flex items-center gap-2 px-3 py-2 text-sm transition-colors"
                                    style="color: var(--ds-text);"
                                    onmouseenter={(e) => e.currentTarget.style.background = 'var(--ds-surface-hovered)'}
                                    onmouseleave={(e) => e.currentTarget.style.background = ''}
                                  >
                                    {#if ws.avatar_url}
                                      <img src={ws.avatar_url} alt="{ws.name} avatar" class="w-5 h-5 rounded object-cover flex-shrink-0" />
                                    {:else}
                                      <div class="w-5 h-5 rounded flex items-center justify-center flex-shrink-0" style="background-color: {ws.color || 'var(--ds-interactive)'};">
                                        {#if ws.icon}
                                          <svelte:component this={iconMap[ws.icon] || Package} class="w-3 h-3 text-white" />
                                        {:else}
                                          <Package class="w-3 h-3 text-white" />
                                        {/if}
                                      </div>
                                    {/if}
                                    <span class="truncate">{ws.name}</span>
                                  </button>
                                {/each}
                              </div>
                            </div>
                          </div>

                          <!-- Item Type Selector - Icon Only Button -->
                          <div class="relative">
                            <button
                              type="button"
                              onclick={() => {
                                const dropdownId = `itemtype-dropdown-${backboneItem.id}`;
                                const dropdown = document.getElementById(dropdownId);
                                if (dropdown) {
                                  dropdown.classList.toggle('hidden');
                                }
                              }}
                              class="w-7 h-7 rounded-md flex items-center justify-center border transition-all hover:scale-105"
                              style="background-color: {selectedItemType?.color || '#6b7280'}; border-color: var(--ds-border);"
                              title={selectedItemType?.name || 'Select item type'}
                            >
                              {#if selectedItemType?.icon}
                                <svelte:component this={iconMap[selectedItemType.icon] || FileText} class="w-3 h-3 text-white" />
                              {:else}
                                <FileText class="w-3 h-3 text-white" />
                              {/if}
                            </button>

                            <!-- Item Type Dropdown -->
                            <div
                              id="itemtype-dropdown-{backboneItem.id}"
                              class="hidden absolute bottom-full left-0 mb-1 w-40 rounded border shadow-lg z-50"
                              style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
                            >
                              <div class="max-h-48 overflow-y-auto">
                                {#if state.availableTypes && state.availableTypes.length > 0}
                                  {#each state.availableTypes as type}
                                    <button
                                      type="button"
                                      onclick={() => {
                                        updateQuickAddField(backboneItem.id, 'itemTypeId', type.id);
                                        const dropdownId = `itemtype-dropdown-${backboneItem.id}`;
                                        const dropdown = document.getElementById(dropdownId);
                                        if (dropdown) dropdown.classList.add('hidden');
                                      }}
                                      class="w-full flex items-center gap-2 px-3 py-2 text-sm hover-bg transition-colors"
                                      style="color: var(--ds-text);"
                                    >
                                      <div class="w-5 h-5 rounded flex items-center justify-center flex-shrink-0" style="background-color: {type.color};">
                                        <svelte:component this={iconMap[type.icon] || FileText} class="w-3 h-3 text-white" />
                                      </div>
                                      <span class="truncate">{type.name}</span>
                                    </button>
                                  {/each}
                                {:else}
                                  <div class="px-3 py-2 text-sm" style="color: var(--ds-text-subtle);">{t('collections.noTypesAvailable')}</div>
                                {/if}
                              </div>
                            </div>
                          </div>

                          <!-- Action Buttons -->
                          <button
                            onclick={() => createChildItem(backboneItem.id)}
                            class="px-2.5 py-1.5 text-sm font-medium rounded transition-colors"
                            style="background-color: var(--ds-background-accent-blue); color: white;"
                            disabled={!state.workspaceId || !state.itemTypeId || !state.title?.trim()}
                          >
                            {t('collections.create')}
                          </button>
                          <button
                            onclick={() => cancelQuickAdd(backboneItem.id)}
                            class="px-2.5 py-1.5 text-sm font-medium rounded transition-colors hover:bg-gray-50"
                            style="color: var(--ds-text-subtle);"
                          >
                            {t('common.cancel')}
                          </button>
                        </div>

                        <!-- Right side: Error Message -->
                        {#if state.error}
                          <div class="text-xs text-red-600">
                            {state.error}
                          </div>
                        {/if}
                      </div>
                    </div>
                  {/if}
                </div>
              </div>
            </div>
          {/each}

          <!-- Empty State for when there are no backbone items -->
          {#if backboneItems.length === 0}
            <div class="flex items-center justify-center w-full min-h-[400px]">
              <EmptyState
                icon={MapPin}
                title={t('collections.noTopLevelItems')}
                description={t('collections.noTopLevelItemsDesc')}
              />
            </div>
          {/if}
        </div>
      </div>
    </div>

  </div>
{:else}
  <div class="p-6">
    <EmptyState
      icon={null}
      title={t('collections.workspaceNotFound')}
    />
  </div>
{/if}

<style>
  /* Enhanced drag and drop styles */
  [data-testid^="draggable-item"]:hover {
    transform: translateY(-1px);
  }

  [data-testid^="drop-zone"] {
    transition: border-color 0.2s ease, background-color 0.2s ease;
  }

  [data-testid^="drop-zone"]:hover {
    border-color: var(--ds-border-focused);
    background-color: var(--ds-background-selected);
  }
</style>

<!-- Item Detail Modal -->
{#if showItemModal && selectedItemId}
  <ItemDetail
    isModal={true}
    itemId={selectedItemId}
    {workspaceId}
    onclose={closeItemModal}
  />
{/if}

