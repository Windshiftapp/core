<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { UserPlus, Trash2, Shield } from 'lucide-svelte';
  import Button from '../components/Button.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import Select from '../components/Select.svelte';
  import UserPicker from '../pickers/UserPicker.svelte';
  import DataTable from '../components/DataTable.svelte';
  import SearchInput from '../components/SearchInput.svelte';
  import Pagination from '../components/Pagination.svelte';
  import Avatar from '../components/Avatar.svelte';
  import Text from '../components/Text.svelte';
  import Label from '../components/Label.svelte';
  import { confirm } from '../composables/useConfirm.js';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';

  export let workspaceId;

  let members = [];
  let roles = [];
  let everyoneRole = null; // { role_id, role_name, source }
  let everyoneRoleValue = '';
  let loading = true;
  let updatingEveryone = false;
let error = null;
const defaultRoleOrder = ['Viewer', 'Editor', 'Administrator', 'Tester'];

  // Add member modal state
  let showModal = false;
  let selectedUserId = null;
  let selectedRoleId = null;
  let adding = false;

  // Search and pagination state
  let searchQuery = '';
  let currentPage = 1;
  let itemsPerPage = 20;

  onMount(async () => {
    await loadData();
  });

  async function loadData() {
    const workspaceIdNum = Number(workspaceId);
    if (!workspaceIdNum) {
      error = 'Invalid workspace';
      return;
    }
    loading = true;
    error = null;
    try {
      const [membersData, rolesData, everyoneData] = await Promise.all([
        api.workspaceRoles.getWorkspaceAssignments(workspaceIdNum),
        api.workspaceRoles.getAll(),
        api.workspaceRoles.getEveryoneRole(workspaceIdNum)
      ]);
      members = membersData || [];
      roles = rolesData || [];
      everyoneRole = everyoneData || null;
      everyoneRoleValue = everyoneRole?.role_id ?? '';
    } catch (err) {
      console.error('Failed to load workspace members:', err);
      error = err.message || 'Failed to load workspace members';
    } finally {
      loading = false;
    }
  }

  async function handleSubmit() {
    const roleId = selectedRoleId ? Number(selectedRoleId) : null;
    const userId = selectedUserId ? Number(selectedUserId) : null;
    const workspaceIdNum = Number(workspaceId);
    if (!userId || !roleId) {
      return;
    }

    try {
      adding = true;
      await api.workspaceRoles.assignToUser({
        user_id: userId,
        workspace_id: workspaceIdNum,
        role_id: roleId
      });

      // Reset form
      selectedUserId = null;
      selectedRoleId = null;
      showModal = false;

      // Reload data
      await loadData();
    } catch (err) {
      console.error('Failed to add member:', err);
      alert(`Failed to add member: ${err.message}`);
    } finally {
      adding = false;
    }
  }

  async function handleRemoveMemberRole(member, role) {
    const userName = `${member.first_name || ''} ${member.last_name || ''}`.trim() || member.username;
    const confirmed = await confirm(
      `Remove ${role.role_name} role from ${userName}?`,
      'This will revoke this role assignment from the user.'
    );

    if (!confirmed) return;

    try {
      const workspaceIdNum = Number(workspaceId);
      await api.workspaceRoles.revokeFromUser(member.user_id, workspaceIdNum, role.role_id);

      // Reload data
      await loadData();
    } catch (err) {
      console.error('Failed to remove role:', err);
      alert(`Failed to remove role: ${err.message}`);
    }
  }

  function getRoleName(roleId) {
    const role = roles.find(r => r.id === roleId);
    return role?.name || 'Unknown';
  }

  function getEveryoneLabel() {
    if (!everyoneRole) return 'Viewer (default)';
    if (everyoneRole.role_id === null || everyoneRole.role_id === undefined) return 'No access (locked)';
    return everyoneRole.role_name || getRoleName(everyoneRole.role_id) || 'Viewer';
  }

  async function setEveryoneRole(roleId) {
    try {
      updatingEveryone = true;
      const workspaceIdNum = Number(workspaceId);
      await api.workspaceRoles.setEveryoneRole(workspaceIdNum, { role_id: roleId });
      const updated = await api.workspaceRoles.getEveryoneRole(workspaceIdNum);
      everyoneRole = updated;
      everyoneRoleValue = updated?.role_id ?? '';
      // Refresh members to update viewer cascade display
      await loadData();
    } catch (err) {
      console.error('Failed to update Everyone role:', err);
      alert(`Failed to update Everyone role: ${err.message || err}`);
    } finally {
      updatingEveryone = false;
    }
  }

  function getRoleBadgeStyle(roleId) {
    const role = roles.find(r => r.id === roleId);
    if (!role) return 'background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);';

    // Role-specific styling using design system variables
    if (role.name === 'Administrator') {
      return 'background-color: var(--ds-background-accent-purple-subtler); color: var(--ds-accent-purple);';
    } else if (role.name === 'Editor') {
      return 'background-color: var(--ds-accent-blue-subtler); color: var(--ds-accent-blue);';
    } else if (role.name === 'Viewer') {
      return 'background-color: var(--ds-background-accent-green-subtler); color: var(--ds-accent-green);';
    } else if (role.name === 'Tester') {
      return 'background-color: var(--ds-accent-teal-subtler); color: var(--ds-accent-teal);';
    }
    return 'background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);';
  }

  function handleCancel() {
    showModal = false;
    selectedUserId = null;
    selectedRoleId = null;
  }

  // DataTable columns
  const columns = [
    {
      key: 'user',
      label: 'User',
      slot: 'user'
    },
    {
      key: 'roles',
      label: 'Roles',
      slot: 'role'
    },
    {
      key: 'actions',
      label: 'Actions',
      align: 'text-right'
    }
  ];

  function getActionItems(member) {
    // Create a menu item for each role
    return member.roles.map(role => ({
      title: `Remove ${role.role_name}`,
      icon: Trash2,
      onClick: () => handleRemoveMemberRole(member, role),
      hoverClass: 'hover:bg-red-50',
      iconClass: 'text-red-500'
    }));
  }

  // Search filtering
  $: filteredMembers = members.filter(member => {
    if (!searchQuery.trim()) return true;

    const query = searchQuery.toLowerCase();
    return (
      member.first_name?.toLowerCase().includes(query) ||
      member.last_name?.toLowerCase().includes(query) ||
      member.email?.toLowerCase().includes(query) ||
      member.username?.toLowerCase().includes(query)
    );
  });

  // Pagination
  $: paginatedMembers = filteredMembers.slice(
    (currentPage - 1) * itemsPerPage,
    currentPage * itemsPerPage
  );

  // Reset to page 1 when search changes
  $: if (searchQuery) {
    currentPage = 1;
  }

  // Event handlers for pagination
  function handlePageChange(event) {
    currentPage = event.detail.page;
    itemsPerPage = event.detail.itemsPerPage;
  }

  function handlePageSizeChange(event) {
    itemsPerPage = event.detail.itemsPerPage;
    currentPage = 1;
  }
