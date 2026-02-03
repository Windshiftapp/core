<script>
  import { Globe, Building2, Calendar, Tag, FileText } from 'lucide-svelte';
  import Modal from './Modal.svelte';
  import DialogFooter from './DialogFooter.svelte';
  import Button from '../components/Button.svelte';
  import Input from '../components/Input.svelte';
  import Select from '../components/Select.svelte';
  import Textarea from '../components/Textarea.svelte';
  import Label from '../components/Label.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let {
    iteration = null,
    workspaceId = null,
    iterationTypes = [],
    canManageGlobal = false,
    onsave = () => {},
    oncancel = () => {}
  } = $props();

  let formData = $state({
    name: iteration?.name || '',
    description: iteration?.description || '',
    start_date: iteration?.start_date ? iteration.start_date.split('T')[0] : '',
    end_date: iteration?.end_date ? iteration.end_date.split('T')[0] : '',
    status: iteration?.status || 'planned',
    type_id: iteration?.type_id || null,
    is_global: iteration?.is_global || false,
    workspace_id: iteration?.workspace_id || (workspaceId ? parseInt(workspaceId) : null)
  });

  let error = $state('');
  let saving = $state(false);

  const statusOptions = $derived([
    { value: 'planned', label: t('sprints.statusPlanned') },
    { value: 'active', label: t('sprints.statusActive') },
    { value: 'completed', label: t('sprints.statusCompleted') },
    { value: 'cancelled', label: t('sprints.statusCancelled') }
  ]);

  function handleCancel() {
    oncancel();
  }

  async function handleSave() {
    error = '';

    // Validation
    if (!formData.name.trim()) {
      error = t('sprints.iterationNameRequired');
      return;
    }

    if (!formData.start_date) {
      error = t('sprints.startDateRequired');
      return;
    }

    if (!formData.end_date) {
      error = t('sprints.endDateRequired');
      return;
    }

    if (!formData.type_id) {
      error = t('sprints.typeRequired');
      return;
    }

    if (new Date(formData.end_date) < new Date(formData.start_date)) {
      error = t('sprints.endDateMustBeAfterStart');
      return;
    }

    // Ensure global iterations don't have workspace_id
    const dataToSave = { ...formData };
    if (dataToSave.is_global) {
      dataToSave.workspace_id = null;
    } else {
      dataToSave.workspace_id = workspaceId ? parseInt(workspaceId) : null;
    }

    try {
      saving = true;
      onsave(dataToSave);
    } catch (err) {
      error = err.message || t('sprints.failedToSaveIteration');
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

  let canToggleGlobal = $derived(canManageGlobal && (!iteration || iteration.is_global));
</script>

<Modal
  isOpen={true}
  onclose={handleCancel}
  maxWidth="max-w-2xl"
  onSubmit={handleSave}
  submitDisabled={saving}
  let:submitHint
>
  <!-- Modal header -->
  <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
    <h3 class="text-lg font-semibold" style="color: var(--ds-text);">
      {iteration ? t('sprints.editIteration') : t('sprints.createIteration')}
    </h3>
  </div>

  <!-- Modal content -->
  <div class="px-6 py-4">
    <form onsubmit={(e) => { e.preventDefault(); handleSave(); }} class="space-y-4">
      <!-- Error Message -->
      {#if error}
        <div class="p-3 rounded" style="background-color: #fee; border: 1px solid #fcc;">
          <p class="text-sm" style="color: #c33;">{error}</p>
        </div>
      {/if}

      <!-- Scope Toggle -->
      <div class="p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-3">
            {#if formData.is_global}
              <Globe class="w-5 h-5" style="color: var(--ds-interactive);" />
              <div>
                <p class="font-medium text-sm" style="color: var(--ds-text);">{t('sprints.globalIteration')}</p>
                <p class="text-xs" style="color: var(--ds-text-subtle);">{t('sprints.globalIterationDescription')}</p>
              </div>
            {:else}
              <Building2 class="w-5 h-5" style="color: var(--ds-interactive);" />
              <div>
                <p class="font-medium text-sm" style="color: var(--ds-text);">{t('sprints.localIteration')}</p>
                <p class="text-xs" style="color: var(--ds-text-subtle);">{t('sprints.localIterationDescription')}</p>
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
              {t('sprints.switchTo', { scope: formData.is_global ? t('sprints.local') : t('sprints.global') })}
            </button>
          {/if}
        </div>
      </div>

      <!-- Name -->
      <div>
        <Label color="default" required class="mb-1.5">{t('common.name')}</Label>
        <Input
          bind:value={formData.name}
          placeholder={t('sprints.iterationNamePlaceholder')}
          required
        />
      </div>

      <!-- Description -->
      <div>
        <Label color="default" class="mb-1.5">{t('common.description')}</Label>
        <Textarea
          bind:value={formData.description}
          placeholder={t('sprints.iterationDescriptionPlaceholder')}
          rows={3}
        />
      </div>

      <!-- Type -->
      <div>
        <Label color="default" required class="mb-1.5"><Tag class="w-4 h-4 inline-block mr-1" />{t('common.type')}</Label>
        <Select bind:value={formData.type_id} required>
          <option value={null} disabled>{t('sprints.selectType')}</option>
          {#each iterationTypes as type}
            <option value={type.id}>{type.name}</option>
          {/each}
        </Select>
      </div>

      <!-- Date Range -->
      <div class="grid grid-cols-2 gap-4">
        <div>
          <Label color="default" required class="mb-1.5"><Calendar class="w-4 h-4 inline-block mr-1" />{t('common.startDate')}</Label>
          <Input
            type="date"
            bind:value={formData.start_date}
            required
          />
        </div>
        <div>
          <Label color="default" required class="mb-1.5"><Calendar class="w-4 h-4 inline-block mr-1" />{t('common.endDate')}</Label>
          <Input
            type="date"
            bind:value={formData.end_date}
            required
          />
        </div>
      </div>

      <!-- Status -->
      <div>
        <Label color="default" class="mb-1.5">{t('common.status')}</Label>
        <Select bind:value={formData.status}>
          {#each statusOptions as status}
            <option value={status.value}>{status.label}</option>
          {/each}
        </Select>
      </div>

    </form>
  </div>

  <DialogFooter
    onCancel={handleCancel}
    onConfirm={handleSave}
    confirmLabel={iteration ? t('sprints.updateIteration') : t('sprints.createIteration')}
    disabled={saving}
    loading={saving}
    showKeyboardHint={true}
    confirmKeyboardHint={submitHint}
  />
</Modal>
