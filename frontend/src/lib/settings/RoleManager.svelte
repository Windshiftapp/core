<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { BadgeCheck, Eye, CheckCircle } from 'lucide-svelte';
  import DataTable from '../components/DataTable.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import AlertBox from '../components/AlertBox.svelte';

  let roles = [];
  let loading = true;
  let selectedRole = null;
  let rolePermissions = [];

  const columns = [
    { key: 'name', label: 'Role Name', sortable: true },
    { key: 'description', label: 'Description' },
    {
      key: 'is_system',
      label: 'Type',
      render: (item) => item.is_system ? 'System' : 'Custom',
      sortable: true
    },
    { key: 'actions', label: '', width: 'w-16' }
  ];

  onMount(async () => {
    await loadRoles();
  });

  async function loadRoles() {
    try {
      loading = true;
      const data = await api.get('/workspace-roles') || [];
      roles = data;
    } catch (error) {
      console.error('Failed to load workspace roles:', error);
      roles = [];
    } finally {
      loading = false;
    }
  }

  async function viewRoleDetails(role) {
    try {
      const fullRole = await api.get(`/workspace-roles/${role.id}`);
      selectedRole = fullRole;
      rolePermissions = fullRole.permissions || [];
    } catch (error) {
      console.error('Failed to load role details:', error);
      alert('Failed to load role details: ' + (error.message || error));
    }
  }

  function closeDetails() {
    selectedRole = null;
    rolePermissions = [];
  }

  function buildRoleDropdownItems(role) {
    return [
      {
        id: 'view',
        label: 'View Permissions',
        icon: Eye,
        action: () => viewRoleDetails(role)
      }
    ];
  }
</script>

<div class="space-y-6">
  <PageHeader
    title="Workspace Roles"
    description="System-defined roles that bundle permissions for common access patterns"
    icon={BadgeCheck}
  />

  {#if selectedRole}
    <div class="border rounded p-6 shadow-sm" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
      <div class="flex justify-between items-start mb-4">
        <div>
          <h3 class="text-lg font-semibold" style="color: var(--ds-text);">{selectedRole.name}</h3>
          <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">{selectedRole.description}</p>
        </div>
        <button
          onclick={closeDetails}
          class="close-btn"
          style="color: var(--ds-icon-subtle);"
        >
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>

      <div class="border-t pt-4" style="border-color: var(--ds-border);">
        <h4 class="font-medium mb-3" style="color: var(--ds-text);">Included Permissions</h4>
        <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
          {#each rolePermissions as permission}
            <div class="flex items-start space-x-2 p-3 rounded-md" style="background-color: var(--ds-interactive-subtle);">
              <CheckCircle class="w-5 h-5 mt-0.5" style="color: var(--ds-text-success);" />
              <div>
                <div class="font-medium text-sm" style="color: var(--ds-text);">{permission.permission_name}</div>
                <div class="text-xs" style="color: var(--ds-text-subtle);">{permission.description}</div>
                <div class="text-xs mt-0.5" style="color: var(--ds-text-subtlest);">{permission.permission_key}</div>
              </div>
            </div>
          {/each}
        </div>
      </div>
    </div>
  {/if}

  <DataTable
    data={roles}
    {columns}
    {loading}
    actionItems={buildRoleDropdownItems}
    emptyMessage="No roles found."
  />

  <AlertBox type="info">
    <h4 class="font-semibold mb-2">About Workspace Roles</h4>
    <p class="text-sm mb-2">
      Workspace roles are predefined permission bundles that make it easy to grant common access levels:
    </p>
    <ul class="text-sm space-y-1 ml-4 list-disc">
      <li><strong>Viewer:</strong> Can view items and add comments</li>
      <li><strong>Editor:</strong> Can view, create, edit items and change their status</li>
      <li><strong>Administrator:</strong> Full workspace administration including permission management</li>
    </ul>
    <p class="text-sm mt-3">
      To assign roles to users, go to Workspace Settings → Members.
    </p>
  </AlertBox>
</div>

<style>
  .close-btn:hover {
    color: var(--ds-icon);
  }
</style>
