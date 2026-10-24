<script>
  import { onMount } from 'svelte';
  import { useEventListener } from 'runed';
  import { collections as collectionsApi } from '../../api/collections.js';
  import { navigate, currentRoute } from '../../router.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast } from '../../stores/toasts.svelte.js';
  import { confirm } from '../../composables/useConfirm.js';
  import FolderOpen from '@lucide/svelte/icons/folder-open';
  import Plus from '@lucide/svelte/icons/plus';
  import Eye from '@lucide/svelte/icons/eye';
  import Pencil from '@lucide/svelte/icons/pencil';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import Button from '../../components/Button.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import CategoryModal from '../../dialogs/CategoryModal.svelte';
  import CollectionsNavigation from '../collections/CollectionsNavigation.svelte';
  import { collectionCategoriesStore } from '../../stores/collectionCategories.js';
  import { formatDate } from '../../utils/dateFormatter.js';
  import { toHotkeyString } from '../../utils/keyboardShortcuts.js';
  import { workspacesStore } from '../../stores/workspaces.svelte.js';
  import { authStore } from '../../stores/auth.svelte.js';
  import WorkspaceSelector from '../../workspaces/WorkspaceSelector.svelte';
  import ColorDot from '../../components/ColorDot.svelte';
  import Badge from '../../components/Badge.svelte';
  import PageHeader from '../../layout/PageHeader.svelte';

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
      slot: 'name',
      sortable: true,
      sortValue: (c) => c.name
    },
    {
      key: 'ql_query',
      label: t('collections.queryColumn'),
      slot: 'query'
    },
    {
      key: 'created_at',
      label: t('collections.created'),
      render: (collection) => formatDate(collection.created_at) || '-',
      textColor: 'var(--ds-text-subtle)',
      sortable: true
    },
    {
      key: 'actions',
      label: t('collections.actions')
    }
  ]);

  let workspaceColumn = $derived({
    key: 'workspace',
    label: t('workspaces.workspace'),
    render: (collection) => getWorkspaceName(collection.workspace_id) || '—',
    sortable: true,
    sortValue: (collection) => getWorkspaceName(collection.workspace_id) || ''
  });

  let categoryColumn = $derived({
    key: 'category',
    label: t('common.category'),
    slot: 'category',
    sortable: true,
    sortValue: (collection) => collection.category_name || ''
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
      collections = await collectionsApi.getAll() || [];
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
    if (collection.workspace_id) {
      navigate(`/workspaces/${collection.workspace_id}/collections/${collection.id}/board`);
    } else {
      navigate(`/collections/${collection.id}/board`);
    }
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
      await collectionsApi.delete(collection.id);
      await loadCollections();
    } catch (error) {
      console.error('Failed to delete collection:', error);
      errorToast(t('dialogs.alerts.failedToDelete', { error: error.message || error }));
    }
  }

  function editCollection(collection) {
    if (collection.workspace_id) {
      navigate(`/workspaces/${collection.workspace_id}/collections/${collection.id}`);
    } else {
      navigate(`/collections/${collection.id}`);
    }
  }

  let currentUserId = $derived(authStore.currentUser?.id ?? null);

  function canManageCollection(collection) {
    return collection.created_by != null && String(collection.created_by) === String(currentUserId);
  }

  function buildCollectionActions(collection) {
    /** @type {Array<Record<string, any>>} */
    const actions = [
      {
        id: 'view',
        testid: 'collection-action-view',
        type: 'regular',
        icon: Eye,
        title: t('collections.viewCollection'),
        onClick: () => viewCollection(collection)
      }
    ];
    if (!canManageCollection(collection)) return actions;
    actions.push(
      {
        id: 'edit',
        testid: 'collection-action-edit',
        type: 'regular',
        icon: Pencil,
        title: t('collections.editCollection'),
        onClick: () => editCollection(collection)
      },
      { type: 'divider' },
      {
        id: 'delete',
        testid: 'collection-action-delete',
        type: 'regular',
        icon: Trash2,
        title: t('common.delete'),
        color: 'var(--ds-text-danger)',
        hoverClass: 'hover-danger',
        onClick: () => deleteCollection(collection)
      }
    );
    return actions;
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

<div class="flex h-full min-h-0 overflow-hidden" style="background-color: var(--ds-surface);">
  <CollectionsNavigation />

  <div class="flex-1 min-h-0 overflow-y-auto">
    <div class="p-6">
      <!-- Header -->
      <PageHeader
        title={pageTitle}
        subtitle={filteredCollections.length === 1
          ? t('collections.collectionCount', { count: filteredCollections.length })
          : t('collections.collectionCountPlural', { count: filteredCollections.length })}
      >
        {#snippet actions()}
          <Button
            dataTestid="collection-create"
            onclick={createNewCollection}
            variant="primary"
            icon={Plus}
            keyboardHint="A"
            hotkeyConfig={{ key: toHotkeyString('collections', 'add'), guard: () => true }}
          >
            {t('collections.newCollection')}
          </Button>
        {/snippet}
      </PageHeader>

      <!-- Workspace filter (only shown in workspace view) -->
      {#if isWorkspaceView}
        <div class="flex flex-wrap items-center gap-3 mb-6">
          <span class="text-sm font-medium" style="color: var(--ds-text-subtle);">{t('collections.workspaceFilter')}</span>
          <div class="min-w-[260px]">
            <WorkspaceSelector
              value={selectedWorkspaceFilter}
              workspaces={workspaceOptions}
              placeholder={t('collections.allWorkspaces')}
              allowClear={true}
              onSelect={(event) => {
                selectedWorkspaceFilter = event?.id || null;
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
        emptyMessage={t('collections.noCollectionsTitle')}
        emptyDescription={t('collections.noCollectionsFound')}
        emptyIcon={FolderOpen}
        actionItems={buildCollectionActions}
        actionTriggerTestid={(collection) => `collection-actions-${collection.id}`}
        onRowClick={(collection) => viewCollection(collection)}
        rowAttrs={(collection) => ({ 'data-testid': `collection-row-${collection.id}` })}
      >
        {#snippet name(collection)}
          {@const href = collection.workspace_id
            ? `/workspaces/${collection.workspace_id}/collections/${collection.id}/board`
            : `/collections/${collection.id}/board`}
          <a href={href} class="block no-underline" style="color: inherit;">
            <div class="flex items-center gap-2">
              <div style="color: var(--ds-text);">{collection.name}</div>
              {#if collection.is_private}
                <Badge variant="default">{t('collections.private')}</Badge>
              {:else if collection.is_public}
                <Badge variant="info">{t('collections.public')}</Badge>
              {/if}
            </div>
            {#if collection.description}
              <div class="text-sm mt-1" style="color: var(--ds-text-subtle);">{collection.description}</div>
            {/if}
          </a>
        {/snippet}

        {#snippet category(collection)}
          {#if collection.category_name}
            <div class="flex items-center gap-2">
              <ColorDot color={collection.category_color || '#6b7280'} size="sm" />
              <span class="text-sm" style="color: var(--ds-text);">{collection.category_name}</span>
            </div>
          {:else}
            <span class="text-sm" style="color: var(--ds-text-subtle);">—</span>
          {/if}
        {/snippet}

        {#snippet query(collection)}
          <div class="font-mono text-sm" style="color: var(--ds-text-subtle);">
            {collection.ql_query || t('collections.noQuery')}
          </div>
        {/snippet}
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
