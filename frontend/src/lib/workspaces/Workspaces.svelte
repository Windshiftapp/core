<script>
  import { onMount } from 'svelte';
  import { workspaces } from '../api/workspaces.js';
  import { navigate } from '../router.js';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import Edit from '@lucide/svelte/icons/edit';
  import Plus from '@lucide/svelte/icons/plus';
  import Circle from '@lucide/svelte/icons/circle';
  import Grip from '@lucide/svelte/icons/grip';
  import { workspaceIconMap } from '../utils/icons.js';
  import Button from '../components/Button.svelte';
  import DataTable from '../components/DataTable.svelte';
  import SearchInput from '../components/SearchInput.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import { toHotkeyString, getShortcutDisplay } from '../utils/keyboardShortcuts.js';
  import { useDebouncedAction } from '../composables/useDebouncedAction.svelte.js';
  import { workspacesStore } from '../stores/workspaces.svelte.js';
  import { permissionStore, isSystemAdmin } from '../stores/permissions.svelte.js';
  import { formatDateSimple } from '../utils/dateFormatter.js';

  // Props
  let { showPageHeader = true, noPadding = false, showAdminHeader = false } = $props();

  const canCreate = $derived($permissionStore.userPermissionKeys?.has('workspace.create') || $isSystemAdmin);

  // Use centralized icon map for workspace icons
  const iconMap = workspaceIconMap;

  // Server-side directory paging and search (WI-1446). The admin table reads
  // one page at a time and excludes personal workspaces server-side so the
  // pagination total matches the visible rows.
  const pageSize = 50;
  let workspaceRows = $state([]);
  let workspaceTotal = $state(0);
  let currentPage = $state(1);
  let searchQuery = $state('');
  // Guards against a slow earlier request overwriting a newer page/search.
  let loadGeneration = 0;

  async function loadWorkspaceRows(page = currentPage, search = searchQuery) {
    const generation = ++loadGeneration;
    try {
      const document = await workspaces.getPage({
        page,
        page_size: pageSize,
        search,
        personal: false,
      });
      if (generation !== loadGeneration) return;
      workspaceRows = document?.data ?? [];
      workspaceTotal = document?.pagination?.total_items ?? 0;
      currentPage = page;
      searchQuery = search;
    } catch (error) {
      if (generation !== loadGeneration) return;
      console.error('Failed to load workspaces:', error);
      workspaceRows = [];
      workspaceTotal = 0;
    }
  }

  const runSearch = useDebouncedAction((value) => {
    currentPage = 1;
    loadWorkspaceRows(1, value);
  }, 250);

  function onSearchInput() {
    runSearch(searchQuery);
  }

  function onPageChange(page) {
    loadWorkspaceRows(page, searchQuery);
  }

  onMount(() => {
    // Refresh the shared directory cache alongside the admin table so other
    // surfaces pick up workspaces created outside this session.
    workspacesStore.load({ force: true });
    return loadWorkspaceRows();
  });

  function startCreate() {
    window.dispatchEvent(new CustomEvent('show-create-modal', { detail: { type: 'workspace' } }));
  }

  async function deleteWorkspace(workspace) {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('workspaces.confirmDelete', { name: workspace.name }),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger'
    });
    if (confirmed) {
      try {
        await workspaces.delete(workspace.id);
        workspacesStore.remove(workspace.id);
        // Stepping back keeps the table on a populated page when the last row
        // of the final page is removed.
        const targetPage =
          workspaceRows.length === 1 && currentPage > 1 ? currentPage - 1 : currentPage;
        await loadWorkspaceRows(targetPage, searchQuery);
      } catch (error) {
        console.error('Failed to delete workspace:', error);
        errorToast(t('dialogs.alerts.failedToDelete', { error: error.message || error }));
      }
    }
  }

  function getStatusBadgeClass(active) {
    return active
      ? 'bg-ds-success-subtle text-ds-text-success'
      : 'bg-ds-background-neutral text-ds-text-subtle';
  }

  function buildWorkspaceDropdownItems(workspace) {
    // Personal workspaces cannot be edited
    if (workspace.is_personal) {
      return [];
    }

    return [
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: t('common.edit'),
        hoverClass: 'hover-bg',
        onClick: () => navigate(`/workspaces/${workspace.id}`)
      }
      // Delete action removed - workspaces can only be deleted from workspace settings
    ];
  }

  // Table column definitions
  const workspaceColumns = $derived([
    {
      key: 'name',
      label: t('workspaces.workspace'),
      slot: 'name'
    },
    {
      key: 'active',
      label: t('common.status'),
      slot: 'status'
    },
    {
      key: 'visibility',
      label: t('workspaces.visibility'),
      slot: 'visibility'
    },
    {
      key: 'created_at',
      label: t('common.created'),
      sortable: true,
      sortValue: (workspace) => workspace.created_at,
      render: (workspace) => formatDateSimple(workspace.created_at),
      textColor: 'var(--ds-text-subtle)'
    },
    {
      key: 'actions',
      label: t('common.actions')
    }
  ]);


