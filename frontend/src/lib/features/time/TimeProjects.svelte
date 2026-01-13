<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import Button from '../../components/Button.svelte';
  import TimeProjectCategories from './TimeProjectCategories.svelte';
  import TimeProjectModal from '../../dialogs/TimeProjectModal.svelte';
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
  import PageHeader from '../../layout/PageHeader.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import { Plus, X, Briefcase, Edit, Trash2 } from 'lucide-svelte';
  import SearchInput from '../../components/SearchInput.svelte';
  import ColorDot from '../../components/ColorDot.svelte';
  import { createShortcutHandler, getShortcutDisplay } from '../../utils/keyboardShortcuts.js';

  let activeTab = 'projects';
  let projects = [];
  let customers = [];
  let categories = [];
  let showCreateForm = false;
  let editingProject = null;

  // Filter state
  let selectedCategoryId = null;
  let selectedStatuses = ['Active']; // Default to showing only active projects
  let searchQuery = '';
  let formData = {
    customer_id: '',
    category_id: '',
    name: '',
    description: '',
    status: 'Active',
    color: '',
    hourly_rate: 0,
    active: true
  };

  const statusOptions = ['Active', 'On Hold', 'Completed', 'Archived'];

  onMount(async () => {
    await Promise.all([loadProjects(), loadCustomers(), loadCategories()]);
  });

  async function loadProjects() {
    try {
      const result = await api.time.projects.getAll();
      projects = result || [];
    } catch (error) {
      console.error('Failed to load projects:', error);
      projects = [];
    }
  }

  async function loadCustomers() {
    try {
      const result = await api.time.customers.getAll();
      customers = result || [];
    } catch (error) {
      console.error('Failed to load customers:', error);
      customers = [];
    }
  }

  async function loadCategories() {
    try {
      const result = await api.time.projectCategories.getAll();
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
    editingProject = project;
    formData = {
      customer_id: project.customer_id || '',
      category_id: project.category_id || '',
      name: project.name,
      description: project.description || '',
      status: project.status || 'Active',
      color: project.color || '',
      hourly_rate: project.hourly_rate,
      active: project.active
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
      active: true
    };
  }

  function cancelForm() {
    showCreateForm = false;
    editingProject = null;
    resetForm();
  }

  async function saveProject() {
    try {
      const data = {
        ...formData,
        customer_id: formData.customer_id ? parseInt(formData.customer_id) : null,
        category_id: formData.category_id ? parseInt(formData.category_id) : null,
        hourly_rate: Number(formData.hourly_rate) || 0
      };

      if (editingProject) {
        await api.time.projects.update(editingProject.id, data);
      } else {
        await api.time.projects.create(data);
      }
      await loadProjects();
      cancelForm();
    } catch (error) {
      console.error('Failed to save project:', error);
      alert('Failed to save project: ' + (error.message || error));
    }
  }

  async function deleteProject(project) {
    if (confirm(`Are you sure you want to delete "${project.name}"?`)) {
      try {
        await api.time.projects.delete(project.id);
        await loadProjects();
      } catch (error) {
        console.error('Failed to delete project:', error);
      }
    }
  }

  function getCustomerName(customerId) {
    const customer = customers.find(c => c.id === customerId);
    return customer ? customer.name : 'Unknown Customer';
  }

  // Build dropdown items for filters
  $: categoryDropdownItems = [
    {
      id: 'all',
      title: 'All Categories',
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
  ];

  $: statusDropdownItems = statusOptions.map(status => ({
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
  }));

  // Reactive statement to get selected category name
  $: selectedCategoryName = (() => {
    if (selectedCategoryId === null) return 'All Categories';
    const category = categories.find(c => c.id === selectedCategoryId);
    return category ? category.name : 'All Categories';
  })();

  // Reactive statement to get status filter label
  $: statusFilterLabel = (() => {
    if (selectedStatuses.length === 0) return 'All Statuses';
    if (selectedStatuses.length === 1) return selectedStatuses[0];
    return `${selectedStatuses.length} statuses`;
  })();

  // Filter projects by all criteria
  $: filteredProjects = projects.filter(p => {
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
      const matchesCustomer = (p.customer_name || '').toLowerCase().includes(query);
      return matchesName || matchesDescription || matchesCustomer;
    }

    return true;
  });

  // Keyboard shortcut handler
  const handleGlobalKeydown = createShortcutHandler({
    addProject: () => {
      if (!showCreateForm && activeTab === 'projects') {
        startCreate();
      }
    }
  }, 'timeProjects');

  // DataTable columns configuration
  const projectColumns = [
    { key: 'name', label: 'Project', slot: 'project' },
    { key: 'category', label: 'Category', slot: 'category' },
    { key: 'customer', label: 'Customer', slot: 'customer' },
    { key: 'status', label: 'Status', slot: 'status' },
    { key: 'hourly_rate', label: 'Rate', slot: 'rate' },
    { key: 'actions', label: 'Actions' }
  ];

  // Build dropdown action items for each project
  function buildProjectDropdownItems(project) {
    return [
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: 'Edit',
        hoverClass: 'hover:bg-gray-100',
        onClick: () => startEdit(project)
      },
      {
        id: 'delete',
        type: 'danger',
        icon: Trash2,
        title: 'Delete',
        hoverClass: 'hover:bg-red-50',
        onClick: () => deleteProject(project)
      }
    ];
  }
