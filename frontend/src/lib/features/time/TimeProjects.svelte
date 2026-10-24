<script>
  import { onMount } from 'svelte';
  import TabStrip from '../../components/TabStrip.svelte';
  import { time } from '../../api/time.js';
  import { customerOrganisations } from '../../api/portal.js';
  import Button from '../../components/Button.svelte';
  import TimeProjectCategories from './TimeProjectCategories.svelte';
  import TimeProjectModal from '../../dialogs/TimeProjectModal.svelte';
  import TimeProjectPermissionsModal from '../../dialogs/TimeProjectPermissionsModal.svelte';
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
  import PageHeader from '../../layout/PageHeader.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import Plus from '@lucide/svelte/icons/plus';
  import Briefcase from '@lucide/svelte/icons/briefcase';
  import Edit from '@lucide/svelte/icons/edit';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import Shield from '@lucide/svelte/icons/shield';
  import SearchInput from '../../components/SearchInput.svelte';
  import ColorDot from '../../components/ColorDot.svelte';
  import { toHotkeyString } from '../../utils/keyboardShortcuts.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { customerDisplayName, isNoCustomer } from '../../utils/customerLabel.js';
  import { errorToast } from '../../stores/toasts.svelte.js';
  import { permissionStore, isSystemAdmin } from '../../stores/permissions.svelte.js';
  import { confirm } from '../../composables/useConfirm.js';

  let activeTab = $state('projects');
  let projects = $state([]);
  let customers = $state([]);
  let categories = $state([]);
  let showCreateForm = $state(false);
  let editingProject = $state(null);
  let showPermissionsModal = $state(false);
  let permissionsProject = $state(null);

  const canManage = $derived($permissionStore.userPermissionKeys?.has('project.manage') || $isSystemAdmin);

  // Filter state
  let selectedCategoryId = $state(null);
  let selectedStatuses = $state(['Active']); // Default to showing only active projects
  let searchQuery = $state('');
  let formData = $state({
    customer_id: '',
    category_id: '',
    name: '',
    description: '',
    status: 'Active',
    color: '',
    hourly_rate: 0,
    settings: { max_hours: '' }
  });

  const statusOptions = ['Active', 'On Hold', 'Completed', 'Archived'];

  onMount(async () => {
    await Promise.all([loadProjects(), loadCustomers(), loadCategories()]);
  });

  async function loadProjects() {
    try {
      const result = await time.projects.getAll();
      projects = result || [];
    } catch (error) {
      console.error('Failed to load projects:', error);
      projects = [];
    }
  }

  async function loadCustomers() {
    try {
      const result = await customerOrganisations.getAll();
      customers = result || [];
    } catch (error) {
      console.error('Failed to load customers:', error);
      customers = [];
    }
  }

  async function loadCategories() {
    try {
      const result = await time.projectCategories.getAll();
      categories = result || [];
    } catch (error) {
      console.error('Failed to load categories:', error);
      categories = [];
    }
  }

  function startCreate() {
    showCreateForm = true;
    editingProject = null;
    resetForm();
  }

  function startEdit(project) {
    const noCustomerId = customers.find(isNoCustomer)?.id;
    editingProject = project;
    formData = {
      customer_id: project.customer_id && project.customer_id !== noCustomerId ? project.customer_id : '',
      category_id: project.category_id || '',
      name: project.name,
      description: project.description || '',
      status: project.status || 'Active',
      color: project.color || '',
      hourly_rate: project.hourly_rate,
      settings: { max_hours: project.settings?.max_hours || '' }
    };
    showCreateForm = true;
  }

  function resetForm() {
    formData = {
      customer_id: '',
      category_id: '',
      name: '',
      description: '',
      status: 'Active',
      color: '',
      hourly_rate: 0,
      settings: { max_hours: '' }
    };
  }

  function cancelForm() {
    showCreateForm = false;
    editingProject = null;
    resetForm();
  }

  async function saveProject() {
    try {
      // Build settings object - only include max_hours if it has a valid value
      const settings = {};
      const maxHoursValue = formData.settings?.max_hours;
      if (maxHoursValue !== '' && maxHoursValue !== null && maxHoursValue !== undefined) {
        const parsed = parseFloat(maxHoursValue);
        if (!isNaN(parsed) && parsed > 0) {
          settings.max_hours = parsed;
        }
      }

      const data = {
        ...formData,
        customer_id: formData.customer_id ? parseInt(formData.customer_id) : null,
        category_id: formData.category_id ? parseInt(formData.category_id) : null,
        hourly_rate: Number(formData.hourly_rate) || 0,
        settings: Object.keys(settings).length > 0 ? settings : null
      };

      if (editingProject) {
        await time.projects.update(editingProject.id, data);
      } else {
        await time.projects.create(data);
      }
      await loadProjects();
      cancelForm();
    } catch (error) {
      console.error('Failed to save project:', error);
      errorToast(t('time.projects.failedToSave') + ': ' + (error.message || error));
    }
  }

  async function deleteProject(project) {
    const confirmed = await confirm({
      title: t('time.projects.deleteProject'),
      message: t('time.projects.confirmDelete', { name: project.name }),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger'
    });

    if (confirmed) {
      try {
        await time.projects.delete(project.id);
        await loadProjects();
      } catch (error) {
        console.error('Failed to delete project:', error);
      }
    }
  }

  function projectCustomerName(project) {
    return customerDisplayName(project.customer_name, project.customer_builtin_key, t) || '-';
  }

  function openPermissions(project) {
    permissionsProject = project;
    showPermissionsModal = true;
  }

  function closePermissionsModal() {
    showPermissionsModal = false;
    permissionsProject = null;
  }

  // Build dropdown items for filters
  const categoryDropdownItems = $derived([
    {
      id: 'all',
      title: t('time.projects.allCategories'),
      checked: selectedCategoryId === null,
      onClick: () => { selectedCategoryId = null; }
    },
    ...categories.map(cat => ({
      id: cat.id,
      title: cat.name,
      color: cat.color,
      checked: selectedCategoryId === cat.id,
      onClick: () => { selectedCategoryId = cat.id; }
    }))
  ]);

  const statusDropdownItems = $derived(statusOptions.map(status => ({
    id: status,
    type: 'checkbox',
    title: status,
    checked: selectedStatuses.includes(status),
    onChange: (checked) => {
      if (checked) {
        selectedStatuses = [...selectedStatuses, status];
      } else {
        selectedStatuses = selectedStatuses.filter(s => s !== status);
      }
    }
  })));

  // Reactive statement to get selected category name
  const selectedCategoryName = $derived(
    selectedCategoryId === null 
      ? t('time.projects.allCategories')
      : (categories.find(c => c.id === selectedCategoryId)?.name || t('time.projects.allCategories'))
  );

  // Reactive statement to get status filter label
  const statusFilterLabel = $derived(
    selectedStatuses.length === 0
      ? t('time.projects.allStatuses')
      : selectedStatuses.length === 1
        ? selectedStatuses[0]
        : t('time.projects.statusCount', { count: selectedStatuses.length })
  );

  // Filter projects by all criteria
  const filteredProjects = $derived(projects.filter(p => {
    // Category filter
    if (selectedCategoryId !== null && p.category_id !== selectedCategoryId) {
      return false;
    }

    // Status filter
    if (selectedStatuses.length > 0 && !selectedStatuses.includes(p.status || 'Active')) {
      return false;
    }

    // Search filter
    if (searchQuery.trim()) {
      const query = searchQuery.toLowerCase();
      const matchesName = p.name.toLowerCase().includes(query);
      const matchesDescription = (p.description || '').toLowerCase().includes(query);
      const matchesCustomer = projectCustomerName(p).toLowerCase().includes(query);
      return matchesName || matchesDescription || matchesCustomer;
    }

    return true;
  }));

  // DataTable columns configuration - use $derived for reactivity
  const projectColumns = $derived([
    { key: 'name', label: t('time.projects.project'), slot: 'project' },
    { key: 'category', label: t('common.category'), slot: 'category' },
    { key: 'customer', label: t('time.projects.customer'), slot: 'customer' },
    { key: 'status', label: t('common.status'), slot: 'status' },
    { key: 'hourly_rate', label: t('time.projects.rate'), slot: 'rate' },
    { key: 'actions', label: t('common.actions') }
  ]);

  // Build dropdown action items for each project
  function buildProjectDropdownItems(project) {
    if (!canManage) return [];

    return [
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: t('common.edit'),
        hoverClass: 'hover-bg',
        onClick: () => startEdit(project)
      },
      {
        id: 'permissions',
        type: 'regular',
        icon: Shield,
        title: t('time.permissions.managePermissions'),
        hoverClass: 'hover-bg',
        onClick: () => openPermissions(project)
      },
      {
        id: 'delete',
        type: 'danger',
        icon: Trash2,
        title: t('common.delete'),
        hoverClass: 'hover-danger',
        onClick: () => deleteProject(project)
      }
    ];
  }
