<script>
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast, successToast } from '../stores/toasts.svelte.js';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';
  import Input from '../components/Input.svelte';
  import Textarea from '../components/Textarea.svelte';
  import UserPicker from '../pickers/UserPicker.svelte';

  let { scheduleId, onSaved, onCancel } = $props();

  let userValue = $state(null);
  let overrideValue = $state(null);
  let formData = $state({
    user_id: null,
    override_user_id: null,
    start_time: '',
    end_time: '',
    reason: '',
  });
  let busy = $state(false);

  function onUserPicked(user) {
    if (user?.id != null) formData.user_id = user.id;
  }
  function onOverridePicked(user) {
    if (user?.id != null) formData.override_user_id = user.id;
  }

  function toRFC3339(localDateTime) {
    if (!localDateTime) return '';
    // datetime-local gives "YYYY-MM-DDTHH:MM"; convert to ISO with local TZ offset.
    const d = new Date(localDateTime);
    if (Number.isNaN(d.getTime())) return '';
    return d.toISOString();
  }

  async function save() {
    if (!formData.user_id || !formData.override_user_id) {
      errorToast(t('teams.oncall.overrideUsersRequired'));
      return;
    }
    if (!formData.start_time || !formData.end_time) {
      errorToast(t('teams.oncall.overrideTimesRequired'));
      return;
    }
    busy = true;
    try {
      await api.onCallSchedules.createOverride(scheduleId, {
        user_id: formData.user_id,
        override_user_id: formData.override_user_id,
        start_time: toRFC3339(formData.start_time),
        end_time: toRFC3339(formData.end_time),
        reason: formData.reason,
      });
      successToast(t('teams.oncall.overrideCreated'));
      onSaved?.();
    } catch (err) {
      errorToast(err.message || t('teams.oncall.failedToCreateOverride'));
    } finally {
      busy = false;
    }
  }
</script>

<Modal isOpen={true} onclose={onCancel} onSubmit={save} submitDisabled={busy} maxWidth="max-w-lg">
  <ModalHeader title={t('teams.oncall.createOverride')} onClose={onCancel} />
  <div class="px-6 py-4 space-y-4">
    <div>
      <div class="block text-sm font-medium mb-1" style="color: var(--ds-text)">
        {t('teams.oncall.replacedUser')}
      </div>
      <UserPicker
        bind:value={userValue}
        placeholder={t('teams.searchUser')}
        onSelect={onUserPicked}
      />
    </div>
    <div>
      <div class="block text-sm font-medium mb-1" style="color: var(--ds-text)">
        {t('teams.oncall.replacementUser')}
      </div>
      <UserPicker
        bind:value={overrideValue}
        placeholder={t('teams.searchUser')}
        onSelect={onOverridePicked}
      />
    </div>
    <div>
      <label for="override-start" class="block text-sm font-medium" style="color: var(--ds-text)">
        {t('teams.oncall.startTime')}
      </label>
      <Input id="override-start" type="datetime-local" bind:value={formData.start_time} required />
    </div>
    <div>
      <label for="override-end" class="block text-sm font-medium" style="color: var(--ds-text)">
        {t('teams.oncall.endTime')}
      </label>
      <Input id="override-end" type="datetime-local" bind:value={formData.end_time} required />
    </div>
    <div>
      <label for="override-reason" class="block text-sm font-medium" style="color: var(--ds-text)">
        {t('teams.oncall.reasonOptional')}
      </label>
      <Textarea id="override-reason" bind:value={formData.reason} rows={2} />
    </div>
  </div>
  <DialogFooter
    confirmLabel={t('teams.oncall.createOverride')}
    onCancel={onCancel}
    onConfirm={save}
    showKeyboardHint
    confirmTestid="override-save"
  />
</Modal>
