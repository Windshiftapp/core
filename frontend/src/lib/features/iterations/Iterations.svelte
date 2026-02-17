<script>
  import { onMount } from 'svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import {
    Calendar, CheckCircle, Clock, Plus, Edit, Trash2,
    Globe, Building2, Target
  } from 'lucide-svelte';
  import DataTable from '../../components/DataTable.svelte';
  import Button from '../../components/Button.svelte';
  import IterationModal from '../../dialogs/IterationModal.svelte';
  import IterationNavigation from './IterationNavigation.svelte';
  import { formatDateShort } from '../../utils/dateFormatter.js';
  import { api } from '../../api.js';
  import { permissionStore, isSystemAdmin } from '../../stores/permissions.svelte.js';
  import { currentRoute, navigate } from '../../router.js';
  import ColorDot from '../../components/ColorDot.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import SectionHeader from '../../layout/SectionHeader.svelte';
  import { toHotkeyString } from '../../utils/keyboardShortcuts.js';
  import EmptyState from '../../components/EmptyState.svelte';

  // Props for workspace-scoped view (backward compatibility)
  let { workspaceId = null, typeId = null } = $props();

  // Determine if this is global view (no workspaceId) or workspace-scoped
  const isGlobalView = $derived(!workspaceId);

  // Get active type from URL params (for global view)
  let activeTypeId = $derived(typeId || $currentRoute.params?.typeId || null);

  let iterations = $state([]);
  let iterationTypes = $state([]);
  let loading = $state(true);
  let showModal = $state(false);
  let editingIteration = $state(null);

  const canManageGlobal = $derived(
    permissionStore.hasPermissionKey('iteration.manage') || $isSystemAdmin
  );

  // Filter iterations by active type when in global view
  let filteredIterations = $derived.by(() => {
    if (!isGlobalView || !activeTypeId) {
      return iterations;
    }
    return iterations.filter(i => i.type_id === parseInt(activeTypeId));
  });

  let localIterations = $derived(
    filteredIterations.filter(i => !i.is_global)
  );

  let globalIterations = $derived(
    filteredIterations.filter(i => i.is_global)
  );

  let statusOptions = $derived([
    { value: 'planned', label: t('iterations.status.planned'), lozengeColor: 'grey', icon: Clock },
    { value: 'active', label: t('iterations.status.active'), lozengeColor: 'blue', icon: Target },
    { value: 'completed', label: t('iterations.status.completed'), lozengeColor: 'green', icon: CheckCircle },
    { value: 'cancelled', label: t('iterations.status.cancelled'), lozengeColor: 'red', icon: Target }
  ]);

  onMount(async () => {
    await loadData();
  });

  async function loadData() {
    try {
      loading = true;
      // In global view, load all iterations; in workspace view, filter by workspace
      const filters = isGlobalView
        ? {}
        : { workspace_id: workspaceId, include_global: true };
      const [iterationsData, typesData] = await Promise.all([
        api.iterations.getAll(filters),
        api.iterationTypes.getAll()
      ]);
      // Force reactivity by creating new array
      iterations = Array.isArray(iterationsData) ? [...iterationsData] : [];
      iterationTypes = Array.isArray(typesData) ? [...typesData] : [];
    } catch (error) {
      console.error('Failed to load iterations:', error);
    } finally {
      loading = false;
    }
  }

  function handleIterationClick(iteration) {
    const url = workspaceId
      ? `/iterations/${iteration.id}?workspaceId=${workspaceId}`
      : `/iterations/${iteration.id}`;
    navigate(url);
  }

  function startCreate() {
    console.log('startCreate called');
    editingIteration = null;
    showModal = true;
    console.log('showModal is now:', showModal);
  }

  function startEdit(iteration) {
    editingIteration = iteration;
    showModal = true;
  }

  async function handleSave(data) {
    try {
      if (editingIteration) {
        await api.iterations.update(editingIteration.id, data);
      } else {
        await api.iterations.create(data);
      }
      await loadData();
      showModal = false;
      editingIteration = null;
    } catch (error) {
      console.error('Failed to save iteration:', error);
      throw error; // Let modal handle the error display
    }
  }

  function handleCancel() {
    showModal = false;
    editingIteration = null;
  }

  async function deleteIteration(iteration) {
    if (confirm(t('iterations.confirmDelete', { name: iteration.name }))) {
      try {
        await api.iterations.delete(iteration.id);
        await loadData();
      } catch (error) {
        console.error('Failed to delete iteration:', error);
        alert(t('dialogs.alerts.failedToDelete', { error: error.message || error }));
      }
    }
  }

  function getStatusInfo(status) {
    return statusOptions.find(s => s.value === status) || statusOptions[0];
  }

  function buildIterationDropdownItems(iteration) {
    // Only allow editing/deleting if:
    // - It's a local iteration (user must have workspace.admin)
    // - It's a global iteration and user has iteration.manage permission
    const canEdit = !iteration.is_global || canManageGlobal;

    const items = [];
    if (canEdit) {
      items.push({
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: t('common.edit'),
        hoverClass: 'hover-bg',
        onClick: () => startEdit(iteration)
      });
      items.push({
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: t('common.delete'),
        color: '#dc2626',
        hoverClass: 'hover:bg-red-50',
        onClick: () => deleteIteration(iteration)
      });
    }
    return items;
  }

  function getDateRange(iteration) {
    const start = formatDateShort(iteration.start_date);
    const end = formatDateShort(iteration.end_date);
    return `${start} → ${end}`;
  }

  function isActive(iteration) {
    if (iteration.status !== 'active') return false;
    const today = new Date();
    const start = new Date(iteration.start_date);
    const end = new Date(iteration.end_date);
    return today >= start && today <= end;
  }

  function getDaysRemaining(iteration) {
    if (iteration.status === 'completed' || iteration.status === 'cancelled') return '';
    const today = new Date();
    const end = new Date(iteration.end_date);
    const diffTime = end - today;
    const diffDays = Math.ceil(diffTime / (1000 * 60 * 60 * 24));

    if (diffDays < 0) return t('iterations.daysOverdue', { count: Math.abs(diffDays) });
    if (diffDays === 0) return t('iterations.endsToday');
    if (diffDays === 1) return t('iterations.oneDayRemaining');
    return t('iterations.daysRemaining', { count: diffDays });
  }

  function isOverdue(endDate, status) {
    if (status === 'completed' || status === 'cancelled' || !endDate) return false;
    const today = new Date();
    const end = new Date(endDate);
    return end < today;
  }

  // DataTable configuration
  let columns = $derived([
    {
      key: 'status',
      label: t('common.status'),
      sortable: true,
      width: 'w-40',
      slot: 'status'
    },
    {
      key: 'name',
      label: t('common.name'),
      sortable: true,
      slot: 'name'
    },
    {
      key: 'type',
      label: t('common.type'),
      sortable: true,
      width: 'w-32',
      slot: 'type'
    },
    {
      key: 'date_range',
      label: t('iterations.dateRange'),
      sortable: true,
      width: 'w-48',
      slot: 'date_range'
    },
    {
      key: 'actions',
      label: '',
      width: 'w-16',
      sortable: false
    }
  ]);

  // No need for tableData wrapper - pass props directly
