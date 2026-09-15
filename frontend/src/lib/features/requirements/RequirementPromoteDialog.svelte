<script>
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';
  import Button from '../../components/Button.svelte';
  import Checkbox from '../../components/Checkbox.svelte';
  import FormField from '../../components/FormField.svelte';
  import Select from '../../components/Select.svelte';
  import UserPicker from '../../pickers/UserPicker.svelte';
  import Spinner from '../../components/Spinner.svelte';
  import { api } from '../../api.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { requirementTypeOptions } from './requirementTypes.js';
  import { requirementStatusOptions } from './requirementStatuses.js';
  import { getRequirementStarterContent } from './requirementStarterTemplates.js';

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
  let pageContent = $state('');
  let pageLoading = $state(false);
  let insertTemplate = $state(true);
  let loadSeq = 0;

  const typeOptions = $derived(requirementTypeOptions(t));
  const statusOptions = $derived(requirementStatusOptions(t));
  const pageIsEmpty = $derived(!pageContent.trim());

  async function loadPage() {
    const seq = ++loadSeq;
    pageLoading = true;
    error = '';
    try {
      const page = await api.pages.getPage(workspaceId, pageId);
      if (seq !== loadSeq) return;
      pageContent = page?.content ?? '';
      insertTemplate = !pageContent.trim();
    } catch (err) {
      if (seq !== loadSeq) return;
      console.error('Failed to load page for promote:', err);
      pageContent = '';
      insertTemplate = true;
    } finally {
      if (seq === loadSeq) pageLoading = false;
    }
  }

  $effect(() => {
    if (open && workspaceId && pageId) {
      requirementType = 'use_case';
      status = 'draft';
      ownerId = null;
      void loadPage();
    }
    if (!open) {
      loadSeq += 1;
      error = '';
      pageContent = '';
      pageLoading = false;
      insertTemplate = true;
    }
  });

  async function submit() {
    saving = true;
    error = '';
    try {
      if (pageIsEmpty && insertTemplate) {
        const template = getRequirementStarterContent(requirementType, t);
        if (template) {
          await api.pages.updatePage(workspaceId, pageId, { content: template });
        }
      }
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
    {#if pageLoading}
      <div class="flex justify-center py-2"><Spinner /></div>
    {/if}
    {#if error}
      <p class="text-sm text-[var(--ds-text-danger)]">{error}</p>
    {/if}
    <FormField label={t('requirements.fieldType')} required>
      <Select bind:value={requirementType} options={typeOptions} disabled={pageLoading || saving} />
    </FormField>
    <FormField label={t('requirements.fieldStatus')}>
      <Select bind:value={status} options={statusOptions} disabled={pageLoading || saving} />
    </FormField>
    <FormField label={t('requirements.fieldOwner')}>
      <UserPicker bind:value={ownerId} {workspaceId} disabled={pageLoading || saving} />
    </FormField>
    {#if !pageLoading && pageIsEmpty}
      <Checkbox
        bind:checked={insertTemplate}
        label={t('requirements.templates.insertOnPromote')}
        disabled={saving}
      />
    {/if}
  </div>
  <DialogFooter>
    <Button variant="secondary" onclick={() => (open = false)} disabled={saving}>
      {t('common.cancel')}
    </Button>
    <Button onclick={submit} disabled={saving || pageLoading}>
      {t('requirements.promote')}
    </Button>
  </DialogFooter>
</Modal>