</script>

<!-- Header -->
<div class="mb-6">
  <PageHeader
    icon={Briefcase}
    title={t('time.projects.title')}
    subtitle={t('time.projects.subtitle')}
  >
    {#snippet actions()}
      {#if activeTab === 'projects' && canManage}
        <Button
          variant="primary"
          onclick={startCreate}
          icon={Plus}
          size="medium"
          keyboardHint="A"
          hotkeyConfig={{ key: toHotkeyString('timeProjects', 'addProject'), guard: () => !showCreateForm && activeTab === 'projects' }}
        >
          {t('time.projects.addProject')}
        </Button>
      {/if}
    {/snippet}
  </PageHeader>

  <!-- Tabs -->
  <TabStrip
    tabs={[
      { id: 'projects', label: t('time.projects.projectsTab') },
      { id: 'categories', label: t('time.projects.categoriesTab') }
    ]}
    bind:activeTab
  />

  <!-- Filters Bar (only show on projects tab) -->
  {#if activeTab === 'projects'}
    <div class="mt-4">
      <!-- Search and Filter Controls -->
      <div class="flex items-center gap-3 flex-wrap">
        <!-- Search -->
        <SearchInput
          bind:value={searchQuery}
          placeholder={t('time.projects.searchProjects')}
          class="flex-1 min-w-[200px] max-w-md"
        />

        <!-- Category Filter -->
        {#if categories.length > 0}
          {@const selectedCategory = selectedCategoryId !== null ? categories.find(c => c.id === selectedCategoryId) : null}
          <div class="flex items-center gap-2 px-4 py-2 rounded text-sm font-medium border cursor-pointer" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
            {#if selectedCategory}
              <ColorDot size="md" color={selectedCategory.color} />
            {/if}
            <DropdownMenu
              items={categoryDropdownItems}
              triggerText={selectedCategoryName}
              showChevron={true}
              triggerClass="!p-0 !border-0"
            />
          </div>
        {/if}

        <!-- Status Filter -->
        <DropdownMenu
          items={statusDropdownItems}
          triggerText={statusFilterLabel}
        />
      </div>
    </div>
  {/if}
</div>

{#if activeTab === 'projects'}
  <DataTable
    columns={projectColumns}
    data={filteredProjects}
    keyField="id"
    emptyMessage={selectedCategoryId !== null ? t('time.projects.noProjectsInCategory') : t('time.projects.noProjects')}
    emptyIcon={Briefcase}
    actionItems={buildProjectDropdownItems}
  >
    <!-- Project name with color dot and description -->
    {#snippet project(project)}
      <div class="flex items-center gap-2" data-testid={`time-project-${project.id}`}>
        {#if project.color}
          <ColorDot color={project.color} size="md" />
        {/if}
        <div>
          <div style="color: var(--ds-text);">{project.name}</div>
          {#if project.description}
            <div class="text-sm mt-1" style="color: var(--ds-text-subtle);">{project.description}</div>
          {/if}
        </div>
      </div>
    {/snippet}

    <!-- Category with color dot -->
    {#snippet category(project)}
      {#if project.category_name}
        <div class="flex items-center gap-2">
          {#if project.category_color}
            <ColorDot size="md" color={project.category_color} class="flex-shrink-0" />
          {/if}
          <span class="text-sm" style="color: var(--ds-text);">{project.category_name}</span>
        </div>
      {:else}
        <span class="text-sm" style="color: var(--ds-text);">-</span>
      {/if}
    {/snippet}

    <!-- Customer -->
    {#snippet customer(project)}
      <span class="text-sm" style="color: var(--ds-text);">
        {projectCustomerName(project)}
      </span>
    {/snippet}

    <!-- Status badge -->
    {#snippet status(project)}
      <Lozenge
        color={project.status === 'Active' ? 'green' : project.status === 'On Hold' ? 'yellow' : project.status === 'Completed' ? 'blue' : 'gray'}
        text={project.status || 'Active'}
      />
    {/snippet}

    <!-- Hourly rate -->
    {#snippet rate(project)}
      {#if project.hourly_rate > 0}
        <span class="text-sm" style="color: var(--ds-text);">
          ${project.hourly_rate.toFixed(2)}/hr
        </span>
      {:else}
        <span class="text-sm" style="color: var(--ds-text-subtle);">-</span>
      {/if}
    {/snippet}
  </DataTable>
{:else if activeTab === 'categories'}
  <TimeProjectCategories />
{/if}

<!-- Time Project Modal -->
<TimeProjectModal
  isOpen={showCreateForm}
  bind:formData
  {customers}
  {categories}
  {statusOptions}
  isEditing={!!editingProject}
  onsave={saveProject}
  oncancel={cancelForm}
/>

<!-- Time Project Permissions Modal -->
<TimeProjectPermissionsModal
  isOpen={showPermissionsModal}
  project={permissionsProject}
  onClose={closePermissionsModal}
/>

