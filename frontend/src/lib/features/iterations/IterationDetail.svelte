<script>
  import { onMount } from 'svelte';
  import IconCalendar from '@tabler/icons-svelte-runes/icons/calendar';
  import IconTarget from '@tabler/icons-svelte-runes/icons/target';
  import IconEdit from '@tabler/icons-svelte-runes/icons/edit';
  import IconTrash from '@tabler/icons-svelte-runes/icons/trash';
  import IconDots from '@tabler/icons-svelte-runes/icons/dots';
  import IconWorld from '@tabler/icons-svelte-runes/icons/world';
  import IconBuilding from '@tabler/icons-svelte-runes/icons/building';
  import IconMessageChatbot from '@tabler/icons-svelte-runes/icons/message-chatbot';
  import IconCircleCheck from '@tabler/icons-svelte-runes/icons/circle-check';
  import IterationBurndown from './IterationBurndown.svelte';
  import { iterations } from '../../api/milestones.js';
  import { navigate } from '../../router.js';
  import Button from '../../components/Button.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import Label from '../../components/Label.svelte';
  import { formatDateShort, daysUntil } from '../../utils/dateFormatter.js';
  import BasePicker from '../../pickers/BasePicker.svelte';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';
  import CompleteIterationDialog from '../../dialogs/CompleteIterationDialog.svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast, successToast } from '../../stores/toasts.svelte.js';
  import { confirm } from '../../composables/useConfirm.js';
  import { aiStore } from '../../stores/aiStore.svelte.js';
  import { permissionStore, isSystemAdmin } from '../../stores/permissions.svelte.js';
  import { workspacePermissions } from '../../stores/workspacePermissions.svelte.js';
  import { useProgressDetail } from '../../composables/useProgressDetail.svelte.js';
  import PlanningEntityDetail from '../shared/PlanningEntityDetail.svelte';
  import TextField from '../../components/TextField.svelte';
  import TextareaField from '../../components/TextareaField.svelte';

  let { iterationId, workspaceId = null } = $props();

  let showEditModal = $state(false);
  let showCompleteDialog = $state(false);
  let completionTargets = $state([]);
  let burndownData = $state(null);
  let formData = $state({
    name: '',
    description: '',
    start_date: '',
    end_date: '',
    status: 'planned',
    type_id: null,
    is_global: true,
    workspace_id: null
  });

  const detail = useProgressDetail({
    errorLabel: 'iteration progress',
    getStatusOptions: () => statusOptions,
    load: async () => {
      const [progress, entity, burndown] = await Promise.all([
        iterations.getProgress(iterationId),
        iterations.get(iterationId),
        iterations.getBurndown(iterationId).catch(() => null)
      ]);
      burndownData = burndown;
      return { progress, entity };
    }
  });

  let statusOptions = $derived([
    { value: 'planned', label: t('iterations.status.planned'), lozengeColor: 'grey' },
    { value: 'active', label: t('iterations.status.active'), lozengeColor: 'blue' },
    ...(detail.entity?.status === 'completed'
      ? [{ value: 'completed', label: t('iterations.status.completed'), lozengeColor: 'green' }]
      : []),
    { value: 'cancelled', label: t('iterations.status.cancelled'), lozengeColor: 'red' }
  ]);

  const canManage = $derived.by(() => {
    if (!detail.entity) return false;
    if ($isSystemAdmin) return true;
    if (detail.entity.is_global) {
      return $permissionStore.userPermissionKeys?.has('iteration.manage');
    } else {
      const wsId = detail.entity.workspace_id || workspaceId;
      if (!wsId) return false;
      return workspacePermissions.canAdminWorkspace(wsId) || 
             workspacePermissions.hasPermission(wsId, 'item.edit');
    }
  });

  onMount(() => {
    void detail.loadProgress();
  });

  function goBack() {
    if (workspaceId) {
      navigate(`/workspaces/${workspaceId}/iterations`);
    } else {
      navigate('/iterations');
    }
  }

  function startEdit() {
    if (detail.entity) {
      formData = {
        name: detail.entity.name,
        description: detail.entity.description || '',
        start_date: detail.entity.start_date || '',
        end_date: detail.entity.end_date || '',
        status: detail.entity.status,
        type_id: detail.entity.type_id,
        is_global: detail.entity.is_global !== false,
        workspace_id: detail.entity.workspace_id || null
      };
      showEditModal = true;
    }
  }

  async function saveIteration() {
    try {
      await iterations.update(iterationId, formData);
      showEditModal = false;
      await detail.loadProgress();
    } catch (err) {
      console.error('Failed to update iteration:', err);
      errorToast(t('dialogs.alerts.failedToUpdate', { error: err.message || err }));
    }
  }

  async function deleteIteration() {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('iterations.confirmDelete', { name: detail.progress?.iteration_name }),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger'
    });
    if (confirmed) {
      try {
        await iterations.delete(iterationId);
        goBack();
      } catch (err) {
        console.error('Failed to delete iteration:', err);
        errorToast(t('dialogs.alerts.failedToDelete', { error: err.message || err }));
      }
    }
  }

  async function openCompleteDialog() {
    try {
      const filters = detail.entity?.is_global
        ? { include_global: true }
        : { workspace_id: detail.entity?.workspace_id || workspaceId, include_global: true };
      const candidates = await iterations.getAll(filters);
      completionTargets = (candidates || []).filter(candidate => {
        if (candidate.id === detail.entity.id || !['planned', 'active'].includes(candidate.status)) return false;
        if (detail.entity.is_global) return candidate.is_global;
        return candidate.is_global || candidate.workspace_id === detail.entity.workspace_id;
      });
      showCompleteDialog = true;
    } catch (err) {
      errorToast(t('dialogs.alerts.failedToLoad', { error: err.message || err }));
    }
  }

  async function completeIteration(moveTarget) {
    try {
      const targetIterationId = moveTarget.type === 'iteration' ? moveTarget.iterationId : null;
      await iterations.complete(iterationId, targetIterationId);
      successToast(t('iterations.iterationCompleted', { name: detail.entity.name }));
      await detail.loadProgress();
    } catch (err) {
      errorToast(t('dialogs.alerts.failedToUpdate', { error: err.message || err }));
    }
  }

  const daysInfo = $derived(detail.progress?.end_date ? daysUntil(detail.progress.end_date, {
    overdue: (n) => t('iterations.daysOverdue', { count: n }),
    today: t('iterations.endsToday'),
    oneDay: t('iterations.oneDayRemaining'),
    remaining: (n) => t('iterations.daysRemaining', { count: n }),
  }) : null);

  function buildDropdownItems() {
    const items = [];

    if (canManage) {
      if (detail.entity.status === 'planned' || detail.entity.status === 'active') {
        items.push({
          id: 'complete',
          type: 'regular',
          icon: IconCircleCheck,
          title: t('iterations.completeIteration'),
          hoverClass: 'hover-bg',
          onClick: openCompleteDialog
        });
      }
      items.push({
        id: 'edit',
        type: 'regular',
        icon: IconEdit,
        title: t('common.edit'),
        hoverClass: 'hover-bg',
        onClick: startEdit
      });
    }

    if (aiStore.available && detail.progress?.total_items > 0) {
      items.push({
        id: 'dependencies',
        type: 'regular',
        icon: IconMessageChatbot,
        title: 'Analyze Dependencies',
        hoverClass: 'hover-bg',
        onClick: () => navigate(`/iterations/${iterationId}/dependencies`)
      });
    }

    if (canManage) {
      items.push({
        id: 'delete',
        type: 'regular',
        icon: IconTrash,
        title: t('common.delete'),
        color: 'var(--ds-text-danger)',
        hoverClass: 'hover-danger',
        onClick: deleteIteration
      });
    }

    return items;
  }
