<script>
  import { groups as groupsApi } from '../api/permissions.js';
  import { getAdminUsers } from '../api/users.js';
  import Edit from '@lucide/svelte/icons/edit';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import UserStar from '@lucide/svelte/icons/user-star';
  import UserPlus from '@lucide/svelte/icons/user-plus';
  import UserMinus from '@lucide/svelte/icons/user-minus';
  import Circle from '@lucide/svelte/icons/circle';
  import X from '@lucide/svelte/icons/x';
  import Button from '../components/Button.svelte';
  import Avatar from '../components/Avatar.svelte';
  import Input from '../components/Input.svelte';
  import Textarea from '../components/Textarea.svelte';
  import UserPicker from '../pickers/UserPicker.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import EntityManagerPage from './EntityManagerPage.svelte';

  let users = $state([]);
  let editingGroup = $state(null);
  let showMemberModal = $state(false);
  let selectedGroup = $state(null);
  let availableUsers = $state([]);
  let selectedUserIds = $state([]);
  let selectedUserId = $state(null);
  let selectedUsersToAdd = $state([]);
  let currentMembers = $derived(
    (selectedGroup?.member_ids || [])
      .map((id) => users.find((user) => user.id === id))
      .filter(Boolean)
  );

  let formData = $state({ name: '', description: '' });

  async function loadGroups() {
    const [groups] = await Promise.all([groupsApi.getAdminAll(), loadUsers()]);
    return groups;
  }

  async function loadUsers() {
    try {
      users = await getAdminUsers();
    } catch (err) {
      errorToast(t('settings.groups.failedToLoad'));
    }
  }

  function openCreate() {
    formData = { name: '', description: '' };
    editingGroup = null;
  }

  function openEdit(group) {
    formData = { name: group.name, description: group.description || '' };
    editingGroup = group;
  }

  async function saveGroup() {
    if (editingGroup) {
      await groupsApi.update(editingGroup.id, { ...formData, is_active: editingGroup.is_active });
    } else {
      await groupsApi.create(formData);
    }
  }

  async function deleteGroup(group) {
    await groupsApi.delete(group.id);
  }

  async function manageMembers(group) {
    try {
      selectedGroup = await groupsApi.get(group.id);
      selectedUsersToAdd = [];
      selectedUserId = null;
      showMemberModal = true;
    } catch (err) {
      errorToast(err.message || t('settings.groups.failedToLoadDetails'));
    }
  }

  function onUserSelected(user) {
    if (!user || user.id === null) return;

    const isAlreadyToAdd = selectedUsersToAdd.some((u) => u.id === user.id);
    const isAlreadyMember = selectedGroup.member_ids?.includes(user.id);

    if (!isAlreadyToAdd && !isAlreadyMember) {
      selectedUsersToAdd = [...selectedUsersToAdd, user];
    }

    selectedUserId = null;
  }

  function removeUserFromAddList(userId) {
    selectedUsersToAdd = selectedUsersToAdd.filter((u) => u.id !== userId);
  }

  async function addSelectedMembers() {
    if (selectedUsersToAdd.length === 0) return;

    try {
      const userIds = selectedUsersToAdd.map((u) => u.id);
      await groupsApi.addMembers(selectedGroup.id, userIds);
      selectedGroup = await groupsApi.get(selectedGroup.id);
      selectedUsersToAdd = [];
      selectedUserId = null;
      await reloadGroups();
    } catch (err) {
      errorToast(err.message || t('settings.groups.failedToAddMembers'));
    }
  }

  async function removeMember(userId) {
    const confirmed = await confirm({
      title: t('common.remove'),
      message: t('settings.groups.confirmRemoveMember'),
      confirmText: t('common.remove'),
      cancelText: t('common.cancel'),
      variant: 'danger',
    });
    if (!confirmed) return;

    try {
      await groupsApi.removeMembers(selectedGroup.id, [userId]);
      selectedGroup = await groupsApi.get(selectedGroup.id);
      await reloadGroups();
    } catch (err) {
      errorToast(err.message || t('settings.groups.failedToRemoveMember'));
    }
  }

  function closeMemberModal() {
    showMemberModal = false;
    selectedGroup = null;
    availableUsers = [];
    selectedUserIds = [];
    selectedUserId = null;
    selectedUsersToAdd = [];
  }

  const groupColumns = [
    { key: 'name', label: t('settings.groups.groupName') },
    {
      key: 'description',
      label: t('settings.groups.description'),
      textColor: 'var(--ds-text-subtle)',
    },
    {
      key: 'member_count',
      label: t('settings.groups.members'),
      textColor: 'var(--ds-text-subtle)',
      render: (group) => {
        const count = group.member_count || 0;
        return count === 0
          ? t('settings.groups.noMembers')
          : t('settings.groups.membersCount', { count });
      },
    },
    { key: 'ldap_sync_enabled', label: t('settings.groups.ldapSync'), slot: 'ldap_sync' },
    { key: 'is_active', label: t('settings.groups.status'), slot: 'status' },
    { key: 'actions', label: t('settings.groups.actions') },
  ];

  function buildGroupDropdownItems(group, { edit, remove }) {
    const items = [
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: t('settings.groups.edit'),
        hoverClass: 'hover-bg',
        onClick: () => edit(group),
      },
      {
        id: 'members',
        type: 'regular',
        icon: UserStar,
        title: t('settings.groups.manageMembers'),
        hoverClass: 'hover-bg',
        onClick: () => manageMembers(group),
      },
    ];

    if (!group.is_system_group) {
      items.push({
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: t('settings.groups.delete'),
        color: 'var(--ds-text-danger)',
        hoverClass: 'hover-danger',
        onClick: () => remove(group),
      });
    }

    return items;
  }

  // The scaffold owns the list; member edits reload it through this handle.
  let reloadGroups = () => {};
