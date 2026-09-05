<script>
  import { untrack } from 'svelte';
  import { useEventListener } from 'runed';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast } from '../../stores/toasts.svelte.js';
  import { api } from '../../api.js';
  import { navigate } from '../../router.js';
  import { applyCollectionItem, collectionStore, reloadCollection } from '../../stores/collectionContext.js';
  import { createDeleteItemHandler, createItemActionsBuilder } from '../../utils/workItemTableHelpers.js';
  import {
    buildListColumnConfiguration,
    DEFAULT_LIST_COLUMNS,
    getListColumnLabel,
    listColumnsFromConfig,
  } from '../../utils/workItemListColumns.js';
  import { useGradientStyles } from '../../stores/workspaceGradient.svelte.js';
  import { workspacePermissions } from '../../stores/workspacePermissions.svelte.js';
  import { collectionEditorOptions, collectionFieldLinks, workspaceDataStore } from '../../stores/index.js';
  import { MoreHorizontal, ArrowUp, ArrowDown, ArrowUpDown } from '@lucide/svelte';
  import { draggable } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import SearchInput from '../../components/SearchInput.svelte';
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
  import Pagination from '../../components/Pagination.svelte';
  import ViewHeader from '../../layout/ViewHeader.svelte';
  import StaticViewBackground from '../../layout/StaticViewBackground.svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import TableHeaderBar from '../../components/TableHeaderBar.svelte';
  import ListCellRenderer from './ListCellRenderer.svelte';
  import ColumnSelector from './ColumnSelector.svelte';
  import SubFilterBar from './SubFilterBar.svelte';
  import LazyRender from '../../components/LazyRender.svelte';
  import { checkItemVisibility } from './collectionService.js';

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
  let sortKey = $state(null);
  let sortDirection = $state(null); // 'asc' | 'desc' | null

  let sortableFields = $derived(new Set(collectionStore.sortableFields));

  function toggleSort(fieldIdentifier) {
    if (!sortableFields.has(fieldIdentifier)) return;
    if (sortKey === fieldIdentifier) {
      sortDirection = sortDirection === 'asc' ? 'desc' : 'asc';
    } else {
      sortKey = fieldIdentifier;
      sortDirection = 'asc';
    }
    collectionStore.setSorting(sortKey, sortDirection);
  }

  // Centralized gradient styling
  const styles = useGradientStyles();

  // Computed: Check if user can configure columns (workspace admin)
  let canConfigureColumns = $derived(workspacePermissions.canAdminWorkspace(workspaceId));

  // A workspace-scoped list already has these option sets in the shared
  // workspace store. Prime the row-editor cache so opening a cell does not
  // repeat those requests. Global collections deliberately skip this: their
  // rows can belong to different workspaces and must resolve independently.
  $effect(() => {
    const id = Number(workspaceId);
    if (
      !Number.isInteger(id) ||
      id <= 0 ||
      !workspaceDataStore.initialized ||
      Number(workspaceDataStore.workspaceId) !== id
    ) {
      return;
    }

    collectionEditorOptions.prime(id, {
      statuses,
      users,
      milestones,
      iterations,
      projects,
    });
  });

  // Computed: Calculate total grid columns (sum of widths + 1 for actions)
  let totalGridColumns = $derived(
    listColumns.reduce((sum, col) => sum + col.width, 0) + 1
  );

  // Computed: Generate grid-template-columns CSS
  // Per-column baselines (rem) — what "M" looks like today.
  // S/L/XL scale around this so the size picker has visible effect.
  const baseFixedWidths = {
    status: 8,
    priority: 7,
    assignee: 9,
    milestone: 12,
    iteration: 9,
    due_date: 7,
    created_at: 7,
    project: 9,
  };

  // width values: 1=S, 2=M, 3=L, 4=XL
  const widthScale = { 1: 0.75, 2: 1, 3: 1.5, 4: 2 };

  function columnTrack(col) {
    if (col.field_identifier === 'key') return 'max-content';
    const base = baseFixedWidths[col.field_identifier];
    if (base !== undefined) {
      const scale = widthScale[col.width] ?? 1;
      return `${base * scale}rem`;
    }
    return `${col.width}fr`;
  }

  let gridTemplateColumns = $derived(
    listColumns.map(columnTrack).join(' ') + ' auto'
  );


  useEventListener(() => window, 'refresh-work-items', async (/** @type {CustomEvent} */ event) => {
    const item = event.detail?.item;
    if (!item) {
      reloadCollection();
      return;
    }
    const belongsToView = collectionId
      ? await checkItemVisibility(item.id, { collection_id: collectionId })
      : Number(item.workspace_id) === Number(workspaceId);
    if (belongsToView) applyCollectionItem(item);
  });

  // Board config depends only on the viewed collection/workspace, not on the
  // item set or load state — fetch it when the view changes, not every time
  // the collection reloads (which toggles collectionStore.loading).
  let viewSignature = $derived(`${collectionId ?? ''}|${workspaceId ?? ''}`);
  $effect(() => {
    viewSignature;
    if (collectionId || workspaceId) {
      untrack(() => loadBoardConfiguration());
    }
  });

  // Sync collection name + clear local loading from the central store.
  $effect(() => {
    if (!collectionStore.loading) {
      currentCollectionName = collectionStore.collectionName;
      loading = false;
    }
  });

  async function loadBoardConfiguration() {
    try {
      const config = await collectionStore.getBoardConfiguration(workspaceId, collectionId);
      boardConfig = config;
      listColumns = listColumnsFromConfig(config);
    } catch (error) {
      boardConfig = null;
      listColumns = [...DEFAULT_LIST_COLUMNS];
    }
  }

  async function saveBoardConfiguration(newColumns) {
    try {
      const configData = buildListColumnConfiguration(boardConfig, newColumns);

      if (boardConfig?.id) {
        // Update existing config
        const updated = await api.collections.updateBoardConfiguration(
          collectionId,
          boardConfig.id,
          configData,
          workspaceId
        );
        boardConfig = updated;
        listColumns = listColumnsFromConfig(updated);
      } else {
        // Create new config - pass raw collectionId so API can detect workspace-level config
        const created = await api.collections.createBoardConfiguration(
          collectionId,
          workspaceId,
          configData
        );
        boardConfig = created;
        listColumns = listColumnsFromConfig(created);
      }
    } catch (error) {
      console.error('Failed to save board configuration:', error);
      errorToast(t('dialogs.alerts.failedToSave', { error: error.message || error }));
    }
  }

  function handleColumnChange(data) {
    const { columns: newColumns } = data;
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
    return workItems.filter(item => {
      if (item.title.toLowerCase().includes(query)) return true;
      if (item.description && item.description.toLowerCase().includes(query)) return true;
      const itemKey = `${item.workspace_key || ''}-${item.workspace_item_number}`.toLowerCase();
      if (itemKey.includes(query)) return true;
      return false;
    });
  });
  let filteredItemsById = $derived(new Map(filteredItems.map((item) => [item.id, item])));

  function registerListRow(element, itemId) {
    const cleanup = draggable({
      element,
      getInitialData: () => ({
        item: filteredItemsById.get(itemId),
        type: 'work-item'
      }),
      onDragStart: () => {
        element.style.opacity = '0.5';
      },
      onDrop: () => {
        element.style.opacity = '';
      }
    });
    return {
      destroy() {
        element.style.opacity = '';
        cleanup();
      }
    };
  }

  // Linking fields live in item_links rather than custom_field_values. Hydrate
  // all visible rows through the bounded batch endpoint so adding one linking
  // column does not issue one request per row.
  $effect(() => {
    const linkingFieldIds = new Set(
      customFieldDefinitions
        .filter((field) => field.field_type === 'linking')
        .map((field) => String(field.id)),
    );
    const hasLinkingColumn = listColumns.some(
      (column) =>
        column.field_type === 'custom' && linkingFieldIds.has(String(column.field_identifier)),
    );
    if (hasLinkingColumn) {
      collectionFieldLinks.loadForItems(filteredItems.map((item) => item.id));
    }
  });

  function viewItem(item) {
    const wsId = workspaceId || item.workspace_id;
    const url = collectionId && workspaceId
      ? `/workspaces/${workspaceId}/collections/${collectionId}/items/${item.id}`
      : `/workspaces/${wsId}/items/${item.id}`;
    navigate(url);
  }

  const deleteItem = createDeleteItemHandler({
    confirmMessage: (item) => t('collections.confirmDeleteItem', { title: item.title }),
    onDeleted: () => reloadCollection(),
  });

  const buildItemActions = createItemActionsBuilder({ viewItem, deleteItem });

  function handleItemUpdated(data) {
    applyCollectionItem(data.item);
  }

  function handleUpdateError(data) {
    const { error, field, value } = data;
    console.error(`Failed to update ${field}:`, error);
    errorToast(t('dialogs.alerts.failedToUpdate', { error: `${field}: ${error}` }));
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
    testid="list-view"
  >
    <!-- Content Container -->
      <div class="mb-6">
        <ViewHeader
          workspaceName={workspace?.name || ''}
          collection={currentCollectionName}
          viewName="List"
          itemCount={itemsPagination?.total_items ?? workItems.length}
        />
      </div>

      <!-- Controls Bar -->
      <div class="flex items-center justify-between mb-6">
        <div class="flex items-center gap-4">
          <!-- Search -->
          <SearchInput
            bind:value={searchQuery}
            placeholder={t('common.search')}
          />
          <SubFilterBar {workspaceId} />
        </div>

        <div class="flex items-center gap-2">
          <!-- Column Selector -->
          <ColumnSelector
            columns={listColumns}
            {customFieldDefinitions}
            canConfigure={canConfigureColumns}
            onchange={handleColumnChange}
          />
        </div>
      </div>

      <!-- Work Items Table -->
      {#if loadingItems}
        <div class="p-8 text-center">
          <div class="animate-pulse" style="color: var(--ctx-text-subtle, var(--ds-text-subtle));">{t('common.loading')}</div>
        </div>
      {:else if filteredItems.length === 0}
        {#if workItems.length === 0}
          <EmptyState
            title={t('items.noItems')}
            description={t('items.createToStart')}
          />
        {:else}
          <EmptyState
            title={t('items.noItemsInFilter')}
            description={t('items.noItemsInFilter')}
          />
        {/if}
      {:else}
        <div class="rounded-xl border shadow-sm overflow-hidden" style="{styles.tableStyle(12)} border-color: var(--ctx-border, var(--ds-border));">
          <!-- Table Header -->
          <TableHeaderBar
            columns={gridTemplateColumns}
            style={styles.tableHeaderStyle}
          >
            {#each listColumns as column (column.field_identifier)}
              {#if sortableFields.has(column.field_identifier)}
                <button
                  class="group inline-flex items-center gap-1 cursor-pointer select-none"
                  onclick={() => toggleSort(column.field_identifier)}
                >
                  {getListColumnLabel(column, customFieldDefinitions)}
                  {#if sortKey === column.field_identifier && sortDirection === 'asc'}
                    <ArrowUp class="w-3.5 h-3.5" style="color: var(--ds-text-subtle);" />
                  {:else if sortKey === column.field_identifier && sortDirection === 'desc'}
                    <ArrowDown class="w-3.5 h-3.5" style="color: var(--ds-text-subtle);" />
                  {:else}
                    <ArrowUpDown class="w-3.5 h-3.5 opacity-0 group-hover:opacity-100 transition-opacity" style="color: var(--ds-text-subtlest);" />
                  {/if}
                </button>
              {:else}
                <div>{getListColumnLabel(column, customFieldDefinitions)}</div>
              {/if}
            {/each}
            <div>{t('common.actions')}</div>
          </TableHeaderBar>

          <!-- Table Body -->
          <div>
            {#each filteredItems as item (item.id)}
              <div use:registerListRow={item.id} class="px-4 py-3 list-row transition-colors" style="border-top: 1px solid var(--ds-border);" data-item-row data-item-id={item.id} data-testid={`workspace-item-row-${item.id}`}>
                <LazyRender>
                  {#snippet children()}
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
                            canEdit={workspacePermissions.canEdit(item.workspace_id)}
                            {statuses}
                            {statusCategories}
                            {priorities}
                            {milestones}
                            {iterations}
                            {users}
                            {projects}
                            {itemTypes}
                            {customFieldDefinitions}
                            onitemUpdated={handleItemUpdated}
                            onupdateError={handleUpdateError}
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
                        />
                      </div>
                    </div>
                  {/snippet}
                </LazyRender>
              </div>
            {/each}
          </div>
        </div>

        <!-- Pagination -->
        {#if itemsPagination && itemsPagination.total > 0 && workItems.length > 0}
          <div class="mt-6">
            <Pagination
              currentPage={itemsPagination.page}
              totalItems={itemsPagination.total}
              itemsPerPage={itemsPagination.limit}
              maxItems={10000}
              onpageChange={handlePageChange}
              onpageSizeChange={handlePageSizeChange}
            />
          </div>
        {:else}
          <!-- Results Summary for legacy/non-paginated responses -->
          <div class="mt-4 text-sm  text-center" style="color: var(--ctx-text-subtle, var(--ds-text-subtle));">
            {t('collections.showingWorkItems', { count: filteredItems.length })}
          </div>
        {/if}
      {/if}
  </StaticViewBackground>
{:else}
  <div class="p-6">
    <div class="text-center " style="color: var(--ctx-text-subtle, var(--ds-text-subtle));">
      {t('workspaces.noWorkspaces')}
    </div>
  </div>
{/if}

<style>
  .list-row:hover {
    background-color: var(--ds-background-neutral-hovered);
  }

  :global(.item-key:hover) {
    background-color: var(--ds-background-neutral-hovered) !important;
  }
</style>
