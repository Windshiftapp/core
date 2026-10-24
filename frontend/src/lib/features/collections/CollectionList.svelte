<script>
  import { untrack } from 'svelte';
  import { useEventListener } from 'runed';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast } from '../../stores/toasts.svelte.js';
  import {
    applyCollectionItem,
    collectionStore,
    reloadCollection,
    scheduleCollectionReconciliation,
  } from '../../stores/collectionContext.js';
  import { useCollectionEventStream } from '../../composables/useCollectionEventStream.svelte.js';
  import { useCollectionSearch } from '../../composables/useCollectionSearch.svelte.js';
  import { createDeleteItemHandler, createItemActionsBuilder } from '../../utils/workItemTableHelpers.js';
  import {
    getListColumnLabel,
    listGridMinWidth,
    listGridTemplateColumns,
  } from '../../utils/workItemListColumns.js';
  import { useGradientStyles } from '../../stores/workspaceGradient.svelte.js';
  import { workspacePermissions } from '../../stores/workspacePermissions.svelte.js';
  import { collectionEditorOptions } from '../../stores/collectionEditorOptions.svelte.js';
  import { collectionFieldLinks } from '../../stores/collectionFieldLinks.svelte.js';
  import { itemModalStore } from '../../stores/itemModalStore.svelte.js';
  import { workspaceDataStore } from '../../stores/workspaceDataStore.svelte.js';
  import MoreHorizontal from '@lucide/svelte/icons/more-horizontal';
  import ArrowUp from '@lucide/svelte/icons/arrow-up';
  import ArrowDown from '@lucide/svelte/icons/arrow-down';
  import ArrowUpDown from '@lucide/svelte/icons/arrow-up-down';
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
  import { useListColumnConfig } from './listColumnConfig.svelte.js';

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

  // List columns from the board configuration (shared with the tree view)
  const columnConfig = useListColumnConfig(() => ({ workspaceId, collectionId }));
  let listColumns = $derived(columnConfig.columns);

  let loading = $state(true);
  let loadingItems = $state(false);
  let currentCollectionName = $state('Default');
  let currentView = $state('list');
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

    untrack(() =>
      collectionEditorOptions.prime(
        id,
        {
          statuses,
          users,
          milestones,
          iterations,
          projects,
        },
        workspaceDataStore.planningCompleteness
      )
    );
  });

  // Computed: Generate grid-template-columns CSS via the shared list grid
  // helpers (also used by the support queue's list-shaped table).
  let gridTemplateColumns = $derived(listGridTemplateColumns(listColumns));

  let gridMinWidth = $derived(listGridMinWidth(listColumns));

  useEventListener(() => window, 'refresh-work-items', (/** @type {CustomEvent} */ event) => {
    const item = event.detail?.item;
    // Workspace-scoped views can apply the item immediately as a display
    // optimization. Collection membership is verified by the store's
    // coalesced reconciliation instead of one query per event.
    if (item && !collectionId && Number(item.workspace_id) === Number(workspaceId)) {
      applyCollectionItem(item);
    }
    scheduleCollectionReconciliation();
  });

  // Scoped SSE invalidations (WI-1624): any frame runs the existing delta fetch,
  // and a reconnect reconciles. Without this the list kept showing items that
  // were deleted from the item detail modal, unlike the board views.
  useCollectionEventStream(
    () => (collectionId ? { kind: 'collection', id: collectionId } : { kind: 'workspace', id: workspaceId }),
    { onInvalidate: () => scheduleCollectionReconciliation() }
  );

  // Sync collection name + clear local loading from the central store.
  $effect(() => {
    if (!collectionStore.loading) {
      currentCollectionName = collectionStore.collectionName;
      loading = false;
    }
  });

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

  // Server-side search shared with the board: a query spans every page of the
  // collection instead of filtering the loaded page (GH #328).
  const search = useCollectionSearch(
    () => `${workspaceId ?? ''}|${collectionId ?? ''}|${collectionStore.effectiveSubFilterQL}`
  );
  let filteredItems = $derived(search.items);
  let filteredItemsById = $derived(new Map(filteredItems.map((item) => [item.id, item])));

  // Resolve only the assignees the visible rows reference instead of waiting
  // for the whole directory before the list can render.
  $effect(() => {
    const ids = filteredItems.map((item) => item.assignee_id).filter(Boolean);
    if (ids.length > 0) void workspaceDataStore.hydrateUsers(ids);
  });

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
    itemModalStore.open(item.id, item.workspace_id ?? workspaceId);
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
          itemCount={collectionStore.collectionTotal}
          shownCount={collectionStore.loading ? null : filteredItems.length}
        />
      </div>

      <!-- Controls Bar -->
      <div class="flex items-center justify-between mb-6">
        <div class="flex items-center gap-4">
          <!-- Search -->
          <SearchInput
            bind:value={search.query}
            placeholder={t('common.search')}
            dataTestid="list-search-input"
          />
          <SubFilterBar {workspaceId} />
        </div>

        <div class="flex items-center gap-2">
          <!-- Column Selector -->
          <ColumnSelector
            columns={listColumns}
            {customFieldDefinitions}
            canConfigure={columnConfig.canConfigure}
            onchange={({ columns }) => columnConfig.save(columns)}
          />
        </div>
      </div>

      {#if search.active && (search.debouncing || collectionStore.searchLoading)}
        <p class="-mt-2 mb-4 text-sm" style="color: var(--ctx-text-subtle, var(--ds-text-subtle));" data-testid="list-search-status">
          {t('common.searching')}
        </p>
      {:else if search.active && collectionStore.searchError}
        <p class="-mt-2 mb-4 text-sm" style="color: var(--ds-text-danger);" data-testid="list-search-error">
          {t('common.error')}
        </p>
      {/if}

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
        <div class="rounded-xl border shadow-sm overflow-x-auto" style="{styles.tableStyle(12)} border-color: var(--ctx-border, var(--ds-border));">
          <!-- Table Header -->
          <TableHeaderBar
            columns={gridTemplateColumns}
            minWidth={gridMinWidth}
            style={styles.tableHeaderStyle}
          >
            {#each listColumns as column (column.field_identifier)}
              {#if sortableFields.has(column.field_identifier)}
                <button
                  data-testid={`list-sort-${column.field_identifier}`}
                  class="group inline-flex items-center gap-1 cursor-pointer select-none"
                  onclick={() => toggleSort(column.field_identifier)}
                >
                  {getListColumnLabel(column, customFieldDefinitions)}
                  {#if sortKey === column.field_identifier && sortDirection === 'asc'}
                    <ArrowUp class="w-3.5 h-3.5" style="color: var(--ds-text-subtle);" />
                  {:else if sortKey === column.field_identifier && sortDirection === 'desc'}
                    <ArrowDown class="w-3.5 h-3.5" style="color: var(--ds-text-subtle);" />
                  {:else}
                    <ArrowUpDown class="w-3.5 h-3.5 opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 pointer-coarse:opacity-100 transition-opacity" style="color: var(--ds-text-subtlest);" />
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
                      style="grid-template-columns: {gridTemplateColumns}; min-width: {gridMinWidth};"
                    >
                      {#each listColumns as column (column.field_identifier)}
                        <div class="min-w-0 overflow-hidden">
                          <ListCellRenderer
                            {item}
                            {column}
                            {workspace}
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

        <!-- Search results page on the server, so they use load-more instead of
             the page control that drives the unfiltered list. -->
        {#if search.active}
          {#if search.hasMore}
            <div class="mt-6 text-center">
              <button
                data-testid="list-search-load-more"
                onclick={() => search.loadMore()}
                disabled={search.loadingMore}
                class="px-4 py-2 text-sm rounded-lg border transition-colors"
                style="border-color: var(--ctx-border, var(--ds-border)); color: var(--ds-text);"
              >
                {search.loadingMore ? t('common.loading') : t('common.loadMore')}
                {#if search.remainingCount > 0}
                  ({search.remainingCount} {t('common.remaining')})
                {/if}
              </button>
            </div>
          {:else}
            <div class="mt-4 text-sm text-center" style="color: var(--ctx-text-subtle, var(--ds-text-subtle));" data-testid="list-search-summary">
              {t('collections.showingWorkItems', { count: filteredItems.length })}
            </div>
          {/if}
        {:else if itemsPagination && itemsPagination.total_items > 0 && workItems.length > 0}
          <div class="mt-6">
            <Pagination
              currentPage={itemsPagination.page}
              totalItems={itemsPagination.total_items}
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
