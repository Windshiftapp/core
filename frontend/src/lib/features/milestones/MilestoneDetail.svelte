<script>
  import { onMount } from 'svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { confirm } from '../../composables/useConfirm.js';
  import { errorToast } from '../../stores/toasts.svelte.js';
  import { IconArrowLeft as ArrowLeft, IconCalendar as Calendar, IconFlag as Flag, IconEdit as Edit, IconTrash as Trash2, IconDots as MoreHorizontal, IconTag as Tag, IconExternalLink as ExternalLink } from '@tabler/icons-svelte-runes';
  import { api } from '../../api.js';
  import { navigate } from '../../router.js';
  import Button from '../../components/Button.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import Label from '../../components/Label.svelte';
  import Input from '../../components/Input.svelte';
  import { milestonesStore } from '../../stores/milestones.js';
  import { formatDateShort, daysUntil } from '../../utils/dateFormatter.js';
  import { safeHref } from '../../utils/sanitize';
  import ProgressSummary from '../../components/ProgressSummary.svelte';
  import ItemsByStatusCategory from '../../components/ItemsByStatusCategory.svelte';
  import { permissionStore, isSystemAdmin } from '../../stores/permissions.svelte.js';
  import { workspacePermissions } from '../../stores/workspacePermissions.svelte.js';
  import BasePicker from '../../pickers/BasePicker.svelte';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';
  import MilestoneReleaseModal from './MilestoneReleaseModal.svelte';

  let { milestoneId, workspaceId = null } = $props();

  let loading = $state(true);
  let error = $state(null);
  let progress = $state(null);
  let milestone = $state(null); // full milestone record (includes latest_release)
  let expandedCategories = $state({});
  let showEditModal = $state(false);
  let showReleaseModal = $state(false);
  /** @type {{ name: string, description: string, target_date: string, status: string, category_id: any, is_global?: boolean, workspace_id?: number | null }} */
  let formData = $state({
    name: '',
    description: '',
    target_date: '',
    status: 'planning',
    category_id: null
  });

  const canManage = $derived.by(() => {
    if (!progress) return false;
    if ($isSystemAdmin) return true;
    if (progress.is_global) {
      return $permissionStore.userPermissionKeys?.has('milestone.create');
    } else {
      const wsId = progress.workspace_id || workspaceId;
      if (!wsId) return false;
      return workspacePermissions.canAdminWorkspace(wsId) || 
             workspacePermissions.hasPermission(wsId, 'item.edit');
    }
  });

  let statusOptions = $derived([
    { value: 'planning', label: t('milestones.status.planning'), lozengeColor: 'grey' },
    { value: 'in-progress', label: t('milestones.status.inProgress'), lozengeColor: 'blue' },
    { value: 'completed', label: t('milestones.status.completed'), lozengeColor: 'green' },
    { value: 'cancelled', label: t('milestones.status.cancelled'), lozengeColor: 'red' }
  ]);


  onMount(async () => {
    await loadProgress();
  });

  async function loadProgress() {
    loading = true;
    error = null;
    try {
      [progress, milestone] = await Promise.all([
        api.milestones.getProgress(milestoneId),
        api.milestones.get(milestoneId)
      ]);
      // Expand all categories by default
      if (progress?.status_breakdown) {
        progress.status_breakdown.forEach(cat => {
          expandedCategories[cat.category_name] = true;
        });
      }
    } catch (err) {
      console.error('Failed to load milestone progress:', err);
      error = err.message || t('dialogs.alerts.failedToLoad', { error: 'milestone progress' });
    } finally {
      loading = false;
    }
  }

  function goBack() {
    if (workspaceId) {
      navigate(`/workspaces/${workspaceId}/milestones`);
    } else {
      navigate('/milestones');
    }
  }

  function getStatusInfo(status) {
    return statusOptions.find(s => s.value === status) || statusOptions[0];
  }

  function toggleCategory(categoryName) {
    expandedCategories[categoryName] = !expandedCategories[categoryName];
  }

  function startEdit() {
    if (progress) {
      formData = {
        name: progress.milestone_name,
        description: progress.description || '',
        target_date: progress.target_date ? progress.target_date.split('T')[0] : '',
        status: progress.status,
        category_id: null, // We don't have this in progress response, but it's optional
        is_global: progress.is_global ?? !workspaceId,
        workspace_id: progress.workspace_id ?? (workspaceId ? parseInt(workspaceId, 10) : null)
      };
      showEditModal = true;
    }
  }

  async function saveMilestone() {
    try {
      // Convert empty strings to null for optional date fields
      const dataToSave = {
        ...formData,
        target_date: formData.target_date || null
      };
      await milestonesStore.update(milestoneId, dataToSave);
      showEditModal = false;
      await loadProgress();
    } catch (err) {
      console.error('Failed to update milestone:', err);
      errorToast(err.message || String(err), t('errors.failedToUpdate'));
    }
  }

  async function deleteMilestone() {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('milestones.confirmDelete', { name: progress?.milestone_name }),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger'
    });
    if (confirmed) {
      try {
        await milestonesStore.delete(milestoneId);
        goBack();
      } catch (err) {
        console.error('Failed to delete milestone:', err);
        errorToast(err.message || String(err), t('errors.failedToDelete'));
      }
    }
  }

  const daysInfo = $derived(progress?.target_date ? daysUntil(progress.target_date, {
    overdue: (n) => t('milestones.daysOverdue', { count: n }),
    today: t('milestones.dueToday'),
    oneDay: t('milestones.oneDayRemaining'),
    remaining: (n) => t('milestones.daysRemaining', { count: n }),
  }) : null);

  function buildDropdownItems() {
    if (!canManage) return [];

    return [
      {
        id: 'release',
        type: 'regular',
        icon: Tag,
        title: 'Release',
        hoverClass: 'hover-bg',
        onClick: () => { showReleaseModal = true; }
      },
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: t('common.edit'),
        hoverClass: 'hover-bg',
        onClick: startEdit
      },
      {
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: t('common.delete'),
        color: 'var(--ds-text-danger)',
        hoverClass: 'hover-danger',
        onClick: deleteMilestone
      }
    ];
  }

  async function handleReleased(updatedMilestone) {
    showReleaseModal = false;
    await loadProgress();
  }
