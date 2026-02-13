<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { navigate } from '../router.js';
  import { Edit, Plus, Circle, Grip } from 'lucide-svelte';
  import { workspaceIconMap } from '../utils/icons.js';
  import Button from '../components/Button.svelte';
  import DataTable from '../components/DataTable.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import { toHotkeyString, getShortcutDisplay } from '../utils/keyboardShortcuts.js';
  import { workspacesStore } from '../stores';

  // Props
  export let showPageHeader = true; // Whether to show admin header and use admin layout
  export let noPadding = false;

  // Use centralized icon map for workspace icons
  const iconMap = workspaceIconMap;

  onMount(async () => {
    // Load workspaces from store
    await workspacesStore.load();
  });

  function startCreate() {
    window.dispatchEvent(new CustomEvent('set-create-type', { detail: { type: 'workspace' } }));
    window.dispatchEvent(new CustomEvent('open-create-modal'));
  }

  async function deleteWorkspace(workspace) {
    if (confirm(`Are you sure you want to delete workspace "${workspace.name}"? This will affect all associated projects.`)) {
      try {
        await api.workspaces.delete(workspace.id);
        await workspacesStore.reload();
      } catch (error) {
        console.error('Failed to delete workspace:', error);
        alert('Failed to delete workspace: ' + (error.message || error));
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
        title: 'Edit',
        hoverClass: 'hover-bg',
        onClick: () => navigate(`/workspaces/${workspace.id}`)
      }
      // Delete action removed - workspaces can only be deleted from workspace settings
    ];
  }

  // Table column definitions
  const workspaceColumns = [
    {
      key: 'name',
      label: 'Workspace',
      slot: 'name'
    },
    {
      key: 'active',
      label: 'Status',
      slot: 'status'
    },
    {
      key: 'created_at',
      label: 'Created',
      render: (workspace) => new Date(workspace.created_at).toLocaleDateString(),
      textColor: 'var(--ds-text-subtle)'
    },
    {
      key: 'actions',
      label: 'Actions'
    }
  ];


</script>

<div class="min-h-screen" style="background-color: var(--ds-surface);">
    <div class="{noPadding ? '' : 'px-6 pt-6'}">
      <PageHeader
        icon={Grip}
        title="Workspaces"
        subtitle="Organize and manage your projects within workspaces"
      >
        {#snippet actions()}
          <Button
            variant="primary"
            icon={Plus}
            onclick={startCreate}
            keyboardHint={getShortcutDisplay('workspaces', 'addWorkspace')}
            hotkeyConfig={{ key: toHotkeyString('workspaces', 'addWorkspace'), guard: () => true }}
          >
            Add Workspace
          </Button>
        {/snippet}
      </PageHeader>
    </div>


    <div class="{noPadding ? '' : 'px-6 pb-6'}">
      <DataTable
        columns={workspaceColumns}
        data={$workspacesStore.regularWorkspaces}
        keyField="id"
        emptyMessage="No workspaces found. Create your first workspace to get started."
        emptyIcon={Circle}
        actionItems={buildWorkspaceDropdownItems}
        onRowClick={(workspace) => navigate(`/workspaces/${workspace.id}`)}
      >
    <div slot="name" let:item={workspace}>
      <div class="flex items-center gap-3">
        <!-- Workspace Visual Identity -->
        {#if workspace.avatar_url}
          <img src={workspace.avatar_url} alt="{workspace.name} avatar" class="w-8 h-8 rounded-md object-cover flex-shrink-0" />
        {:else}
          <div class="w-8 h-8 rounded-md flex items-center justify-center flex-shrink-0" style="background-color: {workspace.color || '#3b82f6'};">
            <svelte:component this={iconMap[workspace.icon] || Grip} size={16} color="white" />
          </div>
        {/if}

        <div class="flex-1 min-w-0">
          <div class="flex items-center gap-2">
            <div class="font-semibold" style="color: var(--ds-text);">{workspace.name}</div>
            {#if workspace.is_personal}
              <span class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-purple-100 text-purple-800">
                Personal
              </span>
            {/if}
          </div>
          {#if workspace.description}
            <div class="text-sm mt-1" style="color: var(--ds-text-subtle);">{workspace.description}</div>
          {/if}
        </div>
      </div>
    </div>

    <Lozenge slot="status" let:item={workspace} color={workspace.active ? 'green' : 'gray'} text={workspace.active ? 'Active' : 'Inactive'} />
  </DataTable>
    </div>
</div>
