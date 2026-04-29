<script>
  import { IconCircle, IconTrash } from '@tabler/icons-svelte-runes';
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import { errorToast, successToast } from '../stores/toasts.svelte.js';
  import DataTable from '../components/DataTable.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import GroupPicker from '../pickers/GroupPicker.svelte';

  let { team, canEdit, onUpdated } = $props();

  let pickerValue = $state(null);
  let busy = $state(false);

  async function onGroupPicked(group) {
    if (!group || group.id == null) return;
    if (team.mapped_groups?.some((mg) => mg.group_id === group.id)) {
      pickerValue = null;
      return;
    }
    busy = true;
    try {
      await api.teams.addGroups(team.id, [group.id]);
      successToast(t('teams.groupsAdded'));
      pickerValue = null;
      await onUpdated?.();
    } catch (err) {
      errorToast(err.message || t('teams.failedToAddGroups'));
    } finally {
      busy = false;
    }
  }

  async function removeGroup(mapping) {
    const confirmed = await confirm({
      title: t('common.remove'),
      message: t('teams.confirmRemoveGroup', { name: mapping.group_name || '' }),
      confirmText: t('common.remove'),
      cancelText: t('common.cancel'),
      variant: 'danger',
    });
    if (!confirmed) return;
    try {
      await api.teams.removeGroups(team.id, [mapping.group_id]);
      successToast(t('teams.groupRemoved'));
      await onUpdated?.();
    } catch (err) {
      errorToast(err.message || t('teams.failedToRemoveGroup'));
    }
  }

  function buildRowDropdown(mapping) {
    if (!canEdit) return [];
    return [
      {
        id: 'remove',
        type: 'regular',
        icon: IconTrash,
        title: t('common.remove'),
        color: 'var(--ds-text-danger)',
        hoverClass: 'hover-danger',
        onClick: () => removeGroup(mapping),
      },
    ];
  }

  const columns = $derived([
    { key: 'group_name', label: t('teams.group') },
    {
      key: 'member_count',
      label: t('teams.memberCount'),
      textColor: 'var(--ds-text-subtle)',
      render: (m) => `${m.member_count ?? 0}`,
    },
    { key: 'actions', label: t('teams.actions') },
  ]);
</script>

<div class="space-y-8">
  {#if canEdit}
    <section class="space-y-3">
      <h4 class="text-sm font-medium" style="color: var(--ds-text)">
        {t('teams.attachGroup')}
      </h4>
      <div class="max-w-xl" data-testid="team-add-group">
        <GroupPicker
          bind:value={pickerValue}
          placeholder={t('teams.searchGroup')}
          onSelect={onGroupPicked}
          disabled={busy}
        />
      </div>
    </section>
  {/if}

  <section class="space-y-3">
    <h4 class="text-sm font-medium" style="color: var(--ds-text)">
      {t('teams.mappedGroups')}
    </h4>
    {#if !team.mapped_groups || team.mapped_groups.length === 0}
      <EmptyState icon={IconCircle} message={t('teams.noMappedGroups')} />
    {:else}
      <DataTable
        columns={columns}
        data={team.mapped_groups}
        keyField="group_id"
        actionItems={buildRowDropdown}
      />
    {/if}
  </section>
</div>
