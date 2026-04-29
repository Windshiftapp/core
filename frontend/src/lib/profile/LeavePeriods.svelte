<script>
  import { onMount } from 'svelte';
  import { IconPlus, IconEdit, IconTrash, IconCircle } from '@tabler/icons-svelte-runes';
  import { api } from '../api.js';
  import { authStore } from '../stores';
  import { t } from '../stores/i18n.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import { errorToast, successToast } from '../stores/toasts.svelte.js';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';
  import Button from '../components/Button.svelte';
  import Input from '../components/Input.svelte';
  import Textarea from '../components/Textarea.svelte';
  import DataTable from '../components/DataTable.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import UserPicker from '../pickers/UserPicker.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';

  let leavePeriods = $state([]);
  let loading = $state(false);
  let error = $state('');
  let showModal = $state(false);
  let editing = $state(null);
  let busy = $state(false);
  let substituteValue = $state(null);
  let pickedSubstitute = $state(null);
  let formData = $state(emptyForm());

  const userId = $derived($authStore.currentUser?.id);

  function emptyForm() {
    return {
      start_date: '',
      end_date: '',
      reason: '',
      substitute_user_id: null,
    };
  }

  async function load() {
    if (!userId) return;
    loading = true;
    try {
      leavePeriods = (await api.leave.list(userId)) || [];
      error = '';
    } catch (err) {
      error = err.message || t('profile.leave.failedToLoad');
    } finally {
      loading = false;
    }
  }

  function openCreate() {
    editing = null;
    formData = emptyForm();
    substituteValue = null;
    pickedSubstitute = null;
    showModal = true;
  }

  function openEdit(period) {
    editing = period;
    formData = {
      start_date: period.start_date,
      end_date: period.end_date,
      reason: period.reason || '',
      substitute_user_id: period.substitute_user_id ?? null,
    };
    substituteValue = period.substitute_user_id ?? null;
    pickedSubstitute = period.substitute_user_id
      ? { id: period.substitute_user_id, name: period.substitute_name }
      : null;
    showModal = true;
  }

  function closeModal() {
    showModal = false;
    editing = null;
    formData = emptyForm();
    substituteValue = null;
    pickedSubstitute = null;
  }

  function onSubstitutePicked(user) {
    pickedSubstitute = user;
    formData.substitute_user_id = user?.id ?? null;
  }

  async function save() {
    if (!formData.start_date || !formData.end_date) {
      errorToast(t('profile.leave.datesRequired'));
      return;
    }
    if (formData.end_date < formData.start_date) {
      errorToast(t('profile.leave.endBeforeStart'));
      return;
    }
    busy = true;
    try {
      const payload = {
        start_date: formData.start_date,
        end_date: formData.end_date,
        reason: formData.reason,
        substitute_user_id: formData.substitute_user_id,
      };
      if (editing) {
        await api.leave.update(userId, editing.id, payload);
        successToast(t('profile.leave.updated'));
      } else {
        await api.leave.create(userId, payload);
        successToast(t('profile.leave.created'));
      }
      closeModal();
      await load();
    } catch (err) {
      errorToast(err.message || t('profile.leave.failedToSave'));
    } finally {
      busy = false;
    }
  }

  async function deletePeriod(period) {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('profile.leave.confirmDelete'),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger',
    });
    if (!confirmed) return;
    try {
      await api.leave.delete(userId, period.id);
      successToast(t('profile.leave.deleted'));
      await load();
    } catch (err) {
      errorToast(err.message || t('profile.leave.failedToDelete'));
    }
  }

  function buildRowDropdown(period) {
    return [
      {
        id: 'edit',
        type: 'regular',
        icon: IconEdit,
        title: t('common.edit'),
        hoverClass: 'hover-bg',
        onClick: () => openEdit(period),
      },
      {
        id: 'delete',
        type: 'regular',
        icon: IconTrash,
        title: t('common.delete'),
        color: 'var(--ds-text-danger)',
        hoverClass: 'hover-danger',
        onClick: () => deletePeriod(period),
      },
    ];
  }

  function statusOf(period) {
    const today = new Date().toISOString().slice(0, 10);
    if (period.end_date < today) return { color: 'gray', text: t('profile.leave.past') };
    if (period.start_date > today) return { color: 'blue', text: t('profile.leave.upcoming') };
    return { color: 'orange', text: t('profile.leave.active') };
  }

  const columns = $derived([
    { key: 'start_date', label: t('profile.leave.startDate') },
    { key: 'end_date', label: t('profile.leave.endDate') },
    {
      key: 'substitute_name',
      label: t('profile.leave.substitute'),
      textColor: 'var(--ds-text-subtle)',
      render: (p) => p.substitute_name || '—',
    },
    { key: 'reason', label: t('profile.leave.reason'), textColor: 'var(--ds-text-subtle)' },
    { key: 'status', label: t('profile.leave.status'), slot: 'status' },
    { key: 'actions', label: t('teams.actions') },
  ]);

  onMount(() => {
    load();
  });
