<script>
  import { onMount } from 'svelte';
  import { IconPlus, IconUsersGroup, IconEdit, IconTrash, IconCircle } from '@tabler/icons-svelte-runes';
  import { api } from '../api.js';
  import { navigate } from '../router.js';
  import { authStore, permissionStore } from '../stores';
  import { t } from '../stores/i18n.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import { errorToast, successToast } from '../stores/toasts.svelte.js';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';
  import PageHeader from '../layout/PageHeader.svelte';
  import Button from '../components/Button.svelte';
  import Input from '../components/Input.svelte';
  import Textarea from '../components/Textarea.svelte';
  import DataTable from '../components/DataTable.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';

  let teams = $state([]);
  let myTeamRoles = $state(new Map());
  let loading = $state(false);
  let error = $state('');

  let showCreateForm = $state(false);
  let editingTeam = $state(null);
  let formData = $state({ name: '', description: '', is_active: true });

  const hasGlobalManage = $derived(permissionStore.hasPermissionKey('teams.manage'));

  function isTeamAdmin(teamId) {
    return myTeamRoles.get(teamId) === 'admin';
  }

  function canEditTeam(team) {
    return hasGlobalManage || isTeamAdmin(team.id);
  }

  async function loadTeams() {
    loading = true;
    try {
      teams = await api.teams.getAll();
      error = '';
    } catch (err) {
      error = err.message || t('teams.failedToLoad');
    } finally {
      loading = false;
    }
  }

  async function loadMyRoles() {
    const userId = $authStore.currentUser?.id;
    if (!userId) return;
    try {
      const myTeams = await api.teams.getTeamsForUser(userId);
      const map = new Map();
      for (const team of myTeams || []) {
        if (team.role) map.set(team.id, team.role);
      }
      myTeamRoles = map;
    } catch (err) {
      console.warn('Failed to load user team roles:', err);
    }
  }

  function resetForm() {
    formData = { name: '', description: '', is_active: true };
    editingTeam = null;
    showCreateForm = false;
  }

  function startCreate() {
    resetForm();
    showCreateForm = true;
  }

  function startEdit(team) {
    formData = {
      name: team.name,
      description: team.description || '',
      is_active: team.is_active,
    };
    editingTeam = team;
    showCreateForm = true;
  }

  async function saveTeam() {
    if (!formData.name?.trim()) {
      error = t('teams.nameRequired');
      return;
    }
    try {
      if (editingTeam) {
        await api.teams.update(editingTeam.id, formData);
        successToast(t('teams.updated'));
      } else {
        await api.teams.create(formData);
        successToast(t('teams.created'));
      }
      resetForm();
      await loadTeams();
    } catch (err) {
      error = err.message || t('teams.failedToSave');
    }
  }

  async function deleteTeam(team) {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('teams.confirmDelete', { name: team.name }),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger',
    });
    if (!confirmed) return;
    try {
      await api.teams.delete(team.id);
      successToast(t('teams.deleted'));
      await loadTeams();
    } catch (err) {
      errorToast(err.message || t('teams.failedToDelete'));
    }
  }

  function openTeam(team) {
    navigate(`/teams/${team.id}`);
  }

  function buildRowDropdown(team) {
    const items = [];
    if (canEditTeam(team)) {
      items.push({
        id: 'edit',
        type: 'regular',
        icon: IconEdit,
        title: t('common.edit'),
        hoverClass: 'hover-bg',
        onClick: () => startEdit(team),
      });
    }
    if (hasGlobalManage) {
      items.push({
        id: 'delete',
        type: 'regular',
        icon: IconTrash,
        title: t('common.delete'),
        color: 'var(--ds-text-danger)',
        hoverClass: 'hover-danger',
        onClick: () => deleteTeam(team),
      });
    }
    return items;
  }

  const columns = $derived([
    { key: 'name', label: t('teams.name') },
    { key: 'description', label: t('teams.description'), textColor: 'var(--ds-text-subtle)' },
    {
      key: 'resolved_member_count',
      label: t('teams.members'),
      textColor: 'var(--ds-text-subtle)',
      render: (team) => `${team.resolved_member_count ?? 0}`,
    },
    {
      key: 'group_count',
      label: t('teams.groups'),
      textColor: 'var(--ds-text-subtle)',
      render: (team) => `${team.group_count ?? 0}`,
    },
    { key: 'is_active', label: t('teams.status'), slot: 'status' },
    { key: 'actions', label: t('teams.actions') },
  ]);

  onMount(() => {
    loadTeams();
    loadMyRoles();
  });
</script>

<div class="space-y-6">
  <PageHeader
    icon={IconUsersGroup}
    title={t('teams.title')}
    subtitle={t('teams.subtitle')}
  >
    {#snippet actions()}
      {#if hasGlobalManage}
        <Button
          variant="primary"
          icon={IconPlus}
          onclick={startCreate}
          keyboardHint="A"
          hotkeyConfig={{ key: toHotkeyString('teams', 'add'), guard: () => !showCreateForm }}
          dataTestid="team-create-button"
        >
          {t('teams.createTeam')}
        </Button>
      {/if}
    {/snippet}
  </PageHeader>

  {#if error}
    <AlertBox message={error} />
  {/if}

  <Modal isOpen={showCreateForm} onclose={resetForm} maxWidth="max-w-lg">
    <ModalHeader
      title={editingTeam ? t('teams.editTeam') : t('teams.createTeam')}
      onClose={resetForm}
    />
    <div class="px-6 py-4">
      <form onsubmit={(e) => { e.preventDefault(); saveTeam(); }} class="space-y-4">
        <div>
          <label for="team-name" class="block text-sm font-medium" style="color: var(--ds-text)">
            {t('teams.name')}
          </label>
          <Input
            id="team-name"
            bind:value={formData.name}
            required
            placeholder={t('teams.namePlaceholder')}
          />
        </div>
        <div>
          <label for="team-description" class="block text-sm font-medium" style="color: var(--ds-text)">
            {t('teams.descriptionOptional')}
          </label>
          <Textarea
            id="team-description"
            bind:value={formData.description}
            rows={3}
            placeholder={t('teams.descriptionPlaceholder')}
          />
        </div>
        {#if editingTeam}
          <div class="flex items-center gap-2">
            <input
              id="team-is-active"
              type="checkbox"
              bind:checked={formData.is_active}
            />
            <label for="team-is-active" class="text-sm" style="color: var(--ds-text)">
              {t('teams.active')}
            </label>
          </div>
        {/if}
      </form>
    </div>
    <DialogFooter
      confirmLabel={editingTeam ? t('common.save') : t('teams.createTeam')}
      onCancel={resetForm}
      onConfirm={saveTeam}
      confirmTestid="team-save"
    />
  </Modal>

  {#if loading}
    <div class="text-center py-8" style="color: var(--ds-text-subtle)">
      {t('teams.loading')}
    </div>
  {:else}
    <DataTable
      columns={columns}
      data={teams}
      keyField="id"
      onRowClick={openTeam}
      emptyMessage={t('teams.empty')}
      emptyIcon={IconCircle}
      actionItems={buildRowDropdown}
    >
      {#snippet status(team)}
        <Lozenge
          color={team.is_active ? 'green' : 'gray'}
          text={team.is_active ? t('teams.active') : t('teams.inactive')}
        />
      {/snippet}
    </DataTable>
  {/if}
</div>
