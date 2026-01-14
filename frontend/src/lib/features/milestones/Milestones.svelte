<script>
  import { onMount } from 'svelte';
  import {
    Milestone, Calendar, CheckCircle, Clock, Plus, Edit, Trash2,
    MoreHorizontal, Tag, MessageSquare, Globe, Building2
  } from 'lucide-svelte';
  import DataTable from '../../components/DataTable.svelte';
  import Button from '../../components/Button.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import CategoryModal from '../../dialogs/CategoryModal.svelte';
  import MilestoneNavigation from './MilestoneNavigation.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import { categoriesStore } from '../../stores/categories.js';
  import { milestonesStore } from '../../stores/milestones.js';
  import { moduleSettings } from '../../stores/moduleSettings.js';
  import { currentRoute, navigate } from '../../router.js';
  import { formatDateShort } from '../../utils/dateFormatter.js';
  import { api } from '../../api.js';
  import ColorDot from '../../components/ColorDot.svelte';
  import Label from '../../components/Label.svelte';
  import BasePicker from '../../pickers/BasePicker.svelte';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';
  import { createShortcutHandler } from '../../utils/keyboardShortcuts.js';

  // Props for workspace-scoped view (optional)
  let { workspaceId = null } = $props();

  // Determine if this is global view (no workspaceId) or workspace-scoped
  const isGlobalView = $derived(!workspaceId);

  let showCreateForm = $state(false);
  let editingMilestone = $state(null);
  let showCategoryForm = $state(false);
  let testStatistics = $state({}); // Store test stats by milestone ID
  let workspaces = $state([]); // For workspace picker when creating local milestones
  let formData = $state({
    name: '',
    description: '',
    target_date: '',
    status: 'planning',
    category_id: null,
    is_global: true,
    workspace_id: null
  });

  const statusOptions = [
    { value: 'planning', label: 'Planning', lozengeColor: 'grey', icon: Clock },
    { value: 'in-progress', label: 'In Progress', lozengeColor: 'blue', icon: Milestone },
    { value: 'completed', label: 'Completed', lozengeColor: 'green', icon: CheckCircle },
    { value: 'cancelled', label: 'Cancelled', lozengeColor: 'red', icon: Milestone }
  ];

  // Get active category from URL params (only used in global view)
  let activeCategoryId = $derived($currentRoute.params?.categoryId || null);

  onMount(async () => {
    await Promise.all([
      loadData(),
      moduleSettings.load()
    ]);

    // Load test statistics if test management is enabled
    if ($moduleSettings.test_management_enabled) {
      await loadTestStatistics();
    }

    // Listen for manage categories event from navigation
    document.addEventListener('manage-categories', () => {
      showCategoryForm = true;
    });

  });

  function handleGlobalKeydown(event) {
    createShortcutHandler({
      add: startCreate
    }, 'milestones', { guard: () => !showCreateForm })(event);
  }

  async function loadData() {
    try {
      // In workspace view, filter milestones by workspace_id and include global
      const filters = isGlobalView ? {} : { workspace_id: workspaceId, include_global: true };
      const [_, milestones, ws] = await Promise.all([
        categoriesStore.init(),
        api.milestones.getAll(filters),
        api.workspaces.getAll()
      ]);
      // Update the store with filtered milestones
      milestonesStore.set(milestones || []);
      workspaces = ws || [];
    } catch (error) {
      console.error('Failed to load data:', error);
    }
  }

  async function loadTestStatistics() {
    try {
      const milestones = $milestonesStore;
      const statsPromises = milestones.map(async (milestone) => {
        try {
          const stats = await api.milestones.getTestStatistics(milestone.id);
          return { milestoneId: milestone.id, stats };
        } catch (error) {
          console.error(`Failed to load test stats for milestone ${milestone.id}:`, error);
          return { milestoneId: milestone.id, stats: null };
        }
      });
      
      const results = await Promise.all(statsPromises);
      const newTestStatistics = {};
      results.forEach(({ milestoneId, stats }) => {
        if (stats) {
          newTestStatistics[milestoneId] = stats;
        }
      });
      
      testStatistics = newTestStatistics;
    } catch (error) {
      console.error('Failed to load test statistics:', error);
    }
  }

  function startCreate() {
    showCreateForm = true;
    editingMilestone = null;
    resetForm();
  }

  function startEdit(milestone) {
    editingMilestone = milestone;
    formData = {
      name: milestone.name,
      description: milestone.description || '',
      target_date: milestone.target_date ? milestone.target_date.split('T')[0] : '',
      status: milestone.status,
      category_id: milestone.category_id,
      is_global: milestone.is_global !== false, // Default to true if undefined
      workspace_id: milestone.workspace_id || null
    };
    showCreateForm = true;
  }

  function resetForm() {
    formData = {
      name: '',
      description: '',
      target_date: '',
      status: 'planning',
      category_id: null,
      // Auto-set scope based on view context
      is_global: isGlobalView,
      workspace_id: isGlobalView ? null : workspaceId
    };
  }

  function cancelForm() {
    showCreateForm = false;
    editingMilestone = null;
    resetForm();
  }

  async function saveMilestone() {
    try {
      if (editingMilestone) {
        // Update existing milestone
        await milestonesStore.update(editingMilestone.id, formData);
      } else {
        // Create new milestone
        await milestonesStore.add(formData);
      }
      
      cancelForm();
    } catch (error) {
      console.error('Failed to save milestone:', error);
      alert('Failed to save milestone: ' + (error.message || error));
    }
  }

  async function deleteMilestone(milestone) {
    if (confirm(`Are you sure you want to delete milestone "${milestone.name}"?`)) {
      try {
        await milestonesStore.delete(milestone.id);
      } catch (error) {
        console.error('Failed to delete milestone:', error);
        alert('Failed to delete milestone: ' + (error.message || error));
      }
    }
  }

  function getStatusInfo(status) {
    return statusOptions.find(s => s.value === status) || statusOptions[0];
  }

  function buildMilestoneDropdownItems(milestone) {
    return [
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: 'Edit',
        hoverClass: 'hover:bg-gray-100',
        onClick: () => startEdit(milestone)
      },
      {
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: 'Delete',
        color: '#dc2626',
        hoverClass: 'hover:bg-red-50',
        onClick: () => deleteMilestone(milestone)
      }
    ];
  }

  function isOverdue(targetDate, status) {
    if (status === 'completed' || status === 'cancelled' || !targetDate) return false;
    const today = new Date();
    const target = new Date(targetDate);
    return target < today;
  }

  function getDaysUntil(targetDate) {
    if (!targetDate) return '';
    const today = new Date();
    const target = new Date(targetDate);
    const diffTime = target - today;
    const diffDays = Math.ceil(diffTime / (1000 * 60 * 60 * 24));
    
    if (diffDays < 0) return `${Math.abs(diffDays)} days overdue`;
    if (diffDays === 0) return 'Due today';
    if (diffDays === 1) return '1 day remaining';
    return `${diffDays} days remaining`;
  }

  function getCategoryById(categoryId, categories) {
    return categoriesStore.getById(categoryId, categories);
  }

  async function handleAddCategory(data) {
    await categoriesStore.add(data);
  }

  async function handleDeleteCategory(categoryId) {
    await categoriesStore.delete(categoryId);
  }

  // Filter milestones based on active category (only applies in global view)
  let filteredMilestones = $derived(
    isGlobalView && activeCategoryId
      ? $milestonesStore.filter(m => m.category_id === parseInt(activeCategoryId))
      : $milestonesStore
  );

  // DataTable configuration
  let milestoneColumns = $derived([
    { 
      key: 'status', 
      label: 'Status', 
      width: 'w-32',
      slot: 'status'
    },
    { 
      key: 'name', 
      label: 'Milestone', 
      slot: 'name'
    },
    { 
      key: 'target_date', 
      label: 'Target Date', 
      width: 'w-40',
      render: (milestone) => {
        return formatDateShort(milestone.target_date) || '-';
      }
    },
    { 
      key: 'days_remaining', 
      label: 'Timeline', 
      width: 'w-48',
      slot: 'days_remaining'
    },
    ...$moduleSettings.test_management_enabled ? [{
      key: 'tests',
      label: 'Tests',
      width: 'w-24',
      slot: 'tests'
    }] : [],
    {
      key: 'actions',
      label: '',
      width: 'w-16'
    }
  ]);
