<script>
  import { onMount } from 'svelte';
  import { useEventListener } from 'runed';
  import { api } from '../../api.js';
  import { navigate, currentRoute } from '../../router.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { confirm } from '../../composables/useConfirm.js';
  import { FolderOpen, Plus, Eye, Trash2 } from 'lucide-svelte';
  import Button from '../../components/Button.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import CategoryModal from '../../dialogs/CategoryModal.svelte';
  import CollectionsNavigation from '../collections/CollectionsNavigation.svelte';
  import { collectionCategoriesStore } from '../../stores/collectionCategories.js';
  import { formatDate } from '../../utils/dateFormatter.js';
  import { toHotkeyString } from '../../utils/keyboardShortcuts.js';
  import { workspacesStore } from '../../stores';
  import WorkspaceSelector from '../../workspaces/WorkspaceSelector.svelte';
  import ColorDot from '../../components/ColorDot.svelte';

  let collections = $state([]);
  let loading = $state(true);
  let selectedWorkspaceFilter = $state(null);

  // Category management modal
  let showCategoryModal = $state(false);

  // Determine view based on URL
  let activeCategoryId = $derived($currentRoute.params?.categoryId || null);
  let isWorkspaceView = $derived($currentRoute.path?.includes('/workspace'));

  // Separate collections by type
  let workspaceCollections = $derived(collections.filter(c => c.workspace_id));
  let globalCollections = $derived(collections.filter(c => !c.workspace_id));

  // Filter based on current view
  let filteredCollections = $derived.by(() => {
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
  });

  // Dynamic page title
  let pageTitle = $derived.by(() => {
    if (isWorkspaceView) {
      return t('collections.workspaceCollectionsTitle');
    } else if (activeCategoryId) {
      const category = collectionCategoriesStore.getById(parseInt(activeCategoryId), $collectionCategoriesStore);
      return category ? t('collections.categoryCollections', { category: category.name }) : t('collections.categoryCollections', { category: '' });
    }
    return t('collections.allGlobalCollections');
  });

  const getWorkspaceId = (workspaceId) =>
    typeof workspaceId === 'string' ? parseInt(workspaceId, 10) : workspaceId;

  // Column definitions for DataTable
  let baseCollectionColumns = $derived([
    {
      key: 'name',
      label: t('collections.collection'),
      slot: 'name'
    },
    {
      key: 'cql_query',
      label: t('collections.queryColumn'),
      slot: 'query'
    },
    {
      key: 'created_at',
      label: t('collections.created'),
      render: (collection) => formatDate(collection.created_at) || '-',
      textColor: 'var(--ds-text-subtle)'
    },
    {
      key: 'actions',
      label: t('collections.actions')
    }
  ]);

  let workspaceColumn = $derived({
    key: 'workspace',
    label: t('workspaces.workspace'),
    render: (collection) => getWorkspaceName(collection.workspace_id) || '—'
  });

  let categoryColumn = $derived({
    key: 'category',
    label: t('common.category'),
    slot: 'category'
  });

  let collectionColumns = $derived(isWorkspaceView
    ? [baseCollectionColumns[0], workspaceColumn, ...baseCollectionColumns.slice(1)]
    : (!activeCategoryId
      ? [baseCollectionColumns[0], categoryColumn, ...baseCollectionColumns.slice(1)]
      : baseCollectionColumns));

  let workspaceOptions = $derived(($workspacesStore?.allWorkspaces || []).filter(ws => !ws.is_personal));
  let workspaceMap = $derived(new Map(workspaceOptions.map(ws => [ws.id, ws])));

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
  });

  useEventListener(() => document, 'manage-collection-categories', handleManageCategoriesEvent);

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
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('collections.confirmDeleteCollection', { name: collection.name }),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger'
    });
    if (!confirmed) return;

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
        title: t('collections.viewCollection'),
        onClick: () => viewCollection(collection)
      },
      { type: 'divider' },
      {
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: t('common.delete'),
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
            {filteredCollections.length === 1 ? t('collections.collectionCount', { count: filteredCollections.length }) : t('collections.collectionCountPlural', { count: filteredCollections.length })}
          </p>
        </div>
        <div class="flex-shrink-0">
          <Button
            onclick={createNewCollection}
            variant="primary"
            icon={Plus}
            keyboardHint="A"
            hotkeyConfig={{ key: toHotkeyString('collections', 'add'), guard: () => true }}
          >
            {t('collections.newCollection')}
          </Button>
        </div>
      </div>

      <!-- Workspace filter (only shown in workspace view) -->
      {#if isWorkspaceView}
        <div class="flex flex-wrap items-center gap-3 mb-6">
          <label class="text-sm font-medium" style="color: var(--ds-text-subtle);">{t('collections.workspaceFilter')}</label>
          <div class="min-w-[260px]">
            <WorkspaceSelector
              value={selectedWorkspaceFilter}
              workspaces={workspaceOptions}
              placeholder={t('collections.allWorkspaces')}
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
        emptyMessage={t('collections.noCollectionsFound')}
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
                  {t('collections.public')}
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
            {collection.cql_query || t('collections.noQuery')}
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
  title={t('collections.manageCategories')}
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
