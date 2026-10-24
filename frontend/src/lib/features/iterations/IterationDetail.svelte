<script>
  import { onMount } from 'svelte';
  import StateDisplay from '../../components/StateDisplay.svelte';
  import BackButton from '../../components/BackButton.svelte';
  import IconCalendar from '@tabler/icons-svelte-runes/icons/calendar';
  import IconTarget from '@tabler/icons-svelte-runes/icons/target';
  import IconEdit from '@tabler/icons-svelte-runes/icons/edit';
  import IconTrash from '@tabler/icons-svelte-runes/icons/trash';
  import IconDots from '@tabler/icons-svelte-runes/icons/dots';
  import IconWorld from '@tabler/icons-svelte-runes/icons/world';
  import IconBuilding from '@tabler/icons-svelte-runes/icons/building';
  import IconSparkles from '@tabler/icons-svelte-runes/icons/sparkles';
  import IconCircleCheck from '@tabler/icons-svelte-runes/icons/circle-check';
  import IterationBurndown from './IterationBurndown.svelte';
  import { iterations } from '../../api/milestones.js';
  import { navigate } from '../../router.js';
  import Button from '../../components/Button.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
  import Label from '../../components/Label.svelte';
  import { formatDateShort, daysUntil } from '../../utils/dateFormatter.js';
  import ProgressSummary from '../../components/ProgressSummary.svelte';
  import ItemsByStatusCategory from '../../components/ItemsByStatusCategory.svelte';
  import BasePicker from '../../pickers/BasePicker.svelte';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';
  import CompleteIterationDialog from '../../dialogs/CompleteIterationDialog.svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast, successToast } from '../../stores/toasts.svelte.js';
  import { confirm } from '../../composables/useConfirm.js';
  import { aiStore } from '../../stores/aiStore.svelte.js';
  import { permissionStore, isSystemAdmin } from '../../stores/permissions.svelte.js';
  import { workspacePermissions } from '../../stores/workspacePermissions.svelte.js';
	import TextField from '../../components/TextField.svelte';
	import TextareaField from '../../components/TextareaField.svelte';

  let { iterationId, workspaceId = null } = $props();

  let loading = $state(true);
  let error = $state(null);
  let progress = $state(null);
  let iteration = $state(null);
  let burndownData = $state(null);
  let expandedCategories = $state({});
  let showEditModal = $state(false);
  let showCompleteDialog = $state(false);
  let completionTargets = $state([]);
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

  const canManage = $derived.by(() => {
    if (!iteration) return false;
    if ($isSystemAdmin) return true;
    if (iteration.is_global) {
      return $permissionStore.userPermissionKeys?.has('iteration.manage');
    } else {
      const wsId = iteration.workspace_id || workspaceId;
      if (!wsId) return false;
      return workspacePermissions.canAdminWorkspace(wsId) || 
             workspacePermissions.hasPermission(wsId, 'item.edit');
    }
  });

  let statusOptions = $derived([
    { value: 'planned', label: t('iterations.status.planned'), lozengeColor: 'grey' },
    { value: 'active', label: t('iterations.status.active'), lozengeColor: 'blue' },
    ...(iteration?.status === 'completed'
      ? [{ value: 'completed', label: t('iterations.status.completed'), lozengeColor: 'green' }]
      : []),
    { value: 'cancelled', label: t('iterations.status.cancelled'), lozengeColor: 'red' }
  ]);


  onMount(async () => {
    await loadProgress();
  });

  async function loadProgress() {
    loading = true;
    error = null;
    try {
      const [progressData, iterationData, burndownResult] = await Promise.all([
        iterations.getProgress(iterationId),
        iterations.get(iterationId),
        iterations.getBurndown(iterationId).catch(() => null)
      ]);
      progress = progressData;
      iteration = iterationData;
      burndownData = burndownResult;
      // Expand all categories by default
      if (progress?.status_breakdown) {
        progress.status_breakdown.forEach(cat => {
          expandedCategories[cat.category_name] = true;
        });
      }
    } catch (err) {
      console.error('Failed to load iteration progress:', err);
      error = err.message || t('dialogs.alerts.failedToLoad', { error: 'iteration progress' });
    } finally {
      loading = false;
    }
  }

  function goBack() {
    if (workspaceId) {
      navigate(`/workspaces/${workspaceId}/iterations`);
    } else {
      navigate('/iterations');
    }
  }

  function getStatusInfo(status) {
    return statusOptions.find(s => s.value === status) || statusOptions[0];
  }

  function toggleCategory(categoryName) {
    expandedCategories[categoryName] = !expandedCategories[categoryName];
  }

  function startEdit() {
    if (iteration) {
      formData = {
        name: iteration.name,
        description: iteration.description || '',
        start_date: iteration.start_date || '',
        end_date: iteration.end_date || '',
        status: iteration.status,
        type_id: iteration.type_id,
        is_global: iteration.is_global !== false,
        workspace_id: iteration.workspace_id || null
      };
      showEditModal = true;
    }
  }

  async function saveIteration() {
    try {
      await iterations.update(iterationId, formData);
      showEditModal = false;
      await loadProgress();
    } catch (err) {
      console.error('Failed to update iteration:', err);
      errorToast(t('dialogs.alerts.failedToUpdate', { error: err.message || err }));
    }
  }

  async function deleteIteration() {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('iterations.confirmDelete', { name: progress?.iteration_name }),
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
      const filters = iteration?.is_global
        ? { include_global: true }
        : { workspace_id: iteration?.workspace_id || workspaceId, include_global: true };
      const candidates = await iterations.getAll(filters);
      completionTargets = (candidates || []).filter(candidate => {
        if (candidate.id === iteration.id || !['planned', 'active'].includes(candidate.status)) return false;
        if (iteration.is_global) return candidate.is_global;
        return candidate.is_global || candidate.workspace_id === iteration.workspace_id;
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
      successToast(t('iterations.iterationCompleted', { name: iteration.name }));
      await loadProgress();
    } catch (err) {
      errorToast(t('dialogs.alerts.failedToUpdate', { error: err.message || err }));
    }
  }

  const daysInfo = $derived(progress?.end_date ? daysUntil(progress.end_date, {
    overdue: (n) => t('iterations.daysOverdue', { count: n }),
    today: t('iterations.endsToday'),
    oneDay: t('iterations.oneDayRemaining'),
    remaining: (n) => t('iterations.daysRemaining', { count: n }),
  }) : null);

  function buildDropdownItems() {
    const items = [];

    if (canManage) {
      if (iteration.status === 'planned' || iteration.status === 'active') {
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

    if (aiStore.available && progress?.total_items > 0) {
      items.push({
        id: 'dependencies',
        type: 'regular',
        icon: IconSparkles,
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

<div class="flex min-h-screen" style="background-color: var(--ds-surface);">
  <div class="flex-1 max-w-5xl mx-auto p-6">
    <!-- Header -->
    <div class="flex items-center justify-between mb-6">
      <BackButton onclick={goBack} label={t('iterations.backToIterations')} />

      {#if progress}
        <DropdownMenu
          triggerIcon={IconDots}
          triggerClass="w-8 h-8 flex items-center justify-center rounded-md transition-colors"
          triggerStyle="background-color: var(--ds-surface); color: var(--ds-text-subtle);"
          items={buildDropdownItems()}
          maxWidth="max-w-48"
          showChevron={false}
          iconOnly={true}
        />
      {/if}
    </div>

    {#if loading}
      <StateDisplay type="loading" />
    {:else if error}
      <div class="text-center py-20">
        <p class="text-ds-text-danger">{error}</p>
        <Button onclick={loadProgress} class="mt-4">{t('common.retry')}</Button>
      </div>
    {:else if progress}
      <!-- Iteration Header Card -->
      <div class="rounded-xl border p-6 mb-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <div class="flex items-start justify-between mb-4">
          <div class="flex items-center gap-3">
            <div
              class="w-12 h-12 rounded-full flex items-center justify-center"
              style="background-color: {progress.type_color ? progress.type_color + '20' : 'rgba(20,184,166,0.12)'};"
            >
              <IconTarget class="w-6 h-6" style="color: {progress.type_color || '#14b8a6'};" />
            </div>
            <div>
              <div class="flex items-center gap-2">
                <h1 class="text-2xl font-semibold" style="color: var(--ds-text);">{progress.iteration_name}</h1>
                {#if iteration?.is_global}
                  <div class="flex items-center gap-1 px-2 py-0.5 rounded text-xs" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                    <IconWorld class="w-3 h-3" />
                    {t('iterations.global')}
                  </div>
                {:else if iteration?.workspace_name}
                  <div class="flex items-center gap-1 px-2 py-0.5 rounded text-xs" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                    <IconBuilding class="w-3 h-3" />
                    {iteration.workspace_name}
                  </div>
                {/if}
              </div>
              {#if progress.description}
                <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">{progress.description}</p>
              {/if}
            </div>
          </div>
          {#if progress.status}
            {@const statusInfo = getStatusInfo(progress.status)}
            <Lozenge color={statusInfo.lozengeColor} text={statusInfo.label} />
          {/if}
        </div>

        <div class="flex items-center gap-4 text-sm" style="color: var(--ds-text-subtle);">
          <div class="flex items-center gap-2">
            <IconCalendar class="w-4 h-4" />
            <span>{formatDateShort(progress.start_date)} - {formatDateShort(progress.end_date)}</span>
          </div>
          {#if daysInfo}
            <span>|</span>
            <span class={daysInfo.overdue ? 'font-medium' : ''} style="color: var({daysInfo.overdue ? '--ds-text-danger' : '--ds-text-info'})">
              {daysInfo.text}
            </span>
          {/if}
        </div>
      </div>

      <ProgressSummary
        {progress}
        ariaLabel="Iteration progress"
        completeLabel={t('iterations.complete')}
        noItemsLabel={t('iterations.noItems')}
        summaryLabel={t('iterations.summary')}
        totalLabel={t('iterations.totalItems')}
        completedLabel={t('iterations.completed')}
        remainingLabel={t('iterations.remaining')}
        statusLabel={t('iterations.byStatusCategory')}
        noStatusDataLabel={t('iterations.noStatusData')}
      />

      <IterationBurndown data={burndownData} />

      <!-- Items Grouped by Category -->
      <ItemsByStatusCategory
        statusBreakdown={progress.status_breakdown}
        itemsByCategory={progress.items_by_category}
        {expandedCategories}
        title={t('iterations.workItems')}
        emptyIcon={IconTarget}
        emptyTitle={t('iterations.noItemsAssigned')}
        emptyDescription={t('iterations.assignItemsHint')}
        ontoggle={toggleCategory}
      />
    {/if}
  </div>
</div>

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
  iteration={iteration ? { ...iteration, _totalItems: progress?.total_items || 0 } : null}
  incompleteItems={Array.from({ length: Math.max(0, (progress?.total_items || 0) - (progress?.completed_items || 0)) })}
  targetIterations={completionTargets}
  onconfirm={completeIteration}
/>
