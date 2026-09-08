<script>
  import { onMount } from 'svelte';
  import { AlertCircle, Search } from '@lucide/svelte';
  import { t } from '../stores/i18n.svelte.js';
  import PageHeader from '../layout/PageHeader.svelte';
  import Card from '../components/Card.svelte';
  import DataTable from '../components/DataTable.svelte';
  import Pagination from '../components/Pagination.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import Button from '../components/Button.svelte';
  import QlQueryBar from '../features/shared/QlQueryBar.svelte';
  import WorkItemFilterPanel from '../features/items/WorkItemFilterPanel.svelte';
  import { createWorkItemSearchStore } from '../stores/searchStore.svelte.js';
  import { itemUrl } from '../utils/urls.js';
  import { navigate } from '../router.js';
  import { createWorkItemSearchHandlers } from '../composables/useWorkItemSearch.svelte.js';
  import { buildWorkItemColumns, updatedAtColumn } from '../utils/workItemColumns.js';
  import {
    decorateWorkItems,
    createDeleteItemHandler,
    createItemActionsBuilder,
    createSearchPaginationHandlers,
  } from '../utils/workItemTableHelpers.js';

  const store = createWorkItemSearchStore();
  /** @type {Record<string, any>} */
  let storeState = $state({});
  store.subscribe((value) => (storeState = value));

  let workspaces = $derived(storeState.workspaces ?? []);
  let allStatuses = $derived(storeState.allStatuses ?? []);
  let allPriorities = $derived(storeState.allPriorities ?? []);
  let statusCategories = $derived(storeState.statusCategories ?? []);
  let selectedWorkspaces = $derived(storeState.selectedWorkspaces ?? []);
  let selectedStatuses = $derived(storeState.selectedStatuses ?? []);
  let selectedPriorities = $derived(storeState.selectedPriorities ?? []);
  let searchQuery = $derived(storeState.searchQuery ?? '');
  let dynamicFilters = $derived(storeState.dynamicFilters ?? []);
  let rawMode = $derived(storeState.rawMode ?? false);
  let qlQuery = $derived(storeState.qlQuery ?? '');
  let qlError = $derived(storeState.qlError ?? null);
  let workItems = $derived(storeState.workItems ?? []);
  let loadingItems = $derived(storeState.loadingItems ?? false);
  let itemsPagination = $derived(storeState.pagination ?? null);
  let hasFilters = $derived(storeState.hasFilters ?? false);

  let currentPage = $state(1);
  let itemsPerPage = $state(/** @type {number} */ (50));
  let explicitSearchVersion = 0;

  onMount(() => {
    async function restoreAndSearch() {
      currentPage = 1;
      store.restoreFromURL();
      await store.executeSearch({ page: 1, limit: itemsPerPage });
    }

    function handleHistoryNavigation() {
      void restoreAndSearch();
    }

    window.addEventListener('popstate', handleHistoryNavigation);
    // Hydrate synchronously before yielding to reference-data requests. This
    // prevents a fast first keystroke from being overwritten when those
    // requests finish, while executeSearch still waits for workspace names.
    store.restoreFromURL();
    const startupSearchVersion = explicitSearchVersion;
    void store.loadReferenceData().then(() => {
      if (explicitSearchVersion !== startupSearchVersion) return;
      return store.executeSearch({ page: 1, limit: itemsPerPage });
    });

    return () => window.removeEventListener('popstate', handleHistoryNavigation);
  });

  function retrySearch() {
    explicitSearchVersion += 1;
    return store.executeSearch({ page: currentPage, limit: itemsPerPage });
  }

  const {
    handleUpdateWorkspaces,
    handleUpdateStatuses,
    handleUpdatePriorities,
    handleUpdateSearch,
    handleUpdateDynamicFilters,
    handleExecuteQL: executeSearch,
    handleEnterRawMode,
    handleResetToBuilder: resetToBuilder,
    handleQueryChange,
  } = createWorkItemSearchHandlers(store, {
    getRawMode: () => rawMode,
    getItemsPerPage: () => itemsPerPage,
    onPageReset: () => {
      currentPage = 1;
    },
  });

  async function handleExecuteQL() {
    explicitSearchVersion += 1;
    await executeSearch();
  }

  async function handleResetToBuilder() {
    explicitSearchVersion += 1;
    await resetToBuilder();
  }

  let workItemColumns = $derived(
    buildWorkItemColumns({
      itemUrl: (item) => itemUrl({ workspaceId: item.workspace_id, itemId: item.id }),
      lastColumn: updatedAtColumn(t('common.updated')),
      allStatuses,
      statusCategories,
    })
  );

  let tableData = $derived(decorateWorkItems(workItems, workspaces));

  function viewItem(item) {
    navigate(itemUrl({ workspaceId: item.workspace_id, itemId: item.id }));
  }

  const deleteItem = createDeleteItemHandler({
    confirmMessage: (item) => t('dialogs.confirmations.deleteItem', { name: item.title }),
    onDeleted: () => store.executeSearch({ page: currentPage, limit: itemsPerPage }),
  });

  const buildItemActions = createItemActionsBuilder({
    viewItem,
    deleteItem,
    viewTitleKey: 'common.viewDetails',
  });

  const { handlePageChange, handlePageSizeChange } = createSearchPaginationHandlers(store, {
    setPage: (page) => (currentPage = page),
    setItemsPerPage: (size) => (itemsPerPage = size),
  });
