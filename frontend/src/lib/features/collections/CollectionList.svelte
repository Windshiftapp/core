<script>
  import { onMount, onDestroy } from 'svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { api } from '../../api.js';
  import { navigate } from '../../router.js';
  import { getCollection } from '../collections/collectionService.js';
  import { useGradientStyles, loadWorkspaceGradient } from '../../stores/workspaceGradient.svelte.js';
  import { workspacePermissions } from '../../stores/workspacePermissions.svelte.js';
  import { MoreHorizontal, Trash2, Eye } from 'lucide-svelte';
  import SearchInput from '../../components/SearchInput.svelte';
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
  import Pagination from '../../components/Pagination.svelte';
  import ViewHeader from '../../layout/ViewHeader.svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import ListCellRenderer from './ListCellRenderer.svelte';
  import ColumnSelector from './ColumnSelector.svelte';

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
  let iterations = $state([]);
  let priorities = $state([]);
  let projects = $state([]);
  let customFieldDefinitions = $state([]);

  // Board configuration for list columns
  let boardConfig = $state(null);
  let listColumns = $state([]);

  let loading = $state(true);
  let loadingItems = $state(false);
  let currentCollectionName = $state('Default');
  let currentView = $state('list');
  let searchQuery = $state('');
  let currentPage = $state(1);
  let itemsPerPage = $state(50);

  // Default column configuration
  const defaultColumns = [
    { field_identifier: 'key', field_type: 'system', display_order: 0, width: 1 },
    { field_identifier: 'title', field_type: 'system', display_order: 1, width: 4 },
    { field_identifier: 'status', field_type: 'system', display_order: 2, width: 2 },
    { field_identifier: 'priority', field_type: 'system', display_order: 3, width: 2 },
    { field_identifier: 'created_at', field_type: 'system', display_order: 4, width: 2 }
  ];

  // Centralized gradient styling
  const styles = useGradientStyles();

  // Computed: Check if user can edit items
  let canEdit = $derived(workspacePermissions.canEdit(workspaceId));

  // Computed: Check if user can configure columns (workspace admin)
  let canConfigureColumns = $derived(workspacePermissions.canAdminWorkspace(workspaceId));

  // Computed: Calculate total grid columns (sum of widths + 1 for actions)
  let totalGridColumns = $derived(
    listColumns.reduce((sum, col) => sum + col.width, 0) + 1
  );

  // Computed: Generate grid-template-columns CSS
  let gridTemplateColumns = $derived(
    listColumns.map(col => `${col.width}fr`).join(' ') + ' auto'
  );

  onMount(async () => {
    if (workspaceId) {
      await loadWorkspaceGradient(workspaceId);
      await loadWorkspace();
      await loadBoardConfiguration();
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
      const [workspaceData, itemTypesData, statusesData, statusCategoriesData, usersData, milestonesData, iterationsData, prioritiesData, projectsData] = await Promise.all([
        api.workspaces.get(workspaceId),
        api.itemTypes.getAll(),
        api.workspaces.getStatuses(workspaceId),
        api.statusCategories.getAll(),
        api.getUsers(),
        api.milestones.getAll(),
        api.iterations.getAll(),
        api.priorities.getAll(),
        api.workspaces.getProjects ? api.workspaces.getProjects(workspaceId) : Promise.resolve([])
      ]);
      workspace = workspaceData;
      itemTypes = itemTypesData || [];
      statuses = statusesData || [];
      statusCategories = statusCategoriesData || [];
      users = usersData || [];
      milestones = milestonesData || [];
      iterations = iterationsData || [];
      priorities = prioritiesData || [];
      projects = projectsData || [];

      // Load custom field definitions for the workspace
      if (workspaceData.configuration_set_id) {
        try {
          const configSet = await api.configurationSets?.get(workspaceData.configuration_set_id);
          customFieldDefinitions = configSet?.custom_fields || [];
        } catch (e) {
          console.warn('Failed to load custom field definitions:', e);
          customFieldDefinitions = [];
        }
      }
    } catch (error) {
      console.error('Failed to load workspace:', error);
    }
  }

  async function loadBoardConfiguration() {
    try {
      // Try to load collection-specific config, or workspace default
      const config = await api.collections.getBoardConfiguration(collectionId, workspaceId);
      boardConfig = config;
      listColumns = config.list_columns && config.list_columns.length > 0
        ? [...config.list_columns].sort((a, b) => a.display_order - b.display_order)
        : [...defaultColumns];
    } catch (error) {
      // If no config exists, use defaults
      console.log('No board configuration found, using defaults');
      boardConfig = null;
      listColumns = [...defaultColumns];
    }
  }

  async function saveBoardConfiguration(newColumns) {
    try {
      const configData = {
        columns: boardConfig?.columns || [],
        backlog_status_ids: boardConfig?.backlog_status_ids || [],
        list_columns: newColumns
      };

      if (boardConfig?.id) {
        // Update existing config
        const updated = await api.collections.updateBoardConfiguration(
          collectionId,
          boardConfig.id,
          configData
        );
        boardConfig = updated;
        listColumns = updated.list_columns && updated.list_columns.length > 0
          ? [...updated.list_columns].sort((a, b) => a.display_order - b.display_order)
          : [...defaultColumns];
      } else {
        // Create new config - pass raw collectionId so API can detect workspace-level config
        const created = await api.collections.createBoardConfiguration(
          collectionId,
          workspaceId,
          configData
        );
        boardConfig = created;
        listColumns = created.list_columns && created.list_columns.length > 0
          ? [...created.list_columns].sort((a, b) => a.display_order - b.display_order)
          : [...defaultColumns];
      }
    } catch (error) {
      console.error('Failed to save board configuration:', error);
      alert(t('dialogs.alerts.failedToSave', { error: error.message || error }));
    }
  }

  function handleColumnChange(event) {
    const { columns: newColumns } = event.detail;
    saveBoardConfiguration(newColumns);
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
    alert(t('dialogs.alerts.failedToUpdate', { error: `${field}: ${error}` }));
  }

  // Reload data when workspaceId or collectionId changes
  let lastWorkspaceId = workspaceId;
  let lastCollectionId = collectionId;
  $effect(() => {
    if (workspaceId && !loading) {
      if (workspaceId !== lastWorkspaceId || collectionId !== lastCollectionId) {
        lastWorkspaceId = workspaceId;
        lastCollectionId = collectionId;
        loadBoardConfiguration();
        loadWorkItems();
      }
    }
  });

  // Get column header name
  function getColumnHeaderName(column) {
    const systemFieldNames = {
      key: t('common.key'),
      title: t('common.title'),
      status: t('common.status'),
      priority: t('common.priority'),
      assignee: t('common.assignee'),
      milestone: t('common.milestone'),
      iteration: t('common.iteration'),
      due_date: t('common.dueDate'),
      created_at: t('common.created'),
      project: t('common.project')
    };

    if (column.field_type === 'system') {
      return systemFieldNames[column.field_identifier] || column.field_identifier;
    } else {
      const customField = customFieldDefinitions.find(f => f.identifier === column.field_identifier);
      return customField?.name || column.field_identifier;
    }
  }

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

        <div class="flex items-center gap-2">
          <!-- Column Selector -->
          <ColumnSelector
            columns={listColumns}
            {customFieldDefinitions}
            canConfigure={canConfigureColumns}
            on:change={handleColumnChange}
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
            <div
              class="grid gap-4 text-xs font-semibold uppercase tracking-wider"
              style="grid-template-columns: {gridTemplateColumns}; {styles.glassSubtleTextStyle}"
            >
              {#each listColumns as column (column.field_identifier)}
                <div>{getColumnHeaderName(column)}</div>
              {/each}
              <div>{t('common.actions')}</div>
            </div>
          </div>

          <!-- Table Body -->
          <div>
            {#each filteredItems as item (item.id)}
              <div class="px-4 py-3 list-row transition-colors" style="border-top: 1px solid var(--ds-border);">
                <div
                  class="grid gap-4 items-center"
                  style="grid-template-columns: {gridTemplateColumns};"
                >
                  {#each listColumns as column (column.field_identifier)}
                    <div class="min-w-0">
                      <ListCellRenderer
                        {item}
                        {column}
                        {workspace}
                        {collectionId}
                        {canEdit}
                        {statuses}
                        {statusCategories}
                        {priorities}
                        {milestones}
                        {iterations}
                        {users}
                        {projects}
                        {itemTypes}
                        {customFieldDefinitions}
                        on:item-updated={handleItemUpdated}
                        on:update-error={handleUpdateError}
                      />
                    </div>
                  {/each}

                  <!-- Actions -->
                  <div>
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
