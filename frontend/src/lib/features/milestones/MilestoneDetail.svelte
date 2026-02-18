<script>
  import { onMount } from 'svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast } from '../../stores/toasts.svelte.js';
  import { ArrowLeft, Calendar, Flag, Edit, Trash2, ChevronDown, ChevronRight, MoreHorizontal, Tag } from 'lucide-svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import { api } from '../../api.js';
  import { navigate } from '../../router.js';
  import Button from '../../components/Button.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import Label from '../../components/Label.svelte';
  import { milestonesStore } from '../../stores/milestones.js';
  import { formatDateShort } from '../../utils/dateFormatter.js';
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
  let formData = $state({
    name: '',
    description: '',
    target_date: '',
    status: 'planning',
    category_id: null
  });

  let statusOptions = $derived([
    { value: 'planning', label: t('milestones.status.planning'), lozengeColor: 'grey' },
    { value: 'in-progress', label: t('milestones.status.inProgress'), lozengeColor: 'blue' },
    { value: 'completed', label: t('milestones.status.completed'), lozengeColor: 'green' },
    { value: 'cancelled', label: t('milestones.status.cancelled'), lozengeColor: 'red' }
  ]);

  const radius = 48;
  const circumference = 2 * Math.PI * radius;
  const fallbackColors = ['#22c55e', '#3b82f6', '#d1d5db', '#f97316', '#ec4899', '#8b5cf6'];

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

  function formatPercent(value) {
    if (typeof value === 'number' && Number.isFinite(value)) {
      return Math.min(100, Math.max(0, Math.round(value)));
    }
    return 0;
  }

  function getDaysUntil(targetDate) {
    if (!targetDate) return null;
    const today = new Date();
    const target = new Date(targetDate);
    const diffTime = target - today;
    const diffDays = Math.ceil(diffTime / (1000 * 60 * 60 * 24));

    if (diffDays < 0) return { text: t('milestones.daysOverdue', { count: Math.abs(diffDays) }), overdue: true };
    if (diffDays === 0) return { text: t('milestones.dueToday'), overdue: false };
    if (diffDays === 1) return { text: t('milestones.oneDayRemaining'), overdue: false };
    return { text: t('milestones.daysRemaining', { count: diffDays }), overdue: false };
  }

  function buildSegments(breakdown, totalItems) {
    if (!breakdown || !totalItems || totalItems <= 0) return [];
    let offset = 0;
    return breakdown
      .filter(segment => segment.item_count > 0)
      .map((segment, index) => {
        const fraction = segment.item_count / totalItems;
        const arcLength = Math.max(fraction * circumference, 0);
        const dasharray = `${arcLength} ${circumference}`;
        const segmentData = {
          ...segment,
          dasharray,
          offset,
          color: segment.category_color || fallbackColors[index % fallbackColors.length]
        };
        offset -= arcLength;
        return segmentData;
      });
  }

  function toggleCategory(categoryName) {
    expandedCategories[categoryName] = !expandedCategories[categoryName];
  }

  function navigateToItem(item) {
    navigate(`/workspaces/${item.workspace_id}/items/${item.id}`);
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
    if (confirm(t('milestones.confirmDelete', { name: progress?.milestone_name }))) {
      try {
        await milestonesStore.delete(milestoneId);
        goBack();
      } catch (err) {
        console.error('Failed to delete milestone:', err);
        errorToast(err.message || String(err), t('errors.failedToDelete'));
      }
    }
  }

  const segments = $derived(progress ? buildSegments(progress.status_breakdown, progress.total_items) : []);
  const daysInfo = $derived(progress?.target_date ? getDaysUntil(progress.target_date) : null);

  function buildDropdownItems() {
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
        color: '#dc2626',
        hoverClass: 'hover:bg-red-50',
        onClick: deleteMilestone
      }
    ];
  }

  async function handleReleased(event) {
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
          <div class="flex items-center gap-3">
            <div
              class="w-12 h-12 rounded-full flex items-center justify-center"
              style="background-color: {progress.category_color ? progress.category_color + '20' : 'rgba(37,99,235,0.12)'};"
            >
              <Flag class="w-6 h-6" style="color: {progress.category_color || '#2563eb'};" />
            </div>
            <div>
              <h1 class="text-2xl font-semibold" style="color: var(--ds-text);">{progress.milestone_name}</h1>
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

        {#if progress.target_date}
          <div class="flex items-center gap-2 text-sm" style="color: var(--ds-text-subtle);">
            <Calendar class="w-4 h-4" />
            <span>Target: {formatDateShort(progress.target_date)}</span>
            {#if daysInfo}
              <span class="mx-2">|</span>
              <span class={daysInfo.overdue ? 'text-red-500 font-medium' : 'text-blue-500'}>
                {daysInfo.text}
              </span>
            {/if}
          </div>
        {/if}
        {#if milestone?.latest_release?.scm_release_url}
          <div class="flex items-center gap-2 text-sm mt-2">
            <Tag class="w-4 h-4" style="color: var(--ds-text-subtle);" />
            <a
              href={milestone.latest_release.scm_release_url}
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

      <!-- Progress Section -->
      <div class="grid grid-cols-1 md:grid-cols-3 gap-6 mb-6">
        <!-- Circular Progress Chart -->
        <div class="rounded-xl border p-6 flex flex-col items-center" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
          <div class="relative">
            {#if progress.total_items > 0}
              <svg viewBox="0 0 140 140" class="w-36 h-36" role="img" aria-label="Milestone progress">
                <circle
                  cx="70"
                  cy="70"
                  r={radius}
                  fill="transparent"
                  stroke="var(--ds-border)"
                  stroke-width="16"
                />
                {#each segments as segment (segment.category_name)}
                  <circle
                    cx="70"
                    cy="70"
                    r={radius}
                    fill="transparent"
                    stroke={segment.color}
                    stroke-width="16"
                    stroke-linecap="butt"
                    stroke-dasharray={segment.dasharray}
                    stroke-dashoffset={segment.offset}
                    transform="rotate(-90 70 70)"
                  />
                {/each}
                <text class="text-2xl font-bold" x="70" y="68" text-anchor="middle" fill="var(--ds-text)">
                  {formatPercent(progress.percent_complete)}%
                </text>
                <text class="text-xs uppercase" x="70" y="86" text-anchor="middle" fill="var(--ds-text-subtle)">
                  {t('milestones.complete')}
                </text>
              </svg>
            {:else}
              <div class="w-36 h-36 rounded-full border-2 border-dashed flex items-center justify-center" style="border-color: var(--ds-border);">
                <span class="text-sm" style="color: var(--ds-text-subtlest);">{t('milestones.noItems')}</span>
              </div>
            {/if}
          </div>
        </div>

        <!-- Summary Stats -->
        <div class="rounded-xl border p-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
          <h3 class="text-sm font-medium mb-4" style="color: var(--ds-text-subtle);">{t('common.summary')}</h3>
          <div class="space-y-3">
            <div class="flex justify-between items-center">
              <span style="color: var(--ds-text-subtle);">{t('common.total')}</span>
              <span class="font-semibold" style="color: var(--ds-text);">{progress.total_items}</span>
            </div>
            <div class="flex justify-between items-center">
              <span style="color: var(--ds-text-subtle);">{t('common.done')}</span>
              <span class="font-semibold text-green-600">{progress.completed_items}</span>
            </div>
            <div class="flex justify-between items-center">
              <span style="color: var(--ds-text-subtle);">{t('time.remaining')}</span>
              <span class="font-semibold" style="color: var(--ds-text);">{progress.total_items - progress.completed_items}</span>
            </div>
          </div>
        </div>

        <!-- Status Breakdown Legend -->
        <div class="rounded-xl border p-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
          <h3 class="text-sm font-medium mb-4" style="color: var(--ds-text-subtle);">{t('milestones.byStatusCategory')}</h3>
          <div class="space-y-2">
            {#if progress.status_breakdown && progress.status_breakdown.length > 0}
              {#each progress.status_breakdown as breakdown}
                <div class="flex items-center justify-between">
                  <div class="flex items-center gap-2">
                    <div
                      class="w-3 h-3 rounded-full"
                      style="background-color: {breakdown.category_color || '#9ca3af'};"
                    ></div>
                    <span class="text-sm" style="color: var(--ds-text);">{breakdown.category_name}</span>
                  </div>
                  <span class="text-sm font-medium" style="color: var(--ds-text-subtle);">{breakdown.item_count}</span>
                </div>
              {/each}
            {:else}
              <p class="text-sm" style="color: var(--ds-text-subtlest);">{t('milestones.noStatusData')}</p>
            {/if}
          </div>
        </div>
      </div>

      <!-- Items Grouped by Category -->
      <div class="space-y-4">
        <h2 class="text-lg font-semibold" style="color: var(--ds-text);">{t('milestones.workItems')}</h2>

        {#if progress.status_breakdown && progress.status_breakdown.length > 0}
          {#each progress.status_breakdown as category}
            {@const items = progress.items_by_category[category.category_name] || []}
            <div class="rounded-xl border overflow-hidden" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
              <button
                onclick={() => toggleCategory(category.category_name)}
                class="w-full px-4 py-3 flex items-center justify-between hover:bg-opacity-50 transition-colors"
                style="background-color: var(--ds-background-neutral);"
              >
                <div class="flex items-center gap-3">
                  {#if expandedCategories[category.category_name]}
                    <ChevronDown class="w-4 h-4" style="color: var(--ds-text-subtle);" />
                  {:else}
                    <ChevronRight class="w-4 h-4" style="color: var(--ds-text-subtle);" />
                  {/if}
                  <div
                    class="w-3 h-3 rounded-full"
                    style="background-color: {category.category_color || '#9ca3af'};"
                  ></div>
                  <span class="font-medium" style="color: var(--ds-text);">{category.category_name}</span>
                  <span class="text-sm" style="color: var(--ds-text-subtle);">({category.item_count} item{category.item_count !== 1 ? 's' : ''})</span>
                </div>
              </button>

              {#if expandedCategories[category.category_name] && items.length > 0}
                <div class="divide-y" style="border-color: var(--ds-border);">
                  {#each items as item}
                    <button
                      onclick={() => navigateToItem(item)}
                      class="w-full px-4 py-3 flex items-center justify-between hover:bg-opacity-50 transition-colors text-left"
                      style="background-color: var(--ds-surface-raised);"
                    >
                      <div class="flex items-center gap-3 min-w-0">
                        <span class="text-sm font-mono shrink-0" style="color: var(--ds-text-subtle);">
                          {item.workspace_key}-{item.item_number}
                        </span>
                        <span class="truncate" style="color: var(--ds-text);">{item.title}</span>
                      </div>
                      <div class="flex items-center gap-3 shrink-0">
                        {#if item.priority_name}
                          <span
                            class="text-xs px-2 py-0.5 rounded"
                            style="background-color: {item.priority_color ? item.priority_color + '20' : 'var(--ds-background-neutral)'}; color: {item.priority_color || 'var(--ds-text-subtle)'};"
                          >
                            {item.priority_name}
                          </span>
                        {/if}
                        {#if item.assignee_name}
                          <span class="text-sm" style="color: var(--ds-text-subtle);">{item.assignee_name}</span>
                        {/if}
                      </div>
                    </button>
                  {/each}
                </div>
              {/if}
            </div>
          {/each}
        {:else}
          <div class="rounded-xl border p-8" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
            <EmptyState
              icon={Flag}
              title={t('milestones.noItemsAssigned')}
              description={t('milestones.assignItemsHint')}
            />
          </div>
        {/if}
      </div>
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
      on:released={handleReleased}
      on:close={() => showReleaseModal = false}
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
  let:submitHint
>
  <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
    <h3 class="text-lg font-semibold" style="color: var(--ds-text);">{t('common.edit')}</h3>
  </div>

  <div class="px-6 py-4">
    <form onsubmit={(e) => { e.preventDefault(); saveMilestone(); }}>
      <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
        <div>
          <Label for="milestone-name" required class="mb-2">{t('milestones.milestoneName')}</Label>
          <input
            id="milestone-name"
            type="text"
            bind:value={formData.name}
            class="w-full px-4 py-3 rounded border transition-all duration-200 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-opacity-50"
            style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
            placeholder={t('milestones.milestoneNamePlaceholder')}
            required
          />
        </div>

        <div>
          <Label for="milestone-target-date" class="mb-2">{t('milestones.targetDate')}</Label>
          <input
            id="milestone-target-date"
            type="date"
            bind:value={formData.target_date}
            class="w-full px-4 py-3 rounded border transition-all duration-200 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-opacity-50"
            style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
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
</Modal>

