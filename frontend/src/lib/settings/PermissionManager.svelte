<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';
  import { Shield, Users as UsersIcon, Plus, X, User, Crown } from 'lucide-svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import AssigneePicker from '../pickers/AssigneePicker.svelte';
  import Spinner from '../components/Spinner.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import DataTable from '../components/DataTable.svelte';
  import Button from '../components/Button.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';

  let permissions = $state([]);
  let users = $state([]);
  let groups = $state([]);
  let userPermissions = $state(new Map()); // Map of userId -> Set of permissionIds
  let groupPermissions = $state(new Map()); // Map of groupId -> Set of permissionIds
  let loading = $state(false);
  let error = $state('');
  let success = $state('');

  // Assign modal state
  let assignModalOpen = $state(false);
  let assignTarget = $state(null); // permission being assigned to
  let assignType = $state('user');
  let assignUserId = $state(null);
  let assignGroupId = $state(null);

  onMount(async () => {
    await loadData();
  });

  async function loadData() {
    loading = true;
    error = '';

    try {
      // Load permissions, users, and groups in parallel
      await Promise.all([
        loadPermissions(),
        loadUsers(),
        loadGroups()
      ]);

      // Load user and group permissions after data is loaded
      await loadAllUserPermissions();
      await loadAllGroupPermissions();
    } catch (err) {
      error = t('settings.permissions.failedToLoadData') + err.message;
    } finally {
      loading = false;
    }
  }

  async function loadPermissions() {
    permissions = await api.permissions.getAll();
  }

  async function loadUsers() {
    users = await api.getUsers();
  }

  async function loadGroups() {
    groups = await api.groups.getAll();
  }

  async function loadAllUserPermissions() {
    userPermissions = new Map();

    // Load permissions for each user
    for (const user of users) {
      try {
        const userPerms = await api.permissions.getUserPermissions(user.id);
        const globalPermissionIds = new Set(
          (userPerms.global_permissions || []).map(p => p.permission_id)
        );
        userPermissions.set(user.id, globalPermissionIds);
      } catch (err) {
        console.warn(`Failed to load permissions for user ${user.id}:`, err);
        userPermissions.set(user.id, new Set());
      }
    }
    // Trigger reactivity
    userPermissions = userPermissions;
  }

  async function loadAllGroupPermissions() {
    groupPermissions = new Map();

    try {
      // Fetch all group permissions from backend
      const allGroupPerms = await api.permissions.getAllGroupPermissions();

      // Defensive check: ensure response is an array
      if (!Array.isArray(allGroupPerms)) {
        console.warn('Failed to load group permissions: response is not an array', allGroupPerms);
        return;
      }

      // Build map of groupId -> Set of permissionIds
      for (const gp of allGroupPerms) {
        if (!groupPermissions.has(gp.group_id)) {
          groupPermissions.set(gp.group_id, new Set());
        }
        groupPermissions.get(gp.group_id).add(gp.permission_id);
      }

      // Trigger reactivity
      groupPermissions = groupPermissions;
    } catch (err) {
      console.warn('Failed to load group permissions:', err);
    }
  }

  function getGlobalPermissions() {
    // Hide user.list - any authenticated user can list users (needed for mentions/assignments)
    // The permission is kept in backend but hidden from management UI
    const hiddenPermissions = ['user.list'];
    return permissions.filter(p => p.scope === 'global' && !hiddenPermissions.includes(p.permission_key));
  }

  function getUsersWithPermission(permissionId) {
    return users.filter(user =>
      userPermissions.get(user.id)?.has(permissionId)
    );
  }

  function getGroupsWithPermission(permissionId) {
    return groups.filter(group =>
      groupPermissions.get(group.id)?.has(permissionId)
    );
  }

  function openAssignModal(permission) {
    assignTarget = permission;
    assignType = 'user';
    assignUserId = null;
    assignGroupId = null;
    assignModalOpen = true;
  }

  function closeAssignModal() {
    assignModalOpen = false;
    assignTarget = null;
    assignUserId = null;
    assignGroupId = null;
    assignType = 'user';
  }

  async function grantPermission() {
    if (!assignTarget) return;
    const permissionId = assignTarget.id;

    try {
      if (assignType === 'user' && assignUserId) {
        await api.permissions.grantGlobal({
          user_id: assignUserId,
          permission_id: permissionId,
        });

        success = t('settings.permissions.permissionGrantedToUser');

        if (!userPermissions.has(assignUserId)) {
          userPermissions.set(assignUserId, new Set());
        }
        userPermissions.get(assignUserId).add(permissionId);
        userPermissions = new Map(userPermissions);
      } else if (assignType === 'group' && assignGroupId) {
        await api.permissions.grantGlobalToGroup({
          group_id: assignGroupId,
          permission_id: permissionId,
        });

        success = t('settings.permissions.permissionGrantedToGroup');

        if (!groupPermissions.has(assignGroupId)) {
          groupPermissions.set(assignGroupId, new Set());
        }
        groupPermissions.get(assignGroupId).add(permissionId);
        groupPermissions = new Map(groupPermissions);

        await loadAllUserPermissions();
      }

      setTimeout(() => success = '', 3000);
      closeAssignModal();
    } catch (err) {
      error = t('settings.permissions.failedToGrantPermission') + err.message;
      setTimeout(() => error = '', 5000);
    }
  }

  async function revokePermissionFromUser(userId, permissionId, permissionKey) {
    // Prevent revoking system admin from the last admin
    if (permissionKey === 'system.admin') {
      const admins = getUsersWithPermission(permissionId);
      if (admins.length <= 1) {
        error = t('settings.permissions.cannotRevokeLastAdmin');
        setTimeout(() => error = '', 5000);
        return;
      }
    }

    try {
      await api.permissions.revokeGlobal(userId, permissionId);

      success = t('settings.permissions.permissionRevokedFromUser');
      setTimeout(() => success = '', 3000);

      // Update local state
      if (userPermissions.has(userId)) {
        userPermissions.get(userId).delete(permissionId);
        userPermissions = new Map(userPermissions);
      }
    } catch (err) {
      error = t('settings.permissions.failedToRevokePermission') + err.message;
      setTimeout(() => error = '', 5000);
    }
  }

  async function revokePermissionFromGroup(groupId, permissionId) {
    try {
      await api.permissions.revokeGlobalFromGroup(groupId, permissionId);

      success = t('settings.permissions.permissionRevokedFromGroup');
      setTimeout(() => success = '', 3000);

      // Update local state
      if (groupPermissions.has(groupId)) {
        groupPermissions.get(groupId).delete(permissionId);
        groupPermissions = new Map(groupPermissions);
      }

      // Refresh user permissions to update inherited permissions
      await loadAllUserPermissions();
    } catch (err) {
      error = t('settings.permissions.failedToRevokePermissionFromGroup') + err.message;
      setTimeout(() => error = '', 5000);
    }
  }

  function getUserDisplayName(user) {
    return `${user.first_name} ${user.last_name}`;
  }

  function getGroupDisplayName(group) {
    return group.name;
  }

  const globalPermissions = $derived(getGlobalPermissions());

  const columns = [
    { key: 'permission_name', label: t('settings.permissions.permission'), slot: 'permission' },
    { key: 'description', label: t('common.description') },
    { key: 'users', label: t('settings.permissions.assignedUsers'), slot: 'users' },
    { key: 'groups', label: t('settings.permissions.assignedGroups'), slot: 'groups' },
    { key: 'actions', label: t('common.actions'), slot: 'actions', width: 'w-32' }
  ];
