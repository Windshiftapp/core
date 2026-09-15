<script>
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';
  import Button from '../../components/Button.svelte';
  import FormField from '../../components/FormField.svelte';
  import Select from '../../components/Select.svelte';
  import UserPicker from '../../pickers/UserPicker.svelte';
  import { api } from '../../api.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { requirementTypeOptions } from './requirementTypes.js';
  import { requirementStatusOptions } from './requirementStatuses.js';

  let {
    workspaceId,
    pageId,
    open = $bindable(false),
    onPromoted = () => {},
  } = $props();

  let requirementType = $state('use_case');
  let status = $state('draft');
  let ownerId = $state(null);
  let saving = $state(false);
  let error = $state('');

  const typeOptions = $derived(requirementTypeOptions(t));
  const statusOptions = $derived(requirementStatusOptions(t));

  async function submit() {
    saving = true;
    error = '';
    try {
      const promoted = await api.requirements.promote(workspaceId, pageId, {
        requirement_type: requirementType,
        status,
        owner_id: ownerId,
      });
      open = false;
      onPromoted(promoted);
    } catch (err) {
      error = err?.message || t('requirements.promoteError');
    } finally {
      saving = false;
    }
  }
</script>

<Modal bind:isOpen={open} maxWidth="max-w-md">
  <ModalHeader title={t('requirements.promote')} onclose={() => (open = false)} />
  <div class="flex flex-col gap-4 p-4">
    {#if error}
      <p class="text-sm text-[var(--ds-text-danger)]">{error}</p>
    {/if}
    <FormField label={t('requirements.fieldType')} required>
      <Select bind:value={requirementType} options={typeOptions} />
    </FormField>
    <FormField label={t('requirements.fieldStatus')}>
      <Select bind:value={status} options={statusOptions} />
    </FormField>
    <FormField label={t('requirements.fieldOwner')}>
      <UserPicker bind:value={ownerId} {workspaceId} />
    </FormField>
  </div>
  <DialogFooter>
    <Button variant="secondary" onclick={() => (open = false)} disabled={saving}>
      {t('common.cancel')}
    </Button>
    <Button onclick={submit} disabled={saving}>
      {t('requirements.promote')}
    </Button>
  </DialogFooter>
</Modal>
