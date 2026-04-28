<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { Search, Eye, Trash2 } from 'lucide-svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import { escapeHtml } from '../utils/sanitize.ts';
  import PageHeader from '../layout/PageHeader.svelte';
  import Card from '../components/Card.svelte';
  import DataTable from '../components/DataTable.svelte';
  import Pagination from '../components/Pagination.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import QlQueryBar from '../features/shared/QlQueryBar.svelte';
  import WorkItemFilterPanel from '../features/items/WorkItemFilterPanel.svelte';
  import { createWorkItemSearchStore } from '../stores/searchStore.svelte.js';
  import { getStatusInlineStyle } from '../utils/statusColors.js';
  import { formatDate } from '../utils/dateFormatter.js';
  import { itemUrl } from '../utils/urls.js';
  import { navigate } from '../router.js';

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
  let itemsPerPage = $state(50);

  onMount(async () => {
    await store.loadReferenceData();
    store.restoreFromURL();
    if (storeState.hasFilters) {
      await store.executeSearch({ page: currentPage, limit: itemsPerPage });
    }
  });

  function handleUpdateWorkspaces(value) {
    if (rawMode) return;
    store.setSelectedWorkspaces(value);
  }

  function handleUpdateStatuses(value) {
    if (rawMode) return;
    store.setSelectedStatuses((value || []).map((v) => Number(v)).filter((id) => !Number.isNaN(id)));
  }

  function handleUpdatePriorities(value) {
    if (rawMode) return;
    store.setSelectedPriorities((value || []).map((v) => Number(v)).filter((id) => !Number.isNaN(id)));
  }

  function handleUpdateSearch(value) {
    if (rawMode) return;
    store.setSearchQuery(value);
  }

  function handleUpdateDynamicFilters(value) {
    if (rawMode) return;
    store.setDynamicFilters(value);
  }

  async function handleExecuteQL() {
    store.syncToURL();
    await store.executeSearch({ page: 1, limit: itemsPerPage });
    currentPage = 1;
  }

  async function handleEnterRawMode() {
    await store.enterRawMode();
    store.syncToURL();
  }

  async function handleResetToBuilder() {
    await store.resetToBuilder();
    store.syncToURL();
    await store.executeSearch({ page: 1, limit: itemsPerPage });
    currentPage = 1;
  }

  function handleQueryChange(value) {
    store.setRawQlQuery(value);
  }

  function getWorkspaceName(workspaceId) {
    return workspaces.find((w) => w.id === workspaceId)?.name || 'Unknown';
  }

  function getWorkspaceKey(workspaceId) {
    return workspaces.find((w) => w.id === workspaceId)?.key || 'WORK';
  }

  let workItemColumns = $derived([
    {
      key: 'display_key',
      label: 'Key',
      width: 'w-28',
      html: true,
      render: (item) =>
        `<a href="${itemUrl({ workspaceId: item.workspace_id, itemId: item.id })}" class="text-xs font-mono px-1.5 py-0.5 rounded whitespace-nowrap no-underline" style="color: var(--ds-text-subtle); background-color: var(--ds-interactive-subtle);">${escapeHtml(item.display_key)}</a>`,
    },
    {
      key: 'title',
      label: 'Title',
      html: true,
      render: (item) =>
        `<a href="${itemUrl({ workspaceId: item.workspace_id, itemId: item.id })}" class="block truncate text-sm no-underline" style="color: inherit;" title="${escapeHtml(item.title)}">${escapeHtml(item.title) || '—'}</a>`,
    },
    {
      key: 'workspace_name',
      label: 'Workspace',
      width: 'w-36',
      html: true,
      render: (item) =>
        `<span class="block truncate" title="${escapeHtml(item.workspace_name)}">${escapeHtml(item.workspace_name) || '—'}</span>`,
    },
    {
      key: 'status_name',
      label: 'Status',
      width: 'w-28',
      html: true,
      render: (item) =>
        item.status_name
          ? `<span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium whitespace-nowrap" style="${getStatusInlineStyle(item.status_name, allStatuses, statusCategories)}">${escapeHtml(item.status_name)}</span>`
          : '—',
    },
    {
      key: 'priority_name',
      label: 'Priority',
      width: 'w-24',
      html: true,
      render: (item) =>
        item.priority_name
          ? `<span class="text-sm font-medium capitalize whitespace-nowrap" style="color: ${escapeHtml(item.priority_color) || 'var(--ds-text-subtle)'}">${escapeHtml(item.priority_name)}</span>`
          : '—',
    },
    {
      key: 'updated_at',
      label: t('common.updated'),
      width: 'w-28',
      html: true,
      render: (item) => `<span class="whitespace-nowrap">${formatDate(item.updated_at) || '—'}</span>`,
    },
    { key: 'actions', label: '', width: 'w-12' },
  ]);

  let tableData = $derived(
    workItems.map((item) => ({
      ...item,
      display_key: `${getWorkspaceKey(item.workspace_id)}-${item.id}`,
      workspace_name: getWorkspaceName(item.workspace_id),
    }))
  );

  function viewItem(item) {
    navigate(itemUrl({ workspaceId: item.workspace_id, itemId: item.id }));
  }

  async function deleteItem(item) {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('dialogs.confirmations.deleteItem', { name: item.title }),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger',
    });
    if (!confirmed) return;

    try {
      await api.items.delete(item.id);
      await store.executeSearch({ page: currentPage, limit: itemsPerPage });
    } catch (error) {
      console.error('Failed to delete item:', error);
      errorToast(t('dialogs.alerts.failedToDelete', { error: error.message || error }));
    }
  }

  function buildItemActions(item) {
    return [
      { id: 'view', type: 'regular', icon: Eye, title: t('common.viewDetails'), onClick: () => viewItem(item) },
      { type: 'divider' },
      {
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: t('common.delete'),
        color: 'var(--ds-text-danger)',
        hoverClass: 'hover-danger',
        onClick: () => deleteItem(item),
      },
    ];
  }

  async function handlePageChange(event) {
    currentPage = event.detail.page;
    itemsPerPage = event.detail.itemsPerPage;
    await store.executeSearch({ page: currentPage, limit: itemsPerPage });
  }

  async function handlePageSizeChange(event) {
    currentPage = event.detail.page;
    itemsPerPage = event.detail.itemsPerPage;
    await store.executeSearch({ page: currentPage, limit: itemsPerPage });
  }
</script>

<div class="min-h-screen" style="background-color: var(--ds-surface);">
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
      <Card rounded="xl" shadow padding="loose" class="text-center">
        <div class="animate-pulse" style="color: var(--ds-text-subtle);">{t('common.loading')}</div>
      </Card>
    {:else if workItems.length === 0 && hasFilters}
      <Card rounded="xl" shadow>
        <EmptyState
          icon={Search}
          title={t('search.noSearchResults')}
          description={t('search.configureFilter')}
        />
      </Card>
    {:else if workItems.length === 0}
      <Card rounded="xl" shadow>
        <EmptyState
          icon={Search}
          title={t('search.title')}
          description={t('search.searchPlaceholder')}
        />
      </Card>
    {:else}
      <DataTable
        data={tableData}
        columns={workItemColumns}
        keyField="id"
        emptyMessage={t('search.noSearchResults')}
        emptyDescription={t('search.configureFilter')}
        emptyIcon={Search}
        actionItems={buildItemActions}
        onRowClick={viewItem}
      />

      {#if itemsPagination && itemsPagination.total > 0}
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
      {/if}
    {/if}
  </div>
</div>