</script>

<div>
  <PageHeader
    icon={Shield}
    title={t('settings.permissions.title')}
    subtitle={t('settings.permissions.subtitle')}
  />

  {#if error}
    <div class="mb-6">
      <AlertBox type="error">{error}</AlertBox>
    </div>
  {/if}

  {#if success}
    <div class="mb-6">
      <AlertBox type="success">{success}</AlertBox>
    </div>
  {/if}

  {#if loading}
    <div class="flex items-center justify-center py-12">
      <Spinner size="lg" />
      <span class="ml-3" style="color: var(--ds-text-subtle);">{t('settings.permissions.loadingPermissions')}</span>
    </div>
  {:else}
    <!-- Global Permissions Table -->
    <h2 class="text-xl font-semibold mb-4" style="color: var(--ds-text);">{t('settings.permissions.globalPermissions')}</h2>
    <DataTable
      {columns}
      data={globalPermissions}
      keyField="id"
      emptyMessage={t('settings.permissions.noUsersAssigned')}
    >
      {#snippet permission(item)}
        <div class="text-sm font-medium flex items-center gap-2" style="color: var(--ds-text);">
          {item.permission_name}
          {#if item.is_system}
            <Crown class="w-4 h-4" style="color: var(--ds-text-warning);" title={t('settings.permissions.systemPermission')} />
          {/if}
        </div>
        <div class="text-xs" style="color: var(--ds-text-subtle);">{item.permission_key}</div>
      {/snippet}

      {#snippet users(item)}
        <div class="flex flex-wrap gap-1">
          {#each getUsersWithPermission(item.id) as user}
            <Lozenge color="blue" size="md">
              <User class="w-3 h-3" />
              {getUserDisplayName(user)}
              {#if user.is_system_admin}
                <Crown class="w-3 h-3" style="color: var(--ds-text-warning);" />
              {/if}
              <button
                class="ml-1 revoke-btn"
                onclick={() => revokePermissionFromUser(user.id, item.id, item.permission_key)}
                title={t('settings.permissions.revokePermission')}
                disabled={item.permission_key === 'system.admin' && getUsersWithPermission(item.id).length <= 1}
              >
                <X class="w-3 h-3" />
              </button>
            </Lozenge>
          {:else}
            <span class="text-sm italic" style="color: var(--ds-text-subtle);">{t('settings.permissions.noUsersAssigned')}</span>
          {/each}
        </div>
      {/snippet}

      {#snippet groups(item)}
        <div class="flex flex-wrap gap-1">
          {#each getGroupsWithPermission(item.id) as group}
            <Lozenge color="purple" size="md">
              <UsersIcon class="w-3 h-3" />
              {getGroupDisplayName(group)}
              <button
                class="ml-1 revoke-btn"
                onclick={() => revokePermissionFromGroup(group.id, item.id)}
                title={t('settings.permissions.revokePermissionFromGroup')}
              >
                <X class="w-3 h-3" />
              </button>
            </Lozenge>
          {:else}
            <span class="text-sm italic" style="color: var(--ds-text-subtle);">{t('settings.permissions.noGroupsAssigned')}</span>
          {/each}
        </div>
      {/snippet}

      {#snippet actions(item)}
        <Button variant="default" size="small" icon={Plus} onclick={() => openAssignModal(item)}>
          {t('settings.permissions.assign')}
        </Button>
      {/snippet}
    </DataTable>
  {/if}
</div>

<Modal bind:isOpen={assignModalOpen} onclose={closeAssignModal} maxWidth="max-w-2xl">
  {#if assignTarget}
    <ModalHeader
      title={t('settings.permissions.assignPermission', { permission: assignTarget.permission_name })}
      onClose={closeAssignModal}
    />
    <div class="px-6 py-4">
      <AssigneePicker
        bind:type={assignType}
        bind:userId={assignUserId}
        bind:groupId={assignGroupId}
        confirmText={t('settings.permissions.grantPermission')}
        cancelText={t('common.cancel')}
        on_confirm={grantPermission}
        on_cancel={closeAssignModal}
      />
    </div>
  {/if}
</Modal>

<style>
  .revoke-btn {
    opacity: 0.7;
    transition: opacity 0.15s, color 0.15s;
  }

  .revoke-btn:hover {
    opacity: 1;
    color: var(--ds-text-danger);
  }
</style>
