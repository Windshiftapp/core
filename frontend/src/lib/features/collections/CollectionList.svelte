<script>
  import { useEventListener } from 'runed';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast } from '../../stores/toasts.svelte.js';
  import { onWorkItemRefresh } from '../../stores/workItemRefresh.js';
  import {
    applyCollectionItem,
    collectionStore,
    reloadCollection,
    scheduleCollectionReconciliation,
  } from '../../stores/collectionContext.js';
  import { useCollectionEventStream } from '../../composables/useCollectionEventStream.svelte.js';
  import { useCollectionSearch } from '../../composables/useCollectionSearch.svelte.js';
  import { useCollectionEditorOptionsPrime } from '../../composables/useCollectionEditorOptionsPrime.svelte.js';
  import { useItemBulkSelection } from '../../composables/useItemBulkSelection.svelte.js';
  import { createDeleteItemHandler, createItemActionsBuilder } from '../../utils/workItemTableHelpers.js';
  import {
    getListColumnLabel,
    listGridMinWidth,
    listGridTemplateColumns,
  } from '../../utils/workItemListColumns.js';
  import { useGradientStyles } from '../../stores/workspaceGradient.svelte.js';
  import { workspacePermissions } from '../../stores/workspacePermissions.svelte.js';
  import { collectionFieldLinks } from '../../stores/collectionFieldLinks.svelte.js';
  import { itemModalStore } from '../../stores/itemModalStore.svelte.js';
  import { workspaceDataStore } from '../../stores/workspaceDataStore.svelte.js';
  import { objectDisplayName } from '../../utils/systemLabels.js';
  import ArrowUp from '@lucide/svelte/icons/arrow-up';
  import ArrowDown from '@lucide/svelte/icons/arrow-down';
  import ArrowUpDown from '@lucide/svelte/icons/arrow-up-down';
  import CheckSquare from '@lucide/svelte/icons/square-check';
  import Square from '@lucide/svelte/icons/square';
  import SearchInput from '../../components/SearchInput.svelte';
  import ViewHeader from '../../layout/ViewHeader.svelte';
  import StaticViewBackground from '../../layout/StaticViewBackground.svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import TableHeaderBar from '../../components/TableHeaderBar.svelte';
  import ButtonGroup from '../../components/ButtonGroup.svelte';
  import WorkItemListRow from './WorkItemListRow.svelte';
  import WorkItemListFooter from './WorkItemListFooter.svelte';
  import ColumnSelector from './ColumnSelector.svelte';
  import BulkActionToolbar from './BulkActionToolbar.svelte';
  import SubFilterBar from './SubFilterBar.svelte';
  import { useListColumnConfig } from './listColumnConfig.svelte.js';
  import { buildDisplayColumns } from './boardColumns.js';
  import VirtualRows from '../../components/VirtualRows.svelte';
  import { collectionItemRowKey } from './collectionVirtualRows';

  let { workspaceId, collectionId = null } = $props();
  let listElement = $state(null);

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
  let wdsLabels = $derived(workspaceDataStore.labels);

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
  useCollectionEditorOptionsPrime(
    () => workspaceId,
    () => ({ statuses, users, milestones, iterations, projects })
  );

  $effect(() =>
    onWorkItemRefresh((detail) => {
      const item = detail?.item;
      // Workspace-scoped views can apply the item immediately as a display
      // optimization. Collection membership is verified by the store's
      // coalesced reconciliation instead of one query per event.
      if (item && !collectionId && Number(item.workspace_id) === Number(workspaceId)) {
        applyCollectionItem(item);
      }
      scheduleCollectionReconciliation();
    })
  );

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

  // Multi-select mirrors the board: the same composable owns the selection set
  // and bulk operations, so both views expose identical actions.
  const selection = useItemBulkSelection({
    items: () => filteredItems,
    scopeWorkspaceId: () => workspaceId,
    reload: reloadCollection,
    labelNameLookup: () => wdsLabels,
  });

  // Move targets mirror the board's configured columns so both views offer the
  // same status destinations.
  let bulkMoveTargets = $derived(
    buildDisplayColumns(columnConfig.boardConfig, statuses)
      .filter((column) => column.status_ids?.length > 0)
      .map((column) => ({
        id: column.id,
        statusId: column.status_ids[0],
        title: objectDisplayName(column, 'status'),
        color: column.color,
      }))
  );

  // Selection mode reserves a leading track for the row checkboxes.
  const SELECT_TRACK = '2rem';
  let gridTemplateColumns = $derived(
    selection.selectionMode
      ? `${SELECT_TRACK} ${listGridTemplateColumns(listColumns)}`
      : listGridTemplateColumns(listColumns)
  );

  let gridMinWidth = $derived(
    selection.selectionMode
      ? `calc(${SELECT_TRACK} + ${listGridMinWidth(listColumns)})`
      : listGridMinWidth(listColumns)
  );

  // Shift selection extends across the visible (filtered) rows.
  function selectRangeTo(itemId) {
    selection.selectionMode = true;
    const ids = filteredItems.map((item) => item.id);
    const from = ids.indexOf(selection.selectionAnchorId);
    const to = ids.indexOf(itemId);
    if (from === -1 || to === -1) {
      selection.toggleSelected(itemId);
      return;
    }
    const start = Math.min(from, to);
    const end = Math.max(from, to);
    selection.selectedIds = [...new Set([...selection.selectedIds, ...ids.slice(start, end + 1)])];
  }

  useEventListener(() => window, 'keydown', (event) => {
    const target = event.target;
    const inEditable =
      target instanceof HTMLElement &&
      (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable);
    if (event.key === 'Escape' && selection.selectionMode && !inEditable) {
      event.preventDefault();
      selection.exitSelectionMode();
    }
  });

  // Resolve only the assignees the visible rows reference instead of waiting
  // for the whole directory before the list can render.
  $effect(() => {
    const ids = filteredItems.map((item) => item.assignee_id).filter(Boolean);
    if (ids.length > 0) void workspaceDataStore.hydrateUsers(ids);
  });

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
          <SubFilterBar {workspaceId} {collectionId} />
        </div>

        <div class="flex items-center gap-2">
          <!-- Selection is an explicit mode, matching the board's toolbar. -->
          <ButtonGroup testid="list-controls-group">
            {#snippet children(segment)}
              <button
                type="button"
                class={segment}
                data-active={selection.selectionMode}
                aria-pressed={selection.selectionMode}
                title={t('collections.selectItems')}
                onclick={selection.toggleSelectionMode}
                data-testid="list-select-mode-toggle"
              >
                <CheckSquare class="w-4 h-4" />
                {t('common.select')}
              </button>
            {/snippet}
          </ButtonGroup>
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
            {#if selection.selectionMode}
              <div></div>
            {/if}
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
          <div bind:this={listElement} style="overflow-anchor: none;">
            <VirtualRows items={filteredItems} getKey={collectionItemRowKey} {listElement}
              dragSources
              layoutRevision={gridTemplateColumns}
              identity={`list:${workspaceId ?? ''}:${collectionId ?? ''}:${collectionStore.effectiveSubFilterQL}:${search.query}:${sortKey}:${sortDirection}:${currentPage}`}
            >
            {#snippet row(item, virtual)}
              <WorkItemListRow
                virtualRow={virtual}
                deferred={false}
                {item}
                columns={listColumns}
                {gridTemplateColumns}
                {gridMinWidth}
                {workspace}
                canEdit={!selection.selectionMode && workspacePermissions.canEdit(item.workspace_id)}
                {statuses}
                {statusCategories}
                {priorities}
                {milestones}
                {iterations}
                {users}
                {projects}
                {itemTypes}
                {customFieldDefinitions}
                actions={buildItemActions(item)}
                onitemUpdated={handleItemUpdated}
                onupdateError={handleUpdateError}
                draggable
                rowTestid={`workspace-item-row-${item.id}`}
                selectionMode={selection.selectionMode}
                selected={selection.selectedIdSet.has(item.id)}
                onselect={selection.toggleSelected}
                onshiftselect={selectRangeTo}
              >
                {#snippet leading(row)}
                  {#if selection.selectionMode}
                    <!-- inline-style-guard-exempt: selection checkbox mirrors the board card's dynamic accent. -->
                    <button
                      type="button"
                      class="-ml-0.5 shrink-0 rounded p-0.5"
                      style="color: {selection.selectedIdSet.has(row.id) ? 'var(--ds-border-focused)' : 'var(--ds-icon-subtle)'};"
                      data-testid={`list-row-select-${row.id}`}
                      aria-pressed={selection.selectedIdSet.has(row.id)}
                      aria-label={t('collections.toggleSelection')}
                      title={t('collections.toggleSelection')}
                      onclick={(event) => {
                        event.stopPropagation();
                        selection.toggleSelected(row.id);
                      }}
                      onkeydown={(event) => event.stopPropagation()}
                    >
                      {#if selection.selectedIdSet.has(row.id)}
                        <CheckSquare class="h-4 w-4" />
                      {:else}
                        <Square class="h-4 w-4" />
                      {/if}
                    </button>
                  {/if}
                {/snippet}
              </WorkItemListRow>
            {/snippet}
            </VirtualRows>
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
          <WorkItemListFooter
            pagination={itemsPagination}
            itemCount={workItems.length}
            onpageChange={handlePageChange}
            onpageSizeChange={handlePageSizeChange}
          />
        {:else}
          <!-- Results Summary for legacy/non-paginated responses -->
          <div class="mt-4 text-sm  text-center" style="color: var(--ctx-text-subtle, var(--ds-text-subtle));">
            {t('collections.showingWorkItems', { count: filteredItems.length })}
          </div>
        {/if}
      {/if}
  </StaticViewBackground>

  <BulkActionToolbar
    {selection}
    moveTargets={bulkMoveTargets}
    {priorities}
    testidPrefix="list"
  />
{:else}
  <div class="p-6">
    <div class="text-center " style="color: var(--ctx-text-subtle, var(--ds-text-subtle));">
      {t('workspaces.noWorkspaces')}
    </div>
  </div>
{/if}

<style>
  :global(.item-key:hover) {
    background-color: var(--ds-background-neutral-hovered) !important;
  }
</style>