</script>

<div class="space-y-6">
  <div class="flex items-center justify-between">
    <div>
      <h2 class="text-lg font-medium" style="color: var(--ds-text)">
        {t('profile.leave.title')}
      </h2>
      <p class="text-sm mt-1" style="color: var(--ds-text-subtle)">
        {t('profile.leave.subtitle')}
      </p>
    </div>
    <Button
      variant="primary"
      icon={IconPlus}
      onclick={openCreate}
      keyboardHint="A"
      hotkeyConfig={{ key: toHotkeyString('profileLeave', 'add'), guard: () => !showModal }}
      dataTestid="leave-add-button"
    >
      {t('profile.leave.scheduleLeave')}
    </Button>
  </div>

  {#if error}
    <AlertBox message={error} />
  {/if}

  {#if loading}
    <div class="text-center py-8" style="color: var(--ds-text-subtle)">
      {t('teams.loading')}
    </div>
  {:else if leavePeriods.length === 0}
    <EmptyState icon={IconCircle} message={t('profile.leave.empty')} />
  {:else}
    <DataTable
      columns={columns}
      data={leavePeriods}
      keyField="id"
      actionItems={buildRowDropdown}
    >
      {#snippet status(period)}
        {@const s = statusOf(period)}
        <Lozenge color={s.color} text={s.text} />
      {/snippet}
    </DataTable>
  {/if}
</div>

<Modal isOpen={showModal} onclose={closeModal} onSubmit={save} submitDisabled={busy} maxWidth="max-w-lg">
  <ModalHeader
    title={editing ? t('profile.leave.editLeave') : t('profile.leave.scheduleLeave')}
    onClose={closeModal}
  />
  <div class="px-6 py-4 space-y-4">
    <div class="grid grid-cols-2 gap-3">
      <div>
        <label for="leave-start" class="block text-sm font-medium mb-1" style="color: var(--ds-text)">
          {t('profile.leave.startDate')}
        </label>
        <Input id="leave-start" type="date" bind:value={formData.start_date} required />
      </div>
      <div>
        <label for="leave-end" class="block text-sm font-medium mb-1" style="color: var(--ds-text)">
          {t('profile.leave.endDate')}
        </label>
        <Input id="leave-end" type="date" bind:value={formData.end_date} required />
      </div>
    </div>
    <div>
      <label class="block text-sm font-medium mb-1" style="color: var(--ds-text)">
        {t('profile.leave.substituteOptional')}
      </label>
      <UserPicker
        bind:value={substituteValue}
        placeholder={t('profile.leave.pickSubstitute')}
        onSelect={onSubstitutePicked}
      />
    </div>
    <div>
      <label for="leave-reason" class="block text-sm font-medium mb-1" style="color: var(--ds-text)">
        {t('profile.leave.reasonOptional')}
      </label>
      <Textarea id="leave-reason" bind:value={formData.reason} rows={2} />
    </div>
  </div>
  <DialogFooter
    confirmLabel={editing ? t('common.save') : t('profile.leave.scheduleLeave')}
    onCancel={closeModal}
    onConfirm={save}
    loading={busy}
    showKeyboardHint
    confirmTestid="leave-save"
  />
</Modal>
