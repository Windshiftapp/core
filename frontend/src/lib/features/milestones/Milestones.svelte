<script>
  import { onMount } from 'svelte';
  import { draggable, dropTargetForElements } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import { attachClosestEdge, extractClosestEdge } from '@atlaskit/pragmatic-drag-and-drop-hitbox/closest-edge';
  import { t } from '../../stores/i18n.svelte.js';
  import { confirm } from '../../composables/useConfirm.js';
  import { errorToast } from '../../stores/toasts.svelte.js';
  import Milestone from '@tabler/icons-svelte-runes/icons/flag';
  import Calendar from '@tabler/icons-svelte-runes/icons/calendar';
  import CheckCircle from '@tabler/icons-svelte-runes/icons/circle-check';
  import Clock from '@tabler/icons-svelte-runes/icons/clock';
  import Plus from '@tabler/icons-svelte-runes/icons/plus';
  import Edit from '@tabler/icons-svelte-runes/icons/edit';
  import Trash2 from '@tabler/icons-svelte-runes/icons/trash';
  import MoreHorizontal from '@tabler/icons-svelte-runes/icons/dots';
  import Tag from '@tabler/icons-svelte-runes/icons/tag';
  import MessageSquare from '@tabler/icons-svelte-runes/icons/message';
  import Globe from '@tabler/icons-svelte-runes/icons/world';
  import Building2 from '@tabler/icons-svelte-runes/icons/building';
  import GitBranch from '@tabler/icons-svelte-runes/icons/git-branch';
  import GripVertical from '@tabler/icons-svelte-runes/icons/grip-vertical';
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
  import { formatDateShort, formatRelativeTime } from '../../utils/dateFormatter.js';
  import { milestones as milestonesApi } from '../../api/milestones.js';
  import { workspacesStore } from '../../stores/workspaces.svelte.js';
  import { permissionStore, isSystemAdmin } from '../../stores/permissions.svelte.js';
  import { workspacePermissions } from '../../stores/workspacePermissions.svelte.js';
  import ColorDot from '../../components/ColorDot.svelte';
  import MilestoneFormDialog from './MilestoneFormDialog.svelte';
  import { toHotkeyString } from '../../utils/keyboardShortcuts.js';
  import EmptyState from '../../components/EmptyState.svelte';
  import SearchInput from '../../components/SearchInput.svelte';
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
  import { markdownExcerpt } from '../../utils/markdownExcerpt.js';
  import PageHeader from '../../layout/PageHeader.svelte';
  import { useEventListener } from 'runed';
  import { loadMilestoneTestStatistics } from './milestoneStatisticsData.js';
  import { canReorderMilestoneRow } from './milestoneScope.js';
  import {
    milestoneLastUpdated,
    milestoneStatusSortValue,
    milestoneTimelineSortValue,
    timestampSortValue
  } from './milestoneListSort.js';
  import {
    GLOBAL_WORKSPACE_KEY,
    MILESTONE_STATUSES,
    defaultMilestoneListFilters,
    filterMilestones,
    milestoneListFiltersAreDefault,
    milestoneWorkspaceOptions,
    parseStatusFilter
  } from './milestoneListFilters.js';
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

  // Global view filters. The status selection is persisted like
  // the workspace view's "hide completed" toggle; a missing or invalid value
  // hides completed and cancelled milestones. Empty selections show all.
  const STATUS_FILTER_KEY = 'milestones.statusFilter';
  let selectedStatuses = $state(readStoredStatusFilter());
  let selectedWorkspaceKeys = $state([]);
  let searchQuery = $state('');

  function readStoredStatusFilter() {
    try {
      return parseStatusFilter(localStorage.getItem(STATUS_FILTER_KEY));
    } catch {
      return parseStatusFilter(null);
    }
  }

  function setStatusFilter(statuses) {
    selectedStatuses = statuses;
    try {
      localStorage.setItem(STATUS_FILTER_KEY, JSON.stringify(selectedStatuses));
    } catch {
      // Ignore localStorage errors (private mode, quota, etc).
    }
  }

  function toggleStatusFilter(status, checked) {
    setStatusFilter(
      checked ? [...selectedStatuses, status] : selectedStatuses.filter((s) => s !== status)
    );
  }

  function toggleWorkspaceFilter(key, checked) {
    selectedWorkspaceKeys = checked
      ? [...selectedWorkspaceKeys, key]
      : selectedWorkspaceKeys.filter((k) => k !== key);
  }

  // "Clear filters" restores the defaults, including the saved
  // status selection.
  function clearListFilters() {
    const defaults = defaultMilestoneListFilters();
    setStatusFilter(defaults.statuses);
    selectedWorkspaceKeys = defaults.workspaceKeys;
    searchQuery = defaults.search;
  }

  // Column sort of the global view's table, bound to its DataTable. The page
  // opens in the manual order, like the workspace view. A column sort
  // replaces that order, so rows only drag (and show the grip column) while
  // no sort is active.
  let globalSort = $state(null);
  const columnSortActive = $derived(isGlobalView && globalSort != null);

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
      const [_, milestones] = await Promise.all([
        categoriesStore.init(),
        milestonesApi.getAll(listFilters())
      ]);
      // Update the store with filtered milestones
      milestonesStore.set(milestones || []);
      workspaces = await workspacesStore.load();
    } catch (error) {
      console.error('Failed to load data:', error);
    }
  }

  // Global view: global milestones plus milestones from every workspace the
  // viewer can access (the unscoped list). Workspace view: the workspace's
  // milestones plus global ones.
  function listFilters() {
    return isGlobalView ? {} : { workspace_id: workspaceId, include_global: true };
  }

  async function loadTestStatistics() {
    try {
      testStatistics = await loadMilestoneTestStatistics($milestonesStore);
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
    try {
      const milestones = await milestonesApi.getAll(listFilters());
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

  // Layer the list filters over the category filter: the global view uses
  // its workspace/status/search filters, the workspace view the "hide
  // completed" toggle (before its local/global split). Hidden milestones
  // remain reachable from the detail page; the filters only affect this list.
  let visibleMilestones = $derived(
    isGlobalView
      ? filterMilestones(filteredMilestones, {
          statuses: selectedStatuses,
          workspaceKeys: selectedWorkspaceKeys,
          search: searchQuery
        })
      : hideCompleted
        ? filteredMilestones.filter(m => m.status !== 'completed')
        : filteredMilestones
  );

  // Workspace view: "hide completed" may be why the list is empty.
  let listIsFiltered = $derived(!isGlobalView && hideCompleted);

  // Global view: milestones exist (in the active category) but the filters
  // hide every one of them, so the empty state says so and offers to clear
  // the filters instead of inviting a first milestone.
  let allFilteredOut = $derived(
    isGlobalView && filteredMilestones.length > 0 && visibleMilestones.length === 0
  );
  let listFiltersAreDefault = $derived(
    milestoneListFiltersAreDefault({
      statuses: selectedStatuses,
      workspaceKeys: selectedWorkspaceKeys,
      search: searchQuery
    })
  );

  // Workspace filter options: Global plus each workspace that has milestones
  // in the loaded list.
  let workspaceFilterItems = $derived([
    { key: GLOBAL_WORKSPACE_KEY, name: t('milestones.global') },
    ...milestoneWorkspaceOptions($milestonesStore, workspaces)
  ].map(({ key, name }) => ({
    id: `workspace-${key}`,
    type: 'checkbox',
    title: name,
    checked: selectedWorkspaceKeys.includes(key),
    closeOnSelect: false,
    testid: `milestone-workspace-filter-${key}`,
    onChange: (checked) => toggleWorkspaceFilter(key, checked)
  })));

  let workspaceFilterLabel = $derived.by(() => {
    if (selectedWorkspaceKeys.length === 0) return t('milestones.allWorkspaces');
    if (selectedWorkspaceKeys.length === 1) {
      return (
        workspaceFilterItems.find((item) => item.checked)?.title ??
        t('milestones.workspaceFilterCount', { count: 1 })
      );
    }
    return t('milestones.workspaceFilterCount', { count: selectedWorkspaceKeys.length });
  });

  let statusFilterItems = $derived(MILESTONE_STATUSES.map((status) => ({
    id: `status-${status}`,
    type: 'checkbox',
    title: getStatusInfo(status).label,
    checked: selectedStatuses.includes(status),
    closeOnSelect: false,
    testid: `milestone-status-filter-${status}`,
    onChange: (checked) => toggleStatusFilter(status, checked)
  })));

  let statusFilterLabel = $derived(
    selectedStatuses.length === 0
      ? t('milestones.allStatuses')
      : selectedStatuses.length === 1
        ? getStatusInfo(selectedStatuses[0]).label
        : t('milestones.statusFilterCount', { count: selectedStatuses.length })
  );

  function workspaceLabel(milestone) {
    if (milestone.is_global) return t('milestones.global');
    return (
      milestone.workspace_name ||
      workspaces.find((workspace) => String(workspace.id) === String(milestone.workspace_id))?.name ||
      `#${milestone.workspace_id}`
    );
  }

  let localMilestones = $derived(
    visibleMilestones.filter(m => !m.is_global)
  );

  let globalMilestones = $derived(
    visibleMilestones.filter(m => m.is_global)
  );

  // Whether the current user may reorder milestones in the active scope(s).
  // Mirrors the backend permission gates (workspaceItemEdit for local,
  // globalMilestoneManage / milestone.create for global). Workspace
  // milestones listed on the global page are reordered on their workspace
  // page, where their whole scope is shown together.
  const canReorderLocal = $derived(
    !isGlobalView && (
      $isSystemAdmin ||
      workspacePermissions.canAdminWorkspace(workspaceId) ||
      workspacePermissions.hasPermission(workspaceId, 'item.edit')
    )
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
    void columnSortActive;

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

        const canDrag = !columnSortActive && (scopeName === 'global' ? canReorderGlobal : canReorderLocal);
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
    // active scope and no column sort is active. The handle is the
    // pragmatic-drag-and-drop dragHandle.
    ...(canReorderGlobal || canReorderLocal) && !columnSortActive
      ? [{ key: 'reorder', label: '', width: 'w-10', slot: 'reorder' }]
      : [],
    // Data columns sort on header click on the global view only; the
    // workspace view keeps its manual order.
    {
      key: 'status',
      label: t('milestones.columnStatus'),
      width: 'w-40',
      slot: 'status',
      sortable: isGlobalView,
      sortValue: milestoneStatusSortValue
    },
    { 
      key: 'name', 
      label: t('milestones.columnMilestone'),
      slot: 'name',
      sortable: isGlobalView,
      sortValue: (milestone) => milestone.name
    },
    ...isGlobalView ? [{
      key: 'workspace',
      label: t('milestones.workspace'),
      width: 'w-48',
      slot: 'workspace',
      sortable: true,
      sortValue: workspaceLabel
    }] : [],
    { 
      key: 'target_date', 
      label: t('milestones.columnTargetDate'),
      width: 'w-40',
      sortable: isGlobalView,
      sortValue: (milestone) => timestampSortValue(milestone.target_date),
      render: (milestone) => {
        return formatDateShort(milestone.target_date) || '-';
      }
    },
    { 
      key: 'days_remaining', 
      label: t('milestones.columnTimeline'),
      width: 'w-48',
      slot: 'days_remaining',
      sortable: isGlobalView,
      sortValue: milestoneTimelineSortValue
    },
    ...isGlobalView ? [{
      key: 'created_at',
      label: t('milestones.columnCreated'),
      width: 'w-36',
      sortable: true,
      sortValue: (milestone) => timestampSortValue(milestone.created_at),
      render: (milestone) => formatDateShort(milestone.created_at),
      textColor: 'var(--ds-text-subtle)'
    }, {
      key: 'last_updated',
      label: t('milestones.columnUpdated'),
      width: 'w-36',
      sortable: true,
      sortValue: (milestone) => timestampSortValue(milestoneLastUpdated(milestone)),
      render: (milestone) => formatRelativeTime(milestoneLastUpdated(milestone)),
      textColor: 'var(--ds-text-subtle)'
    }] : [],
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
<div class="flex h-full min-h-0 overflow-hidden" style="background-color: var(--ds-surface);">
  <!-- Left Sidebar - Navigation (only in global view) -->
  {#if isGlobalView}
    <MilestoneNavigation />
  {/if}

  <!-- Main Content -->
  <div class="flex-1 min-h-0 overflow-y-auto">
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
            {#if !isGlobalView}
              <div data-testid="milestone-hide-completed-toggle" title={t('milestones.hideCompletedHelp')}>
                <Toggle
                  checked={hideCompleted}
                  label={t('milestones.hideCompleted')}
                  labelPosition="left"
                  onchange={handleHideCompletedChange}
                />
              </div>
            {/if}
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

      {#if isGlobalView}
        <div class="flex items-center gap-3 flex-wrap mb-4" data-testid="milestone-filters">
          <SearchInput
            bind:value={searchQuery}
            placeholder={t('milestones.searchMilestones')}
            class="flex-1 min-w-[200px] max-w-md"
            dataTestid="milestone-search"
          />
          <DropdownMenu
            items={workspaceFilterItems}
            triggerIcon={Building2}
            triggerText={workspaceFilterLabel}
            placement="bottom-start"
            maxWidth="max-w-xs"
            triggerClass="px-3 py-2 border"
            triggerStyle="border-color: var(--ds-border); color: var(--ds-text);"
            triggerTestid="milestone-workspace-filter"
          />
          <DropdownMenu
            items={statusFilterItems}
            triggerIcon={Milestone}
            triggerText={statusFilterLabel}
            placement="bottom-start"
            maxWidth="max-w-xs"
            triggerClass="px-3 py-2 border"
            triggerStyle="border-color: var(--ds-border); color: var(--ds-text);"
            triggerTestid="milestone-status-filter"
          />
        </div>
      {/if}


      {#snippet reorderCell(item)}
        {#if canReorderMilestoneRow(item, { canReorderGlobal, canReorderLocal })}
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
              title={markdownExcerpt(item.description) || undefined}
            >
              {item.name}
            </a>
          {/key}
        {/if}
      {/snippet}

      {#snippet workspaceCell(item)}
        {#if item}
          <span class="inline-flex items-center gap-1.5 text-sm" style="color: var(--ds-text-subtle);">
            {#if item.is_global}
              <Globe class="w-4 h-4 flex-shrink-0" />
            {:else}
              <Building2 class="w-4 h-4 flex-shrink-0" />
            {/if}
            {#if item.is_global || item.workspace_id == null}
              <span class="truncate">{workspaceLabel(item)}</span>
            {:else}
              <a href="/workspaces/{item.workspace_id}" class="truncate hover:underline" style="color: inherit;">{workspaceLabel(item)}</a>
            {/if}
          </span>
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
              <span class="text-sm text-ds-text-subtle">{t('milestones.noCategory')}</span>
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
              <span class="text-ds-text-subtlest">—</span>
            {/if}
          {/key}
        {:else}
          <span class="text-ds-text-subtlest">—</span>
        {/if}
      {/snippet}

      <!-- Empty State or DataTable -->
      {#if allFilteredOut}
        <EmptyState
          icon={Milestone}
          title={t('milestones.noMatchingMilestones')}
          description={t('milestones.noMatchingMilestonesDescription')}
        >
          {#snippet action()}
            {#if !listFiltersAreDefault}
              <Button variant="primary" onclick={clearListFilters} dataTestid="milestone-clear-filters">
                {t('milestones.clearFilters')}
              </Button>
            {/if}
          {/snippet}
        </EmptyState>
      {:else if visibleMilestones.length === 0}
        <EmptyState
          icon={Milestone}
          title={isGlobalView && activeCategoryId ? t('milestones.noMilestonesInCategory') : (listIsFiltered ? t('milestones.noVisibleMilestones') : t('milestones.noMilestones'))}
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
          bind:sort={globalSort}
          actionItems={buildMilestoneDropdownItems}
          class="rounded-xl border shadow-sm"
          rowAttrs={(item) => ({ 'data-milestone-row': item.id, 'data-milestone-scope': item.is_global ? 'global' : 'local', 'data-milestone-ws': item.workspace_id ?? '', 'data-milestone-category': item.category_id ?? '' })}
        >
        {#snippet reorder(item)}{@render reorderCell(item)}{/snippet}
        {#snippet name(item)}{@render nameCell(item)}{/snippet}
        {#snippet workspace(item)}{@render workspaceCell(item)}{/snippet}
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
