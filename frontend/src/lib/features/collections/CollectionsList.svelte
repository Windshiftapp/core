<script>
  import { onMount, onDestroy } from 'svelte';
  import { api } from '../../api.js';
  import { navigate, currentRoute } from '../../router.js';
  import { FolderOpen, Plus, Eye, Trash2 } from 'lucide-svelte';
  import Button from '../../components/Button.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import CategoryModal from '../../dialogs/CategoryModal.svelte';
  import CollectionsNavigation from '../collections/CollectionsNavigation.svelte';
  import { collectionCategoriesStore } from '../../stores/collectionCategories.js';
  import { formatDate } from '../../utils/dateFormatter.js';
  import { workspacesStore } from '../../stores';
  import WorkspaceSelector from '../../workspaces/WorkspaceSelector.svelte';
  import ColorDot from '../../components/ColorDot.svelte';

  let collections = [];
  let loading = true;
  let selectedWorkspaceFilter = null;
  let workspaceOptions = [];
  let workspaceMap = new Map();

  // Category management modal
  let showCategoryModal = false;

  // Determine view based on URL
  $: activeCategoryId = $currentRoute.params?.categoryId || null;
  $: isWorkspaceView = $currentRoute.path?.includes('/workspace');

  // Separate collections by type
  $: workspaceCollections = collections.filter(c => c.workspace_id);
  $: globalCollections = collections.filter(c => !c.workspace_id);

  // Filter based on current view
  $: filteredCollections = (() => {
    if (isWorkspaceView) {
      return workspaceCollections.filter(c =>
        !selectedWorkspaceFilter || c.workspace_id === getWorkspaceId(selectedWorkspaceFilter)
      );
    } else {
      // Global collections - filter by category if one is selected
      if (activeCategoryId) {
        return globalCollections.filter(c => c.category_id === parseInt(activeCategoryId));
      }
      return globalCollections;
    }
  })();

  // Dynamic page title
  $: pageTitle = (() => {
    if (isWorkspaceView) {
      return 'Workspace Collections';
    } else if (activeCategoryId) {
      const category = collectionCategoriesStore.getById(parseInt(activeCategoryId), $collectionCategoriesStore);
      return category ? `${category.name} Collections` : 'Category Collections';
    }
    return 'All Global Collections';
  })();

  const getWorkspaceId = (workspaceId) =>
    typeof workspaceId === 'string' ? parseInt(workspaceId, 10) : workspaceId;

  // Column definitions for DataTable
  const baseCollectionColumns = [
    {
      key: 'name',
      label: 'Collection',
      slot: 'name'
    },
    {
      key: 'cql_query',
      label: 'Query',
      slot: 'query'
    },
    {
      key: 'created_at',
      label: 'Created',
      render: (collection) => formatDate(collection.created_at) || '-',
      textColor: 'var(--ds-text-subtle)'
    },
    {
      key: 'actions',
      label: 'Actions'
    }
  ];

  const workspaceColumn = {
    key: 'workspace',
    label: 'Workspace',
    render: (collection) => getWorkspaceName(collection.workspace_id) || '—'
  };

  const categoryColumn = {
    key: 'category',
    label: 'Category',
    slot: 'category'
  };

  $: collectionColumns = isWorkspaceView
    ? [baseCollectionColumns[0], workspaceColumn, ...baseCollectionColumns.slice(1)]
    : (!activeCategoryId
      ? [baseCollectionColumns[0], categoryColumn, ...baseCollectionColumns.slice(1)]
      : baseCollectionColumns);

  $: workspaceOptions = ($workspacesStore?.allWorkspaces || []).filter(ws => !ws.is_personal);
  $: workspaceMap = new Map(workspaceOptions.map(ws => [ws.id, ws]));

  function getWorkspaceName(workspaceId) {
    if (!workspaceId) return '';
    const workspace = workspaceMap.get(workspaceId);
    return workspace ? workspace.name : '';
  }

  onMount(async () => {
    workspacesStore.load();
    await Promise.all([
      loadCollections(),
      collectionCategoriesStore.init()
    ]);

    document.addEventListener('manage-collection-categories', handleManageCategoriesEvent);
  });

  onDestroy(() => {
    document.removeEventListener('manage-collection-categories', handleManageCategoriesEvent);
  });

  function handleManageCategoriesEvent() {
    showCategoryModal = true;
  }

  async function loadCollections() {
    try {
      loading = true;
      collections = await api.collections.getAll() || [];
    } catch (error) {
      console.error('Failed to load collections:', error);
      collections = [];
    } finally {
      loading = false;
    }
  }

  function createNewCollection() {
    window.dispatchEvent(new CustomEvent('show-create-modal', {
      detail: { type: 'collection' }
    }));
  }

  function viewCollection(collection) {
    navigate(`/collections/${collection.id}`);
  }

  async function deleteCollection(collection) {
    if (!confirm(`Are you sure you want to delete the collection "${collection.name}"? This action cannot be undone.`)) {
      return;
    }

    try {
      await api.collections.delete(collection.id);
      await loadCollections();
    } catch (error) {
      console.error('Failed to delete collection:', error);
      alert('Failed to delete collection: ' + (error.message || error));
    }
  }

  function buildCollectionActions(collection) {
    return [
      {
        id: 'view',
        type: 'regular',
        icon: Eye,
        title: 'View Collection',
        onClick: () => viewCollection(collection)
      },
      { type: 'divider' },
      {
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: 'Delete',
        color: '#dc2626',
        hoverClass: 'hover:bg-red-50 hover:text-red-700',
        onClick: () => deleteCollection(collection)
      }
    ];
  }

  // Category management functions
  async function handleAddCategory(data) {
    await collectionCategoriesStore.add(data);
  }

  async function handleDeleteCategory(categoryId) {
    await collectionCategoriesStore.delete(categoryId);
    await loadCollections(); // Refresh to update any affected collections
  }
