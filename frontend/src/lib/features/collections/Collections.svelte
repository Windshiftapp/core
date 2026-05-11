<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { navigate } from '../../router.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast, warningToast } from '../../stores/toasts.svelte.js';
  import { confirm } from '../../composables/useConfirm.js';
  import { IconFilter as Filter, IconSearch as Search, IconTrash as Trash2, IconEye as Eye } from '@tabler/icons-svelte-runes';
  import Button from '../../components/Button.svelte';
  import Card from '../../components/Card.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import Pagination from '../../components/Pagination.svelte';
  import CollectionsSidebar from './CollectionsSidebar.svelte';
  import CollectionsBreadcrumbs from './CollectionsBreadcrumbs.svelte';
  import QlQueryBar from '../shared/QlQueryBar.svelte';
  import { getStatusColor as getStatusColorUtil, getStatusStyle } from '../../utils/statusColors.js';
  import { createWorkItemSearchStore } from '../../stores/searchStore.svelte.js';
  import { createWorkItemSearchHandlers } from '../../composables/useWorkItemSearch.svelte.js';
  import { buildWorkItemColumns, createdAtColumn } from '../../utils/workItemColumns.js';
  import Modal from '../../dialogs/Modal.svelte';
  import WorkspacePicker from '../../pickers/WorkspacePicker.svelte';
  import { collectionCategoriesStore } from '../../stores/collectionCategories.js';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';

  let { collectionId = null } = $props();

  // Each Collections instance owns a fresh search store — no cross-page leakage.
  const store = createWorkItemSearchStore();
  /** @type {Record<string, any>} */
  let storeState = $state({});
  store.subscribe((value) => (storeState = value));

  // Reactive views into the store
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

  // Collection-editor specific state
  let currentCollection = $state(null);
  let loading = $state(true);
  let currentPage = $state(1);
  let itemsPerPage = $state(/** @type {number} */ (50));
  let sidebarCollapsed = $state(false);

  // Workspace / sharing modals
  let returnWorkspaceId = $state(null);
  let returnPath = $state(null);
  let slugSaved = $state(false);
  let savingPublicSharing = $state(false);
  let showWorkspaceAssociationModal = $state(false);
  let workspaceAssociationSelection = $state([]);
  let workspaceAssociationError = $state(null);
  let workspaceAssociationSaving = $state(false);

  onMount(async () => {
    await store.loadReferenceData();
    await collectionCategoriesStore.init();

    const urlParams = new URLSearchParams(window.location.search);
    const wsParam = urlParams.get('workspace');
    if (wsParam) returnWorkspaceId = wsParam;

    const loadCollectionId = collectionId || urlParams.get('load');
    if (returnWorkspaceId && loadCollectionId) {
      returnPath = `/workspaces/${returnWorkspaceId}/collections/${loadCollectionId}`;
    } else if (returnWorkspaceId) {
      returnPath = `/workspaces/${returnWorkspaceId}`;
    }

    if (loadCollectionId) {
      await loadCollectionById(loadCollectionId);
    } else {
      store.restoreFromURL();
      if (storeState.hasFilters) {
        await store.executeSearch({ page: currentPage, limit: itemsPerPage });
      }
    }

    loading = false;
  });

  async function loadCollectionById(id) {
    try {
      const collection = await api.collections.get(id);
      if (!collection) return;
      currentCollection = collection;
      slugSaved = !!(collection.is_public && collection.public_slug);
      hydrateFromCollection(collection);
      await store.executeSearch({ page: 1, limit: itemsPerPage });

      const url = new URL(window.location.href);
      url.searchParams.delete('load');
      window.history.replaceState({}, '', url);
    } catch (error) {
      console.error('Failed to load collection:', error);
    }
  }

  function hydrateFromCollection(collection) {
    const storedQl = collection.ql_query || '';
    const filterState = parseFilterState(collection.filter_state);

    if (filterState) {
      store.hydrate({
        workspaces: Array.isArray(filterState.workspaces) ? filterState.workspaces : [],
        statuses: Array.isArray(filterState.statuses) ? filterState.statuses : [],
        priorities: Array.isArray(filterState.priorities) ? filterState.priorities : [],
        search: filterState.search || '',
        dynamicFields: Array.isArray(filterState.dynamicFields) ? filterState.dynamicFields : [],
      });
    } else if (storedQl.trim()) {
      // Legacy collection with QL but no persisted builder state → raw mode.
      store.hydrate({ rawQl: storedQl });
    } else {
      store.hydrate({});
    }
  }

  function parseFilterState(value) {
    if (!value) return null;
    try {
      const parsed = typeof value === 'string' ? JSON.parse(value) : value;
      return parsed && typeof parsed === 'object' ? parsed : null;
    } catch (err) {
      console.warn('Failed to parse filter_state, falling back to raw mode:', err);
      return null;
    }
  }

  function serializeFilterState() {
    return JSON.stringify({
      workspaces: selectedWorkspaces,
      statuses: selectedStatuses,
      priorities: selectedPriorities,
      search: searchQuery,
      dynamicFields: dynamicFilters,
    });
  }

  // ===== Filter event handlers =====

  const {
    handleUpdateWorkspaces,
    handleUpdateStatuses,
    handleUpdatePriorities,
    handleUpdateSearch,
    handleUpdateDynamicFilters,
    handleExecuteQL,
    handleEnterRawMode,
    handleResetToBuilder,
    handleQueryChange,
  } = createWorkItemSearchHandlers(store, {
    getRawMode: () => rawMode,
    getItemsPerPage: () => itemsPerPage,
    onPageReset: () => {
      currentPage = 1;
    },
  });

  // ===== Table data =====

  function getWorkspaceName(workspaceId) {
    return workspaces.find((w) => w.id === workspaceId)?.name || 'Unknown';
  }

  function getWorkspaceKey(workspaceId) {
    return workspaces.find((w) => w.id === workspaceId)?.key || 'WORK';
  }

  let workItemColumns = $derived(
    buildWorkItemColumns({
      itemUrl: (item) => `/workspaces/${item.workspace_id}/items/${item.id}`,
      lastColumn: createdAtColumn(),
      allStatuses,
      statusCategories,
    })
  );

  let tableData = $derived(
    workItems.map((item) => ({
      ...item,
      display_key: `${getWorkspaceKey(item.workspace_id)}-${item.id}`,
      workspace_name: getWorkspaceName(item.workspace_id),
    }))
  );

  function viewItem(item) {
    navigate(`/workspaces/${item.workspace_id}/items/${item.id}`);
  }

  async function deleteItem(item) {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('collections.confirmDeleteItem', { title: item.title }),
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
      { id: 'view', type: 'regular', icon: Eye, title: t('items.viewItem'), onClick: () => viewItem(item) },
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

  // ===== Public sharing =====

  async function handlePublicToggle() {
    if (!currentCollection) return;
    const newIsPublic = !currentCollection.is_public;
    if (!newIsPublic) {
      savingPublicSharing = true;
      try {
        await api.collections.updatePublicSharing(currentCollection.id, {
          is_public: false,
          public_slug: currentCollection.public_slug || null,
        });
        currentCollection = { ...currentCollection, is_public: false };
        slugSaved = false;
      } catch (error) {
        console.error('Failed to disable public sharing:', error);
      } finally {
        savingPublicSharing = false;
      }
    } else {
      currentCollection = { ...currentCollection, is_public: true };
      slugSaved = false;
    }
  }

  function handleSlugChange(value) {
    if (!currentCollection) return;
    currentCollection = { ...currentCollection, public_slug: value };
    slugSaved = false;
  }

  async function handleSlugSave() {
    if (!currentCollection || !currentCollection.public_slug) return;
    savingPublicSharing = true;
    try {
      await api.collections.updatePublicSharing(currentCollection.id, {
        is_public: currentCollection.is_public,
        public_slug: currentCollection.public_slug,
      });
      slugSaved = true;
    } catch (error) {
      console.error('Failed to save public sharing:', error);
      errorToast(error.message || 'Failed to save public sharing settings');
    } finally {
      savingPublicSharing = false;
    }
  }

  // ===== Save / associate workspace =====

  async function updateCollectionDirectly() {
    if (!currentCollection) return;
    if (!qlQuery.trim()) {
      warningToast(t('collections.noQueryToSave'));
      return;
    }
    try {
      await api.collections.update(currentCollection.id, {
        name: currentCollection.name,
        description: currentCollection.description || null,
        ql_query: qlQuery,
        filter_state: rawMode ? null : serializeFilterState(),
        workspace_id: currentCollection.workspace_id ?? null,
        category_id: currentCollection.category_id ?? null,
      });
      navigate(returnPath || '/collections');
    } catch (error) {
      console.error('Failed to update collection:', error);
      errorToast(t('dialogs.alerts.failedToUpdate', { error: error.message || error }));
    }
  }

  function openAssociateWorkspaceModal() {
    if (!currentCollection) return;
    workspaceAssociationSelection = currentCollection.workspace_id ? [currentCollection.workspace_id] : [];
    workspaceAssociationError = null;
    showWorkspaceAssociationModal = true;
  }

  function closeAssociateWorkspaceModal() {
    showWorkspaceAssociationModal = false;
  }

  async function handleAssociateWorkspaceSave() {
    if (!currentCollection) return;
    workspaceAssociationError = null;
    workspaceAssociationSaving = true;
    const workspaceId =
      workspaceAssociationSelection.length === 1 ? workspaceAssociationSelection[0] : null;

    try {
      await api.collections.update(currentCollection.id, {
        name: currentCollection.name,
        description: currentCollection.description || null,
        ql_query: qlQuery,
        filter_state: rawMode ? null : serializeFilterState(),
        is_public: currentCollection.is_public,
        workspace_id: workspaceId,
      });
      currentCollection = { ...currentCollection, workspace_id: workspaceId };
      showWorkspaceAssociationModal = false;
    } catch (error) {
      console.error('Failed to associate workspace:', error);
      workspaceAssociationError = error.message || 'Failed to associate workspace. Please try again.';
    } finally {
      workspaceAssociationSaving = false;
    }
  }

  let trimmedCollectionName = $derived((currentCollection?.name || '').trim());
  let trimmedQlQuery = $derived(qlQuery.trim());
  let canSubmitCollection = $derived(Boolean(currentCollection && trimmedCollectionName && trimmedQlQuery));
  let associatedWorkspace = $derived(
    currentCollection?.workspace_id ? workspaces.find((w) => w.id === currentCollection.workspace_id) : null
  );

  $effect(() => {
    if (workspaceAssociationSelection.length > 1) {
      workspaceAssociationSelection = [
        workspaceAssociationSelection[workspaceAssociationSelection.length - 1],
      ];
    }
  });
</script>

<div class="min-h-screen flex" style="background-color: var(--ds-surface);">
  <CollectionsSidebar
    bind:collapsed={sidebarCollapsed}
    {workspaces}
    {allStatuses}
    {allPriorities}
    {selectedWorkspaces}
    {selectedStatuses}
    {selectedPriorities}
    {searchQuery}
    {dynamicFilters}
    disabled={rawMode}
    onupdateworkspaces={handleUpdateWorkspaces}
    onupdatestatuses={handleUpdateStatuses}
    onupdatepriorities={handleUpdatePriorities}
    onupdatesearch={handleUpdateSearch}
    onupdatedynamicfilters={handleUpdateDynamicFilters}
    onexecutesearch={handleExecuteQL}
  />

  <div class="flex-1 p-6 overflow-auto">
    <CollectionsBreadcrumbs
      collection={currentCollection}
      workspace={associatedWorkspace}
      isEditing={!!currentCollection}
      canSave={canSubmitCollection}
      categories={$collectionCategoriesStore}
      {returnPath}
      onsave={updateCollectionDirectly}
      onassociateworkspace={openAssociateWorkspaceModal}
      onnamechange={(value) => {
        if (currentCollection) currentCollection.name = value;
      }}
      ondescriptionchange={(value) => {
        if (currentCollection) currentCollection.description = value;
      }}
      oncategorychange={(value) => {
        if (currentCollection) currentCollection = { ...currentCollection, category_id: value };
      }}
      showPublicBoard={!!currentCollection}
      isPublic={currentCollection?.is_public || false}
      publicSlug={currentCollection?.public_slug || null}
      {slugSaved}
      saving={savingPublicSharing}
      onpublictoggle={handlePublicToggle}
      onslugchange={handleSlugChange}
      onslugsave={handleSlugSave}
    />

    <QlQueryBar
      query={qlQuery}
      mode={rawMode ? 'raw' : 'builder'}
      error={qlError}
      onenterrawmode={handleEnterRawMode}
      onreset={handleResetToBuilder}
      onexecute={handleExecuteQL}
      onquerychange={handleQueryChange}
    />

    {#if loading}
      <Card rounded="xl" shadow padding="loose" class="text-center">
        <div class="animate-pulse" style="color: var(--ds-text-subtle);">{t('collections.loadingWorkspaces')}</div>
      </Card>
    {:else if !qlQuery.trim() && dynamicFilters.length === 0 && !currentCollection}
      <Card rounded="xl" shadow padding="generous" class="text-center">
        <Filter class="w-12 h-12 mx-auto mb-4" style="color: var(--ds-icon-subtle);" />
        <h3 class="text-lg font-medium mb-2" style="color: var(--ds-text);">{t('collections.addFiltersToStart')}</h3>
        <p style="color: var(--ds-text-subtle);">{t('collections.addFiltersDesc')}</p>
      </Card>
    {:else if loadingItems}
      <Card rounded="xl" shadow padding="loose" class="text-center">
        <div class="animate-pulse" style="color: var(--ds-text-subtle);">{t('collections.loadingWorkItems')}</div>
      </Card>
    {:else}
      <DataTable
        data={tableData}
        columns={workItemColumns}
        keyField="id"
        emptyMessage={t('collections.noWorkItemsFound')}
        emptyDescription={t('collections.tryAdjustingFilters')}
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
      {:else}
        <div class="mt-4 text-sm text-center" style="color: var(--ds-text-subtle);">
          {t('collections.showingWorkItems', { count: workItems.length })}
        </div>
      {/if}
    {/if}
  </div>
</div>

<Modal isOpen={showWorkspaceAssociationModal} onclose={closeAssociateWorkspaceModal} maxWidth="max-w-2xl">
  <div>
    <div class="px-8 py-6 border-b" style="border-color: var(--ds-border);">
      <h2 class="text-xl font-semibold" style="color: var(--ds-text);">
        {associatedWorkspace ? t('collections.changeWorkspaceAssociation') : t('collections.associateWithWorkspace')}
      </h2>
      <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
        {t('collections.workspaceAssociationDesc')}
      </p>
    </div>
    <div class="px-8 py-6 space-y-4">
      <WorkspacePicker
        bind:value={workspaceAssociationSelection}
        label={t('workspaces.workspace')}
        placeholder={t('collections.searchWorkspace')}
      />
      {#if workspaceAssociationError}
        <div class="text-sm" style="color: var(--ds-text-danger);">{workspaceAssociationError}</div>
      {/if}
      <p class="text-xs" style="color: var(--ds-text-subtle);">
        {t('collections.workspaceAssociationNote')}
      </p>
    </div>
    <DialogFooter
      onCancel={closeAssociateWorkspaceModal}
      onConfirm={handleAssociateWorkspaceSave}
      confirmLabel={t('collections.saveAssociation')}
      disabled={workspaceAssociationSaving}
      loading={workspaceAssociationSaving}
    />
  </div>
</Modal>

<style>
  .cancel-btn:hover {
    background-color: var(--ds-background-neutral-hovered);
  }
</style>