</script>

<svelte:window onkeydown={handleGlobalKeydown} />

<!-- Header -->
<div class="mb-6">
  <PageHeader
    icon={Briefcase}
    title="Projects"
    subtitle="Manage global projects for time tracking across workspaces"
  >
    {#snippet actions()}
      {#if activeTab === 'projects'}
        <Button
          variant="primary"
          onclick={startCreate}
          icon={Plus}
          size="medium"
          keyboardHint={getShortcutDisplay('timeProjects', 'addProject')}
        >
          Add Project
        </Button>
      {/if}
    {/snippet}
  </PageHeader>

  <!-- Tabs -->
  <div class="border-b" style="border-color: var(--ds-border);">
    <div class="flex gap-6">
      <button
        class="px-1 py-3 text-sm font-medium transition-colors border-b-2 {activeTab === 'projects' ? 'border-blue-500 text-blue-600' : 'border-transparent'}"
        style="{activeTab !== 'projects' ? 'color: var(--ds-text-subtle);' : ''}"
        onclick={() => activeTab = 'projects'}
      >
        Projects
      </button>
      <button
        class="px-1 py-3 text-sm font-medium transition-colors border-b-2 {activeTab === 'categories' ? 'border-blue-500 text-blue-600' : 'border-transparent'}"
        style="{activeTab !== 'categories' ? 'color: var(--ds-text-subtle);' : ''}"
        onclick={() => activeTab = 'categories'}
      >
        Categories
      </button>
    </div>
  </div>

  <!-- Filters Bar (only show on projects tab) -->
  {#if activeTab === 'projects'}
    <div class="mt-4">
      <!-- Search and Filter Controls -->
      <div class="flex items-center gap-3 flex-wrap">
        <!-- Search -->
        <SearchInput
          bind:value={searchQuery}
          placeholder="Search projects..."
          class="flex-1 min-w-[200px] max-w-md"
        />

        <!-- Category Filter -->
        {#if categories.length > 0}
          {@const selectedCategory = selectedCategoryId !== null ? categories.find(c => c.id === selectedCategoryId) : null}
          <div class="flex items-center gap-2 px-4 py-2 rounded text-sm font-medium border cursor-pointer" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
            {#if selectedCategory}
              <div class="w-2.5 h-2.5 rounded-full" style="background-color: {selectedCategory.color};"></div>
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
    emptyMessage={selectedCategoryId !== null ? 'No projects in this category.' : 'No projects found. Create your first project to start tracking time.'}
    emptyIcon={Briefcase}
    actionItems={buildProjectDropdownItems}
  >
    <!-- Project name with color dot and description -->
    <div slot="project" let:item={project}>
      <div class="flex items-center gap-2">
        {#if project.color}
          <ColorDot color={project.color} size="md" />
        {/if}
        <div>
          <div class="font-semibold" style="color: var(--ds-text);">{project.name}</div>
          {#if project.description}
            <div class="text-sm mt-1" style="color: var(--ds-text-subtle);">{project.description}</div>
          {/if}
        </div>
      </div>
    </div>

    <!-- Category with color dot -->
    <div slot="category" let:item={project}>
      {#if project.category_name}
        <div class="flex items-center gap-2">
          {#if project.category_color}
            <div class="w-2.5 h-2.5 rounded-full flex-shrink-0" style="background-color: {project.category_color};"></div>
          {/if}
          <span class="text-sm" style="color: var(--ds-text);">{project.category_name}</span>
        </div>
      {:else}
        <span class="text-sm" style="color: var(--ds-text);">-</span>
      {/if}
    </div>

    <!-- Customer -->
    <div slot="customer" let:item={project}>
      <span class="text-sm" style="color: var(--ds-text);">
        {project.customer_name || '-'}
      </span>
    </div>

    <!-- Status badge -->
    <div slot="status" let:item={project}>
      <Lozenge
        color={project.status === 'Active' ? 'green' : project.status === 'On Hold' ? 'yellow' : project.status === 'Completed' ? 'blue' : 'gray'}
        text={project.status || 'Active'}
      />
    </div>

    <!-- Hourly rate -->
    <div slot="rate" let:item={project}>
      {#if project.hourly_rate > 0}
        <span class="text-sm font-medium" style="color: var(--ds-text);">
          ${project.hourly_rate.toFixed(2)}/hr
        </span>
      {:else}
        <span class="text-sm" style="color: var(--ds-text-subtle);">-</span>
      {/if}
    </div>
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
  on:save={saveProject}
  on:cancel={cancelForm}
/>