</script>

<!-- Main container with conditional two-panel layout for global view -->
<div class="flex min-h-screen" style="background-color: var(--ds-surface);">
  <!-- Left Sidebar - Navigation (only in global view) -->
  {#if isGlobalView}
    <IterationNavigation />
  {/if}

  <!-- Main Content -->
  <div class="flex-1">
    <div class="p-6">
      <!-- Header -->
      <SectionHeader
        title={t('iterations.title')}
        subtitle={isGlobalView
          ? (activeTypeId
              ? `Showing ${iterationTypes.find(type => type.id === parseInt(activeTypeId))?.name || 'filtered'} iterations`
              : t('iterations.subtitle'))
          : `${localIterations.length} ${t('sprints.local').toLowerCase()}, ${globalIterations.length} ${t('sprints.global').toLowerCase()}`}
        class="mb-6"
      >
        {#snippet actions()}
          <Button
            variant="primary"
            size="medium"
            icon={Plus}
            keyboardHint="A"
            hotkeyConfig={{ key: toHotkeyString('iterations', 'add'), guard: () => !showModal }}
            onclick={startCreate}
          >
            {t('iterations.createIteration')}
          </Button>
        {/snippet}
      </SectionHeader>

      {#snippet nameCell(item)}
        <span class="inline-block w-2 h-2 rounded-full {isActive(item) ? 'bg-green-500' : ''}" title={isActive(item) ? 'Currently active' : ''}></span>
        {#if item.is_global}
          <Globe class="w-4 h-4" style="color: var(--ds-text-subtle); min-width: 16px;" />
        {:else}
          <Building2 class="w-4 h-4" style="color: var(--ds-text-subtle); min-width: 16px;" />
        {/if}
        <span class="font-medium hover:underline cursor-pointer" style="color: var(--ds-text);">{item.name}</span>
      {/snippet}

      {#snippet typeCell(item)}
        {#if item.type_name}
          <span
            class="inline-flex items-center gap-1.5 px-2 py-1 rounded text-xs font-medium"
            style="background-color: {item.type_color || '#6b7280'}20; color: {item.type_color || '#6b7280'};"
          >
            <ColorDot color={item.type_color || '#6b7280'} />
            {item.type_name}
          </span>
        {:else}
          <span class="text-gray-400">-</span>
        {/if}
      {/snippet}

      {#snippet dateRangeCell(item)}
        <span class="text-sm" style="color: var(--ds-text);">{getDateRange(item)}</span>
        {#if getDaysRemaining(item)}
          <span class="text-xs text-gray-500">{getDaysRemaining(item)}</span>
        {/if}
      {/snippet}

      {#snippet statusCell(item)}
        {#if item}
          {@const statusInfo = getStatusInfo(item.status)}
          {@const overdue = isOverdue(item.end_date, item.status)}
          <Lozenge color={statusInfo.lozengeColor} text={statusInfo.label} />
          {#if overdue}
            <Lozenge color="red" text={t('iterations.overdue')} />
          {/if}
        {/if}
      {/snippet}

      <!-- Iterations Table -->
      {#if loading}
        <div class="flex items-center justify-center p-12">
          <div class="text-center" style="color: var(--ds-text-subtle);">
            {t('common.loading')}
          </div>
        </div>
      {:else if filteredIterations.length === 0}
        <EmptyState
          icon={Calendar}
          title={t('iterations.noIterations')}
          description={t('iterations.noIterations')}
        >
          {#snippet action()}
            <Button variant="primary" size="medium" icon={Plus} keyboardHint="A" onclick={startCreate}>
              {t('iterations.createIteration')}
            </Button>
          {/snippet}
        </EmptyState>
      {:else if isGlobalView}
        <div class="flex-1">
          {#key filteredIterations.length}
            <DataTable
              {columns}
              data={filteredIterations}
              actionItems={buildIterationDropdownItems}
              onRowClick={handleIterationClick}
              class="rounded-xl border shadow-sm"
            >
              <div slot="name" let:item class="flex items-center gap-2">{@render nameCell(item)}</div>
              <div slot="type" let:item>{@render typeCell(item)}</div>
              <div slot="date_range" let:item class="flex flex-col">{@render dateRangeCell(item)}</div>
              <div slot="status" let:item class="flex items-center gap-2">{@render statusCell(item)}</div>
            </DataTable>
          {/key}
        </div>
      {:else}
        <!-- Workspace view: split into local and global sections -->
        <div class="flex-1 space-y-6">
          {#if localIterations.length > 0}
            <section>
              <div class="flex items-center gap-3 mb-3">
                <Building2 class="w-5 h-5" style="color: var(--ds-interactive);" />
                <div>
                  <p class="font-semibold text-base" style="color: var(--ds-text);">{t('sprints.localIterations')}</p>
                  <p class="text-sm" style="color: var(--ds-text-subtle);">{t('sprints.localIterationDescription')}</p>
                </div>
              </div>
              {#key localIterations.length}
                <DataTable
                  {columns}
                  data={localIterations}
                  actionItems={buildIterationDropdownItems}
                  onRowClick={handleIterationClick}
                  class="rounded-xl border shadow-sm"
                >
                  <div slot="name" let:item class="flex items-center gap-2">{@render nameCell(item)}</div>
                  <div slot="type" let:item>{@render typeCell(item)}</div>
                  <div slot="date_range" let:item class="flex flex-col">{@render dateRangeCell(item)}</div>
                  <div slot="status" let:item class="flex items-center gap-2">{@render statusCell(item)}</div>
                </DataTable>
              {/key}
            </section>
          {/if}

          {#if globalIterations.length > 0}
            <section>
              <div class="flex items-center gap-3 mb-3">
                <Globe class="w-5 h-5" style="color: var(--ds-interactive);" />
                <div>
                  <p class="font-semibold text-base" style="color: var(--ds-text);">{t('sprints.globalIterations')}</p>
                  <p class="text-sm" style="color: var(--ds-text-subtle);">{t('sprints.globalIterationDescription')}</p>
                </div>
              </div>
              {#key globalIterations.length}
                <DataTable
                  {columns}
                  data={globalIterations}
                  actionItems={buildIterationDropdownItems}
                  onRowClick={handleIterationClick}
                  class="rounded-xl border shadow-sm"
                >
                  <div slot="name" let:item class="flex items-center gap-2">{@render nameCell(item)}</div>
                  <div slot="type" let:item>{@render typeCell(item)}</div>
                  <div slot="date_range" let:item class="flex flex-col">{@render dateRangeCell(item)}</div>
                  <div slot="status" let:item class="flex items-center gap-2">{@render statusCell(item)}</div>
                </DataTable>
              {/key}
            </section>
          {/if}
        </div>
      {/if}
    </div>
  </div>
</div>

<!-- Iteration Modal -->
{#if showModal}
  {console.log('Rendering IterationModal, props:', { iteration: editingIteration, workspaceId, iterationTypes, canManageGlobal })}
  <IterationModal
    iteration={editingIteration}
    {workspaceId}
    {iterationTypes}
    {canManageGlobal}
    onsave={handleSave}
    oncancel={handleCancel}
  />
{/if}