</script>

<div class="min-h-screen" style="background-color: var(--ds-surface);">
    <div class="{noPadding ? '' : 'px-6 pt-6'}">
      <PageHeader
        icon={Grip}
        title={t('workspaces.title')}
        subtitle={t('workspaces.listSubtitle')}
      >
        {#snippet actions()}
          {#if canCreate}
            <Button
              variant="primary"
              dataTestid="workspaces-create"
              icon={Plus}
              onclick={startCreate}
              keyboardHint={getShortcutDisplay('workspaces', 'addWorkspace')}
              hotkeyConfig={{ key: toHotkeyString('workspaces', 'addWorkspace'), guard: () => true }}
            >
              {t('workspaces.createWorkspace')}
            </Button>
          {/if}
        {/snippet}
      </PageHeader>
    </div>


    <div class="{noPadding ? '' : 'px-6 pb-6'}">
      <div class="flex flex-wrap items-end gap-3 mb-4">
        <SearchInput
          bind:value={searchQuery}
          on_input={onSearchInput}
          placeholder={t('nav.searchWorkspaces')}
          class="min-w-[220px] flex-1 max-w-xs"
          dataTestid="workspaces-search"
        />
      </div>
      <DataTable
        columns={workspaceColumns}
        data={workspaceRows}
        keyField="id"
        pagination={true}
        totalItems={workspaceTotal}
        pageSize={pageSize}
        bind:currentPage={currentPage}
        onPageChange={onPageChange}
        emptyMessage={t('workspaces.empty')}
        emptyIcon={Circle}
        actionItems={buildWorkspaceDropdownItems}
        onRowClick={(workspace) => navigate(`/workspaces/${workspace.id}`)}
        rowAttrs={(workspace) => ({ 'data-testid': `workspace-row-${workspace.id}` })}
      >
    {#snippet name(workspace)}
      {@const WorkspaceIcon = iconMap[workspace.icon] || Grip}
      <a
        href={`/workspaces/${workspace.id}`}
        class="flex items-center gap-3 no-underline"
        style="color: inherit;"
      >
        <!-- Workspace Visual Identity -->
        {#if workspace.avatar_url}
          <img src={workspace.avatar_url} alt="{workspace.name} avatar" class="w-8 h-8 rounded-md object-cover flex-shrink-0" />
        {:else}
          <div class="w-8 h-8 rounded-md flex items-center justify-center flex-shrink-0" style="background-color: {workspace.color || '#3b82f6'};">
            <WorkspaceIcon size={16} color="white" />
          </div>
        {/if}

        <div class="flex-1 min-w-0">
          <div class="flex items-center gap-2">
            <div style="color: var(--ds-text);">{workspace.name}</div>
            {#if workspace.is_personal}
              <span class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-ds-accent-purple-subtle text-ds-text-accent-purple">
                {t('workspaces.personal')}
              </span>
            {/if}
            {#if workspace.is_template}
              <span
                data-testid={`workspace-template-badge-${workspace.id}`}
                class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium"
                style="background-color: var(--ds-accent-blue-subtle); color: var(--ds-text-accent-blue);"
              >
                {t('workspaces.template')}
              </span>
            {/if}
          </div>
          {#if workspace.description}
            <div class="text-sm mt-1" style="color: var(--ds-text-subtle);">{workspace.description}</div>
          {/if}
        </div>
      </a>
    {/snippet}

    {#snippet status(workspace)}
      <Lozenge color={workspace.active ? 'green' : 'gray'} text={workspace.active ? 'Active' : 'Inactive'} />
    {/snippet}

    {#snippet visibility(workspace)}
      {#if workspace.is_restricted}
        <Lozenge
          color="orange"
          text={t('workspaces.restricted')}
          dataTestid={`workspace-restricted-badge-${workspace.id}`}
        />
      {:else}
        <Lozenge
          color="gray"
          text={t('workspaces.open')}
          dataTestid={`workspace-open-${workspace.id}`}
        />
      {/if}
    {/snippet}
  </DataTable>
    </div>
</div>
