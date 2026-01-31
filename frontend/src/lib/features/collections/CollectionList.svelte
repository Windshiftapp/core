<script>
  import { onMount, onDestroy } from 'svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { api } from '../../api.js';
  import { navigate } from '../../router.js';
  import { getCollection } from '../collections/collectionService.js';
  import { getStatusCategory as getStatusCategoryUtil, getStatusColor as getStatusColorUtil, getStatusInlineStyle, getTextColorForBackground } from '../../utils/statusColors.js';
  import { useGradientStyles, loadWorkspaceGradient } from '../../stores/workspaceGradient.svelte.js';
  import { Plus, Filter, MoreHorizontal, Calendar, User, AlertCircle, Trash2, Eye } from 'lucide-svelte';
  import { itemTypeIconMap } from '../../utils/icons.js';
  import SearchInput from '../../components/SearchInput.svelte';
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
  import Pagination from '../../components/Pagination.svelte';
  import ViewHeader from '../../layout/ViewHeader.svelte';
  import InlineFieldEditor from '../../editors/InlineFieldEditor.svelte';
  import ItemPicker from '../../pickers/ItemPicker.svelte';
  import ItemKey from '../items/ItemKey.svelte';
  import ColorDot from '../../components/ColorDot.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import Checkbox from '../../components/Checkbox.svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import { formatDate } from '../../utils/dateFormatter.js';

  let { workspaceId, collectionId = null } = $props();

  let workspace = $state(null);
  let workItems = $state([]);
  let allItems = $state([]); // Store all items for search filtering
  let itemTypes = $state([]);
  let itemsPagination = $state(null);
  let statuses = $state([]);
  let statusCategories = $state([]);
  let users = $state([]);
  let milestones = $state([]);
  let priorities = $state([]);

  let loading = $state(true);
  let loadingItems = $state(false);
  let currentCollectionName = $state('Default');
  let currentView = $state('list');
  let searchQuery = $state('');
  let currentPage = $state(1);
  let itemsPerPage = $state(50);

  // Status transition caching for lazy loading
  let itemTransitions = $state(new Map()); // Cache transitions per item ID
  let loadingTransitions = $state(new Set()); // Track which items are currently loading transitions
  let requestQueue = $state(new Set()); // Queue for pending requests
  const MAX_CONCURRENT_REQUESTS = 3; // Limit concurrent API calls
  let activeRequests = $state(0);

  // Centralized gradient styling
  const styles = useGradientStyles();

  onMount(async () => {
    if (workspaceId) {
      await loadWorkspaceGradient(workspaceId);
      await loadWorkspace();
      await loadWorkItems();
    }
    loading = false;
    
    // Listen for refresh events
    window.addEventListener('focus', handleWindowFocus);
    window.addEventListener('refresh-work-items', loadWorkItems);
  });

  onDestroy(() => {
    window.removeEventListener('focus', handleWindowFocus);
    window.removeEventListener('refresh-work-items', loadWorkItems);
  });

  // Refresh when window gains focus (user returns from another tab/window)
  function handleWindowFocus() {
    if (!loadingItems) {
      loadWorkItems();
    }
  }

  async function loadWorkspace() {
    try {
      const [workspaceData, itemTypesData, statusesData, statusCategoriesData, usersData, milestonesData, prioritiesData] = await Promise.all([
        api.workspaces.get(workspaceId),
        api.itemTypes.getAll(),
        api.workspaces.getStatuses(workspaceId),
        api.statusCategories.getAll(),
        api.getUsers(),
        api.milestones.getAll(),
        api.priorities.getAll()
      ]);
      workspace = workspaceData;
      itemTypes = itemTypesData || [];
      statuses = statusesData || [];
      statusCategories = statusCategoriesData || [];
      users = usersData || [];
      milestones = milestonesData || [];
      priorities = prioritiesData || [];
    } catch (error) {
      console.error('Failed to load workspace:', error);
    }
  }

  async function loadWorkItems(page = 1, limit = itemsPerPage) {
    try {
      loadingItems = true;
      
      // Build base filters
      const filters = { 
        workspace_id: workspaceId,
        page: page,
        limit: limit
      };
      
      // Apply collection filter if specified
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
      
      if (searchQuery.trim()) {
        // When searching, load ALL items and filter locally
        await loadAllItemsForSearch();
        currentPage = page;
        itemsPerPage = limit;
        updatePaginatedResults();
      } else {
        // Normal paginated loading
        const response = await api.items.getAll(filters);
        
        if (response && response.items) {
          // Handle paginated response
          workItems = response.items;
          itemsPagination = response.pagination;
          currentPage = page;
          itemsPerPage = limit;
        } else {
          // Handle legacy response (backward compatibility)
          workItems = response || [];
          itemsPagination = null;
        }
      }
    } catch (error) {
      console.error('Failed to load work items:', error);
      workItems = [];
      itemsPagination = null;
    } finally {
      loadingItems = false;
    }
  }

  async function loadAllItemsForSearch() {
    const filters = { 
      workspace_id: workspaceId,
      page: 1,
      limit: 1000 // Load a large number to get all items
    };
    
    // Apply collection filter if specified
    if (collectionId) {
      const collection = await getCollection(collectionId);
      if (collection?.cql_query) {
        filters.vql = collection.cql_query;
      }
    }
    
    const response = await api.items.getAll(filters);
    if (response && response.items) {
      allItems = response.items;
    } else {
      allItems = response || [];
    }
  }

  function updatePaginatedResults() {
    // Filter the items based on search query
    const filteredItems = allItems.filter(item => {
      const query = searchQuery.toLowerCase();
      return item.title.toLowerCase().includes(query) || 
             (item.description && item.description.toLowerCase().includes(query));
    });

    // Calculate pagination for filtered results
    const totalFiltered = filteredItems.length;
    const startIndex = (currentPage - 1) * itemsPerPage;
    const endIndex = startIndex + itemsPerPage;
    
    workItems = filteredItems.slice(startIndex, endIndex);
    
    // Create custom pagination object for filtered results
    itemsPagination = {
      page: currentPage,
      limit: itemsPerPage,
      total: totalFiltered,
      totalPages: Math.ceil(totalFiltered / itemsPerPage)
    };
  }

  // Handle pagination events
  async function handlePageChange(event) {
    await loadWorkItems(event.detail.page, event.detail.itemsPerPage);
  }
  
  async function handlePageSizeChange(event) {
    await loadWorkItems(event.detail.page, event.detail.itemsPerPage);
  }
  

  // For display purposes, we now use workItems directly (no additional client-side filtering needed)
  let filteredItems = $derived(workItems);

  // Reload items when search query changes
  let lastSearchQuery = searchQuery;
  $effect(() => {
    if (searchQuery !== lastSearchQuery) {
      lastSearchQuery = searchQuery;
      currentPage = 1; // Reset to first page when search changes
      loadWorkItems(1, itemsPerPage);
    }
  });

  function viewItem(item) {
    const url = collectionId 
      ? `/workspaces/${workspaceId}/collections/${collectionId}/items/${item.id}`
      : `/workspaces/${workspaceId}/items/${item.id}`;
    navigate(url);
  }

  async function deleteItem(item) {
    if (!confirm(t('collections.confirmDeleteItem', { title: item.title }))) {
      return;
    }

    try {
      await api.items.delete(item.id);
      // Refresh the work items list
      await loadWorkItems();
    } catch (error) {
      console.error('Failed to delete item:', error);
      alert(t('dialogs.alerts.failedToDelete', { error: error.message || error }));
    }
  }

  async function toggleTaskStatus(item, isCompleted) {
    const newStatus = isCompleted ? 'completed' : 'open';

    try {
      await api.items.update(item.id, { status: newStatus });
      // Update local state
      item.status = newStatus;
      // Trigger reactivity
      workItems = [...workItems];
    } catch (error) {
      console.error('Failed to update task status:', error);
      alert(t('dialogs.alerts.failedToUpdate', { error: error.message || error }));
    }
  }

  async function handleStatusChange(item, newStatus) {
    if (newStatus === item.status) return;

    try {
      await api.items.update(item.id, { status: newStatus });

      // Update the item in the local workItems array
      const index = workItems.findIndex(workItem => workItem.id === item.id);
      if (index !== -1) {
        workItems[index].status = newStatus;
        workItems = [...workItems]; // Trigger reactivity
      }

      // Also update in allItems if we're in search mode
      if (searchQuery.trim() && allItems.length > 0) {
        const allIndex = allItems.findIndex(workItem => workItem.id === item.id);
        if (allIndex !== -1) {
          allItems[allIndex].status = newStatus;
          allItems = [...allItems]; // Trigger reactivity
        }
      }
    } catch (error) {
      console.error('Failed to update status:', error);
      alert(t('dialogs.alerts.failedToUpdate', { error: error.message || error }));
    }
  }

  function buildItemActions(item) {
    return [
      {
        id: 'view',
        type: 'regular',
        icon: Eye,
        title: t('items.viewItem'),
        onClick: () => viewItem(item)
      },
      { type: 'divider' },
      {
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: t('common.delete'),
        color: '#dc2626',
        hoverClass: 'hover:bg-red-50 hover:text-red-700',
        onClick: () => deleteItem(item)
      }
    ];
  }

  // Handle inline editing events
  function handleItemUpdated(event) {
    const { item: updatedItem, field, value } = event.detail;
    
    // Update the item in the local workItems array
    const index = workItems.findIndex(item => item.id === updatedItem.id);
      if (index !== -1) {
        workItems[index] = {
          ...updatedItem,
          item_type_id: updatedItem.item_type_id ?? workItems[index].item_type_id,
          item_type: itemTypes.find(type => type.id === (updatedItem.item_type_id ?? workItems[index].item_type_id))
        };
        workItems = [...workItems]; // Trigger reactivity
      }
    
    // Also update in allItems if we're in search mode
    if (searchQuery.trim() && allItems.length > 0) {
      const allIndex = allItems.findIndex(item => item.id === updatedItem.id);
      if (allIndex !== -1) {
        allItems[allIndex] = updatedItem;
        allItems = [...allItems]; // Trigger reactivity
      }
    }
  }
  
  function handleUpdateError(event) {
    const { error, field, value } = event.detail;
    console.error(`Failed to update ${field}:`, error);
    // You could show a toast notification here
    alert(t('dialogs.alerts.failedToUpdate', { error: `${field}: ${error}` }));
  }

  // Throttled status transition loader
  async function loadStatusTransitions(itemId) {
    // Return cached result if available
    if (itemTransitions.has(itemId)) {
      return itemTransitions.get(itemId);
    }
    
    // Don't load if already loading
    if (loadingTransitions.has(itemId)) {
      return null;
    }
    
    // If too many requests are active, queue this one
    if (activeRequests >= MAX_CONCURRENT_REQUESTS) {
      requestQueue.add(itemId);
      return null;
    }
    
    return await executeStatusTransitionRequest(itemId);
  }
  
  // Execute the actual API request
  async function executeStatusTransitionRequest(itemId) {
    try {
      activeRequests++;
      loadingTransitions.add(itemId);
      
      const result = await api.items.getAvailableStatusTransitions(itemId);
      
      // Cache the result
      itemTransitions.set(itemId, result.available_transitions || []);
      return result.available_transitions || [];
    } catch (error) {
      console.error('Failed to load status transitions:', error);
      // Cache empty result to prevent repeated failures
      itemTransitions.set(itemId, []);
      return [];
    } finally {
      activeRequests--;
      loadingTransitions.delete(itemId);
      
      // Trigger reactivity update
      itemTransitions = itemTransitions;
      
      // Process next item in queue
      processQueue();
    }
  }
  
  // Process queued requests
  function processQueue() {
    if (requestQueue.size > 0 && activeRequests < MAX_CONCURRENT_REQUESTS) {
      const nextItemId = requestQueue.values().next().value;
      requestQueue.delete(nextItemId);
      executeStatusTransitionRequest(nextItemId);
    }
  }

  // Reload data when workspaceId or collectionId changes
  let lastWorkspaceId = workspaceId;
  let lastCollectionId = collectionId;
  $effect(() => {
    if (workspaceId && !loading) {
      if (workspaceId !== lastWorkspaceId || collectionId !== lastCollectionId) {
        lastWorkspaceId = workspaceId;
        lastCollectionId = collectionId;
        loadWorkItems();
      }
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
      <div class="mb-6">
        <ViewHeader
          workspaceName={workspace.name}
          collection={currentCollectionName}
          viewName="List"
          itemCount={itemsPagination?.total || workItems.length}
          hasGradient={styles.hasCustomBackground}
          textStyle={styles.textStyle}
          subtleTextStyle={styles.subtleTextStyle}
        />
      </div>

      <!-- Controls Bar -->
      <div class="flex items-center justify-between mb-6">
        <div class="flex items-center gap-4">
          <!-- Search -->
          <SearchInput
            bind:value={searchQuery}
            placeholder={t('common.search')}
            hasGradient={styles.hasCustomBackground}
          />

        </div>

      </div>

      <!-- Work Items Table -->
      {#if loadingItems}
        <div class="p-8 text-center">
          <div class="animate-pulse" style="{styles.subtleTextStyle}">{t('common.loading')}</div>
        </div>
      {:else if filteredItems.length === 0}
        {#if workItems.length === 0}
          <EmptyState
            title={t('items.noItems')}
            description={t('items.createToStart')}
            hasGradient={styles.hasCustomBackground}
          />
        {:else}
          <EmptyState
            title={t('items.noItemsInFilter')}
            description={t('items.noItemsInFilter')}
            hasGradient={styles.hasCustomBackground}
          />
        {/if}
      {:else}
        <div class="rounded-xl border shadow-sm overflow-hidden" style="{styles.tableStyle(12)} {styles.hasGradient ? 'border-color: rgba(0, 0, 0, 0.1);' : 'border-color: var(--ds-border);'}">
          <!-- Table Header -->
          <div class="px-4 py-3 border-b" style="{styles.tableHeaderStyle} {styles.hasGradient ? 'border-color: rgba(0, 0, 0, 0.1);' : 'border-color: var(--ds-border);'}">
            <div class="grid grid-cols-12 gap-4 text-xs font-semibold uppercase tracking-wider" style="{styles.glassSubtleTextStyle}">
              <div class="col-span-6">{t('common.title')}</div>
              <div class="col-span-2">{t('common.status')}</div>
              <div class="col-span-2">{t('common.priority')}</div>
              <div class="col-span-1">{t('common.created')}</div>
              <div class="col-span-1">{t('common.actions')}</div>
            </div>
          </div>

          <!-- Table Body -->
          <div>
            {#each filteredItems as item}
              <div class="px-4 py-3 list-row transition-colors" style="border-top: 1px solid var(--ds-border);">
                <div class="grid grid-cols-12 gap-4 items-center">
                  <!-- Title -->
                  <div class="col-span-6">
                    <div class="flex items-center gap-2 min-w-0">
                      <!-- Issue Key -->
                      <ItemKey
                        {item}
                        {workspace}
                        href={collectionId
                          ? `/workspaces/${workspaceId}/collections/${collectionId}/items/${item.id}`
                          : `/workspaces/${workspaceId}/items/${item.id}`}
                        className="text-xs font-mono px-1.5 py-0.5 rounded whitespace-nowrap flex-shrink-0 transition-colors cursor-pointer item-key"
                        style="background-color: var(--ds-interactive-subtle); color: var(--ds-text-subtle);"
                      />
                      <!-- Item Type Icon -->
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
                      <!-- Task Icon (fallback for task items without type) -->
                      {#if item.is_task && (!item.item_type_id || !itemTypes.find(type => type.id === item.item_type_id))}
                        <CheckSquare class="w-4 h-4 text-blue-500 flex-shrink-0" />
                      {/if}
                      <!-- Inline Title Editor -->
                      <div class="flex-1 min-w-0">
                        <InlineFieldEditor
                          {item}
                          field="title"
                          fieldType="text"
                          placeholder="Enter title..."
                          required={true}
                          className="font-medium"
                          on:item-updated={handleItemUpdated}
                          on:update-error={handleUpdateError}
                        />
                      </div>
                    </div>
                  </div>

                  <!-- Status / Task Checkbox -->
                  <div class="col-span-2">
                    {#if item.is_task}
                      <!-- Task checkbox -->
                      <Checkbox
                        checked={item.status === 'completed'}
                        onchange={(checked) => toggleTaskStatus(item, checked)}
                        label={item.status === 'completed' ? 'Done' : 'Todo'}
                        size="small"
                      />
                    {:else}
                      <!-- Status Picker -->
                      {@const selectedStatus = statuses.find(s => s.id === item.status_id)}
                      {@const statusCategory = selectedStatus ? statusCategories.find(sc => sc.id === selectedStatus.category_id) : null}
                      <ItemPicker
                        value={item.status_id}
                        items={statuses}
                        config={{
                          icon: {
                            type: 'color-dot',
                            source: (status) => {
                              const category = statusCategories.find(sc => sc.id === status.category_id);
                              return category?.color || '#6b7280';
                            },
                            size: 'w-2 h-2'
                          },
                          primary: { text: (status) => status.name },
                          getValue: (status) => status.id,
                          getLabel: (status) => status.name,
                          searchFields: ['name']
                        }}
                        placeholder="Set status"
                        showUnassigned={false}
                        allowClear={false}
                        on:select={async (e) => {
                          const statusId = e.detail?.id;
                          if (statusId && statusId !== item.status_id) {
                            try {
                              const updatedItem = await api.items.update(item.id, { status_id: statusId });
                              handleItemUpdated({ detail: { item: updatedItem, field: 'status_id', value: statusId } });
                            } catch (error) {
                              handleUpdateError({ detail: { error: error.message, field: 'status_id', value: statusId } });
                            }
                          }
                        }}
                      >
                        {#snippet children()}
                          <span class="cursor-pointer">
                            <Lozenge
                              text={selectedStatus ? selectedStatus.name : 'Set status'}
                              customBg={statusCategory?.color || '#6b7280'}
                            />
                          </span>
                        {/snippet}
                      </ItemPicker>
                    {/if}
                  </div>

                  <!-- Priority -->
                  <div class="col-span-2">
                  <!-- Priority -->
                  <ItemPicker
                    value={item.priority_id}
                    items={priorities}
                    config={{
                      icon: {
                        type: 'color-dot',
                        source: (priority) => priority.color || '#6b7280',
                        size: 'w-2 h-2'
                      },
                      primary: { text: (priority) => priority.name },
                      getValue: (priority) => priority.id,
                      getLabel: (priority) => priority.name,
                      searchFields: ['name']
                    }}
                    placeholder="Select priority"
                    showUnassigned={true}
                    unassignedLabel="No priority"
                    allowClear={true}
                    on:select={async (e) => {
                      const priorityId = e.detail?.id || null;
                      try {
                        const updatedItem = await api.items.update(item.id, { priority_id: priorityId });
                        handleItemUpdated({ detail: { item: updatedItem, field: 'priority_id', value: priorityId } });
                      } catch (error) {
                        handleUpdateError({ detail: { error: error.message, field: 'priority_id', value: priorityId } });
                      }
                    }}
                  >
                    {#snippet children()}
                      {#if item.priority_id}
                        {@const selectedPriority = priorities.find(p => p.id === item.priority_id)}
                        <span
                          class="w-full flex items-center justify-start gap-2 text-sm text-left cursor-pointer"
                          style={selectedPriority && selectedPriority.color ? `color: ${selectedPriority.color};` : 'color: var(--ds-text-subtle);'}
                        >
                          {#if selectedPriority}
                            <ColorDot color={selectedPriority.color} />
                            {selectedPriority.name}
                          {/if}
                        </span>
                      {:else}
                        <span
                          class="w-full flex items-center justify-start gap-2 text-sm text-left cursor-pointer"
                          style="color: var(--ds-text-subtle);"
                        >
                          {t('pickers.selectPriority')}
                        </span>
                      {/if}
                    {/snippet}
                  </ItemPicker>
                  </div>

                  <!-- Created Date -->
                  <div class="col-span-1">
                    <div class="flex items-center gap-1 text-sm" style="color: var(--ds-text-subtle);">
                      <Calendar class="w-4 h-4" />
                      {formatDate(item.created_at) || '-'}
                    </div>
                  </div>

                  <!-- Actions -->
                  <div class="col-span-1">
                    <DropdownMenu
                      triggerText=""
                      triggerIcon={MoreHorizontal}
                      triggerClass="p-2 rounded action-btn transition-colors"
                      items={buildItemActions(item)}
                      align="right"
                    />
                  </div>
                </div>
              </div>
            {/each}
          </div>
        </div>

        <!-- Pagination -->
        {#if itemsPagination && itemsPagination.total > 0}
          <div class="mt-6">
            <Pagination
              currentPage={itemsPagination.page}
              totalItems={itemsPagination.total}
              itemsPerPage={itemsPagination.limit}
              maxItems={100}
              hasGradient={styles.hasCustomBackground}
              on:pageChange={handlePageChange}
              on:pageSizeChange={handlePageSizeChange}
            />
          </div>
        {:else}
          <!-- Results Summary for legacy/non-paginated responses -->
          <div class="mt-4 text-sm  text-center" style="{styles.subtleTextStyle}">
            {t('collections.showingWorkItems', { count: filteredItems.length })}
          </div>
        {/if}
      {/if}
    </div>
  </div>
{:else}
  <div class="p-6">
    <div class="text-center " style="{styles.subtleTextStyle}">
      {t('workspaces.noWorkspaces')}
    </div>
  </div>
{/if}

<style>
  .list-row:hover {
    background-color: var(--ds-surface-hovered);
  }

  :global(.item-key:hover) {
    background-color: var(--ds-surface-hovered) !important;
  }

  .action-btn:hover {
    background-color: var(--ds-surface-hovered);
  }
</style>