</script>

<PlanningEntityDetail
  loading={detail.loading}
  error={detail.error}
  progress={detail.progress}
  backLabel={t('iterations.backToIterations')}
  onBack={goBack}
  dropdownIcon={IconDots}
  dropdownItems={buildDropdownItems()}
  onRetry={() => detail.loadProgress()}
  summary={{
    ariaLabel: 'Iteration progress',
    completeLabel: t('iterations.complete'),
    noItemsLabel: t('iterations.noItems'),
    summaryLabel: t('iterations.summary'),
    totalLabel: t('iterations.totalItems'),
    completedLabel: t('iterations.completed'),
    remainingLabel: t('iterations.remaining'),
    statusLabel: t('iterations.byStatusCategory'),
    noStatusDataLabel: t('iterations.noStatusData'),
  }}
  items={{
    title: t('iterations.workItems'),
    emptyIcon: IconTarget,
    emptyTitle: t('iterations.noItemsAssigned'),
    emptyDescription: t('iterations.assignItemsHint'),
  }}
  expandedCategories={detail.expandedCategories}
  ontoggle={detail.toggleCategory}
>
  {#snippet header()}
    <div class="flex items-start justify-between mb-4">
      <div class="flex items-center gap-3">
        <div
          class="w-12 h-12 rounded-full flex items-center justify-center"
          style="background-color: {detail.progress.type_color ? detail.progress.type_color + '20' : 'rgba(20,184,166,0.12)'};"
        >
          <IconTarget class="w-6 h-6" style="color: {detail.progress.type_color || '#14b8a6'};" />
        </div>
        <div>
          <div class="flex items-center gap-2">
            <h1 class="text-2xl font-semibold" style="color: var(--ds-text);">{detail.progress.iteration_name}</h1>
            {#if detail.entity?.is_global}
              <div class="flex items-center gap-1 px-2 py-0.5 rounded text-xs" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                <IconWorld class="w-3 h-3" />
                {t('iterations.global')}
              </div>
            {:else if detail.entity?.workspace_name}
              <div class="flex items-center gap-1 px-2 py-0.5 rounded text-xs" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                <IconBuilding class="w-3 h-3" />
                {detail.entity.workspace_name}
              </div>
            {/if}
          </div>
          {#if detail.progress.description}
            <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">{detail.progress.description}</p>
          {/if}
        </div>
      </div>
      {#if detail.progress.status}
        {@const statusInfo = detail.getStatusInfo(detail.progress.status)}
        <Lozenge color={statusInfo.lozengeColor} text={statusInfo.label} />
      {/if}
    </div>

    <div class="flex items-center gap-4 text-sm" style="color: var(--ds-text-subtle);">
      <div class="flex items-center gap-2">
        <IconCalendar class="w-4 h-4" />
        <span>{formatDateShort(detail.progress.start_date)} - {formatDateShort(detail.progress.end_date)}</span>
      </div>
      {#if daysInfo}
        <span>|</span>
        <span class={daysInfo.overdue ? 'font-medium' : ''} style="color: var({daysInfo.overdue ? '--ds-text-danger' : '--ds-text-info'})">
          {daysInfo.text}
        </span>
      {/if}
    </div>
  {/snippet}

  {#snippet afterSummary()}
    <IterationBurndown data={burndownData} />
  {/snippet}
</PlanningEntityDetail>

<!-- Edit Modal -->
<Modal
  isOpen={showEditModal}
  onclose={() => showEditModal = false}
  onSubmit={saveIteration}
  submitDisabled={!formData.name.trim() || !formData.start_date || !formData.end_date}
  maxWidth="max-w-2xl"
>
  {#snippet children(submitHint)}
  <ModalHeader title={t('iterations.editIteration')} showCloseButton={false} />

  <div class="px-6 py-4">
    <form onsubmit={(e) => { e.preventDefault(); saveIteration(); }}>
      <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
        <div>
          <TextField
            label={t('iterations.iterationName')}
            id="iteration-name"
            required
            type="text"
            placeholder={t('iterations.iterationNamePlaceholder')}
            bind:value={formData.name}
          />
        </div>

        <div>
          <Label for="iteration-status" class="mb-2">{t('common.status')}</Label>
          <BasePicker
            bind:value={formData.status}
            items={statusOptions}
            placeholder={t('iterations.selectStatus')}
            getValue={(item) => item.value}
            getLabel={(item) => item.label}
          />
        </div>

        <div>
          <TextField
            label={t('iterations.startDate')}
            id="iteration-start-date"
            required
            type="date"
            bind:value={formData.start_date}
          />
        </div>

        <div>
          <TextField
            label={t('iterations.endDate')}
            id="iteration-end-date"
            required
            type="date"
            bind:value={formData.end_date}
          />
        </div>

        <div class="md:col-span-2">
          <TextareaField
            label={t('common.description')}
            id="iteration-description"
            rows={3}
            placeholder={t('iterations.descriptionPlaceholder')}
            bind:value={formData.description}
          />
        </div>
      </div>
    </form>
  </div>

  <DialogFooter
    onCancel={() => showEditModal = false}
    onConfirm={saveIteration}
    confirmLabel={t('iterations.updateIteration')}
    disabled={!formData.name.trim() || !formData.start_date || !formData.end_date}
    showKeyboardHint={true}
    confirmKeyboardHint={submitHint}
  />
  {/snippet}
</Modal>

<CompleteIterationDialog
  bind:show={showCompleteDialog}
  iteration={detail.entity ? { ...detail.entity, _totalItems: detail.progress?.total_items || 0 } : null}
  incompleteItems={Array.from({ length: Math.max(0, (detail.progress?.total_items || 0) - (detail.progress?.completed_items || 0)) })}
  targetIterations={completionTargets}
  onconfirm={completeIteration}
/>
