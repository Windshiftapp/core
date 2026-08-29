<script>
  import { onMount } from 'svelte';
  import { draggable, dropTargetForElements } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import { attachClosestEdge, extractClosestEdge } from '@atlaskit/pragmatic-drag-and-drop-hitbox/closest-edge';
  import { t } from '../../stores/i18n.svelte.js';
  import { confirm } from '../../composables/useConfirm.js';
  import { errorToast } from '../../stores/toasts.svelte.js';
  import {
    IconFlag as Milestone, IconCalendar as Calendar, IconCircleCheck as CheckCircle, IconClock as Clock, IconPlus as Plus, IconEdit as Edit, IconTrash as Trash2,
    IconDots as MoreHorizontal, IconTag as Tag, IconMessage as MessageSquare, IconWorld as Globe, IconBuilding as Building2, IconGitBranch as GitBranch, IconGripVertical as GripVertical
  } from '@tabler/icons-svelte-runes';
  import DataTable from '../../components/DataTable.svelte';
  import Button from '../../components/Button.svelte';
  import Toggle from '../../components/Toggle.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import CategoryModal from '../../dialogs/CategoryModal.svelte';
  import MilestoneNavigation from './MilestoneNavigation.svelte';
  import MilestoneReleaseModal from './MilestoneReleaseModal.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import { categoriesStore } from '../../stores/categories.js';
  import { milestonesStore } from '../../stores/milestones.js';
  import { moduleSettings } from '../../stores/moduleSettings.js';
  import { currentRoute } from '../../router.js';
  import { formatDateShort } from '../../utils/dateFormatter.js';
  import { api } from '../../api.js';
  import { permissionStore, isSystemAdmin } from '../../stores/permissions.svelte.js';
  import { workspacePermissions } from '../../stores/workspacePermissions.svelte.js';
  import ColorDot from '../../components/ColorDot.svelte';
  import MilestoneFormDialog from './MilestoneFormDialog.svelte';
  import { toHotkeyString } from '../../utils/keyboardShortcuts.js';
  import EmptyState from '../../components/EmptyState.svelte';
  import PageHeader from '../../layout/PageHeader.svelte';
  import { useEventListener } from 'runed';
  import { loadMilestoneTestStatistics } from './milestoneStatisticsData.js';
  import {
    preservePlanningScope
  } from '../../utils/planningScope.js';

  // Props for workspace-scoped view (optional)
  let { workspaceId = null } = $props();

  // Determine if this is global view (no workspaceId) or workspace-scoped
  const isGlobalView = $derived(!workspaceId);

  const canManageGlobal = $derived(
    $permissionStore.userPermissionKeys?.has('milestone.create') || $isSystemAdmin
  );

  const canCreate = $derived.by(() => {
    if ($isSystemAdmin) return true;
    if (isGlobalView) {
      return canManageGlobal;
    } else {
      // For local milestones, require workspace.admin or item.edit (which is what backend checks)
      return workspacePermissions.canAdminWorkspace(workspaceId) || 
             workspacePermissions.hasPermission(workspaceId, 'item.edit');
    }
  });

  let showCreateForm = $state(false);
  let editingMilestone = $state(null);
  let showCategoryForm = $state(false);
  let testStatistics = $state({}); // Store test stats by milestone ID
  let workspaces = $state([]); // For workspace picker when creating local milestones
  let showReleaseModal = $state(false);
  let releasingMilestone = $state(null);

  // "Hide completed" toggle — defaults ON. Persisted in localStorage so the
  // preference survives reloads; a missing key means ON (default).
  const HIDE_COMPLETED_KEY = 'milestones.hideCompleted';
  let hideCompleted = $state(localStorage.getItem(HIDE_COMPLETED_KEY) !== 'false');
  function handleHideCompletedChange(checked) {
    hideCompleted = checked;
    try {
      localStorage.setItem(HIDE_COMPLETED_KEY, String(hideCompleted));
    } catch {
      // Ignore localStorage errors (private mode, quota, etc).
    }
  }

  // Drag-and-drop reorder state. One set of cleanups per render; the
  // milestone rows are re-wired whenever the visible list changes.
  let dragCleanups = [];
  let dragEdge = $state({}); // { [rowKey]: 'before' | 'after' | null }

  let formData = $state({
    name: '',
    description: '',
    target_date: '',
    status: 'planning',
    category_id: null,
    is_global: true,
    workspace_id: null
  });

  let statusOptions = $derived([
    { value: 'planning', label: t('milestones.status.planning'), lozengeColor: 'grey', icon: Clock },
    { value: 'in-progress', label: t('milestones.status.inProgress'), lozengeColor: 'blue', icon: Milestone },
    { value: 'completed', label: t('milestones.status.completed'), lozengeColor: 'green', icon: CheckCircle },
    { value: 'cancelled', label: t('milestones.status.cancelled'), lozengeColor: 'red', icon: Milestone }
  ]);

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
  });

  useEventListener(() => document, 'manage-categories', () => { showCategoryForm = true; });

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
      testStatistics = await loadMilestoneTestStatistics(api, $milestonesStore);
    } catch (error) {
      console.error('Failed to load test statistics:', error);
      testStatistics = {};
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
      status: milestone.status ?? 'planning',
      category_id: milestone.category_id ?? null,
      is_global: milestone.is_global !== false, // Default to true if undefined
      workspace_id: milestone.workspace_id ? parseInt(milestone.workspace_id, 10) : null
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
      workspace_id: isGlobalView ? null : (workspaceId ? parseInt(workspaceId, 10) : null)
    };
  }

  function cancelForm() {
    showCreateForm = false;
    editingMilestone = null;
    resetForm();
  }

  async function saveMilestone() {
    try {
      // Convert empty strings to null for optional date fields
      const dataToSave = preservePlanningScope(
        {
          ...formData,
          target_date: formData.target_date || null
        },
        editingMilestone
      );
      if (dataToSave.is_global) {
        dataToSave.workspace_id = null;
      }

      if (editingMilestone) {
        // Update existing milestone
        await milestonesStore.update(editingMilestone.id, dataToSave);
      } else {
        // Create new milestone
        await milestonesStore.add(dataToSave);
      }

      cancelForm();
    } catch (error) {
      console.error('Failed to save milestone:', error);
      errorToast(error.message || String(error), t('errors.failedToSave'));
    }
  }

  async function deleteMilestone(milestone) {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('milestones.confirmDelete', { name: milestone.name }),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger'
    });
    if (confirmed) {
      try {
        await milestonesStore.delete(milestone.id);
      } catch (error) {
        console.error('Failed to delete milestone:', error);
        errorToast(error.message || String(error), t('errors.failedToDelete'));
      }
    }
  }

  function getStatusInfo(status) {
    return statusOptions.find(s => s.value === status) || statusOptions[0];
  }

  function buildMilestoneDropdownItems(milestone) {
    const canManage = milestone.is_global 
      ? canManageGlobal 
      : (workspacePermissions.canAdminWorkspace(milestone.workspace_id || workspaceId) || 
         workspacePermissions.hasPermission(milestone.workspace_id || workspaceId, 'item.edit'));

    if (!canManage) return [];

    return [
      {
        id: 'release',
        type: 'regular',
        icon: Tag,
        title: 'Release',
        hoverClass: 'hover-bg',
        onClick: () => { releasingMilestone = milestone; showReleaseModal = true; }
      },
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: t('common.edit'),
        hoverClass: 'hover-bg',
        onClick: () => startEdit(milestone)
      },
      {
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: t('common.delete'),
        color: 'var(--ds-text-danger)',
        hoverClass: 'hover-danger',
        onClick: () => deleteMilestone(milestone)
      }
    ];
  }

  async function handleReleased(updatedMilestone) {
    showReleaseModal = false;
    releasingMilestone = null;
    // Refresh milestones to show updated status
    const filters = isGlobalView ? {} : { workspace_id: workspaceId, include_global: true };
    try {
      const milestones = await api.milestones.getAll(filters);
      milestonesStore.set(milestones || []);
    } catch (err) {
      console.error('Failed to refresh milestones:', err);
    }
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
    const diffTime = target.getTime() - today.getTime();
    const diffDays = Math.ceil(diffTime / (1000 * 60 * 60 * 24));

    if (diffDays < 0) return t('milestones.daysOverdue', { count: Math.abs(diffDays) });
    if (diffDays === 0) return t('milestones.dueToday');
    if (diffDays === 1) return t('milestones.oneDayRemaining');
    return t('milestones.daysRemaining', { count: diffDays });
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

  // Layer "hide completed" over the category filter before the local/global
  // split so both sections (and the global-view table) honor the toggle.
  // Completed milestones remain reachable from the detail nav and detail page;
  // the toggle only affects this list view.
  let visibleMilestones = $derived(
    hideCompleted ? filteredMilestones.filter(m => m.status !== 'completed') : filteredMilestones
  );

  let localMilestones = $derived(
    visibleMilestones.filter(m => !m.is_global)
  );

  let globalMilestones = $derived(
    visibleMilestones.filter(m => m.is_global)
  );

  // Whether the current user may reorder milestones in the active scope(s).
  // Mirrors the backend permission gates (workspaceItemEdit for local,
  // globalMilestoneManage / milestone.create for global).
  const canReorderLocal = $derived(
    $isSystemAdmin ||
    workspacePermissions.canAdminWorkspace(workspaceId) ||
    workspacePermissions.hasPermission(workspaceId, 'item.edit')
  );
  const canReorderGlobal = $derived(canManageGlobal);

  function parseOptionalInt(value) {
    const parsed = parseInt(value, 10);
    return Number.isNaN(parsed) ? null : parsed;
  }

  function milestoneInReorderScope(milestone, scope) {
    if (milestone.is_global !== scope.is_global) return false;
    if (!scope.is_global && (milestone.workspace_id ?? null) !== (scope.workspace_id ?? null)) return false;
    return (milestone.category_id ?? null) === (scope.category_id ?? null);
  }

  // Re-compute the ordered id list for a scope after a drag and persist it.
  // `scope` is { is_global, workspace_id, category_id }; `sourceId` is the
  // dragged milestone; `targetId` is the drop target; `edge` is 'before' or
  // 'after' relative to the target. The backend requires the complete scope,
  // so include hidden completed milestones and only move within one category.
  async function handleReorder(scope, sourceId, targetId, edge) {
    const allMilestones = [...$milestonesStore];
    const list = allMilestones
      .filter((m) => milestoneInReorderScope(m, scope))
      .sort((a, b) => (a.position ?? 0) - (b.position ?? 0) || a.name.localeCompare(b.name));

    // Build the new ordering: remove the dragged item, then insert it above
    // or below the drop target.
    const withoutSource = list.filter((m) => m.id !== sourceId);
    const dragged = list.find((m) => m.id === sourceId);
    if (!dragged) return;
    let insertIndex = withoutSource.findIndex((m) => m.id === targetId);
    if (insertIndex === -1) {
      insertIndex = withoutSource.length;
    } else if (edge === 'after') {
      insertIndex += 1;
    }
    const reordered = [...withoutSource];
    reordered.splice(insertIndex, 0, dragged);
    const orderedIds = reordered.map((m) => m.id);

    try {
      await milestonesStore.reorder(scope, orderedIds, allMilestones);
    } catch (error) {
      errorToast(error.message || String(error), t('errors.failedToSave'));
    }
  }

  // Wire pragmatic-drag-and-drop onto rendered milestone rows. Re-runs
  // whenever the visible lists change (rows are re-created by DataTable on
  // each render). Drag handle is the first cell's grip icon when present.
  $effect(() => {
    // Read the derived lists so this effect re-runs when they change.
    void visibleMilestones;
    void localMilestones;
    void globalMilestones;

    // Defer until after the DOM reflects the new rows.
    const timer = setTimeout(() => {
      dragCleanups.forEach((fn) => fn());
      dragCleanups = [];
      dragEdge = {};

      /** @type {NodeListOf<HTMLElement>} */ (document.querySelectorAll('[data-milestone-row]')).forEach((row) => {
        const id = parseInt(row.dataset.milestoneRow, 10);
        const scopeName = row.dataset.milestoneScope; // 'global' | 'local'
        const ws = row.dataset.milestoneWs;
        const categoryId = row.dataset.milestoneCategory;
        if (Number.isNaN(id)) return;

        const canDrag = scopeName === 'global' ? canReorderGlobal : canReorderLocal;
        if (!canDrag) return;

        dragEdge[id] = null;

        const draggableCleanup = draggable({
          element: row,
          dragHandle: row.querySelector('[data-milestone-drag-handle]') || row,
          getInitialData: () => ({ id, scope: scopeName, workspaceId: ws, categoryId, type: 'milestone' }),
          onDragStart: () => { row.style.opacity = '0.4'; },
          onDrop: () => {
            row.style.opacity = '';
            dragEdge = {};
          },
        });

        const dropTargetCleanup = dropTargetForElements({
          element: row,
          canDrop: ({ source }) =>
            source.data?.type === 'milestone' &&
            source.data?.scope === scopeName &&
            source.data?.workspaceId === ws &&
            source.data?.categoryId === categoryId,
          getData: ({ input, element }) =>
            attachClosestEdge({}, { input, element, allowedEdges: ['top', 'bottom'] }),
          onDragEnter: ({ self }) => {
            dragEdge[id] = extractClosestEdge(self.data) === 'bottom' ? 'after' : 'before';
            dragEdge = { ...dragEdge };
          },
          onDragLeave: () => {
            dragEdge[id] = null;
            dragEdge = { ...dragEdge };
          },
          onDrop: ({ self, source }) => {
            const edge = extractClosestEdge(self.data) === 'bottom' ? 'after' : 'before';
            const sourceId = source.data.id;
            dragEdge[id] = null;
            dragEdge = { ...dragEdge };
            if (sourceId === id) return;
            const scope =
              scopeName === 'global'
                ? { is_global: true, category_id: parseOptionalInt(categoryId) }
                : { is_global: false, workspace_id: parseOptionalInt(ws), category_id: parseOptionalInt(categoryId) };
            handleReorder(scope, sourceId, id, edge);
          },
        });

        dragCleanups.push(() => { draggableCleanup(); dropTargetCleanup(); });
      });
    }, 0);

    return () => {
      clearTimeout(timer);
      dragCleanups.forEach((fn) => fn());
      dragCleanups = [];
    };
  });

  // DataTable configuration
  let milestoneColumns = $derived([
    // Drag handle column — only present when the user may reorder the
    // active scope. The handle is the pragmatic-drag-and-drop dragHandle.
    ...(canReorderGlobal || canReorderLocal)
      ? [{ key: 'reorder', label: '', width: 'w-10', slot: 'reorder' }]
      : [],
    {
      key: 'status',
      label: t('milestones.columnStatus'),
      width: 'w-40',
      slot: 'status'
    },
    { 
      key: 'name', 
      label: t('milestones.columnMilestone'),
      slot: 'name'
    },
    { 
      key: 'target_date', 
      label: t('milestones.columnTargetDate'),
      width: 'w-40',
      render: (milestone) => {
        return formatDateShort(milestone.target_date) || '-';
      }
    },
    { 
      key: 'days_remaining', 
      label: t('milestones.columnTimeline'),
      width: 'w-48',
      slot: 'days_remaining'
    },
    ...$moduleSettings.test_management_enabled ? [{
      key: 'tests',
      label: t('milestones.columnTests'),
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
      <PageHeader
        title={isGlobalView
          ? (activeCategoryId
              ? `${getCategoryById(parseInt(activeCategoryId), $categoriesStore)?.name || t('common.category')} ${t('milestones.title')}`
              : t('milestones.allMilestones'))
          : t('milestones.workspaceMilestones')}
        subtitle={!isGlobalView
          ? `${localMilestones.length} ${t('milestones.local').toLowerCase()}, ${globalMilestones.length} ${t('milestones.global').toLowerCase()}`
          : activeCategoryId
            ? t('milestones.visibleInCategory', { count: visibleMilestones.length })
            : t('milestones.visibleCount', { count: visibleMilestones.length })}
      >
        {#snippet actions()}
          <div class="flex items-center gap-2">
            <div data-testid="milestone-hide-completed-toggle" title={t('milestones.hideCompletedHelp')}>
              <Toggle
                checked={hideCompleted}
                label={t('milestones.hideCompleted')}
                labelPosition="left"
                onchange={handleHideCompletedChange}
              />
            </div>
            {#if canCreate}
              <Button
                variant="primary"
                icon={Plus}
                onclick={startCreate}
                keyboardHint="A"
                hotkeyConfig={{ key: toHotkeyString('milestones', 'add'), guard: () => !showCreateForm }}
                dataTestid="milestone-create-button"
              >
                {t('milestones.addMilestone')}
              </Button>
            {/if}
          </div>
        {/snippet}
      </PageHeader>


      {#snippet reorderCell(item)}
        {#if item}
          <span
            data-milestone-drag-handle
            class="inline-flex items-center justify-center cursor-grab active:cursor-grabbing"
            style="color: var(--ds-text-subtlest);"
            title={t('milestones.dragToReorder')}
          >
            <GripVertical class="w-4 h-4" />
          </span>
        {/if}
      {/snippet}

      {#snippet nameCell(item)}
        {#if item}
          {#key item.id}
            <a
              href="/milestones/{item.id}{workspaceId ? `?workspaceId=${workspaceId}` : ''}"
              class="font-medium hover:underline cursor-pointer"
              style="color: var(--ds-text);"
              title={item.description || ''}
            >
              {item.name}
            </a>
          {/key}
        {/if}
      {/snippet}

      {#snippet statusCell(item)}
        {#if item}
          {#key item.id}
            {@const statusInfo = getStatusInfo(item.status)}
            {@const overdue = isOverdue(item.target_date, item.status)}
            <Lozenge color={statusInfo.lozengeColor} text={statusInfo.label} />
            {#if overdue}
              <Lozenge color="red" text={t('milestones.overdue')} />
            {/if}
          {/key}
        {/if}
      {/snippet}

      {#snippet categoryCell(item)}
        {#if item}
          {#key item.id}
            {@const category = getCategoryById(item.category_id, $categoriesStore)}
            {#if category}
              <ColorDot color={category.color} size="md" />
              <span class="text-sm">{category.name}</span>
            {:else}
              <span class="text-sm text-gray-500">{t('milestones.noCategory')}</span>
            {/if}
          {/key}
        {/if}
      {/snippet}

      {#snippet daysRemainingCell(item)}
        {#if item}
          {#key item.id}
            {@const overdue = isOverdue(item.target_date, item.status)}
            {@const daysText = getDaysUntil(item.target_date)}
            <span class="text-sm font-medium" style="color: var({overdue ? '--ds-text-danger' : item.status === 'completed' ? '--ds-text-success' : '--ds-text-info'})">
              {item.status === 'completed' ? t('milestones.status.completed') : item.status === 'cancelled' ? t('milestones.status.cancelled') : daysText || t('milestones.openEnded')}
            </span>
          {/key}
        {/if}
      {/snippet}

      {#snippet testsCell(item)}
        {#if item && $moduleSettings.test_management_enabled}
          {#key item.id}
            {@const stats = testStatistics[item.id]}
            {#if stats}
              <div class="flex flex-col">
                <span style="color: var(--ds-text-success);">{stats.successful_test_runs} ✓</span>
                {#if stats.failed_test_runs > 0}
                  <span style="color: var(--ds-text-danger);">{stats.failed_test_runs} ✗</span>
                {/if}
              </div>
            {:else}
              <span class="text-gray-400">—</span>
            {/if}
          {/key}
        {:else}
          <span class="text-gray-400">—</span>
        {/if}
      {/snippet}

      <!-- Empty State or DataTable -->
      {#if visibleMilestones.length === 0}
        <EmptyState
          icon={Milestone}
          title={isGlobalView && activeCategoryId ? t('milestones.noMilestonesInCategory') : (hideCompleted ? t('milestones.noVisibleMilestones') : t('milestones.noMilestones'))}
          description={isGlobalView && activeCategoryId ? t('categories.noCategorizedWork') : t('milestones.noMilestonesDescription')}
        >
          {#snippet action()}
            {#if canCreate}
              <Button variant="primary" icon={Plus} onclick={startCreate} keyboardHint="A">
                {t('milestones.addMilestone')}
              </Button>
            {/if}
          {/snippet}
        </EmptyState>
      {:else if isGlobalView}
        <DataTable
          columns={milestoneColumns}
          data={visibleMilestones}
          keyField="id"
          actionItems={buildMilestoneDropdownItems}
          class="rounded-xl border shadow-sm"
          rowAttrs={(item) => ({ 'data-milestone-row': item.id, 'data-milestone-scope': item.is_global ? 'global' : 'local', 'data-milestone-ws': item.workspace_id ?? '', 'data-milestone-category': item.category_id ?? '' })}
        >
        {#snippet reorder(item)}{@render reorderCell(item)}{/snippet}
        {#snippet name(item)}{@render nameCell(item)}{/snippet}
        {#snippet status(item)}<div class="flex items-center gap-2">{@render statusCell(item)}</div>{/snippet}
        {#snippet category(item)}<div class="flex items-center gap-2">{@render categoryCell(item)}</div>{/snippet}
        {#snippet days_remaining(item)}{@render daysRemainingCell(item)}{/snippet}
        {#snippet tests(item)}<div class="text-sm">{@render testsCell(item)}</div>{/snippet}
        </DataTable>
      {:else}
        <!-- Workspace view: split into Local and Global sections -->
        <div class="space-y-6">
          {#if localMilestones.length > 0}
            <section class="mt-4">
              <div class="flex items-center gap-3 mb-3">
                <Building2 class="w-5 h-5" style="color: var(--ds-interactive);" />
                <div>
                  <p class="font-semibold text-base" style="color: var(--ds-text);">{t('milestones.localMilestones')}</p>
                  <p class="text-sm" style="color: var(--ds-text-subtle);">{t('milestones.localMilestoneDescription')}</p>
                </div>
              </div>
              <DataTable
                columns={milestoneColumns}
                data={localMilestones}
                keyField="id"
                actionItems={buildMilestoneDropdownItems}
                class="rounded-xl border shadow-sm"
                rowAttrs={(item) => ({ 'data-milestone-row': item.id, 'data-milestone-scope': 'local', 'data-milestone-ws': workspaceId ?? '', 'data-milestone-category': item.category_id ?? '' })}
              >
              {#snippet reorder(item)}{@render reorderCell(item)}{/snippet}
              {#snippet name(item)}{@render nameCell(item)}{/snippet}
              {#snippet status(item)}<div class="flex items-center gap-2">{@render statusCell(item)}</div>{/snippet}
              {#snippet category(item)}<div class="flex items-center gap-2">{@render categoryCell(item)}</div>{/snippet}
              {#snippet days_remaining(item)}{@render daysRemainingCell(item)}{/snippet}
              {#snippet tests(item)}<div class="text-sm">{@render testsCell(item)}</div>{/snippet}
              </DataTable>
            </section>
          {/if}

          {#if globalMilestones.length > 0}
            <section class="mt-12">
              <div class="flex items-center gap-3 mb-3">
                <Globe class="w-5 h-5" style="color: var(--ds-interactive);" />
                <div>
                  <p class="font-semibold text-base" style="color: var(--ds-text);">{t('milestones.globalMilestones')}</p>
                  <p class="text-sm" style="color: var(--ds-text-subtle);">{t('milestones.globalMilestoneDescription')}</p>
                </div>
              </div>
              <DataTable
                columns={milestoneColumns}
                data={globalMilestones}
                keyField="id"
                actionItems={buildMilestoneDropdownItems}
                class="rounded-xl border shadow-sm"
                rowAttrs={(item) => ({ 'data-milestone-row': item.id, 'data-milestone-scope': 'global', 'data-milestone-ws': '', 'data-milestone-category': item.category_id ?? '' })}
              >
              {#snippet reorder(item)}{@render reorderCell(item)}{/snippet}
              {#snippet name(item)}{@render nameCell(item)}{/snippet}
              {#snippet status(item)}<div class="flex items-center gap-2">{@render statusCell(item)}</div>{/snippet}
              {#snippet category(item)}<div class="flex items-center gap-2">{@render categoryCell(item)}</div>{/snippet}
              {#snippet days_remaining(item)}{@render daysRemainingCell(item)}{/snippet}
              {#snippet tests(item)}<div class="text-sm">{@render testsCell(item)}</div>{/snippet}
              </DataTable>
            </section>
          {/if}
        </div>
      {/if}
    </div>
  </div>
</div>

<!-- Create/Edit Milestone Modal -->
<MilestoneFormDialog
  bind:isOpen={showCreateForm}
  bind:formData
  {editingMilestone}
  {isGlobalView}
  {workspaceId}
  canManageGlobal={canManageGlobal}
  canManageWorkspace={!isGlobalView && canCreate}
  onclose={cancelForm}
  onSubmit={saveMilestone}
/>

<!-- Release Modal -->
{#if showReleaseModal && releasingMilestone}
  <Modal
    isOpen={showReleaseModal}
    onclose={() => { showReleaseModal = false; releasingMilestone = null; }}
    maxWidth="max-w-4xl"
    maxHeight="85vh"
  >
    <MilestoneReleaseModal
      milestone={releasingMilestone}
      workspaceId={releasingMilestone.workspace_id ?? workspaceId}
      onreleased={handleReleased}
      onclose={() => { showReleaseModal = false; releasingMilestone = null; }}
    />
  </Modal>
{/if}

<!-- Category Management Modal -->
<CategoryModal
  isOpen={showCategoryForm}
  onClose={() => showCategoryForm = false}
  title={t('milestones.manageMilestoneCategories')}
  categories={$categoriesStore}
  onAdd={handleAddCategory}
  onDelete={handleDeleteCategory}
  showColorPicker={true}
/>