</script>

<EntityManagerPage
  icon={UserStar}
  title={t('settings.groups.title')}
  subtitle={t('settings.groups.subtitle')}
  addLabel={t('settings.groups.addGroup')}
  hotkeyScope="groups"
  emptyMessage={t('settings.groups.noGroupsFound')}
  emptyIcon={Circle}
  columns={groupColumns}
  load={loadGroups}
  create={saveGroup}
  update={saveGroup}
  remove={deleteGroup}
  onOpenCreate={openCreate}
  onOpenEdit={openEdit}
  confirmDeleteMessage={() => t('settings.groups.confirmDelete')}
  createTitle={t('settings.groups.createGroup')}
  editTitle={t('settings.groups.editGroup')}
  confirmTestid="dialog-confirm"
  submitDisabled={() => !formData.name.trim()}
  errorLabels={{
    load: t('settings.groups.failedToLoad'),
    save: t('settings.groups.failedToSave'),
    delete: t('settings.groups.failedToDelete'),
  }}
  actionItems={buildGroupDropdownItems}
  cellSnippets={{ ldap_sync: groupLdapCell, status: groupStatusCell }}
  formFields={groupForm}
  onReady={({ reload }) => (reloadGroups = reload)}
>
</EntityManagerPage>

{#snippet groupLdapCell(group)}
  <Lozenge
    color={group.ldap_sync_enabled ? 'blue' : 'gray'}
    text={group.ldap_sync_enabled ? t('settings.groups.enabled') : t('settings.groups.manual')}
  />
{/snippet}

{#snippet groupStatusCell(group)}
  <Lozenge
    color={group.is_active ? 'green' : 'red'}
    text={group.is_active ? t('settings.groups.active') : t('settings.groups.inactive')}
  />
{/snippet}

{#snippet groupForm()}
  <div>
    <label for="name" class="block text-sm font-medium" style="color: var(--ds-text)">
      {t('settings.groups.groupName')}
    </label>
    <Input
      id="name"
      bind:value={formData.name}
      required
      placeholder={t('settings.groups.groupNamePlaceholder')}
    />
  </div>

  <div>
    <label for="description" class="block text-sm font-medium" style="color: var(--ds-text)">
      {t('settings.groups.descriptionOptional')}
    </label>
    <Textarea
      id="description"
      bind:value={formData.description}
      rows={3}
      placeholder={t('settings.groups.descriptionPlaceholder')}
    />
  </div>
{/snippet}

<Modal isOpen={showMemberModal && selectedGroup} onclose={closeMemberModal} maxWidth="max-w-2xl">
  <ModalHeader
    title="{t('settings.groups.manageMembers')}: {selectedGroup?.name}"
    subtitle={t('settings.groups.membersCount', { count: selectedGroup?.member_count || 0 })}
    onClose={closeMemberModal}
  />

  <div class="px-6 py-4 space-y-6 max-h-[60vh] overflow-y-auto">
    {#if currentMembers.length > 0}
      <div>
        <h4 class="text-sm font-medium mb-3" style="color: var(--ds-text)">{t('settings.groups.currentMembers')}</h4>
        <div class="space-y-2">
          {#each currentMembers as member}
            <div class="flex items-center justify-between p-3 rounded border" style="background-color: var(--ds-surface); border-color: var(--ds-border)">
              <Avatar size="sm" variant="neutral" name={member.full_name || '?'} class="mr-3" />
              <div>
                <div class="text-sm font-medium" style="color: var(--ds-text)">
                  {member.full_name || t('settings.groups.unknownUser')}
                </div>
                <div class="text-xs" style="color: var(--ds-text-subtle)">
                  {member.email}
                </div>
              </div>
              <Button variant="danger-ghost" size="sm" icon={UserMinus} onclick={() => removeMember(member.id)}>
                {t('settings.groups.remove')}
              </Button>
            </div>
          {/each}
        </div>
      </div>
    {/if}

    <div>
      <h4 class="text-sm font-medium mb-3" style="color: var(--ds-text)">{t('settings.groups.addMembers')}</h4>
      <div class="space-y-4">
        <div>
          <UserPicker
            bind:value={selectedUserId}
            placeholder={t('settings.groups.searchAndSelectUser')}
            onSelect={onUserSelected}
          />
        </div>

        {#if selectedUsersToAdd.length > 0}
          <div>
            <p class="text-sm mb-2" style="color: var(--ds-text-subtle)">{t('settings.groups.usersToAdd')} ({selectedUsersToAdd.length}):</p>
            <div class="space-y-2 max-h-32 overflow-y-auto">
              {#each selectedUsersToAdd as user}
                <div class="flex items-center justify-between p-2 bg-ds-accent-blue-subtle rounded border">
                  <div class="flex items-center">
                    <Avatar size="xs" variant="blue" firstName={user.first_name} lastName={user.last_name} class="mr-2" />
                    <div>
                      <div class="text-sm font-medium" style="color: var(--ds-text)">
                        {user.first_name} {user.last_name}
                      </div>
                      <div class="text-xs" style="color: var(--ds-text-subtle)">
                        {user.email}
                      </div>
                    </div>
                  </div>
                  <button
                    class="p-1 rounded"
                    style="color: var(--ds-text-subtle)"
                    onclick={() => removeUserFromAddList(user.id)}
                    title={t('settings.groups.remove')}
                  >
                    <X class="w-4 h-4" />
                  </button>
                </div>
              {/each}
            </div>

            <div class="flex justify-end mt-3">
              <Button variant="primary" size="sm" icon={UserPlus} onclick={addSelectedMembers}>
                {t('common.add')} {selectedUsersToAdd.length === 1 ? t('settings.groups.memberCount', { count: selectedUsersToAdd.length }) : t('settings.groups.membersCount', { count: selectedUsersToAdd.length })}
              </Button>
            </div>
          </div>
        {/if}
      </div>
    </div>
  </div>

  <DialogFooter showCancel={false} confirmLabel={t('common.close')} variant="secondary" onConfirm={closeMemberModal} />
</Modal>
