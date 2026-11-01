<script>
  import { onMount } from 'svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { confirm } from '../../composables/useConfirm.js';
  import { errorToast } from '../../stores/toasts.svelte.js';
  import Calendar from '@tabler/icons-svelte-runes/icons/calendar';
  import Flag from '@tabler/icons-svelte-runes/icons/flag';
  import Edit from '@tabler/icons-svelte-runes/icons/edit';
  import Trash2 from '@tabler/icons-svelte-runes/icons/trash';
  import MoreHorizontal from '@tabler/icons-svelte-runes/icons/dots';
  import Tag from '@tabler/icons-svelte-runes/icons/tag';
  import ExternalLink from '@tabler/icons-svelte-runes/icons/external-link';
  import { milestones } from '../../api/milestones.js';
  import { navigate } from '../../router.js';
  import Lozenge from '../../components/Lozenge.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import Label from '../../components/Label.svelte';
  import ProgressSummary from '../../components/ProgressSummary.svelte';
  import ItemsByStatusCategory from '../../components/ItemsByStatusCategory.svelte';
  import TabStrip from '../../components/TabStrip.svelte';
  import { milestonesStore } from '../../stores/milestones.js';
  import { formatDateShort, daysUntil } from '../../utils/dateFormatter.js';
  import { safeHref } from '../../utils/sanitize';
  import { permissionStore, isSystemAdmin } from '../../stores/permissions.svelte.js';
  import { workspacePermissions } from '../../stores/workspacePermissions.svelte.js';
  import BasePicker from '../../pickers/BasePicker.svelte';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';
  import MilestoneReleaseModal from './MilestoneReleaseModal.svelte';
  import { useProgressDetail } from '../../composables/useProgressDetail.svelte.js';
  import PlanningEntityDetail from '../shared/PlanningEntityDetail.svelte';
  import TextField from '../../components/TextField.svelte';
  import MilkdownEditor from '../../editors/LazyMilkdownEditor.svelte';
  import MilestoneComments from './MilestoneComments.svelte';
  import MilestonePages from './MilestonePages.svelte';
  import MilestoneActivity from './MilestoneActivity.svelte';
  import { canManageMilestone, milestoneWorkspaceId } from './milestoneScope.js';
  import { milestoneEditForm } from './milestoneEditForm.js';
  import { categoriesStore } from '../../stores/categories.js';

  let { milestoneId, workspaceId = null } = $props();

  let showEditModal = $state(false);
  // Tabs: Overview keeps the progress and comments (pages sit in
  // the header card above the tabs);
  // Activity shows the shared milestone feed.
  let activeTab = $state('overview');
  // Bumped after each reload so an open Activity tab shows the new entries.
  let activityReloadToken = $state(0);
  const detailTabs = $derived([
    { id: 'overview', label: t('milestones.activity.tabOverview'), testid: 'milestone-tab-overview' },
    { id: 'activity', label: t('milestones.activity.tabActivity'), testid: 'milestone-tab-activity' }
  ]);
  let showReleaseModal = $state(false);
  /** @type {{ name: string, description: string, target_date: string, status: string, category_id: any, is_global?: boolean, workspace_id?: number | null }} */
  let formData = $state({
    name: '',
    description: '',
    target_date: '',
    status: 'planning',
    category_id: null
  });

  const detail = useProgressDetail({
    errorLabel: 'milestone progress',
    getStatusOptions: () => statusOptions,
    load: async () => {
      const [progress, entity] = await Promise.all([
        milestones.getProgress(milestoneId),
        milestones.get(milestoneId)
      ]);
      return { progress, entity };
    }
  });

  let statusOptions = $derived([
    { value: 'planning', label: t('milestones.status.planning'), lozengeColor: 'grey' },
    { value: 'in-progress', label: t('milestones.status.inProgress'), lozengeColor: 'blue' },
    { value: 'completed', label: t('milestones.status.completed'), lozengeColor: 'green' },
    { value: 'cancelled', label: t('milestones.status.cancelled'), lozengeColor: 'red' }
  ]);

  // Scope and edit rights come from the milestone record: the progress
  // response has no is_global or workspace_id.
  const milestoneWsId = $derived(milestoneWorkspaceId(detail.entity));

  const canManage = $derived(
    canManageMilestone(detail.entity, {
      isSystemAdmin: $isSystemAdmin,
      hasGlobalPermission: (key) => $permissionStore.userPermissionKeys?.has(key) ?? false,
      hasWorkspacePermission: (wsId, key) => workspacePermissions.hasPermission(wsId, key)
    })
  );

  onMount(() => {
    // Options for the edit dialog's category picker.
    categoriesStore.init();
    void reload();
  });

  // Reload the milestone and nudge an open Activity tab to fetch the new
  // entries.
  async function reload() {
    await detail.loadProgress();
    activityReloadToken += 1;
  }

  function goBack() {
    if (workspaceId) {
      navigate(`/workspaces/${workspaceId}/milestones`);
    } else {
      navigate('/milestones');
    }
  }

  function startEdit() {
    if (detail.progress) {
      formData = milestoneEditForm({ progress: detail.progress, milestone: detail.entity, workspaceId });
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
      await reload();
    } catch (err) {
      console.error('Failed to update milestone:', err);
      errorToast(err.message || String(err), t('errors.failedToUpdate'));
    }
  }

  async function deleteMilestone() {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('milestones.confirmDelete', { name: detail.progress?.milestone_name }),
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

  const daysInfo = $derived(detail.progress?.target_date ? daysUntil(detail.progress.target_date, {
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
    await reload();
  }
</script>

<PlanningEntityDetail
  loading={detail.loading}
  error={detail.error}
  progress={detail.progress}
  backLabel={t('common.back')}
  onBack={goBack}
  dropdownIcon={MoreHorizontal}
  dropdownItems={buildDropdownItems()}
  onRetry={reload}
  summary={{
    ariaLabel: 'Milestone progress',
    completeLabel: t('milestones.complete'),
    noItemsLabel: t('milestones.noItems'),
    summaryLabel: t('common.summary'),
    totalLabel: t('common.total'),
    completedLabel: t('common.done'),
    remainingLabel: t('time.remaining'),
    statusLabel: t('milestones.byStatusCategory'),
    noStatusDataLabel: t('milestones.noStatusData'),
  }}
  items={{
    title: t('milestones.workItems'),
    emptyIcon: Flag,
    emptyTitle: t('milestones.noItemsAssigned'),
    emptyDescription: t('milestones.assignItemsHint'),
  }}
  expandedCategories={detail.expandedCategories}
  ontoggle={detail.toggleCategory}
>
  {#snippet header()}
    <div class="flex items-start justify-between mb-4">
      <div class="min-w-0 flex-1">
        <h1 class="text-2xl font-semibold" style="color: var(--ds-text);">{detail.progress.milestone_name}</h1>
        {#if detail.progress.description}
          <div class="mt-1" data-testid="milestone-description-display">
            <MilkdownEditor content={detail.progress.description} readonly={true} showToolbar={false} />
          </div>
        {/if}
      </div>
      {#if detail.progress.status}
        {@const statusInfo = detail.getStatusInfo(detail.progress.status)}
        <Lozenge color={statusInfo.lozengeColor} text={statusInfo.label} />
      {/if}
    </div>

    {#if detail.progress.target_date}
      <div class="flex items-center gap-2 text-sm" style="color: var(--ds-text-subtle);">
        <Calendar class="w-4 h-4" />
        <span>Target: {formatDateShort(detail.progress.target_date)}</span>
        {#if daysInfo}
          <span class="mx-2">|</span>
          <span class={daysInfo.overdue ? 'font-medium' : ''} style="color: var({daysInfo.overdue ? '--ds-text-danger' : '--ds-text-info'})">
            {daysInfo.text}
          </span>
        {/if}
      </div>
    {/if}
    {#if detail.entity?.releases?.length > 0}
      <div class="flex flex-col gap-1.5 mt-2">
        {#each detail.entity.releases as release (release.id)}
          <div class="flex items-center gap-2 text-sm" data-testid={`milestone-release-${release.id}`}>
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
            {#if release.release_status && release.release_status !== 'released'}
              <Lozenge color={release.release_status === 'missing' ? 'red' : 'blue'} size="sm">
                {release.release_status.replaceAll('_', ' ')}
              </Lozenge>
            {/if}
            {#if release.scm_release_url || release.tag_url}
              <a
                href={safeHref(release.scm_release_url || release.tag_url)}
                target="_blank"
                rel="noopener noreferrer"
                class="hover:underline inline-flex items-center gap-1"
                style="color: var(--ds-link);"
              >
                <ExternalLink class="w-3 h-3" />
              </a>
            {/if}
            {#if release.assets?.length}
              <span class="text-xs" style="color: var(--ds-text-subtlest);">{release.assets.length} assets</span>
            {/if}
            {#if release.released_at}
              <span class="text-xs" style="color: var(--ds-text-subtlest);">{formatDateShort(release.released_at)}</span>
            {/if}
          </div>
        {/each}
      </div>
    {:else if detail.entity?.latest_release?.scm_release_url}
      <div class="flex items-center gap-2 text-sm mt-2">
        <Tag class="w-4 h-4" style="color: var(--ds-text-subtle);" />
        <a
          href={safeHref(detail.entity.latest_release.scm_release_url)}
          target="_blank"
          rel="noopener noreferrer"
          class="hover:underline"
          style="color: var(--ds-link);"
        >
          View release
        </a>
      </div>
    {/if}

    <!-- Pages list: workspace milestones only; global milestones have no
         workspace. Part of the header card, so it shows on both tabs. -->
    {#if milestoneWsId != null}
      <MilestonePages
        {milestoneId}
        workspaceId={milestoneWsId}
        canEdit={canManage}
      />
    {/if}
  {/snippet}

  {#snippet body(progress)}
    <TabStrip tabs={detailTabs} bind:activeTab ariaLabel={progress.milestone_name} class="mb-6" />

    <!-- Overview stays mounted while the Activity tab is open so a comment
         draft survives switching tabs. -->
    <div class:hidden={activeTab !== 'overview'} data-testid="milestone-overview">
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
        expandedCategories={detail.expandedCategories}
        title={t('milestones.workItems')}
        emptyIcon={Flag}
        emptyTitle={t('milestones.noItemsAssigned')}
        emptyDescription={t('milestones.assignItemsHint')}
        ontoggle={detail.toggleCategory}
      />

      <!-- Comments (global and local milestones; no notifications). Kept
           outside the loading branch so a reload after editing the
           milestone keeps the thread and any draft mounted. -->
      <MilestoneComments
        {milestoneId}
        workspaceId={milestoneWsId}
      />
    </div>

    {#if activeTab === 'activity'}
      <MilestoneActivity {milestoneId} reloadToken={activityReloadToken} />
    {/if}
  {/snippet}
</PlanningEntityDetail>

<!-- Release Modal -->
{#if showReleaseModal && detail.progress}
  <Modal
    isOpen={showReleaseModal}
    onclose={() => showReleaseModal = false}
    maxWidth="max-w-4xl"
    maxHeight="85vh"
  >
    <MilestoneReleaseModal
      milestone={detail.entity ?? { id: milestoneId, name: detail.progress.milestone_name, description: detail.progress.description }}
      {workspaceId}
      hasExistingRelease={detail.entity?.releases?.length > 0 || detail.entity?.latest_release != null}
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
    onCancel={() => showEditModal = false}
    onConfirm={saveMilestone}
    confirmLabel={t('common.update')}
    disabled={!formData.name.trim()}
    showKeyboardHint={true}
    confirmKeyboardHint={submitHint}
  />
  {/snippet}
</Modal>