</script>

<div data-testid="global-search-page" class="min-h-screen" style="background-color: var(--ds-surface);">
  <div class="p-6">
    <PageHeader icon={Search} title={t('search.title')} subtitle={t('search.subtitle')} />

    <div class="mb-6 p-4 rounded-lg border" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
      <QlQueryBar
        query={qlQuery}
        mode={rawMode ? 'raw' : 'builder'}
        error={qlError}
        onenterrawmode={handleEnterRawMode}
        onreset={handleResetToBuilder}
        onexecute={handleExecuteQL}
        onquerychange={handleQueryChange}
      />

      <WorkItemFilterPanel
        testIdPrefix="global-search"
        {workspaces}
        {allStatuses}
        {allPriorities}
        {selectedWorkspaces}
        {selectedStatuses}
        {selectedPriorities}
        {searchQuery}
        {dynamicFilters}
        disabled={rawMode}
        searchInputMode="inline"
        onupdateworkspaces={handleUpdateWorkspaces}
        onupdatestatuses={handleUpdateStatuses}
        onupdatepriorities={handleUpdatePriorities}
        onupdatesearch={handleUpdateSearch}
        onupdatedynamicfilters={handleUpdateDynamicFilters}
        onexecutesearch={handleExecuteQL}
      />
    </div>

    {#if loadingItems}
      <div data-testid="global-search-loading">
        <Card rounded="xl" shadow padding="loose" class="text-center">
        <div class="animate-pulse" style="color: var(--ds-text-subtle);">{t('common.loading')}</div>
        </Card>
      </div>
    {:else if qlError}
      <div data-testid="global-search-error">
        <Card rounded="xl" shadow padding="loose">
          <div class="flex items-start gap-3">
            <AlertCircle class="w-5 h-5 flex-shrink-0 mt-0.5" style="color: var(--ds-text-danger);" />
            <div class="min-w-0 flex-1">
              <p class="font-medium" style="color: var(--ds-text-danger);">{t('common.error')}</p>
              <p class="mt-1 text-sm break-words" style="color: var(--ds-text-subtle);">{qlError}</p>
              <Button
                dataTestid="global-search-retry"
                variant="secondary"
                size="sm"
                onclick={retrySearch}
                class="mt-3"
              >
                {t('common.retry')}
              </Button>
            </div>
          </div>
        </Card>
      </div>
    {:else if workItems.length === 0 && hasFilters}
      <div data-testid="global-search-empty-results">
        <Card rounded="xl" shadow>
          <EmptyState
            icon={Search}
            title={t('search.noSearchResults')}
            description={t('search.configureFilter')}
          />
        </Card>
      </div>
    {:else if workItems.length === 0}
      <div data-testid="global-search-prompt">
        <Card rounded="xl" shadow>
          <EmptyState
            icon={Search}
            title={t('search.title')}
            description={t('search.searchPlaceholder')}
          />
        </Card>
      </div>
    {:else}
      <div data-testid="global-search-results">
        <DataTable
          data={tableData}
          columns={workItemColumns}
          keyField="id"
          rowAttrs={(item) => ({ 'data-testid': `global-search-result-${item.id}` })}
          emptyMessage={t('search.noSearchResults')}
          emptyDescription={t('search.configureFilter')}
          emptyIcon={Search}
          actionItems={buildItemActions}
          onRowClick={viewItem}
        />
      </div>

      {#if itemsPagination && itemsPagination.total_items > 0}
        <div data-testid="global-search-pagination" class="mt-6">
          <Pagination
            currentPage={itemsPagination.page}
            totalItems={itemsPagination.total_items}
            itemsPerPage={itemsPagination.page_size}
            maxItems={10000}
            onpageChange={handlePageChange}
            onpageSizeChange={handlePageSizeChange}
          />
        </div>
      {/if}
    {/if}
  </div>
</div>