</script>

<div class="space-y-6">
  <!-- Role Summary -->
  <div class="space-y-4 mb-8">
    <div class="flex items-start gap-3">
      <Shield class="w-4 h-4 text-blue-600 mt-0.5" />
      <div>
        <h3 class="text-sm font-semibold" style="color: var(--ds-text);">Summary</h3>
        <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
          Shows effective role assignments for this workspace. Viewers have access to all content; higher roles inherit Viewer permissions.
        </p>
      </div>
    </div>

    <div class="overflow-hidden rounded border" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
      <table class="w-full text-sm">
        <thead>
          <tr style="background-color: var(--ds-interactive-subtle); border-bottom: 1px solid var(--ds-border);">
            <th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider" style="color: var(--ds-text-subtle);">Role</th>
            <th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider" style="color: var(--ds-text-subtle);">Effective Access</th>
            <th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider" style="color: var(--ds-text-subtle);">Members</th>
          </tr>
        </thead>
        <tbody>
          {#each defaultRoleOrder as roleName}
            {#if roles.find(r => r.name === roleName)}
              {@const role = roles.find(r => r.name === roleName)}
              {@const roleMembers = members.filter((m) => m.roles.some(r => r.role_name === roleName))}
              {@const hasExplicitMembers = roleMembers.length > 0}
              {@const isEveryoneRole = everyoneRoleValue && String(everyoneRoleValue) === String(role.id)}
              {@const viewerRole = roles.find((r) => r.name === 'Viewer')}
              {@const everyoneIsViewer = everyoneRoleValue && viewerRole && String(everyoneRoleValue) === String(viewerRole.id)}
              {@const viewerMembers = members.filter((m) => m.roles.some(r => r.role_name === 'Viewer'))}
              {@const hasViewers = viewerMembers.length > 0}
              <tr class="border-t" style="border-color: var(--ds-border);">
                <td class="px-6 py-4">
                  <div class="font-medium" style="color: var(--ds-text);">{roleName}</div>
                  <div class="text-xs mt-1" style="color: var(--ds-text-subtle);">{role.description}</div>
                </td>
                <td class="px-6 py-4">
                  {#if hasExplicitMembers}
                    <span class="inline-flex items-center gap-1 px-2 py-1 rounded-full text-xs" style="background-color: var(--ds-accent-blue-subtler); color: var(--ds-accent-blue);">
                      {roleMembers.length} {roleMembers.length === 1 ? 'member' : 'members'}
                    </span>
                  {:else if isEveryoneRole}
                    <span class="inline-flex items-center gap-1 px-2 py-1 rounded-full text-xs" style="background-color: var(--ds-background-accent-green-subtler); color: var(--ds-accent-green);">
                      Everyone (default)
                    </span>
                  {:else if hasViewers && roleName !== 'Viewer'}
                    <span class="inline-flex items-center gap-1 px-2 py-1 rounded-full text-xs" style="background-color: var(--ds-background-accent-green-subtler); color: var(--ds-accent-green);">
                      All Viewers
                    </span>
                  {:else if everyoneIsViewer && roleName !== 'Viewer'}
                    <span class="inline-flex items-center gap-1 px-2 py-1 rounded-full text-xs" style="background-color: var(--ds-background-accent-green-subtler); color: var(--ds-accent-green);">
                      Everyone (default)
                    </span>
                  {:else}
                    <span class="text-xs" style="color: var(--ds-text-subtle);">—</span>
                  {/if}
                </td>
                <td class="px-6 py-4">
                  {#if hasExplicitMembers}
                    <div class="flex flex-wrap gap-2">
                      {#each roleMembers as member}
                        <span class="inline-flex items-center gap-1 px-2 py-1 rounded-full text-xs" style="background-color: var(--ds-accent-blue-subtler); color: var(--ds-accent-blue);">
                          {member.first_name} {member.last_name}
                        </span>
                      {/each}
                    </div>
                  {:else}
                    <span class="text-xs" style="color: var(--ds-text-subtle);">No direct members</span>
                  {/if}
                </td>
              </tr>
            {/if}
          {/each}
        </tbody>
      </table>
    </div>
  </div>

  <!-- Search Box and Add Member Button -->
  <div class="flex items-center justify-between gap-4">
    <SearchInput
      bind:value={searchQuery}
      placeholder="Search members by name or email..."
      class="flex-1"
    />
    <Button variant="primary" size="medium" onclick={() => showModal = true} keyboardHint="A" hotkeyConfig={{ key: toHotkeyString('workspaceMembers', 'addMember'), guard: () => !showModal }}>
      <UserPlus class="w-4 h-4 mr-2" />
      Add Member
    </Button>
  </div>

  <!-- Members Table -->
  {#if loading}
    <div class="text-center py-12" style="color: var(--ds-text-subtle);">
      Loading workspace members...
    </div>
  {:else if error}
    <div class="text-center py-12 text-red-600">
      {error}
    </div>
  {:else}
    <DataTable
      {columns}
      data={paginatedMembers}
      keyField="user_id"
      emptyMessage="No members yet. Add users to this workspace to grant them access."
      emptyIcon={Shield}
      actionItems={getActionItems}
    >
      <svelte:fragment slot="user" let:item>
        <div class="flex items-center gap-3">
          <Avatar
            src={item.avatar_url}
            firstName={item.first_name}
            lastName={item.last_name}
            size="sm"
            variant="blue"
          />
          <div>
            <Text size="sm" weight="medium">{item.first_name} {item.last_name}</Text>
            <Text size="xs" variant="subtle">{item.email}</Text>
          </div>
        </div>
      </svelte:fragment>

      <svelte:fragment slot="role" let:item>
        <div class="flex flex-wrap gap-2">
          {#each item.roles as role}
            <span class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium" style={getRoleBadgeStyle(role.role_id)}>
              <Shield class="w-3 h-3" />
              {role.role_name}
            </span>
          {/each}
        </div>
      </svelte:fragment>
    </DataTable>

    {#if filteredMembers.length > 0}
      <Pagination
        currentPage={currentPage}
        totalItems={filteredMembers.length}
        itemsPerPage={itemsPerPage}
        pageSizeOptions={[10, 20, 50]}
        onpageChange={handlePageChange}
        onpageSizeChange={handlePageSizeChange}
      />
    {:else if searchQuery.trim()}
      <div class="text-sm text-center py-4" style="color: var(--ds-text-subtle);">
        No members found matching "{searchQuery}"
      </div>
    {/if}
  {/if}
</div>

<!-- Add Member Modal -->
<Modal
  isOpen={showModal}
  onSubmit={handleSubmit}
  submitDisabled={!selectedUserId || !selectedRoleId || adding}
  maxWidth="max-w-2xl"
  onclose={handleCancel}
  let:submitHint
>
  <div class="p-6">
    <h2 class="text-xl font-semibold mb-6" style="color: var(--ds-text);">
      Add Workspace Member
    </h2>

    <div class="space-y-4">
      <div>
        <Label color="default" required class="mb-2">User</Label>
        <UserPicker bind:value={selectedUserId} placeholder="Select user..." />
      </div>

      <div>
        <Label color="default" required class="mb-2">Role</Label>
        <Select
          bind:value={selectedRoleId}
          onchange={(e) => selectedRoleId = e.target.value ? Number(e.target.value) : null}
        >
          <option value={null}>Select role...</option>
          {#each roles as role}
            <option value={role.id}>{role.name} - {role.description}</option>
          {/each}
        </Select>
      </div>
    </div>

    <div class="mt-8 flex gap-3">
      <Button
        variant="primary"
        size="medium"
        onclick={handleSubmit}
        disabled={!selectedUserId || !selectedRoleId || adding}
        keyboardHint={submitHint}
      >
        {adding ? 'Adding...' : 'Add Member'}
      </Button>
      <Button
        variant="default"
        size="medium"
        onclick={handleCancel}
        disabled={adding}
        keyboardHint="Esc"
      >
        Cancel
      </Button>
    </div>
  </div>
</Modal>
