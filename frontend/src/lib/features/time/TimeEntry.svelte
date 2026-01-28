<script>
  import { onMount } from 'svelte';
  import { Filter, Plus, Edit, Trash2, MoreHorizontal, AlertTriangle } from 'lucide-svelte';
  import { api } from '../../api.js';
  import { navigate } from '../../router.js';
  import Button from '../../components/Button.svelte';
  import Input from '../../components/Input.svelte';
  import Select from '../../components/Select.svelte';
  import TimeTrackingOnboarding from './TimeTrackingOnboarding.svelte';
  import TimeLogModal from '../../dialogs/TimeLogModal.svelte';
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
  import { toHotkeyString } from '../../utils/keyboardShortcuts.js';
  import { t } from '../../stores/i18n.svelte.js';

  let worklogs = $state([]);
  let customers = $state([]);
  let projects = $state([]);
  let workItems = $state([]);
  let workspaces = $state([]);
  let editingWorklog = $state(null);
  let showOnboarding = $state(false);
  let showTimeLogModal = $state(false);

  let filters = $state({
    customer_id: '',
    project_id: '',
    date_from: '',
    date_to: ''
  });

  onMount(async () => {
    await Promise.all([loadWorklogs(), loadCustomers(), loadProjects(), loadWorkItems(), loadWorkspaces()]);

    if (customers.length === 0 && projects.length === 0) {
      showOnboarding = true;
    }

    const now = new Date();
    filters.date_from = new Date(now.getFullYear(), now.getMonth(), 1).toISOString().split('T')[0];
    filters.date_to = new Date(now.getFullYear(), now.getMonth() + 1, 0).toISOString().split('T')[0];
    await loadWorklogs();
  });

  async function loadWorklogs() {
    try {
      worklogs = await api.time.worklogs.getAll(filters) || [];
    } catch (error) {
      console.error('Failed to load worklogs:', error);
      worklogs = [];
    }
  }

  async function loadCustomers() {
    try {
      customers = await api.time.customers.getAll() || [];
    } catch (error) {
      console.error('Failed to load customers:', error);
      customers = [];
    }
  }

  async function loadProjects() {
    try {
      projects = await api.time.projects.getAll() || [];
    } catch (error) {
      console.error('Failed to load projects:', error);
      projects = [];
    }
  }

  async function loadWorkItems() {
    try {
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

  function buildWorklogDropdownItems(worklog) {
    return [
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: t('common.edit'),
        hoverClass: 'hover-bg',
        onClick: () => editWorklog(worklog)
      },
      {
        id: 'delete',
        type: 'danger',
        icon: Trash2,
        title: t('common.delete'),
        hoverClass: 'hover:bg-red-50',
        onClick: () => deleteWorklog(worklog)
      }
    ];
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
      editingWorklog = null;
    } catch (error) {
      console.error('Failed to save worklog:', error);
      alert(t('time.entry.failedToSave'));
    }
  }

  function handleModalCancel() {
    showTimeLogModal = false;
    editingWorklog = null;
  }

  function openLogTimeModal() {
    editingWorklog = null;
    showTimeLogModal = true;
  }

  function editWorklog(worklog) {
    editingWorklog = worklog;
    showTimeLogModal = true;
  }

  async function deleteWorklog(worklog) {
    if (confirm(t('time.entry.confirmDelete'))) {
      try {
        await api.time.worklogs.delete(worklog.id);
        await loadWorklogs();
      } catch (error) {
        console.error('Failed to delete worklog:', error);
      }
    }
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

  function isProjectOverBudget(worklog) {
    if (!worklog.project_max_hours || worklog.project_max_hours <= 0) return false;
    return (worklog.project_total_hours || 0) > worklog.project_max_hours;
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

  function navigateToItem(workspaceId, itemId) {
    if (workspaceId && itemId) {
      navigate(`/workspaces/${workspaceId}/items/${itemId}`);
    }
  }

  function handleOnboardingCancel() {
    showOnboarding = false;
  }

  async function handleOnboardingCompleted(event) {
    await Promise.all([loadCustomers(), loadProjects()]);
    showOnboarding = false;
  }

  const activeProjects = $derived(projects.filter(p => p.active));
  const filteredProjects = $derived(filters.customer_id
    ? activeProjects.filter(p => p.customer_id === parseInt(filters.customer_id))
    : activeProjects);
</script>

<!-- Header -->
<div class="mb-6 flex justify-between items-start">
  <div>
    <h2 class="text-lg font-semibold" style="color: var(--ds-text);">{t('time.entry.title')}</h2>
    <div class="text-xs mt-1" style="color: var(--ds-text-subtle);">
      {t('time.entry.subtitle')}
    </div>
  </div>
  <Button
    variant="primary"
    size="medium"
    icon={Plus}
    onclick={openLogTimeModal}
    keyboardHint="A"
    hotkeyConfig={{ key: toHotkeyString('timeEntry', 'openLog'), guard: () => !showTimeLogModal }}
    title={t('time.entry.addTimeEntry')}
  >
    {t('time.logTime')}
  </Button>
</div>

{#if activeProjects.length === 0 && !showOnboarding}
  <div class="rounded-xl p-6 mb-6 border" style="background-color: var(--ds-background-warning); border-color: var(--ds-border-warning);">
    <p style="color: var(--ds-text-warning);">
      {t('time.entry.needProjects')}
      <a href="/time/projects" class="font-medium underline hover:opacity-80 transition-opacity" style="color: var(--ds-link);">{t('time.entry.goToProjects')}</a>
      {#if customers.length === 0}
        {t('common.or')} <button onclick={() => showOnboarding = true} class="font-medium underline hover:opacity-80 transition-opacity" style="color: var(--ds-link);">{t('time.entry.startSetupWizard')}</button>
      {/if}
    </p>
  </div>
{/if}

<!-- Filters -->
<div class="rounded-xl p-6 mb-6 border shadow-sm" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
  <h3 class="text-sm font-semibold mb-4" style="color: var(--ds-text);">{t('time.reports.filters')}</h3>
  <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
    <div>
      <label class="block text-xs font-medium mb-2" style="color: var(--ds-text-subtle);">{t('time.reports.customer')}</label>
      <Select bind:value={filters.customer_id} size="small">
        <option value="">{t('time.reports.allCustomers')}</option>
        {#each customers as customer}
          <option value={customer.id}>{customer.name}</option>
        {/each}
      </Select>
    </div>
    <div>
      <label class="block text-xs font-medium mb-2" style="color: var(--ds-text-subtle);">{t('time.reports.project')}</label>
      <Select bind:value={filters.project_id} size="small">
        <option value="">{t('time.reports.allProjects')}</option>
        {#each filteredProjects as project}
          <option value={project.id}>{project.name}</option>
        {/each}
      </Select>
    </div>
    <div>
      <label class="block text-xs font-medium mb-2" style="color: var(--ds-text-subtle);">{t('time.reports.fromDate')}</label>
      <Input type="date" bind:value={filters.date_from} size="small" />
    </div>
    <div>
      <label class="block text-xs font-medium mb-2" style="color: var(--ds-text-subtle);">{t('time.reports.toDate')}</label>
      <Input type="date" bind:value={filters.date_to} size="small" />
    </div>
  </div>
  <div class="mt-5 flex gap-3">
    <Button
      variant="primary"
      onclick={applyFilters}
      icon={Filter}
      size="medium"
      title={t('time.entry.applyFiltersTitle')}
    >
      {t('time.reports.applyFilters')}
    </Button>
    <Button
      variant="default"
      onclick={clearFilters}
      size="medium"
      title={t('time.entry.clearFiltersTitle')}
    >
      {t('common.clear')}
    </Button>
  </div>
</div>

<!-- Time Entries -->
<div class="rounded-xl border shadow-sm overflow-hidden" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
  {#if worklogs.length === 0}
    <div class="p-8 text-center" style="color: var(--ds-text-subtle);">
      {t('time.entry.noEntries')}
    </div>
  {:else}
    <div class="overflow-x-auto">
      <table class="w-full">
        <thead style="background-color: var(--ds-background-neutral);">
          <tr>
            <th class="px-6 py-4 text-left text-xs font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle);">{t('common.date')}</th>
            <th class="px-6 py-4 text-left text-xs font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle);">{t('time.reports.project')}</th>
            <th class="px-6 py-4 text-left text-xs font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle);">{t('items.workItem')}</th>
            <th class="px-6 py-4 text-left text-xs font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle);">{t('common.description')}</th>
            <th class="px-6 py-4 text-left text-xs font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle);">{t('common.time')}</th>
            <th class="px-6 py-4 text-left text-xs font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle);">{t('time.duration')}</th>
            <th class="px-6 py-4 text-right text-xs font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle);">{t('common.actions')}</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-200">
          {#each worklogs as worklog (worklog.id)}
            <tr class="transition-colors duration-150 hover:bg-opacity-50" style="hover:background-color: var(--ds-background-neutral-hovered);">
              <td class="px-6 py-4 text-sm" style="color: var(--ds-text);">
                {new Date(worklog.date * 1000).toLocaleDateString()}
              </td>
              <td class="px-6 py-4">
                <div class="flex items-center gap-2">
                  <div class="text-sm">
                    <div class="font-semibold" style="color: var(--ds-text);">{worklog.project_name}</div>
                    <div class="text-xs mt-1" style="color: var(--ds-text-subtle);">{worklog.customer_name}</div>
                  </div>
                  {#if isProjectOverBudget(worklog)}
                    <div title="{worklog.project_total_hours?.toFixed(1)}h / {worklog.project_max_hours?.toFixed(1)}h {t('time.entry.budgetExceeded')}">
                      <AlertTriangle size={16} class="text-amber-500" />
                    </div>
                  {/if}
                </div>
              </td>
              <td class="px-6 py-4 text-sm" style="color: var(--ds-text);">
                {#if worklog.item_title && worklog.workspace_key && worklog.workspace_item_number}
                  <button
                    class="font-medium text-blue-600 hover:text-blue-800 cursor-pointer text-left hover:underline"
                    onclick={() => navigateToItem(worklog.workspace_id, worklog.item_id)}
                    title={t('time.entry.clickToView', { key: worklog.workspace_key, number: worklog.workspace_item_number })}
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
                  triggerClass="p-2 rounded-md hover-bg transition-colors duration-150"
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
        {t('time.reports.totalTime')}: {formatDuration(worklogs.reduce((sum, w) => sum + w.duration_minutes, 0))}
        <span class="ml-2 font-normal" style="color: var(--ds-text-subtle);">({t('time.reports.entriesShown', { count: worklogs.length })})</span>
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
    {editingWorklog}
    onsave={handleModalSave}
    oncancel={handleModalCancel}
  />
{/if}