</script>

<div class="flex min-h-screen" style="background-color: var(--ds-surface);">
  <CollectionsNavigation />

  <div class="flex-1">
    <div class="p-6">
      <!-- Header -->
      <div class="mb-6 flex items-start justify-between gap-4">
        <div class="flex-1 min-w-0">
          <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">
            {pageTitle}
          </h1>
          <p class="text-base" style="color: var(--ds-text-subtle);">
            {filteredCollections.length} collection{filteredCollections.length !== 1 ? 's' : ''}
          </p>
        </div>
        <div class="flex-shrink-0">
          <Button
            onclick={createNewCollection}
            variant="primary"
            icon={Plus}
            keyboardHint="A"
          >
            New Collection
          </Button>
        </div>
      </div>

      <!-- Workspace filter (only shown in workspace view) -->
      {#if isWorkspaceView}
        <div class="flex flex-wrap items-center gap-3 mb-6">
          <label class="text-sm font-medium" style="color: var(--ds-text-subtle);">Workspace Filter</label>
          <div class="min-w-[260px]">
            <WorkspaceSelector
              value={selectedWorkspaceFilter}
              workspaces={workspaceOptions}
              placeholder="All workspaces"
              allowClear={true}
              on:select={(event) => {
                selectedWorkspaceFilter = event.detail?.id || null;
              }}
              class="!py-2"
            />
          </div>
        </div>
      {/if}

      <!-- Data Table -->
      <DataTable
        columns={collectionColumns}
        data={filteredCollections}
        keyField="id"
        loading={loading}
        emptyMessage="No collections found. Create your first collection to save and reuse work item queries."
        emptyIcon={FolderOpen}
        actionItems={buildCollectionActions}
        onRowClick={(collection) => viewCollection(collection)}
      >
        <div slot="name" let:item={collection}>
          <div>
            <div class="flex items-center gap-2">
              <div class="font-semibold" style="color: var(--ds-text);">{collection.name}</div>
              {#if collection.is_public}
                <span class="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-blue-100 text-blue-800">
                  Public
                </span>
              {/if}
            </div>
            {#if collection.description}
              <div class="text-sm mt-1" style="color: var(--ds-text-subtle);">{collection.description}</div>
            {/if}
          </div>
        </div>

        <div slot="category" let:item={collection}>
          {#if collection.category_name}
            <div class="flex items-center gap-2">
              <ColorDot color={collection.category_color || '#6b7280'} size="sm" />
              <span class="text-sm" style="color: var(--ds-text);">{collection.category_name}</span>
            </div>
          {:else}
            <span class="text-sm" style="color: var(--ds-text-subtle);">—</span>
          {/if}
        </div>

        <div slot="query" let:item={collection}>
          <div class="font-mono text-sm" style="color: var(--ds-text-subtle);">
            {collection.cql_query || 'No query'}
          </div>
        </div>
      </DataTable>
    </div>
  </div>
</div>

<!-- Category Management Modal -->
<CategoryModal
  isOpen={showCategoryModal}
  onClose={() => showCategoryModal = false}
  title="Manage Collection Categories"
  categories={$collectionCategoriesStore}
  onAdd={handleAddCategory}
  onDelete={handleDeleteCategory}
  showColorPicker={true}
/>

<style>
  .line-clamp-2 {
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
</style>