</script>

<svelte:window onkeydown={handleGlobalKeydown} />

<!-- Main container with two-panel layout -->
<div class="flex min-h-screen" style="background-color: var(--ds-surface);">
  <!-- Left Sidebar - Navigation (only in global view) -->
  {#if isGlobalView}
    <MilestoneNavigation />
  {/if}

  <!-- Main Content -->
  <div class="flex-1">
    <div class="p-6">
      <!-- Header -->
      <div class="flex justify-between items-center mb-6">
        <div>
          <h1 class="text-xl font-semibold" style="color: var(--ds-text);">
            {#if isGlobalView}
              {#if activeCategoryId}
                {@const category = getCategoryById(parseInt(activeCategoryId), $categoriesStore)}
                {category ? category.name : 'Category'} Milestones
              {:else}
                All Milestones
              {/if}
            {:else}
              Workspace Milestones
            {/if}
          </h1>
          <p class="mt-1 text-sm" style="color: var(--ds-text-subtle);">
            {filteredMilestones.length} milestone{filteredMilestones.length !== 1 ? 's' : ''}
            {#if isGlobalView && activeCategoryId}in this category{/if}
            {#if !isGlobalView}(local + global){/if}
          </p>
        </div>
        <Button
          variant="primary"
          icon={Plus}
          onclick={startCreate}
          keyboardHint="A"
        >
          Add Milestone
        </Button>
      </div>


      <!-- Empty State or DataTable -->
      {#if filteredMilestones.length === 0}
        <div class="flex flex-col items-center justify-center p-12 border-2 border-dashed rounded" style="border-color: var(--ds-border);">
          <Milestone class="w-12 h-12 mb-4" style="color: var(--ds-text-subtle);" />
          <h3 class="text-lg font-medium mb-2" style="color: var(--ds-text);">No milestones yet</h3>
          <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
            {#if isGlobalView && activeCategoryId}
              No milestones in this category
            {:else}
              Create your first milestone to track releases and goals
            {/if}
          </p>
          <Button
            variant="primary"
            icon={Plus}
            onclick={startCreate}
            keyboardHint="A"
          >
            Add Milestone
          </Button>
        </div>
      {:else}
        <DataTable
          columns={milestoneColumns}
          data={filteredMilestones}
          keyField="id"
          actionItems={buildMilestoneDropdownItems}
          class="rounded-xl border shadow-sm"
        >
        <!-- Custom Name Cell with Description Tooltip -->
        <div slot="name" let:item>
          {#if item}
            {#key item.id}
              <a
                href="/milestones/{item.id}"
                class="font-medium hover:underline cursor-pointer"
                style="color: var(--ds-text);"
                title={item.description || ''}
                onclick={(e) => { e.preventDefault(); navigate(`/milestones/${item.id}`); }}
              >
                {item.name}
              </a>
            {/key}
          {/if}
        </div>

        <!-- Custom Status Cell -->
        <div slot="status" let:item class="flex items-center gap-2">
          {#if item}
            {#key item.id}
              {@const statusInfo = getStatusInfo(item.status)}
              {@const overdue = isOverdue(item.target_date, item.status)}
              <Lozenge color={statusInfo.lozengeColor} text={statusInfo.label} />
              {#if overdue}
                <Lozenge color="red" text="Overdue" />
              {/if}
            {/key}
          {/if}
        </div>

        <!-- Custom Category Cell -->
        <div slot="category" let:item class="flex items-center gap-2">
          {#if item}
            {#key item.id}
              {@const category = getCategoryById(item.category_id, $categoriesStore)}
              {#if category}
                <ColorDot color={category.color} size="md" />
                <span class="text-sm">{category.name}</span>
              {:else}
                <span class="text-sm text-gray-500">No category</span>
              {/if}
            {/key}
          {/if}
        </div>

        <!-- Custom Timeline Cell -->
        <div slot="days_remaining" let:item>
          {#if item}
            {#key item.id}
              {@const overdue = isOverdue(item.target_date, item.status)}
              {@const daysText = getDaysUntil(item.target_date)}
              <span class="text-sm font-medium {overdue ? 'text-red-600' : item.status === 'completed' ? 'text-green-600' : 'text-blue-600'}">
                {item.status === 'completed' ? 'Completed' : item.status === 'cancelled' ? 'Cancelled' : daysText || 'Open-ended'}
              </span>
            {/key}
          {/if}
        </div>

        <!-- Custom Tests Cell -->
        <div slot="tests" let:item class="text-sm">
          {#if item && $moduleSettings.test_management_enabled}
            {#key item.id}
              {@const stats = testStatistics[item.id]}
              {#if stats}
                <div class="flex flex-col">
                  <span class="text-green-600">{stats.successful_test_runs} ✓</span>
                  {#if stats.failed_test_runs > 0}
                    <span class="text-red-600">{stats.failed_test_runs} ✗</span>
                  {/if}
                </div>
              {:else}
                <span class="text-gray-400">—</span>
              {/if}
            {/key}
          {:else}
            <span class="text-gray-400">—</span>
          {/if}
        </div>
        </DataTable>
      {/if}
    </div>
  </div>
</div>

<!-- Create/Edit Milestone Modal -->
<Modal
  isOpen={showCreateForm}
  onclose={cancelForm}
  onSubmit={saveMilestone}
  submitDisabled={!formData.name.trim()}
  maxWidth="max-w-2xl"
  let:submitHint
>
  <!-- Modal header -->
  <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
    <h3 class="text-lg font-semibold" style="color: var(--ds-text);">
      {editingMilestone ? 'Edit Milestone' : 'New Milestone'}
    </h3>
  </div>

  <!-- Modal content -->
  <div class="px-6 py-4">
    <form onsubmit={(e) => { e.preventDefault(); saveMilestone(); }}>
      <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
        <div>
          <Label for="milestone-name" required class="mb-2">Milestone Name</Label>
          <input
            id="milestone-name"
            type="text"
            bind:value={formData.name}
            class="w-full px-4 py-3 rounded border transition-all duration-200 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-opacity-50"
            style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
            placeholder="e.g., Q1 Release, Beta Launch"
            required
          />
        </div>

        <div>
          <Label for="milestone-target-date" class="mb-2">Target Date</Label>
          <input
            id="milestone-target-date"
            type="date"
            bind:value={formData.target_date}
            class="w-full px-4 py-3 rounded border transition-all duration-200 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-opacity-50"
            style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
          />
        </div>

        <div>
          <Label for="milestone-category" class="mb-2">Category</Label>
          <BasePicker
            bind:value={formData.category_id}
            items={$categoriesStore}
            placeholder="No Category"
            showUnassigned={true}
            unassignedLabel="No Category"
            getValue={(item) => item.id}
            getLabel={(item) => item.name}
          />
        </div>

        <div>
          <Label for="milestone-status" class="mb-2">Status</Label>
          <BasePicker
            bind:value={formData.status}
            items={statusOptions}
            placeholder="Select status"
            getValue={(item) => item.value}
            getLabel={(item) => item.label}
          />
        </div>

        <!-- Scope Toggle (only shown when editing existing milestone) -->
        {#if editingMilestone}
          <div>
            <Label class="mb-2">Scope</Label>
            <div class="flex rounded-lg overflow-hidden border" style="border-color: var(--ds-border);">
              <button
                type="button"
                onclick={() => formData = { ...formData, is_global: true, workspace_id: null }}
                class="flex-1 px-4 py-3 text-sm font-medium flex items-center justify-center gap-2 transition-colors"
                style="
                  background-color: {formData.is_global ? 'var(--ds-interactive-primary)' : 'var(--ds-background-input)'};
                  color: {formData.is_global ? 'white' : 'var(--ds-text)'};
                "
              >
                <Globe class="w-4 h-4" />
                Global
              </button>
              <button
                type="button"
                onclick={() => formData = { ...formData, is_global: false }}
                class="flex-1 px-4 py-3 text-sm font-medium flex items-center justify-center gap-2 transition-colors"
                style="
                  background-color: {!formData.is_global ? 'var(--ds-interactive-primary)' : 'var(--ds-background-input)'};
                  color: {!formData.is_global ? 'white' : 'var(--ds-text)'};
                "
              >
                <Building2 class="w-4 h-4" />
                Local
              </button>
            </div>
          </div>

          <!-- Workspace Picker (only shown for local milestones when editing) -->
          {#if !formData.is_global}
            <div>
              <Label required class="mb-2">Workspace</Label>
              <BasePicker
                bind:value={formData.workspace_id}
                items={workspaces}
                placeholder="Select workspace"
                getValue={(item) => item.id}
                getLabel={(item) => item.name}
              />
            </div>
          {/if}
        {/if}

        <div class="md:col-span-2">
          <Label for="milestone-description" class="mb-2">Description</Label>
          <Textarea
            id="milestone-description"
            bind:value={formData.description}
            rows={3}
            placeholder="Optional description of this milestone"
          />
        </div>
      </div>
    </form>
  </div>

  <DialogFooter
    onCancel={cancelForm}
    onConfirm={saveMilestone}
    confirmLabel={editingMilestone ? 'Update Milestone' : 'Create Milestone'}
    disabled={!formData.name.trim()}
    showKeyboardHint={true}
    confirmKeyboardHint={submitHint}
  />
</Modal>

<!-- Category Management Modal -->
<CategoryModal
  isOpen={showCategoryForm}
  onClose={() => showCategoryForm = false}
  title="Manage Milestone Categories"
  categories={$categoriesStore}
  onAdd={handleAddCategory}
  onDelete={handleDeleteCategory}
  showColorPicker={true}
/>
