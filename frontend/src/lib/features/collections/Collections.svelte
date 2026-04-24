<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { navigate } from '../../router.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast, warningToast } from '../../stores/toasts.svelte.js';
  import { confirm } from '../../composables/useConfirm.js';
  import { IconFilter as Filter, IconSearch as Search, IconTrash as Trash2, IconEye as Eye } from '@tabler/icons-svelte-runes';
  import { escapeHtml } from '../../utils/sanitize.ts';
  import Button from '../../components/Button.svelte';
  import Card from '../../components/Card.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import Pagination from '../../components/Pagination.svelte';
  import CollectionsSidebar from '../collections/CollectionsSidebar.svelte';
  import CollectionsBreadcrumbs from '../collections/CollectionsBreadcrumbs.svelte';
  import CollectionQueryBar from '../collections/CollectionQueryBar.svelte';
  import { QLBuilder } from '../../utils/ql.js';
  import { getStatusColor as getStatusColorUtil, getStatusInlineStyle, getStatusStyle } from '../../utils/statusColors.js';
  import { formatDate } from '../../utils/dateFormatter.js';
  import { searchStore } from '../../stores/searchStore.svelte.js';
  import Modal from '../../dialogs/Modal.svelte';
  import WorkspacePicker from '../../pickers/WorkspacePicker.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import { collectionCategoriesStore } from '../../stores/collectionCategories.js';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';

  // Props
  let { collectionId = null } = $props(); // When provided, load and edit this collection

  let workspaces = $state([]);
  let currentCollection = $state(null); // Store the loaded collection data
  let selectedWorkspaces = $state([]);
  let statuses = $state([]);
  let selectedStatuses = $state([]);
  let priorities = $state([]);
  let selectedPriorities = $state([]);

  let statusCategories = $state([]);
  let dynamicFilters = $state([]);
  // Items are now managed by searchStore for reactivity
  let itemsPagination = $state(null);
  let loading = $state(true);
  let searchQuery = $state('');
  let currentPage = $state(1);
  let itemsPerPage = $state(50);

  // Subscribe to searchStore for reactive items and loading state
  /** @type {{ searchResults?: any[], loading?: boolean, error?: string }} */
  let storeState = $state({});
  searchStore.subscribe(value => storeState = value);

  // Reactive unpacking for items and loading
  let workItems = $derived(storeState.searchResults || []);
  let loadingItems = $derived(storeState.loading || false);
  let qlErrorFromStore = $derived(storeState.error);

  // QL state
  // `rawMode` is the single source of truth for which editing mode is active.
  // In builder mode, `qlQuery` is derived from the builder-state fields.
  // In raw mode, `rawQlQuery` is the source and `qlQuery` echoes it.
  let rawMode = $state(false);
  let rawQlQuery = $state('');
  let qlError = $state(null);
  let qlQuery = $derived(rawMode
    ? rawQlQuery
    : QLBuilder.buildQuery({
        workspaces: selectedWorkspaces
          .map(id => workspaces.find(w => w.id === id)?.name)
          .filter(Boolean),
        statuses: selectedStatuses,
        priorities: selectedPriorities,
        search: searchQuery,
        dynamicFields: dynamicFilters
      })
  );

  // Sidebar state
  let sidebarCollapsed = $state(false);


  // Workspace association modal state
  // Read workspace return context from query param
  let returnWorkspaceId = $state(null);
  let returnPath = $state(null);

  // Public sharing state
  let slugSaved = $state(false);
  let savingPublicSharing = $state(false);

  let showWorkspaceAssociationModal = $state(false);
  let workspaceAssociationSelection = $state([]);
  let workspaceAssociationError = $state(null);
  let workspaceAssociationSaving = $state(false);

  onMount(async () => {
    await loadWorkspaces();
    await loadStatusesAndCategories();
    await loadPriorities();
    await collectionCategoriesStore.init();

    // Check if we need to load a specific collection from collectionId prop or URL params
    const urlParams = new URLSearchParams(window.location.search);

    // Capture workspace return context before it gets cleared
    const wsParam = urlParams.get('workspace');
    if (wsParam) {
      returnWorkspaceId = wsParam;
    }
    const loadCollectionId = collectionId || urlParams.get('load');
    if (returnWorkspaceId && loadCollectionId) {
      returnPath = `/workspaces/${returnWorkspaceId}/collections/${loadCollectionId}`;
    } else if (returnWorkspaceId) {
      returnPath = `/workspaces/${returnWorkspaceId}`;
    }
    if (loadCollectionId) {
      await loadCollectionById(loadCollectionId);
    } else {
      restoreFromURL();
      if (rawMode || selectedWorkspaces.length > 0 || selectedStatuses.length > 0 || selectedPriorities.length > 0 || searchQuery || dynamicFilters.length > 0) {
        await loadWorkItems(1, itemsPerPage);
      }
    }

    loading = false;
  });

  async function loadCollectionById(collectionId) {
    try {
      searchStore.setAutoSearch(false); // Prevent debounced auto-search from overwriting results

      const collection = await api.collections.get(collectionId);
      if (collection) {
        currentCollection = collection;
        slugSaved = !!(collection.is_public && collection.public_slug);

        hydrateFromCollection(collection);
        syncFiltersToSearchStore();

        await loadWorkItems(1, itemsPerPage);

        const url = new URL(window.location.href);
        url.searchParams.delete('load');
        window.history.replaceState({}, '', url);
      }
    } catch (error) {
      console.error('Failed to load collection:', error);
    } finally {
      searchStore.setAutoSearch(true);
    }
  }

  function hydrateFromCollection(collection) {
    const storedQl = collection.ql_query || '';
    const state = parseFilterState(collection.filter_state);

    // Reset builder state first.
    selectedWorkspaces = [];
    selectedStatuses = [];
    selectedPriorities = [];
    searchQuery = '';
    dynamicFilters = [];
    rawQlQuery = '';

    if (state) {
      // Persisted builder state → builder mode, hydrate directly.
      selectedWorkspaces = Array.isArray(state.workspaces) ? state.workspaces : [];
      selectedStatuses = Array.isArray(state.statuses) ? state.statuses : [];
      selectedPriorities = Array.isArray(state.priorities) ? state.priorities : [];
      searchQuery = state.search || '';
      dynamicFilters = Array.isArray(state.dynamicFields) ? state.dynamicFields : [];
      rawMode = false;
    } else if (storedQl.trim()) {
      // Legacy collection with CQL but no persisted state → raw mode.
      rawQlQuery = storedQl;
      rawMode = true;
    } else {
      // Fresh or empty collection → default to builder mode.
      rawMode = false;
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
      dynamicFields: dynamicFilters
    });
  }


  async function loadWorkspaces() {
    try {
      const result = await api.workspaces.getAll();
      workspaces = result || [];
      searchStore.setWorkspaces(workspaces);
    } catch (error) {
      console.error('Failed to load workspaces:', error);
      workspaces = [];
    }
  }

  async function loadStatusesAndCategories() {
    try {
      // Fetch statuses
      const statusesResponse = await api.statuses.getAll();
      statuses = statusesResponse || [];
      searchStore.setStatuses(statuses);

      // Fetch status categories for colors
      const categoriesResponse = await api.statusCategories.getAll();
      statusCategories = categoriesResponse || [];
    } catch (error) {
      console.error('Failed to load statuses:', error);
      statuses = [];
      statusCategories = [];
    }
  }

  async function loadPriorities() {
    try {
      const result = await api.priorities.getAll();
      priorities = result || [];
      searchStore.setPriorities(priorities);
    } catch (error) {
      console.error('Failed to load priorities:', error);
      priorities = [];
    }
  }

  async function loadWorkItems(page = 1, limit = itemsPerPage) {
    try {
      searchStore.setLoading(true);
      searchStore.setError(null);

      let filters = {
        page: page,
        limit: limit
      };

      if (qlQuery.trim()) {
        filters.ql = qlQuery;
        qlError = null;
      }

      // Only proceed with API call if we have a QL query OR if loading a collection
      if (filters.ql || currentCollection) {
        try {
          const response = await api.items.getAll(filters);

          if (response && response.items) {
            // Handle paginated response from backend
            searchStore.setSearchResults(response.items);
            itemsPagination = response.pagination;
            currentPage = page;
            itemsPerPage = limit;
          } else {
            // Handle legacy response (backward compatibility)
            searchStore.setSearchResults(response || []);
            itemsPagination = null;
          }
        } catch (error) {
          console.error('QL query error:', error);
          qlError = error.message;
          searchStore.setSearchResults([]);
          searchStore.setError(error.message);
          itemsPagination = null;
        }
      } else {
        // No query to execute, clear results
        searchStore.setSearchResults([]);
        itemsPagination = null;
      }
    } catch (error) {
      console.error('Failed to load work items:', error);
      searchStore.setSearchResults([]);
      itemsPagination = null;
      if (!qlError) {
        qlError = error.message;
        searchStore.setError(error.message);
      }
    } finally {
      searchStore.setLoading(false);
    }
  }

  // Sync filter state to URL parameters. In raw mode, only `raw` is written;
  // in builder mode, structured params are written.
  function syncURLParams() {
    const url = new URL(window.location.href);
    url.searchParams.delete('load');
    url.searchParams.delete('showQL');
    url.searchParams.delete('ql');
    url.searchParams.delete('raw');
    url.searchParams.delete('workspaces');
    url.searchParams.delete('statuses');
    url.searchParams.delete('priorities');
    url.searchParams.delete('search');
    url.searchParams.delete('dynamicFilters');

    if (rawMode) {
      if (rawQlQuery.trim()) url.searchParams.set('raw', rawQlQuery);
      window.history.pushState({}, '', url);
      return;
    }

    if (selectedWorkspaces.length > 0) url.searchParams.set('workspaces', selectedWorkspaces.join(','));
    if (selectedStatuses.length > 0) url.searchParams.set('statuses', selectedStatuses.join(','));
    if (selectedPriorities.length > 0) url.searchParams.set('priorities', selectedPriorities.join(','));
    if (searchQuery.trim()) url.searchParams.set('search', searchQuery);

    const serializableDyn = dynamicFilters.filter(f => f.field && (f.value || (f.values && f.values.length > 0)));
    if (serializableDyn.length > 0) url.searchParams.set('dynamicFilters', JSON.stringify(serializableDyn));

    window.history.pushState({}, '', url);
  }

  // Restore filter state from URL parameters. `?raw=<cql>` triggers raw mode;
  // otherwise structured params hydrate the builder.
  function restoreFromURL() {
    const urlParams = new URLSearchParams(window.location.search);

    const urlRaw = urlParams.get('raw') ?? urlParams.get('ql');
    if (urlRaw) {
      rawQlQuery = urlRaw;
      rawMode = true;
      syncFiltersToSearchStore();
      return;
    }

    const urlWorkspaces = urlParams.get('workspaces');
    if (urlWorkspaces) {
      selectedWorkspaces = urlWorkspaces.split(',').map(id => parseInt(id, 10)).filter(id => !isNaN(id));
    }

    const urlStatuses = urlParams.get('statuses');
    if (urlStatuses) {
      selectedStatuses = urlStatuses
        .split(',')
        .map(value => {
          const parsedId = parseInt(value, 10);
          if (!isNaN(parsedId)) return parsedId;
          const matchingStatus = statuses.find(status =>
            (status.name || status.key || '').toLowerCase() === value.toLowerCase()
          );
          return matchingStatus ? matchingStatus.id : null;
        })
        .filter(id => id !== null && id !== undefined);
    }

    const urlPriorities = urlParams.get('priorities');
    if (urlPriorities) {
      selectedPriorities = urlPriorities
        .split(',')
        .map(value => {
          const parsedId = parseInt(value, 10);
          if (!isNaN(parsedId)) return parsedId;
          const matchingPriority = priorities.find(priority =>
            priority.name?.toLowerCase() === value.toLowerCase()
          );
          return matchingPriority ? matchingPriority.id : null;
        })
        .filter(id => id !== null && id !== undefined);
    }

    const urlSearch = urlParams.get('search');
    if (urlSearch) searchQuery = urlSearch;

    const urlDynamicFilters = urlParams.get('dynamicFilters');
    if (urlDynamicFilters) {
      try {
        dynamicFilters = JSON.parse(urlDynamicFilters);
      } catch (error) {
        console.error('Failed to parse dynamic filters from URL:', error);
        dynamicFilters = [];
      }
    }

    syncFiltersToSearchStore();
  }

  // Event handlers for sidebar filter callbacks. Ignored in raw mode since the
  // sidebar is visually disabled.
  function handleUpdateWorkspaces(value) {
    if (rawMode) return;
    selectedWorkspaces = value;
  }

  function handleUpdateStatuses(value) {
    if (rawMode) return;
    selectedStatuses = (value || [])
      .map(v => Number(v))
      .filter(id => !Number.isNaN(id));
  }

  function handleUpdatePriorities(value) {
    if (rawMode) return;
    selectedPriorities = (value || [])
      .map(v => Number(v))
      .filter(id => !Number.isNaN(id));
  }

  function handleUpdateSearch(value) {
    if (rawMode) return;
    searchQuery = value;
  }

  function handleUpdateDynamicFilters(value) {
    if (rawMode) return;
    dynamicFilters = value;
  }

  function handleExecuteQL() {
    qlError = null;
    syncURLParams();
    loadWorkItems(1, itemsPerPage);
  }

  async function enterRawMode() {
    if (rawMode) return;
    const confirmed = await confirm({
      title: t('collections.rawModeConfirmTitle'),
      message: t('collections.rawModeConfirmMessage'),
      confirmText: t('collections.rawModeConfirmAccept'),
      cancelText: t('common.cancel'),
      variant: 'warning'
    });
    if (!confirmed) return;
    // Snapshot the currently-derived CQL so the user starts editing from there.
    rawQlQuery = qlQuery;
    selectedWorkspaces = [];
    selectedStatuses = [];
    selectedPriorities = [];
    searchQuery = '';
    dynamicFilters = [];
    rawMode = true;
    syncFiltersToSearchStore();
    syncURLParams();
  }

  function resetToBuilder() {
    // Best-effort: if the raw CQL matches the builder's supported shapes,
    // carry its fields over so the user doesn't have to start from scratch.
    // Anything we don't recognize is dropped.
    const parsed = QLBuilder.tryParseToBuilder(rawQlQuery);

    selectedWorkspaces = parsed
      ? parsed.workspaces
          .map((name) => workspaces.find((w) => w.name === name)?.id)
          .filter(Boolean)
      : [];
    selectedStatuses = parsed ? parsed.statuses : [];
    selectedPriorities = parsed ? parsed.priorities : [];
    searchQuery = parsed ? parsed.search : '';
    dynamicFilters = [];
    rawQlQuery = '';
    rawMode = false;
    qlError = null;
    syncFiltersToSearchStore();
    syncURLParams();
    loadWorkItems(1, itemsPerPage);
  }

  function syncFiltersToSearchStore() {
    searchStore.setSelectedWorkspaces(selectedWorkspaces);
    searchStore.setSelectedStatuses(selectedStatuses);
    searchStore.setSelectedPriorities(selectedPriorities);
    searchStore.setSearchQuery(searchQuery);
    searchStore.setDynamicFilters(dynamicFilters);
  }

  function getStatusColor(status) {
    // Use utility function if we have status data, otherwise fall back to design system colors
    if (statuses.length > 0 && statusCategories.length > 0) {
      return getStatusColorUtil(status, statuses, statusCategories);
    }
    // Fallback to design system status colors
    return getStatusStyle(status);
  }

  function getPriorityColor(priority) {
    const colors = {
      low: 'text-gray-500',
      medium: 'text-blue-500',
      high: 'text-orange-500',
      critical: 'text-red-500'
    };
    return colors[priority] || 'text-gray-500';
  }

  function getWorkspaceName(workspaceId) {
    const workspace = workspaces.find(w => w.id === workspaceId);
    return workspace ? workspace.name : 'Unknown';
  }

  function getWorkspaceKey(workspaceId) {
    const workspace = workspaces.find(w => w.id === workspaceId);
    return workspace ? workspace.key : 'WORK';
  }

  // DataTable column configuration
  const workItemColumns = [
    {
      key: 'display_key',
      label: 'Key',
      width: 'w-28',
      html: true,
      render: (item) => `<a href="/workspaces/${item.workspace_id}/items/${item.id}" class="text-xs font-mono px-1.5 py-0.5 rounded whitespace-nowrap no-underline" style="color: var(--ds-text-subtle); background-color: var(--ds-interactive-subtle);">${escapeHtml(item.display_key)}</a>`
    },
    {
      key: 'title',
      label: 'Title',
      html: true,
      render: (item) => `<a href="/workspaces/${item.workspace_id}/items/${item.id}" class="block truncate text-sm no-underline" style="color: inherit;" title="${escapeHtml(item.title)}">${escapeHtml(item.title) || '—'}</a>`
    },
    {
      key: 'workspace_name',
      label: 'Workspace',
      width: 'w-36',
      html: true,
      render: (item) => `<span class="block truncate" title="${escapeHtml(item.workspace_name)}">${escapeHtml(item.workspace_name) || '—'}</span>`
    },
    {
      key: 'status_name',
      label: 'Status',
      width: 'w-28',
      html: true,
      render: (item) => item.status_name ? `<span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium whitespace-nowrap" style="${getStatusInlineStyle(item.status_name, statuses, statusCategories)}">${escapeHtml(item.status_name)}</span>` : '—'
    },
    {
      key: 'priority_name',
      label: 'Priority',
      width: 'w-24',
      html: true,
      render: (item) => item.priority_name ? `<span class="text-sm font-medium capitalize whitespace-nowrap" style="color: ${escapeHtml(item.priority_color) || 'var(--ds-text-subtle)'}">${escapeHtml(item.priority_name)}</span>` : '—'
    },
    {
      key: 'created_at',
      label: 'Created',
      width: 'w-28',
      html: true,
      render: (item) => `<span class="whitespace-nowrap">${formatDate(item.created_at) || '—'}</span>`
    },
    { key: 'actions', label: '', width: 'w-12' }
  ];

  // Transform work items for DataTable
  let tableData = $derived(workItems.map(item => ({
    ...item,
    display_key: `${getWorkspaceKey(item.workspace_id)}-${item.id}`,
    workspace_name: getWorkspaceName(item.workspace_id)
  })));

  function viewItem(item) {
    navigate(`/workspaces/${item.workspace_id}/items/${item.id}`);
  }

  async function deleteItem(item) {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('collections.confirmDeleteItem', { title: item.title }),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger'
    });
    if (!confirmed) return;
    
    try {
      await api.items.delete(item.id);
      // Refresh the work items list
      await loadWorkItems(currentPage, itemsPerPage);
    } catch (error) {
      console.error('Failed to delete item:', error);
      errorToast(t('dialogs.alerts.failedToDelete', { error: error.message || error }));
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
        color: 'var(--ds-text-danger)',
        hoverClass: 'hover-danger',
        onClick: () => deleteItem(item)
      }
    ];
  }

  // Handle pagination events
  async function handlePageChange(event) {
    await loadWorkItems(event.detail.page, event.detail.itemsPerPage);
  }

  async function handlePageSizeChange(event) {
    await loadWorkItems(event.detail.page, event.detail.itemsPerPage);
  }

  // Public sharing handlers
  async function handlePublicToggle() {
    if (!currentCollection) return;
    const newIsPublic = !currentCollection.is_public;
    if (!newIsPublic) {
      // Disabling: save immediately
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
      // Enabling: just update local state, user must enter slug and save
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

  // Collections functions
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
    workspaceAssociationSelection = currentCollection.workspace_id
      ? [currentCollection.workspace_id]
      : [];
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
    const workspaceId = workspaceAssociationSelection.length === 1 ? workspaceAssociationSelection[0] : null;

    try {
      await api.collections.update(currentCollection.id, {
        name: currentCollection.name,
        description: currentCollection.description || null,
        ql_query: qlQuery,
        is_public: currentCollection.is_public,
        workspace_id: workspaceId
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
  let associatedWorkspace = $derived(currentCollection?.workspace_id
    ? workspaces.find(w => w.id === currentCollection.workspace_id)
    : null);
  let associatedWorkspaceName = $derived(associatedWorkspace
    ? `${associatedWorkspace.name}${associatedWorkspace.key ? ` (${associatedWorkspace.key})` : ''}`
    : '');

  $effect(() => {
    if (workspaceAssociationSelection.length > 1) {
      workspaceAssociationSelection = [workspaceAssociationSelection[workspaceAssociationSelection.length - 1]];
    }
  });
</script>

<div class="min-h-screen flex" style="background-color: var(--ds-surface);">
  <!-- Collapsible Sidebar -->
  <CollectionsSidebar
    bind:collapsed={sidebarCollapsed}
    {workspaces}
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

  <!-- Main Content -->
  <div class="flex-1 p-6 overflow-auto">
    <!-- Breadcrumbs with Actions -->
    <CollectionsBreadcrumbs
      collection={currentCollection}
      workspace={associatedWorkspace}
      isEditing={!!currentCollection}
      canSave={canSubmitCollection}
      categories={$collectionCategoriesStore}
      {returnPath}
      onsave={updateCollectionDirectly}
      onassociateworkspace={openAssociateWorkspaceModal}
      onnamechange={(value) => { if (currentCollection) currentCollection.name = value; }}
      ondescriptionchange={(value) => { if (currentCollection) currentCollection.description = value; }}
      oncategorychange={(value) => { if (currentCollection) currentCollection = { ...currentCollection, category_id: value }; }}
      showPublicBoard={!!currentCollection}
      isPublic={currentCollection?.is_public || false}
      publicSlug={currentCollection?.public_slug || null}
      {slugSaved}
      saving={savingPublicSharing}
      onpublictoggle={handlePublicToggle}
      onslugchange={handleSlugChange}
      onslugsave={handleSlugSave}
    />

    <!-- Always-visible QL Query Bar -->
    <CollectionQueryBar
      query={qlQuery}
      mode={rawMode ? 'raw' : 'builder'}
      error={qlError}
      onenterrawmode={enterRawMode}
      onreset={resetToBuilder}
      onexecute={handleExecuteQL}
      onquerychange={(value) => { rawQlQuery = value; }}
    />

    <!-- Results Section -->
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
      <!-- Work Items Table -->
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

      <!-- Pagination -->
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
        <!-- Results Summary -->
        <div class="mt-4 text-sm text-center" style="color: var(--ds-text-subtle);">
          {t('collections.showingWorkItems', { count: workItems.length })}
        </div>
      {/if}
    {/if}

  </div>
</div>

<Modal
  isOpen={showWorkspaceAssociationModal}
  onclose={closeAssociateWorkspaceModal}
  maxWidth="max-w-2xl"
>
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
    background-color: var(--ds-surface-hovered);
  }
</style>
