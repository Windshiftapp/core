<script>
  import { onMount, untrack } from 'svelte';
  import { useEventListener } from 'runed';
  import { api } from '../../api.js';
  import { navigate } from '../../router.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { getCollection, checkItemVisibility } from '../collections/collectionService.js';
  import { collectionStore, reloadCollection } from '../../stores/collectionContext.js';
  import { workspaceDataStore, workspacesStore } from '../../stores/index.js';
  import { useGradientStyles, loadWorkspaceGradient } from '../../stores/workspaceGradient.svelte.js';
  import { Plus, ChevronDown, ChevronRight, Home, MapPin } from '@lucide/svelte';
  import ItemTypeIcon from '../../components/ItemTypeIcon.svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import { draggable, dropTargetForElements, monitorForElements } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import { autoScrollForElements } from '@atlaskit/pragmatic-drag-and-drop-auto-scroll/element';
  import Tooltip from '../../components/Tooltip.svelte';
  import ViewHeader from '../../layout/ViewHeader.svelte';
  import StaticViewBackground from '../../layout/StaticViewBackground.svelte';
  import SubFilterBar from './SubFilterBar.svelte';
  import ItemDetail from '../items/ItemDetail.svelte';
  import { infoToast, errorToast } from '../../stores/toasts.svelte.js';
  import ItemKey from '../items/ItemKey.svelte';
  import ItemCard from '../items/ItemCard.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import { getStatusCategory } from '../../utils/statusColors.js';
  import CollectionViewSwitcher from './CollectionViewSwitcher.svelte';
  import QuickAddForm from './QuickAddForm.svelte';
  import { childItemTypesForParent } from '../../utils/hierarchy.js';
  import { indexCollectionHierarchy } from './collectionHierarchy.js';

  let { workspaceId, collectionId = null } = $props();

  let workspace = $derived(workspaceDataStore.workspace);
  let loading = $state(true);
  let backboneItems = $state([]); // Current backbone items (horizontal)
  let childItemsByParent = $state({}); // Child items grouped by parent ID
  let itemTypes = $derived(workspaceDataStore.itemTypes);
  let statuses = $derived(workspaceDataStore.statuses);
  let statusCategories = $derived(workspaceDataStore.statusCategories);
  let workspaces = $derived($workspacesStore.regularWorkspaces || []);
  let currentParentId = $state(null); // null = root level, otherwise parent ID for current backbone
  let hierarchyBreadcrumbs = $state([]); // Navigation breadcrumbs for hierarchy levels

  let currentCollectionName = $state('Default');

  // Quick-add state per parent
  let quickAddState = $state({}); // { [parentId]: { show: boolean, workspace: null, itemType: null, title: '', error: null } }

  // Inline editing state
  let editingItemId = $state(null);
  let editingTitle = $state('');

  // Item detail modal state
  let selectedItemId = $state(null);
  let showItemModal = $state(false);
  let mapScrollElement = $state(null);


  // Centralized gradient styling
  const styles = useGradientStyles();

  // Listen for browser back/forward navigation
  function handlePopState() {
    loadStoryMapDataFromURL();
  }

  useEventListener(() => window, 'popstate', handlePopState);

  $effect(() => {
    if (!mapScrollElement) return;
    return autoScrollForElements({
      element: mapScrollElement,
      getAllowedAxis: () => 'horizontal',
    });
  });

  onMount(async () => {
    if (workspaceId) {
      await loadWorkspaceGradient(workspaceId);
    }
    await loadAllData();
  });

  // Sync items from central store
  $effect(() => {
    if (!collectionStore.loading) {
      currentCollectionName = collectionStore.collectionName;
      const currentItems = collectionStore.items;
      untrack(() => processMapItems(currentItems));
    }
  });

  async function loadAllData() {
    loading = true;
    await Promise.all([
      workspaceId
        ? workspaceDataStore.initialize(workspaceId)
        : workspaceDataStore.initializeGlobal(),
      workspacesStore.load(),
    ]);
    loadStoryMapDataFromURL();
    loading = false;
  }

  function loadStoryMapDataFromURL() {
    // Get parent ID from URL parameters
    const urlParams = new URLSearchParams(window.location.search);
    const parentParam = urlParams.get('parent');
    const parentId = parentParam ? parseInt(parentParam) : null;
    
    return loadStoryMapData(parentId);
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
      try {
        // Build the path from root down to the current parent. /items/{id}/ancestors
        // returns the chain root→direct-parent (excluding the item itself) in one
        // request; append the current parent to complete the path. Two requests
        // total instead of one GET /items/{id} per ancestor level.
        const [ancestors, currentItem] = await Promise.all([
          api.items.getAncestors(currentParentId),
          api.items.get(currentParentId),
        ]);
        const pathItems = [...(ancestors || []), currentItem];

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
        if (error?.name === 'AbortError') {
          return;
        }
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

  function processMapItems(items) {
    const parentId = currentParentId;

    // Compute into local variables to avoid read-after-write on $state
    const hierarchyIndex = indexCollectionHierarchy(items);
    const newBackbone = parentId === null
      ? [...hierarchyIndex.roots].sort((a, b) => a.id - b.id)
      : [...(hierarchyIndex.childrenByParent.get(parentId) || [])].sort((a, b) => a.id - b.id);

    // Group child items by their parent ID (children of current backbone items)
    const newChildren = {};
    for (const backbone of newBackbone) {
      const children = hierarchyIndex.childrenByParent.get(backbone.id);
      if (children?.length) newChildren[backbone.id] = [...children];
    }

    // Sort child items within each parent group
    Object.keys(newChildren).forEach(pid => {
      newChildren[pid].sort((a, b) => a.id - b.id);
    });

    // Batch-assign to $state at the end
    backboneItems = newBackbone;
    childItemsByParent = newChildren;

    // Update breadcrumbs
    updateHierarchyBreadcrumbs();
  }

  function loadStoryMapData(parentId = null) {
    currentParentId = parentId;
    processMapItems(collectionStore.items);
  }

  function updateURL(parentId) {
    const url = new URL(window.location.href);
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

  $effect(() => {
    return monitorForElements({
      onDrop({ source, location }) {
        const draggedItemId = parseInt(String(source.data.itemId));
        const targetParentId = location.current.dropTargets.length > 0
          ? parseInt(String(location.current.dropTargets[0].data.parentId))
          : null;
        if (targetParentId && draggedItemId) {
          moveItemToParent(draggedItemId, targetParentId);
        }
      }
    });
  });

  function registerMapItem(element, itemId) {
    const cleanup = draggable({
      element,
      getInitialData: () => ({ itemId })
    });
    return { destroy: cleanup };
  }

  function registerMapDropZone(element, parentId) {
    const reset = () => {
      element.style.borderColor = 'var(--ctx-border, var(--ds-border))';
      element.style.boxShadow = '';
    };
    const cleanup = dropTargetForElements({
      element,
      getData: () => ({ parentId }),
      onDragEnter: () => {
        element.style.borderColor = 'var(--ds-border-focused)';
        element.style.boxShadow = 'inset 0 0 0 2px var(--ds-border-focused)';
      },
      onDragLeave: reset,
      onDrop: reset,
    });
    return {
      destroy() {
        reset();
        cleanup();
      }
    };
  }

  async function moveItemToParent(itemId, newParentId) {
    try {

      // Update the item's parent_id
      const result = await api.items.update(itemId, { parent_id: newParentId });

      // Reload data from central store
      reloadCollection();
    } catch (error) {
      console.error('Failed to move item:', error);

      // Show user-friendly error toast
      errorToast(error.message || 'Failed to move item due to hierarchy constraints', 'Cannot move item');
    }
  }

  function getItemTypeInfo(itemTypeId) {
    return itemTypes.find(type => type.id === itemTypeId);
  }

  function navigateToItem(item) {
    const wsId = workspaceId || item.workspace_id;
    const url = collectionId && workspaceId
      ? `/workspaces/${workspaceId}/collections/${collectionId}/items/${item.id}`
      : `/workspaces/${wsId}/items/${item.id}`;
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

    const childTypes = childItemTypesForParent(itemTypes, parentType);

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
        availableTypes = childItemTypesForParent(itemTypes, parentType)
          .sort((a, b) => a.sort_order - b.sort_order);
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

    quickAddState[parentId] = {
      show: true,
      workspaceId: preselectedWorkspaceId,
      itemTypeId: availableTypes.length > 0 ? availableTypes[0].id : null,
      availableTypes: availableTypes,
      title: '',
      error: null
    };

    // Focus the textarea after it's rendered
    setTimeout(() => {
      const textarea = /** @type {HTMLTextAreaElement | null} */ (document.querySelector(`textarea[data-quick-add-parent="${parentId}"]`));
      if (textarea) {
        textarea.focus();
      }
    }, 0);
  }

  function cancelQuickAdd(parentId) {
    delete quickAddState[parentId];
  }

  async function createChildItem(parentId) {
    const state = quickAddState[parentId];
    if (!state) return;

    // Validate
    if (!state.workspaceId) {
      quickAddState[parentId].error = 'Please select a workspace';
      return;
    }
    if (!state.itemTypeId) {
      quickAddState[parentId].error = 'Please select an item type';
      return;
    }
    if (!state.title?.trim()) {
      quickAddState[parentId].error = 'Please enter a title';
      return;
    }

    try {
      const newItem = await api.items.create({
        workspace_id: state.workspaceId,
        item_type_id: state.itemTypeId,
        title: state.title.trim(),
        description: '',
        parent_id: parentId
      });

      // Check if the created item will be visible in the current collection view
      const filters = { workspace_id: workspaceId };

      if (collectionId) {
        const collection = await getCollection(collectionId);
        if (collection?.ql_query) {
          filters.ql = collection.ql_query;
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
      reloadCollection();
    } catch (error) {
      console.error('Failed to create child item:', error);
      quickAddState[parentId].error = 'Failed to create item: ' + (error.message || error);
    }
  }

  function updateQuickAddField(parentId, field, value) {
    if (quickAddState[parentId]) {
      quickAddState[parentId][field] = value;
      quickAddState[parentId].error = null;
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
      const textarea = /** @type {HTMLTextAreaElement | null} */ (document.querySelector(`textarea[data-item-id="${item.id}"]`));
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
    if (event?.hasChanges) {
      reloadCollection();
    }
  }
</script>

{#if loading}
  <div class="p-6">
    <div class="animate-pulse">{t('collections.loadingStoryMap')}</div>
  </div>
{:else if workspace || !workspaceId}
  <StaticViewBackground
    backgroundStyle={styles.backgroundStyle}
    contextVars={styles.contextVars}
    contentClass=""
    rootStyle="width: 100%; min-width: 0; max-width: 100%;"
    testid="map-view"
  >
    <!-- Header -->
    <div class="p-6 border-b" style="border-color: var(--ctx-border, var(--ds-border));">

      <ViewHeader
        workspaceName={workspace?.name || ''}
        collection={currentCollectionName}
        viewName="Map"
        itemCount={collectionStore.itemsPagination?.total_items ?? (backboneItems.length + Object.values(childItemsByParent).flat().length)}
      />

      <!-- Controls Bar -->
      <div class="flex items-center mt-4">
        <SubFilterBar {workspaceId} />
      </div>

      <!-- Hierarchy Breadcrumbs -->
      {#if hierarchyBreadcrumbs.length > 0}
        <div class="mt-4">
          <!-- Breadcrumb Navigation -->
          <div class="flex items-center gap-1 flex-wrap">
            {#each hierarchyBreadcrumbs as breadcrumb, index (breadcrumb.id)}
              <!-- Separator -->
              {#if index > 0}
                <ChevronRight class="w-4 h-4" style="color: var(--ctx-text-subtle, var(--ds-text-subtle));" />
              {/if}

              <!-- Breadcrumb Button -->
              <button
                onclick={() => navigateToLevel(breadcrumb.id)}
                class="flex items-center gap-1.5 px-2.5 py-1.5 rounded-md transition-all {breadcrumb.isCurrent ? 'font-medium' : ''}"
                style="{breadcrumb.isCurrent
                  ? 'color: var(--ctx-text-interactive, var(--ds-interactive)); background-color: var(--ctx-surface-info, var(--ds-surface-information)); border: 1px solid var(--ctx-border-focused, var(--ds-border-focused)); backdrop-filter: var(--ctx-backdrop, none);'
                  : 'color: var(--ctx-text-subtle, var(--ds-text)); background-color: transparent; border: 1px solid transparent;'}"
              >
                <!-- Icon -->
                {#if breadcrumb.level === 'root'}
                  <Home class="w-3.5 h-3.5" />
                {:else if breadcrumb.itemType}
                  <ItemTypeIcon itemType={breadcrumb.itemType} />
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
              <div class="mt-3 flex items-center gap-3 text-xs" style="color: var(--ctx-text-subtle, var(--ds-text-subtle));">
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
    <div
      bind:this={mapScrollElement}
      class="p-6 overflow-x-auto"
      data-testid="map-scroll-container"
    >
      <div class="min-w-max">
        <!-- Backbone (Horizontal) -->
        <div
          class="grid gap-x-6 gap-y-10 mb-8"
          style="grid-template-columns: repeat({backboneItems.length}, 16rem); grid-template-rows: auto 1fr;"
        >
          {#each backboneItems as backboneItem}
            {@const itemType = getItemTypeInfo(backboneItem.item_type_id)}
            <div class="row-span-2 grid grid-rows-subgrid">
              <!-- Backbone Item -->
              <div class="self-start">
                <ItemCard compact>
                  <!-- Title -->
                  <button
                    data-testid="map-backbone-item-{backboneItem.id}"
                    onclick={() => navigateToItem(backboneItem)}
                    class="text-sm mb-2 leading-snug text-left w-full line-clamp-2 transition-colors"
                    style="{styles.glassTextStyle}"
                  >
                    {backboneItem.title}
                  </button>

                  <!-- Bottom row: Key, Icon, Status, Drill Down -->
                  <div class="flex items-center justify-between">
                    <div class="flex items-center gap-2">
                      {#if itemType}
                        <ItemTypeIcon {itemType} />
                      {/if}
                      <ItemKey item={backboneItem} {workspace}
                        onClick={(e) => handleKeyClick(backboneItem, e)}
                        style={styles.glassSubtleTextStyle}
                      />
                    </div>

                    <div class="flex items-center gap-1.5">
                      <Tooltip class="flex items-center" content={(backboneItem.status_name || backboneItem.status)?.replace('_', ' ') || 'Status'}>
                        {#snippet children()}
                          <Lozenge
                            square
                            customBg={getStatusCategory(backboneItem.status_name || backboneItem.status, statuses, statusCategories)?.color || 'var(--ds-text-subtle)'}
                          />
                        {/snippet}
                      </Tooltip>
                      <!-- Drill Down Arrow (only show if item has children) -->
                      {#if childItemsByParent[backboneItem.id]?.length > 0}
                        <Tooltip content={t('collections.drillDown')}>
                          {#snippet children()}
                            <button
                              data-testid="map-drill-down-{backboneItem.id}"
                              onclick={() => drillDown(backboneItem.id)}
                              class="p-1.5 rounded-full transition-colors group"
                              style="color: var(--ds-interactive);"
                              onmouseenter={(e) => e.currentTarget.style.background = 'var(--ds-background-neutral-hovered)'}
                              onmouseleave={(e) => e.currentTarget.style.background = ''}
                            >
                              <ChevronDown class="w-3.5 h-3.5 group-hover:scale-110 transition-transform" />
                            </button>
                          {/snippet}
                        </Tooltip>
                      {/if}
                    </div>
                  </div>
                </ItemCard>
              </div>

              <!-- Drop Zone for this parent -->
              <div
                use:registerMapDropZone={backboneItem.id}
                class="min-h-96 p-3 rounded border-2 border-dashed transition-all"
                style="border-color: var(--ctx-border, var(--ds-border)); background-color: var(--ctx-surface-overlay, var(--ds-surface-overlay)); backdrop-filter: var(--ctx-backdrop, none);"
                data-parent-id={backboneItem.id}
                data-testid="drop-zone-{backboneItem.id}"
              >
                <h3 class="text-sm font-medium mb-3 text-center" style={styles.glassTextStyle}>
                  {t('collections.childWorkItems', { count: childItemsByParent[backboneItem.id]?.length || 0 })}
                </h3>

                <!-- Child Items Column -->
                <div class="space-y-2">
                  {#each childItemsByParent[backboneItem.id] || [] as childItem}
                    {@const childItemType = getItemTypeInfo(childItem.item_type_id)}
                    <!-- svelte-ignore a11y_no_static_element_interactions -->
                    <div
                      use:registerMapItem={childItem.id}
                      class="item-card rounded border p-3 cursor-move"
                      style="box-shadow: var(--ds-shadow-raised); {styles.cardStyle(4)}"
                      data-item-id={childItem.id}
                      data-testid="draggable-item-{childItem.id}"
                      ondblclick={(e) => startEditingItem(childItem, e)}
                    >
                      <!-- Title -->
                      {#if editingItemId === childItem.id}
                        <Textarea
                          bind:value={editingTitle}
                          data-item-id={childItem.id}
                          class="text-sm mb-2 leading-snug w-full resize-none overflow-hidden bg-transparent border-none outline-none p-0 m-0"
                          style="color: var(--ds-text); caret-color: var(--ds-text);"
                          rows={2}
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
                        <h4 class="text-sm mb-2 leading-snug line-clamp-2" style="{styles.glassTextStyle}">
                          {childItem.title}
                        </h4>
                      {/if}

                      <!-- Bottom row: Key, Icon, Status -->
                      <div class="flex items-center justify-between">
                        <div class="flex items-center gap-2">
                          {#if childItemType}
                            <ItemTypeIcon itemType={childItemType} />
                          {/if}
                          <ItemKey item={childItem} {workspace}
                            onClick={(e) => handleKeyClick(childItem, e)}
                            style={styles.glassSubtleTextStyle}
                          />
                        </div>
                        <Tooltip class="flex items-center" content={(childItem.status_name || childItem.status)?.replace('_', ' ') || 'Status'}>
                          {#snippet children()}
                            <Lozenge
                              square
                              customBg={getStatusCategory(childItem.status_name || childItem.status, statuses, statusCategories)?.color || 'var(--ds-text-subtle)'}
                            />
                          {/snippet}
                        </Tooltip>
                      </div>
                    </div>
                  {/each}

                  <!-- Add Card button when there are existing items -->
                  {#if !quickAddState[backboneItem.id]?.show && childItemsByParent[backboneItem.id]?.length > 0 && canAddChildren(backboneItem.id)}
                    <button
                      data-testid="map-add-card-{backboneItem.id}"
                      onclick={() => initQuickAdd(backboneItem.id)}
                      class="w-full flex items-center gap-2 px-3 py-2 text-sm font-medium rounded border-2 border-dashed transition-colors "
                      style="border-color: var(--ctx-border, var(--ds-border)); background-color: transparent; color: var(--ds-text-subtle);"
                      onmouseenter={(e) => {
                        e.currentTarget.style.borderColor = 'var(--ds-border-focused)';
                        e.currentTarget.style.color = 'var(--ds-interactive)';
                      }}
                      onmouseleave={(e) => {
                        e.currentTarget.style.borderColor = 'var(--ctx-border, var(--ds-border))';
                        e.currentTarget.style.color = 'var(--ds-text-subtle)';
                      }}
                    >
                      <Plus class="w-4 h-4" />
                      {t('collections.addCard')}
                    </button>
                  {/if}

                  <!-- Quick Add / Empty State -->
                  {#if !quickAddState[backboneItem.id]?.show && (!childItemsByParent[backboneItem.id] || childItemsByParent[backboneItem.id].length === 0)}
                    {#if canAddChildren(backboneItem.id)}
                      <button
                        data-testid="map-add-card-{backboneItem.id}"
                        onclick={() => initQuickAdd(backboneItem.id)}
                        class="w-full flex items-center gap-2 px-3 py-2 text-sm font-medium rounded border-2 border-dashed transition-colors"
                        style="border-color: var(--ctx-border, var(--ds-border)); background-color: transparent; color: var(--ds-text-subtle);"
                        onmouseenter={(e) => {
                          e.currentTarget.style.borderColor = 'var(--ds-border-focused)';
                          e.currentTarget.style.color = 'var(--ds-interactive)';
                        }}
                        onmouseleave={(e) => {
                          e.currentTarget.style.borderColor = 'var(--ctx-border, var(--ds-border))';
                          e.currentTarget.style.color = 'var(--ds-text-subtle)';
                        }}
                      >
                        <Plus class="w-4 h-4" />
                        {t('collections.addCard')}
                      </button>
                    {/if}
                  {/if}

                  <!-- Quick Add Form -->
                  {#if quickAddState[backboneItem.id]?.show}
                    <QuickAddForm
                      parentId={backboneItem.id}
                      formState={quickAddState[backboneItem.id]}
                      {workspaces}
                      compact={true}
                      cardBgStyle={styles.cardStyle(8)}
                      onUpdateField={updateQuickAddField}
                      onCreate={createChildItem}
                      onCancel={cancelQuickAdd}
                    />
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

  </StaticViewBackground>
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
  [data-testid^="draggable-item"] {
    transition: background-color 140ms ease-in-out;
  }

  [data-testid^="draggable-item"]:hover {
    background-color: var(--ds-surface-raised-hovered) !important;
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
