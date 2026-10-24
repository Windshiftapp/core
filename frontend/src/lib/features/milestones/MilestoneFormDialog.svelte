<script>
  import Building2 from '@tabler/icons-svelte-runes/icons/building';
  import Globe from '@tabler/icons-svelte-runes/icons/world';
  import { t } from '../../stores/i18n.svelte.js';
  import { categoriesStore } from '../../stores/categories.js';
  import { canChangePlanningScope } from '../../utils/planningScope.js';
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';
  import BasePicker from '../../pickers/BasePicker.svelte';
  import Label from '../../components/Label.svelte';
	import TextField from '../../components/TextField.svelte';
  import MilkdownEditor from '../../editors/LazyMilkdownEditor.svelte';

  let {
    isOpen = $bindable(false),
    formData = $bindable({
      name: '',
      description: '',
      target_date: '',
      status: 'planning',
      category_id: null,
      is_global: true,
      workspace_id: null
    }),
    editingMilestone = null,
    isGlobalView = false,
    workspaceId = null,
    canManageGlobal = false,
    canManageWorkspace = false,
    saving = false,
    onclose = null,
    onSubmit = null
  } = $props();

  const statusOptions = $derived([
    { value: 'planning', label: t('milestones.status.planning') },
    { value: 'in-progress', label: t('milestones.status.inProgress') },
    { value: 'completed', label: t('milestones.status.completed') },
    { value: 'cancelled', label: t('milestones.status.cancelled') }
  ]);

  const canToggleScope = $derived(
    !isGlobalView &&
      canManageGlobal &&
      canManageWorkspace &&
      canChangePlanningScope(canManageGlobal, editingMilestone)
  );

  function toggleScope() {
    const nextIsGlobal = !formData.is_global;
    formData = {
      ...formData,
      is_global: nextIsGlobal,
      workspace_id: nextIsGlobal ? null : (workspaceId ? parseInt(workspaceId, 10) : null)
    };
  }

  function close() {
    isOpen = false;
    onclose?.();
  }

  function submit() {
    if (saving || !formData.name.trim()) return;
    onSubmit?.();
  }
</script>

<Modal
  {isOpen}
  onclose={close}
  onSubmit={submit}
  submitDisabled={saving || !formData.name.trim()}
  maxWidth="max-w-2xl"
  dataTestid="milestone-form-dialog"
>
  {#snippet children(submitHint)}
    <ModalHeader title={editingMilestone ? t('common.edit') : t('common.create')} showCloseButton={false} />

    <div class="px-6 py-4">
      <form onsubmit={(event) => { event.preventDefault(); submit(); }}>
        <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
          <div>
            <TextField
              label={t('milestones.milestoneName')}
              id="milestone-name"
              required
              type="text"
              placeholder={t('milestones.milestoneNamePlaceholder')}
              bind:value={formData.name}
            />
          </div>

          <div>
            <TextField
              label={t('milestones.targetDate')}
              id="milestone-target-date"
              type="date"
              bind:value={formData.target_date}
            />
          </div>

          <div>
            <Label for="milestone-category" class="mb-2">{t('common.category')}</Label>
            <BasePicker
              bind:value={formData.category_id}
              items={$categoriesStore}
              placeholder={t('milestones.noCategory')}
              showUnassigned={true}
              unassignedLabel={t('milestones.noCategory')}
              getValue={(item) => item.id}
              getLabel={(item) => item.name}
            />
          </div>

          <div>
            <Label for="milestone-status-picker" class="mb-2">{t('common.status')}</Label>
            <BasePicker
              id="milestone-status-picker"
              bind:value={formData.status}
              items={statusOptions}
              placeholder={t('milestones.selectStatus')}
              getValue={(item) => item.value}
              getLabel={(item) => item.label}
            />
          </div>

          {#if !isGlobalView && canManageGlobal}
            <div class="md:col-span-2">
              <div class="p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
                <div class="flex items-center justify-between">
                  <div class="flex items-center gap-3">
                    {#if formData.is_global}
                      <Globe class="w-5 h-5" style="color: var(--ds-interactive);" />
                      <div>
                        <p class="font-medium text-sm" style="color: var(--ds-text);">{t('milestones.globalMilestone')}</p>
                        <p class="text-xs" style="color: var(--ds-text-subtle);">{t('milestones.globalMilestoneDescription')}</p>
                      </div>
                    {:else}
                      <Building2 class="w-5 h-5" style="color: var(--ds-interactive);" />
                      <div>
                        <p class="font-medium text-sm" style="color: var(--ds-text);">{t('milestones.localMilestone')}</p>
                        <p class="text-xs" style="color: var(--ds-text-subtle);">{t('milestones.localMilestoneDescription')}</p>
                      </div>
                    {/if}
                  </div>
                  {#if canToggleScope}
                    <button
                      type="button"
                      class="px-3 py-1.5 text-sm rounded border transition-colors"
                      style="border-color: var(--ds-border); color: var(--ds-interactive);"
                      onclick={toggleScope}
                    >
                      {t('milestones.switchTo', { scope: formData.is_global ? t('milestones.local') : t('milestones.global') })}
                    </button>
                  {/if}
                </div>
              </div>
            </div>
          {/if}

          <div class="md:col-span-2">
            <Label class="mb-2">{t('common.description')}</Label>
            <MilkdownEditor
              bind:content={formData.description}
              placeholder={t('milestones.descriptionPlaceholder')}
              showToolbar={true}
              compact={true}
              allowImageUpload={false}
              testId="milestone-description-editor"
            />
          </div>
        </div>
      </form>
    </div>

    <DialogFooter
      onCancel={close}
      onConfirm={submit}
      confirmLabel={editingMilestone ? t('common.update') : t('common.create')}
      disabled={!formData.name.trim()}
      loading={saving}
      showKeyboardHint={true}
      confirmKeyboardHint={submitHint}
    />
  {/snippet}
</Modal>
