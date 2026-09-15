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
  import { confirm } from '../../composables/useConfirm.js';
  import { requirementTypeOptions } from './requirementTypes.js';
  import { requirementStatusOptions } from './requirementStatuses.js';
  import { getRequirementStarterContent } from './requirementStarterTemplates.js';

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
  let previousRequirementType = $state('use_case');
  let contentTouched = $state(false);

  const typeOptions = $derived(requirementTypeOptions(t));
  const statusOptions = $derived(requirementStatusOptions(t));

  function applyTemplate(type) {
    content = getRequirementStarterContent(type, t);
    contentTouched = false;
  }

  function resetForm() {
    title = '';
    requirementType = 'use_case';
    status = 'draft';
    ownerId = null;
    parentId = null;
    error = '';
    previousRequirementType = 'use_case';
    applyTemplate('use_case');
  }

  async function confirmReplace() {
    return confirm({
      title: t('requirements.fieldType'),
      message: t('requirements.templates.replaceConfirm'),
      confirmText: t('common.confirm'),
      cancelText: t('common.cancel'),
      variant: 'info',
    });
  }

  async function handleTypeChange() {
    const newType = requirementType;
    const oldType = previousRequirementType;
    if (newType === oldType) return;

    if (content.trim() && contentTouched) {
      const ok = await confirmReplace();
      if (!ok) {
        requirementType = oldType;
        return;
      }
    }

    applyTemplate(newType);
    previousRequirementType = newType;
  }

  async function resetToTemplate() {
    if (content.trim() && contentTouched) {
      const ok = await confirmReplace();
      if (!ok) return;
    }
    applyTemplate(requirementType);
  }

  function onContentInput() {
    contentTouched = true;
  }

  $effect(() => {
    if (open) resetForm();
  });

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
      onCreated(created);
    } catch (err) {
      error = err?.message || t('requirements.createError');
    } finally {
      saving = false;
    }
  }
</script>

<Modal bind:isOpen={open} maxWidth="max-w-lg">
  <ModalHeader title={t('requirements.create')} onclose={() => (open = false)} />
  <div class="flex flex-col gap-4 p-4">
    {#if error}
      <p class="text-sm text-[var(--ds-text-danger)]">{error}</p>
    {/if}
    <FormField label={t('requirements.fieldTitle')} required>
      <Input bind:value={title} />
    </FormField>
    <FormField label={t('requirements.fieldContent')}>
      <div class="content-field">
        <Textarea bind:value={content} rows={8} oninput={onContentInput} />
        <Button variant="ghost" size="sm" onclick={resetToTemplate} disabled={saving}>
          {t('requirements.templates.reset')}
        </Button>
      </div>
    </FormField>
    <FormField label={t('requirements.fieldType')} required>
      <Select bind:value={requirementType} options={typeOptions} onchange={handleTypeChange} />
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

<style>
  .content-field {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.35rem;
  }
</style>
