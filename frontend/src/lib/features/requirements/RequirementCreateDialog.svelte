<script>
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';
  import Button from '../../components/Button.svelte';
  import FormField from '../../components/FormField.svelte';
  import Input from '../../components/Input.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import Select from '../../components/Select.svelte';
  import UserPicker from '../../pickers/UserPicker.svelte';
  import PagePicker from '../../pickers/PagePicker.svelte';
  import { api } from '../../api.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { requirementTypeOptions } from './requirementTypes.js';
  import { requirementStatusOptions } from './requirementStatuses.js';

  let {
    workspaceId,
    open = $bindable(false),
    onCreated = () => {},
  } = $props();

  let title = $state('');
  let content = $state('');
  let requirementType = $state('use_case');
  let status = $state('draft');
  let ownerId = $state(null);
  let parentId = $state(null);
  let saving = $state(false);
  let error = $state('');

  const typeOptions = $derived(requirementTypeOptions(t));
  const statusOptions = $derived(requirementStatusOptions(t));

  function resetForm() {
    title = '';
    content = '';
    requirementType = 'use_case';
    status = 'draft';
    ownerId = null;
    parentId = null;
    error = '';
  }

  async function submit() {
    if (!title.trim()) {
      error = t('requirements.createTitleRequired');
      return;
    }
    saving = true;
    error = '';
    try {
      const created = await api.requirements.create(workspaceId, {
        title: title.trim(),
        content,
        requirement_type: requirementType,
        status,
        owner_id: ownerId,
        parent_id: parentId,
      });
      open = false;
      resetForm();
      onCreated(created);
    } catch (err) {
      error = err?.message || t('requirements.createError');
    } finally {
      saving = false;
    }
  }
</script>

<Modal bind:isOpen={open} maxWidth="max-w-lg" onclose={resetForm}>
  <ModalHeader title={t('requirements.create')} onclose={() => (open = false)} />
  <div class="flex flex-col gap-4 p-4">
    {#if error}
      <p class="text-sm text-[var(--ds-text-danger)]">{error}</p>
    {/if}
    <FormField label={t('requirements.fieldTitle')} required>
      <Input bind:value={title} />
    </FormField>
    <FormField label={t('requirements.fieldContent')}>
      <Textarea bind:value={content} rows={4} />
    </FormField>
    <FormField label={t('requirements.fieldType')} required>
      <Select bind:value={requirementType} options={typeOptions} />
    </FormField>
    <FormField label={t('requirements.fieldStatus')}>
      <Select bind:value={status} options={statusOptions} />
    </FormField>
    <FormField label={t('requirements.fieldOwner')}>
      <UserPicker bind:value={ownerId} {workspaceId} />
    </FormField>
    <FormField label={t('requirements.fieldParent')}>
      <PagePicker bind:value={parentId} {workspaceId} />
    </FormField>
  </div>
  <DialogFooter>
    <Button variant="secondary" onclick={() => (open = false)} disabled={saving}>
      {t('common.cancel')}
    </Button>
    <Button onclick={submit} disabled={saving}>
      {t('requirements.create')}
    </Button>
  </DialogFooter>
</Modal>
