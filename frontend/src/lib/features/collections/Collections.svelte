<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { navigate } from '../../router.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { Filter, Search, MoreHorizontal, Calendar, User, AlertCircle, Trash2, Eye, Save, SquareKanban } from 'lucide-svelte';
  import { escapeHtml } from '../../utils/sanitize.ts';
  import Button from '../../components/Button.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import Pagination from '../../components/Pagination.svelte';
  import CollectionsSidebar from '../collections/CollectionsSidebar.svelte';
  import CollectionsBreadcrumbs from '../collections/CollectionsBreadcrumbs.svelte';
  import CollectionQueryBar from '../collections/CollectionQueryBar.svelte';
  import { QLEvaluator, QLBuilder } from '../../utils/ql.js';
  import { getStatusColor as getStatusColorUtil, getStatusInlineStyle, getStatusStyle } from '../../utils/statusColors.js';
  import { formatDate } from '../../utils/dateFormatter.js';
  import { searchStore } from '../../stores/searchStore.svelte.js';
  import Modal from '../../dialogs/Modal.svelte';
  import WorkspacePicker from '../../pickers/WorkspacePicker.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import Select from '../../components/Select.svelte';
  import { collectionCategoriesStore } from '../../stores/collectionCategories.js';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';

  // Props
  export let collectionId = null; // When provided, load and edit this collection

  let workspaces = [];
  let currentCollection = null; // Store the loaded collection data
  let selectedWorkspaces = [];
  let statuses = [];
  let selectedStatuses = [];
  let priorities = [];
  let selectedPriorities = [];

  let statusCategories = [];
  let dynamicFilters = [];
  // Items are now managed by searchStore for reactivity
  let itemsPagination = null;
  let loading = true;
  let searchQuery = '';
  let currentPage = 1;
  let itemsPerPage = 50;

  // Subscribe to searchStore for reactive items and loading state
  let storeState = {};
  searchStore.subscribe(value => storeState = value);

  // Reactive unpacking for items and loading
  $: workItems = storeState.searchResults || [];
  $: loadingItems = storeState.loading || false;
  $: qlErrorFromStore = storeState.error;

  // QL state
  let qlQuery = '';
  let showQLInput = false;
  let qlEvaluator = null;
  let qlError = null;
  let qlManuallyEdited = false;

  // Sidebar state
  let sidebarCollapsed = false;


  // Workspace association modal state
  // Read workspace return context from query param
  let returnWorkspaceId = null;
  let returnPath = null;

  let showWorkspaceAssociationModal = false;
  let workspaceAssociationSelection = [];
  let workspaceAssociationError = null;
  let workspaceAssociationSaving = false;

  onMount(async () => {
    await loadWorkspaces();
    await loadStatusesAndCategories();
    await loadPriorities();
    await collectionCategoriesStore.init();
    qlEvaluator = new QLEvaluator(workspaces);

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
      // Restore filter state from URL params
      restoreFromURL();
      syncQLQuery();

      // Execute search if we have filters from URL
      if (selectedWorkspaces.length > 0 || selectedStatuses.length > 0 || selectedPriorities.length > 0 || searchQuery || dynamicFilters.length > 0 || qlQuery) {
        await loadWorkItems(1, itemsPerPage);
      }
    }

    loading = false;
  });

  async function loadCollectionById(collectionId) {
    try {
      const collection = await api.collections.get(collectionId);
      if (collection) {
        // Store the collection data
        currentCollection = collection;

        // Set the QL query from the collection
        qlQuery = collection.ql_query || '';

        // Parse QL to extract filters and set UI accordingly
        const parsedFilters = QLBuilder.parseFiltersFromQuery(qlQuery, workspaces, priorities, statuses);

        if (parsedFilters) {
          // Convert workspace names back to IDs for the UI
          selectedWorkspaces = parsedFilters.workspaces
            .map(name => workspaces.find(w => w.name === name)?.id)
            .filter(Boolean);

          // Set other filters directly
          selectedStatuses = parsedFilters.statuses || [];
          selectedPriorities = parsedFilters.priorities || [];
          searchQuery = parsedFilters.search || '';
          dynamicFilters = parsedFilters.dynamicFields || [];

          // If we successfully parsed filters, don't force manual QL editing
          // This allows users to use UI filters to modify the collection
          qlManuallyEdited = false;
          showQLInput = false; // Start with UI filters visible
        } else {
          // If parsing failed or QL is complex, show QL input
          selectedWorkspaces = [];
          selectedStatuses = [];
          selectedPriorities = [];
          searchQuery = '';
          dynamicFilters = [];
          qlManuallyEdited = true;
          showQLInput = true;
        }

        syncFiltersToSearchStore();

        // Execute the loaded query
        await loadWorkItems(1, itemsPerPage);

        // Remove the load parameter from URL without refreshing
        const url = new URL(window.location);
        url.searchParams.delete('load');
        window.history.replaceState({}, '', url);
      }
    } catch (error) {
      console.error('Failed to load collection:', error);
      syncQLQuery();
    }
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

      // Always use QL for backend processing
      if (qlQuery.trim()) {
        // Manual QL query - always execute when present
        filters.ql = qlQuery;
        qlError = null;
      } else if (selectedWorkspaces.length > 0 || selectedStatuses.length > 0 || selectedPriorities.length > 0 || searchQuery.trim() || dynamicFilters.length > 0) {
        // Build QL from UI filters - let backend handle the complexity
        // Convert workspace IDs to names for QL builder
        const workspaceNames = selectedWorkspaces
          .map(id => workspaces.find(w => w.id === id)?.name)
          .filter(Boolean);

        const generatedQL = QLBuilder.buildQuery({
          workspaces: workspaceNames,
          statuses: selectedStatuses,
          priorities: selectedPriorities,
          search: searchQuery,
          dynamicFields: dynamicFilters
        });

        if (generatedQL.trim()) {
          filters.ql = generatedQL;
          qlError = null;
        }
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

  function syncQLQuery() {
    // Only auto-generate QL if it hasn't been manually edited
    // This prevents overwriting user's manual QL changes
    if (!qlManuallyEdited) {
      // Convert workspace IDs to names for QL builder
      const workspaceNames = selectedWorkspaces
        .map(id => workspaces.find(w => w.id === id)?.name)
        .filter(Boolean);

      const filters = {
        workspaces: workspaceNames,
        statuses: selectedStatuses,
        priorities: selectedPriorities,
        search: searchQuery,
        dynamicFields: dynamicFilters
      };
      qlQuery = QLBuilder.buildQuery(filters);
    }
  }

  // Sync filter state to URL parameters
  function syncURLParams() {
    const url = new URL(window.location);
    url.searchParams.delete('load'); // Remove load param if present

    // Only add params if they have values
    if (qlQuery.trim()) {
      url.searchParams.set('ql', qlQuery);
    } else {
      url.searchParams.delete('ql');
    }

    if (selectedWorkspaces.length > 0) {
      url.searchParams.set('workspaces', selectedWorkspaces.join(','));
    } else {
      url.searchParams.delete('workspaces');
    }

    if (selectedStatuses.length > 0) {
      url.searchParams.set('statuses', selectedStatuses.join(','));
    } else {
      url.searchParams.delete('statuses');
    }

    if (selectedPriorities.length > 0) {
      url.searchParams.set('priorities', selectedPriorities.join(','));
    } else {
      url.searchParams.delete('priorities');
    }

    if (searchQuery.trim()) {
      url.searchParams.set('search', searchQuery);
    } else {
      url.searchParams.delete('search');
    }

    if (dynamicFilters.length > 0) {
      // Serialize dynamic filters to JSON
      const filtersToSerialize = dynamicFilters.filter(f => f.field && (f.value || (f.values && f.values.length > 0)));
      if (filtersToSerialize.length > 0) {
        url.searchParams.set('dynamicFilters', JSON.stringify(filtersToSerialize));
      } else {
        url.searchParams.delete('dynamicFilters');
      }
    } else {
      url.searchParams.delete('dynamicFilters');
    }

    if (showQLInput) {
      url.searchParams.set('showQL', 'true');
    } else {
      url.searchParams.delete('showQL');
    }

    window.history.pushState({}, '', url);
  }

  // Restore filter state from URL parameters
  function restoreFromURL() {
    const urlParams = new URLSearchParams(window.location.search);

    // Restore QL query
    const urlQL = urlParams.get('ql');
    if (urlQL) {
      qlQuery = urlQL;
      qlManuallyEdited = true;
    }

    // Restore workspaces
    const urlWorkspaces = urlParams.get('workspaces');
    if (urlWorkspaces) {
      selectedWorkspaces = urlWorkspaces.split(',').map(id => parseInt(id, 10)).filter(id => !isNaN(id));
    }

    // Restore statuses
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

    // Restore priorities
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

    // Restore search query
    const urlSearch = urlParams.get('search');
    if (urlSearch) {
      searchQuery = urlSearch;
    }

    // Restore dynamic filters
    const urlDynamicFilters = urlParams.get('dynamicFilters');
    if (urlDynamicFilters) {
      try {
        dynamicFilters = JSON.parse(urlDynamicFilters);
      } catch (error) {
        console.error('Failed to parse dynamic filters from URL:', error);
        dynamicFilters = [];
      }
    }

    // Restore QL input visibility
    const urlShowQL = urlParams.get('showQL');
    if (urlShowQL === 'true') {
      showQLInput = true;
    }

    syncFiltersToSearchStore();
  }

  // Event handlers for WorkItemFilter component
  function handleUpdateWorkspaces(event) {
    selectedWorkspaces = event.detail;
    qlManuallyEdited = false;
    syncQLQuery();
  }

  function handleUpdateStatuses(event) {
    selectedStatuses = (event.detail || [])
      .map(value => Number(value))
      .filter(id => !Number.isNaN(id));
    qlManuallyEdited = false;
    syncQLQuery();
  }

  function handleUpdatePriorities(event) {
    selectedPriorities = (event.detail || [])
      .map(value => Number(value))
      .filter(id => !Number.isNaN(id));
    qlManuallyEdited = false;
    syncQLQuery();
  }

  function handleUpdateSearch(event) {
    searchQuery = event.detail;
    qlManuallyEdited = false;
    syncQLQuery();
  }

  function handleUpdateQL(event) {
    // Handle both old string format and new object format for backward compatibility
    const detail = typeof event.detail === 'string'
      ? { ql: event.detail, isManual: true }
      : event.detail;

    qlQuery = detail.ql;
    qlManuallyEdited = detail.isManual;
  }

  function handleUpdateQLMode(event) {
    showQLInput = event.detail;
    if (!showQLInput) {
      syncQLQuery(); // Sync UI filters to QL when hiding QL input
    }
    qlError = null;
  }

  function handleUpdateDynamicFilters(event) {
    dynamicFilters = event.detail;
    qlManuallyEdited = false;
    syncQLQuery();
  }

  function handleExecuteQL() {
    qlError = null;
    syncURLParams(); // Update URL when executing search
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
      render: (item) => `<span class="text-xs font-mono px-1.5 py-0.5 rounded whitespace-nowrap" style="color: var(--ds-text-subtle); background-color: var(--ds-interactive-subtle);">${escapeHtml(item.display_key)}</span>`
    },
    {
      key: 'title',
      label: 'Title',
      html: true,
      render: (item) => `<span class="block truncate" title="${escapeHtml(item.title)}">${escapeHtml(item.title) || '—'}</span>`
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
  $: tableData = workItems.map(item => ({
    ...item,
    display_key: `${getWorkspaceKey(item.workspace_id)}-${item.id}`,
    workspace_name: getWorkspaceName(item.workspace_id)
  }));

  function viewItem(item) {
    navigate(`/workspaces/${item.workspace_id}/items/${item.id}`);
  }

  async function deleteItem(item) {
    if (!confirm(t('collections.confirmDeleteItem', { title: item.title }))) {
      return;
    }
    
    try {
      await api.items.delete(item.id);
      // Refresh the work items list
      await loadWorkItems(currentPage, itemsPerPage);
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

  // Handle pagination events
  async function handlePageChange(event) {
    await loadWorkItems(event.detail.page, event.detail.itemsPerPage);
  }
  
  async function handlePageSizeChange(event) {
    await loadWorkItems(event.detail.page, event.detail.itemsPerPage);
  }

  // Collections functions
  async function updateCollectionDirectly() {
    if (!currentCollection) return;

    if (!qlQuery.trim()) {
      alert(t('collections.noQueryToSave'));
      return;
    }

    try {
      await api.collections.update(currentCollection.id, {
        name: currentCollection.name,
        description: currentCollection.description || null,
        ql_query: qlQuery,
        is_public: currentCollection.is_public,
        workspace_id: currentCollection.workspace_id ?? null,
        category_id: currentCollection.category_id ?? null
      });

      // Navigate back to workspace if we came from one, otherwise collections list
      navigate(returnPath || '/collections');
    } catch (error) {
      console.error('Failed to update collection:', error);
      alert(t('dialogs.alerts.failedToUpdate', { error: error.message || error }));
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

  // Track previous search query to detect actual changes
  let previousSearchQuery = searchQuery;

  // Reload items when search query actually changes (not on initial load)
  $: if (searchQuery !== previousSearchQuery && !loading) {
    previousSearchQuery = searchQuery;
    currentPage = 1;
    qlManuallyEdited = false; // Reset flag when using search
    syncQLQuery();
    syncURLParams(); // Update URL when search changes
    loadWorkItems(1, itemsPerPage);
  }
  $: trimmedCollectionName = (currentCollection?.name || '').trim();
  $: trimmedQlQuery = qlQuery.trim();
  $: canSubmitCollection = Boolean(currentCollection && trimmedCollectionName && trimmedQlQuery);
  $: associatedWorkspace = currentCollection?.workspace_id
    ? workspaces.find(w => w.id === currentCollection.workspace_id)
    : null;
  $: associatedWorkspaceName = associatedWorkspace
    ? `${associatedWorkspace.name}${associatedWorkspace.key ? ` (${associatedWorkspace.key})` : ''}`
    : '';
  $: if (workspaceAssociationSelection.length > 1) {
    workspaceAssociationSelection = [workspaceAssociationSelection[workspaceAssociationSelection.length - 1]];
  }
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
    on:update-workspaces={handleUpdateWorkspaces}
    on:update-statuses={handleUpdateStatuses}
    on:update-priorities={handleUpdatePriorities}
    on:update-search={handleUpdateSearch}
    on:update-dynamic-filters={handleUpdateDynamicFilters}
    on:execute-search={handleExecuteQL}
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
      on:save={updateCollectionDirectly}
      on:associate-workspace={openAssociateWorkspaceModal}
      on:name-change={(e) => { if (currentCollection) currentCollection.name = e.detail; }}
      on:description-change={(e) => { if (currentCollection) currentCollection.description = e.detail; }}
      on:category-change={(e) => { if (currentCollection) currentCollection.category_id = e.detail; }}
    />

    <!-- Always-visible QL Query Bar -->
    <CollectionQueryBar
      query={qlQuery}
      isEditing={showQLInput}
      error={qlError}
      on:toggle-edit={() => showQLInput = !showQLInput}
      on:execute={handleExecuteQL}
      on:clear={() => { qlQuery = ''; qlManuallyEdited = false; syncQLQuery(); handleExecuteQL(); }}
      on:query-change={(e) => { qlQuery = e.detail; qlManuallyEdited = true; }}
    />

    <!-- Results Section -->
    {#if loading}
      <div class="rounded-xl border shadow-sm p-8 text-center" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <div class="animate-pulse" style="color: var(--ds-text-subtle);">{t('collections.loadingWorkspaces')}</div>
      </div>
    {:else if !qlQuery.trim() && dynamicFilters.length === 0 && !currentCollection}
      <div class="rounded-xl border shadow-sm p-12 text-center" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <Filter class="w-12 h-12 mx-auto mb-4" style="color: var(--ds-icon-subtle);" />
        <h3 class="text-lg font-medium mb-2" style="color: var(--ds-text);">{t('collections.addFiltersToStart')}</h3>
        <p style="color: var(--ds-text-subtle);">{t('collections.addFiltersDesc')}</p>
      </div>
    {:else if loadingItems}
      <div class="rounded-xl border shadow-sm p-8 text-center" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <div class="animate-pulse" style="color: var(--ds-text-subtle);">{t('collections.loadingWorkItems')}</div>
      </div>
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
            maxItems={100}
            on:pageChange={handlePageChange}
            on:pageSizeChange={handlePageSizeChange}
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
