<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';
  import { Shield, Users as UsersIcon, Plus, X, AlertCircle, Check, User, Crown } from 'lucide-svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import AssigneePicker from '../pickers/AssigneePicker.svelte';
  import Spinner from '../components/Spinner.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import Lozenge from '../components/Lozenge.svelte';

  let permissions = $state([]);
  let users = $state([]);
  let groups = $state([]);
  let userPermissions = $state(new Map()); // Map of userId -> Set of permissionIds
  let groupPermissions = $state(new Map()); // Map of groupId -> Set of permissionIds
  let loading = $state(false);
  let error = $state('');
  let success = $state('');

  // State for adding permissions - use $state for reactivity
  let permissionState = $state({}); // permissionId -> { showForm, type, selectedUserId, selectedGroupId }

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

  function toggleAddAssignee(permissionId) {
    if (!permissionState[permissionId]) {
      permissionState[permissionId] = {
        showForm: true,
        type: 'user',
        selectedUserId: null,
        selectedGroupId: null
      };
    } else {
      permissionState[permissionId].showForm = !permissionState[permissionId].showForm;
      if (!permissionState[permissionId].showForm) {
        permissionState[permissionId].selectedUserId = null;
        permissionState[permissionId].selectedGroupId = null;
        permissionState[permissionId].type = 'user';
      }
    }
  }

  async function grantPermission(permissionId) {
    const state = permissionState[permissionId];
    if (!state) return;

    const type = state.type || 'user';
    const userId = state.selectedUserId;
    const groupId = state.selectedGroupId;

    try {
      if (type === 'user' && userId) {
        await api.permissions.grantGlobal({
          user_id: userId,
          permission_id: permissionId,
        });

        success = t('settings.permissions.permissionGrantedToUser');

        // Update local state
        if (!userPermissions.has(userId)) {
          userPermissions.set(userId, new Set());
        }
        userPermissions.get(userId).add(permissionId);
        userPermissions = new Map(userPermissions);
      } else if (type === 'group' && groupId) {
        await api.permissions.grantGlobalToGroup({
          group_id: groupId,
          permission_id: permissionId,
        });

        success = t('settings.permissions.permissionGrantedToGroup');

        // Update local state
        if (!groupPermissions.has(groupId)) {
          groupPermissions.set(groupId, new Set());
        }
        groupPermissions.get(groupId).add(permissionId);
        groupPermissions = new Map(groupPermissions);

        // Refresh user permissions to show inherited permissions
        await loadAllUserPermissions();
      }

      setTimeout(() => success = '', 3000);
      toggleAddAssignee(permissionId);
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
    <div class="rounded shadow overflow-hidden" style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);">
      <div class="px-6 py-4" style="border-bottom: 1px solid var(--ds-border); background-color: var(--ds-interactive-subtle);">
        <h2 class="text-xl font-semibold" style="color: var(--ds-text);">{t('settings.permissions.globalPermissions')}</h2>
      </div>

      <div class="overflow-x-auto">
        <table class="min-w-full" style="border-collapse: separate; border-spacing: 0;">
          <thead style="background-color: var(--ds-interactive-subtle);">
            <tr>
              <th class="px-6 py-3 text-left text-xs font-semibold tracking-wide" style="color: var(--ds-text); border-bottom: 1px solid var(--ds-border);">
                {t('settings.permissions.permission')}
              </th>
              <th class="px-6 py-3 text-left text-xs font-semibold tracking-wide" style="color: var(--ds-text); border-bottom: 1px solid var(--ds-border);">
                {t('common.description')}
              </th>
              <th class="px-6 py-3 text-left text-xs font-semibold tracking-wide" style="color: var(--ds-text); border-bottom: 1px solid var(--ds-border);">
                {t('settings.permissions.assignedUsers')}
              </th>
              <th class="px-6 py-3 text-left text-xs font-semibold tracking-wide" style="color: var(--ds-text); border-bottom: 1px solid var(--ds-border);">
                {t('settings.permissions.assignedGroups')}
              </th>
              <th class="px-6 py-3 text-left text-xs font-semibold tracking-wide" style="color: var(--ds-text); border-bottom: 1px solid var(--ds-border);">
                {t('common.actions')}
              </th>
            </tr>
          </thead>
          <tbody>
            {#each getGlobalPermissions() as permission}
              <tr class="table-row">
                <td class="px-6 py-4 whitespace-nowrap" style="border-bottom: 1px solid var(--ds-border);">
                  <div class="flex items-center">
                    <div>
                      <div class="text-sm font-medium flex items-center gap-2" style="color: var(--ds-text);">
                        {permission.permission_name}
                        {#if permission.is_system}
                          <Crown class="w-4 h-4" style="color: var(--ds-text-warning);" title={t('settings.permissions.systemPermission')} />
                        {/if}
                      </div>
                      <div class="text-xs" style="color: var(--ds-text-subtle);">{permission.permission_key}</div>
                    </div>
                  </div>
                </td>
                <td class="px-6 py-4" style="border-bottom: 1px solid var(--ds-border);">
                  <div class="text-sm" style="color: var(--ds-text);">{permission.description}</div>
                </td>
                <td class="px-6 py-4" style="border-bottom: 1px solid var(--ds-border);">
                  <div class="flex flex-wrap gap-1">
                    {#each getUsersWithPermission(permission.id) as user}
                      <Lozenge color="blue" size="md">
                        <User class="w-3 h-3" />
                        {getUserDisplayName(user)}
                        {#if user.is_system_admin}
                          <Crown class="w-3 h-3" style="color: var(--ds-text-warning);" />
                        {/if}
                        <button
                          class="ml-1 revoke-btn"
                          onclick={() => revokePermissionFromUser(user.id, permission.id, permission.permission_key)}
                          title={t('settings.permissions.revokePermission')}
                          disabled={permission.permission_key === 'system.admin' && getUsersWithPermission(permission.id).length <= 1}
                        >
                          <X class="w-3 h-3" />
                        </button>
                      </Lozenge>
                    {:else}
                      <span class="text-sm italic" style="color: var(--ds-text-subtle);">{t('settings.permissions.noUsersAssigned')}</span>
                    {/each}
                  </div>
                </td>
                <td class="px-6 py-4" style="border-bottom: 1px solid var(--ds-border);">
                  <div class="flex flex-wrap gap-1">
                    {#each getGroupsWithPermission(permission.id) as group}
                      <Lozenge color="purple" size="md">
                        <UsersIcon class="w-3 h-3" />
                        {getGroupDisplayName(group)}
                        <button
                          class="ml-1 revoke-btn"
                          onclick={() => revokePermissionFromGroup(group.id, permission.id)}
                          title={t('settings.permissions.revokePermissionFromGroup')}
                        >
                          <X class="w-3 h-3" />
                        </button>
                      </Lozenge>
                    {:else}
                      <span class="text-sm italic" style="color: var(--ds-text-subtle);">{t('settings.permissions.noGroupsAssigned')}</span>
                    {/each}
                  </div>
                </td>
                <td class="px-6 py-4 whitespace-nowrap" style="border-bottom: 1px solid var(--ds-border);">
                  <button
                    class="inline-flex items-center px-3 py-1 border shadow-sm text-xs font-medium rounded-md focus:outline-none focus:ring-2 focus:ring-offset-2 action-btn"
                    style="border-color: var(--ds-border); color: var(--ds-text); background-color: var(--ds-surface-raised);"
                    onclick={() => toggleAddAssignee(permission.id)}
                  >
                    {#if permissionState[permission.id]?.showForm}
                      <X class="w-3 h-3 mr-1" />
                      {t('common.cancel')}
                    {:else}
                      <Plus class="w-3 h-3 mr-1" />
                      {t('settings.permissions.assign')}
                    {/if}
                  </button>
                </td>
              </tr>

              <!-- Inline Add Assignee Form -->
              {#if permissionState[permission.id]?.showForm}
                <tr style="background-color: var(--ds-interactive-subtle);">
                  <td colspan="5" class="px-6 py-4" style="border-bottom: 1px solid var(--ds-border);">
                    <div class="max-w-2xl">
                      <h4 class="text-sm font-medium mb-3" style="color: var(--ds-text);">
                        {t('settings.permissions.assignPermission', { permission: permission.permission_name })}
                      </h4>

                      <AssigneePicker
                        bind:type={permissionState[permission.id].type}
                        bind:userId={permissionState[permission.id].selectedUserId}
                        bind:groupId={permissionState[permission.id].selectedGroupId}
                        confirmText={t('settings.permissions.grantPermission')}
                        cancelText={t('common.cancel')}
                        on_confirm={() => grantPermission(permission.id)}
                        on_cancel={() => toggleAddAssignee(permission.id)}
                      />
                    </div>
                  </td>
                </tr>
              {/if}
            {/each}
          </tbody>
        </table>
      </div>
    </div>
  {/if}
</div>

<style>
  .table-row:hover {
    background-color: var(--ds-surface-hovered);
  }

  .action-btn:hover {
    background-color: var(--ds-surface-hovered);
  }

  .revoke-btn {
    opacity: 0.7;
    transition: opacity 0.15s, color 0.15s;
  }

  .revoke-btn:hover {
    opacity: 1;
    color: var(--ds-text-danger);
  }
</style>
