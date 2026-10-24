<script>
  import Globe from '@lucide/svelte/icons/globe';
  import Building2 from '@lucide/svelte/icons/building-2';
  import Calendar from '@lucide/svelte/icons/calendar';
  import Tag from '@lucide/svelte/icons/tag';
  import Modal from './Modal.svelte';
  import ModalHeader from './ModalHeader.svelte';
  import DialogFooter from './DialogFooter.svelte';
  import Button from '../components/Button.svelte';
  import Input from '../components/Input.svelte';
  import Select from '../components/Select.svelte';
  import Label from '../components/Label.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { canChangePlanningScope } from '../utils/planningScope.js';
	import SelectField from '../components/SelectField.svelte';
	import TextareaField from '../components/TextareaField.svelte';
	import TextField from '../components/TextField.svelte';

  let {
    iteration = null,
    workspaceId = null,
    iterationTypes = [],
    canManageGlobal = false,
    onsave = () => {},
    oncancel = () => {}
  } = $props();

  // Snapshot iteration prop into editable form state.
  // svelte-ignore state_referenced_locally
  let formData = $state({
    name: iteration?.name || '',
    description: iteration?.description || '',
    start_date: iteration?.start_date ? iteration.start_date.split('T')[0] : '',
    end_date: iteration?.end_date ? iteration.end_date.split('T')[0] : '',
    status: iteration?.status || 'planned',
    type_id: iteration?.type_id || null,
    is_global: iteration ? iteration.is_global : !workspaceId,
    workspace_id: iteration?.workspace_id || (workspaceId ? parseInt(workspaceId) : null)
  });

  let error = $state('');
  let saving = $state(false);

  const statusOptions = $derived([
    { value: 'planned', label: t('iterations.statusPlanned') },
    { value: 'active', label: t('iterations.statusActive') },
    ...(formData.status === 'completed'
      ? [{ value: 'completed', label: t('iterations.statusCompleted') }]
      : []),
    { value: 'cancelled', label: t('iterations.statusCancelled') }
  ]);

  function handleCancel() {
    oncancel();
  }

  async function handleSave() {
    if (saving) return;

    error = '';

    // Validation
    if (!formData.name.trim()) {
      error = t('iterations.iterationNameRequired');
      return;
    }

    if (!formData.start_date) {
      error = t('iterations.startDateRequired');
      return;
    }

    if (!formData.end_date) {
      error = t('iterations.endDateRequired');
      return;
    }

    if (!formData.type_id) {
      error = t('iterations.typeRequired');
      return;
    }

    if (new Date(formData.end_date) < new Date(formData.start_date)) {
      error = t('iterations.endDateMustBeAfterStart');
      return;
    }

    // Ensure global iterations don't have workspace_id
    const dataToSave = { ...formData };
    if (dataToSave.is_global) {
      dataToSave.workspace_id = null;
    } else {
      dataToSave.workspace_id = workspaceId ? parseInt(workspaceId) : null;
    }

    saving = true;
    try {
      await onsave(dataToSave);
    } catch (err) {
      error = err.message || t('iterations.failedToSaveIteration');
    } finally {
      saving = false;
    }
  }

  function toggleScope() {
    formData.is_global = !formData.is_global;
    if (formData.is_global) {
      formData.workspace_id = null;
    } else {
      formData.workspace_id = workspaceId ? parseInt(workspaceId) : null;
    }
  }

  let canToggleGlobal = $derived(canChangePlanningScope(canManageGlobal, iteration));
</script>

<Modal
  isOpen={true}
  onclose={handleCancel}
  maxWidth="max-w-2xl"
  onSubmit={handleSave}
  submitDisabled={saving}
