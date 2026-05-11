<script>
  import { IconPlus, IconCircle, IconTrash } from '@tabler/icons-svelte-runes';
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import { errorToast, successToast } from '../stores/toasts.svelte.js';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';
  import Button from '../components/Button.svelte';
  import DataTable from '../components/DataTable.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import GroupPicker from '../pickers/GroupPicker.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';

  let { team, canEdit, onUpdated } = $props();

  let showAddModal = $state(false);
  let pickerValue = $state(null);
  let pickedGroup = $state(null);
  let busy = $state(false);

  function openAddModal() {
    pickerValue = null;
    pickedGroup = null;
    showAddModal = true;
  }

  function closeAddModal() {
    showAddModal = false;
    pickerValue = null;
    pickedGroup = null;
  }

  function onGroupPicked(group) {
    pickedGroup = group;
  }

  async function commitAdd() {
    if (!pickedGroup?.id) {
      errorToast(t('teams.pickGroupFirst'));
      return;
    }
    if (team.mapped_groups?.some((mg) => mg.group_id === pickedGroup.id)) {
      errorToast(t('teams.alreadyAttached'));
      return;
    }
    busy = true;
    try {
      await api.teams.addGroups(team.id, [pickedGroup.id]);
      successToast(t('teams.groupsAdded'));
      closeAddModal();
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

<div class="space-y-3">
  <div class="flex items-center justify-between">
    <h4 class="text-sm font-medium" style="color: var(--ds-text)">
      {t('teams.mappedGroups')}
    </h4>
    {#if canEdit}
      <Button
        variant="primary"
        size="sm"
        icon={IconPlus}
        onclick={openAddModal}
        keyboardHint="A"
        hotkeyConfig={{ key: toHotkeyString('teamGroups', 'add'), guard: () => !showAddModal }}
        dataTestid="team-add-group"
      >
        {t('teams.attachGroup')}
      </Button>
    {/if}
  </div>
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
</div>

<Modal isOpen={showAddModal} onclose={closeAddModal} onSubmit={commitAdd} submitDisabled={busy || !pickedGroup} maxWidth="max-w-lg">
  <ModalHeader title={t('teams.attachGroup')} onClose={closeAddModal} />
  <div class="px-6 py-4">
    <div class="block text-sm font-medium mb-1" style="color: var(--ds-text)">
      {t('teams.group')}
    </div>
    <GroupPicker
      bind:value={pickerValue}
      placeholder={t('teams.searchGroup')}
      onSelect={onGroupPicked}
    />
  </div>
  <DialogFooter
    confirmLabel={t('common.add')}
    onCancel={closeAddModal}
    onConfirm={commitAdd}
    disabled={!pickedGroup}
    loading={busy}
    showKeyboardHint
    confirmTestid="add-group-confirm"
  />
</Modal>
