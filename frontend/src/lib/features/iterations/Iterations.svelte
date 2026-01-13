<script>
  import { onMount, onDestroy } from 'svelte';
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
  import SectionHeader from '../../layout/SectionHeader.svelte';

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

  const statusOptions = [
    { value: 'planned', label: 'Planned', lozengeColor: 'grey', icon: Clock },
    { value: 'active', label: 'Active', lozengeColor: 'blue', icon: Target },
    { value: 'completed', label: 'Completed', lozengeColor: 'green', icon: CheckCircle },
    { value: 'cancelled', label: 'Cancelled', lozengeColor: 'red', icon: Target }
  ];

  onMount(async () => {
    await loadData();

    // Listen for manage iteration types event from navigation
    document.addEventListener('manage-iteration-types', handleManageTypes);

    // Add global keyboard listener for 'A' to add iteration
    window.addEventListener('keydown', handleGlobalKeydown);
  });

  onDestroy(() => {
    document.removeEventListener('manage-iteration-types', handleManageTypes);
    window.removeEventListener('keydown', handleGlobalKeydown);
  });

  function handleManageTypes() {
    // Navigate to admin panel iteration types tab
    navigate('/admin/iteration-types');
  }

  function handleGlobalKeydown(e) {
    // Check if we're in an input, textarea, or content-editable element
    const isInInputField = e.target.tagName === 'INPUT' ||
                          e.target.tagName === 'TEXTAREA' ||
                          e.target.contentEditable === 'true' ||
                          e.target.closest('[contenteditable="true"]');

    // "A" key for create iteration (only when not in input fields and no modifiers)
    if (e.key === 'a' && !e.ctrlKey && !e.metaKey && !e.altKey && !isInInputField && !showModal) {
      e.preventDefault();
      startCreate();
    }
  }

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
    if (isGlobalView) {
      navigate(`/iterations/${iteration.id}`);
    }
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

  async function handleSave(event) {
    const data = event.detail;
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
    if (confirm(`Are you sure you want to delete iteration "${iteration.name}"?`)) {
      try {
        await api.iterations.delete(iteration.id);
        await loadData();
      } catch (error) {
        console.error('Failed to delete iteration:', error);
        alert('Failed to delete iteration: ' + (error.message || error));
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
        title: 'Edit',
        hoverClass: 'hover:bg-gray-100',
        onClick: () => startEdit(iteration)
      });
      items.push({
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: 'Delete',
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

    if (diffDays < 0) return `${Math.abs(diffDays)} days overdue`;
    if (diffDays === 0) return 'Ends today';
    if (diffDays === 1) return '1 day left';
    return `${diffDays} days left`;
  }

  // DataTable configuration
  let columns = $derived([
    {
      key: 'name',
      label: 'Name',
      sortable: true,
      width: '25%',
      slot: 'name'
    },
    {
      key: 'type',
      label: 'Type',
      sortable: true,
      width: '15%',
      slot: 'type'
    },
    {
      key: 'date_range',
      label: 'Date Range',
      sortable: true,
      width: '20%',
      slot: 'date_range'
    },
    {
      key: 'status',
      label: 'Status',
      sortable: true,
      width: '12%',
      slot: 'status'
    },
    {
      key: 'scope',
      label: 'Scope',
      sortable: true,
      width: '15%',
      slot: 'scope'
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
        title="Iterations"
        subtitle={isGlobalView
          ? (activeTypeId
              ? `Showing ${iterationTypes.find(t => t.id === parseInt(activeTypeId))?.name || 'filtered'} iterations`
              : 'Manage sprints, PIs, and other agile iteration cycles')
          : 'Manage workspace iterations'}
        class="mb-6"
      >
        {#snippet actions()}
          <Button
            variant="primary"
            size="medium"
            icon={Plus}
            keyboardHint="A"
            onclick={startCreate}
          >
            Create Iteration
          </Button>
        {/snippet}
      </SectionHeader>

      <!-- Iterations Table -->
      {#if loading}
        <div class="flex items-center justify-center p-12">
          <div class="text-center" style="color: var(--ds-text-subtle);">
            Loading iterations...
          </div>
        </div>
      {:else if filteredIterations.length === 0}
        <div class="flex flex-col items-center justify-center p-12 border-2 border-dashed rounded" style="border-color: var(--ds-border);">
          <Calendar class="w-12 h-12 mb-4" style="color: var(--ds-text-subtle);" />
          <h3 class="text-lg font-medium mb-2" style="color: var(--ds-text);">No iterations yet</h3>
          <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
            {#if activeTypeId}
              No iterations found for this type
            {:else}
              Create your first iteration to start planning your work cycles
            {/if}
          </p>
          <Button
            variant="primary"
            size="medium"
            icon={Plus}
            keyboardHint="A"
            onclick={startCreate}
          >
            Create Iteration
          </Button>
        </div>
      {:else}
        <div class="flex-1 overflow-hidden">
          {#key filteredIterations.length}
            <DataTable
              {columns}
              data={filteredIterations}
              actionItems={buildIterationDropdownItems}
              onRowClick={isGlobalView ? handleIterationClick : undefined}
            >
              <div slot="name" let:item class="flex items-center gap-2">
                <!-- Always reserve space for active indicator for consistent alignment -->
                <span class="inline-block w-2 h-2 rounded-full {isActive(item) ? 'bg-green-500' : ''}" title={isActive(item) ? 'Currently active' : ''}></span>
                {#if item.is_global}
                  <Globe class="w-4 h-4" style="color: var(--ds-text-subtle); min-width: 16px;" />
                {:else}
                  <Building2 class="w-4 h-4" style="color: var(--ds-text-subtle); min-width: 16px;" />
                {/if}
                <span class="font-medium {isGlobalView ? 'hover:underline cursor-pointer' : ''}" style="color: var(--ds-text);">{item.name}</span>
              </div>

              <div slot="type" let:item>
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
              </div>

              <div slot="date_range" let:item class="flex flex-col">
                <span class="text-sm" style="color: var(--ds-text);">{getDateRange(item)}</span>
                {#if getDaysRemaining(item)}
                  <span class="text-xs text-gray-500">{getDaysRemaining(item)}</span>
                {/if}
              </div>

              <span slot="status" let:item class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium" style="{(() => {
                const statusInfo = getStatusInfo(item.status);
                const lozengeStyles = {
                  grey: 'background-color: #f3f4f6; color: #6b7280;',
                  blue: 'background-color: #dbeafe; color: #1e40af;',
                  green: 'background-color: #dcfce7; color: #166534;',
                  red: 'background-color: #fee2e2; color: #991b1b;'
                };
                return lozengeStyles[statusInfo.lozengeColor] || lozengeStyles.grey;
              })()}">
                {getStatusInfo(item.status).label}
              </span>

              <span slot="scope" let:item class="text-sm text-gray-600">
                {#if item.is_global}
                  Global
                {:else}
                  {item.workspace_name || 'This workspace'}
                {/if}
              </span>
            </DataTable>
          {/key}
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
