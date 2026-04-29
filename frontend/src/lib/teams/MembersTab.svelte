<script>
  import { onMount } from 'svelte';
  import { IconUserMinus, IconCircle } from '@tabler/icons-svelte-runes';
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import { errorToast, successToast } from '../stores/toasts.svelte.js';
  import Select from '../components/Select.svelte';
  import DataTable from '../components/DataTable.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import UserPicker from '../pickers/UserPicker.svelte';
  import EmptyState from '../components/EmptyState.svelte';

  let { team, canEdit, onUpdated } = $props();

  let resolvedMembers = $state([]);
  let pickerValue = $state(null);
  let pickerRole = $state('member');
  let busy = $state(false);

  const roleOptions = [
    { value: 'member', label: 'Member' },
    { value: 'admin', label: 'Admin' },
  ];

  async function loadResolved() {
    try {
      resolvedMembers = await api.teams.getResolvedMembers(team.id);
    } catch (err) {
      console.warn('Failed to load resolved members', err);
    }
  }

  async function onUserPicked(user) {
    if (!user || user.id == null) return;
    if (team.direct_members?.some((m) => m.user_id === user.id)) {
      pickerValue = null;
      return;
    }
    busy = true;
    try {
      await api.teams.addMembers(team.id, [user.id], pickerRole);
      successToast(t('teams.membersAdded'));
      pickerValue = null;
      await onUpdated?.();
      await loadResolved();
    } catch (err) {
      errorToast(err.message || t('teams.failedToAddMembers'));
    } finally {
      busy = false;
    }
  }

  async function changeRole(member, newRole) {
    if (!newRole || newRole === member.role) return;
    try {
      await api.teams.updateMemberRole(team.id, member.user_id, newRole);
      successToast(t('teams.roleUpdated'));
      await onUpdated?.();
    } catch (err) {
      errorToast(err.message || t('teams.failedToUpdateRole'));
    }
  }

  async function removeMember(member) {
    const confirmed = await confirm({
      title: t('common.remove'),
      message: t('teams.confirmRemoveMember', { name: member.user_name || '' }),
      confirmText: t('common.remove'),
      cancelText: t('common.cancel'),
      variant: 'danger',
    });
    if (!confirmed) return;
    try {
      await api.teams.removeMembers(team.id, [member.user_id]);
      successToast(t('teams.memberRemoved'));
      await onUpdated?.();
      await loadResolved();
    } catch (err) {
      errorToast(err.message || t('teams.failedToRemoveMember'));
    }
  }

  function buildRowDropdown(member) {
    if (!canEdit) return [];
    return [
      {
        id: 'remove',
        type: 'regular',
        icon: IconUserMinus,
        title: t('common.remove'),
        color: 'var(--ds-text-danger)',
        hoverClass: 'hover-danger',
        onClick: () => removeMember(member),
      },
    ];
  }

  const memberColumns = $derived([
    { key: 'user_name', label: t('teams.member') },
    { key: 'user_email', label: t('teams.email'), textColor: 'var(--ds-text-subtle)' },
    { key: 'role', label: t('teams.role'), slot: 'role' },
    { key: 'actions', label: t('teams.actions') },
  ]);

  const resolvedColumns = $derived([
    { key: 'user_name', label: t('teams.member') },
    { key: 'user_email', label: t('teams.email'), textColor: 'var(--ds-text-subtle)' },
    { key: 'source', label: t('teams.source'), slot: 'source' },
    { key: 'leave', label: t('teams.leaveStatus'), slot: 'leave' },
  ]);

  onMount(() => {
    loadResolved();
  });
</script>

<div class="space-y-8">
  {#if canEdit}
    <section class="space-y-3" data-testid="team-add-member">
      <h4 class="text-sm font-medium" style="color: var(--ds-text)">
        {t('teams.addMembers')}
      </h4>
      <div class="flex items-start gap-3 max-w-2xl">
        <div class="flex-1">
          <UserPicker
            bind:value={pickerValue}
            placeholder={t('teams.searchUser')}
            onSelect={onUserPicked}
            disabled={busy}
          />
        </div>
        <div class="w-32">
          <Select bind:value={pickerRole} options={roleOptions} id="staged-role" disabled={busy} />
        </div>
      </div>
    </section>
  {/if}

  <section class="space-y-3">
    <h4 class="text-sm font-medium" style="color: var(--ds-text)">
      {t('teams.directMembers')}
    </h4>
    {#if !team.direct_members || team.direct_members.length === 0}
      <EmptyState icon={IconCircle} message={t('teams.noDirectMembers')} />
    {:else}
      <DataTable
        columns={memberColumns}
        data={team.direct_members}
        keyField="user_id"
        actionItems={buildRowDropdown}
      >
        {#snippet role(member)}
          {#if canEdit}
            <Select
              value={member.role}
              options={roleOptions}
              onchange={(v) => changeRole(member, v)}
              id={`member-role-${member.user_id}`}
            />
          {:else}
            <Lozenge color={member.role === 'admin' ? 'blue' : 'gray'} text={member.role} />
          {/if}
        {/snippet}
      </DataTable>
    {/if}
  </section>

  <section class="space-y-3">
    <h4 class="text-sm font-medium" style="color: var(--ds-text)">
      {t('teams.resolvedMembers')}
      <span class="ml-2 text-xs" style="color: var(--ds-text-subtle)">
        ({t('teams.resolvedMembersHint')})
      </span>
    </h4>
    {#if resolvedMembers.length === 0}
      <EmptyState icon={IconCircle} message={t('teams.noResolvedMembers')} />
    {:else}
      <DataTable
        columns={resolvedColumns}
        data={resolvedMembers}
        keyField="user_id"
      >
        {#snippet source(member)}
          <Lozenge
            color={member.source === 'direct' ? 'blue' : 'purple'}
            text={member.source === 'direct' ? t('teams.sourceDirect') : `${t('teams.sourceGroup')}: ${member.source_name || ''}`}
          />
        {/snippet}
        {#snippet leave(member)}
          {#if member.is_on_leave}
            <Lozenge
              color="orange"
              text={member.substitute_name ? t('teams.onLeaveWithSub', { name: member.substitute_name }) : t('teams.onLeave')}
            />
          {:else}
            <span style="color: var(--ds-text-subtlest)">—</span>
          {/if}
        {/snippet}
      </DataTable>
    {/if}
  </section>
</div>
