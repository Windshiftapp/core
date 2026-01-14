<script>
  import { onMount } from 'svelte';
  import { ArrowLeft, Calendar, Target, Edit, Trash2, ChevronDown, ChevronRight, MoreHorizontal, Globe, Building2 } from 'lucide-svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import { api } from '../../api.js';
  import { navigate } from '../../router.js';
  import Button from '../../components/Button.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
  import Label from '../../components/Label.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import { formatDateShort } from '../../utils/dateFormatter.js';
  import BasePicker from '../../pickers/BasePicker.svelte';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';

  let { iterationId } = $props();

  let loading = $state(true);
  let error = $state(null);
  let progress = $state(null);
  let iteration = $state(null);
  let expandedCategories = $state({});
  let showEditModal = $state(false);
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

  const statusOptions = [
    { value: 'planned', label: 'Planned', lozengeColor: 'grey' },
    { value: 'active', label: 'Active', lozengeColor: 'blue' },
    { value: 'completed', label: 'Completed', lozengeColor: 'green' },
    { value: 'cancelled', label: 'Cancelled', lozengeColor: 'red' }
  ];

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
      const [progressData, iterationData] = await Promise.all([
        api.iterations.getProgress(iterationId),
        api.iterations.get(iterationId)
      ]);
      progress = progressData;
      iteration = iterationData;
      // Expand all categories by default
      if (progress?.status_breakdown) {
        progress.status_breakdown.forEach(cat => {
          expandedCategories[cat.category_name] = true;
        });
      }
    } catch (err) {
      console.error('Failed to load iteration progress:', err);
      error = err.message || 'Failed to load iteration progress';
    } finally {
      loading = false;
    }
  }

  function goBack() {
    navigate('/iterations');
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

  function getDaysRemaining(endDate) {
    if (!endDate) return null;
    const today = new Date();
    const end = new Date(endDate);
    const diffTime = end - today;
    const diffDays = Math.ceil(diffTime / (1000 * 60 * 60 * 24));

    if (diffDays < 0) return { text: `${Math.abs(diffDays)} days overdue`, overdue: true };
    if (diffDays === 0) return { text: 'Ends today', overdue: false };
    if (diffDays === 1) return { text: '1 day remaining', overdue: false };
    return { text: `${diffDays} days remaining`, overdue: false };
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
      await api.iterations.update(iterationId, formData);
      showEditModal = false;
      await loadProgress();
    } catch (err) {
      console.error('Failed to update iteration:', err);
      alert('Failed to update iteration: ' + (err.message || err));
    }
  }

  async function deleteIteration() {
    if (confirm(`Are you sure you want to delete iteration "${progress?.iteration_name}"?`)) {
      try {
        await api.iterations.delete(iterationId);
        navigate('/iterations');
      } catch (err) {
        console.error('Failed to delete iteration:', err);
        alert('Failed to delete iteration: ' + (err.message || err));
      }
    }
  }

  const segments = $derived(progress ? buildSegments(progress.status_breakdown, progress.total_items) : []);
  const daysInfo = $derived(progress?.end_date ? getDaysRemaining(progress.end_date) : null);

  function buildDropdownItems() {
    return [
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: 'Edit',
        hoverClass: 'hover-bg',
        onClick: startEdit
      },
      {
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: 'Delete',
        color: '#dc2626',
        hoverClass: 'hover:bg-red-50',
        onClick: deleteIteration
      }
    ];
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
        Back to Iterations
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
        <Button onclick={loadProgress} class="mt-4">Retry</Button>
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
              <Target class="w-6 h-6" style="color: {progress.type_color || '#14b8a6'};" />
            </div>
            <div>
              <div class="flex items-center gap-2">
                <h1 class="text-2xl font-semibold" style="color: var(--ds-text);">{progress.iteration_name}</h1>
                {#if iteration?.is_global}
                  <div class="flex items-center gap-1 px-2 py-0.5 rounded text-xs" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                    <Globe class="w-3 h-3" />
                    Global
                  </div>
                {:else if iteration?.workspace_name}
                  <div class="flex items-center gap-1 px-2 py-0.5 rounded text-xs" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                    <Building2 class="w-3 h-3" />
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
            <Calendar class="w-4 h-4" />
            <span>{formatDateShort(progress.start_date)} - {formatDateShort(progress.end_date)}</span>
          </div>
          {#if daysInfo}
            <span>|</span>
            <span class={daysInfo.overdue ? 'text-red-500 font-medium' : 'text-blue-500'}>
              {daysInfo.text}
            </span>
          {/if}
        </div>
      </div>

      <!-- Progress Section -->
      <div class="grid grid-cols-1 md:grid-cols-3 gap-6 mb-6">
        <!-- Circular Progress Chart -->
        <div class="rounded-xl border p-6 flex flex-col items-center" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
          <div class="relative">
            {#if progress.total_items > 0}
              <svg viewBox="0 0 140 140" class="w-36 h-36" role="img" aria-label="Iteration progress">
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
                  complete
                </text>
              </svg>
            {:else}
              <div class="w-36 h-36 rounded-full border-2 border-dashed flex items-center justify-center" style="border-color: var(--ds-border);">
                <span class="text-sm" style="color: var(--ds-text-subtlest);">No items</span>
              </div>
            {/if}
          </div>
        </div>

        <!-- Summary Stats -->
        <div class="rounded-xl border p-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
          <h3 class="text-sm font-medium mb-4" style="color: var(--ds-text-subtle);">Summary</h3>
          <div class="space-y-3">
            <div class="flex justify-between items-center">
              <span style="color: var(--ds-text-subtle);">Total Items</span>
              <span class="font-semibold" style="color: var(--ds-text);">{progress.total_items}</span>
            </div>
            <div class="flex justify-between items-center">
              <span style="color: var(--ds-text-subtle);">Completed</span>
              <span class="font-semibold text-green-600">{progress.completed_items}</span>
            </div>
            <div class="flex justify-between items-center">
              <span style="color: var(--ds-text-subtle);">Remaining</span>
              <span class="font-semibold" style="color: var(--ds-text);">{progress.total_items - progress.completed_items}</span>
            </div>
          </div>
        </div>

        <!-- Status Breakdown Legend -->
        <div class="rounded-xl border p-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
          <h3 class="text-sm font-medium mb-4" style="color: var(--ds-text-subtle);">By Status Category</h3>
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
              <p class="text-sm" style="color: var(--ds-text-subtlest);">No status data</p>
            {/if}
          </div>
        </div>
      </div>

      <!-- Items Grouped by Category -->
      <div class="space-y-4">
        <h2 class="text-lg font-semibold" style="color: var(--ds-text);">Work Items</h2>

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
              icon={Target}
              title="No items assigned"
              description="Assign work items to this iteration to track progress"
            />
          </div>
        {/if}
      </div>
    {/if}
  </div>
</div>

<!-- Edit Modal -->
<Modal
  isOpen={showEditModal}
  on:close={() => showEditModal = false}
  onSubmit={saveIteration}
  submitDisabled={!formData.name.trim() || !formData.start_date || !formData.end_date}
  maxWidth="max-w-2xl"
  let:submitHint
>
  <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
    <h3 class="text-lg font-semibold" style="color: var(--ds-text);">Edit Iteration</h3>
  </div>

  <div class="px-6 py-4">
    <form onsubmit={(e) => { e.preventDefault(); saveIteration(); }}>
      <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
        <div>
          <Label for="iteration-name" required class="mb-2">Iteration Name</Label>
          <input
            id="iteration-name"
            type="text"
            bind:value={formData.name}
            class="w-full px-4 py-3 rounded border transition-all duration-200 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-opacity-50"
            style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
            placeholder="e.g., Sprint 1"
            required
          />
        </div>

        <div>
          <Label for="iteration-status" class="mb-2">Status</Label>
          <BasePicker
            bind:value={formData.status}
            items={statusOptions}
            placeholder="Select status..."
            getValue={(item) => item.value}
            getLabel={(item) => item.label}
          />
        </div>

        <div>
          <Label for="iteration-start-date" required class="mb-2">Start Date</Label>
          <input
            id="iteration-start-date"
            type="date"
            bind:value={formData.start_date}
            class="w-full px-4 py-3 rounded border transition-all duration-200 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-opacity-50"
            style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
            required
          />
        </div>

        <div>
          <Label for="iteration-end-date" required class="mb-2">End Date</Label>
          <input
            id="iteration-end-date"
            type="date"
            bind:value={formData.end_date}
            class="w-full px-4 py-3 rounded border transition-all duration-200 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-opacity-50"
            style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
            required
          />
        </div>

        <div class="md:col-span-2">
          <Label for="iteration-description" class="mb-2">Description</Label>
          <Textarea
            id="iteration-description"
            bind:value={formData.description}
            rows={3}
            placeholder="Optional description"
          />
        </div>
      </div>
    </form>
  </div>

  <DialogFooter
    onCancel={() => showEditModal = false}
    onConfirm={saveIteration}
    confirmLabel="Update Iteration"
    disabled={!formData.name.trim() || !formData.start_date || !formData.end_date}
    showKeyboardHint={true}
    confirmKeyboardHint={submitHint}
  />
</Modal>

