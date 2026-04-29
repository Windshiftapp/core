<script>
  import { IconCircle, IconStack2, IconTrash, IconX } from '@tabler/icons-svelte-runes';
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import { errorToast, successToast } from '../stores/toasts.svelte.js';
  import Button from '../components/Button.svelte';
  import DataTable from '../components/DataTable.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import GroupPicker from '../pickers/GroupPicker.svelte';

  let { team, canEdit, onUpdated } = $props();

  let pickerValue = $state(null);
  let stagedGroups = $state([]);
  let busy = $state(false);

  function onGroupPicked(group) {
    if (!group || group.id == null) return;
    if (stagedGroups.some((g) => g.id === group.id)) {
      pickerValue = null;
      return;
    }
    if (team.mapped_groups?.some((mg) => mg.group_id === group.id)) {
      pickerValue = null;
      return;
    }
    stagedGroups = [...stagedGroups, group];
    pickerValue = null;
  }

  function unstage(groupId) {
    stagedGroups = stagedGroups.filter((g) => g.id !== groupId);
  }

  async function commitAdd() {
    if (stagedGroups.length === 0) return;
    busy = true;
    try {
      const ids = stagedGroups.map((g) => g.id);
      await api.teams.addGroups(team.id, ids);
      successToast(t('teams.groupsAdded'));
      stagedGroups = [];
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
      <div class="max-w-xl">
        <GroupPicker
          bind:value={pickerValue}
          placeholder={t('teams.searchGroup')}
          onSelect={onGroupPicked}
        />
      </div>

      {#if stagedGroups.length > 0}
        <div class="max-w-xl space-y-2">
          <p class="text-sm" style="color: var(--ds-text-subtle)">
            {t('teams.groupsToAdd')} ({stagedGroups.length})
          </p>
          <div class="space-y-2">
            {#each stagedGroups as group (group.id)}
              <div class="flex items-center justify-between p-2 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
                <span class="text-sm" style="color: var(--ds-text)">
                  {group.group_name || group.name}
                </span>
                <button
                  class="p-1 rounded"
                  style="color: var(--ds-text-subtle)"
                  onclick={() => unstage(group.id)}
                  aria-label={t('common.remove')}
                >
                  <IconX class="w-4 h-4" />
                </button>
              </div>
            {/each}
          </div>
          <div class="flex justify-end">
            <Button
              variant="primary"
              size="sm"
              icon={IconStack2}
              onclick={commitAdd}
              disabled={busy}
              dataTestid="team-add-group"
            >
              {t('common.add')}
            </Button>
          </div>
        </div>
      {/if}
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
