<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { navigate } from '../router.js';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import { Edit, Plus, Circle, Grip } from '@lucide/svelte';
  import { workspaceIconMap } from '../utils/icons.js';
  import Button from '../components/Button.svelte';
  import DataTable from '../components/DataTable.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import { toHotkeyString, getShortcutDisplay } from '../utils/keyboardShortcuts.js';
  import { workspacesStore, permissionStore, isSystemAdmin } from '../stores';
  import { formatDateSimple } from '../utils/dateFormatter.js';

  // Props
  let { showPageHeader = true, noPadding = false, showAdminHeader = false } = $props();

  const canCreate = $derived($permissionStore.userPermissionKeys?.has('workspace.create') || $isSystemAdmin);

  // Use centralized icon map for workspace icons
  const iconMap = workspaceIconMap;

  onMount(async () => {
    // Load workspaces from store
    await workspacesStore.load();
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
        await api.workspaces.delete(workspace.id);
        await workspacesStore.reload();
      } catch (error) {
        console.error('Failed to delete workspace:', error);
        errorToast(t('dialogs.alerts.failedToDelete', { error: error.message || error }));
      }
    }
  }

  function getStatusBadgeClass(active) {
    return active
      ? 'bg-green-100 text-green-800'
      : 'bg-gray-100 text-gray-800';
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
      key: 'created_at',
      label: t('common.created'),
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
      <DataTable
        columns={workspaceColumns}
        data={$workspacesStore.regularWorkspaces}
        keyField="id"
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
              <span class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-purple-100 text-purple-800">
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
  </DataTable>
    </div>
</div>
