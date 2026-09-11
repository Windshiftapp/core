<script>
  import { onMount, onDestroy } from 'svelte';
  import { api } from '../../api.js';
  import { navigate } from '../../router.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast, warningToast } from '../../stores/toasts.svelte.js';
  import { IconFilter as Filter, IconSearch as Search } from '@tabler/icons-svelte-runes';
  import Card from '../../components/Card.svelte';
  import Pagination from '../../components/Pagination.svelte';
  import CollectionsSidebar from './CollectionsSidebar.svelte';
  import CollectionsBreadcrumbs from './CollectionsBreadcrumbs.svelte';
  import QlQueryBar from '../shared/QlQueryBar.svelte';
  import { createWorkItemSearchStore } from '../../stores/searchStore.svelte.js';
  import { createWorkItemSearchHandlers } from '../../composables/useWorkItemSearch.svelte.js';
  import {
    decorateWorkItems,
    createDeleteItemHandler,
    createItemActionsBuilder,
    createSearchPaginationHandlers,
  } from '../../utils/workItemTableHelpers.js';
  import Modal from '../../dialogs/Modal.svelte';
  import WorkspacePicker from '../../pickers/WorkspacePicker.svelte';
  import { collectionCategoriesStore } from '../../stores/collectionCategories.js';
  import { permissionStore, isSystemAdmin, authStore, workspaceDataStore } from '../../stores';
  import ColumnSelector from './ColumnSelector.svelte';
  import WorkItemListTable from './WorkItemListTable.svelte';
  import {
    buildListColumnConfiguration,
    DEFAULT_LIST_COLUMNS,
    listColumnsFromConfig,
  } from '../../utils/workItemListColumns.js';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';

  let { collectionId = null } = $props();

  // Each Collections instance owns a fresh search store — no cross-page leakage.
  const store = createWorkItemSearchStore({ allowEmptyQuery: true });
  /** @type {Record<string, any>} */
  let storeState = $state({});
  const unsubscribeStore = store.subscribe((value) => (storeState = value));
  onDestroy(unsubscribeStore);

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
  let boardConfig = $state(null);
  let listColumns = $state([...DEFAULT_LIST_COLUMNS]);

  let itemTypes = $derived(workspaceDataStore.itemTypes);
  let listStatuses = $derived(workspaceDataStore.statuses);
  let listStatusCategories = $derived(workspaceDataStore.statusCategories);
  let listPriorities = $derived(workspaceDataStore.priorities);
  let customFieldDefinitions = $derived(workspaceDataStore.customFieldDefinitions);
  let canConfigureColumns = $derived(
    Boolean(
      currentCollection &&
        authStore.currentUser?.id != null &&
        String(currentCollection.created_by) === String(authStore.currentUser.id),
    ),
  );

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
    await Promise.all([
      store.loadReferenceData(),
      collectionCategoriesStore.init(),
      workspaceDataStore.initializeGlobal(),
    ]);

    const urlParams = new URLSearchParams(window.location.search);
    const hasURLQuery = [
      'raw',
      'ql',
      'workspaces',
      'statuses',
      'priorities',
      'search',
      'dynamicFilters',
    ].some((key) => urlParams.has(key));
    const wsParam = urlParams.get('workspace');
    if (wsParam) returnWorkspaceId = wsParam;

    const loadCollectionId = collectionId || urlParams.get('load');
    if (returnWorkspaceId && loadCollectionId) {
      returnPath = `/workspaces/${returnWorkspaceId}/collections/${loadCollectionId}`;
    } else if (returnWorkspaceId) {
      returnPath = `/workspaces/${returnWorkspaceId}`;
    }

    if (loadCollectionId) {
      await loadCollectionById(loadCollectionId, hasURLQuery);
    } else {
      boardConfig = null;
      listColumns = [...DEFAULT_LIST_COLUMNS];
      store.restoreFromURL();
      if (storeState.hasFilters) {
        await store.executeSearch({ page: currentPage, limit: itemsPerPage });
      }
    }

    loading = false;
  });

  async function loadCollectionById(id, restoreURLQuery = false) {
    try {
      const collection = await api.collections.get(id);
      if (!collection) return;
      currentCollection = collection;
      slugSaved = !!(collection.is_public && collection.public_slug);
      await loadBoardConfiguration(collection.id);
      if (restoreURLQuery) {
        store.restoreFromURL();
      } else {
        await hydrateFromCollection(collection);
      }
      await store.executeSearch({ page: 1, limit: itemsPerPage });

      const url = new URL(window.location.href);
      url.searchParams.delete('load');
      window.history.replaceState({}, '', url);
    } catch (error) {
      console.error('Failed to load collection:', error);
    }
  }

  async function loadBoardConfiguration(id) {
    try {
      const bootstrap = await api.collections.getBoardConfigurationBootstrap(id);
      const config = bootstrap?.board_configuration ?? null;
      boardConfig = config;
      listColumns = listColumnsFromConfig(config);
    } catch (error) {
      boardConfig = null;
      listColumns = [...DEFAULT_LIST_COLUMNS];
      if (error?.status !== 404) {
        console.error('Failed to load list column configuration:', error);
      }
    }
  }

  async function saveBoardConfiguration(newColumns) {
    if (!currentCollection) return;

    try {
      const configData = buildListColumnConfiguration(boardConfig, newColumns);
      const savedConfig = boardConfig?.id
        ? await api.collections.updateBoardConfiguration(
            currentCollection.id,
            boardConfig.id,
            configData,
          )
        : await api.collections.createBoardConfiguration(
            currentCollection.id,
            null,
            configData,
          );
      boardConfig = savedConfig;
      listColumns = listColumnsFromConfig(savedConfig);
    } catch (error) {
      console.error('Failed to save list column configuration:', error);
      errorToast(t('dialogs.alerts.failedToSave', { error: error.message || error }));
    }
  }

  function handleColumnChange({ columns }) {
    void saveBoardConfiguration(columns);
  }

  async function hydrateFromCollection(collection) {
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
      // Legacy collection with QL but no persisted builder state. Try to
      // recover into builder fields first (WI-6); tryHydrateFromQl falls back
      // to raw mode internally when the parser would drop clauses, so the
      // user never loses content.
      store.hydrate({});
      await store.tryHydrateFromQl(storedQl);
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

  let tableData = $derived(decorateWorkItems(workItems, workspaces));

  function viewItem(item) {
    navigate(`/workspaces/${item.workspace_id}/items/${item.id}`);
  }

  const deleteItem = createDeleteItemHandler({
    confirmMessage: (item) => t('collections.confirmDeleteItem', { title: item.title }),
    onDeleted: () => store.executeSearch({ page: currentPage, limit: itemsPerPage }),
  });

  const buildItemActions = createItemActionsBuilder({ viewItem, deleteItem });

  const { handlePageChange, handlePageSizeChange } = createSearchPaginationHandlers(store, {
    setPage: (page) => (currentPage = page),
    setItemsPerPage: (size) => (itemsPerPage = size),
  });

  // ===== Public sharing =====

  function buildCollectionUpdate(overrides = {}) {
    return {
      name: currentCollection.name,
      description: currentCollection.description || null,
      ql_query: qlQuery,
      filter_state: rawMode ? null : serializeFilterState(),
      workspace_id: currentCollection.workspace_id ?? null,
      category_id: currentCollection.category_id ?? null,
      ...overrides,
    };
  }

  async function handlePublicSharingSave({ isPublic, publicSlug }) {
    if (!currentCollection) return { ok: false, error: 'Collection not found.' };
    savingPublicSharing = true;
    try {
      const update = buildCollectionUpdate({
        is_public: isPublic,
        public_slug: publicSlug,
      });
      await api.collections.update(currentCollection.id, update);
      currentCollection = {
        ...currentCollection,
        ql_query: update.ql_query,
        filter_state: update.filter_state,
        is_public: isPublic,
        public_slug: publicSlug,
      };
      slugSaved = Boolean(isPublic && publicSlug);
      return { ok: true };
    } catch (error) {
      console.error('Failed to save public sharing:', error);
      errorToast(error.message || 'Failed to save public sharing settings');
      return { ok: false, error: error.message || 'Failed to save public sharing settings.' };
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
      await api.collections.update(currentCollection.id, buildCollectionUpdate());
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
  // Public-board sharing is gated on the global public_board.manage permission
  // AND ownership of this collection (system admins may manage any). Mirrors the
  // backend: publishing requires the permission, and editing a collection's
  // public state requires being its creator (requireCollectionOwner).
  let canManagePublicBoard = $derived(
    Boolean(currentCollection) &&
      ($isSystemAdmin ||
        (($permissionStore.userPermissionKeys?.has('public_board.manage') ?? false) &&
          currentCollection.created_by === authStore.currentUser?.id))
  );
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
    {loading}
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
      showPublicBoard={canManagePublicBoard}
      isPublic={currentCollection?.is_public || false}
      publicSlug={currentCollection?.public_slug || null}
      {slugSaved}
      saving={savingPublicSharing}
      onpublicsave={handlePublicSharingSave}
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
      {#if currentCollection}
        <div class="flex justify-end mb-4">
          <ColumnSelector
            columns={listColumns}
            {customFieldDefinitions}
            canConfigure={canConfigureColumns}
            testId="collection-column-selector-trigger"
            onchange={handleColumnChange}
          />
        </div>
      {/if}

      <WorkItemListTable
        data={tableData}
        columns={listColumns}
        {workspaces}
        {customFieldDefinitions}
        {itemTypes}
        includeWorkspace={true}
        statuses={listStatuses.length > 0 ? listStatuses : allStatuses}
        statusCategories={listStatusCategories.length > 0 ? listStatusCategories : statusCategories}
        priorities={listPriorities.length > 0 ? listPriorities : allPriorities}
        collectionId={null}
        canEdit={false}
        testId="collection-search-results-table"
        emptyMessage={t('collections.noWorkItemsFound')}
        emptyDescription={t('collections.tryAdjustingFilters')}
        emptyIcon={Search}
        actionItems={buildItemActions}
        onRowClick={viewItem}
        rowAttrs={(item) => ({ 'data-testid': `collection-result-${item.id}` })}
      />

      {#if itemsPagination && itemsPagination.total_items > 0}
        <div class="mt-6">
          <Pagination
            currentPage={itemsPagination.page}
            totalItems={itemsPagination.total_items}
            itemsPerPage={itemsPagination.page_size}
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
