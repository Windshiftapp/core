<script>
  import { onMount } from 'svelte';
  import { createCombobox, melt } from '@melt-ui/svelte';
  import { fly } from 'svelte/transition';
  import { Check, SquareKanban, Filter, Plus, Edit, Trash2, MoreHorizontal } from 'lucide-svelte';
  import { api } from '../../api.js';
  import { navigate } from '../../router.js';
  import Button from '../../components/Button.svelte';
  import Input from '../../components/Input.svelte';
  import Select from '../../components/Select.svelte';
  import TimeTrackingOnboarding from './TimeTrackingOnboarding.svelte';
  import TimeLogModal from '../../dialogs/TimeLogModal.svelte';
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
  import {
    addMinutesToTime,
    createDurationSync,
    durationToString,
    minutesBetweenTimes,
    parseDuration
  } from '../../utils/timeUtils.js';
  import { getShortcut, matchesShortcut } from '../../utils/keyboardShortcuts.js';

  const submitShortcut = getShortcut('modal', 'submit');

  let worklogs = $state([]);
  let customers = $state([]);
  let projects = $state([]);
  let workItems = $state([]); // Available work items for selection
  let workspaces = $state([]); // Available workspaces for project lookup
  let editingWorklog = $state(null);
  let projectAutoSelected = $state(false);
  let showWorkItemSelector = $state(false);
  let showOnboarding = $state(false);
  let showTimeLogModal = $state(false);
  const durationSync = createDurationSync(); // Guarded updates for time/duration sync
  // Always show the quick entry form
  let filters = $state({
    customer_id: '',
    project_id: '',
    date_from: '',
    date_to: ''
  });
  let formData = $state({
    project_id: null,
    item_id: null, // Optional work item association
    description: '',
    date: new Date().toISOString().split('T')[0], // Today's date
    start_time: '',
    end_time: '',
    duration: ''
  });

  onMount(async () => {
    await Promise.all([loadWorklogs(), loadCustomers(), loadProjects(), loadWorkItems(), loadWorkspaces()]);
    
    // Check if onboarding should be shown
    if (customers.length === 0 && projects.length === 0) {
      showOnboarding = true;
    }
    
    // Set default date range filter to current month
    const now = new Date();
    filters.date_from = new Date(now.getFullYear(), now.getMonth(), 1).toISOString().split('T')[0];
    filters.date_to = new Date(now.getFullYear(), now.getMonth() + 1, 0).toISOString().split('T')[0];
    await loadWorklogs();
    
    // Listen for command palette events
    window.addEventListener('focus-time-entry-form', focusTimeEntryForm);
    
    // Add keyboard shortcuts
    function handleKeydown(event) {
      // Don't trigger shortcuts when modal is open or when typing in input fields
      if (showTimeLogModal) return;
      if (event.target.tagName === 'INPUT' || event.target.tagName === 'TEXTAREA' || event.target.tagName === 'SELECT') return;

      if (event.key === 'a' || event.key === 'A') {
        event.preventDefault();
        openLogTimeModal();
      }
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'l') {
        event.preventDefault();
        showWorkItemSelector = !showWorkItemSelector;
      }
      if (matchesShortcut(event, submitShortcut)) {
        event.preventDefault();
        if (formData.project_id && formData.description.trim() && formData.duration) {
          saveWorklog();
        }
      }
    }
    
    window.addEventListener('keydown', handleKeydown);
    
    return () => {
      window.removeEventListener('focus-time-entry-form', focusTimeEntryForm);
      window.removeEventListener('keydown', handleKeydown);
    };
  });

  async function loadWorklogs() {
    try {
      const result = await api.time.worklogs.getAll(filters);
      worklogs = result || [];
    } catch (error) {
      console.error('Failed to load worklogs:', error);
      worklogs = [];
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

  async function loadProjects() {
    try {
      const result = await api.time.projects.getAll();
      projects = result || [];
    } catch (error) {
      console.error('Failed to load projects:', error);
      projects = [];
    }
  }

  async function loadWorkItems() {
    try {
      // Load recent work items (limit to recent ones for performance)
      const result = await api.items.getAll({ limit: 100 });
      workItems = result.items || [];
    } catch (error) {
      console.error('Failed to load work items:', error);
      workItems = [];
    }
  }

  async function loadWorkspaces() {
    try {
      workspaces = await api.workspaces.getAll() || [];
    } catch (error) {
      console.error('Failed to load workspaces:', error);
      workspaces = [];
    }
  }

  function getWorkspaceDefaultProject(workItem) {
    if (!workItem || !workItem.workspace_id) return null;
    
    const workspace = workspaces.find(w => w.id === workItem.workspace_id);
    return workspace?.time_project_id || null;
  }

  function buildWorklogDropdownItems(worklog) {
    return [
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: 'Edit',
        hoverClass: 'hover:bg-gray-100',
        onClick: () => editWorklog(worklog)
      },
      {
        id: 'delete',
        type: 'danger',
        icon: Trash2,
        title: 'Delete',
        hoverClass: 'hover:bg-red-50',
        onClick: () => deleteWorklog(worklog)
      }
    ];
  }

  function resetForm() {
    editingWorklog = null;
    projectAutoSelected = false;
    showWorkItemSelector = false;
    formData = {
      project_id: null,
      item_id: null,
      description: '',
      date: new Date().toISOString().split('T')[0],
      start_time: '',
      end_time: '',
      duration: ''
    };
  }

  async function saveWorklog() {
    try {
      const data = {
        project_id: parseInt(formData.project_id),
        item_id: formData.item_id ? parseInt(formData.item_id) : undefined,
        description: formData.description,
        date: formData.date,
        start_time: formData.start_time || undefined,
        end_time: formData.end_time || undefined,
        duration: formData.duration || undefined
      };

      if (editingWorklog) {
        await api.time.worklogs.update(editingWorklog.id, data);
      } else {
        await api.time.worklogs.create(data);
      }
      await loadWorklogs();
      resetForm();
    } catch (error) {
      console.error('Failed to save worklog:', error);
      alert('Failed to save time entry. Please check your input.');
    }
  }

  async function handleModalSave(event) {
    try {
      const data = event.detail;

      if (editingWorklog) {
        await api.time.worklogs.update(editingWorklog.id, data);
      } else {
        await api.time.worklogs.create(data);
      }
      await loadWorklogs();
      showTimeLogModal = false;
      resetForm();
    } catch (error) {
      console.error('Failed to save worklog:', error);
      alert('Failed to save time entry. Please check your input.');
    }
  }

  function handleModalCancel() {
    showTimeLogModal = false;
    resetForm();
  }

  function openLogTimeModal() {
    resetForm();
    showTimeLogModal = true;
  }

  async function deleteWorklog(worklog) {
    if (confirm('Are you sure you want to delete this time entry?')) {
      try {
        await api.time.worklogs.delete(worklog.id);
        await loadWorklogs();
      } catch (error) {
        console.error('Failed to delete worklog:', error);
      }
    }
  }

  function editWorklog(worklog) {
    editingWorklog = worklog;
    formData = {
      project_id: worklog.project_id,
      item_id: worklog.item_id || null,
      description: worklog.description,
      date: new Date(worklog.date * 1000).toISOString().split('T')[0],
      start_time: formatTime(worklog.start_time).substring(0, 5), // Remove seconds
      end_time: formatTime(worklog.end_time).substring(0, 5),
      duration: formatDuration(worklog.duration_minutes)
    };
    
    // Set project combobox value
    const selectedProject = projectOptions.find(p => p.id === worklog.project_id);
    if (selectedProject) {
      $projectInputValue = selectedProject.name;
    }

    // Set work item combobox value
    if (formData.item_id) {
      const selectedItem = workItemOptions.find(w => w.id === worklog.item_id);
      if (selectedItem) {
        $workItemInputValue = selectedItem.title;
      }
    }
  }

  function getProjectName(projectId) {
    const project = projects.find(p => p.id === projectId);
    return project ? project.name : 'Unknown Project';
  }

  function getWorkItemTitle(itemId) {
    const workItem = workItems.find(w => w.id === itemId);
    return workItem ? workItem.title : 'Unknown Work Item';
  }

  function formatTime(unixTimestamp) {
    const date = new Date(unixTimestamp * 1000);
    return date.toLocaleTimeString('en-US', { 
      hour: '2-digit', 
      minute: '2-digit',
      hour12: false 
    });
  }

  function formatDuration(minutes) {
    const hours = Math.floor(minutes / 60);
    const mins = minutes % 60;
    if (hours === 0) return `${mins}m`;
    if (mins === 0) return `${hours}h`;
    return `${hours}h ${mins}m`;
  }

  // Handle start time changes - update end time if duration is set
  function onStartTimeChange() {
    durationSync.guard(() => {
      if (formData.start_time && formData.duration) {
        const durationMinutes = parseDuration(formData.duration);
        if (durationMinutes > 0) {
          formData.end_time = addMinutesToTime(formData.start_time, durationMinutes);
        }
      }
    });
  }
  
  // Handle duration changes - update end time if start time is set
  function onDurationChange() {
    durationSync.guard(() => {
      if (formData.start_time && formData.duration) {
        const durationMinutes = parseDuration(formData.duration);
        if (durationMinutes > 0) {
          formData.end_time = addMinutesToTime(formData.start_time, durationMinutes);
        }
      }
    });
  }
  
  // Handle end time changes - update duration if start time is set
  function onEndTimeChange() {
    durationSync.guard(() => {
      if (formData.start_time && formData.end_time) {
        const durationMinutes = minutesBetweenTimes(formData.start_time, formData.end_time);
        if (durationMinutes > 0) {
          formData.duration = durationToString(durationMinutes);
        }
      }
    });
  }

  async function applyFilters() {
    await loadWorklogs();
  }

  function clearFilters() {
    filters = {
      customer_id: '',
      project_id: '',
      date_from: '',
      date_to: ''
    };
    loadWorklogs();
  }

  function focusTimeEntryForm() {
    // Focus on the description field and scroll to form
    setTimeout(() => {
      const descriptionInput = document.querySelector('input[placeholder="What have you worked on?"]');
      if (descriptionInput) {
        descriptionInput.focus();
        descriptionInput.scrollIntoView({ behavior: 'smooth', block: 'center' });
      }
    }, 100);
  }

  function navigateToItem(workspaceId, itemId) {
    if (workspaceId && itemId) {
      navigate(`/workspaces/${workspaceId}/items/${itemId}`);
    }
  }

  function handleOnboardingCancel() {
    // Simply close the onboarding dialog
    showOnboarding = false;
  }

  async function handleOnboardingCompleted(event) {
    const { customer, project } = event.detail;

    // Reload data to include the newly created customer and project
    await Promise.all([loadCustomers(), loadProjects()]);

    // Pre-select the created project in the form
    formData.project_id = project.id;
    
    // Update project combobox display value
    const selectedProject = projectOptions.find(p => p.id === project.id);
    if (selectedProject) {
      $projectInputValue = selectedProject.name;
    }
    
    // Hide onboarding
    showOnboarding = false;
    
    // Focus the description field to start time entry
    setTimeout(() => {
      focusTimeEntryForm();
    }, 500);
  }

  // Removed quick entry presets - users can type directly

  const activeProjects = $derived(projects.filter(p => p.active));
  const filteredProjects = $derived(filters.customer_id 
    ? activeProjects.filter(p => p.customer_id === parseInt(filters.customer_id))
    : activeProjects);

  // Prepare projects for combobox with customer info
  const projectOptions = $derived(activeProjects.map(project => {
    const customer = customers.find(c => c.id === project.customer_id);
    return {
      id: project.id,
      name: project.name,
      subtitle: customer?.name || 'Unknown Customer',
      project
    };
  }));

  // Prepare work items for combobox with workspace info
  const workItemOptions = $derived(workItems.map(item => ({
    id: item.id,
    title: item.title,
    subtitle: item.workspace_name || 'Unknown Workspace',
    status: item.status,
    item
  })));

  
  // Project combobox setup
  const {
    elements: { menu: projectMenu, input: projectInput, option: projectOption },
    states: { open: projectOpen, inputValue: projectInputValue, touchedInput: projectTouchedInput, selected: projectSelected },
    helpers: { isSelected: isProjectSelected }
  } = createCombobox({
    forceVisible: true,
    preventScroll: false
  });

  // Work item combobox setup
  const {
    elements: { menu: workItemMenu, input: workItemInput, option: workItemOption },
    states: { open: workItemOpen, inputValue: workItemInputValue, touchedInput: workItemTouchedInput, selected: workItemSelected },
    helpers: { isSelected: isWorkItemSelected }
  } = createCombobox({
    forceVisible: true,
    preventScroll: false
  });

  // Handle project selection
  $effect(() => {
    if ($projectSelected) {
      const selectedProject = projectOptions.find(p => p.id === $projectSelected.value);
      if (selectedProject) {
        formData.project_id = selectedProject.id;
      }
    }
  });

  // Handle work item selection
  $effect(() => {
    if ($workItemSelected) {
      const selectedItem = workItemOptions.find(w => w.id === $workItemSelected.value);
      if (selectedItem) {
        formData.item_id = selectedItem.id;
        // Auto-populate description with item title if description is empty
        if (!formData.description.trim()) {
          formData.description = selectedItem.title;
        }
        
        // Auto-select workspace's default time tracking project if available and no project selected
        // Check work item's time_project_id first, then fall back to workspace default
        const projectId = selectedItem.item?.time_project_id || getWorkspaceDefaultProject(selectedItem.item);
        
        if (!formData.project_id && projectId) {
          formData.project_id = projectId;
          projectAutoSelected = true;
          
          // Update project combobox display value and selected state
          const selectedProject = projectOptions.find(p => p.id === projectId);
          if (selectedProject) {
            $projectInputValue = selectedProject.name;
            // Trigger project selection in combobox
            $projectSelected = { value: projectId, label: selectedProject.name, item: selectedProject };
          }
          
          // Auto-hide the indicator after 3 seconds
          setTimeout(() => {
            projectAutoSelected = false;
          }, 3000);
          
          // Close the selector after successful selection
          setTimeout(() => {
            showWorkItemSelector = false;
          }, 1500);
        }
      }
    }
  });

  // Set display value when project is selected externally
  $effect(() => {
    if (!$projectTouchedInput && formData.project_id) {
      const selectedProject = projectOptions.find(p => p.id === formData.project_id);
      if (selectedProject) {
        $projectInputValue = selectedProject.name;
      }
    } else if (!formData.project_id) {
      $projectInputValue = '';
    }
  });

  // Set display value when work item is selected externally
  $effect(() => {
    if (!$workItemTouchedInput && formData.item_id) {
      const selectedItem = workItemOptions.find(w => w.id === formData.item_id);
      if (selectedItem) {
        $workItemInputValue = selectedItem.title;
      }
    } else if (!formData.item_id) {
      $workItemInputValue = '';
    }
  });

  // Filter projects based on search input
  const filteredProjectsForDisplay = $derived.by(() => {
    if (!$projectTouchedInput || !$projectInputValue) {
      return projectOptions;
    }
    const search = $projectInputValue.toLowerCase();
    return projectOptions.filter(project =>
      project.name.toLowerCase().includes(search) ||
      project.subtitle?.toLowerCase().includes(search)
    );
  });

  // Filter work items based on search input
  const filteredWorkItemsForDisplay = $derived.by(() => {
    if (!$workItemTouchedInput || !$workItemInputValue) {
      return workItemOptions.slice(0, 20); // Limit initial display
    }
    const search = $workItemInputValue.toLowerCase();
    return workItemOptions.filter(item =>
      item.title.toLowerCase().includes(search) ||
      item.subtitle?.toLowerCase().includes(search)
    ).slice(0, 20); // Limit results
  });

  // Debug reactive statements
  $effect(() => {
  });
</script>

<!-- Header -->
<div class="mb-6 flex justify-between items-start">
  <div>
    <h2 class="text-lg font-semibold" style="color: var(--ds-text);">Time Entry</h2>
    <div class="text-xs mt-1" style="color: var(--ds-text-subtle);">
      Log your work hours and manage time entries
    </div>
  </div>
  <Button
    variant="primary"
    size="medium"
    icon={Plus}
    onclick={openLogTimeModal}
    keyboardHint="A"
    title="Add a new time entry"
  >
    Log Time
  </Button>
</div>

  {#if activeProjects.length === 0 && !showOnboarding}
    <div class="rounded-xl p-6 mb-6 border" style="background-color: var(--ds-background-warning); border-color: var(--ds-border-warning);">
      <p style="color: var(--ds-text-warning);">
        You need to create active projects before logging time.
        <a href="/time/projects" class="font-medium underline hover:opacity-80 transition-opacity" style="color: var(--ds-link);">Go to Projects</a>
        {#if customers.length === 0}
          or <button onclick={() => showOnboarding = true} class="font-medium underline hover:opacity-80 transition-opacity" style="color: var(--ds-link);">start the setup wizard</button>
        {/if}
      </p>
    </div>
  {/if}

  <!-- Filters -->
  <div class="rounded-xl p-6 mb-6 border shadow-sm" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
    <h3 class="text-sm font-semibold mb-4" style="color: var(--ds-text);">Filters</h3>
    <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
      <div>
        <label class="block text-xs font-medium mb-2" style="color: var(--ds-text-subtle);">Customer</label>
        <Select bind:value={filters.customer_id} size="small">
          <option value="">All customers</option>
          {#each customers as customer}
            <option value={customer.id}>{customer.name}</option>
          {/each}
        </Select>
      </div>
      <div>
        <label class="block text-xs font-medium mb-2" style="color: var(--ds-text-subtle);">Project</label>
        <Select bind:value={filters.project_id} size="small">
          <option value="">All projects</option>
          {#each filteredProjects as project}
            <option value={project.id}>{project.name}</option>
          {/each}
        </Select>
      </div>
      <div>
        <label class="block text-xs font-medium mb-2" style="color: var(--ds-text-subtle);">From Date</label>
        <Input type="date" bind:value={filters.date_from} size="small" />
      </div>
      <div>
        <label class="block text-xs font-medium mb-2" style="color: var(--ds-text-subtle);">To Date</label>
        <Input type="date" bind:value={filters.date_to} size="small" />
      </div>
    </div>
    <div class="mt-5 flex gap-3">
      <Button
        variant="primary"
        onclick={applyFilters}
        icon={Filter}
        size="medium"
        title="Apply the selected filters to the time entries list"
      >
        Apply Filters
      </Button>
      <Button
        variant="default"
        onclick={clearFilters}
        size="medium"
        title="Clear all filters and show all time entries"
      >
        Clear
      </Button>
    </div>
  </div>

<!-- Time Entries -->
  <div class="rounded-xl border shadow-sm overflow-hidden" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
    {#if worklogs.length === 0}
      <div class="p-8 text-center" style="color: var(--ds-text-subtle);">
        No time entries found. Log your first time entry to get started.
      </div>
    {:else}
      <div class="overflow-x-auto">
        <table class="w-full">
          <thead style="background-color: var(--ds-background-neutral);">
            <tr>
              <th class="px-6 py-4 text-left text-xs font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle);">Date</th>
              <th class="px-6 py-4 text-left text-xs font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle);">Project</th>
              <th class="px-6 py-4 text-left text-xs font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle);">Work Item</th>
              <th class="px-6 py-4 text-left text-xs font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle);">Description</th>
              <th class="px-6 py-4 text-left text-xs font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle);">Time</th>
              <th class="px-6 py-4 text-left text-xs font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle);">Duration</th>
              <th class="px-6 py-4 text-right text-xs font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle);">Actions</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-200">
            {#each worklogs as worklog (worklog.id)}
              <tr class="transition-colors duration-150 hover:bg-opacity-50" style="hover:background-color: var(--ds-background-neutral-hovered);">
                <td class="px-6 py-4 text-sm" style="color: var(--ds-text);">
                  {new Date(worklog.date * 1000).toLocaleDateString()}
                </td>
                <td class="px-6 py-4">
                  <div class="text-sm">
                    <div class="font-semibold" style="color: var(--ds-text);">{worklog.project_name}</div>
                    <div class="text-xs mt-1" style="color: var(--ds-text-subtle);">{worklog.customer_name}</div>
                  </div>
                </td>
                <td class="px-6 py-4 text-sm" style="color: var(--ds-text);">
                  {#if worklog.item_title && worklog.workspace_key && worklog.workspace_item_number}
                    <button
                      class="font-medium text-blue-600 hover:text-blue-800 cursor-pointer text-left hover:underline"
                      onclick={() => navigateToItem(worklog.workspace_id, worklog.item_id)}
                      title="Click to view {worklog.workspace_key}-{worklog.workspace_item_number}"
                    >
                      {worklog.workspace_key}-{worklog.workspace_item_number}: {worklog.item_title}
                    </button>
                  {:else}
                    <span class="text-gray-400 text-xs">—</span>
                  {/if}
                </td>
                <td class="px-6 py-4 text-sm" style="color: var(--ds-text);">
                  {worklog.description}
                </td>
                <td class="px-6 py-4 text-sm font-mono" style="color: var(--ds-text-subtle);">
                  {formatTime(worklog.start_time)} - {formatTime(worklog.end_time)}
                </td>
                <td class="px-6 py-4 text-sm font-semibold" style="color: var(--ds-text);">
                  {formatDuration(worklog.duration_minutes)}
                </td>
                <td class="px-6 py-4 text-right text-sm font-medium">
                  <DropdownMenu
                    items={buildWorklogDropdownItems(worklog)}
                    triggerIcon={MoreHorizontal}
                    showChevron={false}
                    iconOnly={true}
                    triggerClass="p-2 rounded-md hover:bg-gray-100 transition-colors duration-150"
                  />
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
      
      <!-- Summary -->
      <div class="px-6 py-4 border-t" style="background-color: var(--ds-background-neutral); border-color: var(--ds-border);">
        <div class="text-sm font-semibold" style="color: var(--ds-text);">
          Total Time: {formatDuration(worklogs.reduce((sum, w) => sum + w.duration_minutes, 0))}
          <span class="ml-2 font-normal" style="color: var(--ds-text-subtle);">({worklogs.length} entries)</span>
        </div>
      </div>
    {/if}
  </div>

<!-- Onboarding Modal -->
{#if showOnboarding}
  <TimeTrackingOnboarding oncompleted={handleOnboardingCompleted} oncancel={handleOnboardingCancel} />
{/if}

<!-- Time Log Modal -->
{#if showTimeLogModal}
  <TimeLogModal
    {projects}
    {customers}
    {workItems}
    {workspaces}
    showProjectField={true}
    showWorkItemField={true}
    onsave={handleModalSave}
    oncancel={handleModalCancel}
  />
{/if}