</script>

<div class="flex min-h-screen" style="background-color: var(--ds-surface);">
  <div class="flex-1 max-w-5xl mx-auto p-6">
    <!-- Header -->
    <div class="flex items-center justify-between mb-6">
      <button
        onclick={goBack}
        class="flex items-center gap-2 text-sm font-medium hover:opacity-80 transition-opacity"
        style="color: var(--ds-text-subtle);"
      >
        <ArrowLeft class="w-4 h-4" />
        {t('common.back')}
      </button>

      {#if progress}
        <DropdownMenu
          triggerIcon={MoreHorizontal}
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
      <div class="flex items-center justify-center py-20">
        <div class="animate-spin rounded-full h-8 w-8 border-b-2" style="border-color: var(--ds-text-subtle);"></div>
      </div>
    {:else if error}
      <div class="text-center py-20">
        <p class="text-red-500">{error}</p>
        <Button onclick={loadProgress} class="mt-4">{t('common.retry')}</Button>
      </div>
    {:else if progress}
      <!-- Milestone Header Card -->
      <div class="rounded-xl border p-6 mb-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <div class="flex items-start justify-between mb-4">
          <div>
            <h1 class="text-2xl font-semibold" style="color: var(--ds-text);">{progress.milestone_name}</h1>
            {#if progress.description}
              <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">{progress.description}</p>
            {/if}
          </div>
          {#if progress.status}
            {@const statusInfo = getStatusInfo(progress.status)}
            <Lozenge color={statusInfo.lozengeColor} text={statusInfo.label} />
          {/if}
        </div>

        {#if progress.target_date}
          <div class="flex items-center gap-2 text-sm" style="color: var(--ds-text-subtle);">
            <Calendar class="w-4 h-4" />
            <span>Target: {formatDateShort(progress.target_date)}</span>
            {#if daysInfo}
              <span class="mx-2">|</span>
              <span class={daysInfo.overdue ? 'font-medium' : ''} style="color: var({daysInfo.overdue ? '--ds-text-danger' : '--ds-text-info'})">
                {daysInfo.text}
              </span>
            {/if}
          </div>
        {/if}
        {#if milestone?.releases?.length > 0}
          <div class="flex flex-col gap-1.5 mt-2">
            {#each milestone.releases as release}
              <div class="flex items-center gap-2 text-sm">
                <Tag class="w-4 h-4 shrink-0" style="color: var(--ds-text-subtle);" />
                <span class="font-mono text-xs" style="color: var(--ds-text);">{release.tag_name}</span>
                {#if release.name && release.name !== release.tag_name}
                  <span style="color: var(--ds-text-subtle);">—</span>
                  <span style="color: var(--ds-text-subtle);">{release.name}</span>
                {/if}
                {#if release.is_draft}
                  <Lozenge color="grey" size="sm">Draft</Lozenge>
                {/if}
                {#if release.is_prerelease}
                  <Lozenge color="yellow" size="sm">Pre-release</Lozenge>
                {/if}
                {#if release.scm_release_url}
                  <a
                    href={safeHref(release.scm_release_url)}
                    target="_blank"
                    rel="noopener noreferrer"
                    class="hover:underline inline-flex items-center gap-1"
                    style="color: var(--ds-link);"
                  >
                    <ExternalLink class="w-3 h-3" />
                  </a>
                {/if}
                <span class="text-xs" style="color: var(--ds-text-subtlest);">{formatDateShort(release.created_at)}</span>
              </div>
            {/each}
          </div>
        {:else if milestone?.latest_release?.scm_release_url}
          <div class="flex items-center gap-2 text-sm mt-2">
            <Tag class="w-4 h-4" style="color: var(--ds-text-subtle);" />
            <a
              href={safeHref(milestone.latest_release.scm_release_url)}
              target="_blank"
              rel="noopener noreferrer"
              class="hover:underline"
              style="color: var(--ds-link);"
            >
              View release
            </a>
          </div>
        {/if}
      </div>

      <ProgressSummary
        {progress}
        ariaLabel="Milestone progress"
        completeLabel={t('milestones.complete')}
        noItemsLabel={t('milestones.noItems')}
        summaryLabel={t('common.summary')}
        totalLabel={t('common.total')}
        completedLabel={t('common.done')}
        remainingLabel={t('time.remaining')}
        statusLabel={t('milestones.byStatusCategory')}
        noStatusDataLabel={t('milestones.noStatusData')}
      />

      <!-- Items Grouped by Category -->
      <ItemsByStatusCategory
        statusBreakdown={progress.status_breakdown}
        itemsByCategory={progress.items_by_category}
        {expandedCategories}
        title={t('milestones.workItems')}
        emptyIcon={Flag}
        emptyTitle={t('milestones.noItemsAssigned')}
        emptyDescription={t('milestones.assignItemsHint')}
        ontoggle={toggleCategory}
      />
    {/if}
  </div>
</div>

<!-- Release Modal -->
{#if showReleaseModal && progress}
  <Modal
    isOpen={showReleaseModal}
    onclose={() => showReleaseModal = false}
    maxWidth="max-w-4xl"
    maxHeight="85vh"
  >
    <MilestoneReleaseModal
      milestone={milestone ?? { id: milestoneId, name: progress.milestone_name, description: progress.description }}
      {workspaceId}
      hasExistingRelease={milestone?.releases?.length > 0 || milestone?.latest_release != null}
      onreleased={handleReleased}
      onclose={() => showReleaseModal = false}
    />
  </Modal>
{/if}

<!-- Edit Modal -->
<Modal
  isOpen={showEditModal}
  onclose={() => showEditModal = false}
  onSubmit={saveMilestone}
  submitDisabled={!formData.name.trim()}
  maxWidth="max-w-2xl"
>
  {#snippet children(submitHint)}
  <ModalHeader title={t('common.edit')} showCloseButton={false} />

  <div class="px-6 py-4">
    <form onsubmit={(e) => { e.preventDefault(); saveMilestone(); }}>
      <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
        <div>
          <Label for="milestone-name" required class="mb-2">{t('milestones.milestoneName')}</Label>
          <Input
            id="milestone-name"
            type="text"
            bind:value={formData.name}
            placeholder={t('milestones.milestoneNamePlaceholder')}
            required
          />
        </div>

        <div>
          <Label for="milestone-target-date" class="mb-2">{t('milestones.targetDate')}</Label>
          <Input
            id="milestone-target-date"
            type="date"
            bind:value={formData.target_date}
          />
        </div>

        <div>
          <Label for="milestone-status" class="mb-2">{t('common.status')}</Label>
          <BasePicker
            bind:value={formData.status}
            items={statusOptions}
            placeholder={t('milestones.selectStatus')}
            getValue={(item) => item.value}
            getLabel={(item) => item.label}
          />
        </div>

        <div class="md:col-span-2">
          <Label for="milestone-description" class="mb-2">{t('common.description')}</Label>
          <Textarea
            id="milestone-description"
            bind:value={formData.description}
            rows={3}
            placeholder={t('milestones.descriptionPlaceholder')}
          />
        </div>
      </div>
    </form>
  </div>

  <DialogFooter
    onCancel={() => showEditModal = false}
    onConfirm={saveMilestone}
    confirmLabel={t('common.update')}
    disabled={!formData.name.trim()}
    showKeyboardHint={true}
    confirmKeyboardHint={submitHint}
  />
  {/snippet}
</Modal>