>
  {#snippet children(submitHint)}
  <!-- Modal header -->
  <ModalHeader title={iteration ? t('iterations.editIteration') : t('iterations.createIteration')} showCloseButton={false} />

  <!-- Modal content -->
  <div class="px-6 py-4">
    <form onsubmit={(e) => { e.preventDefault(); handleSave(); }} class="space-y-4">
      <!-- Error Message -->
      {#if error}
        <div
          class="p-3 rounded"
          style="background-color: var(--ds-danger-subtle); border: 1px solid var(--ds-border-danger);"
        >
          <p class="text-sm" style="color: var(--ds-text-danger);">{error}</p>
        </div>
      {/if}

      {#if workspaceId}
      <!-- Scope Toggle -->
      <div class="p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-3">
            {#if formData.is_global}
              <Globe class="w-5 h-5" style="color: var(--ds-interactive);" />
              <div>
                <p class="font-medium text-sm" style="color: var(--ds-text);">{t('iterations.globalIteration')}</p>
                <p class="text-xs" style="color: var(--ds-text-subtle);">{t('iterations.globalIterationDescription')}</p>
              </div>
            {:else}
              <Building2 class="w-5 h-5" style="color: var(--ds-interactive);" />
              <div>
                <p class="font-medium text-sm" style="color: var(--ds-text);">{t('iterations.localIteration')}</p>
                <p class="text-xs" style="color: var(--ds-text-subtle);">{t('iterations.localIterationDescription')}</p>
              </div>
            {/if}
          </div>
          {#if canToggleGlobal}
            <button
              type="button"
              class="px-3 py-1.5 text-sm rounded border transition-colors"
              style="border-color: var(--ds-border); color: var(--ds-interactive);"
              onclick={toggleScope}
            >
              {t('iterations.switchTo', { scope: formData.is_global ? t('iterations.local') : t('iterations.global') })}
            </button>
          {/if}
        </div>
      </div>
      {/if}

      <!-- Name -->
      <div>
        <TextField
          label={t('common.name')}
          id="iteration-name-input"
          required
          labelColor="default"
          placeholder={t('iterations.iterationNamePlaceholder')}
          bind:value={formData.name}
        />
      </div>

      <!-- Description -->
      <div>
        <TextareaField
          label={t('common.description')}
          labelColor="default"
          placeholder={t('iterations.iterationDescriptionPlaceholder')}
          rows={3}
          bind:value={formData.description}
        />
      </div>

      <!-- Type -->
      <div>
        <Label color="default" required class="mb-1.5"><Tag class="w-4 h-4 inline-block mr-1" />{t('common.type')}</Label>
        <Select id="iteration-type-select" bind:value={formData.type_id} required options={[{ value: null, label: t('iterations.selectType'), disabled: true }, ...iterationTypes.map(type => ({ value: type.id, label: type.name }))]} />
      </div>

      <!-- Date Range -->
      <div class="grid grid-cols-2 gap-4">
        <div>
          <Label color="default" required class="mb-1.5"><Calendar class="w-4 h-4 inline-block mr-1" />{t('common.startDate')}</Label>
          <Input
            id="iteration-start-date-input"
            type="date"
            bind:value={formData.start_date}
            required
          />
        </div>
        <div>
          <Label color="default" required class="mb-1.5"><Calendar class="w-4 h-4 inline-block mr-1" />{t('common.endDate')}</Label>
          <Input
            id="iteration-end-date-input"
            type="date"
            bind:value={formData.end_date}
            required
          />
        </div>
      </div>

      <!-- Status -->
      <div>
        <SelectField
          label={t('common.status')}
          id="iteration-status-select"
          labelColor="default"
          options={statusOptions.map(status => ({ value: status.value, label: status.label }))}
          bind:value={formData.status}
        />
      </div>

    </form>
  </div>

  <DialogFooter
    onCancel={handleCancel}
    onConfirm={handleSave}
    confirmLabel={iteration ? t('iterations.updateIteration') : t('iterations.createIteration')}
    disabled={saving}
    loading={saving}
    showKeyboardHint={true}
    confirmKeyboardHint={submitHint}
  />
  {/snippet}
</Modal>
