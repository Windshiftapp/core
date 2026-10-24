<script>
  import { onCallSchedules } from '../api/oncall.js';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast, successToast } from '../stores/toasts.svelte.js';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';
  import Input from '../components/Input.svelte';
  import Textarea from '../components/Textarea.svelte';
  import Checkbox from '../components/Checkbox.svelte';
  import TimezonePicker from '../pickers/TimezonePicker.svelte';

  let { teamId, schedule = null, onSaved, onCancel } = $props();

  // Snapshot schedule into editable form state.
  // svelte-ignore state_referenced_locally
  let formData = $state({
    name: schedule?.name || '',
    description: schedule?.description || '',
    timezone: schedule?.timezone || (typeof Intl !== 'undefined' ? Intl.DateTimeFormat().resolvedOptions().timeZone : 'UTC'),
    is_active: schedule?.is_active ?? true,
  });
  let busy = $state(false);

  async function save() {
    if (!formData.name?.trim()) {
      errorToast(t('teams.oncall.nameRequired'));
      return;
    }
    busy = true;
    try {
      const payload = {
        name: formData.name,
        description: formData.description,
        timezone: formData.timezone,
        is_active: formData.is_active,
      };
      if (schedule?.id) {
        await onCallSchedules.update(schedule.id, payload);
        successToast(t('teams.oncall.scheduleUpdated'));
      } else {
        await onCallSchedules.createForTeam(teamId, payload);
        successToast(t('teams.oncall.scheduleCreated'));
      }
      onSaved?.();
    } catch (err) {
      errorToast(err.message || t('teams.oncall.failedToSaveSchedule'));
    } finally {
      busy = false;
    }
  }
</script>

<Modal isOpen={true} onclose={onCancel} onSubmit={save} submitDisabled={busy} maxWidth="max-w-lg">
  <ModalHeader
    title={schedule ? t('teams.oncall.editSchedule') : t('teams.oncall.createSchedule')}
    onClose={onCancel}
  />
  <div class="px-6 py-4">
    <form onsubmit={(e) => { e.preventDefault(); save(); }} class="space-y-4">
      <div>
        <label for="schedule-name" class="block text-sm font-medium" style="color: var(--ds-text)">
          {t('teams.oncall.name')}
        </label>
        <Input id="schedule-name" bind:value={formData.name} required />
      </div>
      <div>
        <label for="schedule-description" class="block text-sm font-medium" style="color: var(--ds-text)">
          {t('teams.descriptionOptional')}
        </label>
        <Textarea id="schedule-description" bind:value={formData.description} rows={2} />
      </div>
      <div>
        <label for="schedule-timezone" class="block text-sm font-medium" style="color: var(--ds-text)">
          {t('teams.oncall.timezone')}
        </label>
        <TimezonePicker
          id="schedule-timezone"
          bind:value={formData.timezone}
          inputTestid="schedule-timezone"
          menuTestid="schedule-timezone-listbox"
          optionTestid={() => 'schedule-timezone-option'}
        />
      </div>
      <div>
        <Checkbox
          id="schedule-is-active"
          bind:checked={formData.is_active}
          label={t('teams.active')}
          size="small"
        />
      </div>
    </form>
  </div>
  <DialogFooter
    confirmLabel={schedule ? t('common.save') : t('teams.oncall.createSchedule')}
    onCancel={onCancel}
    onConfirm={save}
    showKeyboardHint
    confirmTestid="schedule-save"
  />
</Modal>
