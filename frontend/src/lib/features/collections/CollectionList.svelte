<script>
  import { onMount, untrack } from 'svelte';
  import { useEventListener } from 'runed';
  import { t } from '../../stores/i18n.svelte.js';
  import { api } from '../../api.js';
  import { navigate } from '../../router.js';
  import { collectionStore, reloadCollection } from '../../stores/collectionContext.js';
  import { useGradientStyles, loadWorkspaceGradient } from '../../stores/workspaceGradient.svelte.js';
  import { workspacePermissions } from '../../stores/workspacePermissions.svelte.js';
  import { workspaceDataStore } from '../../stores/index.js';
  import { useWorkItemPoller } from '../../composables/useWorkItemPoller.svelte.js';
  import { MoreHorizontal, Trash2, Eye } from 'lucide-svelte';
  import SearchInput from '../../components/SearchInput.svelte';
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
  import Pagination from '../../components/Pagination.svelte';
  import ViewHeader from '../../layout/ViewHeader.svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import ListCellRenderer from './ListCellRenderer.svelte';
  import ColumnSelector from './ColumnSelector.svelte';

  let { workspaceId, collectionId = null } = $props();

  // Reference data from shared workspace store
  let workspace = $derived(workspaceDataStore.workspace);
  let itemTypes = $derived(workspaceDataStore.itemTypes);
  let statuses = $derived(workspaceDataStore.statuses);
  let statusCategories = $derived(workspaceDataStore.statusCategories);
  let users = $derived(workspaceDataStore.users);
  let milestones = $derived(workspaceDataStore.milestones);
  let iterations = $derived(workspaceDataStore.iterations);
  let priorities = $derived(workspaceDataStore.priorities);
  let projects = $derived(workspaceDataStore.projects);
  let customFieldDefinitions = $derived(workspaceDataStore.customFieldDefinitions);

  // Dynamic view-specific state
  let workItems = $derived(collectionStore.items);
  let itemsPagination = $derived(collectionStore.itemsPagination);

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


  useEventListener(() => window, 'refresh-work-items', () => reloadCollection());

  // Sync collection name and load board config from central store
  $effect(() => {
    if (!collectionStore.loading) {
      currentCollectionName = collectionStore.collectionName;
      untrack(() => {
        loadBoardConfiguration();
        loading = false;
      });
    }
  });

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
    currentPage = page;
    itemsPerPage = limit;
    await collectionStore.setItemsPage(page, limit);
  }

  // Handle pagination events
  async function handlePageChange(event) {
    await loadWorkItems(event.detail.page, event.detail.itemsPerPage);
  }

  async function handlePageSizeChange(event) {
    await loadWorkItems(event.detail.page, event.detail.itemsPerPage);
  }

  // Client-side search filtering on current page of items
  let filteredItems = $derived.by(() => {
    if (!searchQuery.trim()) return workItems;
    const query = searchQuery.toLowerCase();
    return workItems.filter(item =>
      item.title.toLowerCase().includes(query) ||
      (item.description && item.description.toLowerCase().includes(query))
    );
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
      reloadCollection();
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

  // Handle inline editing events — reload from server to get fresh data
  function handleItemUpdated(event) {
    reloadCollection();
  }

  function handleUpdateError(event) {
    const { error, field, value } = event.detail;
    console.error(`Failed to update ${field}:`, error);
    alert(t('dialogs.alerts.failedToUpdate', { error: `${field}: ${error}` }));
  }


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
      const customField = customFieldDefinitions.find(f => String(f.id) === column.field_identifier);
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
          itemCount={itemsPagination?.total ?? workItems.length}
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
              maxItems={10000}
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
